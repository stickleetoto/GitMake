package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitmake/internal/archive"
	"gitmake/internal/config"
	"gitmake/internal/github"
	"gitmake/internal/gitops"
	"gitmake/internal/installer"
	"gitmake/internal/runner"
	"gitmake/internal/syncer"
	"gitmake/internal/upgrader"
)

const Version = "0.3.0"

type Options struct {
	Command     string
	ConfigPath  string
	SourceArg   string
	DryRun      bool
	Verbose     bool
	KeepTemp    bool
	CreateOnly  bool
	UpdateOnly  bool
	NoRelease   bool
	VersionOnly bool
}

func Main(args []string) int {
	opts, err := parseArgs(args)
	if err != nil {
		printFriendlyError(err)
		return 2
	}
	if opts.VersionOnly {
		fmt.Println("gitmake", Version)
		return 0
	}
	if err := Run(opts); err != nil {
		printFriendlyError(err)
		return 1
	}
	return 0
}

func printFriendlyError(err error) {
	msg := err.Error()
	fmt.Fprintln(os.Stderr, "GitMake couldn't continue.")
	fmt.Fprintln(os.Stderr)
	fmt.Fprintln(os.Stderr, "×", msg)
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "github cli") && strings.Contains(lower, "not found"):
		fmt.Fprintln(os.Stderr, "\nInstall GitHub CLI (`gh`) and run GitMake again.")
	case strings.Contains(lower, "git not found"):
		fmt.Fprintln(os.Stderr, "\nInstall Git and run GitMake again.")
	case strings.Contains(lower, "gh auth login") || strings.Contains(lower, "authentication"):
		fmt.Fprintln(os.Stderr, "\nRun:\n  gh auth login")
	case strings.Contains(lower, "user.name") || strings.Contains(lower, "user.email"):
		fmt.Fprintln(os.Stderr, "\nConfigure your Git identity, for example:\n  git config --global user.name \"Your Name\"\n  git config --global user.email \"you@example.com\"")
	case strings.Contains(lower, "multiple zip") || strings.Contains(lower, "multiple zip files"):
		fmt.Fprintln(os.Stderr, "\nSelect the source explicitly:\n  gitmake YourProject.zip")
	}
	fmt.Fprintln(os.Stderr, "\nDiagnostics:\n  gitmake doctor\n  gitmake --verbose")
}

func parseArgs(args []string) (Options, error) {
	var o Options
	o.Command = "publish"
	if len(args) > 0 {
		switch args[0] {
		case "init", "doctor", "install", "upgrade", "help":
			o.Command = args[0]
			args = args[1:]
		}
	}

	fs := flag.NewFlagSet("gitmake", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&o.ConfigPath, "config", "gitmake.json", "path to gitmake JSON config")
	fs.BoolVar(&o.DryRun, "dry-run", false, "show changes without creating, committing, pushing, or releasing")
	fs.BoolVar(&o.Verbose, "verbose", false, "print external commands")
	fs.BoolVar(&o.KeepTemp, "keep-temp", false, "keep temporary workspace for debugging")
	fs.BoolVar(&o.CreateOnly, "create-only", false, "fail if the GitHub repository already exists")
	fs.BoolVar(&o.UpdateOnly, "update-only", false, "fail if the GitHub repository does not exist")
	fs.BoolVar(&o.NoRelease, "no-release", false, "skip release creation even when release.enabled is true")
	fs.BoolVar(&o.VersionOnly, "version", false, "print version")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	rest := fs.Args()
	if o.CreateOnly && o.UpdateOnly {
		return Options{}, errors.New("--create-only and --update-only cannot be used together")
	}

	switch o.Command {
	case "publish":
		if len(rest) > 1 {
			return Options{}, fmt.Errorf("expected at most one ZIP path, got: %s", strings.Join(rest, " "))
		}
		if len(rest) == 1 {
			o.SourceArg = rest[0]
		}
	case "init":
		if len(rest) > 1 {
			return Options{}, fmt.Errorf("usage: gitmake init [project.zip]")
		}
		if len(rest) == 1 {
			o.SourceArg = rest[0]
		}
	case "doctor", "install", "upgrade", "help":
		if len(rest) != 0 {
			return Options{}, fmt.Errorf("gitmake %s does not accept positional arguments", o.Command)
		}
	}
	return o, nil
}

func Run(o Options) error {
	switch o.Command {
	case "help":
		printHelp()
		return nil
	case "doctor":
		return runDoctor(o)
	case "install":
		return runInstall()
	case "upgrade":
		return runUpgrade(o)
	case "init":
		return runInit(o)
	default:
		return runPublish(o)
	}
}

