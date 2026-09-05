package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitmake/internal/history"
)

// Undo is the first command GitMake has that changes a repository without
// publishing anything, so what it refuses matters at least as much as what it
// does. These drive it end to end against the stubbed GitHub CLI and a bare
// repository on disk, so the remote can be inspected afterwards rather than
// assumed.

func undoNow(t *testing.T, o Options) (string, error) {
	t.Helper()
	o.Command = "undo"
	o.State = newPipeline(o)
	return captureOutput(func() error { return runUndo(o) })
}

// publishTwice creates the repository and then makes one ordinary update, so
// there is an undoable publish sitting on top of an earlier state.
func publishTwice(t *testing.T, project string) {
	t.Helper()
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Demo\n\nsecond revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "added.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatalf("second publish: %v", err)
	}
	recordHistory(o, nil)
}

func remoteFileContent(t *testing.T, target, path string) (string, bool) {
	t.Helper()
	owner, name, _ := strings.Cut(target, "/")
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")
	out, err := exec.Command("git", "--git-dir="+remote, "show", "HEAD:"+path).Output()
	if err != nil {
		return "", false
	}
	return string(out), true
}

func TestUndoRevertsTheLastPublish(t *testing.T) {
	project := publishEnv(t)
	publishTwice(t, project)

	before := remoteCommitCount(t, "testuser/Demo")
	out, err := undoNow(t, Options{Yes: true})
	if err != nil {
		t.Fatalf("undo: %v\n%s", err, out)
	}

	// A revert adds a commit; it never removes one.
	after := remoteCommitCount(t, "testuser/Demo")
	if before != "2" || after != "3" {
		t.Fatalf("commits went %s -> %s, want 2 -> 3: an undo adds a commit", before, after)
	}

	// The content is back to the first publish.
	readme, ok := remoteFileContent(t, "testuser/Demo", "README.md")
	if !ok {
		t.Fatal("README.md is missing from the remote after an undo")
	}
	if strings.Contains(readme, "second revision") {
		t.Fatalf("README was not reverted:\n%s", readme)
	}
	if _, ok := remoteFileContent(t, "testuser/Demo", "added.txt"); ok {
		t.Fatal("a file added by the undone publish is still on the remote")
	}
}

// TestUndoWarnsThatPublishedContentRemains is the honesty requirement. An undo
// that lets a user believe a leaked credential is gone is worse than no undo.
func TestUndoWarnsThatPublishedContentRemains(t *testing.T) {
	project := publishEnv(t)
	publishTwice(t, project)

	out, err := undoNow(t, Options{Yes: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"stays published", "rotate it"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Fatalf("undo output must say %q:\n%s", want, out)
		}
	}
}

