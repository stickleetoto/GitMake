package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gitmake/internal/planstore"
)

func TestMCPDefaultToolsetIsReadOnly(t *testing.T) {
	s := &mcpServer{}
	names := map[string]bool{}
	for _, tool := range s.tools() {
		names[tool.Name] = true
	}
	for _, want := range []string{"gitmake_describe", "gitmake_prepare", "gitmake_project_inspect", "gitmake_doctor", "gitmake_discover", "gitmake_config_suggest", "gitmake_config_schema", "gitmake_config_validate", "gitmake_preview", "gitmake_plan", "gitmake_history"} {
		if !names[want] {
			t.Fatalf("missing read-only tool %s", want)
		}
	}
	for _, forbidden := range []string{"gitmake_config_write", "gitmake_config_patch", "gitmake_apply"} {
		if names[forbidden] {
			t.Fatalf("mutating tool %s must not be exposed by default", forbidden)
		}
	}
}

func TestMCPPrepareIsAvailableReadOnlyAndCannotForcePersistence(t *testing.T) {
	s := &mcpServer{}
	found := false
	for _, tool := range s.tools() {
		if tool.Name == "gitmake_prepare" {
			found = true
			b, _ := json.Marshal(tool)
			if !bytes.Contains(b, []byte("Do NOT create or edit gitmake.json")) {
				t.Fatalf("prepare tool must steer agents away from host writes: %s", b)
			}
		}
	}
	if !found {
		t.Fatal("gitmake_prepare tool missing")
	}
	_, err := s.callTool("gitmake_prepare", map[string]any{"project_dir": t.TempDir(), "persist_config": true})
	if err == nil || !bytes.Contains([]byte(err.Error()), []byte("requires MCP write access")) {
		t.Fatalf("read-only prepare must reject forced persistence, got %v", err)
	}
}

func TestMCPAllowWriteAddsGatedTools(t *testing.T) {
	s := &mcpServer{allowWrite: true}
	names := map[string]bool{}
	for _, tool := range s.tools() {
		names[tool.Name] = true
	}
	for _, want := range []string{"gitmake_config_write", "gitmake_config_patch", "gitmake_apply"} {
		if !names[want] {
			t.Fatalf("missing write tool %s", want)
		}
	}
}

func TestMCPApplyUsesTokenlessLocalApproval(t *testing.T) {
	s := &mcpServer{allowWrite: true}
	for _, tool := range s.tools() {
		if tool.Name != "gitmake_apply" {
			continue
		}
		req, _ := tool.InputSchema["required"].([]string)
		if len(req) != 1 || req[0] != "plan_id" {
			t.Fatalf("gitmake_apply must require only plan_id in v1 tokenless flow: %#v", req)
		}
		b, _ := json.Marshal(tool)
		if !bytes.Contains(b, []byte("MCP client supports elicitation")) || !bytes.Contains(b, []byte("gitmake approve")) {
			t.Fatalf("apply tool should explain chat approval and terminal fallback: %s", b)
		}
		return
	}
	t.Fatal("gitmake_apply tool missing")
}

func TestMCPLegacyInitialize(t *testing.T) {
	s := &mcpServer{}
	params, _ := json.Marshal(map[string]any{"protocolVersion": "2025-11-25"})
	resp, ok := s.handle(mcpRequest{JSONRPC: "2.0", ID: float64(1), Method: "initialize", Params: params})
	if !ok || resp.Error != nil {
		t.Fatalf("initialize failed: %#v", resp)
	}
	b, _ := json.Marshal(resp.Result)
	if !bytes.Contains(b, []byte(`"name":"gitmake"`)) {
		t.Fatalf("server info missing: %s", b)
	}
}

func TestMCPModernToolsListWithoutInitialize(t *testing.T) {
	s := &mcpServer{}
	resp, ok := s.handle(mcpRequest{JSONRPC: "2.0", ID: float64(1), Method: "tools/list"})
	if !ok || resp.Error != nil {
		t.Fatalf("tools/list failed: %#v", resp)
	}
	b, _ := json.Marshal(resp.Result)
	if !bytes.Contains(b, []byte("gitmake_preview")) {
		t.Fatalf("tools/list missing preview: %s", b)
	}
}

