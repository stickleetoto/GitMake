package github

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gitmake/internal/runner"
)

// This package shells out to the GitHub CLI for every remote mutation and had
// no test coverage at all. These tests drive the real client against the
// compiled fake gh, so both sides of the contract are exercised: the arguments
// GitMake sends and the output it parses.

// withFakeGH compiles the fake GitHub CLI, puts it on PATH under the name the
// client actually looks up, and points it at a scratch state directory.
func withFakeGH(t *testing.T) Client {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}

	bin := t.TempDir()
	name := "gh"
	if runtime.GOOS == "windows" {
		name = "gh.exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(bin, name), "gitmake/internal/testsupport/fakegh")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake gh: %v: %s", err, string(out))
	}

	state := t.TempDir()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_ROOT", filepath.Join(state, "remotes"))

	// Keep git operations off the developer's real configuration.
	gitconfig := filepath.Join(state, "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", gitconfig)
	t.Setenv("GIT_CONFIG_SYSTEM", filepath.Join(state, "gitconfig-system"))
	for _, kv := range [][2]string{
		{"user.name", "GitMake Test"},
		{"user.email", "test@example.test"},
		{"init.defaultBranch", "main"},
	} {
		cmd := exec.Command("git", "config", "--global", kv[0], kv[1])
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git config %s: %v: %s", kv[0], err, string(out))
		}
	}

	return Client{Run: runner.Runner{}}
}

// seedProject creates a committed working tree suitable for `repo create --push`.
func seedProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q"},
		{"add", "-A"},
		{"commit", "-qm", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, string(out))
		}
	}
	return dir
}

func TestPreflightAndCurrentUser(t *testing.T) {
	c := withFakeGH(t)
	if err := c.Preflight(); err != nil {
		t.Fatalf("preflight: %v", err)
	}
	user, err := c.CurrentUser()
	if err != nil {
		t.Fatal(err)
	}
	if user != "testuser" {
		t.Fatalf("current user = %q, want testuser", user)
	}
}

func TestPreflightFailsWhenNotAuthenticated(t *testing.T) {
	c := withFakeGH(t)
	t.Setenv("FAKE_GH_AUTH_FAIL", "1")
	err := c.Preflight()
	if err == nil {
		t.Fatal("expected preflight to fail when gh is not authenticated")
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Fatalf("error should tell the user how to recover, got %v", err)
	}
}

// TestRepoReportsAbsenceRatherThanError pins the distinction the publish
// pipeline depends on: a missing repository is "create", not a failure.
func TestRepoReportsAbsenceRatherThanError(t *testing.T) {
	c := withFakeGH(t)
	info, exists, err := c.Repo("testuser", "NotThere")
	if err != nil {
		t.Fatalf("a missing repository must not be an error: %v", err)
	}
	if exists {
		t.Fatalf("unexpected repository %+v", info)
	}
}

func TestCreateAndPushThenView(t *testing.T) {
	c := withFakeGH(t)
	source := seedProject(t)

	url, err := c.CreateAndPush("testuser/Demo", "public", "a demo", source)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if url == "" {
		t.Fatal("create returned an empty URL")
	}

	info, exists, err := c.Repo("testuser", "Demo")
	if err != nil || !exists {
		t.Fatalf("repo after create: exists=%v err=%v", exists, err)
	}
	if info.NameWithOwner != "testuser/Demo" {
		t.Fatalf("nameWithOwner = %q", info.NameWithOwner)
	}
	// Visibility must survive the round trip: GitMake refuses to silently
	// change it on update, so reading it back wrong would be a safety bug.
	if info.Visibility != "PUBLIC" {
		t.Fatalf("visibility = %q, want PUBLIC", info.Visibility)
	}
	if info.DefaultBranch() != "main" {
		t.Fatalf("default branch = %q, want main", info.DefaultBranch())
	}

	dest := filepath.Join(t.TempDir(), "clone")
	if err := c.Clone("testuser/Demo", dest); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err != nil {
		t.Fatalf("cloned tree is missing the committed file: %v", err)
	}
}

