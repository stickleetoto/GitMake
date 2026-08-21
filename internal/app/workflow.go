package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitmake/internal/history"
	"gitmake/internal/planstore"
)

func runPlan(o Options) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	preview := o
	preview.Command = "publish"
	preview.DryRun = true
	preview.ReadOnly = true
	preview.JSON = false
	preview.State = newPipeline(preview)

	output, runErr := captureOutput(func() error { return runPublish(preview) })
	if runErr != nil {
		return runErr
	}

	id, err := planstore.NewID()
	if err != nil {
		return fmt.Errorf("create plan id: %w", err)
	}
	preview.State.PlanID = id
	p, err := planFromState(id, cwd, preview.State)
	if err != nil {
		return err
	}
	p.CreatedAt = time.Now().UTC()
	path, err := planstore.Save(p)
	if err != nil {
		return fmt.Errorf("save plan: %w", err)
	}

	if o.State != nil {
		*o.State = *preview.State
	}
	if o.JSON {
		return emitJSON(p)
	}

	fmt.Printf("GitMake Plan · %s\n\n", Version)
	fmt.Printf("✓ Plan ID              %s\n", p.ID)
	fmt.Printf("· Working directory    %s\n", p.WorkingDirectory)
	if p.ConfigPath != "" {
		fmt.Printf("· Config               %s\n", p.ConfigPath)
	} else {
		fmt.Println("· Config               inferred in memory")
	}
	fmt.Printf("· Source               %s\n", p.SourcePath)
	fmt.Printf("✓ Repository           %s · %s\n", p.Repository, p.Mode)
	if p.RemoteVisibility != "" {
		fmt.Printf("· Visibility           config %s · remote %s\n", p.Visibility, p.RemoteVisibility)
	} else {
		fmt.Printf("· Visibility           %s\n", p.Visibility)
	}
	fmt.Printf("✓ Source SHA-256       %.16s…\n", p.SourceSHA256)
	fmt.Printf("✓ Changes              +%d ~%d -%d\n", p.Changes.Added, p.Changes.Modified, p.Changes.Deleted)
	if p.Identity.Status != "" {
		fmt.Printf("· Project identity     %s", p.Identity.Status)
		if p.Identity.ProjectID != "" {
			fmt.Printf(" · %s", p.Identity.ProjectID)
		}
		fmt.Println()
	}
	if p.Risk.Destructive {
		fmt.Printf("× Risk                 HIGH · destructive · %.1f%% managed deletion\n", p.Risk.DeletionRatio*100)
		for _, reason := range p.Risk.Reasons {
			fmt.Printf("  ! %s\n", reason)
		}
	} else if p.Risk.Level != "" && p.Risk.Level != "low" {
		fmt.Printf("! Risk                 %s\n", strings.ToUpper(p.Risk.Level))
		for _, reason := range p.Risk.Reasons {
			fmt.Printf("  ! %s\n", reason)
		}
	} else {
		fmt.Println("✓ Risk                 low")
	}
	if p.Release.Enabled {
		fmt.Printf("· Release              %s · %d assets\n", p.Release.Tag, len(p.Release.Assets))
	}
	fmt.Printf("· Stored               %s\n", path)
	if strings.TrimSpace(output) != "" && o.Verbose {
		fmt.Println("\nPreview output:\n" + strings.TrimSpace(output))
	}
	if p.Risk.Destructive {
		fmt.Printf("\nThis plan is destructive. Local CLI apply requires:\n  gitmake apply %s --destructive\n", p.ID)
		fmt.Printf("For AI/MCP, only a human can mint the destructive token:\n  gitmake approve %s --destructive\n", p.ID)
	} else {
		fmt.Printf("\nReview complete. Local CLI apply:\n  gitmake apply %s\n", p.ID)
		fmt.Printf("For AI/MCP, create a one-shot approval token first:\n  gitmake approve %s\n", p.ID)
	}
	return nil
}

