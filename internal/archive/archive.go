package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	maxEntries          = 100000
	maxUncompressedSize = uint64(8) * 1024 * 1024 * 1024 // 8 GiB
)

type entry struct {
	file  *zip.File
	name  string
	isDir bool
}

func ExtractSafe(zipPath, dest string, stripRoot bool) (int, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return 0, fmt.Errorf("open ZIP: %w", err)
	}
	defer zr.Close()

	if len(zr.File) > maxEntries {
		return 0, fmt.Errorf("ZIP contains too many entries (%d > %d)", len(zr.File), maxEntries)
	}

	entries := make([]entry, 0, len(zr.File))
	seen := make(map[string]bool, len(zr.File)) // Windows-style case-insensitive key -> isDir
	var total uint64
	for _, f := range zr.File {
		name, err := sanitizeName(f.Name)
		if err != nil {
			return 0, err
		}
		if name == "" {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return 0, fmt.Errorf("ZIP symlink is not allowed: %q", f.Name)
		}
		isDir := f.FileInfo().IsDir()
		if !f.FileInfo().Mode().IsRegular() && !isDir {
			return 0, fmt.Errorf("unsupported ZIP entry type: %q", f.Name)
		}
		if f.UncompressedSize64 > maxUncompressedSize-total {
			return 0, fmt.Errorf("ZIP uncompressed size exceeds 8 GiB safety limit")
		}
		total += f.UncompressedSize64

		key := windowsPathKey(name)
		if _, ok := seen[key]; ok {
			return 0, fmt.Errorf("ZIP contains duplicate or case-colliding path: %q", f.Name)
		}
		seen[key] = isDir
		entries = append(entries, entry{file: f, name: name, isDir: isDir})
	}

	if err := validatePathConflicts(seen); err != nil {
		return 0, err
	}

	root := ""
	if stripRoot {
		root = commonTopLevel(entries)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return 0, err
	}
	written := 0
	var actualTotal uint64
	for _, e := range entries {
		name := e.name
		if root != "" {
			if name == root {
				continue
			}
			prefix := root + "/"
			if !strings.HasPrefix(name, prefix) {
				return 0, fmt.Errorf("internal strip_root mismatch for %q", name)
			}
			name = strings.TrimPrefix(name, prefix)
		}
		if name == "" {
			continue
		}

		target := filepath.Join(dest, filepath.FromSlash(name))
		if !within(dest, target) {
			return 0, fmt.Errorf("unsafe ZIP path escapes destination: %q", e.file.Name)
		}

		if e.isDir {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return 0, fmt.Errorf("create directory %q: %w", target, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		rc, err := e.file.Open()
		if err != nil {
			return 0, fmt.Errorf("open ZIP entry %q: %w", e.file.Name, err)
		}
		mode := e.file.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		mode &= 0o777
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return 0, fmt.Errorf("create %q: %w", target, err)
		}
		remaining := maxUncompressedSize - actualTotal
		limited := &io.LimitedReader{R: rc, N: int64(remaining) + 1}
		n, copyErr := io.Copy(out, limited)
		closeErr1 := out.Close()
		closeErr2 := rc.Close()
		if copyErr != nil {
			return 0, fmt.Errorf("extract %q: %w", e.file.Name, copyErr)
		}
		if uint64(n) > remaining {
			return 0, fmt.Errorf("ZIP actual uncompressed data exceeds 8 GiB safety limit")
		}
		actualTotal += uint64(n)
		if uint64(n) != e.file.UncompressedSize64 {
			return 0, fmt.Errorf("ZIP entry size mismatch for %q (declared %d, extracted %d)", e.file.Name, e.file.UncompressedSize64, n)
		}
		if closeErr1 != nil {
			return 0, closeErr1
		}
		if closeErr2 != nil {
			return 0, closeErr2
		}
		written++
	}
	return written, nil
}

func sanitizeName(raw string) (string, error) {
	if !utf8.ValidString(raw) {
		return "", fmt.Errorf("ZIP path is not valid UTF-8: %q", raw)
	}
	s := strings.ReplaceAll(raw, "\\", "/")
	if strings.ContainsRune(s, '\x00') {
		return "", fmt.Errorf("ZIP path contains NUL: %q", raw)
	}
	if strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") {
		return "", fmt.Errorf("absolute ZIP path is not allowed: %q", raw)
	}
	if len(s) >= 3 && ((s[0] >= 'A' && s[0] <= 'Z') || (s[0] >= 'a' && s[0] <= 'z')) && s[1] == ':' && s[2] == '/' {
		return "", fmt.Errorf("Windows absolute ZIP path is not allowed: %q", raw)
	}
	for _, rawPart := range strings.Split(s, "/") {
		if rawPart == ".." {
			return "", fmt.Errorf("ZIP path traversal component '..' is not allowed: %q", raw)
		}
	}
	clean := path.Clean(s)
	if clean == "." {
		return "", nil
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("ZIP path traversal is not allowed: %q", raw)
	}
	parts := strings.Split(clean, "/")
	for _, p := range parts {
		if strings.EqualFold(p, ".git") {
			return "", fmt.Errorf("ZIP contains protected .git path: %q", raw)
		}
		if err := validateWindowsComponent(p); err != nil {
			return "", fmt.Errorf("ZIP path %q is not safe on Windows: %w", raw, err)
		}
	}
	return clean, nil
}

func validateWindowsComponent(component string) error {
	if component == "" {
		return fmt.Errorf("empty path component")
	}
	if strings.HasSuffix(component, " ") || strings.HasSuffix(component, ".") {
		return fmt.Errorf("path component %q ends with a space or dot", component)
	}
	if len(utf16.Encode([]rune(component))) > 255 {
		return fmt.Errorf("path component %q is longer than 255 UTF-16 code units", component)
	}
	for _, r := range component {
		if r < 32 {
			return fmt.Errorf("path component %q contains a control character", component)
		}
		if strings.ContainsRune(`<>:"|?*`, r) {
			return fmt.Errorf("path component %q contains reserved character %q", component, r)
		}
	}
	base := component
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	base = strings.TrimRight(base, " .")
	upper := strings.ToUpper(base)
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" {
		return fmt.Errorf("path component %q uses a reserved Windows device name", component)
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
		return fmt.Errorf("path component %q uses a reserved Windows device name", component)
	}
	return nil
}

func windowsPathKey(name string) string {
	return strings.ToLower(strings.TrimSuffix(name, "/"))
}

func validatePathConflicts(seen map[string]bool) error {
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for i, key := range keys {
		if seen[key] { // directory may contain children
			continue
		}
		if i+1 < len(keys) && strings.HasPrefix(keys[i+1], key+"/") {
			return fmt.Errorf("ZIP path conflict: file %q is also used as a parent directory", key)
		}
	}
	return nil
}

func commonTopLevel(entries []entry) string {
	root := ""
	sawContent := false
	for _, e := range entries {
		if e.isDir && !strings.Contains(strings.TrimSuffix(e.name, "/"), "/") {
			continue
		}
		parts := strings.Split(e.name, "/")
		if len(parts) < 2 {
			return ""
		}
		if !sawContent {
			root = parts[0]
			sawContent = true
		} else if parts[0] != root {
			return ""
		}
	}
	if !sawContent {
		return ""
	}
	return root
}

func within(root, target string) bool {
	r, err1 := filepath.Abs(root)
	t, err2 := filepath.Abs(target)
	if err1 != nil || err2 != nil {
		return false
	}
	rel, err := filepath.Rel(r, t)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && !filepath.IsAbs(rel)
}