func TestBranchPolicy(t *testing.T) {
	c := withFakeGH(t)

	policy, err := c.BranchPolicy("testuser/Demo", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Known || policy.Protected || policy.RequiresPR {
		t.Fatalf("unprotected branch reported as %+v", policy)
	}

	t.Setenv("FAKE_GH_REQUIRE_PR", "1")
	policy, err = c.BranchPolicy("testuser/Demo", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !policy.Known || !policy.Protected || !policy.RequiresPR {
		t.Fatalf("protected branch reported as %+v", policy)
	}
}

func TestTagExists(t *testing.T) {
	c := withFakeGH(t)
	source := seedProject(t)
	if _, err := c.CreateAndPush("testuser/Tags", "private", "", source); err != nil {
		t.Fatal(err)
	}

	exists, err := c.TagExists("testuser/Tags", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("tag reported as existing before it was created")
	}

	tag := exec.Command("git", "push", "origin", "HEAD:refs/tags/v1.0.0")
	tag.Dir = source
	if out, err := tag.CombinedOutput(); err != nil {
		t.Fatalf("push tag: %v: %s", err, string(out))
	}

	exists, err = c.TagExists("testuser/Tags", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("tag reported as missing after it was pushed")
	}
}

func TestReleaseLifecycle(t *testing.T) {
	c := withFakeGH(t)
	source := seedProject(t)
	if _, err := c.CreateAndPush("testuser/Rel", "public", "", source); err != nil {
		t.Fatal(err)
	}

	if _, exists, err := c.Release("testuser/Rel", "v1.0.0"); err != nil || exists {
		t.Fatalf("release reported before creation: exists=%v err=%v", exists, err)
	}

	assetDir := t.TempDir()
	first := filepath.Join(assetDir, "app-win.zip")
	second := filepath.Join(assetDir, "app-mac.zip")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("payload"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	url, err := c.CreateRelease("testuser/Rel", ReleaseCreateOptions{
		Tag: "v1.0.0", Target: "main", Title: "First", Notes: "notes", Assets: []string{first},
	})
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	if url == "" {
		t.Fatal("create release returned an empty URL")
	}

	info, exists, err := c.Release("testuser/Rel", "v1.0.0")
	if err != nil || !exists {
		t.Fatalf("release after create: exists=%v err=%v", exists, err)
	}
	if info.TagName != "v1.0.0" || len(info.Assets) != 1 || info.Assets[0].Name != "app-win.zip" {
		t.Fatalf("unexpected release %+v", info)
	}

	// The resume path uploads into an existing release rather than recreating it.
	if err := c.UploadReleaseAssets("testuser/Rel", "v1.0.0", []string{second}); err != nil {
		t.Fatalf("upload assets: %v", err)
	}
	info, _, err = c.Release("testuser/Rel", "v1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(info.Assets) != 2 {
		t.Fatalf("release holds %d assets after upload, want 2", len(info.Assets))
	}

	latest, err := c.LatestReleaseTag("testuser/Rel")
	if err != nil {
		t.Fatal(err)
	}
	if latest != "v1.0.0" {
		t.Fatalf("latest release tag = %q, want v1.0.0", latest)
	}

	dir := t.TempDir()
	path, err := c.DownloadReleaseAsset("testuser/Rel", "v1.0.0", "app-win.zip", dir)
	if err != nil {
		t.Fatalf("download asset: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "payload" {
		t.Fatalf("downloaded asset = %q err=%v", string(data), err)
	}
}

// TestCreateReleaseRejectsADuplicateTag matters because the publish pipeline
// relies on a failure here to honour release.on_existing: "error".
func TestCreateReleaseRejectsADuplicateTag(t *testing.T) {
	c := withFakeGH(t)
	source := seedProject(t)
	if _, err := c.CreateAndPush("testuser/Dup", "private", "", source); err != nil {
		t.Fatal(err)
	}
	opts := ReleaseCreateOptions{Tag: "v2.0.0", Target: "main", Notes: "n"}
	if _, err := c.CreateRelease("testuser/Dup", opts); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateRelease("testuser/Dup", opts); err == nil {
		t.Fatal("expected the second release with the same tag to fail")
	}
}

// TestCreateReleaseRejectsAMissingTargetBranch guards against publishing a
// release that points at a branch which was never pushed.
func TestCreateReleaseRejectsAMissingTargetBranch(t *testing.T) {
	c := withFakeGH(t)
	source := seedProject(t)
	if _, err := c.CreateAndPush("testuser/Branch", "private", "", source); err != nil {
		t.Fatal(err)
	}
	_, err := c.CreateRelease("testuser/Branch", ReleaseCreateOptions{Tag: "v1.0.0", Target: "nope", Notes: "n"})
	if err == nil {
		t.Fatal("expected a release targeting a missing branch to fail")
	}
}
