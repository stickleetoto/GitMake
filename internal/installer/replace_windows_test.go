//go:build windows

package installer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceOrStageInstallsWithoutDeletingTheTargetFirst covers the install
// path that previously called os.Remove(target) before it knew a replacement
// could be put in place, and that reported a staged replacement which never
// actually ran.
func TestReplaceOrStageInstallsWithoutDeletingTheTargetFirst(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "gitmake.exe")
	tmp := target + ".new"
	if err := os.WriteFile(target, []byte("old-image"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmp, []byte("new-image"), 0o755); err != nil {
		t.Fatal(err)
	}

	staged, _, err := replaceOrStageWindows(tmp, target)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if staged {
		t.Fatal("an unlocked target must be replaced immediately, not staged")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-image" {
		t.Fatalf("installed content = %q", string(got))
	}
	if _, err := os.Stat(tmp); err == nil {
		t.Fatal("staging file was left behind after a successful install")
	}
}

func TestReplaceOrStageCreatesAFirstInstall(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "install", "gitmake.exe")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp := target + ".new"
	if err := os.WriteFile(tmp, []byte("first-image"), 0o755); err != nil {
		t.Fatal(err)
	}

	staged, _, err := replaceOrStageWindows(tmp, target)
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	if staged {
		t.Fatal("a first install must not be staged")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first-image" {
		t.Fatalf("installed content = %q", string(got))
	}
}
