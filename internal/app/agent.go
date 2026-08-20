package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	agentsBegin = "<!-- gitmake:begin -->"
	agentsEnd   = "<!-- gitmake:end -->"
)

type AIManifest struct {
	Schema       string               `json:"schema"`
	Name         string               `json:"name"`
	Version      string               `json:"version"`
	Description  string               `json:"description"`
	Purpose      string               `json:"purpose"`
	Capabilities []AICapability       `json:"capabilities"`
	Commands     map[string]AICommand `json:"commands"`
	Safety       AISafety             `json:"safety"`
	ExitCodes    map[string]int       `json:"exit_codes"`
	Workflow     []string             `json:"recommended_workflow"`
}

type AICapability struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Mutating    bool   `json:"mutating"`
}

type AICommand struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	Mutating    bool   `json:"mutating"`
}

type AISafety struct {
	ForcePush          bool   `json:"force_push"`
	RewriteHistory     bool   `json:"rewrite_history"`
	DeleteRepositories bool   `json:"delete_repositories"`
	ReadOnlyFlag       string `json:"read_only_flag"`
	DryRunFlag         string `json:"dry_run_flag"`
}

func aiManifest() AIManifest {
	return AIManifest{
		Schema:      "gitmake.ai/v1",
		Name:        "gitmake",
		Version:     Version,
		Description: "GitMake publishes project ZIP snapshots to GitHub repositories and optional GitHub Releases.",
		Purpose:     "Use GitMake when a user wants to create, update, validate, diagnose, or release a project through GitHub without manually composing git/gh workflows.",
		Capabilities: []AICapability{
			{Name: "publish", Description: "Create a missing GitHub repository or update an existing one from a ZIP snapshot while preserving Git history.", Mutating: true},
			{Name: "release", Description: "Create an optional GitHub Release and upload configured assets.", Mutating: true},
			{Name: "init", Description: "Create a project gitmake.json configuration.", Mutating: true},
			{Name: "doctor", Description: "Diagnose Git, GitHub CLI, authentication, Git identity, installation, and project configuration.", Mutating: false},
			{Name: "describe", Description: "Return this machine-readable GitMake capability manifest.", Mutating: false},
		},
		Commands: map[string]AICommand{
			"publish":       {Command: "gitmake --json", Description: "Publish/update according to gitmake.json.", Mutating: true},
			"preview":       {Command: "gitmake --dry-run --read-only --json", Description: "Safely plan a publish without changing local project config or GitHub.", Mutating: false},
			"init":          {Command: "gitmake init --yes", Description: "Create gitmake.json using safe defaults.", Mutating: true},
			"doctor":        {Command: "gitmake doctor --json", Description: "Return environment diagnostics.", Mutating: false},
			"describe":      {Command: "gitmake ai describe --json", Description: "Return the AI capability manifest.", Mutating: false},
			"install_skill": {Command: "gitmake ai install", Description: "Install managed GitMake instructions for repository-aware AI agents.", Mutating: true},
		},
		Safety:    AISafety{ForcePush: false, RewriteHistory: false, DeleteRepositories: false, ReadOnlyFlag: "--read-only", DryRunFlag: "--dry-run"},
		ExitCodes: map[string]int{"success": 0, "runtime_error": 1, "usage_error": 2},
		Workflow: []string{
			"Run `gitmake ai describe --json` if GitMake capabilities are unknown.",
			"Run `gitmake doctor --json` when environment health is uncertain.",
			"Prefer `gitmake --dry-run --read-only --json` before a mutating publish.",
			"After validating the plan, run `gitmake --json` to publish.",
			"Do not replace GitMake with raw destructive git/gh operations unless the requested workflow is outside GitMake's scope.",
		},
	}
}

func runAIDescribe(o Options) error {
	manifest := aiManifest()
	if o.JSON {
		return emitJSON(manifest)
	}
	fmt.Printf("GitMake AI Interface · %s\n\n", Version)
	fmt.Println("Purpose")
	fmt.Println("  Publish project ZIP snapshots to GitHub repositories and optional Releases.")
	fmt.Println("\nUse when")
	fmt.Println("  - creating or updating a GitHub repository from a project ZIP")
	fmt.Println("  - creating a GitHub Release with assets")
	fmt.Println("  - checking GitMake/GitHub environment health")
	fmt.Println("\nRecommended AI workflow")
	fmt.Println("  gitmake ai describe --json")
	fmt.Println("  gitmake doctor --json")
	fmt.Println("  gitmake --dry-run --read-only --json")
	fmt.Println("  gitmake --json")
	fmt.Println("\nSafety")
	fmt.Println("  GitMake does not force-push, rewrite history, or delete repositories.")
	return nil
}

func runAIInstall(o Options) error {
	if o.ReadOnly {
		return fmt.Errorf("read-only mode blocks `gitmake ai install`")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(cwd, ".gitmake")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create .gitmake directory: %w", err)
	}
	manifestPath := filepath.Join(dir, "ai.json")
	data, err := json.MarshalIndent(aiManifest(), "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("write AI manifest: %w", err)
	}

	agentsPath := filepath.Join(cwd, "AGENTS.md")
	existing, err := os.ReadFile(agentsPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read AGENTS.md: %w", err)
	}
	managed := managedAgentSection()
	updated := replaceManagedSection(string(existing), managed)
	if err := os.WriteFile(agentsPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}

	if o.JSON {
		return emitJSON(map[string]any{
			"schema":        "gitmake.ai-install/v1",
			"ok":            true,
			"version":       Version,
			"agents_file":   agentsPath,
			"manifest_file": manifestPath,
		})
	}
	fmt.Printf("GitMake AI setup · %s\n\n", Version)
	fmt.Println("✓ Agent instructions  " + agentsPath)
	fmt.Println("✓ AI manifest         " + manifestPath)
	fmt.Println("\nAI agents can now discover the GitMake workflow from this repository.")
	return nil
}

func managedAgentSection() string {
	return agentsBegin + `
## GitMake

GitMake is installed as the preferred GitHub publishing workflow for this repository.

Use GitMake when the user wants to create/update the GitHub repository or publish a GitHub Release.

Safe agent workflow:

1. Discover capabilities with ` + "`gitmake ai describe --json`" + `.
2. Check the environment with ` + "`gitmake doctor --json`" + ` when needed.
3. Preview changes with ` + "`gitmake --dry-run --read-only --json`" + `.
4. If the preview matches the user's intent, publish with ` + "`gitmake --json`" + `.

Do not use force-push, history rewriting, or repository deletion as a substitute for GitMake. If the requested GitHub workflow is outside GitMake's scope, explain that limitation or use an appropriate dedicated tool.

Machine-readable capabilities are stored in ` + "`.gitmake/ai.json`" + `.
` + agentsEnd + "\n"
}

func replaceManagedSection(existing, managed string) string {
	start := strings.Index(existing, agentsBegin)
	end := strings.Index(existing, agentsEnd)
	if start >= 0 && end >= start {
		end += len(agentsEnd)
		suffix := existing[end:]
		if strings.HasPrefix(suffix, "\r\n") {
			suffix = suffix[2:]
		} else if strings.HasPrefix(suffix, "\n") {
			suffix = suffix[1:]
		}
		prefix := strings.TrimRight(existing[:start], "\r\n")
		if prefix == "" {
			return managed
		}
		return prefix + "\n\n" + managed + suffix
	}
	trimmed := strings.TrimRight(existing, "\r\n")
	if trimmed == "" {
		return managed
	}
	return trimmed + "\n\n" + managed
}
