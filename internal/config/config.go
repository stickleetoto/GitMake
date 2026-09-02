package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gitmake/internal/gmerr"
)

const CurrentSchemaVersion = 1

const (
	placeholderRepo = "YOUR_REPOSITORY"
	placeholderZIP  = "YOUR_PROJECT.zip"
)

var ErrNoProjectZIP = errors.New("no project ZIP found")

var (
	repoNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	ownerRE    = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	branchRE   = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

type Config struct {
	SchemaVersion int            `json:"schema_version"`
	Repo          RepoConfig     `json:"repo"`
	Source        SourceConfig   `json:"source"`
	Git           GitConfig      `json:"git"`
	Sync          SyncConfig     `json:"sync,omitempty"`
	Security      SecurityConfig `json:"security,omitempty"`
	Release       ReleaseConfig  `json:"release,omitempty"`
}

type RepoConfig struct {
	Owner       string `json:"owner,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Visibility  string `json:"visibility,omitempty"`
}

type SourceConfig struct {
	ZIP       string `json:"zip,omitempty"`
	Folder    string `json:"folder,omitempty"`
	StripRoot *bool  `json:"strip_root,omitempty"`
}

type GitConfig struct {
	Branch               string `json:"branch,omitempty"`
	InitialCommitMessage string `json:"initial_commit_message,omitempty"`
	CommitMessage        string `json:"commit_message,omitempty"`
}

type SyncConfig struct {
	Mode           string   `json:"mode,omitempty"`
	ProtectedPaths []string `json:"protected_paths,omitempty"`
}

type SecurityConfig struct {
	SecretScan       *bool    `json:"secret_scan,omitempty"`
	AllowSecretPaths []string `json:"allow_secret_paths,omitempty"`
	WarnFileBytes    int64    `json:"warn_file_bytes,omitempty"`
	MaxGitFileBytes  int64    `json:"max_git_file_bytes,omitempty"`
}

type ReleaseConfig struct {
	Enabled       bool     `json:"enabled,omitempty"`
	Tag           string   `json:"tag,omitempty"`
	Title         string   `json:"title,omitempty"`
	Notes         string   `json:"notes,omitempty"`
	NotesFile     string   `json:"notes_file,omitempty"`
	GenerateNotes *bool    `json:"generate_notes,omitempty"`
	Assets        []string `json:"assets,omitempty"`
	Draft         bool     `json:"draft,omitempty"`
	Prerelease    bool     `json:"prerelease,omitempty"`
	Latest        *bool    `json:"latest,omitempty"`
	OnExisting    string   `json:"on_existing,omitempty"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, gmerr.Wrap(gmerr.ConfigInvalid, err, "open config")
	}
	if len(data) >= 2 && ((data[0] == 0xFF && data[1] == 0xFE) || (data[0] == 0xFE && data[1] == 0xFF)) {
		return Config{}, gmerr.New(gmerr.ConfigInvalid, "parse config: gitmake.json must be UTF-8, not UTF-16")
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()

	var c Config
	if err := dec.Decode(&c); err != nil {
		return Config{}, gmerr.Wrap(gmerr.ConfigInvalid, err, "parse config")
	}
	var trailing any
	if err := dec.Decode(&trailing); err != io.EOF {
		if err == nil {
			return Config{}, gmerr.New(gmerr.ConfigInvalid, "parse config: multiple JSON values are not allowed")
		}
		return Config{}, gmerr.Wrap(gmerr.ConfigInvalid, err, "parse config trailing content")
	}

	applyDefaults(&c)
	if err := c.Validate(); err != nil {
		return Config{}, gmerr.Wrap(gmerr.ConfigInvalid, err, "config validation")
	}
	return c, nil
}

func applyDefaults(c *Config) {
	if c.SchemaVersion == 0 {
		c.SchemaVersion = CurrentSchemaVersion
	}
	if c.Repo.Visibility == "" {
		c.Repo.Visibility = "private"
	}
	c.Repo.Visibility = strings.ToLower(c.Repo.Visibility)
	if c.Git.Branch == "" {
		c.Git.Branch = "main"
	}
	if c.Git.InitialCommitMessage == "" {
		c.Git.InitialCommitMessage = "Initial commit"
	}
	if c.Git.CommitMessage == "" {
		c.Git.CommitMessage = "Update repository"
	}
	if c.Sync.Mode == "" {
		c.Sync.Mode = "managed"
	}
	c.Sync.Mode = strings.ToLower(c.Sync.Mode)
	if len(c.Sync.ProtectedPaths) == 0 {
		c.Sync.ProtectedPaths = []string{".github/**", ".gitmake/**"}
	}
	if c.Security.SecretScan == nil {
		v := true
		c.Security.SecretScan = &v
	}
	if c.Security.WarnFileBytes == 0 {
		c.Security.WarnFileBytes = 50 * 1024 * 1024
	}
	if c.Security.MaxGitFileBytes == 0 {
		c.Security.MaxGitFileBytes = 95 * 1024 * 1024
	}
	if c.Source.ZIP != "" && c.Source.StripRoot == nil {
		v := true
		c.Source.StripRoot = &v
	}
	if c.Release.Enabled {
		if c.Release.GenerateNotes == nil {
			v := c.Release.Notes == "" && c.Release.NotesFile == ""
			c.Release.GenerateNotes = &v
		}
		if c.Release.OnExisting == "" {
			c.Release.OnExisting = "error"
		}
		c.Release.OnExisting = strings.ToLower(c.Release.OnExisting)
	}
}

// Validate reports configuration problems as CONFIG_INVALID. Tagging them
// here means the schema rules below stay plain error text: the machine code no
// longer depends on any of their wording.
func (c Config) Validate() error {
	return gmerr.Wrap(gmerr.ConfigInvalid, c.validate(), "")
}

func (c Config) validate() error {
	if c.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schema_version %d (supported: %d)", c.SchemaVersion, CurrentSchemaVersion)
	}
	if c.Repo.Name == "" {
		return fmt.Errorf("repo.name is required")
	}
	if !repoNameRE.MatchString(c.Repo.Name) || c.Repo.Name == "." || c.Repo.Name == ".." {
		return fmt.Errorf("repo.name contains unsupported characters: %q", c.Repo.Name)
	}
	if c.Repo.Owner != "" {
		if !ownerRE.MatchString(c.Repo.Owner) || strings.HasPrefix(c.Repo.Owner, "-") || strings.HasSuffix(c.Repo.Owner, "-") {
			return fmt.Errorf("repo.owner contains unsupported characters: %q", c.Repo.Owner)
		}
	}
	switch c.Repo.Visibility {
	case "private", "public", "internal":
	default:
		return fmt.Errorf("repo.visibility must be private, public, or internal")
	}
	zipSet := strings.TrimSpace(c.Source.ZIP) != ""
	folderSet := strings.TrimSpace(c.Source.Folder) != ""
	if zipSet == folderSet {
		return fmt.Errorf("source must set exactly one of source.zip or source.folder")
	}
	if folderSet && c.Source.StripRoot != nil {
		return fmt.Errorf("source.strip_root is only valid with source.zip")
	}
	if c.Git.Branch == "" || !branchRE.MatchString(c.Git.Branch) || strings.Contains(c.Git.Branch, "..") || strings.Contains(c.Git.Branch, "//") || strings.HasPrefix(c.Git.Branch, "/") || strings.HasSuffix(c.Git.Branch, "/") || strings.HasSuffix(c.Git.Branch, ".") {
		return fmt.Errorf("git.branch is invalid: %q", c.Git.Branch)
	}
	for _, part := range strings.Split(c.Git.Branch, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(strings.ToLower(part), ".lock") {
			return fmt.Errorf("git.branch is invalid: %q", c.Git.Branch)
		}
	}
	if strings.TrimSpace(c.Git.InitialCommitMessage) == "" {
		return fmt.Errorf("git.initial_commit_message must not be blank")
	}
	if strings.TrimSpace(c.Git.CommitMessage) == "" {
		return fmt.Errorf("git.commit_message must not be blank")
	}
	switch c.Sync.Mode {
	case "managed", "snapshot":
	default:
		return fmt.Errorf("sync.mode must be managed or snapshot")
	}
	for _, pattern := range c.Sync.ProtectedPaths {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("sync.protected_paths must not contain blank patterns")
		}
		if filepath.IsAbs(pattern) || strings.Contains(strings.ReplaceAll(pattern, "\\", "/"), "../") {
			return fmt.Errorf("sync.protected_paths must contain repository-relative patterns: %q", pattern)
		}
	}
	if c.Security.WarnFileBytes < 0 || c.Security.MaxGitFileBytes < 0 {
		return fmt.Errorf("security file size thresholds must be non-negative")
	}
	if c.Security.MaxGitFileBytes > 0 && c.Security.WarnFileBytes > c.Security.MaxGitFileBytes {
		return fmt.Errorf("security.warn_file_bytes must not exceed security.max_git_file_bytes")
	}
	for _, pattern := range c.Security.AllowSecretPaths {
		if strings.TrimSpace(pattern) == "" {
			return fmt.Errorf("security.allow_secret_paths must not contain blank patterns")
		}
	}
	if c.Release.Enabled {
		if strings.TrimSpace(c.Release.Tag) == "" {
			return fmt.Errorf("release.tag is required when release.enabled is true")
		}
		if strings.ContainsAny(c.Release.Tag, "\r\n\t") {
			return fmt.Errorf("release.tag contains control whitespace")
		}
		if c.Release.Notes != "" && c.Release.NotesFile != "" {
			return fmt.Errorf("release.notes and release.notes_file cannot both be set")
		}
		if c.Release.GenerateNotes != nil && !*c.Release.GenerateNotes && c.Release.Notes == "" && c.Release.NotesFile == "" {
			return fmt.Errorf("release requires notes, notes_file, or generate_notes=true to stay non-interactive")
		}
		switch c.Release.OnExisting {
		case "error", "skip", "resume":
		default:
			return fmt.Errorf("release.on_existing must be error, skip, or resume")
		}
		for _, asset := range c.Release.Assets {
			if strings.TrimSpace(asset) == "" {
				return fmt.Errorf("release.assets must not contain blank paths")
			}
		}
	}
	return nil
}

