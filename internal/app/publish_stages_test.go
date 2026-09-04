package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The publish pipeline was one 295-line function, so none of it could be
// tested: reaching any stage meant reaching all of them, with a real GitHub
// account and a terminal. Now that the stages are separate they can be driven
// individually against the compiled fake GitHub CLI.

// publishEnv puts a project directory, a stubbed GitHub CLI and isolated
// caches in place, and makes the project the working directory.
func publishEnv(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain unavailable")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}

	root := t.TempDir()

	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "gh"
	if runtime.GOOS == "windows" {
		name = "gh.exe"
	}
	build := exec.Command("go", "build", "-o", filepath.Join(bin, name), "gitmake/internal/testsupport/fakegh")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fake gh: %v: %s", err, string(out))
	}

	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_GH_ROOT", filepath.Join(root, "remotes"))

	// Keep plans, approvals and git config out of the developer's real state.
	cache := filepath.Join(root, "cache")
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LocalAppData", cache)
	t.Setenv("HOME", root)
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(root, "gitconfig"))
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

	project := filepath.Join(root, "Demo")
	if err := os.MkdirAll(filepath.Join(project, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{
		"README.md":   "# Demo\n",
		"src/main.go": "package main\n",
	} {
		if err := os.WriteFile(filepath.Join(project, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	// Leave the directory before TempDir cleanup runs; Windows cannot remove a
	// directory that is any process's working directory.
	t.Cleanup(func() { _ = os.Chdir(previous) })

	return project
}

func publishOptions() Options {
	o := Options{Command: "publish", ConfigPath: "gitmake.json", DryRun: true, ReadOnly: true}
	o.State = newPipeline(o)
	return o
}

// --- DISCOVER ---------------------------------------------------------------

func TestDiscoverInfersAZeroConfigFolderPublish(t *testing.T) {
	project := publishEnv(t)
	o := publishOptions()

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	if in.ConfigSource != "inferred" {
		t.Fatalf("config source = %q, want inferred", in.ConfigSource)
	}
	if in.ConfigPersisted {
		t.Fatal("zero-config publishing must not report a persisted config")
	}
	if in.Source.Mode != "folder" {
		t.Fatalf("source mode = %q, want folder", in.Source.Mode)
	}
	if !strings.EqualFold(in.Source.Path, project) {
		t.Fatalf("source path = %q, want %q", in.Source.Path, project)
	}
	if in.SourceSHA256 == "" {
		t.Fatal("a reviewed plan binds to the source hash; it must be computed")
	}
	// Nothing is written to disk by inference.
	if _, err := os.Stat(filepath.Join(project, "gitmake.json")); err == nil {
		t.Fatal("inference must not write gitmake.json")
	}

	if o.State.Config == nil || o.State.Config.Source != "inferred" {
		t.Fatalf("pipeline state config = %+v", o.State.Config)
	}
	if o.State.Stage != "PLAN" {
		t.Fatalf("stage = %q, want PLAN after discovery", o.State.Stage)
	}
}

func TestDiscoverLoadsAPersistedConfig(t *testing.T) {
	project := publishEnv(t)
	body := `{"schema_version":1,"repo":{"name":"Persisted","visibility":"public"},"source":{"folder":"."},"git":{"branch":"main"}}`
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	in, err := discoverPublishInput(publishOptions())
	if err != nil {
		t.Fatal(err)
	}
	if in.ConfigSource != "file" || !in.ConfigPersisted {
		t.Fatalf("config source = %q persisted = %v", in.ConfigSource, in.ConfigPersisted)
	}
	if in.Config.Repo.Name != "Persisted" || in.Config.Repo.Visibility != "public" {
		t.Fatalf("config not honoured: %+v", in.Config.Repo)
	}
	// A persisted config is hashed so a plan can detect it changing.
	if in.ConfigSHA256 == "" {
		t.Fatal("a persisted config must be hashed")
	}
}

func TestDiscoverRejectsAnInvalidConfig(t *testing.T) {
	project := publishEnv(t)
	if err := os.WriteFile(filepath.Join(project, "gitmake.json"), []byte(`{"schema_version":99}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := discoverPublishInput(publishOptions())
	if err == nil {
		t.Fatal("an unsupported schema version must fail")
	}
	if got := classifyMachineError(err, nil); got.Code != "CONFIG_INVALID" {
		t.Fatalf("error code = %s, want CONFIG_INVALID", got.Code)
	}
}

// TestDiscoverGuidanceIsNotAFailure covers the one path that ends the command
// successfully without publishing: a human in a folder with nothing to publish
// is shown what to do instead.
func TestDiscoverGuidanceIsNotAFailure(t *testing.T) {
	empty := t.TempDir()
	previous, _ := os.Getwd()
	publishEnv(t)
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	o := publishOptions()
	o.ReadOnly = false
	o.JSON = false

	out, err := captureOutput(func() error {
		_, discoverErr := discoverPublishInput(o)
		return discoverErr
	})
	if err == nil {
		t.Fatal("expected the guidance sentinel")
	}
	if err.Error() != errSourceGuidanceShown.Error() {
		t.Fatalf("error = %v, want the guidance sentinel", err)
	}
	if !strings.Contains(out, "No project source could be selected") {
		t.Fatalf("guidance was not shown:\n%s", out)
	}
}

// TestDiscoverRefusesToGuessForMachineCallers pins the safety rule: a machine
// caller gets a hard error rather than guidance it cannot act on.
func TestDiscoverRefusesToGuessForMachineCallers(t *testing.T) {
	empty := t.TempDir()
	previous, _ := os.Getwd()
	publishEnv(t)
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	for _, o := range []Options{
		{Command: "publish", ConfigPath: "gitmake.json", JSON: true},
		{Command: "publish", ConfigPath: "gitmake.json", ReadOnly: true},
	} {
		o.State = newPipeline(o)
		_, err := discoverPublishInput(o)
		if err == nil {
			t.Fatal("a machine caller must not be given a guessed source")
		}
		if err.Error() == errSourceGuidanceShown.Error() {
			t.Fatal("a machine caller must get an error, not printed guidance")
		}
	}
}

// --- PLAN -------------------------------------------------------------------

func TestPlanReportsCreateForANewRepository(t *testing.T) {
	publishEnv(t)
	o := publishOptions()

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	cfg := in.Config
	plan, release, err := planPublishTarget(o, in, &cfg)
	defer release()
	if err != nil {
		t.Fatal(err)
	}
	if plan.Exists {
		t.Fatal("a repository that was never created must not report as existing")
	}
	if plan.Owner != "testuser" {
		t.Fatalf("owner = %q, want testuser", plan.Owner)
	}
	if plan.Target != "testuser/Demo" {
		t.Fatalf("target = %q", plan.Target)
	}
	if o.State.Mode != "CREATE" {
		t.Fatalf("mode = %q, want CREATE", o.State.Mode)
	}
}

func TestPlanEnforcesCreateOnlyAndUpdateOnly(t *testing.T) {
	publishEnv(t)

	o := publishOptions()
	o.UpdateOnly = true
	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	cfg := in.Config
	_, release, err := planPublishTarget(o, in, &cfg)
	release()
	if err == nil {
		t.Fatal("--update-only must fail when the repository does not exist")
	}
	if !strings.Contains(err.Error(), "--update-only") {
		t.Fatalf("error should name the flag, got %v", err)
	}
}

// TestPlanHeaderExplainsInferredConfiguration matters because a zero-config
// publish decides the target and visibility on the user's behalf. They are
// entitled to see where those came from before confirming.
func TestPlanHeaderExplainsInferredConfiguration(t *testing.T) {
	publishEnv(t)
	o := publishOptions()

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	cfg := in.Config
	plan, release, err := planPublishTarget(o, in, &cfg)
	defer release()
	if err != nil {
		t.Fatal(err)
	}

	out, err := captureOutput(func() error {
		printPlanHeader(in, cfg, plan)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"testuser/Demo", "inferred in memory", "Source mode", "folder"} {
		if !strings.Contains(out, want) {
			t.Fatalf("header is missing %q:\n%s", want, out)
		}
	}
}

// --- PREPARE / SECURITY -----------------------------------------------------

func TestPrepareSnapshotCountsFilesAndClearsSecurity(t *testing.T) {
	publishEnv(t)
	o := publishOptions()

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := prepareSnapshot(o, in.Source, in.Config, in.SourceSHA256, false)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}
	if snap.Files != 2 {
		t.Fatalf("files = %d, want 2", snap.Files)
	}
	if _, statErr := os.Stat(filepath.Join(snap.Path, "README.md")); statErr != nil {
		t.Fatalf("snapshot is missing a source file: %v", statErr)
	}
	if !snap.Security.SecretScan {
		t.Fatal("the secret scan should have run")
	}
	if snap.Security.Blocking {
		t.Fatalf("a clean project must not block: %+v", snap.Security.Findings)
	}
	if o.State.Stage != "SECURITY" {
		t.Fatalf("stage = %q, want SECURITY", o.State.Stage)
	}
}

// TestPrepareSnapshotBlocksOnASecret is the gate GitMake exists to provide,
// exercised through the pipeline rather than the scanner alone.
func TestPrepareSnapshotBlocksOnASecret(t *testing.T) {
	project := publishEnv(t)
	leak := "AKIA" + "ABCDEFGHIJKLMNOP"
	if err := os.WriteFile(filepath.Join(project, "src", "creds.txt"), []byte("key="+leak+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	o := publishOptions()
	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := prepareSnapshot(o, in.Source, in.Config, in.SourceSHA256, false)
	defer cleanup()
	if err == nil {
		t.Fatal("a detected secret must block the publish")
	}
	if got := classifyMachineError(err, nil); got.Code != "SECRET_DETECTED" {
		t.Fatalf("error code = %s, want SECRET_DETECTED", got.Code)
	}
	// The findings must survive the block so the user can act on them.
	if len(snap.Security.Findings) == 0 {
		t.Fatal("a blocked publish must still report which findings blocked it")
	}
	if o.State.Security == nil {
		t.Fatal("the pipeline state must carry the security report even when blocked")
	}
}

func TestPrepareSnapshotCleanupRemovesTheWorkspace(t *testing.T) {
	publishEnv(t)
	o := publishOptions()

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := prepareSnapshot(o, in.Source, in.Config, in.SourceSHA256, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(snap.Workspace); statErr != nil {
		t.Fatalf("workspace missing while in use: %v", statErr)
	}
	cleanup()
	if _, statErr := os.Stat(snap.Workspace); statErr == nil {
		t.Fatal("cleanup left the workspace behind")
	}
}

// TestKeepTempPreservesTheWorkspace covers the debugging flag: --keep-temp
// turns the cleanup into a no-op rather than skipping a deferral.
func TestKeepTempPreservesTheWorkspace(t *testing.T) {
	publishEnv(t)
	o := publishOptions()
	o.KeepTemp = true

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	var snap publishSnapshot
	var cleanup func()
	out, err := captureOutput(func() error {
		var stageErr error
		snap, cleanup, stageErr = prepareSnapshot(o, in.Source, in.Config, in.SourceSHA256, false)
		return stageErr
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	if _, statErr := os.Stat(snap.Workspace); statErr != nil {
		t.Fatal("--keep-temp must leave the workspace in place")
	}
	if !strings.Contains(out, "Temporary workspace") {
		t.Fatalf("--keep-temp should report the path:\n%s", out)
	}
	_ = os.RemoveAll(snap.Workspace)
}

func TestReportValidatedSourceShowsWhatWasChecked(t *testing.T) {
	publishEnv(t)
	o := publishOptions()

	in, err := discoverPublishInput(o)
	if err != nil {
		t.Fatal(err)
	}
	snap, cleanup, err := prepareSnapshot(o, in.Source, in.Config, in.SourceSHA256, false)
	defer cleanup()
	if err != nil {
		t.Fatal(err)
	}

	out, err := captureOutput(func() error {
		reportValidatedSource(o, in.Source, snap)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Source validated") || !strings.Contains(out, "2 files") {
		t.Fatalf("validation summary:\n%s", out)
	}
	if !strings.Contains(out, "Security scan") {
		t.Fatalf("the security scan should be reported:\n%s", out)
	}
	if o.State.Stage != "VALIDATE" {
		t.Fatalf("stage = %q, want VALIDATE", o.State.Stage)
	}
}
