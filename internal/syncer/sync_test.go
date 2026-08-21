package syncer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotSyncPreservesDotGitAndProtectedPaths(t *testing.T) {
	repo := t.TempDir()
	src := t.TempDir()
	mustMkdir(t, filepath.Join(repo, ".git"))
	mustWrite(t, filepath.Join(repo, ".git", "keep"), "1")
	mustMkdir(t, filepath.Join(repo, ".github", "workflows"))
	mustWrite(t, filepath.Join(repo, ".github", "workflows", "ci.yml"), "keep")
	mustWrite(t, filepath.Join(repo, "old.txt"), "old")
	mustWrite(t, filepath.Join(src, "new.txt"), "new")

	if _, err := SyncSnapshot(src, repo, "snapshot", []string{".github/**", ".gitmake/**"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git", "keep")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".github", "workflows", "ci.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old.txt should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "new.txt")); err != nil {
		t.Fatal(err)
	}
}

func TestManagedSyncFirstAdoptionPreservesRemoteOnlyThenDeletesManaged(t *testing.T) {
	repo := t.TempDir()
	src1 := t.TempDir()
	mustMkdir(t, filepath.Join(repo, ".git"))
	mustWrite(t, filepath.Join(repo, "remote-only.txt"), "keep")
	mustWrite(t, filepath.Join(src1, "a.txt"), "a1")
	mustWrite(t, filepath.Join(src1, "old.txt"), "old")

	r, err := SyncSnapshot(src1, repo, "managed", []string{".github/**", ".gitmake/**"})
	if err != nil {
		t.Fatal(err)
	}
	if !r.FirstAdopt {
		t.Fatal("expected first adoption")
	}
	if _, err := os.Stat(filepath.Join(repo, "remote-only.txt")); err != nil {
		t.Fatal("remote-only file should survive first adoption")
	}

	src2 := t.TempDir()
	mustWrite(t, filepath.Join(src2, "a.txt"), "a2")
	r, err = SyncSnapshot(src2, repo, "managed", []string{".github/**", ".gitmake/**"})
	if err != nil {
		t.Fatal(err)
	}
	if r.FirstAdopt {
		t.Fatal("second sync should use manifest")
	}
	if _, err := os.Stat(filepath.Join(repo, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("previously managed old.txt should be deleted, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "remote-only.txt")); err != nil {
		t.Fatal("remote-only file should still survive")
	}
}

func TestSnapshotSyncPreservesNestedProtectedFile(t *testing.T) {
	repo := t.TempDir()
	src := t.TempDir()
	mustWrite(t, filepath.Join(repo, "docs", "keep.md"), "keep")
	mustWrite(t, filepath.Join(repo, "docs", "delete.md"), "delete")
	mustWrite(t, filepath.Join(src, "new.txt"), "new")
	if _, err := SyncSnapshot(src, repo, "snapshot", []string{"docs/keep.md", ".gitmake/**"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "keep.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs", "delete.md")); !os.IsNotExist(err) {
		t.Fatalf("delete.md should be removed, err=%v", err)
	}
}

func mustWrite(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}
func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
