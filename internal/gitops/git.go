package gitops

import (
	"fmt"
	"strings"

	"gitmake/internal/runner"
)

type Client struct{ Run runner.Runner }

func (c Client) Preflight() error {
	res, err := c.Run.Run("", "git", "--version")
	if err != nil {
		return fmt.Errorf("Git not found: %w", err)
	}
	if res.Code != 0 {
		return fmt.Errorf("Git check failed: %s", msg(res))
	}
	return nil
}

func (c Client) HasLFS() bool {
	res, err := c.Run.Run("", "git", "lfs", "version")
	return err == nil && res.Code == 0
}

func (c Client) Init(dir, branch string) error {
	if res, err := c.Run.Run(dir, "git", "init"); err != nil {
		return err
	} else if res.Code != 0 {
		return fmt.Errorf("git init: %s", msg(res))
	}
	if res, err := c.Run.Run(dir, "git", "branch", "-M", branch); err != nil {
		return err
	} else if res.Code != 0 {
		return fmt.Errorf("set branch %s: %s", branch, msg(res))
	}
	return nil
}

func (c Client) Checkout(dir, branch string) error {
	res, err := c.Run.Run(dir, "git", "checkout", branch)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("checkout branch %s: %s", branch, msg(res))
	}
	return nil
}

// PrepareUpdateBranch makes a freshly cloned repository ready for the snapshot
// update. It handles empty GitHub repositories (no HEAD yet) and, for the
// common legacy case where the generated config says main but the repository's
// actual default branch is master or another name, falls back to that default
// branch instead of failing mysteriously.
func (c Client) PrepareUpdateBranch(dir, requested, defaultBranch string) (branch string, fallback bool, err error) {
	hasHead, err := c.HasHead(dir)
	if err != nil {
		return "", false, err
	}
	if !hasHead {
		branch = requested
		if branch == "" {
			branch = defaultBranch
		}
		if branch == "" {
			branch = "main"
		}
		res, runErr := c.Run.Run(dir, "git", "branch", "-M", branch)
		if runErr != nil {
			return "", false, runErr
		}
		if res.Code != 0 {
			return "", false, fmt.Errorf("prepare empty repository branch %s: %s", branch, msg(res))
		}
		return branch, false, nil
	}

	if err := c.Checkout(dir, requested); err == nil {
		return requested, false, nil
	}

	// v0.1 starter configs always wrote "main". Existing repositories may use
	// a different default branch; use it only for this compatibility case.
	if requested == "main" && defaultBranch != "" && defaultBranch != requested {
		if err := c.Checkout(dir, defaultBranch); err == nil {
			return defaultBranch, true, nil
		}
	}

	if defaultBranch != "" && defaultBranch != requested {
		return "", false, fmt.Errorf("configured branch %q was not found; repository default branch is %q (set git.branch in gitmake.json)", requested, defaultBranch)
	}
	return "", false, fmt.Errorf("configured branch %q was not found in the repository", requested)
}

func (c Client) HasHead(dir string) (bool, error) {
	res, err := c.Run.Run(dir, "git", "rev-parse", "--verify", "HEAD")
	if err != nil {
		return false, err
	}
	if res.Code == 0 {
		return true, nil
	}
	// 128 is the normal "no HEAD yet" result in an empty repository. Some Git
	// versions return 1 for equivalent rev-parse failures, so accept both.
	if res.Code == 1 || res.Code == 128 {
		return false, nil
	}
	return false, fmt.Errorf("check repository HEAD: %s", msg(res))
}

func (c Client) HeadSHA(dir string) (string, error) {
	has, err := c.HasHead(dir)
	if err != nil {
		return "", err
	}
	if !has {
		return "", nil
	}
	res, err := c.Run.Run(dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("resolve repository HEAD: %s", msg(res))
	}
	return strings.TrimSpace(res.Stdout), nil
}

func (c Client) AddAll(dir string) error {
	res, err := c.Run.Run(dir, "git", "add", "-A")
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("git add -A: %s", msg(res))
	}
	return nil
}

func (c Client) HasStagedChanges(dir string) (bool, error) {
	res, err := c.Run.Run(dir, "git", "diff", "--cached", "--quiet")
	if err != nil {
		return false, err
	}
	switch res.Code {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, fmt.Errorf("git diff --cached --quiet: %s", msg(res))
	}
}

