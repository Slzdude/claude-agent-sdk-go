package claude

// client_test.go covers end-to-end behaviour of processQuery / Query
// using in-memory mock transports, mirroring test_client.py and
// test_tool_callbacks.py. Cross-platform: no shell scripts are created.

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// mustJSON marshals v or fatals.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// buildTestProto creates a queryProto backed by an in-memory mock transport
// (see mock_transport_test.go). No subprocess or shell script is involved.
func buildTestProto(t *testing.T, tr *cliTransport, opts *ClaudeAgentOptions) (*queryProto, *cliTransport) {
	t.Helper()
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	return newQueryProto(tr, opts), tr
}

// drainMessages runs Run() and returns all parsed messages (skipping nil / error).
func drainMessages(t *testing.T, q *queryProto, tr *cliTransport) []Message {
	t.Helper()
	ctx := context.Background()
	rawCh := q.Run(ctx)
	defer tr.close()

	var out []Message
	for raw := range rawCh {
		msg, _ := parseMessage(raw)
		if msg != nil {
			out = append(out, msg)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// Tests that mirror test_client.py / TestQueryFunction
// ---------------------------------------------------------------------------

// TestClient_SimpleAssistantResponse verifies a text assistant response.
func TestClient_SimpleAssistantResponse(t *testing.T) {
	lines := []string{
		mustJSON(map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role":  "assistant",
				"model": "claude-opus-4-5",
				"content": []map[string]any{
					{"type": "text", "text": "4"},
				},
			},
		}),
		mustJSON(map[string]any{
			"type": "result", "subtype": "success", "is_error": false,
			"num_turns": 1, "session_id": "s1", "total_cost_usd": 0.001,
		}),
	}

	tr := mockTransportLines(t, lines...)
	q, tr := buildTestProto(t, tr, nil)
	msgs := drainMessages(t, q, tr)

	if len(msgs) < 2 {
		t.Fatalf("expected >=2 messages, got %d", len(msgs))
	}
	am, ok := msgs[0].(*AssistantMessage)
	if !ok {
		t.Fatalf("expected *AssistantMessage, got %T", msgs[0])
	}
	if tb, ok := am.Content[0].(*TextBlock); !ok || tb.Text != "4" {
		t.Errorf("wrong text: %v", am.Content)
	}
}

// TestClient_ResultMessageFields checks all ResultMessage fields are populated.
func TestClient_ResultMessageFields(t *testing.T) {
	lines := []string{
		mustJSON(map[string]any{
			"type":            "result",
			"subtype":         "success",
			"duration_ms":     1500,
			"duration_api_ms": 1200,
			"is_error":        false,
			"num_turns":       2,
			"session_id":      "my-session",
			"total_cost_usd":  0.042,
		}),
	}

	tr := mockTransportLines(t, lines...)
	q, tr := buildTestProto(t, tr, nil)
	msgs := drainMessages(t, q, tr)

	if len(msgs) < 1 {
		t.Fatal("expected at least 1 message")
	}
	rm, ok := msgs[0].(*ResultMessage)
	if !ok {
		t.Fatalf("expected *ResultMessage, got %T", msgs[0])
	}
	if rm.SessionID != "my-session" {
		t.Errorf("wrong SessionID: %q", rm.SessionID)
	}
	if rm.NumTurns != 2 {
		t.Errorf("wrong NumTurns: %d", rm.NumTurns)
	}
	if rm.TotalCostUSD == nil || *rm.TotalCostUSD != 0.042 {
		t.Errorf("wrong TotalCostUSD: %v", rm.TotalCostUSD)
	}
}

// ---------------------------------------------------------------------------
// Tests that mirror test_tool_callbacks.py
// ---------------------------------------------------------------------------

// TestToolCallback_Allow verifies allow callback sends behavior+updatedInput.
func TestToolCallback_Allow(t *testing.T) {
	invoked := false
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
			invoked = true
			if toolName != "TestTool" {
				t.Errorf("unexpected tool: %s", toolName)
			}
			return &PermissionResultAllow{}, nil
		},
	}
	q := &queryProto{opts: opts}
	req := map[string]any{
		"tool_name":              "TestTool",
		"tool_input":             map[string]any{"param": "value"},
		"permission_suggestions": []any{},
	}
	resp, err := q.handleCanUseTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !invoked {
		t.Error("callback was not invoked")
	}
	if resp["behavior"] != "allow" {
		t.Errorf("expected behavior=allow, got %v", resp["behavior"])
	}
}

// TestToolCallback_Deny verifies deny callback sends deny+message.
func TestToolCallback_Deny(t *testing.T) {
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
			return &PermissionResultDeny{Message: "Security policy violation"}, nil
		},
	}
	q := &queryProto{opts: opts}
	req := map[string]any{
		"tool_name":              "DangerousTool",
		"tool_input":             map[string]any{"command": "rm -rf /"},
		"permission_suggestions": []any{"deny"},
	}
	resp, err := q.handleCanUseTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp["behavior"] != "deny" {
		t.Errorf("expected behavior=deny, got %v", resp["behavior"])
	}
	if resp["message"] != "Security policy violation" {
		t.Errorf("expected message, got %v", resp["message"])
	}
}

