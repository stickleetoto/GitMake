package gitops

import (
	"os"
	"path/filepath"
	"testing"

	"gitmake/internal/runner"
)

func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	r := runner.Runner{}
	res, err := r.Run(dir, "git", args...)
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != 0 {
		t.Fatalf("git %v failed: %s %s", args, res.Stdout, res.Stderr)
	}
}

func TestPrepareUpdateBranchEmptyRepo(t *testing.T) {
	dir := t.TempDir()
	mustRun(t, dir, "init")
	c := Client{Run: runner.Runner{}}
	branch, fallback, err := c.PrepareUpdateBranch(dir, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "main" || fallback {
		t.Fatalf("branch=%q fallback=%v", branch, fallback)
	}
}

func TestPrepareUpdateBranchFallsBackToDefault(t *testing.T) {
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	mustRun(t, root, "init", "--bare", remote)

	seed := filepath.Join(root, "seed")
	if err := os.Mkdir(seed, 0o755); err != nil {
		t.Fatal(err)
	}
	mustRun(t, seed, "init")
	mustRun(t, seed, "branch", "-M", "master")
	mustRun(t, seed, "config", "user.name", "Test")
	mustRun(t, seed, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(seed, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, seed, "add", "-A")
	mustRun(t, seed, "commit", "-m", "seed")
	mustRun(t, seed, "remote", "add", "origin", remote)
	mustRun(t, seed, "push", "-u", "origin", "master")
	mustRun(t, root, "--git-dir", remote, "symbolic-ref", "HEAD", "refs/heads/master")

	clone := filepath.Join(root, "clone")
	mustRun(t, root, "clone", remote, clone)
	c := Client{Run: runner.Runner{}}
	branch, fallback, err := c.PrepareUpdateBranch(clone, "main", "master")
	if err != nil {
		t.Fatal(err)
	}
	if branch != "master" || !fallback {
		t.Fatalf("branch=%q fallback=%v", branch, fallback)
	}
}
