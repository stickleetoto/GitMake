package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitmake/internal/config"
)

const maxConfigStdin = 2 << 20 // 2 MiB is intentionally generous for a tiny config file.

type configValidationResult struct {
	Schema  string        `json:"schema"`
	OK      bool          `json:"ok"`
	Version string        `json:"version"`
	Path    string        `json:"path"`
	Config  config.Config `json:"config"`
}

type configWriteResult struct {
	Schema  string        `json:"schema"`
	OK      bool          `json:"ok"`
	Version string        `json:"version"`
	Action  string        `json:"action"`
	Path    string        `json:"path"`
	DryRun  bool          `json:"dry_run"`
	Written bool          `json:"written"`
	Config  config.Config `json:"config"`
	SHA256  string        `json:"sha256,omitempty"`
}

func runConfig(o Options) error {
	switch o.ConfigAction {
	case "schema":
		if o.JSON {
			return emitJSON(config.SchemaDocument())
		}
		data, err := json.MarshalIndent(config.SchemaDocument(), "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	case "validate":
		return runConfigValidate(o)
	case "write":
		return runConfigWrite(o, false)
	case "patch":
		return runConfigWrite(o, true)
	default:
		return fmt.Errorf("unknown config action %q", o.ConfigAction)
	}
}

func configPathForCommand(o Options) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return resolveConfigPath(cwd, o.ConfigPath), nil
}

func runConfigValidate(o Options) error {
	path, err := configPathForCommand(o)
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}
	if o.JSON {
		return emitJSON(configValidationResult{Schema: "gitmake.config-validation/v1", OK: true, Version: Version, Path: path, Config: cfg})
	}
	fmt.Printf("GitMake Config · %s\n\n", Version)
	fmt.Println("✓ Valid", path)
	fmt.Printf("  Repository  %s · %s\n", cfg.Repo.Name, cfg.Repo.Visibility)
	if cfg.Source.Folder != "" {
		fmt.Printf("  Source      folder · %s\n", cfg.Source.Folder)
	} else {
		fmt.Printf("  Source      zip · %s\n", cfg.Source.ZIP)
	}
	fmt.Printf("  Branch      %s\n", cfg.Git.Branch)
	if cfg.Release.Enabled {
		fmt.Printf("  Release     %s · %d assets\n", cfg.Release.Tag, len(cfg.Release.Assets))
	}
	return nil
}

func readLimitedStdin() ([]byte, error) {
	lr := &io.LimitedReader{R: os.Stdin, N: maxConfigStdin + 1}
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	if len(data) > maxConfigStdin {
		return nil, fmt.Errorf("config stdin exceeds %d bytes", maxConfigStdin)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("config stdin is empty")
	}
	return data, nil
}

func runConfigWrite(o Options, patch bool) error {
	path, err := configPathForCommand(o)
	if err != nil {
		return err
	}
	incoming, err := readLimitedStdin()
	if err != nil {
		return err
	}

	var cfg config.Config
	action := "write"
	if patch {
		action = "patch"
		baseBytes, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("config patch requires an existing config: %s", path)
			}
			return fmt.Errorf("read config for patch: %w", err)
		}
		merged, err := mergeJSONObjects(baseBytes, incoming)
		if err != nil {
			return fmt.Errorf("merge config patch: %w", err)
		}
		cfg, err = config.ParseBytes(merged)
		if err != nil {
			return err
		}
	} else {
		cfg, err = config.ParseBytes(incoming)
		if err != nil {
			return err
		}
	}

	normalized, err := config.MarshalNormalized(cfg)
	if err != nil {
		return err
	}
	written := false
	sha := ""
	if !o.DryRun {
		if err := atomicWriteFile(path, normalized, 0o644); err != nil {
			return fmt.Errorf("write config: %w", err)
		}
		written = true
		sha, _ = sha256File(path)
	} else {
		sum := sha256Bytes(normalized)
		sha = sum
	}

	if o.JSON {
		return emitJSON(configWriteResult{
			Schema: "gitmake.config-write/v1", OK: true, Version: Version,
			Action: action, Path: path, DryRun: o.DryRun, Written: written, Config: cfg, SHA256: sha,
		})
	}
	fmt.Printf("GitMake Config · %s\n\n", Version)
	if o.DryRun {
		fmt.Printf("· Would %s             %s\n", action, path)
	} else {
		fmt.Printf("✓ Config %sed          %s\n", action, path)
	}
	fmt.Printf("✓ Valid                %s · %s\n", cfg.Repo.Name, cfg.Repo.Visibility)
	fmt.Printf("· SHA-256              %.16s…\n", sha)
	return nil
}

func mergeJSONObjects(base, patch []byte) ([]byte, error) {
	var left map[string]any
	if err := json.Unmarshal(bytes.TrimPrefix(base, []byte{0xEF, 0xBB, 0xBF}), &left); err != nil {
		return nil, fmt.Errorf("parse existing config: %w", err)
	}
	var right map[string]any
	if err := json.Unmarshal(patch, &right); err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}
	if left == nil || right == nil {
		return nil, fmt.Errorf("config and patch must both be JSON objects")
	}
	mergeMap(left, right)
	return json.Marshal(left)
}

// mergeMap uses RFC 7396-style object merge semantics for GitMake patches:
// object values merge recursively, null deletes a field, and all other values
// replace the previous value.
func mergeMap(dst, patch map[string]any) {
	for key, value := range patch {
		if value == nil {
			delete(dst, key)
			continue
		}
		pm, pok := value.(map[string]any)
		if !pok {
			dst[key] = value
			continue
		}
		dm, dok := dst[key].(map[string]any)
		if !dok {
			dm = map[string]any{}
			dst[key] = dm
		}
		mergeMap(dm, pm)
	}
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".gitmake-config-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	_ = os.Chmod(tmp, mode)
	// Windows cannot atomically rename over an existing file. Preserve the old
	// config as a temporary backup until the validated replacement is in place,
	// restoring it if the final rename fails.
	backup := ""
	if _, err := os.Stat(path); err == nil {
		backup = path + ".gitmake-backup"
		_ = os.Remove(backup)
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, path); err != nil {
		if backup != "" {
			_ = os.Rename(backup, path)
		}
		return err
	}
	if backup != "" {
		_ = os.Remove(backup)
	}
	return nil
}

func sha256Bytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
