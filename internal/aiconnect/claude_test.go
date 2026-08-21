package aiconnect

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitmake/internal/runner"
)

type fakeClaude struct {
	registered bool
	command    string
	args       []string
	scope      string
	health     string
	adds       int
	removes    int
}

func (f *fakeClaude) exec(name string, args ...string) (runner.Result, error) {
	if name != "claude" {
		return runner.Result{}, fmt.Errorf("unexpected command %s", name)
	}
	joined := strings.Join(args, " ")
	switch {
	case joined == "--version":
		return runner.Result{Stdout: "2.1.230 (Claude Code)", Code: 0}, nil
	case joined == "mcp get gitmake":
		if !f.registered {
			return runner.Result{Stderr: "No MCP server found with name: gitmake", Code: 1}, nil
		}
		scope := f.scope
		if scope == "" {
			scope = "User config"
		}
		return runner.Result{Stdout: fmt.Sprintf("gitmake:\n  Scope: %s\n  Type: stdio\n  Command: %s\n  Args: %s\n  Status: %s\n", scope, f.command, strings.Join(f.args, " "), f.health), Code: 0}, nil
	case joined == "mcp list":
		if !f.registered {
			return runner.Result{Stdout: "No MCP servers configured.", Code: 0}, nil
		}
		health := f.health
		if health == "" {
			health = "✔ Connected"
		}
		return runner.Result{Stdout: fmt.Sprintf("gitmake: %s %s (stdio) - %s", f.command, strings.Join(f.args, " "), health), Code: 0}, nil
	case joined == "mcp remove gitmake -s user":
		if !f.registered {
			return runner.Result{Stderr: "No MCP server found", Code: 1}, nil
		}
		f.registered = false
		f.removes++
		return runner.Result{Stdout: "Removed MCP server \"gitmake\" from user config", Code: 0}, nil
	default:
		prefix := "mcp add --transport stdio --scope user gitmake -- "
		if strings.HasPrefix(joined, prefix) {
			parts := args
			sep := -1
			for i, a := range parts {
				if a == "--" {
					sep = i
					break
				}
			}
			if sep < 0 || len(parts) < sep+3 {
				return runner.Result{Stderr: "bad add", Code: 2}, nil
			}
			f.command = parts[sep+1]
			f.args = append([]string(nil), parts[sep+2:]...)
			f.registered = true
			f.scope = "User config"
			f.health = "✔ Connected"
			f.adds++
			return runner.Result{Stdout: "Added stdio MCP server gitmake to user config", Code: 0}, nil
		}
	}
	return runner.Result{}, fmt.Errorf("unexpected claude args: %q", joined)
}

func TestSetupReadOnlyAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gitmake")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &fakeClaude{}
	m := Manager{GitMakePath: exe, Exec: fake.exec}

	st, changed, err := m.Setup(false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !st.Registered || st.Access != "read-only" || st.Health != "connected" {
		t.Fatalf("unexpected setup status: %+v changed=%v", st, changed)
	}
	if fake.adds != 1 || fake.removes != 0 {
		t.Fatalf("unexpected calls adds=%d removes=%d", fake.adds, fake.removes)
	}

	st, changed, err = m.Setup(false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("idempotent setup should not rewrite an already-correct registration")
	}
	if fake.adds != 1 || fake.removes != 0 {
		t.Fatalf("idempotent setup changed registration adds=%d removes=%d", fake.adds, fake.removes)
	}
}

func TestSetupCanUpgradeToWriteAccess(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gitmake")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &fakeClaude{registered: true, command: exe, args: []string{"mcp"}, scope: "User config", health: "✔ Connected"}
	m := Manager{GitMakePath: exe, Exec: fake.exec}

	st, changed, err := m.Setup(true)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || st.Access != "write" {
		t.Fatalf("expected write access after update: %+v changed=%v", st, changed)
	}
	if fake.removes != 1 || fake.adds != 1 {
		t.Fatalf("expected one replace, got adds=%d removes=%d", fake.adds, fake.removes)
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gitmake")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &fakeClaude{registered: true, command: exe, args: []string{"mcp"}, scope: "User config", health: "✔ Connected"}
	m := Manager{GitMakePath: exe, Exec: fake.exec}

	removed, st, err := m.Remove()
	if err != nil {
		t.Fatal(err)
	}
	if !removed || st.Registered {
		t.Fatalf("expected removal: removed=%v status=%+v", removed, st)
	}
	removed, _, err = m.Remove()
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("second remove should be a no-op")
	}
}

func TestRefusesNonUserScopedServer(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "gitmake")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := &fakeClaude{registered: true, command: exe, args: []string{"mcp"}, scope: "Project config", health: "✔ Connected"}
	m := Manager{GitMakePath: exe, Exec: fake.exec}

	if _, _, err := m.Setup(false); err == nil || !strings.Contains(err.Error(), "project") {
		t.Fatalf("expected scope refusal, got %v", err)
	}
}
