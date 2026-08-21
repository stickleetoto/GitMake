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

const Schema = "gitmake.approval/v1"
const ttl = 30 * time.Minute

type Record struct {
	Schema    string    `json:"schema"`
	PlanID    string    `json:"plan_id"`
	TokenHash string    `json:"token_hash"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

func Create(planID string) (token string, expires time.Time, err error) {
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
	r := Record{Schema: Schema, PlanID: planID, TokenHash: hash(token), CreatedAt: now, ExpiresAt: expires}
	if err := save(r); err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func Validate(planID, token string) error {
	r, err := load(planID)
	if err != nil {
		return err
	}
	if r.Used {
		return fmt.Errorf("approval token for plan %s was already used", planID)
	}
	if time.Now().UTC().After(r.ExpiresAt) {
		return fmt.Errorf("approval token for plan %s expired", planID)
	}
	if !strings.EqualFold(r.TokenHash, hash(token)) {
		return fmt.Errorf("approval token is invalid for plan %s", planID)
	}
	return nil
}

func Consume(planID, token string) error {
	if err := Validate(planID, token); err != nil {
		return err
	}
	r, err := load(planID)
	if err != nil {
		return err
	}
	r.Used = true
	return save(r)
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
	return os.WriteFile(path, data, 0o600)
}

func load(planID string) (Record, error) {
	path, err := pathFor(planID)
	if err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Record{}, fmt.Errorf("no approval token exists for plan %s; run `gitmake approve %s`", planID, planID)
	}
	if err != nil {
		return Record{}, err
	}
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return Record{}, err
	}
	if r.Schema != Schema || r.PlanID != planID {
		return Record{}, fmt.Errorf("invalid approval record for plan %s", planID)
	}
	return r, nil
}

func hash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
