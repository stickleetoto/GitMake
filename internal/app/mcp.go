package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gitmake/internal/approval"
	"gitmake/internal/planstore"
)

const (
	mcpProtocolLegacy = "2025-11-25"
	mcpProtocolModern = "2026-07-28"
	mcpMaxMessage     = 8 << 20
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type mcpToolCallParams struct {
	Name           string                      `json:"name"`
	Arguments      map[string]any              `json:"arguments"`
	InputResponses map[string]mcpInputResponse `json:"inputResponses,omitempty"`
	RequestState   string                      `json:"requestState,omitempty"`
	Meta           map[string]any              `json:"_meta,omitempty"`
}

func runMCP(o Options) error {
	// MCP stdio owns stdout. Never print normal CLI UI from this function.
	server := &mcpServer{allowWrite: o.MCPAllowWrite}
	return server.serve(os.Stdin, os.Stdout)
}

type mcpServer struct {
	allowWrite        bool
	legacyProtocol    string
	legacyElicitation bool
	nextServerRequest uint64
	stateSecret       []byte
}

func (s *mcpServer) serve(in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 64*1024), mcpMaxMessage)
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = enc.Encode(mcpResponse{JSONRPC: "2.0", ID: nil, Error: &mcpError{Code: -32700, Message: "Parse error"}})
			continue
		}
		// Notifications have no id and receive no response.
		isNotification := req.ID == nil

		// Legacy (2025-11-25) stdio clients support elicitation as a
		// server-to-client request while the original tool call is pending.
		// Handle that flow here because it temporarily owns the input stream.
		if !isNotification && req.Method == "tools/call" && s.legacyProtocol == mcpProtocolLegacy && s.legacyElicitation {
			var p mcpToolCallParams
			if json.Unmarshal(req.Params, &p) == nil {
				if p.Name == "gitmake_publish" && s.allowWrite {
					resp, err := s.legacyPublishWithElicitation(req, p, scanner, enc)
					if err != nil {
						return err
					}
					if err := enc.Encode(resp); err != nil {
						return fmt.Errorf("write MCP response: %w", err)
					}
					continue
				}
				if _, ok := s.legacyShouldElicit(p); ok {
					resp, err := s.legacyApplyWithElicitation(req, p, scanner, enc)
					if err != nil {
						return err
					}
					if err := enc.Encode(resp); err != nil {
						return fmt.Errorf("write MCP response: %w", err)
					}
					continue
				}
			}
		}

		resp, respond := s.handle(req)
		if !respond || isNotification {
			continue
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("write MCP response: %w", err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read MCP stdio: %w", err)
	}
	return nil
}

