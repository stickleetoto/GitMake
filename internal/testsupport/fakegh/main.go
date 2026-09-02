// Command fakegh is a local stand-in for the GitHub CLI, used by the E2E
// suites so they never touch a real GitHub account.
//
// It exists as a compiled program rather than a shell script for one reason:
// Go's exec.LookPath resolves commands through PATHEXT on Windows and cannot
// find an extensionless file. The shell stubs the suites used to embed were
// therefore invisible to GitMake on Windows, which silently fell through to the
// real, authenticated gh and published live repositories during a test run.
// A real executable is found the same way on every platform.
//
// It also replaces sixteen slightly different copies of the same stub with one
// implementation, so a suite can no longer pass because its private stub
// happened to be lenient.
//
// State lives on disk so assertions can inspect it directly:
//
//	$FAKE_GH_ROOT/<owner>/<repo>.git             bare repository
//	$FAKE_GH_ROOT/<owner>/<repo>.git.visibility  PRIVATE | PUBLIC | INTERNAL
//	<parent of FAKE_GH_ROOT>/releases/<owner>/<repo>/<tag>/assets/...
//	<parent of FAKE_GH_ROOT>/releases/<owner>/<repo>/<tag>/target
//
// Behaviour toggles:
//
//	FAKE_GH_USER         login reported by `api user` (default: testuser)
//	FAKE_GH_AUTH_FAIL=1  `auth` fails, as if `gh auth login` had never run
//	FAKE_GH_REQUIRE_PR=1 branch protection reports required pull requests
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func fail(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	return 1
}

// usage mirrors gh's exit code for a request this fake does not implement, so
// an unhandled command surfaces loudly instead of looking like a 404.
func usage(format string, args ...any) int {
	fmt.Fprintf(os.Stderr, "fake gh: "+format+"\n", args...)
	return 2
}

func run(args []string) int {
	if len(args) == 0 {
		return usage("no arguments")
	}
	if args[0] == "--version" {
		fmt.Println("gh version 2.fake")
		return 0
	}
	if args[0] == "auth" {
		if os.Getenv("FAKE_GH_AUTH_FAIL") == "1" {
			return fail("You are not logged into any GitHub hosts. Run gh auth login to authenticate.")
		}
		if len(args) > 1 && args[1] == "status" {
			fmt.Printf("Logged in to github.com as %s\n", user())
			return 0
		}
		return usage("unsupported auth command: %s", strings.Join(args, " "))
	}

	switch args[0] {
	case "api":
		return api(args[1:])
	case "repo":
		return repo(args[1:])
	case "release":
		return release(args[1:])
	}
	return usage("unsupported command: %s", strings.Join(args, " "))
}

func user() string {
	if v := os.Getenv("FAKE_GH_USER"); v != "" {
		return v
	}
	return "testuser"
}

// root is the directory holding bare repositories. It is required so a suite
// that forgets to set it fails loudly rather than inventing state.
func root() (string, int) {
	v := os.Getenv("FAKE_GH_ROOT")
	if v == "" {
		return "", usage("FAKE_GH_ROOT is not set")
	}
	return v, 0
}

func remotePath(target string) (string, error) {
	base, code := root()
	if code != 0 {
		return "", fmt.Errorf("FAKE_GH_ROOT is not set")
	}
	owner, name, ok := splitTarget(target)
	if !ok {
		return "", fmt.Errorf("invalid target %q", target)
	}
	return filepath.Join(base, owner, name+".git"), nil
}

func releasePath(target, tag string) (string, error) {
	base, code := root()
	if code != 0 {
		return "", fmt.Errorf("FAKE_GH_ROOT is not set")
	}
	owner, name, ok := splitTarget(target)
	if !ok {
		return "", fmt.Errorf("invalid target %q", target)
	}
	return filepath.Join(filepath.Dir(base), "releases", owner, name, tag), nil
}

