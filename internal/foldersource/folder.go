package foldersource

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Report struct {
	Root    string `json:"root"`
	Files   int    `json:"files"`
	Ignored int    `json:"ignored"`
	SHA256  string `json:"sha256"`
}

type Detection struct {
	IsProject  bool     `json:"is_project"`
	Score      int      `json:"score"`
	Confidence string   `json:"confidence"`
	Evidence   []string `json:"evidence,omitempty"`
}

type ignoreRule struct {
	base     string
	negative bool
	dirOnly  bool
	re       *regexp.Regexp
}

var defaultIgnoredDirs = map[string]bool{
	".git": true, ".gitmake": true, "node_modules": true, "__pycache__": true,
	".venv": true, "venv": true,
}
var defaultIgnoredFiles = map[string]bool{
	"gitmake.json": true, ".gitmakeignore": true, ".env": true, ".DS_Store": true, "Thumbs.db": true,
}

func DetectProject(dir string) (Detection, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Detection{}, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return Detection{}, err
	}
	markers := map[string]int{
		".git": 5, "go.mod": 5, "Cargo.toml": 5, "package.json": 5,
		"pyproject.toml": 5, "setup.py": 4, "requirements.txt": 3,
		"pom.xml": 5, "build.gradle": 5, "CMakeLists.txt": 4,
		"README.md": 2, "README.rst": 2, "LICENSE": 1,
		"src": 4, "tests": 3, "cmd": 3, "lib": 2,
	}
	sourceExt := map[string]bool{".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true, ".rs": true, ".c": true, ".h": true, ".cpp": true, ".cc": true, ".java": true, ".kt": true, ".cs": true, ".rb": true, ".php": true, ".swift": true}
	d := Detection{}
	zipCount := 0
	for _, e := range entries {
		name := e.Name()
		if defaultIgnoredDirs[name] || defaultIgnoredFiles[name] {
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".zip") && !e.IsDir() {
			zipCount++
			continue
		}
		for marker, score := range markers {
			if strings.EqualFold(name, marker) {
				d.Score += score
				d.Evidence = append(d.Evidence, name)
				break
			}
		}
		if !e.IsDir() && sourceExt[strings.ToLower(filepath.Ext(name))] {
			d.Score += 3
			d.Evidence = append(d.Evidence, "source file: "+name)
		}
	}
	if d.Score >= 5 {
		d.IsProject = true
		d.Confidence = "high"
	} else if d.Score >= 3 {
		d.IsProject = true
		d.Confidence = "medium"
	} else if zipCount == 0 && d.Score >= 2 {
		d.IsProject = true
		d.Confidence = "low"
	}
	sort.Strings(d.Evidence)
	return d, nil
}

func Snapshot(root, dest string) (Report, error) { return walk(root, dest, true) }
func Hash(root string) (Report, error)           { return walk(root, "", false) }