func (s *mcpServer) handle(req mcpRequest) (mcpResponse, bool) {
	base := mcpResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		// Legacy MCP compatibility. Claude Code currently supports form
		// elicitation on stdio, so remember that client capability for the
		// lifetime of this legacy session. Modern 2026 clients declare it
		// independently on every request instead.
		var p struct {
			ProtocolVersion string         `json:"protocolVersion"`
			Capabilities    map[string]any `json:"capabilities"`
		}
		_ = json.Unmarshal(req.Params, &p)
		protocol := p.ProtocolVersion
		if protocol == "" {
			protocol = mcpProtocolLegacy
		}
		s.legacyProtocol = protocol
		_, hasElicitation := p.Capabilities["elicitation"]
		s.legacyElicitation = protocol == mcpProtocolLegacy && hasElicitation
		base.Result = map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "gitmake", "version": Version},
			"instructions":    "For normal publish/upload/create/update requests, use gitmake_publish first. It orchestrates prepare, client-controlled human approval, apply, and final result in one interactive MCP operation. Use gitmake_prepare only when the user explicitly wants a plan without publishing. Terminal `gitmake approve` remains the fallback for clients without elicitation.",
		}
		return base, true
	case "server/discover":
		base.Result = map[string]any{
			"resultType":        "complete",
			"supportedVersions": []string{mcpProtocolModern, mcpProtocolLegacy},
			"capabilities":      map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":        map[string]any{"name": "gitmake", "version": Version},
			"instructions":      "Use gitmake_publish as the primary publishing entry point. It keeps planning, human approval, and apply inside one interactive MCP operation. Use gitmake_prepare when the user explicitly asks for plan-only review. Terminal approval remains a compatibility fallback.",
		}
		return base, true
	case "notifications/initialized", "notifications/cancelled":
		return base, false
	case "ping":
		base.Result = map[string]any{}
		if s.isModernRequest(req.Params) {
			base.Result.(map[string]any)["resultType"] = "complete"
		}
		return base, true
	case "tools/list":
		result := map[string]any{"tools": s.tools()}
		if s.isModernRequest(req.Params) {
			result["resultType"] = "complete"
		}
		base.Result = result
		return base, true
	case "tools/call":
		var p mcpToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			base.Error = &mcpError{Code: -32602, Message: "Invalid params", Data: err.Error()}
			return base, true
		}
		modern := s.isModernRequest(req.Params)
		if modern && p.Name == "gitmake_publish" && (s.requestSupportsElicitation(req.Params) || len(p.InputResponses) > 0) {
			result, inputRequired, handled, err := s.modernPublishResult(p)
			if handled {
				if err != nil {
					resp := s.toolErrorResponse(base, err, true)
					return resp, true
				}
				if inputRequired != nil {
					base.Result = inputRequired
					return base, true
				}
				resp := s.toolSuccessResponse(base, result, true)
				return resp, true
			}
		}
		if modern && p.Name == "gitmake_apply" && (s.requestSupportsElicitation(req.Params) || len(p.InputResponses) > 0) {
			result, inputRequired, handled, err := s.modernApplyResult(p)
			if handled {
				if err != nil {
					resp := s.toolErrorResponse(base, err, true)
					return resp, true
				}
				if inputRequired != nil {
					base.Result = inputRequired
					return base, true
				}
				resp := s.toolSuccessResponse(base, result, true)
				return resp, true
			}
		}
		result, err := s.callTool(p.Name, p.Arguments)
		if err != nil {
			resp := s.toolErrorResponse(base, err, modern)
			return resp, true
		}
		resp := s.toolSuccessResponse(base, result, modern)
		return resp, true
	default:
		base.Error = &mcpError{Code: -32601, Message: "Method not found", Data: req.Method}
		return base, true
	}
}

