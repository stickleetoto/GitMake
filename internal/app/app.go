package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/archive"
	"gitmake/internal/config"
	"gitmake/internal/github"
	"gitmake/internal/gitops"
	"gitmake/internal/runner"
	"gitmake/internal/syncer"
)

const Version = "0.1.3"

type Options struct {
	ConfigPath string
	DryRun     bool
	Verbose    bool
	KeepTemp   bool
	CreateOnly bool
	UpdateOnly bool
}

func Main(args []string) int {
	opts, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		return 2
	}
	if err := Run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		return 1
	}
	return 0
}

func parseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("gitmake", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var o Options
	fs.StringVar(&o.ConfigPath, "config", "gitmake.json", "path to gitmake JSON config")
	fs.BoolVar(&o.DryRun, "dry-run", false, "show changes without creating, committing, or pushing")
	fs.BoolVar(&o.Verbose, "verbose", false, "print external commands")
	fs.BoolVar(&o.KeepTemp, "keep-temp", false, "keep temporary workspace for debugging")
	fs.BoolVar(&o.CreateOnly, "create-only", false, "fail if the GitHub repository already exists")
	fs.BoolVar(&o.UpdateOnly, "update-only", false, "fail if the GitHub repository does not exist")
	version := fs.Bool("version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	if fs.NArg() != 0 {
		return Options{}, fmt.Errorf("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	if o.CreateOnly && o.UpdateOnly {
		return Options{}, errors.New("--create-only and --update-only cannot be used together")
	}
	if *version {
		fmt.Println("gitmake", Version)
		os.Exit(0)
	}
	return o, nil
}

func Run(o Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := o.ConfigPath
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(cwd, configPath)
	}

	fmt.Printf("GitMake v%s\n", Version)
	fmt.Println("[1/7] Loading configuration")
	created, err := config.EnsureStarter(configPath)
	if err != nil {
		return err
	}
	if created {
		fmt.Println("✓ Created starter configuration:", configPath)
	}
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	zipPath, repaired, err := config.ResolveProjectZIP(configPath, &cfg)
	if err != nil {
		if errors.Is(err, config.ErrNoProjectZIP) {
			fmt.Println("! No project ZIP found yet.")
			fmt.Printf("  Put one .zip file beside %s, then run GitMake again.\n", filepath.Base(configPath))
			return nil
		}
		return err
	}
	if repaired {
		fmt.Printf("✓ Auto-selected ZIP: %s\n", filepath.Base(zipPath))
		fmt.Printf("✓ Repository name: %s\n", cfg.Repo.Name)
	}

	run := runner.Runner{Verbose: o.Verbose}
	git := gitops.Client{Run: run}
	gh := github.Client{Run: run}

	fmt.Println("[2/7] Checking Git and GitHub CLI")
	if err := git.Preflight(); err != nil {
		return err
	}
	if err := gh.Preflight(); err != nil {
		return err
	}

	owner := cfg.Repo.Owner
	if owner == "" {
		owner, err = gh.CurrentUser()
		if err != nil {
			return err
		}
	}
	target := owner + "/" + cfg.Repo.Name

	fmt.Printf("[3/7] Checking repository %s\n", target)
	repoInfo, exists, err := gh.Repo(owner, cfg.Repo.Name)
	if err != nil {
		return err
	}
	if exists && o.CreateOnly {
		return fmt.Errorf("repository %s already exists (--create-only)", target)
	}
	if !exists && o.UpdateOnly {
		return fmt.Errorf("repository %s does not exist (--update-only)", target)
	}

	work, err := os.MkdirTemp("", "gitmake-*")
	if err != nil {
		return err
	}
	if o.KeepTemp {
		fmt.Println("Temporary workspace:", work)
	} else {
		defer os.RemoveAll(work)
	}

	snapshot := filepath.Join(work, "snapshot")
	fmt.Println("[4/7] Validating and extracting ZIP")
	files, err := archive.ExtractSafe(zipPath, snapshot, *cfg.Source.StripRoot)
	if err != nil {
		return err
	}
	if files == 0 {
		return fmt.Errorf("source ZIP contains no regular files")
	}

	if exists {
		return updateFlow(o, cfg, target, repoInfo.URL, repoInfo.DefaultBranch(), snapshot, work, git, gh)
	}
	return createFlow(o, cfg, target, snapshot, git, gh)
}

