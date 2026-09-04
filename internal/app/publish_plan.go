package app

import (
	"fmt"
	"strings"

	"gitmake/internal/config"
	"gitmake/internal/github"
	"gitmake/internal/gitops"
	"gitmake/internal/oplock"
	"gitmake/internal/runner"
)

// publishTarget is what PLAN decides: which repository is being published to,
// whether it already exists, what release accompanies it, and the clients that
// will carry out the work.
type publishTarget struct {
	Git    gitops.Client
	GitHub github.Client

	Owner  string
	Target string

	RepoInfo github.RepoInfo
	// Exists selects CREATE or UPDATE, and every later decision follows from it.
	Exists bool

	Release releasePlan
}

// planPublishTarget resolves the publishing target and reserves it.
//
// cfg is taken by pointer because this stage can adopt the remote's visibility:
// when the target was recovered from project memory, GitMake must not silently
// flip a repository's visibility to an inferred default.
//
// The returned release must be deferred by the caller. It holds the repository
// lock that serialises concurrent mutations, and that lock has to be held for
// the whole publish rather than for this function.
func planPublishTarget(o Options, in publishInput, cfg *config.Config) (publishTarget, func(), error) {
	var plan publishTarget
	noRelease := func() {}

	run := runner.Runner{Verbose: o.Verbose}
	plan.Git = gitops.Client{Run: run}
	plan.GitHub = github.Client{Run: run}
	if err := plan.Git.Preflight(); err != nil {
		return plan, noRelease, err
	}
	if err := plan.GitHub.Preflight(); err != nil {
		return plan, noRelease, err
	}

	owner := cfg.Repo.Owner
	if owner == "" {
		resolved, err := plan.GitHub.CurrentUser()
		if err != nil {
			return plan, noRelease, err
		}
		owner = resolved
	}
	plan.Owner = owner
	plan.Target = owner + "/" + cfg.Repo.Name

	// Project memory binds a folder to a repository. Publishing that folder
	// somewhere else is a safety stop, not something to reconcile silently.
	if err := validateFolderProjectMemory(in.Source, plan.Target); err != nil {
		return plan, noRelease, err
	}

	repoInfo, exists, err := plan.GitHub.Repo(owner, cfg.Repo.Name)
	if err != nil {
		return plan, noRelease, err
	}
	plan.RepoInfo, plan.Exists = repoInfo, exists

	if in.MemoryUsed && exists && strings.TrimSpace(repoInfo.Visibility) != "" {
		cfg.Repo.Visibility = strings.ToLower(strings.TrimSpace(repoInfo.Visibility))
	}
	if o.State != nil {
		o.State.Visibility = cfg.Repo.Visibility
		o.State.Repository = plan.Target
		if exists {
			o.State.Mode = "UPDATE"
			o.State.RemoteVisibility = strings.ToLower(strings.TrimSpace(repoInfo.Visibility))
		} else {
			o.State.Mode = "CREATE"
		}
	}

	if exists && o.CreateOnly {
		return plan, noRelease, fmt.Errorf("repository %s already exists (--create-only)", plan.Target)
	}
	if !exists && o.UpdateOnly {
		return plan, noRelease, fmt.Errorf("repository %s does not exist (--update-only)", plan.Target)
	}

	// A dry run mutates nothing, so it takes no lock and blocks no one.
	release := noRelease
	if !o.DryRun {
		repoLock, lockErr := oplock.Acquire("repo:" + strings.ToLower(plan.Target))
		if lockErr != nil {
			return plan, noRelease, lockErr
		}
		release = func() { repoLock.Release() }
	}

	plan.Release, err = prepareReleasePlan(in.ConfigPath, plan.Target, exists, *cfg, o.NoRelease, plan.Git, plan.GitHub)
	if err != nil {
		return plan, release, err
	}
	if o.State != nil {
		releaseState, stateErr := releaseStateFromPlan(plan.Release)
		if stateErr != nil {
			return plan, release, stateErr
		}
		o.State.Release = releaseState
	}
	return plan, release, nil
}

// printPlanHeader shows the user what is about to happen, and why GitMake made
// the choices it did. The explanations are not decoration: a zero-config
// publish infers its target and visibility, and the user is entitled to see
// where those came from before confirming.
func printPlanHeader(in publishInput, cfg config.Config, plan publishTarget) {
	fmt.Printf("GitMake %s\n\n", Version)
	fmt.Printf("  %s\n", cfg.Repo.Name)
	fmt.Printf("  %s · %s\n\n", plan.Target, cfg.Repo.Visibility)

	remoteVisibility := strings.TrimSpace(plan.RepoInfo.Visibility)
	if plan.Exists && remoteVisibility != "" && !strings.EqualFold(remoteVisibility, cfg.Repo.Visibility) {
		fmt.Printf("! Visibility mismatch   config %s · remote %s (remote unchanged)\n", cfg.Repo.Visibility, strings.ToLower(remoteVisibility))
	}
	if in.Source.Repaired {
		fmt.Printf("✓ Source selected       %s\n", sourceDisplay(in.Source))
	}
	switch in.ConfigSource {
	case "stdin":
		fmt.Printf("✓ Config               stdin · validated ephemeral config\n")
	case "inferred":
		fmt.Printf("· Config               inferred in memory · %s mode\n", in.Source.Mode)
	case "project_memory":
		fmt.Printf("✓ Project memory        %s\n", plan.Target)
		fmt.Printf("· Config               inferred in memory · %s mode\n", in.Source.Mode)
	}
	fmt.Printf("· Source mode           %s\n", in.Source.Mode)
	fmt.Printf("· Source path           %s\n", in.Source.Path)
}
