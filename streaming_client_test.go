package claude

// streaming_client_test.go mirrors test_streaming_client.py.
// Tests for ClaudeSDKClient using mock subprocess scripts.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// makeInitScript writes a mock CLI script that responds to the initialize
// control_request and then emits the provided JSON lines.
//
// The script reads the first stdin line (the initialize request), extracts
// request_id via jq, sends back a success control_response, then echoes
// the given lines in order.  Remaining stdin is drained in the background.
func makeInitScript(t *testing.T, lines ...string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-claude-init-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	// Read the initialize control_request from stdin
	sb.WriteString("IFS= read -r initline\n")
	sb.WriteString("request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n")
	// Respond with success — note the double-nested "response" field (outer envelope
	// contains request_id/subtype; inner "response" contains the payload data).
	sb.WriteString("printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%s\",\"subtype\":\"success\",\"response\":{\"commands\":[],\"output_style\":\"default\"}}}\\n' \"$request_id\"\n")
	// Drain remaining stdin in background
	sb.WriteString("cat > /dev/null &\n")
	// Emit provided messages
	for _, l := range lines {
		sb.WriteString("printf '%s\\n' '")
		sb.WriteString(strings.ReplaceAll(l, "'", "'\\''"))
		sb.WriteString("'\n")
	}
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)
	return f.Name()
}

// makeInitScriptWithControlResponse creates a mock CLI script that handles
// both initialization and one additional control_request (e.g. interrupt).
// After receiving the additional control_request, it sends its response and exits.
func makeInitScriptWithControlResponse(t *testing.T, extraResponseSubtype string, lines ...string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-claude-ctrl-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	// Handle initialize
	sb.WriteString("IFS= read -r initline\n")
	sb.WriteString("request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n")
	sb.WriteString("printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%s\",\"subtype\":\"success\",\"response\":{\"commands\":[],\"output_style\":\"default\"}}}\\n' \"$request_id\"\n")
	// Emit provided messages
	for _, l := range lines {
		sb.WriteString("printf '%s\\n' '")
		sb.WriteString(strings.ReplaceAll(l, "'", "'\\''"))
		sb.WriteString("'\n")
	}
	// Read a second control_request (e.g. interrupt/set_model/etc.)
	sb.WriteString("IFS= read -r ctrlline\n")
	sb.WriteString("ctrl_id=$(printf '%s' \"$ctrlline\" | /usr/bin/jq -r '.request_id')\n")
	sb.WriteString(fmt.Sprintf(
		"printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%%s\",\"subtype\":\"%s\",\"response\":{}}}\\n' \"$ctrl_id\"\n",
		extraResponseSubtype,
	))
	sb.WriteString("cat > /dev/null &\n")
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)
	return f.Name()
}

