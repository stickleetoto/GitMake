package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitmake/internal/archive"
	"gitmake/internal/config"
	"gitmake/internal/discovery"
	"gitmake/internal/github"
	"gitmake/internal/gitops"
	"gitmake/internal/installer"
	"gitmake/internal/projectid"
	"gitmake/internal/runner"
	"gitmake/internal/syncer"
	"gitmake/internal/upgrader"
)

const Version = "0.7.2"

type Options struct {
	Command       string
	ConfigPath    string
	SourceArg     string
	DryRun        bool
	Verbose       bool
	KeepTemp      bool
	CreateOnly    bool
	UpdateOnly    bool
	NoRelease     bool
	VersionOnly   bool
	Yes           bool
	JSON          bool
	ReadOnly      bool
	PlanID        string
	ConfigAction  string
	Stdin         bool
	MCPAllowWrite bool
	AIWrite       bool
	ApprovalToken string
	Destructive   bool
	AIClient      string
	State         *PipelineState
}

func Main(args []string) int {
	jsonRequested := hasArg(args, "--json")
	opts, err := parseArgs(args)
	if err != nil {
		if jsonRequested {
			_ = emitJSON(MachineResult{Schema: "gitmake.result/v1", OK: false, Version: Version, Command: "unknown", ExitCode: 2, Error: &MachineError{Kind: "usage_error", Code: "USAGE_ERROR", Message: err.Error(), Recoverable: true, SuggestedAction: "Run `gitmake help`."}})
		} else {
			printFriendlyError(err)
		}
		return 2
	}
	if opts.VersionOnly {
		if opts.JSON {
			_ = emitJSON(map[string]any{"schema": "gitmake.version/v1", "name": "gitmake", "version": Version})
		} else {
			fmt.Println("gitmake", Version)
		}
		return 0
	}

	// AI/config/discovery subcommands own their JSON schema because agents consume
	// these files directly as capability/installation metadata.
	if opts.JSON && (opts.Command == "ai-describe" || opts.Command == "ai-install" || opts.Command == "ai-setup" || opts.Command == "ai-status" || opts.Command == "ai-remove" || opts.Command == "discover" || opts.Command == "inspect" || opts.Command == "plan" || opts.Command == "approve" || opts.Command == "history" || opts.Command == "config") {
		if err := Run(opts); err != nil {
			_ = emitJSON(MachineResult{Schema: "gitmake.result/v1", OK: false, Version: Version, Command: opts.Command, ExitCode: 1, Error: classifyMachineError(err, opts.State)})
			return 1
		}
		return 0
	}

	if opts.JSON {
		opts.State = newPipeline(opts)
		output, runErr := captureOutput(func() error { return Run(opts) })
		result := MachineResult{Schema: "gitmake.result/v1", OK: runErr == nil, Version: Version, Command: opts.Command, ExitCode: 0, Pipeline: opts.State, Output: output}
		if runErr != nil {
			result.ExitCode = 1
			result.Error = classifyMachineError(runErr, opts.State)
		}
		if opts.Command == "publish" || opts.Command == "apply" {
			recordHistory(opts, runErr)
		}
		_ = emitJSON(result)
		return result.ExitCode
	}

	if opts.State == nil && (opts.Command == "publish" || opts.Command == "apply") {
		opts.State = newPipeline(opts)
	}
	runErr := Run(opts)
	if opts.Command == "publish" || opts.Command == "apply" {
		recordHistory(opts, runErr)
	}
	if runErr != nil {
		printFriendlyError(runErr)
		return 1
	}
	return 0
}

func hasArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
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
		if args[0] == "config" {
			if len(args) < 2 {
				return Options{}, errors.New("usage: gitmake config <schema|validate|write|patch>")
			}
			switch args[1] {
			case "schema", "validate", "write", "patch":
				o.Command = "config"
				o.ConfigAction = args[1]
			default:
				return Options{}, fmt.Errorf("unknown config command %q (expected schema, validate, write, or patch)", args[1])
			}
			args = args[2:]
		} else if args[0] == "ai" {
			if len(args) < 2 {
				return Options{}, errors.New("usage: gitmake ai <describe|install|setup|status|remove>")
			}
			switch args[1] {
			case "describe":
				o.Command = "ai-describe"
			case "install":
				o.Command = "ai-install"
			case "setup":
				o.Command = "ai-setup"
			case "status":
				o.Command = "ai-status"
			case "remove":
				o.Command = "ai-remove"
			default:
				return Options{}, fmt.Errorf("unknown AI command %q (expected describe, install, setup, status, or remove)", args[1])
			}
			args = args[2:]
		} else {
			switch args[0] {
			case "init", "doctor", "install", "upgrade", "help", "discover", "plan", "apply", "approve", "inspect", "history", "mcp":
				o.Command = args[0]
				args = args[1:]
			}
		}
	}

	args = normalizeFlagOrder(args)
	fs := flag.NewFlagSet("gitmake", flag.ContinueOnError)
	if hasArg(args, "--json") {
		fs.SetOutput(io.Discard)
	} else {
		fs.SetOutput(os.Stderr)
	}
	fs.StringVar(&o.ConfigPath, "config", "gitmake.json", "path to gitmake JSON config")
	fs.BoolVar(&o.DryRun, "dry-run", false, "show changes without creating, committing, pushing, or releasing")
	fs.BoolVar(&o.Verbose, "verbose", false, "print external commands")
	fs.BoolVar(&o.KeepTemp, "keep-temp", false, "keep temporary workspace for debugging")
	fs.BoolVar(&o.CreateOnly, "create-only", false, "fail if the GitHub repository already exists")
	fs.BoolVar(&o.UpdateOnly, "update-only", false, "fail if the GitHub repository does not exist")
	fs.BoolVar(&o.NoRelease, "no-release", false, "skip release creation even when release.enabled is true")
	fs.BoolVar(&o.VersionOnly, "version", false, "print version")
	fs.BoolVar(&o.Yes, "yes", false, "accept safe setup defaults without prompting")
	fs.BoolVar(&o.JSON, "json", false, "emit machine-readable JSON output")
	fs.BoolVar(&o.ReadOnly, "read-only", false, "block project/GitHub mutations; publish requires --dry-run")
	fs.BoolVar(&o.Stdin, "stdin", false, "read config JSON or patch JSON from standard input")
	fs.BoolVar(&o.MCPAllowWrite, "allow-write", false, "expose mutating tools in MCP mode")
	fs.BoolVar(&o.AIWrite, "write", false, "configure an AI client with reviewed GitMake write tools")
	fs.StringVar(&o.ApprovalToken, "approval", "", "one-shot approval token for MCP plan apply")
	fs.BoolVar(&o.Destructive, "destructive", false, "explicitly approve a plan classified as destructive")
	fs.StringVar(&o.AIClient, "client", "claude", "AI client for setup/status/remove: claude or generic")
	if err := fs.Parse(args); err != nil {
		return Options{}, err
	}
	rest := fs.Args()
	if o.CreateOnly && o.UpdateOnly {
		return Options{}, errors.New("--create-only and --update-only cannot be used together")
	}
	if o.AIWrite && o.Command != "ai-setup" {
		return Options{}, errors.New("--write is only valid with `gitmake ai setup`")
	}
	if o.Destructive && o.Command != "approve" && o.Command != "apply" {
		return Options{}, errors.New("--destructive is only valid with `gitmake approve` or `gitmake apply`")
	}
	if o.MCPAllowWrite && o.Command != "mcp" {
		return Options{}, errors.New("--allow-write is only valid with `gitmake mcp`")
	}
	if strings.HasPrefix(o.Command, "ai-") {
		o.AIClient = strings.ToLower(strings.TrimSpace(o.AIClient))
		switch o.AIClient {
		case "claude", "generic":
		default:
			return Options{}, fmt.Errorf("--client must be claude or generic")
		}
	}

	switch o.Command {
	case "publish", "plan":
		if len(rest) > 1 {
			return Options{}, fmt.Errorf("expected at most one ZIP path, got: %s", strings.Join(rest, " "))
		}
		if len(rest) == 1 {
			o.SourceArg = rest[0]
		}
	case "apply", "approve":
		if len(rest) != 1 {
			return Options{}, fmt.Errorf("usage: gitmake %s <plan_id>", o.Command)
		}
		o.PlanID = rest[0]
	case "init":
		if len(rest) > 1 {
			return Options{}, fmt.Errorf("usage: gitmake init [project.zip]")
		}
		if len(rest) == 1 {
			o.SourceArg = rest[0]
		}
	case "config":
		if len(rest) != 0 {
			return Options{}, fmt.Errorf("gitmake config %s does not accept positional arguments", o.ConfigAction)
		}
		if (o.ConfigAction == "write" || o.ConfigAction == "patch") && !o.Stdin {
			return Options{}, fmt.Errorf("gitmake config %s requires --stdin", o.ConfigAction)
		}
	case "doctor", "install", "upgrade", "help", "discover", "inspect", "history", "mcp", "ai-describe", "ai-install", "ai-setup", "ai-status", "ai-remove":
		if len(rest) != 0 {
			return Options{}, fmt.Errorf("gitmake %s does not accept positional arguments", o.Command)
		}
	}
	return o, nil
}

