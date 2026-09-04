package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The safety behaviour verified here was previously covered only by the
// e2e_v07, v072 and v073 shell suites. Those suites ended in a pre-v1.0
// approval flow that printed a copyable gma_ token, which v1.0 deliberately
// removed, so they could no longer run at all -- taking these checks with them.
// Everything they asserted that GitMake still does is ported here, where it
// also runs on Windows: the shell versions needed a pty.

// commitToRemote adds a file straight to the remote, the way a collaborator or
// a CI workflow would, without GitMake's involvement.
func commitToRemote(t *testing.T, target, path, body string) {
	t.Helper()
	owner, name, _ := strings.Cut(target, "/")
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")

	work := t.TempDir()
	for _, args := range [][]string{
		{"clone", remote, work},
	} {
		cmd := exec.Command("git", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, string(out))
		}
	}
	full := filepath.Join(work, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "-A"},
		{"commit", "-qm", "collaborator change"},
		{"push", "-q", "origin", "HEAD"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = work
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, string(out))
		}
	}
}

// TestManagedSyncLeavesRemoteOnlyFilesAlone is the promise managed sync makes.
// GitMake owns the files it published and may remove them; anything it never
// published belongs to someone else and must survive an update.
func TestManagedSyncLeavesRemoteOnlyFilesAlone(t *testing.T) {
	project := publishEnv(t)

	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	commitToRemote(t, "testuser/Demo", ".github/workflows/ci.yml", "name: CI\n")

	// A normal update: change one managed file.
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Demo\n\nupdated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}

	published := remoteFiles(t, "testuser/Demo")
	found := false
	for _, path := range published {
		if path == ".github/workflows/ci.yml" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a remote-only file was deleted by an update; remote holds %v", published)
	}
}

// TestManagedSyncRemovesFilesItPublished is the other half: a file GitMake put
// there and the source no longer contains must go, or the remote drifts.
func TestManagedSyncRemovesFilesItPublished(t *testing.T) {
	project := publishEnv(t)

	extra := filepath.Join(project, "src", "extra.go")
	if err := os.WriteFile(extra, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(extra); err != nil {
		t.Fatal(err)
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}

	for _, path := range remoteFiles(t, "testuser/Demo") {
		if path == "src/extra.go" {
			t.Fatal("a file GitMake published and the source dropped was left behind")
		}
	}
}

// TestMassDeletionIsClassifiedDestructive covers the gate that catches a
// wrong-folder or wrong-context publish: losing most of what GitMake manages
// is never routine.
func TestMassDeletionIsClassifiedDestructive(t *testing.T) {
	project := publishEnv(t)

	// A baseline large enough to exercise the threshold: destructive needs at
	// least 10 deleted files and at least 30% of the managed baseline.
	bulk := filepath.Join(project, "bulk")
	if err := os.MkdirAll(bulk, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		name := filepath.Join(bulk, fmt.Sprintf("file%02d.txt", i))
		if err := os.WriteFile(name, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(bulk); err != nil {
		t.Fatal(err)
	}

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	out, err := captureOutput(func() error { return runPublish(o) })
	if err == nil {
		t.Fatalf("a mass deletion must be blocked:\n%s", out)
	}
	if got := classifyMachineError(err, nil); got.Code != "DESTRUCTIVE_CHANGE_BLOCKED" {
		t.Fatalf("error code = %s, want DESTRUCTIVE_CHANGE_BLOCKED", got.Code)
	}
	if o.State.Risk == nil || !o.State.Risk.Destructive {
		t.Fatalf("risk = %+v, want destructive", o.State.Risk)
	}
	if o.State.Risk.Level != "high" {
		t.Fatalf("risk level = %q, want high", o.State.Risk.Level)
	}

	// The remote must be untouched: the block happens before mutation.
	if got := remoteCommitCount(t, "testuser/Demo"); got != "1" {
		t.Fatalf("remote has %s commits; a blocked destructive change mutated it", got)
	}
}

// TestSmallDeletionIsNotDestructive keeps the gate from firing on ordinary
// cleanup. A threshold that blocks everything gets switched off.
func TestSmallDeletionIsNotDestructive(t *testing.T) {
	project := publishEnv(t)

	bulk := filepath.Join(project, "bulk")
	if err := os.MkdirAll(bulk, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if err := os.WriteFile(filepath.Join(bulk, fmt.Sprintf("file%02d.txt", i)), []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	// Remove five of forty-two managed files: under both thresholds.
	for i := 0; i < 5; i++ {
		if err := os.Remove(filepath.Join(bulk, fmt.Sprintf("file%02d.txt", i))); err != nil {
			t.Fatal(err)
		}
	}

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatalf("an ordinary cleanup must not be blocked: %v", err)
	}
	if o.State.Risk != nil && o.State.Risk.Destructive {
		t.Fatalf("risk = %+v; five deletions is not destructive", o.State.Risk)
	}
}

// TestUpdateNeverChangesRemoteVisibility is a safety invariant with a quiet
// failure mode: silently flipping a private repository to public.
func TestUpdateNeverChangesRemoteVisibility(t *testing.T) {
	project := publishEnv(t)

	config := `{"schema_version":1,` +
		`"repo":{"name":"Demo","visibility":"private"},` +
		`"source":{"folder":"."},"git":{"branch":"main"}}`
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}

	// The config now disagrees with the remote.
	public := strings.Replace(config, `"visibility":"private"`, `"visibility":"public"`, 1)
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(public), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Demo\n\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	out, err := captureOutput(func() error { return runPublish(o) })
	if err != nil {
		t.Fatalf("an update should proceed and report the mismatch: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Visibility mismatch") {
		t.Fatalf("the mismatch must be reported:\n%s", out)
	}
	if !strings.Contains(out, "remote unchanged") {
		t.Fatalf("the report must say the remote is left alone:\n%s", out)
	}

	visibility, err := os.ReadFile(filepath.Join(os.Getenv("FAKE_GH_ROOT"), "testuser", "Demo.git.visibility"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(visibility)) != "PRIVATE" {
		t.Fatalf("remote visibility = %q; an update must never change it", strings.TrimSpace(string(visibility)))
	}

	// A visibility disagreement is not routine, so the plan is not low risk.
	if o.State.Risk == nil || o.State.Risk.Level == "low" {
		t.Fatalf("risk = %+v, want at least medium for a visibility mismatch", o.State.Risk)
	}
}

// TestIdentityIsCommittedOnFirstPublish covers what makes the binding survive a
// fresh clone rather than living only on the machine that published.
func TestIdentityIsCommittedOnFirstPublish(t *testing.T) {
	publishEnv(t)

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatal(err)
	}
	if o.State.Identity == nil || o.State.Identity.Repository != "testuser/Demo" {
		t.Fatalf("identity = %+v", o.State.Identity)
	}
	if o.State.Identity.Status != "created" {
		t.Fatalf("identity status = %q, want created", o.State.Identity.Status)
	}

	found := false
	for _, path := range remoteFiles(t, "testuser/Demo") {
		if path == ".gitmake/project.json" {
			found = true
		}
	}
	if !found {
		t.Fatal("the project identity must be committed on the first publish")
	}
}