func splitTarget(target string) (owner, name string, ok bool) {
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// notFound uses wording GitMake's not-found matcher recognises, so a missing
// repository or release is classified as absent rather than as a hard error.
func notFound(what string) int {
	fmt.Fprintf(os.Stderr, "%s (HTTP 404: Not Found)\n", what)
	return 1
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// ---------------------------------------------------------------- gh api

func api(args []string) int {
	if len(args) == 0 {
		return usage("api requires an endpoint")
	}
	endpoint := args[0]

	if endpoint == "user" {
		fmt.Println(user())
		return 0
	}

	// repos/<owner>/<repo>/branches/<branch>/protection
	if strings.HasPrefix(endpoint, "repos/") && strings.Contains(endpoint, "/branches/") && strings.HasSuffix(endpoint, "/protection") {
		if os.Getenv("FAKE_GH_REQUIRE_PR") == "1" {
			fmt.Println(`{"required_pull_request_reviews":{"required_approving_review_count":1}}`)
			return 0
		}
		return notFound("Branch not protected")
	}

	// repos/<owner>/<repo>/git/ref/tags/<tag>
	if strings.HasPrefix(endpoint, "repos/") && strings.Contains(endpoint, "/git/ref/tags/") {
		rest := strings.TrimPrefix(endpoint, "repos/")
		idx := strings.Index(rest, "/git/ref/tags/")
		target := rest[:idx]
		tag := rest[idx+len("/git/ref/tags/"):]
		remote, err := remotePath(target)
		if err != nil {
			return usage("%v", err)
		}
		if exists(remote) {
			if _, err := git("", "--git-dir="+remote, "rev-parse", "--verify", "refs/tags/"+tag); err == nil {
				fmt.Printf("{\"ref\":\"refs/tags/%s\"}\n", tag)
				return 0
			}
		}
		return notFound("Tag not found")
	}

	return usage("unsupported api endpoint: %s", endpoint)
}

// --------------------------------------------------------------- gh repo

func repo(args []string) int {
	if len(args) == 0 {
		return usage("repo requires a subcommand")
	}
	switch args[0] {
	case "view":
		return repoView(args[1:])
	case "clone":
		return repoClone(args[1:])
	case "create":
		return repoCreate(args[1:])
	}
	return usage("unsupported repo subcommand: %s", args[0])
}

func repoView(args []string) int {
	if len(args) == 0 {
		return usage("repo view requires a target")
	}
	target := args[0]
	jq := flagValue(args, "--jq")

	remote, err := remotePath(target)
	if err != nil {
		return usage("%v", err)
	}
	if !exists(remote) {
		fmt.Fprintf(os.Stderr, "GraphQL: Could not resolve to a Repository with the name '%s'. (HTTP 404)\n", target)
		return 1
	}

	url := "https://example.test/" + target
	if jq == ".url" {
		fmt.Println(url)
		return 0
	}

	visibility := "PRIVATE"
	if data, err := os.ReadFile(remote + ".visibility"); err == nil {
		if v := strings.TrimSpace(string(data)); v != "" {
			visibility = v
		}
	}

	out := map[string]any{
		"nameWithOwner":    target,
		"url":              url,
		"visibility":       visibility,
		"defaultBranchRef": nil,
	}
	// A bare repository reports a symbolic HEAD even before the first push;
	// only report a default branch once that ref actually resolves, the way
	// GitHub does for a repository that still has no commits.
	if head, err := git("", "--git-dir="+remote, "symbolic-ref", "--short", "HEAD"); err == nil && head != "" {
		if _, err := git("", "--git-dir="+remote, "rev-parse", "--verify", head); err == nil {
			out["defaultBranchRef"] = map[string]string{"name": head}
		}
	}
	return emit(out)
}

func repoClone(args []string) int {
	if len(args) < 2 {
		return usage("repo clone requires a target and destination")
	}
	remote, err := remotePath(args[0])
	if err != nil {
		return usage("%v", err)
	}
	if !exists(remote) {
		return notFound("Repository not found")
	}
	if _, err := git("", "clone", remote, args[1]); err != nil {
		return fail("clone failed: %v", err)
	}
	return 0
}

func repoCreate(args []string) int {
	if len(args) == 0 {
		return usage("repo create requires a target")
	}
	target := args[0]
	source := flagValue(args, "--source")
	visibility := "PRIVATE"
	for _, a := range args {
		switch a {
		case "--public":
			visibility = "PUBLIC"
		case "--internal":
			visibility = "INTERNAL"
		case "--private":
			visibility = "PRIVATE"
		}
	}

	remote, err := remotePath(target)
	if err != nil {
		return usage("%v", err)
	}
	if exists(remote) {
		return fail("GraphQL: Name already exists on this account (createRepository)")
	}
	if err := os.MkdirAll(filepath.Dir(remote), 0o755); err != nil {
		return fail("%v", err)
	}
	if _, err := git("", "init", "--bare", remote); err != nil {
		return fail("init bare repository: %v", err)
	}
	if err := os.WriteFile(remote+".visibility", []byte(visibility+"\n"), 0o644); err != nil {
		return fail("%v", err)
	}

	if source != "" {
		branch, err := git(source, "branch", "--show-current")
		if err != nil {
			return fail("read source branch: %v", err)
		}
		if _, err := git(source, "remote", "add", "origin", remote); err != nil {
			return fail("add origin: %v", err)
		}
		if _, err := git(source, "push", "-u", "origin", branch); err != nil {
			return fail("push: %v", err)
		}
		if _, err := git("", "--git-dir="+remote, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
			return fail("set HEAD: %v", err)
		}
	}
	fmt.Println("https://example.test/" + target)
	return 0
}

// ------------------------------------------------------------ gh release

func release(args []string) int {
	if len(args) == 0 {
		return usage("release requires a subcommand")
	}
	switch args[0] {
	case "view":
		return releaseView(args[1:])
	case "create":
		return releaseCreate(args[1:])
	case "upload":
		return releaseUpload(args[1:])
	case "download":
		return releaseDownload(args[1:])
	}
	return usage("unsupported release subcommand: %s", args[0])
}

func releaseView(args []string) int {
	target := flagValue(args, "--repo")
	if target == "" {
		return usage("release view requires --repo")
	}
	tag := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		tag = args[0]
	}

	// No tag means "the latest release", which GitMake reads as a bare tag name.
	if tag == "" {
		latest, err := latestTag(target)
		if err != nil {
			return usage("%v", err)
		}
		if latest == "" {
			return notFound("Release not found")
		}
		if flagValue(args, "--jq") == ".tagName" {
			fmt.Println(latest)
			return 0
		}
		tag = latest
	}

	dir, err := releasePath(target, tag)
	if err != nil {
		return usage("%v", err)
	}
	if !exists(dir) {
		fmt.Fprintln(os.Stderr, "release not found (HTTP 404)")
		return 1
	}

	url := fmt.Sprintf("https://example.test/%s/releases/tag/%s", target, tag)
	if flagValue(args, "--jq") == ".url" {
		fmt.Println(url)
		return 0
	}

	assets := []map[string]string{}
	entries, _ := os.ReadDir(filepath.Join(dir, "assets"))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names {
		assets = append(assets, map[string]string{"name": n})
	}

	return emit(map[string]any{
		"url":          url,
		"tagName":      tag,
		"isDraft":      false,
		"isPrerelease": false,
		"assets":       assets,
	})
}

// latestTag reports the most recently created release for a repository.
// Ordering is by creation time rather than by name, because a lexical sort
// would rank v1.0.9 above v1.0.10.
func latestTag(target string) (string, error) {
	dir, err := releasePath(target, "")
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}
	type release struct {
		tag  string
		when int64
	}
	found := make([]release, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		found = append(found, release{tag: e.Name(), when: info.ModTime().UnixNano()})
	}
	if len(found) == 0 {
		return "", nil
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].when == found[j].when {
			return found[i].tag < found[j].tag
		}
		return found[i].when < found[j].when
	})
	return found[len(found)-1].tag, nil
}