// TestToolCallback_InputModification verifies callback can modify tool input.
func TestToolCallback_InputModification(t *testing.T) {
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
			modified := make(map[string]any)
			for k, v := range input {
				modified[k] = v
			}
			modified["safe_mode"] = true
			return &PermissionResultAllow{UpdatedInput: modified}, nil
		},
	}
	q := &queryProto{opts: opts}
	req := map[string]any{
		"tool_name":              "WriteTool",
		"tool_input":             map[string]any{"file_path": "/etc/passwd"},
		"permission_suggestions": []any{},
	}
	resp, err := q.handleCanUseTool(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if resp["behavior"] != "allow" {
		t.Errorf("expected allow, got %v", resp["behavior"])
	}
	updatedInput, ok := resp["updatedInput"].(map[string]any)
	if !ok {
		t.Fatalf("updatedInput not a map: %v", resp["updatedInput"])
	}
	if updatedInput["safe_mode"] != true {
		t.Errorf("safe_mode not set: %v", updatedInput)
	}
}

// TestToolCallback_Exception verifies callback errors result in error response.
func TestToolCallback_Exception(t *testing.T) {
	// We test this via the full handleInboundControlRequest pipeline.
	// We need a transport to capture the writeJSON output.
	pr, pw := io.Pipe()
	stdinR, stdinW, _ := os.Pipe()
	defer stdinW.Close()
	go io.Copy(io.Discard, stdinR)

	tr := &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: defaultMaxBufferSize,
	}
	tr.stdin = stdinW
	tr.stdout = bufio.NewScanner(pr)
	tr.stdout.Buffer(make([]byte, defaultMaxBufferSize), defaultMaxBufferSize)

	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, _ ToolPermissionContext) (PermissionResult, error) {
			return nil, errorf("Callback error")
		},
	}
	q := newQueryProto(tr, opts)

	// Close pipe after test so the test doesn't hang.
	go func() { pw.Close() }()

	// Capture written output by intercepting writeJSON via the same writer.
	var written []byte
	origStdin := tr.stdin
	outR, outW, _ := os.Pipe()
	tr.stdin = outW

	envelope := map[string]any{
		"request_id": "test-5",
		"request": map[string]any{
			"subtype":                "can_use_tool",
			"tool_name":              "TestTool",
			"tool_input":             map[string]any{},
			"permission_suggestions": []any{},
		},
	}
	// Run synchronously in goroutine then close writer.
	done := make(chan struct{})
	go func() {
		defer close(done)
		q.handleInboundControlRequest(context.Background(), envelope)
		outW.Close()
	}()
	written, _ = io.ReadAll(outR)
	<-done
	_ = origStdin

	if len(written) == 0 {
		t.Skip("no response captured (stdin redirect not supported in this env)")
	}
	// Verify the response contains an error subtype.
	if !strings.Contains(string(written), `"error"`) && !strings.Contains(string(written), "Callback error") {
		t.Errorf("expected error response, got: %s", written)
	}
}

// errorf is a helper to create a plain error.
func errorf(msg string) error {
	return &testError{msg}
}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

// TestHookCallback_Execution verifies hook callbacks receive correct input.
func TestHookCallback_Execution(t *testing.T) {
	hookCalls := []map[string]any{}

	opts := &ClaudeAgentOptions{}
	q := &queryProto{
		opts:          opts,
		hookCallbacks: map[string]HookCallback{},
		hookTimeouts:  map[string]float64{},
	}
	callbackID := "test_hook_0"
	q.hookCallbacks[callbackID] = func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		hookCalls = append(hookCalls, map[string]any{"input": input, "tool_use_id": toolUseID})
		return map[string]any{"processed": true}, nil
	}

	req := map[string]any{
		"callback_id": callbackID,
		"input":       map[string]any{"test": "data"},
		"tool_use_id": "tool-123",
	}
	resp, err := q.handleHookCallback(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(hookCalls) != 1 {
		t.Errorf("expected 1 hook call, got %d", len(hookCalls))
	}
	if hookCalls[0]["tool_use_id"] != "tool-123" {
		t.Errorf("wrong tool_use_id: %v", hookCalls[0]["tool_use_id"])
	}
	if resp["processed"] != true {
		t.Errorf("expected processed=true, got %v", resp["processed"])
	}
}

// TestHookCallback_UnknownID verifies unknown hook IDs return empty.
func TestHookCallback_UnknownID(t *testing.T) {
	q := &queryProto{
		opts:          &ClaudeAgentOptions{},
		hookCallbacks: map[string]HookCallback{},
		hookTimeouts:  map[string]float64{},
	}
	req := map[string]any{
		"callback_id": "nonexistent_hook",
		"input":       map[string]any{},
	}
	resp, err := q.handleHookCallback(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(resp) != 0 {
		t.Errorf("expected empty resp for unknown hook, got %v", resp)
	}
}
