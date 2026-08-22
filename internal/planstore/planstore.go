package planstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const Schema = "gitmake.plan/v1"

type ChangeCounts struct {
	Added    int `json:"added"`
	Modified int `json:"modified"`
	Deleted  int `json:"deleted"`
}

type FileDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type Release struct {
	Enabled     bool         `json:"enabled"`
	Tag         string       `json:"tag,omitempty"`
	Assets      []FileDigest `json:"assets,omitempty"`
	NotesSHA256 string       `json:"notes_sha256,omitempty"`
}

type ProjectIdentity struct {
	Status     string `json:"status"`
	ProjectID  string `json:"project_id,omitempty"`
	Repository string `json:"repository,omitempty"`
}

type Risk struct {
	Level           string   `json:"level"`
	Destructive     bool     `json:"destructive"`
	DeletionRatio   float64  `json:"deletion_ratio"`
	Deleted         int      `json:"deleted"`
	ManagedBaseline int      `json:"managed_baseline"`
	Reasons         []string `json:"reasons,omitempty"`
}

type Plan struct {
	Schema           string          `json:"schema"`
	ID               string          `json:"plan_id"`
	CreatedAt        time.Time       `json:"created_at"`
	WorkingDirectory string          `json:"working_directory"`
	ConfigPath       string          `json:"config_path,omitempty"`
	ConfigPersisted  bool            `json:"config_persisted"`
	ConfigSHA256     string          `json:"config_sha256,omitempty"`
	SourceMode       string          `json:"source_mode,omitempty"`
	SourcePath       string          `json:"source_path"`
	SourceSHA256     string          `json:"source_sha256"`
	Repository       string          `json:"repository"`
	Visibility       string          `json:"visibility"`
	RemoteVisibility string          `json:"remote_visibility,omitempty"`
	Mode             string          `json:"mode"`
	Branch           string          `json:"branch"`
	BaseCommit       string          `json:"base_commit,omitempty"`
	Changes          ChangeCounts    `json:"changes"`
	Release          Release         `json:"release"`
	Identity         ProjectIdentity `json:"project_identity"`
	Risk             Risk            `json:"risk"`
	ReviewNotes      []string        `json:"review_notes,omitempty"`
	DecisionNotes    []string        `json:"decision_notes,omitempty"`
	Fingerprint      string          `json:"fingerprint"`
}

func NewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "gm_" + hex.EncodeToString(b), nil
}

func directory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "GitMake", "plans"), nil
}

func Save(p Plan) (string, error) {
	if p.ID == "" {
		return "", fmt.Errorf("plan id is required")
	}
	dir, err := directory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, p.ID+".json")
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func Load(id string) (Plan, string, error) {
	if id == "" {
		return Plan{}, "", fmt.Errorf("plan id is required")
	}
	dir, err := directory()
	if err != nil {
		return Plan{}, "", err
	}
	path := filepath.Join(dir, id+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Plan{}, path, fmt.Errorf("plan %s not found", id)
	}
	if err != nil {
		return Plan{}, path, err
	}
	var p Plan
	if err := json.Unmarshal(data, &p); err != nil {
		return Plan{}, path, fmt.Errorf("parse plan %s: %w", id, err)
	}
	if p.Schema != Schema {
		return Plan{}, path, fmt.Errorf("unsupported plan schema %q", p.Schema)
	}
	if p.ID != id {
		return Plan{}, path, fmt.Errorf("plan id mismatch: expected %s, found %s", id, p.ID)
	}
	return p, path, nil
}