func (s *mcpServer) tools() []mcpTool {
	projectDir := map[string]any{"type": "string", "description": "Project directory. Defaults to the MCP server working directory."}
	sourceArg := map[string]any{"type": "string", "description": "Optional source path (project folder or ZIP) relative to project_dir or absolute."}
	zipArg := map[string]any{"type": "string", "description": "Deprecated alias for source_path when the source is a ZIP."}
	tools := []mcpTool{
		{Name: "gitmake_describe", Description: "Return GitMake's machine-readable AI capability manifest.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_prepare", Description: "PLAN-ONLY ENTRY POINT. High-level folder-or-ZIP to reviewed-plan workflow. Use this when the user explicitly wants to inspect or prepare a plan without publishing. For actual upload/publish/create/update requests, prefer gitmake_publish so GitMake can keep planning, human approval, apply, and completion in one interactive MCP operation. Do NOT create or edit gitmake.json with host filesystem Write/Edit tools when this tool is available.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_path": sourceArg, "source_zip": zipArg, "persist_config": map[string]any{"type": "boolean", "description": "Persist an inferred missing gitmake.json through GitMake's validated atomic writer. Defaults to false. Set true only when the user explicitly wants a persistent advanced gitmake.json."}})},
		{Name: "gitmake_project_inspect", Description: "Inspect project config and ZIP discovery state without shell commands or project mutation.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_doctor", Description: "Diagnose Git, GitHub CLI, authentication, identity, installation, and project config without mutating the project.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_discover", Description: "Classify ZIP archives conservatively and identify likely project source/release assets without changing files.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_config_suggest", Description: "Build a validated gitmake.json candidate in memory from project discovery. Does not write files.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_path": sourceArg, "source_zip": zipArg, "repo_name": map[string]any{"type": "string"}, "visibility": map[string]any{"type": "string", "enum": []string{"private", "public", "internal"}}, "branch": map[string]any{"type": "string"}})},
		{Name: "gitmake_config_schema", Description: "Return the authoritative JSON Schema for gitmake.json. Use this before authoring config.", InputSchema: objectSchema(nil, map[string]any{})},
		{Name: "gitmake_config_validate", Description: "Strictly validate the current gitmake.json.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_preview", Description: "Read-only dry-run of repository CREATE/UPDATE planning. Does not write config or change GitHub.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_path": sourceArg, "source_zip": zipArg})},
		{Name: "gitmake_plan", Description: "Create a reviewed publish plan with provenance, project identity, risk classification, and source/config/remote hashes. Does not publish. Always surface working_directory, config_path, source_path, repository, changes, and risk to the user before approval.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_path": sourceArg, "source_zip": zipArg})},
		{Name: "gitmake_history", Description: "Read recent GitMake operation history.", InputSchema: objectSchema(nil, map[string]any{})},
	}
	if s.allowWrite {
		configObj := map[string]any{"type": "object", "description": "A complete GitMake config object that conforms to gitmake_config_schema."}
		patchObj := map[string]any{"type": "object", "description": "RFC 7396-style object merge patch for the existing config."}
		tools = append(tools,
			mcpTool{Name: "gitmake_publish", Description: "PRIMARY PUBLISHING TOOL. When the user asks to upload, publish, create, update, or release a GitHub project, call this tool first. It performs prepare -> reviewed plan -> client-controlled human approval -> exact-plan revalidation -> apply -> final result in ONE interactive MCP operation. Do NOT stop the conversation merely to ask the user for approval; GitMake will request approval through the MCP client UI. If the client cannot provide MCP elicitation, use gitmake_prepare + terminal `gitmake approve` + gitmake_apply as the fallback. Never bypass GitMake with raw git/gh/GitHub API.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_path": sourceArg, "source_zip": zipArg, "persist_config": map[string]any{"type": "boolean", "description": "Persist an inferred missing gitmake.json before publishing. Defaults to false."}})},
			mcpTool{Name: "gitmake_config_write", Description: "Validate and atomically write gitmake.json. Prefer this over direct file editing.", InputSchema: objectSchema([]string{"config"}, map[string]any{"project_dir": projectDir, "config": configObj})},
			mcpTool{Name: "gitmake_config_patch", Description: "Validate and atomically patch an existing gitmake.json.", InputSchema: objectSchema([]string{"patch"}, map[string]any{"project_dir": projectDir, "patch": patchObj})},
			mcpTool{Name: "gitmake_apply", Description: "Apply one reviewed plan. If the connected MCP client supports elicitation, GitMake requests human approval inside the client UI and only proceeds after the client returns an accepted human response. Otherwise the stable fallback is local `gitmake approve`. GitMake revalidates the exact plan binding and keeps approval single-use. A legacy approval_token is accepted only for pre-1.0 compatibility.", InputSchema: objectSchema([]string{"plan_id"}, map[string]any{"project_dir": projectDir, "plan_id": map[string]any{"type": "string", "pattern": "^gm_[A-Za-z0-9]+$"}, "approval_token": map[string]any{"type": "string", "description": "Deprecated pre-1.0 compatibility token.", "pattern": "^gma_[A-Fa-f0-9]+$"}})},
		)
	}
	return tools
}

func objectSchema(required []string, props map[string]any) map[string]any {
	s := map[string]any{"type": "object", "additionalProperties": false, "properties": props}
	if len(required) > 0 {
		s["required"] = required
	}
	return s
}