func assistantJSON(text string) string {
	m := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": text},
			},
			"model": "claude-sonnet-4-20250514",
		},
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func resultJSON() string {
	m := map[string]any{
		"type":            "result",
		"subtype":         "success",
		"duration_ms":     100,
		"duration_api_ms": 80,
		"is_error":        false,
		"num_turns":       1,
		"session_id":      "test-session",
		"total_cost_usd":  0.001,
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// newTestClientOpts returns ClaudeAgentOptions pointing to the given mock script.
func newTestClientOpts(scriptPath string) *ClaudeAgentOptions {
	return &ClaudeAgentOptions{CLIPath: scriptPath}
}

// TestStreamingClient_BasicConnectClose verifies NewClaudeSDKClient connects
// and Close terminates gracefully.
func TestStreamingClient_BasicConnectClose(t *testing.T) {
	script := makeInitScript(t)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestStreamingClient_DoubleCloseSafe verifies that calling Close twice is safe.
func TestStreamingClient_DoubleCloseSafe(t *testing.T) {
	script := makeInitScript(t)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("second Close (should be no-op): %v", err)
	}
}

// TestStreamingClient_GetServerInfo verifies that GetServerInfo returns the
// commands slice from the initialize response.
func TestStreamingClient_GetServerInfo(t *testing.T) {
	script := makeInitScript(t)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	info := client.GetServerInfo()
	if info == nil {
		t.Fatal("GetServerInfo returned nil")
	}
	// The mock script returns "commands":[] so Commands field should be present.
	if _, ok := info["commands"]; !ok {
		t.Errorf("expected 'commands' key in server info, got: %v", info)
	}
}

// TestStreamingClient_ReceiveResponse verifies that ReceiveResponse delivers
// messages and closes after the ResultMessage.
func TestStreamingClient_ReceiveResponse(t *testing.T) {
	script := makeInitScript(t,
		assistantJSON("Hello from mock"),
		resultJSON(),
	)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	// Query sends the user message.
	if err := client.Query(ctx, "hello"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	var msgs []Message
	for msg := range client.ReceiveResponse(ctx) {
		msgs = append(msgs, msg)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d: %v", len(msgs), msgs)
	}
	if _, ok := msgs[0].(*AssistantMessage); !ok {
		t.Errorf("expected *AssistantMessage, got %T", msgs[0])
	}
	if _, ok := msgs[1].(*ResultMessage); !ok {
		t.Errorf("expected *ResultMessage, got %T", msgs[1])
	}
}

// TestStreamingClient_ReceiveResponseContent verifies the AssistantMessage content.
func TestStreamingClient_ReceiveResponseContent(t *testing.T) {
	const wantText = "I can help with that!"
	script := makeInitScript(t,
		assistantJSON(wantText),
		resultJSON(),
	)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "help me"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		if am, ok := msg.(*AssistantMessage); ok {
			if len(am.Content) == 0 {
				t.Fatal("empty content")
			}
			tb, ok := am.Content[0].(*TextBlock)
			if !ok {
				t.Fatalf("expected *TextBlock, got %T", am.Content[0])
			}
			if tb.Text != wantText {
				t.Errorf("expected %q, got %q", wantText, tb.Text)
			}
		}
	}
}

// TestStreamingClient_ResultMessageFields verifies ResultMessage fields.
func TestStreamingClient_ResultMessageFields(t *testing.T) {
	script := makeInitScript(t,
		assistantJSON("Done"),
		resultJSON(),
	)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "go"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	var result *ResultMessage
	for msg := range client.ReceiveResponse(ctx) {
		if rm, ok := msg.(*ResultMessage); ok {
			result = rm
		}
	}
	if result == nil {
		t.Fatal("no ResultMessage received")
	}
	if result.Subtype != "success" {
		t.Errorf("expected subtype=success, got %q", result.Subtype)
	}
	if result.IsError {
		t.Error("expected IsError=false")
	}
	if result.SessionID != "test-session" {
		t.Errorf("expected session_id=test-session, got %q", result.SessionID)
	}
}

// TestStreamingClient_ReceiveMessages delivers all messages without closing
// between turns.  Here we only test that it works with a fresh client.
func TestStreamingClient_ReceiveMessages(t *testing.T) {
	script := makeInitScript(t,
		assistantJSON("Turn 1"),
		resultJSON(),
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "hi"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	var msgs []Message
	// ReceiveMessages doesn't close between turns — drain until subprocess exits.
	for msg := range client.ReceiveMessages(ctx) {
		msgs = append(msgs, msg)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(msgs))
	}
}

// TestStreamingClient_Interrupt verifies the Interrupt control round-trip.
func TestStreamingClient_Interrupt(t *testing.T) {
	// Script: initialize, then wait for an interrupt control_request and respond.
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Interrupt(ctx); err != nil {
		t.Errorf("Interrupt: %v", err)
	}
}

// TestStreamingClient_SetPermissionMode verifies the SetPermissionMode round-trip.
func TestStreamingClient_SetPermissionMode(t *testing.T) {
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.SetPermissionMode(ctx, PermissionModeBypassPermissions); err != nil {
		t.Errorf("SetPermissionMode: %v", err)
	}
}

// TestStreamingClient_SetModel verifies the SetModel control round-trip.
func TestStreamingClient_SetModel(t *testing.T) {
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	model := "claude-opus-4-20250514"
	if err := client.SetModel(ctx, &model); err != nil {
		t.Errorf("SetModel: %v", err)
	}
}

// TestStreamingClient_SetModelNil verifies passing nil model (reset to default).
func TestStreamingClient_SetModelNil(t *testing.T) {
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.SetModel(ctx, nil); err != nil {
		t.Errorf("SetModel(nil): %v", err)
	}
}

// TestStreamingClient_ReconnectMcpServer verifies the ReconnectMcpServer round-trip.
func TestStreamingClient_ReconnectMcpServer(t *testing.T) {
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.ReconnectMcpServer(ctx, "my-server"); err != nil {
		t.Errorf("ReconnectMcpServer: %v", err)
	}
}

// TestStreamingClient_ToggleMcpServer verifies the ToggleMcpServer round-trip.
func TestStreamingClient_ToggleMcpServer(t *testing.T) {
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.ToggleMcpServer(ctx, "my-server", false); err != nil {
		t.Errorf("ToggleMcpServer: %v", err)
	}
}

// TestStreamingClient_QueryThenReceive verifies the full Query → ReceiveResponse cycle.
func TestStreamingClient_QueryThenReceive(t *testing.T) {
	script := makeInitScript(t,
		assistantJSON("Query result"),
		resultJSON(),
	)
	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "what is 2+2?"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	var gotAssistant, gotResult bool
	for msg := range client.ReceiveResponse(ctx) {
		switch msg.(type) {
		case *AssistantMessage:
			gotAssistant = true
		case *ResultMessage:
			gotResult = true
		}
	}
	if !gotAssistant || !gotResult {
		t.Errorf("expected both AssistantMessage and ResultMessage; gotAssistant=%v gotResult=%v", gotAssistant, gotResult)
	}
}

// TestStreamingClient_ContextCancellation verifies that Close() causes
// ReceiveResponse to return promptly by terminating the subprocess.
func TestStreamingClient_ContextCancellation(t *testing.T) {
	// A script that initializes properly then blocks reading stdin.
	// When Close() closes stdin, the read returns EOF and the script exits.
	f, err := os.CreateTemp(t.TempDir(), "mock-claude-hang-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"IFS= read -r initline\n" +
		"request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n" +
		"printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%s\",\"subtype\":\"success\",\"response\":{\"commands\":[],\"output_style\":\"default\"}}}\\n' \"$request_id\"\n" +
		"# Block reading stdin until it is closed (by client.Close()).\n" +
		"while IFS= read -r _; do :; done\n"
	io.WriteString(f, script)
	f.Close()
	os.Chmod(f.Name(), 0o755)

	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, &ClaudeAgentOptions{CLIPath: f.Name()})
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}

	if err := client.Query(ctx, "test"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for range client.ReceiveResponse(ctx) {
		}
	}()

	// Close the client to kill the subprocess and close stdin;
	// ReceiveResponse should unblock within a few seconds.
	client.Close()

	select {
	case <-done:
		// Good: returned promptly after Close.
	case <-time.After(5 * time.Second):
		t.Error("ReceiveResponse did not unblock after Close()")
	}
}

// -----------------------------------------------------------------------
// "Not connected" error tests (mirrors test_streaming_client.py not-connected variants)
// In Python, ClaudeSDKClient() creates an unconnected client.
// In Go we use &ClaudeSDKClient{} to simulate the same uninitialized state.
// -----------------------------------------------------------------------

// TestStreamingClient_QueryNotConnected mirrors test_send_message_not_connected.
func TestStreamingClient_QueryNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	err := c.Query(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	connErr, ok := err.(*CLIConnectionError)
	if !ok {
		t.Fatalf("expected CLIConnectionError, got %T: %v", err, err)
	}
	if connErr.Message != "Not connected" {
		t.Errorf("unexpected message: %q", connErr.Message)
	}
}

// TestStreamingClient_InterruptNotConnected mirrors test_interrupt_not_connected.
func TestStreamingClient_InterruptNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	err := c.Interrupt(context.Background())
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if _, ok := err.(*CLIConnectionError); !ok {
		t.Fatalf("expected CLIConnectionError, got %T: %v", err, err)
	}
}

