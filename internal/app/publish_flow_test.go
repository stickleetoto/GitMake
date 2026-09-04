package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These drive the whole publish, not a stage: discover, plan, snapshot,
// validate, and the create or update flow that actually commits and pushes.
// The GitHub CLI is stubbed and the remote is a bare repository on disk, so a
// real push can be inspected afterwards.
//
// runPublish is the path a non-interactive caller takes -- Simple Mode is only
// chosen for a terminal or an explicit --yes -- so a test reaches it without
// any prompt.

func publishNow(t *testing.T, o Options) (string, error) {
	t.Helper()
	o.Command = "publish"
	if o.ConfigPath == "" {
		o.ConfigPath = "gitmake.json"
	}
	o.State = newPipeline(o)
	return captureOutput(func() error { return runPublish(o) })
}

// remoteFiles lists the paths committed to the fake remote's default branch.
func remoteFiles(t *testing.T, target string) []string {
	t.Helper()
	owner, name, _ := strings.Cut(target, "/")
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")
	out, err := exec.Command("git", "--git-dir="+remote, "ls-tree", "-r", "--name-only", "HEAD").Output()
	if err != nil {
		t.Fatalf("list remote files: %v", err)
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func remoteCommitCount(t *testing.T, target string) string {
	t.Helper()
	owner, name, _ := strings.Cut(target, "/")
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), owner, name+".git")
	out, err := exec.Command("git", "--git-dir="+remote, "rev-list", "--count", "HEAD").Output()
	if err != nil {
		t.Fatalf("count remote commits: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func TestPublishCreatesTheRepository(t *testing.T) {
	publishEnv(t)

	out, err := publishNow(t, Options{})
	if err != nil {
		t.Fatalf("publish: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Repository created") && !strings.Contains(out, "CREATE") {
		t.Fatalf("output does not report a creation:\n%s", out)
	}

	files := remoteFiles(t, "testuser/Demo")
	for _, want := range []string{"README.md", "src/main.go"} {
		found := false
		for _, got := range files {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s was not published; remote holds %v", want, files)
		}
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != "1" {
		t.Fatalf("remote has %s commits, want 1", got)
	}
}

// TestPublishRemembersTheProject covers the binding that stops a renamed
// folder from being published to a new repository by accident.
//
// The metadata lives in two places on purpose. A local copy in the source
// folder is what recognises that folder on a later run. The published copies
// are what a fresh clone reads: the identity so the binding survives, and the
// managed-sync baseline so GitMake knows which remote files it owns and which
// it must leave alone.
func TestPublishRemembersTheProject(t *testing.T) {
	project := publishEnv(t)

	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(project, ".gitmake", "project.json"))
	if err != nil {
		t.Fatalf("project memory was not written locally: %v", err)
	}
	if !strings.Contains(string(data), "testuser/Demo") {
		t.Fatalf("project memory does not name the repository: %s", string(data))
	}

	published := remoteFiles(t, "testuser/Demo")
	for _, want := range []string{".gitmake/project.json", ".gitmake/managed.json"} {
		found := false
		for _, got := range published {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s must be published; remote holds %v", want, published)
		}
	}
}

func TestPublishUpdatesAnExistingRepository(t *testing.T) {
	project := publishEnv(t)

	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Demo\n\nsecond revision\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := Options{}
	o.Command = "publish"
	o.ConfigPath = "gitmake.json"
	o.State = newPipeline(o)
	out, err := captureOutput(func() error { return runPublish(o) })
	if err != nil {
		t.Fatalf("second publish: %v\n%s", err, out)
	}
	if o.State.Mode != "UPDATE" {
		t.Fatalf("mode = %q, want UPDATE", o.State.Mode)
	}
	if o.State.Changes == nil || o.State.Changes.Modified != 1 || o.State.Changes.Added != 0 {
		t.Fatalf("changes = %+v, want exactly one modification", o.State.Changes)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != "2" {
		t.Fatalf("remote has %s commits, want 2", got)
	}
}

// TestRepublishingUnchangedSourceIsANoOp keeps GitMake from creating empty
// commits when nothing moved.
func TestRepublishingUnchangedSourceIsANoOp(t *testing.T) {
	publishEnv(t)

	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	out, err := publishNow(t, Options{})
	if err != nil {
		t.Fatalf("republish: %v\n%s", err, out)
	}
	if got := remoteCommitCount(t, "testuser/Demo"); got != "1" {
		t.Fatalf("republishing an unchanged source added a commit: %s", got)
	}
}

// TestDryRunPublishChangesNothing is the promise --dry-run makes.
func TestDryRunPublishChangesNothing(t *testing.T) {
	publishEnv(t)

	o := Options{DryRun: true}
	out, err := publishNow(t, o)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Dry run") {
		t.Fatalf("a dry run should say so:\n%s", out)
	}
	remote := filepath.Join(os.Getenv("FAKE_GH_ROOT"), "testuser", "Demo.git")
	if _, err := os.Stat(remote); err == nil {
		t.Fatal("a dry run created the repository")
	}
}

func TestPublishCreatesAConfiguredRelease(t *testing.T) {
	project := publishEnv(t)
	asset := filepath.Join(project, "app.zip")
	if err := os.WriteFile(asset, []byte("payload"), 0o644); err != nil {
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

	out, err := publishNow(t, Options{})
	if err != nil {
		t.Fatalf("publish with release: %v\n%s", err, out)
	}

	// The fake CLI stores releases beside the remotes directory.
	releaseDir := filepath.Join(filepath.Dir(os.Getenv("FAKE_GH_ROOT")), "releases", "testuser", "Demo", "v1.0.0")
	if _, err := os.Stat(releaseDir); err != nil {
		t.Fatalf("release was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(releaseDir, "assets", "app.zip")); err != nil {
		t.Fatalf("release asset was not uploaded: %v", err)
	}
}

// TestPublishRefusesADuplicateReleaseTag covers release.on_existing: "error",
// the default, which stops a second publish from silently reusing a tag.
func TestPublishRefusesADuplicateReleaseTag(t *testing.T) {
	project := publishEnv(t)
	config := `{"schema_version":1,` +
		`"repo":{"name":"Demo","visibility":"public"},` +
		`"source":{"folder":"."},` +
		`"git":{"branch":"main"},` +
		`"release":{"enabled":true,"tag":"v1.0.0","notes":"first","on_existing":"error"}}`
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "README.md"), []byte("# Demo\n\nchanged\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := publishNow(t, Options{})
	if err == nil {
		t.Fatal("republishing the same release tag must fail")
	}
	if got := classifyMachineError(err, nil); got.Code != "RELEASE_EXISTS" && got.Code != "TAG_CONFLICT" {
		t.Fatalf("error code = %s, want a release conflict", got.Code)
	}
}

// TestPublishRefusesToRetargetABoundProject is the identity guard: once a
// folder is bound to a repository, publishing it somewhere else is a stop, not
// a silent retarget.
func TestPublishRefusesToRetargetABoundProject(t *testing.T) {
	project := publishEnv(t)

	if _, err := publishNow(t, Options{}); err != nil {
		t.Fatal(err)
	}
	config := `{"schema_version":1,` +
		`"repo":{"name":"SomewhereElse","visibility":"private"},` +
		`"source":{"folder":"."},` +
		`"git":{"branch":"main"}}`
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := publishNow(t, Options{})
	if err == nil {
		t.Fatal("publishing a bound folder to a different repository must fail")
	}
	if got := classifyMachineError(err, nil); got.Code != "PROJECT_IDENTITY_MISMATCH" {
		t.Fatalf("error code = %s, want PROJECT_IDENTITY_MISMATCH", got.Code)
	}
}
