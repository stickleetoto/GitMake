package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/approval"
	"gitmake/internal/config"
	"gitmake/internal/discovery"
	"gitmake/internal/planstore"
	"gitmake/internal/securityscan"
)

func runApprove(o Options) error {
	p, _, err := planstore.Load(o.PlanID)
	if err != nil {
		return err
	}
	if p.Risk.Destructive && !o.Destructive {
		return fmt.Errorf("plan %s is classified as destructive: %d of %d managed files would be deleted (%.1f%%); review provenance and run `gitmake approve %s --destructive` yourself if this is intentional", p.ID, p.Risk.Deleted, p.Risk.ManagedBaseline, p.Risk.DeletionRatio*100, p.ID)
	}
	st, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("human approval requires an interactive terminal; run `gitmake approve %s` yourself", o.PlanID)
	}
	confirm := o.PlanID
	if len(confirm) > 6 {
		confirm = confirm[len(confirm)-6:]
	}
	if p.Risk.Destructive {
		confirm = "DESTRUCTIVE-" + confirm
		fmt.Fprintf(os.Stderr, "DESTRUCTIVE approval for plan %s (%d deletions, %.1f%% of managed baseline).\n", o.PlanID, p.Risk.Deleted, p.Risk.DeletionRatio*100)
	} else {
		fmt.Fprintf(os.Stderr, "Approve one MCP apply for plan %s?\n", o.PlanID)
	}
	fmt.Fprintf(os.Stderr, "Type %s to confirm: ", confirm)
	line, readErr := bufio.NewReader(os.Stdin).ReadString('\n')
	if readErr != nil && strings.TrimSpace(line) == "" {
		return readErr
	}
	if strings.TrimSpace(line) != confirm {
		return fmt.Errorf("approval cancelled")
	}
	token, expires, err := approval.Create(o.PlanID, p.Risk.Destructive)
	if err != nil {
		return fmt.Errorf("create approval token: %w", err)
	}
	if o.JSON {
		return emitJSON(map[string]any{
			"schema": "gitmake.approval/v1", "ok": true, "plan_id": o.PlanID,
			"approval_token": token, "expires_at": expires,
			"single_use": true, "destructive": p.Risk.Destructive,
		})
	}
	fmt.Printf("GitMake Approval · %s\n\n", Version)
	fmt.Printf("✓ Plan                 %s\n", o.PlanID)
	fmt.Printf("✓ One-shot token       %s\n", token)
	fmt.Printf("· Expires              %s\n", expires.Local().Format("2006-01-02 15:04:05"))
	if p.Risk.Destructive {
		fmt.Println("! Approval class       DESTRUCTIVE")
	}
	fmt.Println("\nGive this token only to the AI/tool that should apply this reviewed plan.")
	fmt.Println("It becomes unusable after one successful MCP apply.")
	return nil
}

func runInspect(o Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	report := map[string]any{
		"schema":    "gitmake.inspect/v1",
		"version":   Version,
		"directory": cwd,
	}

	configPath := resolveConfigPath(cwd, o.ConfigPath)
	if cfg, err := config.Load(configPath); err == nil {
		report["config"] = map[string]any{"present": true, "valid": true, "path": configPath, "normalized": cfg}
	} else if os.IsNotExist(pathCause(err)) {
		report["config"] = map[string]any{"present": false, "valid": false, "path": configPath}
	} else {
		report["config"] = map[string]any{"present": fileExists(configPath), "valid": false, "path": configPath, "error": err.Error()}
	}

	d, derr := discovery.Analyze(cwd)
	if derr != nil {
		report["discovery_error"] = derr.Error()
	} else {
		report["discovery"] = d
	}

	if o.JSON {
		return emitJSON(report)
	}
	fmt.Printf("GitMake Inspect · %s\n\n", Version)
	if c, ok := report["config"].(map[string]any); ok {
		if c["valid"] == true {
			fmt.Println("✓ Config               valid")
		} else if c["present"] == true {
			fmt.Println("× Config               invalid")
		} else {
			fmt.Println("· Config               not present")
		}
	}
	if d.SelectedSource != "" {
		fmt.Printf("✓ Source               %s · %.0f%% confidence\n", d.SelectedSource, d.SourceConfidenceScore*100)
	} else if d.NeedsInput {
		fmt.Printf("× Source               needs input (%s)\n", d.Reason)
	} else {
		fmt.Println("· Source               not resolved")
	}
	return nil
}