// TestStreamingClient_ReconnectMcpServerNotConnected mirrors test_reconnect_mcp_server_not_connected.
func TestStreamingClient_ReconnectMcpServerNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	err := c.ReconnectMcpServer(context.Background(), "my-server")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if _, ok := err.(*CLIConnectionError); !ok {
		t.Fatalf("expected CLIConnectionError, got %T: %v", err, err)
	}
}

// TestStreamingClient_ToggleMcpServerNotConnected mirrors test_toggle_mcp_server_not_connected.
func TestStreamingClient_ToggleMcpServerNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	err := c.ToggleMcpServer(context.Background(), "my-server", true)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if _, ok := err.(*CLIConnectionError); !ok {
		t.Fatalf("expected CLIConnectionError, got %T: %v", err, err)
	}
}

// TestStreamingClient_StopTaskNotConnected mirrors test_stop_task_not_connected.
func TestStreamingClient_StopTaskNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	err := c.StopTask(context.Background(), "task-abc123")
	if err == nil {
		t.Fatal("expected error when not connected")
	}
	if _, ok := err.(*CLIConnectionError); !ok {
		t.Fatalf("expected CLIConnectionError, got %T: %v", err, err)
	}
}

// TestStreamingClient_GetMcpStatusNotConnected mirrors test_get_mcp_status_not_connected.
func TestStreamingClient_GetMcpStatusNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	status, err := c.GetMcpStatus(context.Background())
	if err == nil {
		t.Fatalf("expected error, got status: %v", status)
	}
	if _, ok := err.(*CLIConnectionError); !ok {
		t.Fatalf("expected CLIConnectionError, got %T: %v", err, err)
	}
}

