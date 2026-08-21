package app

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPDefaultToolsetIsReadOnly(t *testing.T) {
	s := &mcpServer{}
	names := map[string]bool{}
	for _, tool := range s.tools() {
		names[tool.Name] = true
	}
	for _, want := range []string{"gitmake_describe", "gitmake_project_inspect", "gitmake_doctor", "gitmake_discover", "gitmake_config_suggest", "gitmake_config_schema", "gitmake_config_validate", "gitmake_preview", "gitmake_plan", "gitmake_history"} {
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

func TestMCPApplyRequiresOneShotApprovalToken(t *testing.T) {
	s := &mcpServer{allowWrite: true}
	for _, tool := range s.tools() {
		if tool.Name != "gitmake_apply" {
			continue
		}
		req, _ := tool.InputSchema["required"].([]string)
		seen := map[string]bool{}
		for _, v := range req {
			seen[v] = true
		}
		if !seen["plan_id"] || !seen["approval_token"] {
			t.Fatalf("gitmake_apply must require plan_id + approval_token: %#v", req)
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
