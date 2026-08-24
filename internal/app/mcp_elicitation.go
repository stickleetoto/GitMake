package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitmake/internal/approval"
	"gitmake/internal/planstore"
)

const mcpApprovalInputKey = "gitmake_approval"

type mcpInputResponse struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

type mcpInputRequiredResult struct {
	ResultType    string                    `json:"resultType"`
	InputRequests map[string]map[string]any `json:"inputRequests"`
	RequestState  string                    `json:"requestState,omitempty"`
}

type mcpApprovalState struct {
	PlanID      string `json:"plan_id"`
	Fingerprint string `json:"fingerprint"`
	ExpiresUnix int64  `json:"expires_unix"`
	Nonce       string `json:"nonce"`
}

type mcpLegacyResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (s *mcpServer) requestProtocol(params json.RawMessage) string {
	var raw map[string]any
	if len(params) == 0 || json.Unmarshal(params, &raw) != nil {
		return ""
	}
	meta, _ := raw["_meta"].(map[string]any)
	if meta == nil {
		return ""
	}
	v, _ := meta["io.modelcontextprotocol/protocolVersion"].(string)
	return strings.TrimSpace(v)
}

func (s *mcpServer) requestSupportsElicitation(params json.RawMessage) bool {
	var raw map[string]any
	if len(params) == 0 || json.Unmarshal(params, &raw) != nil {
		return false
	}
	meta, _ := raw["_meta"].(map[string]any)
	caps, _ := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any)
	if caps == nil {
		return false
	}
	_, ok := caps["elicitation"]
	return ok
}

func (s *mcpServer) isModernRequest(params json.RawMessage) bool {
	return s.requestProtocol(params) == mcpProtocolModern
}

func (s *mcpServer) approvalPrompt(plan planstore.Plan) (message string, schema map[string]any, requiredConfirmation string) {
	level := strings.ToLower(strings.TrimSpace(plan.Risk.Level))
	if level == "" {
		level = "low"
	}
	message = fmt.Sprintf(
		"GitMake wants your approval to %s %s. Changes: +%d ~%d -%d. Visibility: %s. Risk: %s. Plan: %s. GitMake will revalidate the exact reviewed plan before publishing.",
		strings.ToLower(plan.Mode), plan.Repository, plan.Changes.Added, plan.Changes.Modified, plan.Changes.Deleted, plan.Visibility, level, plan.ID,
	)
	props := map[string]any{}
	required := []string{}
	if plan.Risk.Destructive || level == "high" {
		requiredConfirmation = "DELETE-" + confirmationCode(plan.ID)
		message += " This is destructive. Type the exact confirmation phrase shown in the form."
		props["confirmation"] = map[string]any{
			"type":        "string",
			"title":       "Destructive confirmation",
			"description": "Type exactly: " + requiredConfirmation,
			"enum":        []string{requiredConfirmation},
		}
		required = append(required, "confirmation")
	} else if level == "medium" {
		requiredConfirmation = "PUBLISH"
		message += " This plan has medium risk. Type PUBLISH in the form to continue."
		props["confirmation"] = map[string]any{
			"type":        "string",
			"title":       "Confirmation",
			"description": "Type exactly: PUBLISH",
			"enum":        []string{"PUBLISH"},
		}
		required = append(required, "confirmation")
	}
	schema = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           props,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return message, schema, requiredConfirmation
}

func (s *mcpServer) approvalInputRequest(plan planstore.Plan) map[string]any {
	message, schema, _ := s.approvalPrompt(plan)
	return map[string]any{
		"method": "elicitation/create",
		"params": map[string]any{
			"mode":            "form",
			"message":         message,
			"requestedSchema": schema,
		},
	}
}

func (s *mcpServer) ensureStateSecret() error {
	if len(s.stateSecret) == 32 {
		return nil
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("create MCP approval state key: %w", err)
	}
	s.stateSecret = secret
	return nil
}

