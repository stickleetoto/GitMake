package history

import (
	"os"
	"testing"
	"time"
)

// History used to be write-only decoration. It is now what `gitmake undo`
// reads to find out which commit to revert, so what it stores has to survive a
// round trip and two entries must never share a file.

// historyHome points the store at a temporary directory. os.UserCacheDir reads
// a different variable on each platform, so all three are set.
func historyHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)
	t.Setenv("LocalAppData", dir)
	t.Setenv("HOME", dir)
	return dir
}

func TestRecordRoundTripsWhatUndoNeeds(t *testing.T) {
	historyHome(t)

	want := Entry{
		Command: "publish", OK: true, Repository: "owner/repo", Mode: "UPDATE",
		Branch: "main", Commit: "0f1e2d3c4b5a69788796a5b4c3d2e1f001122334",
		ReleaseTag: "v1.0.0", Added: 3, Modified: 2, Deleted: 1,
	}
	if err := Record(want); err != nil {
		t.Fatal(err)
	}

	got, err := List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d entries, want 1", len(got))
	}
	e := got[0]
	if e.Commit != want.Commit {
		t.Fatalf("commit = %q, want %q; without it there is nothing to revert", e.Commit, want.Commit)
	}
	if e.Branch != want.Branch {
		t.Fatalf("branch = %q, want %q", e.Branch, want.Branch)
	}
	if e.ID == "" {
		t.Fatal("entry has no id, so an undo could not mark it")
	}
	if e.Undone {
		t.Fatal("a fresh entry must not be marked undone")
	}
}

// TestEveryEntryGetsItsOwnFile is the regression guard for a collision the
// selfupdate helper had for the same reason: the name was derived from
// time.Now(), and Windows clock granularity is around 15 ms, so entries written
// back to back shared a name and one silently replaced the other.
func TestEveryEntryGetsItsOwnFile(t *testing.T) {
	dir := historyHome(t)

	const n = 25
	for i := 0; i < n; i++ {
		if err := Record(Entry{Command: "publish", OK: true, Repository: "owner/repo"}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := List(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("stored %d entries, want %d: entries are overwriting each other", len(got), n)
	}

	seen := map[string]bool{}
	for _, e := range got {
		if seen[e.ID] {
			t.Fatalf("two entries share the id %s", e.ID)
		}
		seen[e.ID] = true
	}
	_ = dir
}

func TestMarkUndone(t *testing.T) {
	historyHome(t)
	if err := Record(Entry{Command: "publish", OK: true, Commit: "abc123"}); err != nil {
		t.Fatal(err)
	}
	first, err := List(1)
	if err != nil {
		t.Fatal(err)
	}
	id := first[0].ID

	if err := MarkUndone(id); err != nil {
		t.Fatal(err)
	}
	after, err := List(1)
	if err != nil {
		t.Fatal(err)
	}
	if !after[0].Undone {
		t.Fatal("entry was not marked undone; a second undo would revert it again")
	}
	// Everything else survives the rewrite.
	if after[0].Commit != "abc123" || after[0].ID != id {
		t.Fatalf("marking undone damaged the entry: %+v", after[0])
	}

	// Idempotent, and silent about entries that are not there.
	if err := MarkUndone(id); err != nil {
		t.Fatal(err)
	}
	if err := MarkUndone("no-such-entry"); err != nil {
		t.Fatalf("a missing entry is not an error: %v", err)
	}
	if err := MarkUndone(""); err != nil {
		t.Fatalf("an empty id is not an error: %v", err)
	}
}

// TestMarkUndoneCannotEscapeTheHistoryDirectory covers the id arriving from a
// stored file rather than from code.
func TestMarkUndoneCannotEscapeTheHistoryDirectory(t *testing.T) {
	home := historyHome(t)
	victim := home + "/victim.json"
	if err := os.WriteFile(victim, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := MarkUndone("../../victim"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}\n" {
		t.Fatalf("a traversing id rewrote a file outside the history directory: %q", string(data))
	}
}

func TestListIsNewestFirstAndRespectsTheLimit(t *testing.T) {
	historyHome(t)
	for i := 0; i < 5; i++ {
		if err := Record(Entry{Command: "publish", OK: true, Repository: "r", Time: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := List(3)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i].Time.After(got[i-1].Time) {
			t.Fatalf("entries are not newest first: %v then %v", got[i-1].Time, got[i].Time)
		}
	}
}