func printHelp() {
	fmt.Printf(`GitMake %s

Usage:
  gitmake                     Publish/update the project in this folder
  gitmake Project.zip         Use a specific source ZIP
  gitmake init [Project.zip]  Create gitmake.json
  gitmake doctor              Check Git, GitHub CLI, login, identity, and PATH
  gitmake install             Install GitMake for the current Windows user
  gitmake upgrade             Upgrade GitMake from its latest GitHub Release

Common options:
  --dry-run       Preview without changing GitHub
  --no-release    Skip the configured Release for this run
  --verbose       Show external commands
  --version       Print GitMake version

Safety:
  GitMake never force-pushes, rewrites history, or deletes repositories.
`, Version)
}

func runInit(o Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := resolveConfigPath(cwd, o.ConfigPath)
	if _, err := os.Stat(configPath); err == nil {
		fmt.Println("GitMake setup")
		fmt.Println()
		fmt.Println("✓ Configuration already exists")
		fmt.Println("  " + configPath)
		return nil
	}
	if o.SourceArg != "" {
		zipPath := o.SourceArg
		if !filepath.IsAbs(zipPath) {
			zipPath = filepath.Join(cwd, zipPath)
		}
		cfg, err := config.CreateForZIP(configPath, zipPath, false)
		if err != nil {
			return err
		}
		fmt.Println("GitMake setup")
		fmt.Println()
		fmt.Printf("✓ Source      %s\n", filepath.Base(zipPath))
		fmt.Printf("✓ Repository  %s · %s\n", cfg.Repo.Name, cfg.Repo.Visibility)
		fmt.Printf("✓ Config      %s\n", configPath)
		fmt.Println("\nRun `gitmake` to publish.")
		return nil
	}
	zips, err := config.DiscoverZIPs(cwd)
	if err != nil {
		return err
	}
	if len(zips) == 1 {
		cfg, err := config.CreateForZIP(configPath, filepath.Join(cwd, zips[0]), false)
		if err != nil {
			return err
		}
		fmt.Println("GitMake setup")
		fmt.Println()
		fmt.Printf("✓ Found       %s\n", zips[0])
		fmt.Printf("✓ Repository  %s · %s\n", cfg.Repo.Name, cfg.Repo.Visibility)
		fmt.Printf("✓ Config      %s\n", configPath)
		fmt.Println("\nRun `gitmake` to publish.")
		return nil
	}
	if len(zips) > 1 {
		return multipleZIPError(zips)
	}
	created, err := config.EnsureStarter(configPath)
	if err != nil {
		return err
	}
	if created {
		fmt.Println("GitMake setup")
		fmt.Println()
		fmt.Println("✓ Created gitmake.json template")
		fmt.Println("  Add a project ZIP, edit the placeholders if needed, then run `gitmake`.")
	}
	return nil
}

func runInstall() error {
	fmt.Printf("GitMake %s · Install\n\n", Version)
	target, added, err := installer.InstallSelf()
	if err != nil {
		return err
	}
	fmt.Println("✓ Installed")
	fmt.Println("  " + target)
	if added {
		fmt.Println("✓ Added to your user PATH")
		fmt.Println("\nOpen a new PowerShell/Terminal window, then run:\n  gitmake doctor")
	} else {
		fmt.Println("✓ User PATH already contains GitMake")
	}
	return nil
}

func runUpgrade(o Options) error {
	fmt.Printf("GitMake %s · Upgrade\n\n", Version)
	run := runner.Runner{Verbose: o.Verbose}
	gh := github.Client{Run: run}
	if err := gh.Preflight(); err != nil {
		return err
	}
	tag, staged, err := upgrader.Upgrade(Version, gh)
	if err != nil {
		return err
	}
	if !staged {
		fmt.Printf("✓ Already up to date (%s)\n", tag)
		return nil
	}
	fmt.Printf("✓ Downloaded %s\n", tag)
	fmt.Println("✓ Upgrade staged")
	fmt.Println("  GitMake will replace this executable after it closes.")
	return nil
}

