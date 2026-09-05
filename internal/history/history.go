package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// recordSeq keeps two entries written in the same process from sharing a
// file name. Windows clock granularity is around 15 ms, so consecutive calls
// to time.Now() read the same nanosecond and one entry would overwrite the
// other.
var recordSeq uint64

const Schema = "gitmake.history/v1"

type Entry struct {
	Schema     string    `json:"schema"`
	Time       time.Time `json:"time"`
	Command    string    `json:"command"`
	OK         bool      `json:"ok"`
	Repository string    `json:"repository,omitempty"`
	Mode       string    `json:"mode,omitempty"`
	Source     string    `json:"source,omitempty"`
	PlanID     string    `json:"plan_id,omitempty"`
	DryRun     bool      `json:"dry_run,omitempty"`
	ReadOnly   bool      `json:"read_only,omitempty"`
	Added      int       `json:"added,omitempty"`
	Modified   int       `json:"modified,omitempty"`
	Deleted    int       `json:"deleted,omitempty"`
	ReleaseTag string    `json:"release_tag,omitempty"`
	// Branch and Commit are what an undo needs: which ref moved, and to what.
	// Without them a published change can be described but never returned.
	Branch string `json:"branch,omitempty"`
	Commit string `json:"commit,omitempty"`
	// RepoCreated marks a publish that created the repository. Such a run has
	// no earlier state to be returned to.
	RepoCreated bool `json:"repo_created,omitempty"`
	// Undone records that an undo already reverted this entry, so a second one
	// does not revert it again.
	Undone bool `json:"undone,omitempty"`
	// ID is the file the entry was stored under, so an undo can mark it.
	ID        string `json:"id,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	Error     string `json:"error,omitempty"`
}

func directory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "GitMake", "history"), nil
}

func Record(e Entry) error {
	dir, err := directory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if e.Schema == "" {
		e.Schema = Schema
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	id := fmt.Sprintf("%s-%d-%d", e.Time.Format("20060102T150405.000000000Z"), os.Getpid(), atomic.AddUint64(&recordSeq, 1))
	e.ID = id
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, id+".json"), data, 0o600)
}

func List(limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 20
	}
	dir, err := directory()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if len(names) > limit {
		names = names[:limit]
	}
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			continue
		}
		var e Entry
		if json.Unmarshal(data, &e) == nil {
			// Entries written before the id was stored are still addressable:
			// the file name has always been the id.
			if e.ID == "" {
				e.ID = strings.TrimSuffix(n, ".json")
			}
			out = append(out, e)
		}
	}
	return out, nil
}

// MarkUndone records that an entry has been reverted, so a second undo does
// not revert it again. A missing entry is not an error: history is a
// convenience and must never make the operation it describes fail.
func MarkUndone(id string) error {
	if strings.TrimSpace(id) == "" {
		return nil
	}
	dir, err := directory()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, filepath.Base(id)+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var e Entry
	if err := json.Unmarshal(data, &e); err != nil {
		return err
	}
	if e.Undone {
		return nil
	}
	e.Undone = true
	if e.ID == "" {
		e.ID = filepath.Base(id)
	}
	out, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, byte('\n'))
	return os.WriteFile(path, out, 0o600)
}
