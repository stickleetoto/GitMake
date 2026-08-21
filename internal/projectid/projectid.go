package projectid

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const RelativePath = ".gitmake/project.json"
const Schema = "gitmake.project/v1"

type Record struct {
	Schema     string    `json:"schema"`
	ProjectID  string    `json:"project_id"`
	Repository string    `json:"repository"`
	BoundAt    time.Time `json:"bound_at"`
}

func IDFor(repository string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(repository))))
	return "gmp_" + hex.EncodeToString(sum[:8])
}

func New(repository string) Record {
	repository = strings.TrimSpace(repository)
	return Record{Schema: Schema, ProjectID: IDFor(repository), Repository: repository, BoundAt: time.Now().UTC()}
}

func Read(root string) (Record, bool, error) {
	path := filepath.Join(root, filepath.FromSlash(RelativePath))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("read project identity: %w", err)
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, false, fmt.Errorf("parse project identity: %w", err)
	}
	if r.Schema != Schema {
		return Record{}, false, fmt.Errorf("unsupported project identity schema %q", r.Schema)
	}
	if strings.TrimSpace(r.Repository) == "" || strings.TrimSpace(r.ProjectID) == "" {
		return Record{}, false, fmt.Errorf("project identity is incomplete")
	}
	if r.ProjectID != IDFor(r.Repository) {
		return Record{}, false, fmt.Errorf("project identity checksum does not match repository %s", r.Repository)
	}
	return r, true, nil
}

func Validate(root, repository string) (Record, bool, error) {
	r, exists, err := Read(root)
	if err != nil || !exists {
		return r, exists, err
	}
	if !strings.EqualFold(strings.TrimSpace(r.Repository), strings.TrimSpace(repository)) {
		return r, true, fmt.Errorf("project identity mismatch: repository is bound to %s, but this operation targets %s", r.Repository, repository)
	}
	return r, true, nil
}

func WriteRecord(root string, r Record) (Record, error) {
	if strings.TrimSpace(r.Repository) == "" {
		return Record{}, fmt.Errorf("project identity repository is required")
	}
	if r.Schema == "" {
		r.Schema = Schema
	}
	if r.ProjectID == "" {
		r.ProjectID = IDFor(r.Repository)
	}
	if r.BoundAt.IsZero() {
		r.BoundAt = time.Now().UTC()
	}
	if r.Schema != Schema || r.ProjectID != IDFor(r.Repository) {
		return Record{}, fmt.Errorf("invalid project identity record")
	}
	path := filepath.Join(root, filepath.FromSlash(RelativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Record{}, err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return Record{}, err
	}
	data = append(data, '\n')
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(data) {
		return r, nil
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Record{}, fmt.Errorf("write project identity: %w", err)
	}
	return r, nil
}

func Write(root, repository string) (Record, error) {
	return WriteRecord(root, New(repository))
}
