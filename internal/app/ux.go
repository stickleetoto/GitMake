package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitmake/internal/planstore"
)

// decisionNotesFromState records only explanations supported by the resolved
// pipeline state. The goal is to make automatic choices understandable without
// inventing rationale that GitMake did not actually use.
func decisionNotesFromState(s *PipelineState) []string {
	if s == nil {
		return nil
	}
	var notes []string
	if s.Config != nil {
		switch s.Config.Source {
		case "inferred":
			notes = append(notes, "Configuration was inferred in memory; no gitmake.json was required.")
		case "project_memory":
			notes = append(notes, "Repository target was restored from .gitmake/project.json project memory.")
		case "file":
			notes = append(notes, "Persistent gitmake.json supplied the repository and publishing settings.")
		}
	}
	switch s.SourceMode {
	case "folder":
		notes = append(notes, "The project folder is published as a deterministic snapshot after ignore rules are applied.")
	case "zip":
		notes = append(notes, "The selected ZIP is published as the reviewed source snapshot.")
	}
	if s.Discovery != nil {
		for _, evidence := range s.Discovery.SelectedEvidence {
			evidence = strings.TrimSpace(evidence)
			if evidence != "" {
				notes = append(notes, "Source evidence: "+evidence+".")
				break
			}
		}
	}
	if s.Mode == "CREATE" && strings.EqualFold(s.Visibility, "private") && s.Config != nil && s.Config.Source != "file" {
		notes = append(notes, "Private visibility is the zero-config safety default for a new repository.")
	}
	if s.RemoteVisibility != "" {
		notes = append(notes, "Existing repository visibility is preserved; GitMake does not silently change it during an update.")
	}
	if s.Identity != nil {
		switch s.Identity.Status {
		case "verified":
			notes = append(notes, "Project identity matches the repository binding recorded by GitMake.")
		case "first_adoption", "adopted":
			notes = append(notes, "This is the first managed-sync adoption; remote-only files are preserved.")
		}
	}
	return uniqueStrings(notes)
}

func uniqueStrings(in []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func printDecisionNotes(notes []string) {
	if len(notes) == 0 {
		return
	}
	fmt.Println("\nWhy")
	for _, note := range notes {
		fmt.Println("  ↳ " + note)
	}
}

func confirmationCode(planID string) string {
	raw := strings.TrimPrefix(strings.TrimSpace(planID), "gm_")
	if len(raw) > 6 {
		raw = raw[len(raw)-6:]
	}
	if raw == "" {
		raw = "CONFIRM"
	}
	return strings.ToUpper(raw)
}

func printSimpleSuccess(p planstore.Plan, state *PipelineState, elapsed time.Duration) {
	target := p.Repository
	branch := p.Branch
	changes := p.Changes
	repoURL := ""
	if state != nil {
		if state.Repository != "" {
			target = state.Repository
		}
		if state.Branch != "" {
			branch = state.Branch
		}
		if state.Changes != nil {
			changes = planstore.ChangeCounts{Added: state.Changes.Added, Modified: state.Changes.Modified, Deleted: state.Changes.Deleted}
		}
		repoURL = state.RepositoryURL
	}
	name := target
	if _, tail := filepath.Split(target); tail != "" {
		name = tail
	}
	if strings.Contains(target, "/") {
		parts := strings.Split(target, "/")
		name = parts[len(parts)-1]
	}
	fmt.Printf("✓ Published %s\n\n", name)
	fmt.Printf("Repository  %s\n", target)
	if branch != "" {
		fmt.Printf("Branch      %s\n", branch)
	}
	fmt.Printf("Changes     +%d ~%d -%d\n", changes.Added, changes.Modified, changes.Deleted)
	if p.Release.Enabled {
		fmt.Printf("Release     %s\n", p.Release.Tag)
	} else {
		fmt.Println("Release     none")
	}
	// Simple Mode is deliberately short, but whether GitHub actually holds what
	// was just approved is not a detail: it is the one fact a summary of a
	// publish should not leave the reader to assume.
	if state != nil && state.Verification != nil && state.Verification.Checked {
		if state.Verification.RemoteCommit != "" {
			fmt.Printf("Verified    remote at %.12s\n", state.Verification.RemoteCommit)
		} else {
			fmt.Println("Verified    yes")
		}
	}
	fmt.Printf("Time        %.1fs\n", elapsed.Seconds())
	if repoURL != "" {
		fmt.Println("\n" + repoURL)
	}
}

func printGuidedRecovery(err error) {
	if err == nil {
		return
	}
	me := classifyMachineError(err, nil)
	fmt.Fprintln(os.Stderr, "\nRecommended")
	if me != nil && strings.TrimSpace(me.SuggestedAction) != "" {
		fmt.Fprintln(os.Stderr, "  → "+me.SuggestedAction)
	}
	if me == nil {
		return
	}
	switch me.Code {
	case "SECRET_DETECTED":
		fmt.Fprintln(os.Stderr, "  → If a detected file should never be published, exclude it with .gitignore or .gitmakeignore.")
		fmt.Fprintln(os.Stderr, "  → Do not add broad secret-scan allowlists just to make the warning disappear.")
	case "PLAN_STALE", "REMOTE_MOVED":
		fmt.Fprintln(os.Stderr, "  → Re-run `gitmake` to build and review a fresh plan.")
	case "SOURCE_AMBIGUOUS":
		fmt.Fprintln(os.Stderr, "  → Choose explicitly: `gitmake .` or `gitmake path/to/Project.zip`.")
	case "GH_AUTH_REQUIRED":
		fmt.Fprintln(os.Stderr, "  → Run `gh auth login`, then retry GitMake.")
	case "GIT_LFS_REQUIRED":
		fmt.Fprintln(os.Stderr, "  → Install Git LFS, run `git lfs install`, then retry.")
	case "PROJECT_IDENTITY_MISMATCH":
		fmt.Fprintln(os.Stderr, "  → Verify the folder and target repository. GitMake intentionally provides no automatic override here.")
	case "DESTRUCTIVE_CHANGE_BLOCKED":
		fmt.Fprintln(os.Stderr, "  → Inspect the deletion ratio and project identity before using a destructive approval.")
	}
}