func runDoctor(o Options) error {
	fmt.Printf("GitMake Doctor · %s\n\n", Version)
	run := runner.Runner{Verbose: o.Verbose}
	issues := 0
	check := func(label string, ok bool, detail string) {
		if ok {
			fmt.Printf("✓ %-16s %s\n", label, detail)
		} else {
			fmt.Printf("× %-16s %s\n", label, detail)
			issues++
		}
	}

	gitVer, err := run.Run("", "git", "--version")
	check("Git", err == nil && gitVer.Code == 0, firstNonEmpty(gitVer.Stdout, "not found"))
	ghVer, err := run.Run("", "gh", "--version")
	ghOK := err == nil && ghVer.Code == 0
	check("GitHub CLI", ghOK, firstLine(firstNonEmpty(ghVer.Stdout, "not found")))

	login := "not signed in"
	authOK := false
	if ghOK {
		if res, e := run.Run("", "gh", "auth", "status", "--hostname", "github.com"); e == nil && res.Code == 0 {
			authOK = true
			if u, e2 := run.Run("", "gh", "api", "user", "--jq", ".login"); e2 == nil && u.Code == 0 {
				login = strings.TrimSpace(u.Stdout)
			} else {
				login = "signed in"
			}
		}
	}
	check("GitHub login", authOK, login)

	name, _ := run.Run("", "git", "config", "--global", "--get", "user.name")
	email, _ := run.Run("", "git", "config", "--global", "--get", "user.email")
	identOK := name.Code == 0 && email.Code == 0 && strings.TrimSpace(name.Stdout) != "" && strings.TrimSpace(email.Stdout) != ""
	ident := "not configured"
	if identOK {
		ident = strings.TrimSpace(name.Stdout) + " <" + strings.TrimSpace(email.Stdout) + ">"
	}
	check("Git identity", identOK, ident)

	check("PATH", installer.IsInstalledOnPath(), pathDetail())

	cwd, _ := os.Getwd()
	if _, err := os.Stat(filepath.Join(cwd, "gitmake.json")); err == nil {
		check("Project config", true, filepath.Join(cwd, "gitmake.json"))
	} else {
		fmt.Printf("· %-16s %s\n", "Project config", "not present in this folder (optional)")
	}

	fmt.Println()
	if issues == 0 {
		fmt.Println("Everything looks good.")
		return nil
	}
	fmt.Printf("%d issue(s) found.\n", issues)
	if !authOK && ghOK {
		fmt.Println("Run: gh auth login")
	}
	if !identOK {
		fmt.Println("Set: git config --global user.name ... and user.email ...")
	}
	if !installer.IsInstalledOnPath() {
		fmt.Println("Install: gitmake install")
	}
	return fmt.Errorf("doctor found %d issue(s)", issues)
}

func pathDetail() string {
	if installer.IsInstalledOnPath() {
		return "gitmake command is available"
	}
	if d := installer.InstallDir(); d != "" {
		return "not installed on PATH (target: " + d + ")"
	}
	return "not installed on PATH"
}

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}
func firstLine(v string) string {
	if i := strings.IndexByte(v, '\n'); i >= 0 {
		return v[:i]
	}
	return v
}

