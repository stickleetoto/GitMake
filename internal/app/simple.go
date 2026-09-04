package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gitmake/internal/gmerr"
	"gitmake/internal/planstore"
)

func stdinInteractive() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func shouldUseSimpleMode(o Options) bool {
	if o.Command != "publish" || o.JSON || o.DryRun || o.ReadOnly {
		return false
	}
	// Keep non-interactive scripts/backward-compatible automation non-blocking.
	// --yes explicitly opts into the simple reviewed workflow without prompts.
	return o.Yes || stdinInteractive()
}

func runSimplePublish(o Options) error {
	resolved := o
	if strings.TrimSpace(resolved.SourceArg) == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		configPath := resolveConfigPath(cwd, resolved.ConfigPath)
		if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
			_, err = inferSource(cwd)
			var amb *sourceAmbiguityError
			if errors.As(err, &amb) {
				selected, chooseErr := chooseSimpleSource(amb)
				if chooseErr != nil {
					return chooseErr
				}
				resolved.SourceArg = selected.Path
			} else if err != nil {
				return err
			}
		} else if statErr != nil {
			return fmt.Errorf("check config: %w", statErr)
		}
	}

	planOpts := resolved
	planOpts.Command = "plan"
	planOpts.JSON = true
	planOpts.DryRun = false
	planOpts.ReadOnly = false
	planOpts.State = nil
	out, err := captureOutput(func() error { return runPlan(planOpts) })
	if err != nil {
		return err
	}
	var p planstore.Plan
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &p); err != nil {
		return fmt.Errorf("read reviewed plan: %w", err)
	}

	fmt.Printf("GitMake %s\n\n", Version)
	fmt.Printf("%s\n", p.Repository)
	if p.Mode == "CREATE" {
		fmt.Printf("New repository · %s\n\n", p.Visibility)
	} else {
		vis := p.RemoteVisibility
		if vis == "" {
			vis = p.Visibility
		}
		fmt.Printf("Update · %s\n\n", vis)
	}
	fmt.Printf("Source     %s", p.SourceMode)
	if p.SourceMode == "zip" {
		fmt.Printf(" · %s", filepath.Base(p.SourcePath))
	}
	fmt.Println()
	fmt.Printf("Changes    +%d ~%d -%d\n", p.Changes.Added, p.Changes.Modified, p.Changes.Deleted)
	if p.Risk.Destructive {
		fmt.Printf("Risk       HIGH · destructive · %.1f%% managed deletion\n", p.Risk.DeletionRatio*100)
	} else if p.Risk.Level != "" {
		fmt.Printf("Risk       %s\n", strings.ToLower(p.Risk.Level))
	} else {
		fmt.Println("Risk       low")
	}
	if p.Release.Enabled {
		fmt.Printf("Release    %s\n", p.Release.Tag)
	} else {
		fmt.Println("Release    none")
	}
	fmt.Printf("Plan       %s\n", p.ID)

	printDecisionNotes(p.DecisionNotes)
	if p.Changes.Added == 0 && p.Changes.Modified == 0 && p.Changes.Deleted == 0 && !p.Release.Enabled {
		fmt.Println("\n✓ Already up to date")
		return nil
	}

	confirmed, destructive, err := confirmPlan(p, o.Yes, terminalPrompter{})
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Cancelled. Nothing was published.")
		return nil
	}

	apply := resolved
	apply.Command = "apply"
	apply.PlanID = p.ID
	apply.SourceArg = ""
	apply.JSON = false
	apply.DryRun = false
	apply.ReadOnly = false
	apply.Destructive = destructive
	apply.State = newPipeline(apply)
	started := time.Now()
	detail, applyErr := captureOutput(func() error { return runApply(apply) })
	if applyErr != nil {
		if o.Verbose && strings.TrimSpace(detail) != "" {
			fmt.Println("\nDetails:\n" + strings.TrimSpace(detail))
		}
		return applyErr
	}
	if o.Verbose && strings.TrimSpace(detail) != "" {
		fmt.Println("\nDetails:\n" + strings.TrimSpace(detail))
	}
	fmt.Println()
	printSimpleSuccess(p, apply.State, time.Since(started))
	return nil
}

func chooseSimpleSource(amb *sourceAmbiguityError) (sourceCandidate, error) {
	if amb == nil || len(amb.Candidates) == 0 {
		return sourceCandidate{}, gmerr.New(gmerr.SourceAmbiguous, "multiple source candidates found")
	}
	if !stdinInteractive() {
		return sourceCandidate{}, amb
	}
	fmt.Printf("GitMake %s\n\n", Version)
	fmt.Println("More than one project source looks valid:")
	for i, c := range amb.Candidates {
		label := c.Label
		if c.Recommended {
			label += " (recommended)"
		}
		fmt.Printf("  %d) %s · %s\n", i+1, label, c.Mode)
	}
	fmt.Printf("\nSelect source [1-%d]: ", len(amb.Candidates))
	line, err := readSimpleLine()
	if err != nil {
		return sourceCandidate{}, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		for _, c := range amb.Candidates {
			if c.Recommended {
				return c, nil
			}
		}
		return sourceCandidate{}, fmt.Errorf("source selection cancelled")
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(amb.Candidates) {
		return sourceCandidate{}, fmt.Errorf("invalid source selection %q", line)
	}
	return amb.Candidates[n-1], nil
}

func readSimpleLine() (string, error) {
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil && len(line) == 0 {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