func TestMCPConfigWriteUsesGitMakeAuthoringPath(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "Demo.zip")
	if err := os.WriteFile(zipPath, []byte("not read during config write"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{allowWrite: true}
	result, err := s.callTool("gitmake_config_write", map[string]any{
		"project_dir": dir,
		"config": map[string]any{
			"schema_version": 1,
			"repo":           map[string]any{"name": "Demo", "visibility": "private"},
			"source":         map[string]any{"zip": "Demo.zip", "strip_root": false},
			"git":            map[string]any{"branch": "main"},
		},
	})
	if err != nil {
		t.Fatalf("config write failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gitmake.json")); err != nil {
		t.Fatalf("gitmake.json not written: %v", err)
	}
	b, _ := json.Marshal(result)
	if !bytes.Contains(b, []byte(`"written":true`)) {
		t.Fatalf("unexpected result: %s", b)
	}
}

func TestMCPStdioServerRoundTrip(t *testing.T) {
	input := bytes.NewBufferString("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/list\"}\n")
	var output bytes.Buffer
	s := &mcpServer{}
	if err := s.serve(input, &output); err != nil {
		t.Fatal(err)
	}
	var resp mcpResponse
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &resp); err != nil {
		t.Fatalf("invalid response JSON: %v: %s", err, output.String())
	}
	if resp.Error != nil {
		t.Fatalf("unexpected MCP error: %#v", resp.Error)
	}
}

func TestMCPUsesClaudeProjectDirFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_PROJECT_DIR", dir)
	result, err := invokeGitMakeJSON("", nil, "config", "schema", "--json")
	if err != nil {
		t.Fatalf("schema via env project dir failed: %v", err)
	}
	b, _ := json.Marshal(result)
	if !bytes.Contains(b, []byte(`"$id":"gitmake.config/v1"`)) {
		t.Fatalf("unexpected result: %s", b)
	}
}

