package opjournal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const Schema = "gitmake.operation/v1"

type Record struct {
	Schema     string    `json:"schema"`
	PlanID     string    `json:"plan_id"`
	Repository string    `json:"repository"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	Status     string    `json:"status"`
	Error      string    `json:"error,omitempty"`
}

// Begin records an apply before any mutation. interrupted is true when the
// previous record for the same plan never reached a terminal state.
func Begin(planID, repository string) (interrupted bool, err error) {
	previous, loadErr := load(planID)
	if loadErr == nil && previous.Status == "running" {
		interrupted = true
	}
	now := time.Now().UTC()
	return interrupted, save(Record{Schema: Schema, PlanID: planID, Repository: repository, StartedAt: now, Status: "running"})
}

func Finish(planID string, runErr error) error {
	r, err := load(planID)
	if err != nil {
		return err
	}
	r.FinishedAt = time.Now().UTC()
	if runErr == nil {
		r.Status = "succeeded"
		r.Error = ""
	} else {
		r.Status = "failed"
		r.Error = runErr.Error()
	}
	return save(r)
}

func Cleanup(maxAge time.Duration) error {
	dir, err := directory()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, statErr := e.Info()
		if statErr == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func directory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "GitMake", "operations"), nil
}

func pathFor(planID string) (string, error) {
	if planID == "" || strings.ContainsAny(planID, `/\\`) || strings.Contains(planID, "..") {
		return "", fmt.Errorf("invalid plan id")
	}
	dir, err := directory()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, planID+".json"), nil
}

func save(r Record) error {
	dir, err := directory()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path, err := pathFor(r.PlanID)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o600)
}

func load(planID string) (Record, error) {
	path, err := pathFor(planID)
	if err != nil {
		return Record{}, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(b, &r); err != nil {
		return Record{}, err
	}
	if r.Schema != Schema || r.PlanID != planID {
		return Record{}, fmt.Errorf("invalid operation journal for %s", planID)
	}
	return r, nil
}