func Save(path string, c Config) error {
	applyDefaults(&c)
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("build config: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// EnsureStarter creates gitmake.json when it is missing. If exactly one ZIP is
// present, that ZIP is selected immediately. If there is no unambiguous ZIP,
// placeholders are written; ResolveProjectZIP can repair those placeholders on
// a later run when a single project ZIP appears.
func EnsureStarter(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("check config: %w", err)
	}

	dir := filepath.Dir(path)
	zips, err := DiscoverZIPs(dir)
	if err != nil {
		return false, err
	}

	zipName := placeholderZIP
	repoName := placeholderRepo
	if len(zips) == 1 {
		zipName = zips[0]
		repoName = deriveRepoName(zipName)
	}

	strip := true
	starter := Config{
		SchemaVersion: CurrentSchemaVersion,
		Repo: RepoConfig{
			Name:       repoName,
			Visibility: "private",
		},
		Source: SourceConfig{ZIP: zipName, StripRoot: &strip},
		Git: GitConfig{
			Branch:               "main",
			InitialCommitMessage: "Initial commit",
			CommitMessage:        "Update repository",
		},
	}
	if err := Save(path, starter); err != nil {
		return false, err
	}
	return true, nil
}

// ResolveProjectZIP returns the actual source archive. It also repairs a
// starter config (or a stale single-ZIP path) when exactly one ZIP exists next
// to gitmake.json. This keeps the double-click workflow self-healing instead of
// leaving YOUR_PROJECT.zip behind after the user adds a ZIP later.
func ResolveProjectZIP(configPath string, c *Config) (zipPath string, repaired bool, err error) {
	base := filepath.Dir(configPath)
	configured := c.Source.ZIP
	candidatePath := configured
	if !filepath.IsAbs(candidatePath) {
		candidatePath = filepath.Join(base, candidatePath)
	}

	if info, statErr := os.Stat(candidatePath); statErr == nil {
		if info.IsDir() {
			return "", false, fmt.Errorf("source.zip points to a directory: %s", candidatePath)
		}
		if isPlaceholderRepo(c.Repo.Name) {
			c.Repo.Name = deriveRepoName(filepath.Base(configured))
			if err := Save(configPath, *c); err != nil {
				return "", false, err
			}
			return candidatePath, true, nil
		}
		return candidatePath, false, nil
	} else if !os.IsNotExist(statErr) {
		return "", false, fmt.Errorf("source ZIP: %w", statErr)
	}

	zips, err := DiscoverZIPs(base)
	if err != nil {
		return "", false, err
	}
	if len(zips) == 1 {
		// Only starter/placeholder configs may self-heal to a newly arrived ZIP.
		// Retargeting a real repository config to an unrelated lone archive is
		// destructive: an old gitmake.json beside a new ZIP could otherwise
		// turn an UPDATE into a mass deletion. Require explicit config editing.
		if !isPlaceholderRepo(c.Repo.Name) {
			return "", false, fmt.Errorf("configured source ZIP %q was not found; refusing to retarget repository %s to %q automatically; update source.zip explicitly or run GitMake in the intended project directory", configured, c.Repo.Name, zips[0])
		}
		c.Source.ZIP = zips[0]
		c.Repo.Name = deriveRepoName(zips[0])
		if err := Save(configPath, *c); err != nil {
			return "", false, err
		}
		return filepath.Join(base, zips[0]), true, nil
	}

	if len(zips) == 0 {
		if strings.EqualFold(configured, placeholderZIP) {
			return "", false, fmt.Errorf("%w beside gitmake.json; put one .zip file in %s and run GitMake again", ErrNoProjectZIP, base)
		}
		return "", false, fmt.Errorf("configured source ZIP %q was not found, and there is no other .zip file in %s", configured, base)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "configured source ZIP %q was not found and multiple ZIP files are present; set source.zip in gitmake.json to one of:", configured)
	for _, z := range zips {
		fmt.Fprintf(&b, "\n  - %s", z)
	}
	return "", false, fmt.Errorf("%s", b.String())
}

func DiscoverZIPs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("scan config directory: %w", err)
	}
	var zips []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.EqualFold(filepath.Ext(name), ".zip") {
			zips = append(zips, name)
		}
	}
	sort.Strings(zips)
	return zips, nil
}