func (c Client) NameStatus(dir string) (string, error) {
	res, err := c.Run.Run(dir, "git", "diff", "--cached", "--name-status")
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("git diff --cached --name-status: %s", msg(res))
	}
	return res.Stdout, nil
}

func (c Client) EnsureIdentity(dir string) error {
	for _, key := range []string{"user.name", "user.email"} {
		res, err := c.Run.Run(dir, "git", "config", "--get", key)
		if err != nil {
			return err
		}
		if res.Code != 0 || strings.TrimSpace(res.Stdout) == "" {
			return fmt.Errorf("Git %s is not configured; set it with 'git config --global %s <value>'", key, key)
		}
	}
	return nil
}

func (c Client) Commit(dir, message string) error {
	res, err := c.Run.Run(dir, "git", "commit", "-m", message)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("git commit: %s", msg(res))
	}
	return nil
}

func (c Client) Push(dir, branch string) error {
	res, err := c.Run.Run(dir, "git", "push", "origin", branch)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("git push origin %s: %s", branch, msg(res))
	}
	return nil
}

func (c Client) ValidateTag(tag string) error {
	res, err := c.Run.Run("", "git", "check-ref-format", "refs/tags/"+tag)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		return fmt.Errorf("release.tag is not a valid Git tag name: %q", tag)
	}
	return nil
}

func msg(res runner.Result) string {
	if res.Stderr != "" {
		return res.Stderr
	}
	if res.Stdout != "" {
		return res.Stdout
	}
	return fmt.Sprintf("exit code %d", res.Code)
}

// RemoteHeadSHA reports what the remote branch actually points at.
//
// A push that returns success is not by itself proof that the remote moved:
// the answer GitMake prints should come from asking the remote, not from
// assuming the command that just ran did what it said. It is also what `undo`
// checks before reverting, so a revert cannot land on top of somebody else's
// later push.
//
// An empty string means the branch does not exist on the remote.
func (c Client) RemoteHeadSHA(dir, branch string) (string, error) {
	res, err := c.Run.Run(dir, "git", "ls-remote", "origin", "refs/heads/"+branch)
	if err != nil {
		return "", err
	}
	if res.Code != 0 {
		return "", fmt.Errorf("read remote branch %s: %s", branch, msg(res))
	}
	line := strings.TrimSpace(res.Stdout)
	if line == "" {
		return "", nil
	}
	// "<sha>\trefs/heads/<branch>", one line per matching ref.
	sha, _, ok := strings.Cut(strings.TrimSpace(strings.Split(line, "\n")[0]), "\t")
	if !ok || strings.TrimSpace(sha) == "" {
		return "", fmt.Errorf("read remote branch %s: unexpected ls-remote output %q", branch, line)
	}
	return strings.TrimSpace(sha), nil
}

// Revert creates a commit that undoes commit, without rewriting history.
//
// This is the only shape of undo GitMake performs. Resetting or force-pushing
// would remove the published commit from the branch, which the safety contract
// forbids, and which would not actually unpublish anything: the objects stay
// reachable by SHA regardless.
func (c Client) Revert(dir, commit, message string) error {
	// --no-commit so the commit message stays GitMake's to write, and so an
	// empty revert can be reported as such instead of failing inside git.
	res, err := c.Run.Run(dir, "git", "revert", "--no-commit", commit)
	if err != nil {
		return err
	}
	if res.Code != 0 {
		// --abort, not --quit: --quit forgets the in-progress revert but leaves
		// the conflicted index and working tree behind, so the clone would stay
		// dirty and every later operation in it would start from that mess.
		_, _ = c.Run.Run(dir, "git", "revert", "--abort")
		return fmt.Errorf("git revert %s: %s", commit, msg(res))
	}
	staged, err := c.HasStagedChanges(dir)
	if err != nil {
		return err
	}
	if !staged {
		_, _ = c.Run.Run(dir, "git", "revert", "--abort")
		return fmt.Errorf("commit %s is already undone; there is nothing to revert", commit)
	}
	return c.Commit(dir, message)
}

// CommitExists reports whether dir holds commit, so undo can say "that commit
// is not in this branch" rather than failing inside git.
func (c Client) CommitExists(dir, commit string) (bool, error) {
	res, err := c.Run.Run(dir, "git", "cat-file", "-e", commit+"^{commit}")
	if err != nil {
		return false, err
	}
	return res.Code == 0, nil
}
