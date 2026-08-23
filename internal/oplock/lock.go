package oplock

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

const staleAfter = 6 * time.Hour

type Lock struct {
	path string
}

type record struct {
	Key       string    `json:"key"`
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

func Acquire(key string) (*Lock, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("lock key is required")
	}
	dir, err := directory()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, fileName(key))
	for attempt := 0; attempt < 2; attempt++ {
		f, openErr := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if openErr == nil {
			r := record{Key: key, PID: os.Getpid(), CreatedAt: time.Now().UTC()}
			_ = json.NewEncoder(f).Encode(r)
			_ = f.Close()
			return &Lock{path: path}, nil
		}
		if !os.IsExist(openErr) {
			return nil, openErr
		}
		if !stale(path) {
			return nil, fmt.Errorf("another GitMake operation is already running for %s", key)
		}
		_ = os.Remove(path)
	}
	return nil, fmt.Errorf("could not acquire GitMake operation lock for %s", key)
}

func (l *Lock) Release() {
	if l != nil && l.path != "" {
		_ = os.Remove(l.path)
	}
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
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".lock") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if stale(path) {
			_ = os.Remove(path)
		}
	}
	return nil
}

func stale(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return time.Since(info.ModTime()) > staleAfter
}

func directory() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "GitMake", "locks"), nil
}

func fileName(key string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(key)))
	return hex.EncodeToString(sum[:16]) + ".lock"
}