func isPlaceholderRepo(name string) bool {
	return name == "" || strings.EqualFold(name, placeholderRepo)
}

func deriveRepoName(zipName string) string {
	name := strings.TrimSuffix(zipName, filepath.Ext(zipName))
	// Source archives are commonly named Project_v1.2.3_Source.zip. Strip the
	// archive-role suffix first, then the version, so configless discovery
	// infers the repository as `Project` rather than `Project_v1.2.3_Source`.
	roleSuffix := regexp.MustCompile(`(?i)(?:[-_. ]+(?:source|src|code))$`)
	name = roleSuffix.ReplaceAllString(name, "")
	versionSuffix := regexp.MustCompile(`(?i)(?:[-_. ]+v?\d+(?:\.\d+){0,3}(?:[-_.][A-Za-z0-9]+)?)$`)
	name = versionSuffix.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)

	var b strings.Builder
	lastDash := false
	for _, r := range name {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if valid {
			b.WriteRune(r)
			lastDash = r == '-'
			continue
		}
		if !lastDash && b.Len() > 0 {
			b.WriteByte('-')
			lastDash = true
		}
	}
	result := strings.Trim(b.String(), "-._")
	if result == "" || result == "." || result == ".." {
		return placeholderRepo
	}
	return result
}

// ConfigForZIP builds a validated default configuration for a selected ZIP
// without writing anything. Interactive setup uses this so cancelling the
// wizard leaves the directory untouched.
func ConfigForZIP(configPath, zipPath string) (Config, error) {
	absZip, err := filepath.Abs(zipPath)
	if err != nil {
		return Config{}, err
	}
	info, err := os.Stat(absZip)
	if err != nil {
		return Config{}, fmt.Errorf("source ZIP: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Config{}, fmt.Errorf("source ZIP must be a regular file: %s", absZip)
	}
	if !strings.EqualFold(filepath.Ext(absZip), ".zip") {
		return Config{}, fmt.Errorf("source must be a .zip file: %s", absZip)
	}
	base := filepath.Dir(configPath)
	rel, err := filepath.Rel(base, absZip)
	if err != nil {
		rel = absZip
	}
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = absZip
	}
	strip := true
	cfg := Config{
		SchemaVersion: CurrentSchemaVersion,
		Repo:          RepoConfig{Name: deriveRepoName(filepath.Base(absZip)), Visibility: "private"},
		Source:        SourceConfig{ZIP: rel, StripRoot: &strip},
		Git:           GitConfig{Branch: "main", InitialCommitMessage: "Initial commit", CommitMessage: "Update repository"},
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// CreateForZIP creates or replaces a GitMake configuration for an explicitly
// selected ZIP. Existing configs are only overwritten when overwrite is true.
func CreateForZIP(configPath, zipPath string, overwrite bool) (Config, error) {
	if !overwrite {
		if _, err := os.Stat(configPath); err == nil {
			return Config{}, fmt.Errorf("configuration already exists: %s", configPath)
		} else if !os.IsNotExist(err) {
			return Config{}, err
		}
	}
	cfg, err := ConfigForZIP(configPath, zipPath)
	if err != nil {
		return Config{}, err
	}
	if err := Save(configPath, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ConfigForFolder builds a validated default configuration for a source folder
// without writing anything. The folder may be the project directory itself.
func ConfigForFolder(configPath, folderPath string) (Config, error) {
	absFolder, err := filepath.Abs(folderPath)
	if err != nil {
		return Config{}, err
	}
	info, err := os.Stat(absFolder)
	if err != nil {
		return Config{}, fmt.Errorf("source folder: %w", err)
	}
	if !info.IsDir() {
		return Config{}, fmt.Errorf("source folder must be a directory: %s", absFolder)
	}
	base := filepath.Dir(configPath)
	rel, err := filepath.Rel(base, absFolder)
	if err != nil {
		rel = absFolder
	}
	if rel == "." {
		rel = "."
	} else if strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		rel = absFolder
	}
	cfg := Config{
		SchemaVersion: CurrentSchemaVersion,
		Repo:          RepoConfig{Name: deriveRepoName(filepath.Base(absFolder)), Visibility: "private"},
		Source:        SourceConfig{Folder: rel},
		Git:           GitConfig{Branch: "main", InitialCommitMessage: "Initial commit", CommitMessage: "Update repository"},
	}
	applyDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ConfigForSource accepts either a .zip file or a directory and creates the
// corresponding source configuration in memory.
func ConfigForSource(configPath, sourcePath string) (Config, error) {
	abs, err := filepath.Abs(sourcePath)
	if err != nil {
		return Config{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return Config{}, fmt.Errorf("source: %w", err)
	}
	if info.IsDir() {
		return ConfigForFolder(configPath, abs)
	}
	return ConfigForZIP(configPath, abs)
}

// CreateForSource persists a source config for either a ZIP or a folder.
func CreateForSource(configPath, sourcePath string, overwrite bool) (Config, error) {
	if !overwrite {
		if _, err := os.Stat(configPath); err == nil {
			return Config{}, fmt.Errorf("configuration already exists: %s", configPath)
		} else if !os.IsNotExist(err) {
			return Config{}, err
		}
	}
	cfg, err := ConfigForSource(configPath, sourcePath)
	if err != nil {
		return Config{}, err
	}
	if err := Save(configPath, cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ResolveProjectFolder(configPath string, c Config) (string, error) {
	base := filepath.Dir(configPath)
	candidate := c.Source.Folder
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	candidate = filepath.Clean(candidate)
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("configured source folder %q was not found", c.Source.Folder)
		}
		return "", fmt.Errorf("source folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("source.folder does not point to a directory: %s", candidate)
	}
	return candidate, nil
}

// ResolveProjectZIPReadOnly resolves source.zip without repairing or rewriting
// gitmake.json. It is used by agent/read-only previews so inspection never
// mutates the user's project configuration.
func ResolveProjectZIPReadOnly(configPath string, c Config) (string, error) {
	base := filepath.Dir(configPath)
	candidate := c.Source.ZIP
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	candidate = filepath.Clean(candidate)
	info, err := os.Stat(candidate)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("configured source ZIP %q was not found; read-only mode will not repair gitmake.json", c.Source.ZIP)
		}
		return "", fmt.Errorf("source ZIP: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("source.zip points to a directory: %s", candidate)
	}
	return candidate, nil
}