func (s *mcpServer) createApprovalRequestState(plan planstore.Plan) (string, error) {
	if err := s.ensureStateSecret(); err != nil {
		return "", err
	}
	nonceRaw := make([]byte, 16)
	if _, err := rand.Read(nonceRaw); err != nil {
		return "", err
	}
	st := mcpApprovalState{
		PlanID:      plan.ID,
		Fingerprint: plan.Fingerprint,
		ExpiresUnix: time.Now().UTC().Add(10 * time.Minute).Unix(),
		Nonce:       base64.RawURLEncoding.EncodeToString(nonceRaw),
	}
	body, err := json.Marshal(st)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.stateSecret)
	_, _ = mac.Write(body)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (s *mcpServer) validateApprovalRequestState(encoded string, plan planstore.Plan) error {
	if err := s.ensureStateSecret(); err != nil {
		return err
	}
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		return fmt.Errorf("invalid MCP approval request state")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return fmt.Errorf("invalid MCP approval request state")
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid MCP approval request state")
	}
	mac := hmac.New(sha256.New, s.stateSecret)
	_, _ = mac.Write(body)
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return fmt.Errorf("invalid MCP approval request state signature")
	}
	var st mcpApprovalState
	if err := json.Unmarshal(body, &st); err != nil {
		return fmt.Errorf("invalid MCP approval request state")
	}
	if st.PlanID != plan.ID || !strings.EqualFold(st.Fingerprint, plan.Fingerprint) {
		return fmt.Errorf("MCP approval request no longer matches the reviewed plan")
	}
	if time.Now().UTC().Unix() > st.ExpiresUnix {
		return fmt.Errorf("MCP approval request expired; ask GitMake to apply again")
	}
	return nil
}

func (s *mcpServer) validateApprovalInput(plan planstore.Plan, response mcpInputResponse) (bool, error) {
	action := strings.ToLower(strings.TrimSpace(response.Action))
	switch action {
	case "decline", "cancel":
		return false, nil
	case "accept":
		// Continue below.
	default:
		return false, fmt.Errorf("invalid elicitation action %q", response.Action)
	}
	_, _, required := s.approvalPrompt(plan)
	if required == "" {
		return true, nil
	}
	got, _ := response.Content["confirmation"].(string)
	if strings.TrimSpace(got) != required {
		return false, fmt.Errorf("approval confirmation did not match %s", required)
	}
	return true, nil
}

func (s *mcpServer) modernApplyResult(p mcpToolCallParams) (result any, inputRequired *mcpInputRequiredResult, handled bool, err error) {
	if p.Name != "gitmake_apply" {
		return nil, nil, false, nil
	}
	planID, err := stringArg(p.Arguments, "plan_id", true)
	if err != nil {
		return nil, nil, true, err
	}
	plan, _, err := planstore.Load(planID)
	if err != nil {
		return nil, nil, true, err
	}
	legacyToken, err := stringArg(p.Arguments, "approval_token", false)
	if err != nil {
		return nil, nil, true, err
	}
	if legacyToken != "" {
		result, err := s.callTool(p.Name, p.Arguments)
		return result, nil, true, err
	}
	if _, grantErr := approval.ValidateGrant(planID, approvalBindingFromPlan(plan)); grantErr == nil {
		result, err := s.callTool(p.Name, p.Arguments)
		return result, nil, true, err
	}

	if len(p.InputResponses) == 0 {
		state, err := s.createApprovalRequestState(plan)
		if err != nil {
			return nil, nil, true, err
		}
		return nil, &mcpInputRequiredResult{
			ResultType: "input_required",
			InputRequests: map[string]map[string]any{
				mcpApprovalInputKey: s.approvalInputRequest(plan),
			},
			RequestState: state,
		}, true, nil
	}
	if err := s.validateApprovalRequestState(p.RequestState, plan); err != nil {
		return nil, nil, true, err
	}
	response, ok := p.InputResponses[mcpApprovalInputKey]
	if !ok {
		return nil, nil, true, fmt.Errorf("MCP approval response is missing")
	}
	accepted, err := s.validateApprovalInput(plan, response)
	if err != nil {
		return nil, nil, true, err
	}
	if !accepted {
		return map[string]any{
			"schema": "gitmake.apply-approval/v1", "ok": false, "status": "approval_declined",
			"plan_id": plan.ID, "repository": plan.Repository, "github_mutated": false,
		}, nil, true, nil
	}
	if _, err := approval.CreateGrant(plan.ID, approvalBindingFromPlan(plan), plan.Risk.Destructive); err != nil {
		return nil, nil, true, fmt.Errorf("record MCP human approval: %w", err)
	}
	result, err = s.callTool(p.Name, p.Arguments)
	return result, nil, true, err
}