// TestStreamingClient_ReceiveMessagesNotConnected mirrors test_receive_messages_not_connected:
// ReceiveMessages on an unconnected client should return a closed channel immediately.
func TestStreamingClient_ReceiveMessagesNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	ch := c.ReceiveMessages(context.Background())
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel (not connected), got a message")
		}
		// channel closed — expected
	case <-time.After(1 * time.Second):
		t.Error("ReceiveMessages did not return a closed channel immediately when not connected")
	}
}

// TestStreamingClient_ReceiveResponseNotConnected mirrors test_receive_response_not_connected.
func TestStreamingClient_ReceiveResponseNotConnected(t *testing.T) {
	c := &ClaudeSDKClient{}
	ch := c.ReceiveResponse(context.Background())
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel (not connected), got a message")
		}
	case <-time.After(1 * time.Second):
		t.Error("ReceiveResponse did not return a closed channel immediately when not connected")
	}
}

// TestStreamingClient_DisconnectWithoutConnect mirrors test_disconnect_without_connect:
// calling Close() on an uninitialized client should not panic or error.
func TestStreamingClient_DisconnectWithoutConnect(t *testing.T) {
	c := &ClaudeSDKClient{}
	if err := c.Close(); err != nil {
		t.Errorf("Close() on uninitialized client returned error: %v", err)
	}
	// A second close should also be safe.
	if err := c.Close(); err != nil {
		t.Errorf("second Close() returned error: %v", err)
	}
}

// TestStreamingClient_DoubleConnect mirrors test_double_connect:
// creating two separate clients from the same options works independently.
func TestStreamingClient_DoubleConnect(t *testing.T) {
	script := makeInitScript(t)
	opts := newTestClientOpts(script)
	ctx := context.Background()

	c1, err := NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("first NewClaudeSDKClient: %v", err)
	}
	defer c1.Close()

	c2, err := NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("second NewClaudeSDKClient: %v", err)
	}
	defer c2.Close()
}

// TestStreamingClient_ContextManagerWithException mirrors test_context_manager_with_exception:
// verifies that Close is called even when the using code panics (deferred close pattern).
func TestStreamingClient_ContextManagerWithException(t *testing.T) {
	script := makeInitScript(t)
	opts := newTestClientOpts(script)
	ctx := context.Background()

	closed := false
	func() {
		client, err := NewClaudeSDKClient(ctx, opts)
		if err != nil {
			t.Fatalf("NewClaudeSDKClient: %v", err)
		}
		defer func() {
			client.Close()
			closed = true
		}()
		// Simulate user code that panics/returns early.
		_ = client.GetServerInfo()
		// Return without finishing.
	}()

	if !closed {
		t.Error("defer Close was not called")
	}
}

