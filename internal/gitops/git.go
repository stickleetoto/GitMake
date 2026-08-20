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
