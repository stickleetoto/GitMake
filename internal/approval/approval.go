package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	Schema       = "gitmake.approval/v2"
	LegacySchema = "gitmake.approval/v1"
)

const ttl = 10 * time.Minute

// Binding ties a human approval to the exact reviewed plan contents. The grant
// is intentionally local-only; MCP can consume it but cannot mint one.
type Binding struct {
	Fingerprint  string `json:"fingerprint"`
	SourceSHA256 string `json:"source_sha256"`
	ConfigSHA256 string `json:"config_sha256,omitempty"`
	Repository   string `json:"repository"`
}

type Record struct {
	Schema      string    `json:"schema"`
	PlanID      string    `json:"plan_id"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Used        bool      `json:"used"`
	UsedAt      time.Time `json:"used_at,omitempty"`
	Destructive bool      `json:"destructive"`
	Binding     Binding   `json:"binding,omitempty"`

	// TokenHash is retained only so v1 approval records from pre-1.0 builds
	// can still be consumed during the compatibility window. New approvals do
	// not expose or require a token.
	TokenHash string `json:"token_hash,omitempty"`
}

func CreateGrant(planID string, binding Binding, destructive bool) (Record, error) {
	if strings.TrimSpace(planID) == "" {
		return Record{}, fmt.Errorf("plan id is required")
	}
	if strings.TrimSpace(binding.Fingerprint) == "" || strings.TrimSpace(binding.SourceSHA256) == "" || strings.TrimSpace(binding.Repository) == "" {
		return Record{}, fmt.Errorf("approval binding is incomplete")
	}

	// Creating the same grant is idempotent while it is still unused. Once a
	// reviewed plan has successfully consumed its grant, that exact plan cannot
	// be re-approved through a replayed MCP elicitation. A fresh reviewed plan is
	// required for another mutation.
	if existing, err := load(planID); err == nil && existing.Schema == Schema {
		if !sameBinding(existing.Binding, binding) {
			return Record{}, fmt.Errorf("existing approval for plan %s is bound to different reviewed content", planID)
		}
		if existing.Used {
			return Record{}, fmt.Errorf("approval for plan %s was already used; create a fresh reviewed plan", planID)
		}
		if time.Now().UTC().Before(existing.ExpiresAt) {
			if destructive && !existing.Destructive {
				return Record{}, fmt.Errorf("existing approval for plan %s is not destructive; create a fresh reviewed plan", planID)
			}
			return existing, nil
		}
	}

	now := time.Now().UTC()
	r := Record{
		Schema: Schema, PlanID: planID, CreatedAt: now, ExpiresAt: now.Add(ttl),
		Destructive: destructive, Binding: binding,
	}
	if err := save(r); err != nil {
		return Record{}, err
	}
	return r, nil
}

func ValidateGrant(planID string, binding Binding) (Record, error) {
	r, err := load(planID)
	if err != nil {
		return Record{}, err
	}
	if r.Used {
		return Record{}, fmt.Errorf("approval for plan %s was already used", planID)
	}
	if time.Now().UTC().After(r.ExpiresAt) {
		return Record{}, fmt.Errorf("approval for plan %s expired; run `gitmake approve` again", planID)
	}
	if r.Schema == LegacySchema {
		return Record{}, fmt.Errorf("legacy token approval exists for plan %s; create a fresh tokenless approval with `gitmake approve`", planID)
	}
	if r.Schema != Schema {
		return Record{}, fmt.Errorf("unsupported approval schema %q", r.Schema)
	}
	if !sameBinding(r.Binding, binding) {
		return Record{}, fmt.Errorf("approval for plan %s no longer matches the reviewed plan", planID)
	}
	return r, nil
}

func ConsumeGrant(planID string, binding Binding) error {
	r, err := ValidateGrant(planID, binding)
	if err != nil {
		return err
	}
	r.Used = true
	r.UsedAt = time.Now().UTC()
	return save(r)
}

func sameBinding(a, b Binding) bool {
	return strings.EqualFold(a.Fingerprint, b.Fingerprint) &&
		strings.EqualFold(a.SourceSHA256, b.SourceSHA256) &&
		strings.EqualFold(a.ConfigSHA256, b.ConfigSHA256) &&
		strings.EqualFold(a.Repository, b.Repository)
}

// Legacy token helpers are intentionally preserved for compatibility with an
// already-issued pre-1.0 approval. New UI/MCP flows use CreateGrant instead.
func Create(planID string, destructive ...bool) (token string, expires time.Time, err error) {
	if strings.TrimSpace(planID) == "" {
		return "", time.Time{}, fmt.Errorf("plan id is required")
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token = "gma_" + hex.EncodeToString(raw)
	now := time.Now().UTC()
	expires = now.Add(ttl)
	d := len(destructive) > 0 && destructive[0]
	r := Record{Schema: LegacySchema, PlanID: planID, TokenHash: hash(token), CreatedAt: now, ExpiresAt: expires, Destructive: d}
	if err := save(r); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func ValidateRecord(planID, token string) (Record, error) {
	r, err := load(planID)
	if err != nil {
		return Record{}, err
	}
	if r.Used {
		return Record{}, fmt.Errorf("approval token for plan %s was already used", planID)
	}
	if time.Now().UTC().After(r.ExpiresAt) {
		return Record{}, fmt.Errorf("approval token for plan %s expired", planID)
	}
	if r.Schema != LegacySchema || !strings.EqualFold(r.TokenHash, hash(token)) {
		return Record{}, fmt.Errorf("approval token is invalid for plan %s", planID)
	}
	return r, nil
}

func Validate(planID, token string) error {
	_, err := ValidateRecord(planID, token)
	return err
}

func Consume(planID, token string) error {
	r, err := ValidateRecord(planID, token)
	if err != nil {
		return err
	}
	r.Used = true
	r.UsedAt = time.Now().UTC()
	return save(r)
}

func Cleanup() error {
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
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}
		var r Record
		if json.Unmarshal(data, &r) != nil {
			if info, statErr := e.Info(); statErr == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(path)
			}
			continue
		}
		if r.Used {
			// Keep recently consumed grants for a day so a repeated MCP apply can
			// report "already used" instead of looking like approval vanished.
			usedAt := r.UsedAt
			if usedAt.IsZero() {
				usedAt = r.ExpiresAt
			}
			if !usedAt.IsZero() && time.Since(usedAt) > 24*time.Hour {
				_ = os.Remove(path)
			}
			continue
		}
		if !r.ExpiresAt.IsZero() && time.Now().UTC().After(r.ExpiresAt) {
			_ = os.Remove(path)
		}
	}
	return nil
}

func directory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "GitMake", "approvals"), nil
}

func pathFor(planID string) (string, error) {
	if strings.ContainsAny(planID, `/\\`) || strings.Contains(planID, "..") {
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
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		// Windows cannot atomically replace an existing file with Rename.
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
}

func load(planID string) (Record, error) {
	path, err := pathFor(planID)
	if err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Record{}, fmt.Errorf("no human approval exists for plan %s; run `gitmake approve`", planID)
	}
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, err
	}
	if r.PlanID != planID {
		return Record{}, fmt.Errorf("invalid approval record for plan %s", planID)
	}
	return r, nil
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