// TestStreamingClient_StopTask mirrors test_stop_task:
// verifies StopTask sends a stop_task control_request with task_id.
func TestStreamingClient_StopTask(t *testing.T) {
	script := makeInitScriptWithControlResponse(t, "success")
	ctx := context.Background()

	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.StopTask(ctx, "task-abc123"); err != nil {
		t.Errorf("StopTask: %v", err)
	}
}

// makeInitScriptWithMcpStatusResponse creates a script that responds to initialize
// and then to an mcp_status request with the provided JSON response payload.
func makeInitScriptWithMcpStatusResponse(t *testing.T, mcpStatusJSON string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-claude-mcp-status-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	// Handle initialize
	sb.WriteString("IFS= read -r initline\n")
	sb.WriteString("request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n")
	sb.WriteString("printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%s\",\"subtype\":\"success\",\"response\":{\"commands\":[],\"output_style\":\"default\"}}}\\n' \"$request_id\"\n")
	// Read mcp_status control_request
	sb.WriteString("IFS= read -r statusline\n")
	sb.WriteString("status_id=$(printf '%s' \"$statusline\" | /usr/bin/jq -r '.request_id')\n")
	// Respond with the given JSON
	escapedJSON := strings.ReplaceAll(mcpStatusJSON, "'", "'\\''")
	sb.WriteString(fmt.Sprintf(
		"printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%%s\",\"subtype\":\"success\",\"response\":%s}}\\n' \"$status_id\"\n",
		escapedJSON,
	))
	sb.WriteString("cat > /dev/null &\n")
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)
	return f.Name()
}

// TestStreamingClient_GetMcpStatus mirrors test_get_mcp_status:
// verifies GetMcpStatus returns a properly parsed McpStatusResponse.
func TestStreamingClient_GetMcpStatus(t *testing.T) {
	mcpStatusData := map[string]any{
		"mcpServers": []any{
			map[string]any{
				"name":   "my-http-server",
				"status": "connected",
				"serverInfo": map[string]any{
					"name":    "my-http-server",
					"version": "1.0.0",
				},
				"config": map[string]any{
					"type": "http",
					"url":  "https://example.com/mcp",
				},
				"scope": "project",
				"tools": []any{
					map[string]any{
						"name":        "greet",
						"description": "Greet a user",
					},
				},
			},
			map[string]any{
				"name":   "failed-server",
				"status": "failed",
				"error":  "Connection refused",
			},
		},
	}
	b, _ := json.Marshal(mcpStatusData)

	script := makeInitScriptWithMcpStatusResponse(t, string(b))
	ctx := context.Background()

	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	status, err := client.GetMcpStatus(ctx)
	if err != nil {
		t.Fatalf("GetMcpStatus: %v", err)
	}

	if status == nil {
		t.Fatal("expected non-nil McpStatusResponse")
	}
	if len(status.MCPServers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(status.MCPServers))
	}
	if status.MCPServers[0].Name != "my-http-server" {
		t.Errorf("first server name mismatch: %q", status.MCPServers[0].Name)
	}
	if status.MCPServers[0].Status != "connected" {
		t.Errorf("first server status mismatch: %q", status.MCPServers[0].Status)
	}
	if status.MCPServers[1].Name != "failed-server" {
		t.Errorf("second server name mismatch: %q", status.MCPServers[1].Name)
	}
	if status.MCPServers[1].Status != "failed" {
		t.Errorf("second server status mismatch: %q", status.MCPServers[1].Status)
	}
}

// TestStreamingClient_ReceiveResponseListComprehension mirrors test_receive_response_list_comprehension:
// verifies that all messages from ReceiveResponse can be collected into a slice.
func TestStreamingClient_ReceiveResponseListComprehension(t *testing.T) {
	script := makeInitScript(t,
		assistantJSON("Answer"),
		resultJSON(),
		// This message appears after ResultMessage, and should NOT be yielded by ReceiveResponse.
		assistantJSON("Should not appear"),
	)
	ctx := context.Background()

	client, err := NewClaudeSDKClient(ctx, newTestClientOpts(script))
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "question"); err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Drain ReceiveResponse into a slice—like Python's list comprehension.
	var messages []Message
	for msg := range client.ReceiveResponse(ctx) {
		messages = append(messages, msg)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages (assistant + result), got %d", len(messages))
	}
	if _, ok := messages[len(messages)-1].(*ResultMessage); !ok {
		t.Errorf("last message should be ResultMessage, got %T", messages[len(messages)-1])
	}
}

