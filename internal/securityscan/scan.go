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
	Path string `json:"path"`
	Kind string `json:"kind"`
	// Confidence is "high" for an issuer-specific credential shape and
	// "medium" for a structural match that deserves a look before being
	// allowed. Both block: secret scanning is fail-closed.
	Confidence string `json:"confidence,omitempty"`
	// Line is the 1-based line of the first match, or 0 for a finding about
	// the path rather than the contents. Remediation needs a location.
	Line   int    `json:"line,omitempty"`
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

// confidence describes how much interpretation a rule needs. Both levels
// block; the label tells a user whether they are looking at an unmistakable
// credential or at a heuristic worth reviewing before allowing the path.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
)

type contentRule struct {
	kind       string
	confidence string
	re         *regexp.Regexp
	// reject discards a match that the pattern cannot exclude on its own.
	// Go's regexp has no lookahead, so overlapping vendor prefixes and obvious
	// documentation placeholders are filtered here instead.
	reject *regexp.Regexp
}

// High-confidence rules match issuer-specific shapes that essentially cannot
// occur by accident. Medium-confidence rules read structure rather than a
// vendor prefix, so they are labelled for review.
var contentRules = []contentRule{
	{kind: "private_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY(?: BLOCK)?-----`)},
	{kind: "github_token", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`)},
	{kind: "github_fine_grained_token", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`)},
	{kind: "aws_access_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`)},
	{kind: "slack_token", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bxox(?:b|p|a|r|s)-[A-Za-z0-9-]{10,}\b`)},
	{kind: "slack_webhook", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`https://hooks\.slack\.com/services/[A-Za-z0-9_/+-]{20,}`)},
	{kind: "discord_webhook", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`https://(?:canary\.|ptb\.)?discord(?:app)?\.com/api/webhooks/[0-9]{10,}/[A-Za-z0-9_-]{20,}`)},

	// Model provider keys are the most common credential in AI-authored
	// projects, which is exactly the workload GitMake publishes.
	{kind: "anthropic_api_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`)},
	{kind: "openai_api_key", confidence: ConfidenceHigh,
		re:     regexp.MustCompile(`\bsk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_-]{32,}`),
		reject: regexp.MustCompile(`^sk-ant-`)},
	{kind: "huggingface_token", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bhf_[A-Za-z0-9]{30,}\b`)},

	{kind: "google_api_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{kind: "google_oauth_client_secret", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bGOCSPX-[A-Za-z0-9_-]{20,}`)},
	{kind: "gcp_service_account", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`"type"\s*:\s*"service_account"`)},

	{kind: "stripe_secret_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\b(?:sk|rk)_live_[0-9A-Za-z]{20,}\b`)},
	{kind: "sendgrid_api_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{20,}\.[A-Za-z0-9_-]{20,}\b`)},
	{kind: "npm_token", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{kind: "azure_storage_key", confidence: ConfidenceHigh,
		re: regexp.MustCompile(`AccountKey\s*=\s*[A-Za-z0-9+/]{60,}={0,2}`)},

	// A URL that embeds a password is a credential whatever the service is.
	// Documentation placeholders are rejected so examples do not block a
	// publish.
	{kind: "connection_string_password", confidence: ConfidenceMedium,
		re:     regexp.MustCompile(`\b[a-zA-Z][a-zA-Z0-9+.-]*://[^\s:@/]{1,64}:[^\s:@/]{3,128}@[^\s/"']{1,255}`),
		reject: regexp.MustCompile(`(?i)://[^\s:@/]{1,64}:(?:\*+|x{3,}|pass(?:word|wd)?|secret|token|changeme|your[_-]?\w*|placeholder|<[^>]*>|\$\{[^}]*\}|%[^%]*%)@`)},
	{kind: "jwt", confidence: ConfidenceMedium,
		re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)},
}

const (
	// contentChunk bounds memory per file; contentOverlap keeps a credential
	// that straddles a chunk boundary detectable.
	contentChunk   = 1 << 20
	contentOverlap = 8 << 10
	// maxContentBytes bounds the work spent on any single file. Files used to
	// be skipped entirely above 2 MiB, so a credential in a large log or dump
	// was never looked at.
	maxContentBytes = 64 << 20
)

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
			report.Findings = append(report.Findings, Finding{
				Path: rel, Kind: kind, Confidence: ConfidenceHigh, Detail: detail,
			})
			report.Blocking = true
		}

		matches, scanErr := scanContent(path)
		if scanErr != nil {
			return scanErr
		}
		for _, m := range matches {
			report.Findings = append(report.Findings, Finding{
				Path:       rel,
				Kind:       m.kind,
				Confidence: m.confidence,
				Line:       m.line,
				Detail:     fmt.Sprintf("%s-confidence secret pattern detected at line %d", m.confidence, m.line),
			})
			report.Blocking = true
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

type contentMatch struct {
	kind       string
	confidence string
	line       int
}

// scanContent streams a file looking for every supported credential shape.
//
// It reports the first location of each kind rather than every occurrence:
// one actionable line per kind per file is what remediation needs, and
// reporting all kinds at once is what stops the fix-one-find-another cycle.
func scanContent(path string) ([]contentMatch, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	remaining := make(map[int]bool, len(contentRules))
	for i := range contentRules {
		remaining[i] = true
	}

	buf := make([]byte, contentChunk+contentOverlap)
	carry := 0
	// linesBefore counts newlines in the bytes preceding the current window.
	linesBefore := 0
	var read int64
	var found []contentMatch
	first := true

	for len(remaining) > 0 && read < maxContentBytes {
		n, readErr := io.ReadFull(f, buf[carry:])
		if n == 0 && readErr != nil {
			break
		}
		window := buf[:carry+n]
		read += int64(n)

		// Binary files hold no reviewable secret text and would only produce
		// noise; the first chunk is enough to recognise one.
		if first && bytes.IndexByte(window, 0) >= 0 {
			return nil, nil
		}
		first = false

		for i := range contentRules {
			if !remaining[i] {
				continue
			}
			rule := contentRules[i]
			loc := findMatch(rule, window)
			if loc < 0 {
				continue
			}
			found = append(found, contentMatch{
				kind:       rule.kind,
				confidence: rule.confidence,
				line:       linesBefore + bytes.Count(window[:loc], []byte{'\n'}) + 1,
			})
			delete(remaining, i)
		}

		if readErr != nil {
			break
		}

		// Retain a tail so a credential spanning the boundary is still seen,
		// and account for the newlines that scroll out of the window.
		keep := contentOverlap
		if keep > len(window) {
			keep = len(window)
		}
		advanced := len(window) - keep
		linesBefore += bytes.Count(window[:advanced], []byte{'\n'})
		copy(buf, window[advanced:])
		carry = keep
	}

	sort.Slice(found, func(i, j int) bool { return found[i].kind < found[j].kind })
	return found, nil
}

// findMatch returns the offset of the first acceptable match, or -1.
func findMatch(rule contentRule, window []byte) int {
	offset := 0
	for {
		loc := rule.re.FindIndex(window[offset:])
		if loc == nil {
			return -1
		}
		start, end := offset+loc[0], offset+loc[1]
		if rule.reject == nil || !rule.reject.Match(window[start:end]) {
			return start
		}
		// A rejected match must not hide a real one later in the window.
		offset = start + 1
		if offset >= len(window) {
			return -1
		}
	}
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
