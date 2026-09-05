package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitmake/internal/github"
)

// The point of the VERIFY stage is to catch a publish that reported success
// while GitHub holds something else. A verification that quietly does nothing
// would pass every test here by being absent, so these prove it runs, prove
// what it compares, and prove it fails when the two disagree.

func TestPublishVerifiesTheRemoteCommit(t *testing.T) {
	publishEnv(t)

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	out, err := captureOutput(func() error { return runPublish(o) })
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}

	v := o.State.Verification
	if v == nil {
		t.Fatal("publish produced no verification at all")
	}
	if !v.Checked {
		t.Fatalf("verification did not run: %+v", v)
	}
	if !v.CommitMatches {
		t.Fatalf("remote commit did not match: %+v", v)
	}
	if v.RemoteCommit == "" || v.RemoteCommit != o.State.Commit {
		t.Fatalf("remote %q, pushed %q", v.RemoteCommit, o.State.Commit)
	}
	if !strings.Contains(out, "Verified") {
		t.Fatalf("a verified publish should say so:\n%s", out)
	}

	// And the commit it verified is the one the remote really holds.
	owner, name, _ := strings.Cut("testuser/Demo", "/")
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")
	got, err := exec.Command("git", "--git-dir="+remote, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(got)) != v.RemoteCommit {
		t.Fatalf("verification reported %q but the remote is at %q", v.RemoteCommit, strings.TrimSpace(string(got)))
	}
}

// TestVerificationRecordsTheCommitThatUndoWillRevert ties the two v1.3.0
// features together: verification is what proves the recorded commit is real,
// and that recorded commit is the only thing undo has to work from.
func TestVerificationRecordsTheCommitThatUndoWillRevert(t *testing.T) {
	publishEnv(t)

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatal(err)
	}
	if o.State.Commit == "" {
		t.Fatal("publish recorded no commit; undo would have nothing to revert")
	}
	if len(o.State.Commit) != 40 {
		t.Fatalf("commit %q is not a full SHA", o.State.Commit)
	}
	if !o.State.RepoCreated {
		t.Fatal("a publish that created the repository must say so; such a run cannot be undone")
	}
}

func TestPublishVerifiesReleaseAssets(t *testing.T) {
	project := publishEnv(t)
	if err := os.WriteFile(filepath.Join(project, "app.zip"), []byte("payload-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	config := `{"schema_version":1,` +
		`"repo":{"name":"Demo","visibility":"public"},` +
		`"source":{"folder":"."},` +
		`"git":{"branch":"main"},` +
		`"release":{"enabled":true,"tag":"v1.0.0","notes":"first","assets":["app.zip"]}}`
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	out, err := captureOutput(func() error { return runPublish(o) })
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	v := o.State.Verification
	if v == nil || !v.Checked {
		t.Fatalf("verification did not run: %+v", v)
	}
	if v.ReleaseAssets != 1 {
		t.Fatalf("verified %d assets, want 1", v.ReleaseAssets)
	}
	if !v.AssetsMatch {
		t.Fatalf("assets did not match: %v", v.Problems)
	}
}

// TestCompareAssetsCatchesAMissingUpload is the failure the release check
// exists for: gh returned success but the file is not on the release.
func TestCompareAssetsCatchesAMissingUpload(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(a, []byte("1234567890"), 0o644); err != nil {
		t.Fatal(err)
	}

	problems := compareAssets([]string{a}, []github.ReleaseAsset{})
	if len(problems) != 1 || !strings.Contains(problems[0], "missing") {
		t.Fatalf("a missing asset must be reported, got %v", problems)
	}

	problems = compareAssets([]string{a}, []github.ReleaseAsset{{Name: "app.zip", Size: 10}})
	if len(problems) != 0 {
		t.Fatalf("a matching asset must not be reported: %v", problems)
	}
}

// TestCompareAssetsCatchesATruncatedUpload is the quieter failure: the asset is
// there, so a name check would pass, but not all of it arrived.
func TestCompareAssetsCatchesATruncatedUpload(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(a, []byte("1234567890"), 0o644); err != nil {
		t.Fatal(err)
	}

	problems := compareAssets([]string{a}, []github.ReleaseAsset{{Name: "app.zip", Size: 4}})
	if len(problems) != 1 || !strings.Contains(problems[0], "bytes") {
		t.Fatalf("a truncated asset must be reported, got %v", problems)
	}
}

// TestCompareAssetsToleratesAnUnreportedSize keeps the check from inventing a
// failure when the API does not return the field.
func TestCompareAssetsToleratesAnUnreportedSize(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(a, []byte("1234567890"), 0o644); err != nil {
		t.Fatal(err)
	}
	if problems := compareAssets([]string{a}, []github.ReleaseAsset{{Name: "app.zip"}}); len(problems) != 0 {
		t.Fatalf("an unreported size is not a mismatch: %v", problems)
	}
}

// TestVerificationIsSkippedForADryRun matters because a dry run pushed nothing
// and has nothing to confirm; verifying would have to invent a subject.
func TestVerificationIsSkippedForADryRun(t *testing.T) {
	publishEnv(t)

	o := Options{DryRun: true}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	if _, err := captureOutput(func() error { return runPublish(o) }); err != nil {
		t.Fatal(err)
	}
	if o.State.Verification != nil {
		t.Fatalf("a dry run must not report a verification: %+v", o.State.Verification)
	}
}
