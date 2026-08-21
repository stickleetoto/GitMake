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
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

func runMCP(o Options) error {
	// MCP stdio owns stdout. Never print normal CLI UI from this function.
	server := &mcpServer{allowWrite: o.MCPAllowWrite}
	return server.serve(os.Stdin, os.Stdout)
}

type mcpServer struct {
	allowWrite bool
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
		// Legacy MCP compatibility. Modern 2026 clients can call tools/list directly.
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		protocol := p.ProtocolVersion
		if protocol == "" {
			protocol = mcpProtocolLegacy
		}
		base.Result = map[string]any{
			"protocolVersion": protocol,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "gitmake", "version": Version},
		}
		return base, true
	case "notifications/initialized", "notifications/cancelled":
		return base, false
	case "ping":
		base.Result = map[string]any{}
		return base, true
	case "tools/list":
		base.Result = map[string]any{"tools": s.tools()}
		return base, true
	case "tools/call":
		var p mcpToolCallParams
		if err := json.Unmarshal(req.Params, &p); err != nil {
			base.Error = &mcpError{Code: -32602, Message: "Invalid params", Data: err.Error()}
			return base, true
		}
		result, err := s.callTool(p.Name, p.Arguments)
		if err != nil {
			base.Result = map[string]any{
				"content": []map[string]any{{"type": "text", "text": mustJSONString(map[string]any{"ok": false, "error": err.Error()})}},
				"isError": true,
			}
			return base, true
		}
		base.Result = map[string]any{
			"content":           []map[string]any{{"type": "text", "text": mustJSONString(result)}},
			"structuredContent": result,
			"isError":           false,
		}
		return base, true
	default:
		base.Error = &mcpError{Code: -32601, Message: "Method not found", Data: req.Method}
		return base, true
	}
}

func (s *mcpServer) tools() []mcpTool {
	projectDir := map[string]any{"type": "string", "description": "Project directory. Defaults to the MCP server working directory."}
	zipArg := map[string]any{"type": "string", "description": "Optional source ZIP path relative to project_dir or absolute."}
	tools := []mcpTool{
		{Name: "gitmake_describe", Description: "Return GitMake's machine-readable AI capability manifest.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_project_inspect", Description: "Inspect project config and ZIP discovery state without shell commands or project mutation.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_doctor", Description: "Diagnose Git, GitHub CLI, authentication, identity, installation, and project config without mutating the project.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_discover", Description: "Classify ZIP archives conservatively and identify likely project source/release assets without changing files.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_config_suggest", Description: "Build a validated gitmake.json candidate in memory from project discovery. Does not write files.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_zip": zipArg, "repo_name": map[string]any{"type": "string"}, "visibility": map[string]any{"type": "string", "enum": []string{"private", "public", "internal"}}, "branch": map[string]any{"type": "string"}})},
		{Name: "gitmake_config_schema", Description: "Return the authoritative JSON Schema for gitmake.json. Use this before authoring config.", InputSchema: objectSchema(nil, map[string]any{})},
		{Name: "gitmake_config_validate", Description: "Strictly validate the current gitmake.json.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir})},
		{Name: "gitmake_preview", Description: "Read-only dry-run of repository CREATE/UPDATE planning. Does not write config or change GitHub.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_zip": zipArg})},
		{Name: "gitmake_plan", Description: "Create a reviewed publish plan with source/config/remote hashes. Does not publish.", InputSchema: objectSchema(nil, map[string]any{"project_dir": projectDir, "source_zip": zipArg})},
		{Name: "gitmake_history", Description: "Read recent GitMake operation history.", InputSchema: objectSchema(nil, map[string]any{})},
	}
	if s.allowWrite {
		configObj := map[string]any{"type": "object", "description": "A complete GitMake config object that conforms to gitmake_config_schema."}
		patchObj := map[string]any{"type": "object", "description": "RFC 7396-style object merge patch for the existing config."}
		tools = append(tools,
			mcpTool{Name: "gitmake_config_write", Description: "Validate and atomically write gitmake.json. Prefer this over direct file editing.", InputSchema: objectSchema([]string{"config"}, map[string]any{"project_dir": projectDir, "config": configObj})},
			mcpTool{Name: "gitmake_config_patch", Description: "Validate and atomically patch an existing gitmake.json.", InputSchema: objectSchema([]string{"patch"}, map[string]any{"project_dir": projectDir, "patch": patchObj})},
			mcpTool{Name: "gitmake_apply", Description: "Apply a reviewed plan using a user-created one-shot approval token. Revalidates source/config/remote state and consumes the token only after success.", InputSchema: objectSchema([]string{"plan_id", "approval_token"}, map[string]any{"project_dir": projectDir, "plan_id": map[string]any{"type": "string", "pattern": "^gm_[A-Za-z0-9]+$"}, "approval_token": map[string]any{"type": "string", "pattern": "^gma_[A-Fa-f0-9]+$"}})},
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
	sourceZIP, err := stringArg(args, "source_zip", false)
	if err != nil {
		return nil, err
	}

	switch name {
	case "gitmake_describe":
		return invokeGitMakeJSON(projectDir, nil, "ai", "describe", "--json")
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
		cfg, err := projectConfigForSuggestion(dir, sourceZIP, repoName, visibility, branch)
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
		if sourceZIP != "" {
			cli = append(cli, sourceZIP)
		}
		return invokeGitMakeJSON(projectDir, nil, cli...)
	case "gitmake_plan":
		cli := []string{"plan", "--json"}
		if sourceZIP != "" {
			cli = append(cli, sourceZIP)
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
		token, err := stringArg(args, "approval_token", true)
		if err != nil {
			return nil, err
		}
		if err := approval.Validate(planID, token); err != nil {
			return nil, fmt.Errorf("one-shot approval rejected: %w", err)
		}
		result, err := invokeGitMakeJSON(projectDir, nil, "apply", planID, "--json")
		if err != nil {
			return result, err
		}
		if err := approval.Consume(planID, token); err != nil {
			return result, fmt.Errorf("apply succeeded but approval token could not be consumed: %w", err)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
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