func TestMCPConfigSuggestIsReadOnlyAndSelfContained(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "Demo_Source.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("Demo/go.mod")
	_, _ = w.Write([]byte("module example.com/demo\n"))
	_ = zw.Close()
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{}
	result, err := s.callTool("gitmake_config_suggest", map[string]any{"project_dir": dir, "repo_name": "DemoRepo", "visibility": "private", "branch": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gitmake.json")); !os.IsNotExist(err) {
		t.Fatalf("config suggestion must not write gitmake.json, err=%v", err)
	}
	b, _ := json.Marshal(result)
	if !bytes.Contains(b, []byte(`"name":"DemoRepo"`)) {
		t.Fatalf("unexpected suggestion: %s", b)
	}
}

func TestMCPModernApplyRequestsHumanElicitation(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	plan := planstore.Plan{
		Schema: planstore.Schema, ID: "gm_elicitation_test", WorkingDirectory: t.TempDir(),
		SourcePath: "demo.zip", SourceSHA256: "src", Repository: "owner/demo", Visibility: "private",
		Mode: "CREATE", Branch: "main", Fingerprint: "fp",
		Changes: planstore.ChangeCounts{Added: 3}, Risk: planstore.Risk{Level: "low"},
	}
	if _, err := planstore.Save(plan); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{allowWrite: true}
	params, _ := json.Marshal(map[string]any{
		"name":      "gitmake_apply",
		"arguments": map[string]any{"plan_id": plan.ID},
		"_meta": map[string]any{
			"io.modelcontextprotocol/protocolVersion":    mcpProtocolModern,
			"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "test", "version": "1"},
			"io.modelcontextprotocol/clientCapabilities": map[string]any{"elicitation": map[string]any{}},
		},
	})
	resp, ok := s.handle(mcpRequest{JSONRPC: "2.0", ID: float64(1), Method: "tools/call", Params: params})
	if !ok || resp.Error != nil {
		t.Fatalf("modern apply failed: %#v", resp)
	}
	ir, ok := resp.Result.(*mcpInputRequiredResult)
	if !ok {
		t.Fatalf("expected input_required result, got %#v", resp.Result)
	}
	if ir.ResultType != "input_required" || ir.RequestState == "" || ir.InputRequests[mcpApprovalInputKey] == nil {
		t.Fatalf("bad elicitation result: %#v", ir)
	}
}

func TestMCPModernApprovalStateRejectsTampering(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	plan := planstore.Plan{
		Schema: planstore.Schema, ID: "gm_elicitation_tamper", WorkingDirectory: t.TempDir(),
		SourcePath: "demo.zip", SourceSHA256: "src", Repository: "owner/demo", Visibility: "private",
		Mode: "CREATE", Branch: "main", Fingerprint: "fp",
		Changes: planstore.ChangeCounts{Added: 1}, Risk: planstore.Risk{Level: "low"},
	}
	if _, err := planstore.Save(plan); err != nil {
		t.Fatal(err)
	}
	s := &mcpServer{allowWrite: true}
	meta := map[string]any{
		"io.modelcontextprotocol/protocolVersion":    mcpProtocolModern,
		"io.modelcontextprotocol/clientInfo":         map[string]any{"name": "test", "version": "1"},
		"io.modelcontextprotocol/clientCapabilities": map[string]any{"elicitation": map[string]any{}},
	}
	first, _ := json.Marshal(map[string]any{"name": "gitmake_apply", "arguments": map[string]any{"plan_id": plan.ID}, "_meta": meta})
	resp, _ := s.handle(mcpRequest{JSONRPC: "2.0", ID: float64(1), Method: "tools/call", Params: first})
	ir := resp.Result.(*mcpInputRequiredResult)
	retry, _ := json.Marshal(map[string]any{
		"name": "gitmake_apply", "arguments": map[string]any{"plan_id": plan.ID}, "_meta": meta,
		"requestState":   ir.RequestState + "tampered",
		"inputResponses": map[string]any{mcpApprovalInputKey: map[string]any{"action": "accept", "content": map[string]any{}}},
	})
	resp2, _ := s.handle(mcpRequest{JSONRPC: "2.0", ID: float64(2), Method: "tools/call", Params: retry})
	b, _ := json.Marshal(resp2.Result)
	if !bytes.Contains(b, []byte(`"isError":true`)) || !bytes.Contains(b, []byte("invalid MCP approval request state")) {
		t.Fatalf("tampered requestState should be rejected: %s", b)
	}
}

func TestMCPDestructiveElicitationRequiresPlanPhrase(t *testing.T) {
	plan := planstore.Plan{ID: "gm_123456abcdef", Repository: "owner/demo", Visibility: "public", Mode: "UPDATE", Fingerprint: "fp", SourceSHA256: "src", Risk: planstore.Risk{Level: "high", Destructive: true}, Changes: planstore.ChangeCounts{Deleted: 20}}
	s := &mcpServer{}
	_, schema, required := s.approvalPrompt(plan)
	if required != "DELETE-ABCDEF" {
		t.Fatalf("unexpected destructive confirmation: %s", required)
	}
	b, _ := json.Marshal(schema)
	if !bytes.Contains(b, []byte("DELETE-ABCDEF")) {
		t.Fatalf("destructive schema missing confirmation phrase: %s", b)
	}
}

func TestMCPLegacyInitializeRemembersElicitationCapability(t *testing.T) {
	s := &mcpServer{}
	params, _ := json.Marshal(map[string]any{
		"protocolVersion": mcpProtocolLegacy,
		"capabilities":    map[string]any{"elicitation": map[string]any{}},
	})
	resp, ok := s.handle(mcpRequest{JSONRPC: "2.0", ID: float64(1), Method: "initialize", Params: params})
	if !ok || resp.Error != nil || !s.legacyElicitation {
		t.Fatalf("legacy elicitation capability not remembered: %#v", resp)
	}
}

func TestMCPServerDiscoverAdvertisesModernProtocol(t *testing.T) {
	s := &mcpServer{}
	resp, ok := s.handle(mcpRequest{JSONRPC: "2.0", ID: "d1", Method: "server/discover"})
	if !ok || resp.Error != nil {
		t.Fatalf("server/discover failed: %#v", resp)
	}
	b, _ := json.Marshal(resp.Result)
	if !bytes.Contains(b, []byte(mcpProtocolModern)) || !bytes.Contains(b, []byte(`"resultType":"complete"`)) {
		t.Fatalf("modern discovery response incomplete: %s", b)
	}
}