func runApply(o Options) error {
	p, _, err := planstore.Load(o.PlanID)
	if err != nil {
		return err
	}
	if p.Risk.Destructive && !o.Destructive {
		return fmt.Errorf("destructive plan blocked: %d of %d managed files would be deleted (%.1f%%); explicit human approval requires `gitmake apply %s --destructive` or a destructive MCP approval token", p.Risk.Deleted, p.Risk.ManagedBaseline, p.Risk.DeletionRatio*100, p.ID)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cwd, _ = filepath.Abs(cwd)
	plannedWD, _ := filepath.Abs(p.WorkingDirectory)
	if !samePathForDisplay(cwd, plannedWD) {
		return fmt.Errorf("plan is stale: it was created for %s, current directory is %s", plannedWD, cwd)
	}

	sourceSHA, err := sha256File(p.SourcePath)
	if err != nil {
		return fmt.Errorf("plan is stale: source ZIP is unavailable: %w", err)
	}
	if !strings.EqualFold(sourceSHA, p.SourceSHA256) {
		return fmt.Errorf("plan is stale: source ZIP changed after review")
	}
	if p.ConfigPersisted {
		configSHA, err := sha256File(p.ConfigPath)
		if err != nil {
			return fmt.Errorf("plan is stale: config is unavailable: %w", err)
		}
		if !strings.EqualFold(configSHA, p.ConfigSHA256) {
			return fmt.Errorf("plan is stale: gitmake.json changed after review")
		}
	}

	// Re-run the non-mutating plan immediately before execution. This catches
	// remote branch movement, release state changes, and local asset changes.
	preview := Options{Command: "publish", ConfigPath: p.ConfigPath, SourceArg: p.SourcePath, DryRun: true, ReadOnly: true, JSON: false, NoRelease: o.NoRelease, Verbose: o.Verbose, State: newPipeline(Options{DryRun: true, ReadOnly: true})}
	if preview.ConfigPath == "" {
		preview.ConfigPath = "gitmake.json"
	}
	_, previewErr := captureOutput(func() error { return runPublish(preview) })
	if previewErr != nil {
		return fmt.Errorf("plan is stale: revalidation failed: %w", previewErr)
	}
	fp, err := fingerprintState(preview.State)
	if err != nil {
		return err
	}
	if !strings.EqualFold(fp, p.Fingerprint) {
		return fmt.Errorf("plan is stale: repository, source, release assets, or planned changes no longer match the reviewed plan")
	}

	actual := o
	actual.Command = "publish"
	actual.SourceArg = p.SourcePath
	actual.ConfigPath = p.ConfigPath
	if actual.ConfigPath == "" {
		actual.ConfigPath = "gitmake.json"
	}
	actual.DryRun = false
	actual.ReadOnly = false
	actual.State = newPipeline(actual)
	actual.State.PlanID = p.ID
	err = runPublish(actual)
	if o.State != nil {
		*o.State = *actual.State
		o.State.PlanID = p.ID
	}
	return err
}

func runHistory(o Options) error {
	entries, err := history.List(20)
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	if o.JSON {
		return emitJSON(map[string]any{"schema": history.Schema, "version": Version, "entries": entries})
	}
	fmt.Printf("GitMake History · %s\n\n", Version)
	if len(entries) == 0 {
		fmt.Println("No recorded operations yet.")
		return nil
	}
	for _, e := range entries {
		status := "✓"
		if !e.OK {
			status = "×"
		}
		when := e.Time.Local().Format("2006-01-02 15:04:05")
		target := e.Repository
		if target == "" {
			target = "-"
		}
		detail := fmt.Sprintf("%s %s  %-7s %-24s", status, when, strings.ToUpper(e.Command), target)
		if e.Mode != "" {
			detail += "  " + e.Mode
		}
		if e.Added != 0 || e.Modified != 0 || e.Deleted != 0 {
			detail += fmt.Sprintf("  +%d ~%d -%d", e.Added, e.Modified, e.Deleted)
		}
		if e.ReleaseTag != "" {
			detail += "  " + e.ReleaseTag
		}
		if e.DryRun {
			detail += "  DRY-RUN"
		}
		if e.ReadOnly {
			detail += "  READ-ONLY"
		}
		if e.PlanID != "" {
			detail += "  " + e.PlanID
		}
		fmt.Println(detail)
		if e.ErrorCode != "" {
			fmt.Printf("  %s: %s\n", e.ErrorCode, e.Error)
		}
	}
	return nil
}

func recordHistory(o Options, runErr error) {
	if o.State == nil {
		return
	}
	e := history.Entry{Command: o.Command, OK: runErr == nil, Repository: o.State.Repository, Mode: o.State.Mode, Source: o.State.Source, PlanID: o.State.PlanID, DryRun: o.State.DryRun, ReadOnly: o.State.ReadOnly}
	if o.State.Changes != nil {
		e.Added = o.State.Changes.Added
		e.Modified = o.State.Changes.Modified
		e.Deleted = o.State.Changes.Deleted
	}
	if o.State.Release != nil {
		e.ReleaseTag = o.State.Release.Tag
	}
	if runErr != nil {
		me := classifyMachineError(runErr, o.State)
		e.ErrorCode = me.Code
		e.Error = runErr.Error()
	}
	_ = history.Record(e) // History must never make the requested operation fail.
}