func releaseCreate(args []string) int {
	if len(args) == 0 {
		return usage("release create requires a tag")
	}
	tag := args[0]
	target := flagValue(args, "--repo")
	if target == "" {
		return usage("release create requires --repo")
	}
	targetBranch := flagValue(args, "--target")
	assets, code := positionalAssets(args[1:])
	if code != 0 {
		return code
	}

	remote, err := remotePath(target)
	if err != nil {
		return usage("%v", err)
	}
	if !exists(remote) {
		return notFound("Repository not found")
	}
	if targetBranch != "" {
		if _, err := git("", "--git-dir="+remote, "rev-parse", "--verify", "refs/heads/"+targetBranch); err != nil {
			return fail("target branch %q does not exist", targetBranch)
		}
	}

	dir, err := releasePath(target, tag)
	if err != nil {
		return usage("%v", err)
	}
	if exists(dir) {
		return fail("a release with the tag %q already exists", tag)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		return fail("%v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target"), []byte(targetBranch+"\n"), 0o644); err != nil {
		return fail("%v", err)
	}
	if code := copyAssets(assets, filepath.Join(dir, "assets")); code != 0 {
		return code
	}
	fmt.Printf("https://example.test/%s/releases/tag/%s\n", target, tag)
	return 0
}

func releaseUpload(args []string) int {
	if len(args) == 0 {
		return usage("release upload requires a tag")
	}
	tag := args[0]
	target := flagValue(args, "--repo")
	if target == "" {
		return usage("release upload requires --repo")
	}
	assets, code := positionalAssets(args[1:])
	if code != 0 {
		return code
	}
	dir, err := releasePath(target, tag)
	if err != nil {
		return usage("%v", err)
	}
	if !exists(dir) {
		return notFound("Release not found")
	}
	return copyAssets(assets, filepath.Join(dir, "assets"))
}

