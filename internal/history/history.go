package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

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
	ErrorCode  string    `json:"error_code,omitempty"`
	Error      string    `json:"error,omitempty"`
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
	name := fmt.Sprintf("%s-%d.json", e.Time.Format("20060102T150405.000000000Z"), time.Now().UnixNano())
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(dir, name), data, 0o600)
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
			out = append(out, e)
		}
	}
	return out, nil
}