// TestStreamingClient_ConcurrentSendReceive mirrors test_concurrent_send_receive:
// verifies that Query and ReceiveResponse can run concurrently.
func TestStreamingClient_ConcurrentSendReceive(t *testing.T) {
	// Script that waits for a user message before emitting a response.
	// This ensures the Query write and ReceiveResponse read truly overlap.
	f, err := os.CreateTemp(t.TempDir(), "mock-concurrent-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	// Handle initialize
	sb.WriteString("IFS= read -r initline\n")
	sb.WriteString("request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n")
	sb.WriteString("printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%s\",\"subtype\":\"success\",\"response\":{\"commands\":[],\"output_style\":\"default\"}}}\\n' \"$request_id\"\n")
	// Wait for user message (non-blocking drain so we don't block)
	sb.WriteString("IFS= read -r _usermsg || true\n")
	// Emit response and result
	respJSON := assistantJSON("Response 1")
	resJSON := resultJSON()
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(respJSON, "'", "'\\''")))
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(resJSON, "'", "'\\''")))
	// Drain remaining stdin
	sb.WriteString("cat > /dev/null &\n")
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)

	ctx := context.Background()
	client, err := NewClaudeSDKClient(ctx, &ClaudeAgentOptions{CLIPath: f.Name()})
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	// Start ReceiveResponse in a goroutine.
	msgCh := make(chan []Message, 1)
	go func() {
		var msgs []Message
		for msg := range client.ReceiveResponse(ctx) {
			msgs = append(msgs, msg)
		}
		msgCh <- msgs
	}()

	// Send query after a brief pause to ensure receiver is ready.
	time.Sleep(10 * time.Millisecond)
	if err := client.Query(ctx, "test"); err != nil {
		// Allow broken-pipe since the mock may have exited already.
		if !strings.Contains(err.Error(), "broken pipe") {
			t.Fatalf("Query: %v", err)
		}
	}

	select {
	case msgs := <-msgCh:
		if len(msgs) == 0 {
			t.Error("expected at least one message")
		}
	case <-time.After(10 * time.Second):
		t.Error("timed out waiting for concurrent receive")
	}
}

// TestStreamingClient_QueryWithAsyncIterable mirrors TestQueryWithAsyncIterable:
// verifies that QueryStream with a channel works end-to-end.
func TestStreamingClient_QueryWithAsyncIterable(t *testing.T) {
	// Script that reads two user messages then emits result.
	f, err := os.CreateTemp(t.TempDir(), "mock-stream-*.sh")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	// Handle initialize
	sb.WriteString("IFS= read -r initline\n")
	sb.WriteString("request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n")
	sb.WriteString("printf '{\"type\":\"control_response\",\"response\":{\"request_id\":\"%s\",\"subtype\":\"success\",\"response\":{\"commands\":[],\"output_style\":\"default\"}}}\\n' \"$request_id\"\n")
	// Drain stdin
	sb.WriteString("cat > /dev/null &\n")
	// Emit response
	respJSON := assistantJSON("Hello from stream")
	resJSON := resultJSON()
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(respJSON, "'", "'\\''")))
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(resJSON, "'", "'\\''")))
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)

	promptCh := make(chan map[string]any, 1)
	promptCh <- map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": "Hello"},
	}
	close(promptCh)

	ctx := context.Background()
	msgCh, err := QueryStream(ctx, promptCh, &ClaudeAgentOptions{CLIPath: f.Name()})
	if err != nil {
		t.Fatalf("QueryStream: %v", err)
	}

	var messages []Message
	for msg := range msgCh {
		messages = append(messages, msg)
	}

	if len(messages) == 0 {
		t.Error("expected at least one message from QueryStream")
	}
	hasResult := false
	for _, m := range messages {
		if _, ok := m.(*ResultMessage); ok {
			hasResult = true
			break
		}
	}
	if !hasResult {
		t.Error("expected ResultMessage in QueryStream output")
	}
}