func (s *mcpServer) callTool(name string, args map[string]any) (any, error) {
	if args == nil {
		args = map[string]any{}
	}
	projectDir, err := stringArg(args, "project_dir", false)
	if err != nil {
		return nil, err
	}
	sourcePath, err := stringArg(args, "source_path", false)
	if err != nil {
		return nil, err
	}
	legacyZIP, err := stringArg(args, "source_zip", false)
	if err != nil {
		return nil, err
	}
	if sourcePath == "" {
		sourcePath = legacyZIP
	} else if legacyZIP != "" && sourcePath != legacyZIP {
		return nil, fmt.Errorf("source_path and source_zip disagree; provide only source_path")
	}

	switch name {
	case "gitmake_describe":
		return invokeGitMakeJSON(projectDir, nil, "ai", "describe", "--json")
	case "gitmake_prepare":
		persist, err := boolArg(args, "persist_config", false)
		if err != nil {
			return nil, err
		}
		if persist && !s.allowWrite {
			return nil, fmt.Errorf("persist_config requires MCP write access; run `gitmake ai setup --write` or call gitmake_prepare without persistence")
		}
		return s.prepareProject(projectDir, sourcePath, persist)
	case "gitmake_project_inspect":
		return invokeGitMakeJSON(projectDir, nil, "inspect", "--json")
	case "gitmake_doctor":
		return invokeGitMakeJSON(projectDir, nil, "doctor", "--json")
	case "gitmake_discover":
		return invokeGitMakeJSON(projectDir, nil, "discover", "--json")
	case "gitmake_config_suggest":
		repoName, err := stringArg(args, "repo_name", false)
		if err != nil {
			return nil, err
		}
		visibility, err := stringArg(args, "visibility", false)
		if err != nil {
			return nil, err
		}
		branch, err := stringArg(args, "branch", false)
		if err != nil {
			return nil, err
		}
		dir := effectiveMCPProjectDir(projectDir)
		cfg, err := projectConfigForSuggestion(dir, sourcePath, repoName, visibility, branch)
		if err != nil {
			return nil, err
		}
		return map[string]any{"schema": "gitmake.config-suggestion/v1", "ok": true, "config": cfg}, nil
	case "gitmake_config_schema":
		return invokeGitMakeJSON(projectDir, nil, "config", "schema", "--json")
	case "gitmake_config_validate":
		return invokeGitMakeJSON(projectDir, nil, "config", "validate", "--json")
	case "gitmake_preview":
		cli := []string{"--dry-run", "--read-only", "--json"}
		if sourcePath != "" {
			cli = append(cli, sourcePath)
		}
		return invokeGitMakeJSON(projectDir, nil, cli...)
	case "gitmake_plan":
		cli := []string{"plan", "--json"}
		if sourcePath != "" {
			cli = append(cli, sourcePath)
		}
		return invokeGitMakeJSON(projectDir, nil, cli...)
	case "gitmake_history":
		return invokeGitMakeJSON(projectDir, nil, "history", "--json")
	case "gitmake_config_write":
		if !s.allowWrite {
			return nil, fmt.Errorf("MCP write tools are disabled; restart with `gitmake mcp --allow-write`")
		}
		cfg, ok := args["config"]
		if !ok {
			return nil, fmt.Errorf("config is required")
		}
		b, err := json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("encode config: %w", err)
		}
		return invokeGitMakeJSON(projectDir, b, "config", "write", "--stdin", "--json")
	case "gitmake_config_patch":
		if !s.allowWrite {
			return nil, fmt.Errorf("MCP write tools are disabled; restart with `gitmake mcp --allow-write`")
		}
		patch, ok := args["patch"]
		if !ok {
			return nil, fmt.Errorf("patch is required")
		}
		b, err := json.Marshal(patch)
		if err != nil {
			return nil, fmt.Errorf("encode patch: %w", err)
		}
		return invokeGitMakeJSON(projectDir, b, "config", "patch", "--stdin", "--json")
	case "gitmake_apply":
		if !s.allowWrite {
			return nil, fmt.Errorf("MCP write tools are disabled; restart with `gitmake mcp --allow-write`")
		}
		planID, err := stringArg(args, "plan_id", true)
		if err != nil {
			return nil, err
		}
		plan, _, err := planstore.Load(planID)
		if err != nil {
			return nil, err
		}
		legacyToken, err := stringArg(args, "approval_token", false)
		if err != nil {
			return nil, err
		}
		var destructive bool
		if legacyToken != "" {
			record, legacyErr := approval.ValidateRecord(planID, legacyToken)
			if legacyErr != nil {
				return nil, fmt.Errorf("human approval rejected: %w", legacyErr)
			}
			destructive = record.Destructive
		} else {
			record, grantErr := approval.ValidateGrant(planID, approvalBindingFromPlan(plan))
			if grantErr != nil {
				return nil, fmt.Errorf("human approval required: %w", grantErr)
			}
			destructive = record.Destructive
		}
		if plan.Risk.Destructive && !destructive {
			return nil, fmt.Errorf("human approval rejected: plan %s is destructive and requires `gitmake approve --destructive` in a terminal", planID)
		}
		cli := []string{"apply", planID, "--json"}
		if destructive {
			cli = append(cli, "--destructive")
		}
		result, err := invokeGitMakeJSON(projectDir, nil, cli...)
		if err != nil {
			return result, err
		}
		if legacyToken != "" {
			if err := approval.Consume(planID, legacyToken); err != nil {
				return result, fmt.Errorf("apply succeeded but legacy approval could not be consumed: %w", err)
			}
		} else if err := approval.ConsumeGrant(planID, approvalBindingFromPlan(plan)); err != nil {
			return result, fmt.Errorf("apply succeeded but local approval could not be consumed: %w", err)
		}
		return result, nil
	case "gitmake_publish":
		if !s.allowWrite {
			return nil, fmt.Errorf("MCP write tools are disabled; restart with `gitmake mcp --allow-write`")
		}
		return nil, fmt.Errorf("gitmake_publish requires an MCP client with interactive elicitation; use gitmake_prepare, then `gitmake approve`, then gitmake_apply as the compatibility fallback")
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func boolArg(args map[string]any, name string, defaultValue bool) (bool, error) {
	v, ok := args[name]
	if !ok || v == nil {
		return defaultValue, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", name)
	}
	return b, nil
}

func (s *mcpServer) prepareProject(projectDir, sourcePath string, persistConfig bool) (any, error) {
	dir := effectiveMCPProjectDir(projectDir)
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve project directory: %w", err)
	}
	configPath := filepath.Join(absDir, "gitmake.json")
	_, statErr := os.Stat(configPath)
	configPresent := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("check config: %w", statErr)
	}

	var candidate any
	configAuthored := false
	configPersisted := configPresent
	if configPresent {
		if _, err := invokeGitMakeJSON(absDir, nil, "config", "validate", "--json"); err != nil {
			return nil, fmt.Errorf("existing gitmake.json is invalid: %w", err)
		}
	} else {
		cfg, err := projectConfigForSuggestion(absDir, sourcePath, "", "", "")
		if err != nil {
			return nil, err
		}
		b, err := json.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("encode inferred config: %w", err)
		}
		if err := json.Unmarshal(b, &candidate); err != nil {
			return nil, fmt.Errorf("decode inferred config: %w", err)
		}
		if persistConfig {
			if _, err := invokeGitMakeJSON(absDir, b, "config", "write", "--stdin", "--json"); err != nil {
				return nil, fmt.Errorf("persist inferred config: %w", err)
			}
			configAuthored = true
			configPersisted = true
			if _, err := invokeGitMakeJSON(absDir, nil, "config", "validate", "--json"); err != nil {
				return nil, fmt.Errorf("validate persisted config: %w", err)
			}
		}
	}

	planArgs := []string{"plan", "--json"}
	if sourcePath != "" {
		planArgs = append(planArgs, sourcePath)
	}
	planResult, err := invokeGitMakeJSON(absDir, nil, planArgs...)
	if err != nil {
		return planResult, err
	}

	planMap, _ := planResult.(map[string]any)
	status := "ready_for_approval"
	if planMap == nil {
		status = "prepared"
	}
	configMode := "existing"
	if !configPresent && !configPersisted {
		configMode = "in_memory"
	} else if configAuthored {
		configMode = "gitmake_authored"
	}
	result := map[string]any{
		"schema":  "gitmake.prepare/v1",
		"ok":      true,
		"version": Version,
		"status":  status,
		"access": map[string]any{
			"mcp_write_enabled":      s.allowWrite,
			"project_config_mutated": configAuthored,
			"github_mutated":         false,
		},
		"config": map[string]any{
			"path":           configPath,
			"present_before": configPresent,
			"persisted":      configPersisted,
			"mode":           configMode,
			"validated":      true,
		},
		"plan":        planResult,
		"next_action": "Show the reviewed plan provenance, changes, and risk to the user. If the user asked to publish, call gitmake_apply: on clients with MCP elicitation support GitMake will open a client-controlled human approval dialog before any mutation. Never answer that dialog on the user's behalf. If elicitation is unavailable, fall back to asking the human to run `gitmake approve`.",
	}
	if candidate != nil {
		result["inferred_config"] = candidate
	}
	if !configPersisted {
		result["note"] = "gitmake.json was intentionally kept in memory because GitMake is zero-config by default. The reviewed plan remains usable without persisting config. Use GitMake config tools only when advanced persistent settings are actually needed."
	}
	return result, nil
}