func (s *mcpServer) legacyShouldElicit(p mcpToolCallParams) (planstore.Plan, bool) {
	if p.Name != "gitmake_apply" || !s.allowWrite || !s.legacyElicitation {
		return planstore.Plan{}, false
	}
	legacyToken, _ := stringArg(p.Arguments, "approval_token", false)
	if legacyToken != "" {
		return planstore.Plan{}, false
	}
	planID, err := stringArg(p.Arguments, "plan_id", true)
	if err != nil {
		return planstore.Plan{}, false
	}
	plan, _, err := planstore.Load(planID)
	if err != nil {
		return planstore.Plan{}, false
	}
	if _, err := approval.ValidateGrant(planID, approvalBindingFromPlan(plan)); err == nil {
		return planstore.Plan{}, false
	}
	return plan, true
}

func (s *mcpServer) legacyApplyWithElicitation(req mcpRequest, p mcpToolCallParams, scanner lineScanner, enc responseEncoder) (mcpResponse, error) {
	base := mcpResponse{JSONRPC: "2.0", ID: req.ID}
	planID, err := stringArg(p.Arguments, "plan_id", true)
	if err != nil {
		return s.toolErrorResponse(base, err, false), nil
	}
	plan, _, err := planstore.Load(planID)
	if err != nil {
		return s.toolErrorResponse(base, err, false), nil
	}

	s.nextServerRequest++
	requestID := fmt.Sprintf("gitmake-elicitation-%d", s.nextServerRequest)
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  "elicitation/create",
		"params":  s.approvalInputRequest(plan)["params"],
	}
	if err := enc.Encode(request); err != nil {
		return base, fmt.Errorf("write MCP elicitation request: %w", err)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var msg mcpLegacyResponse
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}
		if fmt.Sprint(msg.ID) != requestID {
			// A notification can arrive while the dialog is open; it does not
			// complete the elicitation. Claude Code serializes tool requests on
			// this stdio channel, so unrelated requests are ignored here.
			continue
		}
		if msg.Error != nil {
			return s.toolErrorResponse(base, fmt.Errorf("MCP elicitation failed: %s", msg.Error.Message), false), nil
		}
		var response mcpInputResponse
		if err := json.Unmarshal(msg.Result, &response); err != nil {
			return s.toolErrorResponse(base, fmt.Errorf("parse MCP elicitation response: %w", err), false), nil
		}
		accepted, err := s.validateApprovalInput(plan, response)
		if err != nil {
			return s.toolErrorResponse(base, err, false), nil
		}
		if !accepted {
			result := map[string]any{
				"schema": "gitmake.apply-approval/v1", "ok": false, "status": "approval_declined",
				"plan_id": plan.ID, "repository": plan.Repository, "github_mutated": false,
			}
			return s.toolSuccessResponse(base, result, false), nil
		}
		if _, err := approval.CreateGrant(plan.ID, approvalBindingFromPlan(plan), plan.Risk.Destructive); err != nil {
			return s.toolErrorResponse(base, fmt.Errorf("record MCP human approval: %w", err), false), nil
		}
		result, err := s.callTool(p.Name, p.Arguments)
		if err != nil {
			return s.toolErrorResponse(base, err, false), nil
		}
		return s.toolSuccessResponse(base, result, false), nil
	}
	if err := scanner.Err(); err != nil {
		return base, fmt.Errorf("read MCP elicitation response: %w", err)
	}
	return s.toolErrorResponse(base, fmt.Errorf("MCP client disconnected before approval was answered"), false), nil
}

// Small interfaces keep legacy elicitation testable without coupling the
// implementation to concrete bufio/json types.
type lineScanner interface {
	Scan() bool
	Text() string
	Err() error
}

type responseEncoder interface {
	Encode(v any) error
}

func (s *mcpServer) toolSuccessResponse(base mcpResponse, result any, modern bool) mcpResponse {
	payload := map[string]any{
		"content":           []map[string]any{{"type": "text", "text": mustJSONString(result)}},
		"structuredContent": result,
		"isError":           false,
	}
	if modern {
		payload["resultType"] = "complete"
	}
	base.Result = payload
	return base
}

func (s *mcpServer) toolErrorResponse(base mcpResponse, err error, modern bool) mcpResponse {
	payload := map[string]any{
		"content": []map[string]any{{"type": "text", "text": mustJSONString(map[string]any{"ok": false, "error": err.Error()})}},
		"isError": true,
	}
	if modern {
		payload["resultType"] = "complete"
	}
	base.Result = payload
	return base
}