func walk(root, dest string, copyFiles bool) (Report, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return Report{}, fmt.Errorf("source folder: %w", err)
	}
	if !st.IsDir() {
		return Report{}, fmt.Errorf("source folder is not a directory: %s", abs)
	}
	if copyFiles {
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return Report{}, err
		}
	}

	var rules []ignoreRule
	rootRules, err := loadIgnoreFile(abs, "", ".gitignore")
	if err != nil {
		return Report{}, err
	}
	rules = append(rules, rootRules...)
	gmRules, err := loadIgnoreFile(abs, "", ".gitmakeignore")
	if err != nil {
		return Report{}, err
	}
	rules = append(rules, gmRules...)

	type item struct {
		rel, full string
		mode      os.FileMode
	}
	var files []item
	ignored := 0
	seenCase := map[string]string{}

	err = filepath.WalkDir(abs, func(full string, de os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if full == abs {
			return nil
		}
		relOS, err := filepath.Rel(abs, full)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		name := de.Name()
		info, err := de.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("source folder symlink is not allowed: %s", rel)
		}
		isDir := de.IsDir()
		if hardIgnored(rel, name, isDir) {
			ignored++
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if ignoredByRules(rules, rel, isDir) {
			ignored++
			if isDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isDir {
			nested, err := loadIgnoreFile(full, rel, ".gitignore")
			if err != nil {
				return err
			}
			rules = append(rules, nested...)
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported source entry type: %s", rel)
		}
		key := strings.ToLower(rel)
		if prev, ok := seenCase[key]; ok && prev != rel {
			return fmt.Errorf("source folder contains case-colliding paths: %q and %q", prev, rel)
		}
		seenCase[key] = rel
		if err := validateRelativePath(rel); err != nil {
			return err
		}
		files = append(files, item{rel: rel, full: full, mode: info.Mode()})
		return nil
	})
	if err != nil {
		return Report{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })

	h := sha256.New()
	for _, f := range files {
		var digest []byte
		if copyFiles {
			// Hash the exact bytes that are copied into the snapshot. This makes
			// source changes between the earlier plan hash and snapshot creation
			// observable as a hash mismatch instead of creating a TOCTOU gap.
			target := filepath.Join(dest, filepath.FromSlash(f.rel))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return Report{}, err
			}
			in, err := os.Open(f.full)
			if err != nil {
				return Report{}, err
			}
			mode := f.mode.Perm()
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				_ = in.Close()
				return Report{}, err
			}
			fileHash := sha256.New()
			_, cErr := io.Copy(io.MultiWriter(out, fileHash), in)
			e1, e2 := out.Close(), in.Close()
			if cErr != nil {
				return Report{}, cErr
			}
			if e1 != nil {
				return Report{}, e1
			}
			if e2 != nil {
				return Report{}, e2
			}
			digest = fileHash.Sum(nil)
		} else {
			contentHash, err := hashFile(f.full)
			if err != nil {
				return Report{}, err
			}
			digest, _ = hex.DecodeString(contentHash)
		}

		writeHashField(h, []byte(f.rel))
		exec := byte(0)
		if f.mode.Perm()&0o111 != 0 {
			exec = 1
		}
		writeHashField(h, []byte{exec})
		writeHashField(h, digest)
	}
	return Report{Root: abs, Files: len(files), Ignored: ignored, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func hardIgnored(rel, name string, isDir bool) bool {
	if isDir && defaultIgnoredDirs[name] {
		return true
	}
	if !isDir && defaultIgnoredFiles[name] {
		return true
	}
	for _, p := range strings.Split(filepath.ToSlash(rel), "/") {
		if p == ".git" || p == ".gitmake" {
			return true
		}
	}
	return false
}

func loadIgnoreFile(dir, base, filename string) ([]ignoreRule, error) {
	p := filepath.Join(dir, filename)
	f, err := os.Open(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	defer f.Close()
	var out []ignoreRule
	s := bufio.NewScanner(f)
	for s.Scan() {
		raw := strings.TrimRight(s.Text(), "\r")
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		neg := strings.HasPrefix(raw, "!")
		if neg {
			raw = strings.TrimPrefix(raw, "!")
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		dirOnly := strings.HasSuffix(raw, "/")
		raw = strings.TrimSuffix(raw, "/")
		anchored := strings.HasPrefix(raw, "/")
		raw = strings.TrimPrefix(raw, "/")
		re, err := compileGitGlob(raw, anchored, dirOnly)
		if err != nil {
			return nil, fmt.Errorf("invalid ignore pattern %q in %s: %w", raw, p, err)
		}
		out = append(out, ignoreRule{base: filepath.ToSlash(base), negative: neg, dirOnly: dirOnly, re: re})
	}
	if err := s.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func ignoredByRules(rules []ignoreRule, rel string, isDir bool) bool {
	ignored := false
	rel = filepath.ToSlash(rel)
	for _, r := range rules {
		sub := rel
		if r.base != "" {
			if rel == r.base {
				sub = ""
			} else if strings.HasPrefix(rel, r.base+"/") {
				sub = strings.TrimPrefix(rel, r.base+"/")
			} else {
				continue
			}
		}
		if sub == "" {
			continue
		}
		if r.re.MatchString(sub) {
			if r.dirOnly && !isDir && !matchesDirectoryPrefix(r.re, sub) {
				continue
			}
			ignored = !r.negative
		}
	}
	return ignored
}

func matchesDirectoryPrefix(re *regexp.Regexp, sub string) bool {
	parts := strings.Split(sub, "/")
	for i := 1; i < len(parts); i++ {
		if re.MatchString(strings.Join(parts[:i], "/")) {
			return true
		}
	}
	return false
}

func compileGitGlob(pattern string, anchored, dirOnly bool) (*regexp.Regexp, error) {
	hasSlash := strings.Contains(pattern, "/")
	var b strings.Builder
	if anchored || hasSlash {
		b.WriteString("^")
	} else {
		b.WriteString("(?:^|.*/)")
	}
	for i := 0; i < len(pattern); {
		c := pattern[i]
		if c == '*' {
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				i += 2
				if i < len(pattern) && pattern[i] == '/' {
					i++
					b.WriteString("(?:.*/)?")
				} else {
					b.WriteString(".*")
				}
			} else {
				i++
				b.WriteString("[^/]*")
			}
			continue
		}
		if c == '?' {
			b.WriteString("[^/]")
			i++
			continue
		}
		b.WriteString(regexp.QuoteMeta(string(c)))
		i++
	}
	if dirOnly {
		b.WriteString("(?:/.*)?$")
	} else {
		b.WriteString("$")
	}
	return regexp.Compile(b.String())
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func writeHashField(w io.Writer, b []byte) {
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(b)))
	_, _ = w.Write(n[:])
	_, _ = w.Write(b)
}

func validateRelativePath(rel string) error {
	clean := path.Clean(filepath.ToSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return fmt.Errorf("unsafe source path: %q", rel)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("unsafe source path: %q", rel)
		}
		if len(part) > 255 {
			return fmt.Errorf("source path component is too long: %q", part)
		}
		if strings.ContainsAny(part, `<>:\"|?*`) {
			return fmt.Errorf("source path contains Windows-reserved characters: %q", rel)
		}
		if strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") {
			return fmt.Errorf("source path component ends with space/dot: %q", rel)
		}
	}
	return nil
}
