package aiconnect

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gitmake/internal/runner"
)

const ServerName = "gitmake"

type ExecFunc func(name string, args ...string) (runner.Result, error)

type Manager struct {
	GitMakePath string
	Exec        ExecFunc
	Verbose     bool
}

type Status struct {
	Schema         string   `json:"schema"`
	Client         string   `json:"client"`
	ClientDetected bool     `json:"client_detected"`
	ClientVersion  string   `json:"client_version,omitempty"`
	Server         string   `json:"server"`
	Registered     bool     `json:"registered"`
	Scope          string   `json:"scope,omitempty"`
	Access         string   `json:"access,omitempty"`
	Command        string   `json:"command,omitempty"`
	Args           []string `json:"args,omitempty"`
	Health         string   `json:"health,omitempty"`
	HealthDetail   string   `json:"health_detail,omitempty"`
}

func (m Manager) run(name string, args ...string) (runner.Result, error) {
	if m.Exec != nil {
		return m.Exec(name, args...)
	}
	r := runner.Runner{Verbose: m.Verbose}
	return r.Run("", name, args...)
}

func (m Manager) Status() (Status, error) {
	s := Status{Schema: "gitmake.ai-status/v1", Client: "claude-code", Server: ServerName, Access: "none", Health: "not_configured"}

	ver, err := m.run("claude", "--version")
	if err != nil {
		s.Health = "client_not_found"
		s.HealthDetail = err.Error()
		return s, nil
	}
	if ver.Code != 0 {
		s.ClientDetected = true
		s.Health = "client_error"
		s.HealthDetail = firstNonEmpty(ver.Stderr, ver.Stdout, fmt.Sprintf("exit %d", ver.Code))
		return s, nil
	}
	s.ClientDetected = true
	s.ClientVersion = firstLine(ver.Stdout)

	get, err := m.run("claude", "mcp", "get", ServerName)
	if err != nil {
		return s, fmt.Errorf("inspect Claude MCP server: %w", err)
	}
	if get.Code != 0 {
		// A missing server is a valid status, not a failure.
		s.HealthDetail = firstNonEmpty(get.Stderr, get.Stdout)
		return s, nil
	}

	s.Registered = true
	parseGetOutput(get.Stdout, &s)
	if containsArg(s.Args, "--allow-write") {
		s.Access = "write"
	} else {
		s.Access = "read-only"
	}
	s.Health = "configured"

	list, err := m.run("claude", "mcp", "list")
	if err == nil && list.Code == 0 {
		health, detail := parseListHealth(list.Stdout, ServerName)
		if health != "" {
			s.Health = health
			s.HealthDetail = detail
		}
	}
	return s, nil
}

func (m Manager) Setup(write bool) (Status, bool, error) {
	desiredPath := strings.TrimSpace(m.GitMakePath)
	if desiredPath == "" {
		return Status{}, false, fmt.Errorf("GitMake executable path is empty")
	}
	if abs, err := filepath.Abs(desiredPath); err == nil {
		desiredPath = abs
	}
	if st, err := os.Stat(desiredPath); err != nil || st.IsDir() {
		if err != nil {
			return Status{}, false, fmt.Errorf("GitMake executable for MCP is unavailable: %w", err)
		}
		return Status{}, false, fmt.Errorf("GitMake executable for MCP is a directory: %s", desiredPath)
	}

	before, err := m.Status()
	if err != nil {
		return before, false, err
	}
	if !before.ClientDetected {
		return before, false, fmt.Errorf("Claude Code CLI was not detected; install Claude Code or make `claude` available on PATH")
	}
	desiredArgs := []string{"mcp"}
	desiredAccess := "read-only"
	if write {
		desiredArgs = append(desiredArgs, "--allow-write")
		desiredAccess = "write"
	}

	if before.Registered {
		if before.Scope != "" && before.Scope != "user" {
			return before, false, fmt.Errorf("Claude MCP server %q already exists in %s scope; GitMake only manages the user-scoped server", ServerName, before.Scope)
		}
		if sameCommand(before.Command, desiredPath) && sameArgs(before.Args, desiredArgs) {
			if before.Health == "failed" {
				return before, false, fmt.Errorf("Claude MCP server is registered but failed to connect: %s", before.HealthDetail)
			}
			return before, false, nil
		}

		rm, err := m.run("claude", "mcp", "remove", ServerName, "-s", "user")
		if err != nil {
			return before, false, fmt.Errorf("remove previous Claude MCP registration: %w", err)
		}
		if rm.Code != 0 {
			return before, false, fmt.Errorf("remove previous Claude MCP registration: %s", firstNonEmpty(rm.Stderr, rm.Stdout, fmt.Sprintf("exit %d", rm.Code)))
		}
	}

	addArgs := []string{"mcp", "add", "--transport", "stdio", "--scope", "user", ServerName, "--", desiredPath}
	addArgs = append(addArgs, desiredArgs...)
	add, err := m.run("claude", addArgs...)
	if err != nil {
		return before, false, fmt.Errorf("register GitMake with Claude Code: %w", err)
	}
	if add.Code != 0 {
		return before, false, fmt.Errorf("register GitMake with Claude Code: %s", firstNonEmpty(add.Stderr, add.Stdout, fmt.Sprintf("exit %d", add.Code)))
	}

	after, err := m.Status()
	if err != nil {
		return after, true, err
	}
	if !after.Registered {
		return after, true, fmt.Errorf("Claude Code did not retain the GitMake MCP registration")
	}
	if after.Access != desiredAccess {
		return after, true, fmt.Errorf("Claude MCP access mismatch: got %s, expected %s", after.Access, desiredAccess)
	}
	if after.Command != "" && !sameCommand(after.Command, desiredPath) {
		return after, true, fmt.Errorf("Claude MCP command mismatch: got %q, expected %q", after.Command, desiredPath)
	}
	if after.Health == "failed" {
		return after, true, fmt.Errorf("Claude MCP server failed to connect: %s", after.HealthDetail)
	}
	return after, true, nil
}

