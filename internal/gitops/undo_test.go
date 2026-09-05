package gitops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitmake/internal/runner"
)

// These drive real git against real repositories on disk. The three operations
// below are what `gitmake undo` is built on: it asks the remote where it
// actually is before touching anything, and it undoes by adding a commit
// rather than by removing one. Asserting that against a mock would prove
// nothing about whether git agrees.

func newClient() Client { return Client{Run: runner.Runner{}} }

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// repoPair builds a bare "remote" and a clone of it holding one commit.
func repoPair(t *testing.T) (clone, remote string, c Client) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	git(t, root, "init", "--bare", "--initial-branch=main", remote)

	clone = filepath.Join(root, "work")
	git(t, root, "clone", remote, clone)
	git(t, clone, "config", "user.name", "GitMake Test")
	git(t, clone, "config", "user.email", "test@example.invalid")
	// Windows checkouts convert to CRLF by default, which would make the
	// content assertions compare bytes the test never wrote.
	git(t, clone, "config", "core.autocrlf", "false")

	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-m", "base")
	git(t, clone, "push", "origin", "main")
	return clone, remote, newClient()
}

func TestRemoteHeadSHAMatchesThePushedCommit(t *testing.T) {
	clone, _, c := repoPair(t)

	local, err := c.HeadSHA(clone)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := c.RemoteHeadSHA(clone, "main")
	if err != nil {
		t.Fatal(err)
	}
	if remote != local {
		t.Fatalf("remote %q, local %q", remote, local)
	}
}

// TestRemoteHeadSHASeesACommitThisCloneDoesNot is the case undo actually cares
// about: somebody else pushed after GitMake did, so the recorded commit is no
// longer the tip and reverting would land on top of their work.
func TestRemoteHeadSHASeesACommitThisCloneDoesNot(t *testing.T) {
	clone, remote, c := repoPair(t)
	mine, err := c.HeadSHA(clone)
	if err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(t.TempDir(), "other")
	git(t, t.TempDir(), "clone", remote, other)
	git(t, other, "config", "user.name", "Someone Else")
	git(t, other, "config", "user.email", "other@example.invalid")
	if err := os.WriteFile(filepath.Join(other, "theirs.txt"), []byte("later work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, other, "add", "-A")
	git(t, other, "commit", "-m", "later work")
	git(t, other, "push", "origin", "main")

	got, err := c.RemoteHeadSHA(clone, "main")
	if err != nil {
		t.Fatal(err)
	}
	if got == mine {
		t.Fatal("remote head should have moved past the recorded commit")
	}
	if got != git(t, other, "rev-parse", "HEAD") {
		t.Fatalf("remote head %q does not match the other clone", got)
	}
}

func TestRemoteHeadSHAIsEmptyForAnUnknownBranch(t *testing.T) {
	clone, _, c := repoPair(t)
	got, err := c.RemoteHeadSHA(clone, "no-such-branch")
	if err != nil {
		t.Fatalf("an absent branch is an answer, not an error: %v", err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestRevertAddsACommitAndRestoresTheContent(t *testing.T) {
	clone, _, c := repoPair(t)
	before := git(t, clone, "rev-parse", "HEAD")

	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, "new.txt"), []byte("added\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-m", "publish")
	published := git(t, clone, "rev-parse", "HEAD")

	if err := c.Revert(clone, published, "Undo GitMake publish"); err != nil {
		t.Fatal(err)
	}

	// History grew; nothing was removed.
	if count := git(t, clone, "rev-list", "--count", "HEAD"); count != "3" {
		t.Fatalf("history has %s commits, want 3: a revert adds, never removes", count)
	}
	if !strings.Contains(git(t, clone, "log", "-1", "--format=%s"), "Undo GitMake publish") {
		t.Fatalf("revert commit does not carry GitMake's message: %s", git(t, clone, "log", "-1", "--format=%s"))
	}
	// The published commit is still reachable -- reverting does not unpublish.
	if git(t, clone, "cat-file", "-t", published) != "commit" {
		t.Fatal("the reverted commit must remain reachable; revert is not deletion")
	}

	// Content is back where it was.
	data, err := os.ReadFile(filepath.Join(clone, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "# base\n" {
		t.Fatalf("README = %q, want the pre-publish content", string(data))
	}
	if _, err := os.Stat(filepath.Join(clone, "new.txt")); !os.IsNotExist(err) {
		t.Fatal("a file added by the publish should be gone after the revert")
	}
	if got := git(t, clone, "rev-parse", "HEAD^{tree}"); got != git(t, clone, "rev-parse", before+"^{tree}") {
		t.Fatal("the tree after the revert should equal the tree before the publish")
	}
}

// TestRevertRefusesWhenThereIsNothingToUndo keeps a second undo from producing
// an empty commit or an error from inside git.
func TestRevertRefusesWhenThereIsNothingToUndo(t *testing.T) {
	clone, _, c := repoPair(t)
	if err := os.WriteFile(filepath.Join(clone, "x.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-m", "publish")
	published := git(t, clone, "rev-parse", "HEAD")

	if err := c.Revert(clone, published, "Undo"); err != nil {
		t.Fatal(err)
	}
	err := c.Revert(clone, published, "Undo again")
	if err == nil {
		t.Fatal("reverting an already-reverted commit must fail")
	}
	if !strings.Contains(err.Error(), "already undone") {
		t.Fatalf("error should say the commit is already undone, got %v", err)
	}
	// And the failed attempt must leave the tree clean for a retry.
	if status := git(t, clone, "status", "--porcelain"); status != "" {
		t.Fatalf("a refused revert left the tree dirty:\n%s", status)
	}
}

// TestRevertLeavesTheTreeCleanOnConflict matters because the clone is reused:
// a half-applied revert would make every later operation start from a dirty
// working tree.
func TestRevertLeavesTheTreeCleanOnConflict(t *testing.T) {
	clone, _, c := repoPair(t)

	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-m", "first")
	target := git(t, clone, "rev-parse", "HEAD")

	// A later commit touching the same lines makes the revert conflict.
	if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("# second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, clone, "add", "-A")
	git(t, clone, "commit", "-m", "second")

	if err := c.Revert(clone, target, "Undo"); err == nil {
		t.Fatal("a conflicting revert must fail rather than commit something half-applied")
	}
	if status := git(t, clone, "status", "--porcelain"); status != "" {
		t.Fatalf("a failed revert left the tree dirty:\n%s", status)
	}
}

func TestCommitExists(t *testing.T) {
	clone, _, c := repoPair(t)
	head := git(t, clone, "rev-parse", "HEAD")

	got, err := c.CommitExists(clone, head)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("HEAD should exist")
	}

	got, err = c.CommitExists(clone, "0123456789012345678901234567890123456789")
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("an unknown SHA should not report as present")
	}
}
