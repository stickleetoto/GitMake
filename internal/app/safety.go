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
	"gitmake/internal/foldersource"
	"gitmake/internal/planstore"
	"gitmake/internal/securityscan"
)

func approvalBindingFromPlan(p planstore.Plan) approval.Binding {
	return approval.Binding{
		Fingerprint:  p.Fingerprint,
		SourceSHA256: p.SourceSHA256,
		ConfigSHA256: p.ConfigSHA256,
		Repository:   p.Repository,
	}
}

func runApprove(o Options) error {
	var p planstore.Plan
	var err error
	if strings.TrimSpace(o.PlanID) == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return cwdErr
		}
		p, _, err = planstore.LatestForDirectory(cwd)
	} else {
		p, _, err = planstore.Load(o.PlanID)
	}
	if err != nil {
		return err
	}
	o.PlanID = p.ID
	if p.Risk.Destructive && !o.Destructive {
		return fmt.Errorf("plan %s is classified as destructive: %d of %d managed files would be deleted (%.1f%%); review provenance and run `gitmake approve --destructive` yourself if this is intentional", p.ID, p.Risk.Deleted, p.Risk.ManagedBaseline, p.Risk.DeletionRatio*100)
	}
	st, err := os.Stdin.Stat()
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeCharDevice == 0 {
		return fmt.Errorf("human approval requires an interactive terminal; run `gitmake approve` yourself")
	}

	fmt.Fprintf(os.Stderr, "GitMake Approval · %s\n\n", Version)
	fmt.Fprintf(os.Stderr, "Plan        %s\n", p.ID)
	fmt.Fprintf(os.Stderr, "Repository  %s\n", p.Repository)
	fmt.Fprintf(os.Stderr, "Changes     +%d ~%d -%d\n", p.Changes.Added, p.Changes.Modified, p.Changes.Deleted)
	level := strings.ToLower(strings.TrimSpace(p.Risk.Level))
	if level == "" {
		level = "low"
	}
	fmt.Fprintf(os.Stderr, "Risk        %s\n", level)

	confirmed := false
	reader := bufio.NewReader(os.Stdin)
	if p.Risk.Destructive || level == "high" {
		code := "DELETE-" + confirmationCode(p.ID)
		fmt.Fprintf(os.Stderr, "\nDestructive approval. Type %s to confirm: ", code)
		line, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(line) == "" {
			return readErr
		}
		confirmed = strings.TrimSpace(line) == code
	} else if level == "medium" {
		fmt.Fprint(os.Stderr, "\nType PUBLISH to approve this reviewed plan: ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(line) == "" {
			return readErr
		}
		confirmed = strings.EqualFold(strings.TrimSpace(line), "PUBLISH")
	} else {
		fmt.Fprint(os.Stderr, "\nApprove this reviewed plan? [Y/n]: ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil && strings.TrimSpace(line) == "" {
			return readErr
		}
		v := strings.ToLower(strings.TrimSpace(line))
		confirmed = v == "" || v == "y" || v == "yes"
	}
	if !confirmed {
		return fmt.Errorf("approval cancelled")
	}

	record, err := approval.CreateGrant(p.ID, approvalBindingFromPlan(p), p.Risk.Destructive)
	if err != nil {
		return fmt.Errorf("create approval grant: %w", err)
	}
	if o.JSON {
		return emitJSON(map[string]any{
			"schema": approval.Schema, "ok": true, "plan_id": p.ID,
			"approved": true, "expires_at": record.ExpiresAt,
			"single_use": true, "destructive": p.Risk.Destructive,
			"tokenless": true,
		})
	}
	fmt.Printf("\n✓ Approved             %s\n", p.ID)
	fmt.Printf("· Expires              %s\n", record.ExpiresAt.Local().Format("2006-01-02 15:04:05"))
	if p.Risk.Destructive {
		fmt.Println("! Approval class       DESTRUCTIVE")
	}
	fmt.Println("\nNo token to copy. Return to the AI; GitMake MCP can now apply this plan once.")
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
	fd, ferr := foldersource.DetectProject(cwd)
	if ferr != nil {
		report["folder_detection_error"] = ferr.Error()
	} else {
		report["folder_detection"] = fd
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
	if fd.IsProject && fd.Confidence == "high" {
		fmt.Printf("✓ Folder source        %s confidence\n", fd.Confidence)
	}
	if d.SelectedSource != "" {
		fmt.Printf("✓ ZIP source           %s · %.0f%% confidence\n", d.SelectedSource, d.SourceConfidenceScore*100)
	} else if d.NeedsInput {
		fmt.Printf("× ZIP source           needs input (%s)\n", d.Reason)
	} else if !fd.IsProject {
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

func projectConfigForSuggestion(projectDir, sourcePath, repoName, visibility, branch string) (config.Config, error) {
	if strings.TrimSpace(projectDir) == "" {
		var err error
		projectDir, err = os.Getwd()
		if err != nil {
			return config.Config{}, err
		}
	}
	if sourcePath == "" {
		sel, err := inferSource(projectDir)
		if err != nil {
			return config.Config{}, err
		}
		sourcePath = sel.Path
	} else if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(projectDir, sourcePath)
	}
	cfg, err := config.ConfigForSource(filepath.Join(projectDir, "gitmake.json"), sourcePath)
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
	b, err := config.MarshalNormalized(cfg)
	if err != nil {
		return config.Config{}, err
	}
	return config.ParseBytes(b)
}