func (m Manager) Remove() (bool, Status, error) {
	before, err := m.Status()
	if err != nil {
		return false, before, err
	}
	if !before.ClientDetected {
		return false, before, fmt.Errorf("Claude Code CLI was not detected; there is no Claude MCP registration to remove")
	}
	if !before.Registered {
		return false, before, nil
	}
	if before.Scope != "" && before.Scope != "user" {
		return false, before, fmt.Errorf("Claude MCP server %q is in %s scope; refusing to remove a server GitMake does not manage", ServerName, before.Scope)
	}

	rm, err := m.run("claude", "mcp", "remove", ServerName, "-s", "user")
	if err != nil {
		return false, before, fmt.Errorf("remove GitMake MCP registration: %w", err)
	}
	if rm.Code != 0 {
		return false, before, fmt.Errorf("remove GitMake MCP registration: %s", firstNonEmpty(rm.Stderr, rm.Stdout, fmt.Sprintf("exit %d", rm.Code)))
	}
	after, err := m.Status()
	if err != nil {
		return true, after, err
	}
	if after.Registered {
		return true, after, fmt.Errorf("GitMake MCP registration still exists after removal")
	}
	return true, after, nil
}

func parseGetOutput(text string, s *Status) {
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "scope:"):
			v := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, line[:len("Scope:")])))
			switch {
			case strings.Contains(v, "user"):
				s.Scope = "user"
			case strings.Contains(v, "project"):
				s.Scope = "project"
			case strings.Contains(v, "local"):
				s.Scope = "local"
			default:
				s.Scope = strings.TrimSpace(v)
			}
		case strings.HasPrefix(lower, "command:"):
			s.Command = strings.TrimSpace(line[len("Command:"):])
		case strings.HasPrefix(lower, "args:"):
			v := strings.TrimSpace(line[len("Args:"):])
			if v != "" {
				s.Args = strings.Fields(v)
			}
		case strings.HasPrefix(lower, "status:"):
			v := strings.TrimSpace(line[len("Status:"):])
			health, detail := classifyHealth(v)
			if health != "" {
				s.Health = health
				s.HealthDetail = detail
			}
		}
	}
}

func parseListHealth(text, name string) (string, string) {
	lowerName := strings.ToLower(name)
	for _, raw := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		lower := strings.ToLower(line)
		if !strings.Contains(lower, lowerName) {
			continue
		}
		if health, detail := classifyHealth(line); health != "" {
			return health, detail
		}
	}
	return "", ""
}

func classifyHealth(v string) (string, string) {
	lower := strings.ToLower(v)
	switch {
	case strings.Contains(lower, "failed") || strings.Contains(lower, "✘"):
		return "failed", strings.TrimSpace(v)
	case strings.Contains(lower, "needs authentication"):
		return "needs_auth", strings.TrimSpace(v)
	case strings.Contains(lower, "pending approval"):
		return "pending_approval", strings.TrimSpace(v)
	case strings.Contains(lower, "connected") || strings.Contains(lower, "✔") || strings.Contains(lower, "✓"):
		return "connected", strings.TrimSpace(v)
	case strings.Contains(lower, "cached"):
		return "cached", strings.TrimSpace(v)
	}
	return "", ""
}

func sameCommand(a, b string) bool {
	a = strings.Trim(strings.TrimSpace(a), `"`)
	b = strings.Trim(strings.TrimSpace(b), `"`)
	if a == "" || b == "" {
		return false
	}
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func sameArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func firstLine(v string) string {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		return strings.TrimSpace(v[:i])
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