func createFlow(o Options, cfg config.Config, target, snapshot string, git gitops.Client, gh github.Client) error {
	fmt.Println("[5/7] Mode: CREATE")
	if err := git.Init(snapshot, cfg.Git.Branch); err != nil {
		return err
	}
	if err := git.AddAll(snapshot); err != nil {
		return err
	}
	changed, err := git.HasStagedChanges(snapshot)
	if err != nil {
		return err
	}
	if !changed {
		return fmt.Errorf("nothing to commit")
	}
	diff, err := git.NameStatus(snapshot)
	if err != nil {
		return err
	}

	if o.DryRun {
		fmt.Println("[6/7] Dry run — GitHub repository will NOT be created")
		printDiff(diff)
		fmt.Printf("[7/7] Planned repository: %s (%s)\n", target, cfg.Repo.Visibility)
		return nil
	}

	if err := git.EnsureIdentity(snapshot); err != nil {
		return err
	}
	fmt.Println("[6/7] Creating initial commit")
	if err := git.Commit(snapshot, cfg.Git.InitialCommitMessage); err != nil {
		return err
	}

	fmt.Println("[7/7] Creating GitHub repository and pushing")
	url, err := gh.CreateAndPush(target, cfg.Repo.Visibility, cfg.Repo.Description, snapshot)
	if err != nil {
		return err
	}
	fmt.Println("✓ Created:", target)
	if url != "" {
		fmt.Println(url)
	}
	return nil
}

func updateFlow(o Options, cfg config.Config, target, repoURL, repoDefaultBranch, snapshot, work string, git gitops.Client, gh github.Client) error {
	fmt.Println("[5/7] Mode: UPDATE")
	repoDir := filepath.Join(work, "repo")
	if err := gh.Clone(target, repoDir); err != nil {
		return err
	}
	branch, fallback, err := git.PrepareUpdateBranch(repoDir, cfg.Git.Branch, repoDefaultBranch)
	if err != nil {
		return err
	}
	if fallback {
		fmt.Printf("↻ Configured branch %q was not present; using repository default branch %q\n", cfg.Git.Branch, branch)
	}
	if err := syncer.MirrorSnapshot(snapshot, repoDir); err != nil {
		return err
	}
	if err := git.AddAll(repoDir); err != nil {
		return err
	}
	changed, err := git.HasStagedChanges(repoDir)
	if err != nil {
		return err
	}
	if !changed {
		fmt.Println("[6/7] No changes detected")
		fmt.Println("[7/7] Nothing to commit or push")
		if repoURL != "" {
			fmt.Println(repoURL)
		}
		return nil
	}
	diff, err := git.NameStatus(repoDir)
	if err != nil {
		return err
	}

	if o.DryRun {
		fmt.Println("[6/7] Dry run — changes will NOT be committed or pushed")
		printDiff(diff)
		fmt.Println("[7/7] Repository unchanged")
		if repoURL != "" {
			fmt.Println(repoURL)
		}
		return nil
	}

	if err := git.EnsureIdentity(repoDir); err != nil {
		return err
	}
	fmt.Println("[6/7] Committing snapshot update")
	if err := git.Commit(repoDir, cfg.Git.CommitMessage); err != nil {
		return err
	}
	fmt.Println("[7/7] Pushing to GitHub")
	if err := git.Push(repoDir, branch); err != nil {
		return err
	}
	fmt.Println("✓ Updated:", target)
	if repoURL != "" {
		fmt.Println(repoURL)
	}
	return nil
}

func printDiff(diff string) {
	if strings.TrimSpace(diff) == "" {
		return
	}
	fmt.Println("Changes:")
	for _, line := range strings.Split(diff, "\n") {
		fmt.Println("  ", line)
	}
}
