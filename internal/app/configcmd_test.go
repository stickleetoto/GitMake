package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeJSONObjectsRecursiveAndNullDelete(t *testing.T) {
	base := []byte(`{"repo":{"name":"demo","visibility":"private"},"source":{"zip":"a.zip"},"release":{"enabled":false}}`)
	patch := []byte(`{"repo":{"visibility":"public"},"release":null}`)
	merged, err := mergeJSONObjects(base, patch)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(merged, &m); err != nil {
		t.Fatal(err)
	}
	repo := m["repo"].(map[string]any)
	if repo["name"] != "demo" || repo["visibility"] != "public" {
		t.Fatalf("unexpected repo merge: %#v", repo)
	}
	if _, ok := m["release"]; ok {
		t.Fatalf("null patch should delete release: %#v", m)
	}
}

func TestAtomicWriteFileReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gitmake.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(path, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("got %q", got)
	}
	if _, err := os.Stat(path + ".gitmake-backup"); !os.IsNotExist(err) {
		t.Fatalf("backup should be removed, err=%v", err)
	}
}

func TestParseFlagsAfterPositional(t *testing.T) {
	o, err := parseArgs([]string{"apply", "gm_123", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Command != "apply" || o.PlanID != "gm_123" || !o.JSON {
		t.Fatalf("unexpected parse: %#v", o)
	}
	o, err = parseArgs([]string{"Project.zip", "--dry-run", "--read-only", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if o.SourceArg != "Project.zip" || !o.DryRun || !o.ReadOnly || !o.JSON {
		t.Fatalf("unexpected publish parse: %#v", o)
	}
}

func TestPublishStdinFlagIsNotSilentlyIgnored(t *testing.T) {
	o, err := parseArgs([]string{"--stdin", "--dry-run", "--read-only", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Command != "publish" || !o.Stdin || !o.DryRun || !o.ReadOnly || !o.JSON {
		t.Fatalf("unexpected stdin publish parse: %#v", o)
	}
	if _, err := parseArgs([]string{"doctor", "--stdin"}); err == nil {
		t.Fatal("--stdin on unrelated commands must be rejected instead of ignored")
	}
	if _, err := parseArgs([]string{"plan", "--stdin"}); err == nil {
		t.Fatal("plan --stdin must be rejected because ephemeral config cannot be revalidated")
	}
}

func TestPreviewPseudoSubcommandGetsGuidance(t *testing.T) {
	_, err := parseArgs([]string{"preview"})
	if err == nil {
		t.Fatal("expected guidance for gitmake preview")
	}
	if got := err.Error(); !strings.Contains(got, "--dry-run --read-only") {
		t.Fatalf("unexpected guidance: %s", got)
	}
}