func runPublish(o Options) error {
	started := time.Now()
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := resolveConfigPath(cwd, o.ConfigPath)

	// Positional ZIP gives an unambiguous source. If no config exists, create
	// one beside the current project automatically. If a config does exist,
	// use the supplied ZIP for this invocation without rewriting unrelated
	// repository/release settings.
	var explicitZip string
	if o.SourceArg != "" {
		explicitZip = o.SourceArg
		if !filepath.IsAbs(explicitZip) {
			explicitZip = filepath.Join(cwd, explicitZip)
		}
		if _, err := os.Stat(explicitZip); err != nil {
			return fmt.Errorf("source ZIP: %w", err)
		}
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if explicitZip != "" {
			if _, err := config.CreateForZIP(configPath, explicitZip, false); err != nil {
				return err
			}
		} else {
			zips, err := config.DiscoverZIPs(cwd)
			if err != nil {
				return err
			}
			if len(zips) == 0 {
				fmt.Printf("GitMake %s\n\n", Version)
				fmt.Println("No project ZIP found in this folder.")
				fmt.Println("\nPut one .zip file here and run `gitmake`, or use:\n  gitmake path\\to\\Project.zip")
				return nil
			}
			if len(zips) > 1 {
				return multipleZIPError(zips)
			}
			if _, err := config.CreateForZIP(configPath, filepath.Join(cwd, zips[0]), false); err != nil {
				return err
			}
		}
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	zipPath := explicitZip
	repaired := false
	if zipPath == "" {
		zipPath, repaired, err = config.ResolveProjectZIP(configPath, &cfg)
		if err != nil {
			return err
		}
	}

	run := runner.Runner{Verbose: o.Verbose}
	git := gitops.Client{Run: run}
	gh := github.Client{Run: run}
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

	release, err := prepareReleasePlan(configPath, target, exists, cfg, o.NoRelease, git, gh)
	if err != nil {
		return err
	}

	fmt.Printf("GitMake %s\n\n", Version)
	fmt.Printf("  %s\n", cfg.Repo.Name)
	fmt.Printf("  %s · %s\n\n", target, cfg.Repo.Visibility)
	if repaired {
		fmt.Printf("✓ Source selected       %s\n", filepath.Base(zipPath))
	}

	work, err := os.MkdirTemp("", "gitmake-*")
	if err != nil {
		return err
	}
	if o.KeepTemp {
		fmt.Println("· Temporary workspace  " + work)
	} else {
		defer os.RemoveAll(work)
	}
	snapshot := filepath.Join(work, "snapshot")
	files, err := archive.ExtractSafe(zipPath, snapshot, *cfg.Source.StripRoot)
	if err != nil {
		return err
	}
	if files == 0 {
		return fmt.Errorf("source ZIP contains no regular files")
	}
	fmt.Printf("✓ Source validated      %d files\n", files)

	if exists {
		if err := updateFlow(o, cfg, target, repoInfo.URL, repoInfo.DefaultBranch(), snapshot, work, git, gh, release); err != nil {
			return err
		}
	} else {
		if err := createFlow(o, cfg, target, snapshot, git, gh, release); err != nil {
			return err
		}
	}
	fmt.Printf("\nDone in %.1fs\n", time.Since(started).Seconds())
	return nil
}

func createFlow(o Options, cfg config.Config, target, snapshot string, git gitops.Client, gh github.Client, release releasePlan) error {
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
	added, modified, deleted := diffCounts(diff)

	if o.DryRun {
		fmt.Printf("✓ Repository plan       CREATE · +%d ~%d -%d\n", added, modified, deleted)
		fmt.Println("· Dry run               GitHub will not be changed")
		return finishRelease(release, target, cfg.Git.Branch, true, gh)
	}
	if err := git.EnsureIdentity(snapshot); err != nil {
		return err
	}
	if err := git.Commit(snapshot, cfg.Git.InitialCommitMessage); err != nil {
		return err
	}
	url, err := gh.CreateAndPush(target, cfg.Repo.Visibility, cfg.Repo.Description, snapshot)
	if err != nil {
		return err
	}
	fmt.Printf("✓ Repository created    +%d ~%d -%d\n", added, modified, deleted)
	fmt.Printf("✓ Pushed                %s\n", cfg.Git.Branch)
	if url != "" {
		fmt.Println("  " + url)
	}
	return finishRelease(release, target, cfg.Git.Branch, false, gh)
}

func updateFlow(o Options, cfg config.Config, target, repoURL, repoDefaultBranch, snapshot, work string, git gitops.Client, gh github.Client, release releasePlan) error {
	repoDir := filepath.Join(work, "repo")
	if err := gh.Clone(target, repoDir); err != nil {
		return err
	}
	branch, fallback, err := git.PrepareUpdateBranch(repoDir, cfg.Git.Branch, repoDefaultBranch)
	if err != nil {
		return err
	}
	if fallback {
		fmt.Printf("· Branch fallback       %s → %s\n", cfg.Git.Branch, branch)
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
		fmt.Println("✓ Repository            already up to date")
		if repoURL != "" {
			fmt.Println("  " + repoURL)
		}
		return finishRelease(release, target, branch, o.DryRun, gh)
	}
	diff, err := git.NameStatus(repoDir)
	if err != nil {
		return err
	}
	added, modified, deleted := diffCounts(diff)
	if o.DryRun {
		fmt.Printf("✓ Repository plan       UPDATE · +%d ~%d -%d\n", added, modified, deleted)
		fmt.Println("· Dry run               no commit or push")
		if repoURL != "" {
			fmt.Println("  " + repoURL)
		}
		return finishRelease(release, target, branch, true, gh)
	}
	if err := git.EnsureIdentity(repoDir); err != nil {
		return err
	}
	if err := git.Commit(repoDir, cfg.Git.CommitMessage); err != nil {
		return err
	}
	if err := git.Push(repoDir, branch); err != nil {
		return err
	}
	fmt.Printf("✓ Repository updated    +%d ~%d -%d\n", added, modified, deleted)
	fmt.Printf("✓ Pushed                %s\n", branch)
	if repoURL != "" {
		fmt.Println("  " + repoURL)
	}
	return finishRelease(release, target, branch, false, gh)
}

func diffCounts(diff string) (added, modified, deleted int) {
	for _, line := range strings.Split(strings.TrimSpace(diff), "\n") {
		if line == "" {
			continue
		}
		status := strings.SplitN(line, "\t", 2)[0]
		if strings.HasPrefix(status, "A") {
			added++
		} else if strings.HasPrefix(status, "D") {
			deleted++
		} else {
			modified++
		}
	}
	return
}

func multipleZIPError(zips []string) error {
	sort.Strings(zips)
	var b strings.Builder
	b.WriteString("multiple ZIP files found; GitMake will not guess:\n")
	for _, z := range zips {
		b.WriteString("  - " + z + "\n")
	}
	return errors.New(strings.TrimSpace(b.String()))
}

func resolveConfigPath(cwd, configured string) string {
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(cwd, configured)
}