func securityStateFromReport(r securityscan.Report) *SecurityState {
	s := &SecurityState{SecretScan: r.SecretScan, ScannedFiles: r.ScannedFiles, LFSRequired: r.LFSRequired, Blocking: r.Blocking}
	for _, f := range r.Findings {
		s.Findings = append(s.Findings, SecurityFinding{Path: f.Path, Kind: f.Kind, Detail: f.Detail})
	}
	for _, f := range r.LargeFiles {
		s.LargeFiles = append(s.LargeFiles, LargeFileState{Path: f.Path, Bytes: f.Bytes, LFSMarked: f.LFSMarked, Blocking: f.Blocking})
	}
	return s
}

func enforceSecurity(snapshot string, cfg config.Config, hasLFS bool) (securityscan.Report, error) {
	secretScan := cfg.Security.SecretScan == nil || *cfg.Security.SecretScan
	report, err := securityscan.Scan(snapshot, securityscan.Options{
		SecretScan:       secretScan,
		AllowSecretPaths: cfg.Security.AllowSecretPaths,
		WarnFileBytes:    cfg.Security.WarnFileBytes,
		MaxGitFileBytes:  cfg.Security.MaxGitFileBytes,
	})
	if err != nil {
		return report, err
	}
	if len(report.Findings) > 0 {
		var names []string
		for _, f := range report.Findings {
			names = append(names, f.Path+" ("+f.Kind+")")
		}
		return report, fmt.Errorf("potential secrets detected; publish blocked: %s", strings.Join(names, ", "))
	}
	for _, lf := range report.LargeFiles {
		if lf.Blocking {
			return report, fmt.Errorf("large file %s (%d bytes) exceeds safe direct-Git threshold; configure Git LFS or reduce the file", lf.Path, lf.Bytes)
		}
		if lf.LFSMarked && !hasLFS {
			return report, fmt.Errorf("%s is marked for Git LFS but git-lfs is not installed", lf.Path)
		}
	}
	return report, nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// pathCause returns the underlying path error when possible. os.IsNotExist can
// still inspect wrapped errors, so returning the original error is enough.
func pathCause(err error) error { return err }

func projectConfigForSuggestion(projectDir, sourceZIP, repoName, visibility, branch string) (config.Config, error) {
	if strings.TrimSpace(projectDir) == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return config.Config{}, err
		}
	}
	if sourceZIP == "" {
		d, err := discovery.Analyze(projectDir)
		if err != nil {
			return config.Config{}, err
		}
		if d.SelectedSource == "" {
			if d.NeedsInput {
				return config.Config{}, fmt.Errorf("source selection requires input: %s", d.Reason)
			}
			return config.Config{}, config.ErrNoProjectZIP
		}
		sourceZIP = filepath.Join(projectDir, d.SelectedSource)
	} else if !filepath.IsAbs(sourceZIP) {
		sourceZIP = filepath.Join(projectDir, sourceZIP)
	}
	cfg, err := config.ConfigForZIP(filepath.Join(projectDir, "gitmake.json"), sourceZIP)
	if err != nil {
		return config.Config{}, err
	}
	if repoName != "" {
		cfg.Repo.Name = repoName
	}
	if visibility != "" {
		cfg.Repo.Visibility = visibility
	}
	if branch != "" {
		cfg.Git.Branch = branch
	}
	// Re-parse normalized JSON to apply strict defaults/validation without
	// exposing package-private default helpers.
	b, err := config.MarshalNormalized(cfg)
	if err != nil {
		return config.Config{}, err
	}
	return config.ParseBytes(b)
}
