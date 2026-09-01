package securityscan

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gitmake/internal/pathmatch"
)

type Finding struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}

type LargeFile struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	LFSMarked bool   `json:"lfs_marked"`
	Blocking  bool   `json:"blocking"`
}

type Report struct {
	Schema       string      `json:"schema"`
	SecretScan   bool        `json:"secret_scan"`
	Findings     []Finding   `json:"findings,omitempty"`
	LargeFiles   []LargeFile `json:"large_files,omitempty"`
	Warnings     []string    `json:"warnings,omitempty"`
	LFSRequired  bool        `json:"lfs_required"`
	Blocking     bool        `json:"blocking"`
	ScannedFiles int         `json:"scanned_files"`
}

type Options struct {
	SecretScan       bool
	AllowSecretPaths []string
	WarnFileBytes    int64
	MaxGitFileBytes  int64
}

var contentRules = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"private_key", regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`)},
	{"github_token", regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`)},
	{"github_fine_grained_token", regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`)},
	{"aws_access_key", regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{"slack_token", regexp.MustCompile(`\bxox(?:b|p|a|r|s)-[A-Za-z0-9-]{10,}\b`)},
}

func Scan(root string, o Options) (Report, error) {
	report := Report{Schema: "gitmake.security/v1", SecretScan: o.SecretScan}
	lfsPatterns, err := readLFSPatterns(root)
	if err != nil {
		return report, err
	}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == root || d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("security scan does not allow symlinks: %s", path)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		report.ScannedFiles++

		lfs := lfsMatches(lfsPatterns, rel)
		if o.WarnFileBytes > 0 && info.Size() >= o.WarnFileBytes {
			lf := LargeFile{Path: rel, Bytes: info.Size(), LFSMarked: lfs}
			if o.MaxGitFileBytes > 0 && info.Size() >= o.MaxGitFileBytes && !lfs {
				lf.Blocking = true
				report.Blocking = true
			}
			if lfs {
				report.LFSRequired = true
			}
			report.LargeFiles = append(report.LargeFiles, lf)
		}

		if !o.SecretScan || pathmatch.Any(o.AllowSecretPaths, rel) {
			return nil
		}
		if kind, detail := suspiciousPath(rel); kind != "" {
			report.Findings = append(report.Findings, Finding{Path: rel, Kind: kind, Detail: detail})
			report.Blocking = true
		}
		// Limit content scanning to 2 MiB per file and skip obvious binary data.
		if info.Size() > 2*1024*1024 {
			return nil
		}
		data, err := readPrefix(path, 2*1024*1024)
		if err != nil {
			return err
		}
		if bytes.IndexByte(data, 0) >= 0 {
			return nil
		}
		for _, rule := range contentRules {
			if rule.re.Match(data) {
				report.Findings = append(report.Findings, Finding{Path: rel, Kind: rule.kind, Detail: "high-confidence secret pattern detected"})
				report.Blocking = true
			}
		}
		return nil
	})
	sort.Slice(report.Findings, func(i, j int) bool {
		if report.Findings[i].Path == report.Findings[j].Path {
			return report.Findings[i].Kind < report.Findings[j].Kind
		}
		return report.Findings[i].Path < report.Findings[j].Path
	})
	sort.Slice(report.LargeFiles, func(i, j int) bool { return report.LargeFiles[i].Path < report.LargeFiles[j].Path })
	return report, err
}

func suspiciousPath(rel string) (string, string) {
	lower := strings.ToLower(filepath.ToSlash(rel))
	base := strings.ToLower(filepath.Base(lower))
	if base == ".env.example" || base == ".env.sample" || base == ".env.template" || strings.HasSuffix(base, ".example") || strings.HasSuffix(base, ".sample") {
		return "", ""
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return "secret_file", "environment secret file"
	}
	switch base {
	case "id_rsa", "id_ed25519", "id_ecdsa", "credentials.json", "service-account.json", "service_account.json":
		return "secret_file", "credential/private-key filename"
	}
	ext := strings.ToLower(filepath.Ext(base))
	switch ext {
	case ".p12", ".pfx":
		return "secret_file", "credential/private-key file extension"
	case ".key":
		if strings.Contains(lower, "test") || strings.Contains(lower, "fixture") || strings.Contains(lower, "example") {
			return "", ""
		}
		return "secret_file", "private-key file extension"
	}
	return "", ""
}

func readPrefix(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

func readLFSPatterns(root string) ([]string, error) {
	path := filepath.Join(root, ".gitattributes")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var patterns []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "filter=lfs") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			patterns = append(patterns, fields[0])
		}
	}
	return patterns, sc.Err()
}

func lfsMatches(patterns []string, rel string) bool {
	for _, p := range patterns {
		p = strings.ReplaceAll(p, "\\", "/")
		// Git attributes patterns are richer than path.Match. These common
		// cases cover extension/path rules without pretending full parity.
		if strings.HasPrefix(p, "*.") && strings.HasSuffix(strings.ToLower(rel), strings.ToLower(strings.TrimPrefix(p, "*"))) {
			return true
		}
		if pathmatch.Match(p, rel) {
			return true
		}
	}
	return false
}