// TestUndoRefusesWhenTheBranchHasMovedOn is the guard against reverting on top
// of work GitMake did not publish.
func TestUndoRefusesWhenTheBranchHasMovedOn(t *testing.T) {
	project := publishEnv(t)
	publishTwice(t, project)

	// Somebody else pushes after GitMake did.
	owner, name, _ := strings.Cut("testuser/Demo", "/")
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")
	other := filepath.Join(t.TempDir(), "other")
	for _, args := range [][]string{
		{"clone", remote, other},
		{"-C", other, "config", "user.name", "Someone Else"},
		{"-C", other, "config", "user.email", "other@example.test"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(other, "theirs.txt"), []byte("later work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", other, "add", "-A"},
		{"-C", other, "commit", "-m", "later work"},
		{"-C", other, "push", "origin", "HEAD"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	countBefore := remoteCommitCount(t, "testuser/Demo")

	out, err := undoNow(t, Options{Yes: true})
	if err == nil {
		t.Fatalf("undo must refuse once the branch has moved on:\n%s", out)
	}
	if !strings.Contains(err.Error(), "moved on") {
		t.Fatalf("error should explain the branch moved, got %v", err)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != countBefore {
		t.Fatalf("a refused undo changed the remote: %s -> %s", countBefore, got)
	}
	_ = project
}

// TestUndoRefusesAPublishThatCreatedTheRepository covers the case with no
// earlier state: GitMake does not delete repositories, so it says so.
func TestUndoRefusesAPublishThatCreatedTheRepository(t *testing.T) {
	publishEnv(t)

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatal(err)
	}
	recordHistory(o, nil)

	out, err := undoNow(t, Options{Yes: true})
	if err == nil {
		t.Fatalf("undoing a repository-creating publish must fail:\n%s", out)
	}
	if !strings.Contains(err.Error(), "no earlier state") {
		t.Fatalf("error should explain there is nothing to return to, got %v", err)
	}
	if !strings.Contains(err.Error(), "does not delete repositories") {
		t.Fatalf("error should say deletion is the user's to do, got %v", err)
	}
}

func TestUndoIsNotRepeatable(t *testing.T) {
	project := publishEnv(t)
	publishTwice(t, project)

	if _, err := undoNow(t, Options{Yes: true}); err != nil {
		t.Fatal(err)
	}
	count := remoteCommitCount(t, "testuser/Demo")

	_, err := undoNow(t, Options{Yes: true})
	if err == nil {
		t.Fatal("a second undo of the same publish must fail")
	}
	if !strings.Contains(err.Error(), "already been undone") {
		t.Fatalf("error should say the publish was already undone, got %v", err)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != count {
		t.Fatalf("the refused second undo still changed the remote: %s -> %s", count, got)
	}
}

// TestUndoDryRunChangesNothing is the promise --dry-run makes everywhere else.
func TestUndoDryRunChangesNothing(t *testing.T) {
	project := publishEnv(t)
	publishTwice(t, project)
	before := remoteCommitCount(t, "testuser/Demo")

	out, err := undoNow(t, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Dry run") {
		t.Fatalf("a dry run should say so:\n%s", out)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != before {
		t.Fatalf("a dry run changed the remote: %s -> %s", before, got)
	}
	// And it must not mark the entry, or the real undo afterwards would refuse.
	if _, err := undoNow(t, Options{Yes: true}); err != nil {
		t.Fatalf("undo after a dry run: %v", err)
	}
}

func TestUndoVerifiesItsOwnPush(t *testing.T) {
	project := publishEnv(t)
	publishTwice(t, project)

	o := Options{Yes: true}
	o.Command = "undo"
	o.State = newPipeline(o)
	out, err := captureOutput(func() error { return runUndo(o) })
	if err != nil {
		t.Fatalf("undo: %v\n%s", err, out)
	}
	v := o.State.Verification
	if v == nil || !v.Checked {
		t.Fatalf("undo did not verify its own push: %+v", v)
	}
	if !v.CommitMatches {
		t.Fatalf("undo verification did not match: %+v", v)
	}
}

// findUndoTarget carries the decisions above, so it is also checked directly:
// the end-to-end tests cannot easily produce every history shape.
func TestFindUndoTarget(t *testing.T) {
	now := time.Now().UTC()
	ok := history.Entry{Command: "publish", OK: true, Repository: "o/r", Commit: "abc", Branch: "main", Time: now}

	cases := []struct {
		name    string
		entries []history.Entry
		wantWhy string
	}{
		{name: "picks the newest usable publish", entries: []history.Entry{ok}},
		{name: "no history", entries: nil, wantWhy: "no completed publish"},
		{name: "skips a dry run", entries: []history.Entry{{Command: "publish", OK: true, DryRun: true, Commit: "x"}, ok}},
		{name: "skips a read-only run", entries: []history.Entry{{Command: "publish", OK: true, ReadOnly: true, Commit: "x"}, ok}},
		{name: "skips a failed publish", entries: []history.Entry{{Command: "publish", OK: false, Commit: "x"}, ok}},
		{name: "skips an unrelated command", entries: []history.Entry{{Command: "doctor", OK: true}, ok}},
		{name: "reports an already undone publish", entries: []history.Entry{{Command: "publish", OK: true, Undone: true, Commit: "x", Repository: "o/r"}, ok}, wantWhy: "already been undone"},
		{name: "reports a repository-creating publish", entries: []history.Entry{{Command: "publish", OK: true, RepoCreated: true, Commit: "x", Repository: "o/r"}, ok}, wantWhy: "no earlier state"},
		{name: "reports a publish that changed nothing", entries: []history.Entry{{Command: "publish", OK: true, Repository: "o/r"}, ok}, wantWhy: "nothing to undo"},
		{name: "accepts an applied plan", entries: []history.Entry{{Command: "apply", OK: true, Repository: "o/r", Commit: "def"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findUndoTarget(tc.entries)
			if tc.wantWhy == "" {
				if got.why != "" {
					t.Fatalf("expected a target, got refusal %q", got.why)
				}
				if got.entry.Commit == "" {
					t.Fatal("target has no commit")
				}
				return
			}
			if !strings.Contains(got.why, tc.wantWhy) {
				t.Fatalf("reason = %q, want it to contain %q", got.why, tc.wantWhy)
			}
		})
	}
}

// TestUndoUsesTheRealManagedBaselineForRisk pins the denominator.
//
// confirmUndo first passed no sync state to calculateRisk, which left
// ManagedBaseline at zero and made the destructive rule unreachable: an undo
// removing every file in a repository would have been offered as an ordinary
// [Y/n] question that --yes could answer. The baseline has to come from the
// manifest the previous run wrote.
func TestUndoUsesTheRealManagedBaselineForRisk(t *testing.T) {
	project := publishEnv(t)

	// A first publish with enough files for the destructive threshold, which
	// needs at least ten deletions and at least 30% of the baseline.
	for i := 0; i < 20; i++ {
		name := filepath.Join(project, fmt.Sprintf("mod%02d.go", i))
		if err := os.WriteFile(name, []byte("package demo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}

	// A second publish that adds many more files. Undoing it deletes them all.
	for i := 0; i < 20; i++ {
		name := filepath.Join(project, fmt.Sprintf("extra%02d.go", i))
		if err := os.WriteFile(name, []byte("package demo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatal(err)
	}
	recordHistory(o, nil)

	before := remoteCommitCount(t, "testuser/Demo")

	// No human is available to answer, and --yes must not answer for one.
	// The prompter is injected rather than inherited so the result does not
	// depend on whatever stdin the test runner was given.
	ask := &scriptedPrompter{interactive: false, answer: "y"}
	restore := undoPrompter
	undoPrompter = ask
	defer func() { undoPrompter = restore }()

	out, err := undoNow(t, Options{Yes: true})
	if err == nil {
		t.Fatalf("--yes accepted an undo that deletes most of the repository:\n%s", out)
	}
	if !strings.Contains(err.Error(), "interactive confirmation") {
		t.Fatalf("error should demand a human, got %v", err)
	}
	if len(ask.asked) != 0 {
		t.Fatalf("no prompt should be issued when confirmation is impossible: %v", ask.asked)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != before {
		t.Fatalf("a refused undo changed the remote: %s -> %s", before, got)
	}
}

// Machine error codes are part of the v1 contract, so what undo reports is
// pinned here rather than left to whatever code happened to be convenient.
func TestUndoReportsAccurateMachineCodes(t *testing.T) {
	t.Run("nothing to undo", func(t *testing.T) {
		publishEnv(t)
		o := Options{}
		o.Command = "publish"
		o.ConfigPath = "gitmake.json"
		o.State = newPipeline(o)
		if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
			t.Fatal(err)
		}
		recordHistory(o, nil)

		_, err := undoNow(t, Options{Yes: true})
		if err == nil {
			t.Fatal("expected a refusal")
		}
		if got := classifyMachineError(err, nil); got.Code != "NOTHING_TO_UNDO" {
			t.Fatalf("code = %s, want NOTHING_TO_UNDO", got.Code)
		}
	})

	t.Run("remote moved", func(t *testing.T) {
		project := publishEnv(t)
		publishTwice(t, project)

		owner, name, _ := strings.Cut("testuser/Demo", "/")
		remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")
		other := filepath.Join(t.TempDir(), "other")
		for _, args := range [][]string{
			{"clone", remote, other},
			{"-C", other, "config", "user.name", "Someone Else"},
			{"-C", other, "config", "user.email", "other@example.test"},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		if err := os.WriteFile(filepath.Join(other, "later.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{
			{"-C", other, "add", "-A"},
			{"-C", other, "commit", "-m", "later"},
			{"-C", other, "push", "origin", "HEAD"},
		} {
			if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}

		_, err := undoNow(t, Options{Yes: true})
		if err == nil {
			t.Fatal("expected a refusal")
		}
		if got := classifyMachineError(err, nil); got.Code != "REMOTE_MOVED" {
			t.Fatalf("code = %s, want REMOTE_MOVED", got.Code)
		}
	})
}

// TestSimpleModePublishIsUndoable is the regression guard for a defect that
// only surfaced once history had a reader.
//
// Simple Mode -- the path a person actually takes -- ran the apply on its own
// pipeline state and never copied it back, so the history entry was written
// from the caller's empty state: no repository, no commit, no branch. Nothing
// noticed while history was only ever read by a human. `gitmake undo` then
// could not find the publish it was meant to return.
func TestSimpleModePublishIsUndoable(t *testing.T) {
	project := publishEnv(t)

	// Create, then one ordinary update, both through Simple Mode.
	o := Options{Yes: true}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	if !shouldUseSimpleMode(o) {
		t.Fatal("--yes should select Simple Mode; this test would otherwise prove nothing")
	}
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runSimplePublish(o) }); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Demo\n\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := Options{Yes: true}
	second.Command = "publish"
	second.ConfigPath = "gitmake.json"
	second.State = newPipeline(second)
	if _, err := captureOutput(func() error { return runSimplePublish(second) }); err != nil {
		t.Fatal(err)
	}

	// This is what recordHistory writes from, so it has to carry the result.
	if second.State.Repository == "" {
		t.Fatal("Simple Mode left no repository on the caller's state")
	}
	if second.State.Commit == "" {
		t.Fatal("Simple Mode left no commit on the caller's state; undo would have nothing to revert")
	}
	recordHistory(second, nil)

	if _, err := undoNow(t, Options{Yes: true}); err != nil {
		t.Fatalf("a Simple Mode publish must be undoable: %v", err)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != "3" {
		t.Fatalf("remote has %s commits after the undo, want 3", got)
	}
}