func stringArg(args map[string]any, name string, required bool) (string, error) {
	v, ok := args[name]
	if !ok || v == nil {
		if required {
			return "", fmt.Errorf("%s is required", name)
		}
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", name)
	}
	s = strings.TrimSpace(s)
	if required && s == "" {
		return "", fmt.Errorf("%s cannot be empty", name)
	}
	return s, nil
}

func effectiveMCPProjectDir(projectDir string) string {
	if strings.TrimSpace(projectDir) != "" {
		return projectDir
	}
	if v := strings.TrimSpace(os.Getenv("GITMAKE_PROJECT_DIR")); v != "" {
		return v
	}
	if v := strings.TrimSpace(os.Getenv("CLAUDE_PROJECT_DIR")); v != "" {
		return v
	}
	cwd, _ := os.Getwd()
	return cwd
}

func invokeGitMakeJSON(projectDir string, stdin []byte, args ...string) (any, error) {
	if strings.TrimSpace(projectDir) == "" {
		// GitMake-specific override works with any MCP client. Claude Code also
		// supplies CLAUDE_PROJECT_DIR to local stdio servers.
		projectDir = strings.TrimSpace(os.Getenv("GITMAKE_PROJECT_DIR"))
		if projectDir == "" {
			projectDir = strings.TrimSpace(os.Getenv("CLAUDE_PROJECT_DIR"))
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if projectDir != "" {
		if !filepath.IsAbs(projectDir) {
			projectDir = filepath.Join(cwd, projectDir)
		}
		if err := os.Chdir(projectDir); err != nil {
			return nil, fmt.Errorf("project_dir: %w", err)
		}
		defer os.Chdir(cwd)
	}

	oldOut, oldErr, oldIn := os.Stdout, os.Stderr, os.Stdin
	outFile, err := os.CreateTemp("", "gitmake-mcp-out-*.json")
	if err != nil {
		return nil, err
	}
	defer os.Remove(outFile.Name())
	defer outFile.Close()
	errFile, err := os.CreateTemp("", "gitmake-mcp-err-*.txt")
	if err != nil {
		return nil, err
	}
	defer os.Remove(errFile.Name())
	defer errFile.Close()

	var inFile *os.File
	if stdin != nil {
		inFile, err = os.CreateTemp("", "gitmake-mcp-in-*.json")
		if err != nil {
			return nil, err
		}
		defer os.Remove(inFile.Name())
		defer inFile.Close()
		if _, err := inFile.Write(stdin); err != nil {
			return nil, err
		}
		if _, err := inFile.Seek(0, 0); err != nil {
			return nil, err
		}
		os.Stdin = inFile
	}
	os.Stdout, os.Stderr = outFile, errFile
	code := Main(args)
	os.Stdout, os.Stderr, os.Stdin = oldOut, oldErr, oldIn

	if _, err := outFile.Seek(0, 0); err != nil {
		return nil, err
	}
	out, _ := io.ReadAll(outFile)
	if _, err := errFile.Seek(0, 0); err != nil {
		return nil, err
	}
	errText, _ := io.ReadAll(errFile)

	var decoded any
	if len(bytes.TrimSpace(out)) > 0 {
		if err := json.Unmarshal(out, &decoded); err != nil {
			return nil, fmt.Errorf("GitMake returned non-JSON output (exit %d): %s %s", code, strings.TrimSpace(string(out)), strings.TrimSpace(string(errText)))
		}
	} else {
		decoded = map[string]any{"schema": "gitmake.mcp-cli/v1", "ok": code == 0, "exit_code": code}
	}
	if code != 0 {
		return decoded, fmt.Errorf("GitMake command failed with exit code %d", code)
	}
	return decoded, nil
}

func mustJSONString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Keep modern protocol version reachable from binary scanners/docs even though
// modern clients can optimistically call tools/list without a handshake.
var _ = mcpProtocolModern
