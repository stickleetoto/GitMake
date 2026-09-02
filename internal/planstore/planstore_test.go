package planstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// planstore holds the reviewed plans that approval and apply are bound to, and
// had no test coverage at all. A defect here means applying a plan the user
// never reviewed, so the checks below focus on what must never be accepted.

// isolateStore points the plan store at a scratch directory. os.UserCacheDir
// reads a different variable on each platform, so all three are set: setting
// only XDG_CACHE_HOME would isolate Linux while leaving Windows and macOS
// writing to the developer's real store.
func isolateStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir) // linux
	t.Setenv("LocalAppData", dir)   // windows
	t.Setenv("HOME", dir)           // darwin
	root, err := directory()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func samplePlan(t *testing.T, id, workdir string) Plan {
	t.Helper()
	return Plan{
		Schema:           Schema,
		ID:               id,
		CreatedAt:        time.Now().UTC(),
		WorkingDirectory: workdir,
		SourceMode:       "folder",
		SourcePath:       workdir,
		SourceSHA256:     "abc123",
		Repository:       "testuser/Demo",
		Visibility:       "private",
		Mode:             "CREATE",
		Branch:           "main",
		Changes:          ChangeCounts{Added: 2},
		Risk:             Risk{Level: "low"},
		Fingerprint:      "fp",
	}
}

func TestNewIDIsPrefixedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(id, "gm_") {
			t.Fatalf("plan id %q is missing the gm_ prefix", id)
		}
		if seen[id] {
			t.Fatalf("duplicate plan id %q", id)
		}
		seen[id] = true
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	isolateStore(t)
	work := t.TempDir()
	want := samplePlan(t, "gm_roundtrip", work)

	path, err := Save(want)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "gm_roundtrip.json" {
		t.Fatalf("saved to %q", path)
	}

	got, loadedPath, err := Load("gm_roundtrip")
	if err != nil {
		t.Fatal(err)
	}
	if loadedPath != path {
		t.Fatalf("load path %q != save path %q", loadedPath, path)
	}
	if got.Repository != want.Repository || got.SourceSHA256 != want.SourceSHA256 || got.Fingerprint != want.Fingerprint {
		t.Fatalf("plan did not survive the round trip: %+v", got)
	}
}

func TestSaveRequiresAnID(t *testing.T) {
	isolateStore(t)
	if _, err := Save(Plan{Schema: Schema}); err == nil {
		t.Fatal("expected saving a plan with no id to fail")
	}
}

// TestLoadRejectsAnIDMismatch is the substitution guard: a plan file renamed to
// another id must not load under that id, or an approval bound to one reviewed
// plan could be spent on a different one.
func TestLoadRejectsAnIDMismatch(t *testing.T) {
	root := isolateStore(t)
	plan := samplePlan(t, "gm_original", t.TempDir())
	if _, err := Save(plan); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(root, "gm_original.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gm_impostor.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Load("gm_impostor"); err == nil {
		t.Fatal("a plan file copied under a different id must not load")
	}
}

func TestLoadRejectsAnUnsupportedSchema(t *testing.T) {
	root := isolateStore(t)
	plan := samplePlan(t, "gm_future", t.TempDir())
	plan.Schema = "gitmake.plan/v99"
	data, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gm_future.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, _, err := Load("gm_future"); err == nil {
		t.Fatal("a plan written by a future schema must not load")
	}
}

func TestLoadReportsAMissingPlan(t *testing.T) {
	isolateStore(t)
	_, _, err := Load("gm_absent")
	if err == nil {
		t.Fatal("expected a missing plan to fail")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error %q should say the plan was not found", err)
	}
	if _, _, err := Load(""); err == nil {
		t.Fatal("expected an empty plan id to fail")
	}
}

func TestLoadRejectsCorruptJSON(t *testing.T) {
	root := isolateStore(t)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "gm_corrupt.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load("gm_corrupt"); err == nil {
		t.Fatal("a corrupt plan file must not load")
	}
}

// TestLatestForDirectoryPicksTheNewestPlanForThatDirectory covers what
// `gitmake approve` relies on when the user does not name a plan id.
func TestLatestForDirectoryPicksTheNewestPlanForThatDirectory(t *testing.T) {
	isolateStore(t)
	mine := t.TempDir()
	other := t.TempDir()

	older := samplePlan(t, "gm_older", mine)
	older.CreatedAt = time.Now().UTC().Add(-time.Hour)
	newer := samplePlan(t, "gm_newer", mine)
	newer.CreatedAt = time.Now().UTC()
	foreign := samplePlan(t, "gm_foreign", other)
	foreign.CreatedAt = time.Now().UTC().Add(time.Hour) // newest overall

	for _, p := range []Plan{older, newer, foreign} {
		if _, err := Save(p); err != nil {
			t.Fatal(err)
		}
	}

	got, _, err := LatestForDirectory(mine)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "gm_newer" {
		t.Fatalf("selected %q; a plan reviewed in another directory must never be picked", got.ID)
	}
}

func TestLatestForDirectoryIgnoresUnusableFiles(t *testing.T) {
	root := isolateStore(t)
	work := t.TempDir()
	if _, err := Save(samplePlan(t, "gm_good", work)); err != nil {
		t.Fatal(err)
	}
	// Debris that must not derail selection.
	if err := os.WriteFile(filepath.Join(root, "gm_broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "nested.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	got, _, err := LatestForDirectory(work)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "gm_good" {
		t.Fatalf("selected %q, want gm_good", got.ID)
	}
}

func TestLatestForDirectoryReportsWhenNothingMatches(t *testing.T) {
	isolateStore(t)
	if _, _, err := LatestForDirectory(t.TempDir()); err == nil {
		t.Fatal("expected an error when no plan exists for the directory")
	}

	// A plan for somewhere else must not satisfy this directory either.
	if _, err := Save(samplePlan(t, "gm_elsewhere", t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LatestForDirectory(t.TempDir()); err == nil {
		t.Fatal("a plan reviewed elsewhere must not be offered for this directory")
	}
}

func TestCleanupRemovesOldPlansAndKeepsRecentOnes(t *testing.T) {
	root := isolateStore(t)
	work := t.TempDir()
	for _, id := range []string{"gm_stale", "gm_fresh"} {
		if _, err := Save(samplePlan(t, id, work)); err != nil {
			t.Fatal(err)
		}
	}

	stale := filepath.Join(root, "gm_stale.json")
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if err := Cleanup(24 * time.Hour); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); err == nil {
		t.Fatal("cleanup left a plan older than the cutoff")
	}
	if _, err := os.Stat(filepath.Join(root, "gm_fresh.json")); err != nil {
		t.Fatalf("cleanup removed a recent reviewed plan: %v", err)
	}
}

func TestCleanupOnAnEmptyStoreIsNotAnError(t *testing.T) {
	isolateStore(t)
	if err := Cleanup(time.Hour); err != nil {
		t.Fatalf("cleanup before any plan exists: %v", err)
	}
}

func TestSamePathMatchesEquivalentSpellings(t *testing.T) {
	base := filepath.Join("a", "b")
	if !samePath(base, filepath.Join("a", "c", "..", "b")) {
		t.Fatal("equivalent paths should match after cleaning")
	}
	if samePath(base, filepath.Join("a", "different")) {
		t.Fatal("distinct paths must not match")
	}
	if filepath.Separator == '\\' && !samePath(base, strings.ToUpper(base)) {
		t.Fatal("path comparison must be case-insensitive on Windows")
	}
}
