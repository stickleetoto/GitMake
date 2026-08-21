package syncer

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gitmake/internal/pathmatch"
)

const manifestRelative = ".gitmake/managed.json"
const projectIdentityRelative = ".gitmake/project.json"
const manifestSchema = "gitmake.managed/v1"

type Manifest struct {
	Schema  string   `json:"schema"`
	Managed []string `json:"managed"`
}

type Result struct {
	ManagedFiles int
	PriorManaged int      `json:"managed_files"`
	Deleted      []string `json:"deleted,omitempty"`
	Preserved    []string `json:"preserved,omitempty"`
	Manifest     string   `json:"manifest"`
	FirstAdopt   bool     `json:"first_adopt"`
}

// SyncSnapshot updates repo from source. In managed mode GitMake only deletes
// files that a previous GitMake manifest says it managed. Remote-only files are
// preserved on first adoption, and protected paths are never deleted. Snapshot
// mode retains the legacy full mirror behavior, except protected paths survive.
func SyncSnapshot(source, repo, mode string, protected []string) (Result, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "managed"
	}
	switch mode {
	case "managed":
		return managedSync(source, repo, protected)
	case "snapshot":
		return snapshotSync(source, repo, protected)
	default:
		return Result{}, fmt.Errorf("unsupported sync mode %q", mode)
	}
}

func PrepareCreateSnapshot(source string) (Result, error) {
	files, err := listRegularFiles(source)
	if err != nil {
		return Result{}, err
	}
	if containsFold(files, manifestRelative) || containsFold(files, projectIdentityRelative) {
		return Result{}, fmt.Errorf("source snapshot contains reserved GitMake metadata path under .gitmake")
	}
	if err := writeManifest(source, files); err != nil {
		return Result{}, err
	}
	return Result{ManagedFiles: len(files), Manifest: manifestRelative, FirstAdopt: true}, nil
}

func managedSync(source, repo string, protected []string) (Result, error) {
	current, err := listRegularFiles(source)
	if err != nil {
		return Result{}, err
	}
	if containsFold(current, manifestRelative) || containsFold(current, projectIdentityRelative) {
		return Result{}, fmt.Errorf("source snapshot contains reserved GitMake metadata path under .gitmake")
	}
	prior, exists, err := loadManifest(repo)
	if err != nil {
		return Result{}, err
	}
	currentSet := toSet(current)
	result := Result{ManagedFiles: len(current), Manifest: manifestRelative, FirstAdopt: !exists}
	if exists {
		result.PriorManaged = len(prior.Managed)
	}

	if exists {
		for _, rel := range prior.Managed {
			rel = cleanRel(rel)
			if rel == "" || strings.EqualFold(rel, manifestRelative) || currentSet[strings.ToLower(rel)] {
				continue
			}
			if pathmatch.Any(protected, rel) {
				result.Preserved = append(result.Preserved, rel)
				continue
			}
			dst := filepath.Join(repo, filepath.FromSlash(rel))
			makeWritable(dst)
			if err := os.RemoveAll(dst); err != nil {
				return Result{}, fmt.Errorf("delete previously managed path %q: %w", rel, err)
			}
			result.Deleted = append(result.Deleted, rel)
			removeEmptyParents(filepath.Dir(dst), repo)
		}
	}

	// Copy every source path over the working tree. Remote-only paths remain.
	entries, err := os.ReadDir(source)
	if err != nil {
		return Result{}, fmt.Errorf("read snapshot: %w", err)
	}
	for _, e := range entries {
		src := filepath.Join(source, e.Name())
		dst := filepath.Join(repo, e.Name())
		if err := replaceTree(src, dst); err != nil {
			return Result{}, err
		}
	}
	if err := writeManifest(repo, current); err != nil {
		return Result{}, err
	}
	sort.Strings(result.Deleted)
	sort.Strings(result.Preserved)
	return result, nil
}

