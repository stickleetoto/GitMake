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
		Description: "GitMake publishes project folders or ZIP snapshots to GitHub repositories and optional GitHub Releases with zero-config safe defaults.",
		Purpose:     "Use GitMake when a user wants to create, update, validate, diagnose, or release a project through GitHub without manually composing git/gh workflows.",
		Capabilities: []AICapability{
			{Name: "publish", Description: "Create or update a GitHub repository from a project folder or ZIP snapshot. In MCP write mode, prefer gitmake_publish so prepare, human approval, apply, and final reporting stay in one interactive tool operation.", Mutating: true},
			{Name: "release", Description: "Create an optional GitHub Release and upload configured assets.", Mutating: true},
			{Name: "init", Description: "Persist an optional advanced gitmake.json configuration; normal publishing is zero-config by default.", Mutating: true},
			{Name: "doctor", Description: "Diagnose Git, GitHub CLI, authentication, Git identity, installation, and project configuration.", Mutating: false},
			{Name: "describe", Description: "Return this machine-readable GitMake capability manifest.", Mutating: false},
			{Name: "discover", Description: "Classify multiple ZIP files and identify the likely project source versus release assets without modifying the project.", Mutating: false},
			{Name: "prepare", Description: "Plan-only MCP workflow for project folders or ZIP snapshots: infer source mode, infer/validate config, snapshot safely, run security/preflight/identity checks, and create a reviewed plan without publishing.", Mutating: false},
			{Name: "publish_orchestrator", Description: "Primary MCP publishing workflow: prepare, create an immutable reviewed plan, request client-controlled human approval, revalidate the exact plan, apply, and return the final result without ending the tool operation between planning and approval.", Mutating: true},
			{Name: "config_authoring", Description: "Expose the authoritative config schema and validate, write, or patch gitmake.json for agents without guessing fields.", Mutating: true},
			{Name: "plan_apply", Description: "Create a reviewed immutable publish plan with provenance, project identity, and deletion risk. Apply only if source/config/remote state still match. MCP apply requires human approval. In clients that support MCP elicitation (including Claude Code), GitMake asks the user inside the client UI; otherwise the stable fallback is a short-lived local grant created by `gitmake approve`. Destructive plans require stronger confirmation.", Mutating: true},
			{Name: "safety_gate", Description: "Scan for likely secrets, unsafe large files, protected branches, tag conflicts, project identity mismatches, stale source retargeting, and destructive mass deletions while preserving remote-only paths through managed sync.", Mutating: false},
			{Name: "history", Description: "Read recent GitMake publish/apply audit records.", Mutating: false},
			{Name: "mcp", Description: "Expose GitMake as a local MCP tool server. Read-only tools are the default; mutating tools require --allow-write.", Mutating: false},
			{Name: "ai_setup", Description: "Register GitMake MCP with Claude Code or export a generic stdio descriptor for other MCP clients. Read-only access is the default.", Mutating: true},
		},
		Commands: map[string]AICommand{
			"publish":          {Command: "gitmake --json", Description: "Publish/update using an existing config or zero-config inferred settings and project memory.", Mutating: true},
			"mcp_publish":      {Command: "MCP tool: gitmake_publish", Description: "Primary AI publishing entry point. Keeps prepare + reviewed plan + human MCP approval + exact-plan apply in one interactive operation. Do not stop the chat merely to ask for approval when this tool is available.", Mutating: true},
			"preview":          {Command: "gitmake --dry-run --read-only --json", Description: "Safely plan a publish without changing local project config or GitHub.", Mutating: false},
			"mcp_prepare":      {Command: "MCP tool: gitmake_prepare", Description: "Plan-only folder-or-ZIP to reviewed-plan workflow. Use when the user explicitly asks not to publish yet.", Mutating: false},
			"init":             {Command: "gitmake init --yes", Description: "Persist gitmake.json only when advanced stable settings are desired.", Mutating: true},
			"doctor":           {Command: "gitmake doctor --json", Description: "Return environment diagnostics.", Mutating: false},
			"discover":         {Command: "gitmake discover --json", Description: "Classify ZIPs and resolve or report source ambiguity without writing files.", Mutating: false},
			"describe":         {Command: "gitmake ai describe --json", Description: "Return the AI capability manifest.", Mutating: false},
			"install_skill":    {Command: "gitmake ai install", Description: "Install managed GitMake instructions for repository-aware AI agents.", Mutating: true},
			"config_schema":    {Command: "gitmake config schema --json", Description: "Return the authoritative JSON Schema for gitmake.json. Never guess the config format.", Mutating: false},
			"config_validate":  {Command: "gitmake config validate --json", Description: "Strictly validate the current gitmake.json and show its normalized interpretation.", Mutating: false},
			"config_write":     {Command: "gitmake config write --stdin --json", Description: "Validate and atomically write a complete config supplied on stdin.", Mutating: true},
			"config_patch":     {Command: "gitmake config patch --stdin --json", Description: "Merge a JSON object patch into an existing config, validate, then atomically write it.", Mutating: true},
			"plan":             {Command: "gitmake plan --json", Description: "Create a stored reviewed plan with working directory/config/source provenance, project identity, risk classification, source/config hashes, and remote baseline.", Mutating: false},
			"apply":            {Command: "gitmake apply <plan_id> --json", Description: "Revalidate and execute exactly a previously reviewed plan from the local CLI.", Mutating: true},
			"approve":          {Command: "gitmake approve --json", Description: "Fallback terminal approval for one MCP apply when the connected client cannot perform MCP elicitation. No approval token is shown or copied. Destructive plans require `--destructive` and stronger human confirmation.", Mutating: true},
			"inspect":          {Command: "gitmake inspect --json", Description: "Inspect config and ZIP discovery state without changing the project.", Mutating: false},
			"history":          {Command: "gitmake history --json", Description: "Read recent GitMake operation history.", Mutating: false},
			"mcp":              {Command: "gitmake mcp", Description: "Run the local MCP stdio server. Add --allow-write only when config changes/apply are intentionally permitted.", Mutating: false},
			"ai_setup":         {Command: "gitmake ai setup", Description: "Connect GitMake MCP to Claude Code at user scope with read-only tools by default.", Mutating: true},
			"ai_setup_generic": {Command: "gitmake ai setup --client generic --json", Description: "Return a standard stdio MCP descriptor for any MCP-compatible client without editing its config.", Mutating: false},
			"ai_setup_write":   {Command: "gitmake ai setup --write", Description: "Replace the Claude Code registration with reviewed GitMake write tools after explicit confirmation.", Mutating: true},
			"ai_status":        {Command: "gitmake ai status --json", Description: "Inspect Claude Code detection, MCP registration, access level, and health.", Mutating: false},
			"ai_remove":        {Command: "gitmake ai remove", Description: "Remove GitMake's user-scoped Claude Code MCP registration.", Mutating: true},
		},
		Safety:    AISafety{ForcePush: false, RewriteHistory: false, DeleteRepositories: false, ReadOnlyFlag: "--read-only", DryRunFlag: "--dry-run"},
		ExitCodes: map[string]int{"success": 0, "runtime_error": 1, "usage_error": 2},
		Workflow: []string{
			"Run `gitmake ai describe --json` if GitMake capabilities are unknown.",
			"Run `gitmake doctor --json` when environment health is uncertain.",
			"Use `gitmake discover --json` when a folder contains multiple ZIPs and source selection is uncertain.",
			"When the user asks to actually upload/publish/create/update a GitHub project and MCP write access is available, call `gitmake_publish` first. It owns prepare, reviewed plan creation, human approval, revalidation, apply, and completion in one interactive MCP operation. Do not end the conversation merely to ask for approval; GitMake requests it through the MCP client.",
			"Use `gitmake_prepare` when the user explicitly asks for plan-only review or says not to publish yet. GitMake is zero-config by default: keep inferred settings in memory unless persistent advanced configuration is explicitly requested. Do not create or edit gitmake.json with host filesystem Write/Edit tools when GitMake config/publish tools are available.",
			"Before authoring gitmake.json manually, read `gitmake config schema --json`; never guess configuration fields.",
			"When writing agent-authored config outside gitmake_prepare, use `gitmake config write --stdin --json` rather than editing gitmake.json directly, then validate it.",
			"Validate agent-authored configuration with `gitmake config validate --json`.",
			"If Claude Code is installed, `gitmake ai setup` can register GitMake MCP automatically; read-only access is the default.",
			"When MCP is available, prefer GitMake MCP tools; enable config-write tools only through explicit `gitmake ai setup --write` or manual --allow-write server configuration.",
			"Prefer `gitmake --dry-run --read-only --json` or `gitmake plan --json` before a mutating publish. Security scanning and managed-sync preservation are part of this preview.",
			"For normal MCP publishing, use `gitmake_publish`; GitMake opens the client-controlled human approval dialog and continues the same tool operation after the human responds. Never answer or auto-accept that dialog on the user's behalf. For plan-only/expert flows, `gitmake_prepare` + `gitmake_apply` remain stable. If elicitation is unavailable, ask the human to run `gitmake approve` (or `gitmake approve --destructive` for destructive plans). Never mint, fake, or bypass approval.",
			"For direct local CLI workflows, `gitmake apply <plan_id> --json` remains available after explicit user approval and rejects stale inputs. Destructive plans additionally require `--destructive`.",
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
	fmt.Println("  Publish project folders or ZIP snapshots with zero-config safe defaults and optional Releases.")
	fmt.Println("\nUse when")
	fmt.Println("  - creating or updating a GitHub repository from a project folder or ZIP")
	fmt.Println("  - creating a GitHub Release with assets")
	fmt.Println("  - checking GitMake/GitHub environment health")
	fmt.Println("\nRecommended AI workflow")
	fmt.Println("  gitmake ai describe --json")
	fmt.Println("  gitmake doctor --json")
	fmt.Println("  MCP: gitmake_publish          # normal one-shot interactive publish")
	fmt.Println("  MCP: gitmake_prepare          # plan-only review")
	fmt.Println("  gitmake discover --json        # when multiple ZIPs are present")
	fmt.Println("  gitmake config schema --json   # before authoring config")
	fmt.Println("  gitmake --dry-run --read-only --json")
	fmt.Println("  gitmake plan --json            # optional approval checkpoint")
	fmt.Println("  gitmake approve                  # local one-shot MCP approval grant")
	fmt.Println("  # destructive plans: gitmake approve --destructive")
	fmt.Println("  gitmake apply <plan_id> --json   # direct local CLI apply")
	fmt.Println("  gitmake ai setup                 # one-command Claude Code MCP registration")
	fmt.Println("  gitmake ai setup --client generic --json # any MCP client")
	fmt.Println("  gitmake mcp                     # raw read-only MCP server, for manual clients")
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
3. If multiple ZIPs are present, inspect classification with ` + "`gitmake discover --json`" + `. For a live source tree, GitMake can use ` + "source.folder" + ` (normally ` + "`.`" + `) and honors common .gitignore rules plus .gitmakeignore.
4. For a normal request to actually publish/upload/create/update a repository, call MCP ` + "`gitmake_publish`" + ` first. It performs prepare, reviewed-plan creation, client-controlled human approval, exact-plan revalidation, apply, and completion inside one interactive tool operation. Do not stop the chat just to ask for approval when this tool is available, and never answer the approval dialog on the user's behalf.
5. Use ` + "`gitmake_prepare`" + ` only when the user explicitly asks for plan-only review or says not to publish yet. If an MCP client cannot perform elicitation, fall back to ` + "`gitmake_prepare`" + ` → terminal ` + "`gitmake approve`" + ` → ` + "`gitmake_apply`" + `.
6. ` + "`gitmake.json`" + ` is optional advanced configuration. Before creating or changing it, read ` + "`gitmake config schema --json`" + `. Prefer GitMake's validated config tools over direct file editing. Never guess the config schema.
7. Treat security scan findings, branch-protection blocks, tag conflicts, source ambiguity, project-identity mismatches, stale plans, and destructive-change blocks as hard stops. Do not bypass them with raw git/gh commands.

Do not use force-push, history rewriting, or repository deletion as a substitute for GitMake. If the requested GitHub workflow is outside GitMake's scope, explain that limitation or use an appropriate dedicated tool.

Machine-readable capabilities are stored in ` + "`.gitmake/ai.json`" + `. For Claude Code, prefer ` + "`gitmake ai setup`" + ` to register the MCP server automatically. The default registration is read-only; publishing tools require explicit ` + "`gitmake ai setup --write`" + `. On elicitation-capable clients, ` + "`gitmake_publish`" + ` is the preferred one-shot publishing path. ` + "`gitmake_prepare`" + ` / ` + "`gitmake_apply`" + ` and terminal ` + "`gitmake approve`" + ` remain stable expert/fallback paths. For other MCP clients, use ` + "`gitmake ai setup --client generic --json`" + ` or ` + "`gitmake mcp`" + `.
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