func normalizeFlagOrder(args []string) []string {
	if len(args) < 2 {
		return args
	}
	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			positional = append(positional, args[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if a == "--config" || a == "-config" || a == "--client" || a == "-client" || a == "--approval" || a == "-approval" {
				if i+1 < len(args) {
					i++
					flags = append(flags, args[i])
				}
			}
			continue
		}
		positional = append(positional, a)
	}
	return append(flags, positional...)
}

func Run(o Options) error {
	if o.ReadOnly {
		switch o.Command {
		case "install", "upgrade", "init", "ai-install", "ai-setup", "ai-remove", "apply", "approve":
			return fmt.Errorf("read-only mode blocks `gitmake %s`", strings.ReplaceAll(o.Command, "ai-", "ai "))
		case "config":
			if o.ConfigAction == "write" || o.ConfigAction == "patch" {
				return fmt.Errorf("read-only mode blocks `gitmake config %s`", o.ConfigAction)
			}
		case "publish":
			if !o.DryRun {
				return errors.New("read-only publish requires --dry-run")
			}
		}
	}
	switch o.Command {
	case "help":
		printHelp()
		return nil
	case "doctor":
		return runDoctor(o)
	case "discover":
		return runDiscover(o)
	case "plan":
		return runPlan(o)
	case "apply":
		return runApply(o)
	case "approve":
		return runApprove(o)
	case "inspect":
		return runInspect(o)
	case "history":
		return runHistory(o)
	case "mcp":
		return runMCP(o)
	case "config":
		return runConfig(o)
	case "install":
		return runInstall()
	case "upgrade":
		return runUpgrade(o)
	case "init":
		return runInit(o)
	case "ai-describe":
		return runAIDescribe(o)
	case "ai-install":
		return runAIInstall(o)
	case "ai-setup":
		return runAISetup(o)
	case "ai-status":
		return runAIStatus(o)
	case "ai-remove":
		return runAIRemove(o)
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
  gitmake discover            Classify project/release ZIPs without changing files
  gitmake plan [Project.zip]  Create an immutable reviewed publish plan
  gitmake apply <plan_id>     Revalidate and apply a previously reviewed plan
  gitmake approve <plan_id>   Create a one-shot MCP approval token
  gitmake inspect             Inspect project/config/security state without mutation
  gitmake history             Show recent GitMake publish/apply operations
  gitmake mcp                 Run the local MCP server (read-only tools by default)
  gitmake config schema       Print the machine-readable config schema
  gitmake config validate     Validate gitmake.json
  gitmake config write --stdin  Validate and write config JSON from stdin
  gitmake config patch --stdin  Merge a JSON patch into gitmake.json
  gitmake install             Install GitMake for the current user
  gitmake upgrade             Upgrade GitMake from its latest GitHub Release
  gitmake ai describe         Explain GitMake capabilities to AI agents
  gitmake ai install          Install managed AGENTS.md + .gitmake/ai.json guidance
  gitmake ai setup            Connect GitMake MCP (Claude by default; read-only)
  gitmake ai status           Show AI client / GitMake MCP connection status
  gitmake ai remove           Remove GitMake-managed AI registration

Common options:
  --dry-run       Preview without changing GitHub
  --no-release    Skip the configured Release for this run
  --verbose       Show external commands
  --yes           Accept safe setup defaults (mainly for gitmake init)
  --json          Emit machine-readable JSON
  --read-only     Block mutations; combine with --dry-run for safe AI previews
  --allow-write   In MCP mode, expose config write/patch and approved plan apply tools
  --write         With gitmake ai setup, enable config writes + approved plan apply
  --client NAME   AI setup client: claude or generic
  --destructive   Required for applying/approving plans with mass deletions
  --version       Print GitMake version

Safety:
  GitMake never force-pushes, rewrites history, or deletes repositories.
  Managed sync preserves remote-only files and protected paths by default.
  Secret/large-file/LFS/branch/tag preflight runs before mutation.
  MCP apply requires a one-shot human approval token.
  Destructive plans require a separate explicit --destructive human approval.
  Existing repository configs are never auto-retargeted to a different lone ZIP.
`, Version)
}

func runInit(o Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := resolveConfigPath(cwd, o.ConfigPath)
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("GitMake setup · %s\n\n", Version)
		fmt.Println("✓ Configuration already exists")
		fmt.Println("  " + configPath)
		fmt.Println("\nEdit it directly, or remove it and run `gitmake init` again.")
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config: %w", err)
	}

	var zipPath string
	if o.SourceArg != "" {
		zipPath = o.SourceArg
		if !filepath.IsAbs(zipPath) {
			zipPath = filepath.Join(cwd, zipPath)
		}
	} else {
		zips, err := config.DiscoverZIPs(cwd)
		if err != nil {
			return err
		}
		switch len(zips) {
		case 0:
			fmt.Printf("GitMake setup · %s\n\n", Version)
			fmt.Println("No project ZIP found in this folder.")
			fmt.Println("\nPut a .zip file here, then run:")
			fmt.Println("  gitmake init")
			fmt.Println("\nOr choose one directly:")
			fmt.Println("  gitmake init path\\to\\Project.zip")
			return nil
		case 1:
			zipPath = filepath.Join(cwd, zips[0])
		default:
			if o.Yes {
				return multipleZIPError(zips)
			}
			fmt.Printf("GitMake setup · %s\n\n", Version)
			p := newInitPrompter()
			selected, err := p.chooseZIP(zips)
			if err != nil {
				return err
			}
			zipPath = filepath.Join(cwd, selected)
			fmt.Println()
		}
	}

	cfg, err := config.ConfigForZIP(configPath, zipPath)
	if err != nil {
		return err
	}
	return runInitWizard(cfg, configPath, filepath.Base(zipPath), o.Yes)
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
	info := func(label, detail string) {
		fmt.Printf("· %-16s %s\n", label, detail)
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

	pathStatus := installer.GetPathStatus()
	installDetail := pathStatus.InstallTarget
	if installDetail == "" {
		installDetail = "installation target unavailable"
	}
	if !pathStatus.InstalledBinary && pathStatus.InstallTarget != "" {
		installDetail = "not installed (target: " + pathStatus.InstallTarget + ")"
	}
	check("GitMake install", pathStatus.InstalledBinary, installDetail)
	check("CLI command", pathStatus.Healthy(), pathStatusDetail(pathStatus))
	if pathStatus.InstalledBinary && pathStatus.UserPathHasInstall && !pathStatus.CommandAvailable && !pathStatus.CurrentIsInstalledCopy {
		info("Current shell", "user PATH is registered; reopen the terminal to refresh command resolution")
	}
	if pathStatus.ResolvedPath != "" && pathStatus.InstallTarget != "" && !samePathForDisplay(pathStatus.ResolvedPath, pathStatus.InstallTarget) {
		info("Resolved copy", pathStatus.ResolvedPath+" (different from the standard install target)")
	}

	cwd, _ := os.Getwd()
	if _, err := os.Stat(filepath.Join(cwd, "gitmake.json")); err == nil {
		check("Project config", true, filepath.Join(cwd, "gitmake.json"))
	} else {
		info("Project config", "not present in this folder (optional)")
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
	if !pathStatus.Healthy() || !pathStatus.InstalledBinary {
		fmt.Println("Install: gitmake install")
	}
	return fmt.Errorf("doctor found %d issue(s)", issues)
}

func pathStatusDetail(s installer.PathStatus) string {
	if s.ResolvedPath != "" {
		return s.ResolvedPath
	}
	if s.CurrentIsInstalledCopy && s.CurrentExecutable != "" {
		return s.CurrentExecutable
	}
	if s.UserPathHasInstall && s.InstalledBinary {
		return "registered in user PATH (restart the terminal if needed)"
	}
	if s.ProcessPathHasInstall && s.InstalledBinary {
		return "install directory is present in the current process PATH"
	}
	if s.InstallDir != "" {
		return "not registered for command use (target: " + s.InstallDir + ")"
	}
	return "not installed on PATH"
}

func samePathForDisplay(a, b string) bool {
	a = strings.TrimRight(filepath.Clean(strings.TrimSpace(a)), `\\/`)
	b = strings.TrimRight(filepath.Clean(strings.TrimSpace(b)), `\\/`)
	return strings.EqualFold(a, b)
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

func runDiscover(o Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	report, err := discovery.Analyze(cwd)
	if err != nil {
		return err
	}
	if o.JSON {
		return emitJSON(report)
	}
	fmt.Printf("GitMake Discovery · %s\n\n", Version)
	if len(report.Archives) == 0 {
		fmt.Println("No ZIP files found in this folder.")
		return nil
	}
	for _, a := range report.Archives {
		label := a.Classification
		if label == "" {
			label = "unknown"
		}
		fmt.Printf("%-14s %s", label, a.Name)
		if a.SourceScore != 0 || a.AssetScore != 0 {
			fmt.Printf("  source=%d asset=%d", a.SourceScore, a.AssetScore)
		}
		if a.Error != "" {
			fmt.Printf("  error=%s", a.Error)
		}
		fmt.Println()
	}
	fmt.Println()
	if report.SelectedSource != "" {
		fmt.Printf("✓ Source             %s (%s)\n", report.SelectedSource, report.SourceConfidence)
	} else if report.NeedsInput {
		fmt.Println("× Source             ambiguous; choose one explicitly with `gitmake Project.zip`")
	} else {
		fmt.Println("× Source             not resolved")
	}
	if len(report.ReleaseAssets) > 0 {
		fmt.Printf("· Release assets     %s\n", strings.Join(report.ReleaseAssets, ", "))
	}
	if len(report.Unknown) > 0 {
		fmt.Printf("· Unclassified       %s\n", strings.Join(report.Unknown, ", "))
	}
	return nil
}

func discoveryStateFromReport(r discovery.Report) *DiscoveryState {
	state := &DiscoveryState{
		SelectedSource:        r.SelectedSource,
		SourceConfidence:      r.SourceConfidence,
		SourceConfidenceScore: r.SourceConfidenceScore,
		SelectedSourceScore:   r.SelectedSourceScore,
		SelectedEvidence:      append([]string(nil), r.SelectedEvidence...),
		ReleaseAssets:         append([]string(nil), r.ReleaseAssets...),
		Unknown:               append([]string(nil), r.Unknown...),
		NeedsInput:            r.NeedsInput,
		Reason:                r.Reason,
	}
	for _, a := range r.Archives {
		if a.Error == "" && a.Classification != "release_asset" {
			state.Candidates = append(state.Candidates, a.Name)
		}
		state.CandidateDetails = append(state.CandidateDetails, DiscoveryCandidate{
			Name: a.Name, Classification: a.Classification, SourceScore: a.SourceScore,
			AssetScore: a.AssetScore, Reasons: append([]string(nil), a.Reasons...),
		})
	}
	return state
}

func runPublish(o Options) error {
	started := time.Now()
	if o.State != nil {
		o.State.enter("DISCOVER")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	configPath := resolveConfigPath(cwd, o.ConfigPath)

	// A positional ZIP is the strongest source-selection signal. It never
	// requires archive classification and, in read-only mode, never causes a
	// config file to be written.
	var explicitZip string
	if o.SourceArg != "" {
		explicitZip = o.SourceArg
		if !filepath.IsAbs(explicitZip) {
			explicitZip = filepath.Join(cwd, explicitZip)
		}
		info, statErr := os.Stat(explicitZip)
		if statErr != nil {
			return fmt.Errorf("source ZIP: %w", statErr)
		}
		if !info.Mode().IsRegular() || !strings.EqualFold(filepath.Ext(explicitZip), ".zip") {
			return fmt.Errorf("source must be a regular .zip file: %s", explicitZip)
		}
	}

	var cfg config.Config
	var zipPath string
	repaired := false
	configPersisted := false
	configSource := "file"
	configExists := false
	if _, statErr := os.Stat(configPath); statErr == nil {
		configExists = true
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("check config: %w", statErr)
	}

	if configExists {
		cfg, err = config.Load(configPath)
		if err != nil {
			return err
		}
		configPersisted = true
		zipPath = explicitZip
		if zipPath == "" {
			if o.ReadOnly {
				zipPath, err = config.ResolveProjectZIPReadOnly(configPath, cfg)
			} else {
				zipPath, repaired, err = config.ResolveProjectZIP(configPath, &cfg)
			}
			if err != nil {
				return err
			}
		}
	} else {
		configSource = "inferred"
		zipPath = explicitZip
		if zipPath == "" {
			report, discoverErr := discovery.Analyze(cwd)
			if discoverErr != nil {
				return discoverErr
			}
			if o.State != nil {
				o.State.Discovery = discoveryStateFromReport(report)
			}
			if report.SelectedSource == "" {
				if len(report.Archives) == 0 {
					if o.ReadOnly || o.JSON {
						return config.ErrNoProjectZIP
					}
					fmt.Printf("GitMake %s\n\n", Version)
					fmt.Println("No project ZIP found in this folder.")
					fmt.Println("\nPut one .zip file here and run `gitmake`, or use:\n  gitmake path\\to\\Project.zip")
					return nil
				}
				if report.NeedsInput {
					return errors.New("multiple source candidates found; run `gitmake discover --json` or choose one explicitly with `gitmake Project.zip`")
				}
				return errors.New("no usable project ZIP could be selected; run `gitmake discover --json` for details")
			}
			zipPath = filepath.Join(cwd, report.SelectedSource)
		}

		// Build the same validated defaults as `gitmake init`, but keep them in
		// memory for read-only previews. A mutating run persists them once.
		cfg, err = config.ConfigForZIP(configPath, zipPath)
		if err != nil {
			return err
		}
		if !o.ReadOnly {
			cfg, err = config.CreateForZIP(configPath, zipPath, false)
			if err != nil {
				return err
			}
			configPersisted = true
		}
	}
	zipPath, err = filepath.Abs(zipPath)
	if err != nil {
		return fmt.Errorf("resolve source ZIP path: %w", err)
	}
	sourceSHA, err := sha256File(zipPath)
	if err != nil {
		return fmt.Errorf("hash source ZIP: %w", err)
	}
	configSHA := ""
	if configPersisted {
		configSHA, err = sha256File(configPath)
		if err != nil {
			return fmt.Errorf("hash config: %w", err)
		}
	}
	if o.State != nil {
		o.State.Source = filepath.Base(zipPath)
		o.State.SourcePath = zipPath
		o.State.SourceSHA256 = sourceSHA
		o.State.Visibility = cfg.Repo.Visibility
		configState := &ConfigState{Source: configSource, Persisted: configPersisted, SHA256: configSHA}
		if configPersisted {
			configState.Path = configPath
		}
		o.State.Config = configState
		o.State.enter("PLAN")
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
	if o.State != nil {
		o.State.Repository = target
		if exists {
			o.State.Mode = "UPDATE"
			o.State.RemoteVisibility = strings.ToLower(strings.TrimSpace(repoInfo.Visibility))
		} else {
			o.State.Mode = "CREATE"
		}
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
	if o.State != nil {
		releaseState, stateErr := releaseStateFromPlan(release)
		if stateErr != nil {
			return stateErr
		}
		o.State.Release = releaseState
	}

	fmt.Printf("GitMake %s\n\n", Version)
	fmt.Printf("  %s\n", cfg.Repo.Name)
	fmt.Printf("  %s · %s\n\n", target, cfg.Repo.Visibility)
	if exists && strings.TrimSpace(repoInfo.Visibility) != "" && !strings.EqualFold(repoInfo.Visibility, cfg.Repo.Visibility) {
		fmt.Printf("! Visibility mismatch   config %s · remote %s (remote unchanged)\n", cfg.Repo.Visibility, strings.ToLower(repoInfo.Visibility))
	}
	if repaired {
		fmt.Printf("✓ Source selected       %s\n", filepath.Base(zipPath))
	}
	if configSource == "inferred" {
		if o.ReadOnly {
			fmt.Printf("· Config inferred       memory only · %s\n", filepath.Base(zipPath))
		} else {
			fmt.Printf("✓ Config created        %s\n", configPath)
		}
	}

	if o.State != nil {
		o.State.enter("PREPARE")
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
	if o.State != nil {
		o.State.Files = files
		o.State.enter("SECURITY")
	}
	securityReport, err := enforceSecurity(snapshot, cfg, git.HasLFS())
	if o.State != nil {
		o.State.Security = securityStateFromReport(securityReport)
	}
	if err != nil {
		return err
	}
	if o.State != nil {
		o.State.enter("VALIDATE")
	}
	fmt.Printf("✓ Source validated      %d files\n", files)
	if securityReport.SecretScan {
		fmt.Printf("✓ Security scan         %d files · no secrets\n", securityReport.ScannedFiles)
	}
	if len(securityReport.LargeFiles) > 0 {
		fmt.Printf("· Large files           %d reviewed\n", len(securityReport.LargeFiles))
	}

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
	if o.State != nil {
		o.State.enter("GIT")
		o.State.Branch = cfg.Git.Branch
	}
	syncResult, err := syncer.PrepareCreateSnapshot(snapshot)
	if err != nil {
		return err
	}
	if o.State != nil {
		o.State.Sync = &SyncState{Mode: cfg.Sync.Mode, ManagedFiles: syncResult.ManagedFiles, PriorManaged: syncResult.PriorManaged, FirstAdopt: syncResult.FirstAdopt}
	}
	identity, err := projectid.Write(snapshot, target)
	if err != nil {
		return err
	}
	if o.State != nil {
		o.State.Identity = &IdentityState{Schema: identity.Schema, ProjectID: identity.ProjectID, Repository: identity.Repository, Status: "created"}
	}
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
	if o.State != nil {
		o.State.Changes = &ChangeCounts{Added: added, Modified: modified, Deleted: deleted}
		o.State.Risk = calculateRisk(o.State.Changes, o.State.Sync, cfg.Repo.Visibility, o.State.RemoteVisibility)
	}

	if o.DryRun {
		fmt.Printf("✓ Repository plan       CREATE · +%d ~%d -%d\n", added, modified, deleted)
		fmt.Println("· Dry run               GitHub will not be changed")
		if o.State != nil {
			o.State.enter("RELEASE")
		}
		err := finishRelease(release, target, cfg.Git.Branch, true, gh)
		if o.State != nil {
			o.State.enter("REPORT")
		}
		return err
	}
	if err := git.EnsureIdentity(snapshot); err != nil {
		return err
	}
	if err := git.Commit(snapshot, cfg.Git.InitialCommitMessage); err != nil {
		return err
	}
	if o.State != nil {
		o.State.enter("PUSH")
	}
	url, err := gh.CreateAndPush(target, cfg.Repo.Visibility, cfg.Repo.Description, snapshot)
	if err != nil {
		return err
	}
	fmt.Printf("✓ Repository created    +%d ~%d -%d\n", added, modified, deleted)
	fmt.Printf("✓ Pushed                %s\n", cfg.Git.Branch)
	if url != "" {
		fmt.Println("  " + url)
		if o.State != nil {
			o.State.RepositoryURL = url
		}
	}
	if o.State != nil {
		o.State.enter("RELEASE")
	}
	err = finishRelease(release, target, cfg.Git.Branch, false, gh)
	if o.State != nil {
		if o.State.Release != nil && release.enabled && !release.skipExisting && err == nil {
			if release.resumeExisting {
				o.State.Release.Resumed = true
			} else {
				o.State.Release.Created = true
			}
		}
		o.State.enter("REPORT")
	}
	return err
}

func updateFlow(o Options, cfg config.Config, target, repoURL, repoDefaultBranch, snapshot, work string, git gitops.Client, gh github.Client, release releasePlan) error {
	if o.State != nil {
		o.State.enter("GIT")
	}
	repoDir := filepath.Join(work, "repo")
	if err := gh.Clone(target, repoDir); err != nil {
		return err
	}
	branch, fallback, err := git.PrepareUpdateBranch(repoDir, cfg.Git.Branch, repoDefaultBranch)
	if err != nil {
		return err
	}
	baseCommit, err := git.HeadSHA(repoDir)
	if err != nil {
		return err
	}
	if o.State != nil {
		o.State.Branch = branch
		o.State.BaseCommit = baseCommit
	}
	if fallback {
		fmt.Printf("· Branch fallback       %s → %s\n", cfg.Git.Branch, branch)
	}
	identity, identityExists, err := projectid.Validate(repoDir, target)
	if err != nil {
		return err
	}
	if o.State != nil {
		if identityExists {
			o.State.Identity = &IdentityState{Schema: identity.Schema, ProjectID: identity.ProjectID, Repository: identity.Repository, Status: "verified"}
		} else {
			o.State.Identity = &IdentityState{Repository: target, Status: "first_adoption"}
		}
	}
	policy, err := gh.BranchPolicy(target, branch)
	if err != nil {
		return err
	}
	if policy.RequiresPR {
		return fmt.Errorf("branch %s in %s requires pull requests; GitMake direct-push workflow will not bypass branch protection", branch, target)
	}
	if !policy.Known {
		fmt.Println("· Branch protection     could not be inspected; normal non-force push remains enforced")
	}
	syncResult, err := syncer.SyncSnapshot(snapshot, repoDir, cfg.Sync.Mode, cfg.Sync.ProtectedPaths)
	if err != nil {
		return err
	}
	if o.State != nil {
		o.State.Sync = &SyncState{Mode: cfg.Sync.Mode, ManagedFiles: syncResult.ManagedFiles, PriorManaged: syncResult.PriorManaged, FirstAdopt: syncResult.FirstAdopt, Deleted: syncResult.Deleted, Preserved: syncResult.Preserved}
	}
	// Re-write the identity after sync as protected GitMake metadata. A source
	// archive may legitimately contain other .gitmake files; it must never be
	// able to erase or replace the repository binding.
	if identityExists {
		identity, err = projectid.WriteRecord(repoDir, identity)
	} else {
		identity, err = projectid.Write(repoDir, target)
	}
	if err != nil {
		return err
	}
	if o.State != nil {
		status := "verified"
		if !identityExists {
			status = "adopted"
		}
		o.State.Identity = &IdentityState{Schema: identity.Schema, ProjectID: identity.ProjectID, Repository: identity.Repository, Status: status}
	}
	if syncResult.FirstAdopt {
		fmt.Println("· Managed sync          first adoption · remote-only files preserved")
	}
	if len(syncResult.Preserved) > 0 {
		fmt.Printf("· Protected paths       %d preserved\n", len(syncResult.Preserved))
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
			if o.State != nil {
				o.State.RepositoryURL = repoURL
			}
		}
		if o.State != nil {
			o.State.Changes = &ChangeCounts{}
			o.State.Risk = calculateRisk(o.State.Changes, o.State.Sync, cfg.Repo.Visibility, o.State.RemoteVisibility)
			o.State.enter("RELEASE")
		}
		err := finishRelease(release, target, branch, o.DryRun, gh)
		if o.State != nil {
			if o.State.Release != nil && release.enabled && !release.skipExisting && !o.DryRun && err == nil {
				if release.resumeExisting {
					o.State.Release.Resumed = true
				} else {
					o.State.Release.Created = true
				}
			}
			o.State.enter("REPORT")
		}
		return err
	}
	diff, err := git.NameStatus(repoDir)
	if err != nil {
		return err
	}
	added, modified, deleted := diffCounts(diff)
	changes := &ChangeCounts{Added: added, Modified: modified, Deleted: deleted}
	syncState := &SyncState{Mode: cfg.Sync.Mode, ManagedFiles: syncResult.ManagedFiles, PriorManaged: syncResult.PriorManaged, FirstAdopt: syncResult.FirstAdopt, Deleted: syncResult.Deleted, Preserved: syncResult.Preserved}
	remoteVisibility := ""
	if o.State != nil {
		remoteVisibility = o.State.RemoteVisibility
	}
	risk := calculateRisk(changes, syncState, cfg.Repo.Visibility, remoteVisibility)
	if o.State != nil {
		o.State.Changes = changes
		o.State.Risk = risk
	}
	if risk.Destructive && !o.DryRun && !o.Destructive {
		return fmt.Errorf("destructive change blocked: %d of %d previously managed files would be deleted (%.1f%%); create a plan, review it, then use `gitmake apply <plan_id> --destructive` or `gitmake approve <plan_id> --destructive` for MCP", risk.Deleted, risk.ManagedBaseline, risk.DeletionRatio*100)
	}
	if o.DryRun {
		fmt.Printf("✓ Repository plan       UPDATE · +%d ~%d -%d\n", added, modified, deleted)
		fmt.Println("· Dry run               no commit or push")
		if repoURL != "" {
			fmt.Println("  " + repoURL)
			if o.State != nil {
				o.State.RepositoryURL = repoURL
			}
		}
		if o.State != nil {
			o.State.enter("RELEASE")
		}
		err := finishRelease(release, target, branch, true, gh)
		if o.State != nil {
			o.State.enter("REPORT")
		}
		return err
	}
	if err := git.EnsureIdentity(repoDir); err != nil {
		return err
	}
	if err := git.Commit(repoDir, cfg.Git.CommitMessage); err != nil {
		return err
	}
	if o.State != nil {
		o.State.enter("PUSH")
	}
	if err := git.Push(repoDir, branch); err != nil {
		return err
	}
	fmt.Printf("✓ Repository updated    +%d ~%d -%d\n", added, modified, deleted)
	fmt.Printf("✓ Pushed                %s\n", branch)
	if repoURL != "" {
		fmt.Println("  " + repoURL)
		if o.State != nil {
			o.State.RepositoryURL = repoURL
		}
	}
	if o.State != nil {
		o.State.enter("RELEASE")
	}
	err = finishRelease(release, target, branch, false, gh)
	if o.State != nil {
		if o.State.Release != nil && release.enabled && !release.skipExisting && err == nil {
			if release.resumeExisting {
				o.State.Release.Resumed = true
			} else {
				o.State.Release.Created = true
			}
		}
		o.State.enter("REPORT")
	}
	return err
}

func diffCounts(diff string) (added, modified, deleted int) {
	for _, line := range strings.Split(strings.TrimSpace(diff), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		status := parts[0]
		path := ""
		if len(parts) > 1 {
			path = filepath.ToSlash(parts[len(parts)-1])
		}
		if strings.EqualFold(path, ".gitmake/managed.json") || strings.EqualFold(path, ".gitmake/project.json") {
			continue // internal ownership/identity metadata is not a user project change
		}
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