func snapshotSync(source, repo string, protected []string) (Result, error) {
	sourceFiles, err := listRegularFiles(source)
	if err != nil {
		return Result{}, err
	}
	if containsFold(sourceFiles, manifestRelative) || containsFold(sourceFiles, projectIdentityRelative) {
		return Result{}, fmt.Errorf("source snapshot contains reserved GitMake metadata path under .gitmake")
	}
	result := Result{ManagedFiles: len(sourceFiles), Manifest: manifestRelative}
	currentSet := toSet(sourceFiles)

	// Delete individual remote files that are absent from the snapshot, rather
	// than deleting top-level directories. This lets nested protected patterns
	// such as docs/important.md survive snapshot mode safely.
	var repoFiles []string
	err = filepath.WalkDir(repo, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == repo {
			return nil
		}
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == ".git" || strings.HasPrefix(rel, ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(rel, manifestRelative) {
			return nil
		}
		repoFiles = append(repoFiles, rel)
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	for _, rel := range repoFiles {
		if currentSet[strings.ToLower(cleanRel(rel))] {
			continue
		}
		if pathmatch.Any(protected, rel) {
			result.Preserved = append(result.Preserved, rel)
			continue
		}
		dst := filepath.Join(repo, filepath.FromSlash(rel))
		makeWritable(dst)
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return Result{}, fmt.Errorf("remove old repository path %q: %w", rel, err)
		}
		removeEmptyParents(filepath.Dir(dst), repo)
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return Result{}, fmt.Errorf("read snapshot: %w", err)
	}
	for _, e := range entries {
		src := filepath.Join(source, e.Name())
		dst := filepath.Join(repo, e.Name())
		if err := replaceTree(src, dst); err != nil {
			return Result{}, err
		}
	}
	if err := writeManifest(repo, sourceFiles); err != nil {
		return Result{}, err
	}
	sort.Strings(result.Preserved)
	return result, nil
}

func loadManifest(repo string) (Manifest, bool, error) {
	path := filepath.Join(repo, filepath.FromSlash(manifestRelative))
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Manifest{}, false, nil
	}
	if err != nil {
		return Manifest{}, false, fmt.Errorf("read GitMake managed manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return Manifest{}, false, fmt.Errorf("parse GitMake managed manifest: %w", err)
	}
	if m.Schema != manifestSchema {
		return Manifest{}, false, fmt.Errorf("unsupported GitMake managed manifest schema %q", m.Schema)
	}
	return m, true, nil
}

func writeManifest(root string, files []string) error {
	clean := make([]string, 0, len(files))
	seen := map[string]bool{}
	for _, rel := range files {
		rel = cleanRel(rel)
		if rel == "" || strings.EqualFold(rel, manifestRelative) {
			continue
		}
		key := strings.ToLower(rel)
		if !seen[key] {
			seen[key] = true
			clean = append(clean, rel)
		}
	}
	sort.Strings(clean)
	m := Manifest{Schema: manifestSchema, Managed: clean}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path := filepath.Join(root, filepath.FromSlash(manifestRelative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write GitMake managed manifest: %w", err)
	}
	return nil
}

func listRegularFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("snapshot symlink is not allowed: %s", path)
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported snapshot file type: %s", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

func toSet(files []string) map[string]bool {
	out := make(map[string]bool, len(files))
	for _, f := range files {
		out[strings.ToLower(cleanRel(f))] = true
	}
	return out
}

func containsFold(items []string, want string) bool {
	for _, v := range items {
		if strings.EqualFold(cleanRel(v), cleanRel(want)) {
			return true
		}
	}
	return false
}

func cleanRel(v string) string {
	v = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(v))), "./")
	if v == "." {
		return ""
	}
	return strings.Trim(v, "/")
}

func removeEmptyParents(dir, stop string) {
	stop = filepath.Clean(stop)
	for {
		dir = filepath.Clean(dir)
		if dir == stop || !strings.HasPrefix(strings.ToLower(dir), strings.ToLower(stop)+strings.ToLower(string(filepath.Separator))) {
			return
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			return
		}
		_ = os.Remove(dir)
		dir = filepath.Dir(dir)
	}
}

func replaceTree(src, dst string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("snapshot symlink is not allowed: %s", src)
	}
	if existing, err := os.Lstat(dst); err == nil {
		if info.IsDir() != existing.IsDir() {
			makeWritable(dst)
			if err := os.RemoveAll(dst); err != nil {
				return err
			}
		} else if !info.IsDir() {
			makeWritable(dst)
		}
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := replaceTree(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported snapshot file type: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, fs.FileMode(info.Mode().Perm()))
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

// makeWritable is best-effort cleanup for Windows repositories that contain
// read-only files.
func makeWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mode := info.Mode().Perm() | 0o200
		if d.IsDir() {
			mode |= 0o100
		}
		_ = os.Chmod(path, mode)
		return nil
	})
}
