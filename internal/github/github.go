package github

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gitmake/internal/runner"
)

type Client struct{ Run runner.Runner }

type defaultBranchRef struct {
	Name string `json:"name"`
}

type RepoInfo struct {
	NameWithOwner    string            `json:"nameWithOwner"`
	URL              string            `json:"url"`
	DefaultBranchRef *defaultBranchRef `json:"defaultBranchRef"`
}

func (r RepoInfo) DefaultBranch() string {
	if r.DefaultBranchRef == nil {
		return ""
	}
	return r.DefaultBranchRef.Name
}

func (c Client) Preflight() error {
	if res, err := c.Run.Run("", "gh", "--version"); err != nil {
		return fmt.Errorf("GitHub CLI (gh) not found: %w", err)
	} else if res.Code != 0 {
		return fmt.Errorf("GitHub CLI check failed: %s", message(res))
	}
	res, err := c.Run.Run("", "gh", "auth", "status", "--hostname", "github.com")
	if err != nil {
		return fmt.Errorf("gh auth status: %w", err)
	}
	if res.Code != 0 {
		return fmt.Errorf("GitHub authentication is not ready; run 'gh auth login': %s", message(res))
	}
	return nil
}

func (c Client) CurrentUser() (string, error) {
	res, err := c.Run.Run("", "gh", "api", "user", "--jq", ".login")
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("resolve GitHub user: %s", message(res))
	}
	user := strings.TrimSpace(res.Stdout)
	if user == "" {
		return "", fmt.Errorf("GitHub returned an empty login")
	}
	return user, nil
}

var notFoundRE = regexp.MustCompile(`(?i)(HTTP\s*404|repository\s+not\s+found|could\s+not\s+resolve|\bnot\s+found\b)`)

func (c Client) Repo(owner, name string) (RepoInfo, bool, error) {
	target := owner + "/" + name
	res, err := c.Run.Run("", "gh", "repo", "view", target, "--json", "nameWithOwner,url,defaultBranchRef")
	if err != nil {
		return RepoInfo{}, false, err
	}
	if res.Code != 0 {
		combined := res.Stdout + "\n" + res.Stderr
		if notFoundRE.MatchString(combined) {
			return RepoInfo{}, false, nil
		}
		return RepoInfo{}, false, fmt.Errorf("check repository %s: %s", target, message(res))
	}
	var info RepoInfo
	if err := json.Unmarshal([]byte(res.Stdout), &info); err != nil {
		return RepoInfo{}, false, fmt.Errorf("parse gh repo view output: %w", err)
	}
	return info, true, nil
}

type BranchPolicy struct {
	Known      bool `json:"known"`
	Protected  bool `json:"protected"`
	RequiresPR bool `json:"requires_pull_request"`
}

func (c Client) BranchPolicy(target, branch string) (BranchPolicy, error) {
	endpoint := fmt.Sprintf("repos/%s/branches/%s/protection", target, url.PathEscape(branch))
	res, err := c.Run.Run("", "gh", "api", endpoint)
	if err != nil {
		return BranchPolicy{}, err
	}
	if res.Code != 0 {
		combined := res.Stdout + "\n" + res.Stderr
		if notFoundRE.MatchString(combined) {
			return BranchPolicy{Known: true, Protected: false}, nil
		}
		if strings.Contains(combined, "403") || strings.Contains(strings.ToLower(combined), "forbidden") {
			return BranchPolicy{Known: false}, nil
		}
		return BranchPolicy{}, fmt.Errorf("inspect branch protection for %s:%s: %s", target, branch, message(res))
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(res.Stdout), &raw); err != nil {
		return BranchPolicy{}, fmt.Errorf("parse branch protection: %w", err)
	}
	_, requires := raw["required_pull_request_reviews"]
	return BranchPolicy{Known: true, Protected: true, RequiresPR: requires && raw["required_pull_request_reviews"] != nil}, nil
}

func (c Client) TagExists(target, tag string) (bool, error) {
	endpoint := fmt.Sprintf("repos/%s/git/ref/tags/%s", target, url.PathEscape(tag))
	res, err := c.Run.Run("", "gh", "api", endpoint)
	if err != nil {
		return false, err
	}
	if res.Code == 0 {
		return true, nil
	}
	combined := res.Stdout + "\n" + res.Stderr
	if notFoundRE.MatchString(combined) {
		return false, nil
	}
	return false, fmt.Errorf("check tag %s in %s: %s", tag, target, message(res))
}

func (c Client) Clone(target, dest string) error {
	res, err := c.Run.Run("", "gh", "repo", "clone", target, dest)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("clone %s: %s", target, message(res))
	}
	return nil
}