func releaseDownload(args []string) int {
	if len(args) == 0 {
		return usage("release download requires a tag")
	}
	tag := args[0]
	target := flagValue(args, "--repo")
	pattern := flagValue(args, "--pattern")
	destination := flagValue(args, "--dir")
	if target == "" || pattern == "" || destination == "" {
		return usage("release download requires --repo, --pattern and --dir")
	}
	dir, err := releasePath(target, tag)
	if err != nil {
		return usage("%v", err)
	}
	source := filepath.Join(dir, "assets", pattern)
	if !exists(source) {
		return notFound("Asset not found")
	}
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return fail("%v", err)
	}
	return copyAssets([]string{source}, destination)
}

// ---------------------------------------------------------------- helpers

// flagValue returns the value that follows name, supporting both `--flag value`
// and `--flag=value`.
func flagValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"=")
		}
	}
	return ""
}

// valueFlags take an argument, so the token after them is never an asset path.
var valueFlags = map[string]bool{
	"--repo": true, "--target": true, "--title": true,
	"--notes": true, "--notes-file": true, "--pattern": true, "--dir": true,
	"--json": true, "--jq": true, "--source": true, "--description": true,
	"--remote": true,
}

// positionalAssets extracts asset paths from a release command line. An
// unrecognised flag is an error rather than something silently treated as a
// file, so a new gh flag cannot quietly turn into a bogus upload.
func positionalAssets(args []string) ([]string, int) {
	var assets []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case valueFlags[a]:
			i++
		case strings.HasPrefix(a, "--") && strings.Contains(a, "="):
			// --latest=true and friends carry their own value.
		case a == "--generate-notes" || a == "--draft" || a == "--prerelease" || a == "--clobber" || a == "--latest":
		case strings.HasPrefix(a, "-"):
			return nil, usage("unsupported release flag: %s", a)
		default:
			assets = append(assets, a)
		}
	}
	return assets, 0
}

func copyAssets(assets []string, destination string) int {
	for _, asset := range assets {
		data, err := os.ReadFile(asset)
		if err != nil {
			return fail("read asset %s: %v", asset, err)
		}
		if err := os.WriteFile(filepath.Join(destination, filepath.Base(asset)), data, 0o644); err != nil {
			return fail("write asset %s: %v", asset, err)
		}
	}
	return 0
}

func emit(v any) int {
	data, err := json.Marshal(v)
	if err != nil {
		return fail("%v", err)
	}
	fmt.Println(string(data))
	return 0
}