func (c Client) CreateAndPush(target, visibility, description, source string) (string, error) {
	args := []string{"repo", "create", target, "--source", source, "--remote", "origin", "--push"}
	switch visibility {
	case "private":
		args = append(args, "--private")
	case "public":
		args = append(args, "--public")
	case "internal":
		args = append(args, "--internal")
	}
	if description != "" {
		args = append(args, "--description", description)
	}
	res, err := c.Run.Run("", "gh", args...)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("create GitHub repository: %s", message(res))
	}
	url := strings.TrimSpace(res.Stdout)
	if url == "" {
		view, err := c.Run.Run("", "gh", "repo", "view", target, "--json", "url", "--jq", ".url")
		if err == nil && view.Code == 0 {
			url = strings.TrimSpace(view.Stdout)
		}
	}
	return url, nil
}

func message(res runner.Result) string {
	if res.Stderr != "" {
		return res.Stderr
	}
	if res.Stdout != "" {
		return res.Stdout
	}
	return fmt.Sprintf("exit code %d", res.Code)
}

type ReleaseAsset struct {
	Name string `json:"name"`
}

type ReleaseInfo struct {
	URL          string         `json:"url"`
	TagName      string         `json:"tagName"`
	IsDraft      bool           `json:"isDraft"`
	IsPrerelease bool           `json:"isPrerelease"`
	Assets       []ReleaseAsset `json:"assets"`
}

type ReleaseCreateOptions struct {
	Tag           string
	Target        string
	Title         string
	Notes         string
	NotesFile     string
	GenerateNotes bool
	Assets        []string
	Draft         bool
	Prerelease    bool
	Latest        *bool
}

func (c Client) Release(target, tag string) (ReleaseInfo, bool, error) {
	res, err := c.Run.Run("", "gh", "release", "view", tag, "--repo", target, "--json", "url,tagName,isDraft,isPrerelease,assets")
	if err != nil {
		return ReleaseInfo{}, false, err
	}
	if res.Code != 0 {
		combined := res.Stdout + "\n" + res.Stderr
		if notFoundRE.MatchString(combined) {
			return ReleaseInfo{}, false, nil
		}
		return ReleaseInfo{}, false, fmt.Errorf("check release %s in %s: %s", tag, target, message(res))
	}
	var info ReleaseInfo
	if err := json.Unmarshal([]byte(res.Stdout), &info); err != nil {
		return ReleaseInfo{}, false, fmt.Errorf("parse gh release view output: %w", err)
	}
	return info, true, nil
}

func (c Client) CreateRelease(target string, o ReleaseCreateOptions) (string, error) {
	args := []string{"release", "create", o.Tag}
	args = append(args, o.Assets...)
	args = append(args, "--repo", target)
	if o.Target != "" {
		args = append(args, "--target", o.Target)
	}
	if o.Title != "" {
		args = append(args, "--title", o.Title)
	}
	if o.NotesFile != "" {
		args = append(args, "--notes-file", o.NotesFile)
	} else if o.Notes != "" {
		args = append(args, "--notes", o.Notes)
	}
	if o.GenerateNotes {
		args = append(args, "--generate-notes")
	}
	if o.Draft {
		args = append(args, "--draft")
	}
	if o.Prerelease {
		args = append(args, "--prerelease")
	}
	if o.Latest != nil {
		args = append(args, fmt.Sprintf("--latest=%t", *o.Latest))
	}

	res, err := c.Run.Run("", "gh", args...)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("create GitHub release %s: %s", o.Tag, message(res))
	}
	url := strings.TrimSpace(res.Stdout)
	if url == "" {
		view, exists, err := c.Release(target, o.Tag)
		if err == nil && exists {
			url = view.URL
		}
	}
	return url, nil
}

func (c Client) UploadReleaseAssets(target, tag string, assets []string) error {
	if len(assets) == 0 {
		return nil
	}
	args := []string{"release", "upload", tag}
	args = append(args, assets...)
	args = append(args, "--repo", target)
	res, err := c.Run.Run("", "gh", args...)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("upload release assets for %s: %s", tag, message(res))
	}
	return nil
}

func (c Client) LatestReleaseTag(target string) (string, error) {
	res, err := c.Run.Run("", "gh", "release", "view", "--repo", target, "--json", "tagName", "--jq", ".tagName")
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("check latest GitMake release: %s", message(res))
	}
	tag := strings.TrimSpace(res.Stdout)
	if tag == "" {
		return "", fmt.Errorf("GitHub returned an empty latest release tag")
	}
	return tag, nil
}

func (c Client) DownloadReleaseAsset(target, tag, asset, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	res, err := c.Run.Run("", "gh", "release", "download", tag, "--repo", target, "--pattern", asset, "--dir", dir, "--clobber")
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("download GitMake release asset: %s", message(res))
	}
	path := filepath.Join(dir, asset)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("downloaded asset not found at %s", path)
	}
	return path, nil
}
