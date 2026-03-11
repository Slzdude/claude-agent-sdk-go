package claude

// streaming_client_test.go mirrors test_streaming_client.py.
// Tests for ClaudeSDKClient using in-memory mock transports (cross-platform).

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// assistantJSON builds a JSON string for an assistant message with the given text.
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

// resultJSON builds a JSON string for a success result message.
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

// newMockSDKClientSimple creates a *ClaudeSDKClient backed by an in-memory
// mock that responds to the initialize handshake and emits the provided lines.
// This replaces NewClaudeSDKClient(ctx, &ClaudeAgentOptions{CLIPath: script})
// with a cross-platform, subprocess-free equivalent.
func newMockSDKClientSimple(ctx context.Context, t *testing.T, lines ...string) (*ClaudeSDKClient, error) {
	t.Helper()
	return newMockSDKClient(ctx, t, nil, mockTransportWithInit(t, lines...))
}

// newMockSDKClientWithControl creates a *ClaudeSDKClient backed by an in-memory
// mock that responds to initialize, emits lines, then handles one additional
// control_request responding with responseSubtype (e.g. "success").
func newMockSDKClientWithControl(ctx context.Context, t *testing.T, responseSubtype string, lines ...string) (*ClaudeSDKClient, error) {
	t.Helper()
	return newMockSDKClient(ctx, t, nil, mockTransportWithInitAndControl(t, responseSubtype, lines...))
}

// newMockSDKClientHanging creates a *ClaudeSDKClient backed by a mock that
// responds to initialize and then blocks reading stdin until it is closed.
// Use for TestStreamingClient_ContextCancellation.
func newMockSDKClientHanging(ctx context.Context, t *testing.T) (*ClaudeSDKClient, error) {
	t.Helper()
	return newMockSDKClient(ctx, t, nil, mockTransportHanging(t))
}

// TestStreamingClient_BasicConnectClose verifies NewClaudeSDKClient connects
// and Close terminates gracefully.
func TestStreamingClient_BasicConnectClose(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestStreamingClient_DoubleCloseSafe verifies that calling Close twice is safe.
func TestStreamingClient_DoubleCloseSafe(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
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
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t, assistantJSON("Hello from mock"), resultJSON())
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t, assistantJSON(wantText), resultJSON())
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t, assistantJSON("Done"), resultJSON())
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := newMockSDKClientSimple(ctx, t, assistantJSON("Turn 1"), resultJSON())
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx := context.Background()
	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.Interrupt(ctx); err != nil {
		t.Errorf("Interrupt: %v", err)
	}
}

// TestStreamingClient_SetPermissionMode verifies the SetPermissionMode round-trip.
func TestStreamingClient_SetPermissionMode(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SetPermissionMode(ctx, PermissionModeBypassPermissions); err != nil {
		t.Errorf("SetPermissionMode: %v", err)
	}
}

// TestStreamingClient_SetModel verifies the SetModel control round-trip.
func TestStreamingClient_SetModel(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	model := "claude-opus-4-20250514"
	if err := client.SetModel(ctx, &model); err != nil {
		t.Errorf("SetModel: %v", err)
	}
}

// TestStreamingClient_SetModelNil verifies passing nil model (reset to default).
func TestStreamingClient_SetModelNil(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SetModel(ctx, nil); err != nil {
		t.Errorf("SetModel(nil): %v", err)
	}
}

// TestStreamingClient_ReconnectMcpServer verifies the ReconnectMcpServer round-trip.
func TestStreamingClient_ReconnectMcpServer(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.ReconnectMcpServer(ctx, "my-server"); err != nil {
		t.Errorf("ReconnectMcpServer: %v", err)
	}
}

// TestStreamingClient_ToggleMcpServer verifies the ToggleMcpServer round-trip.
func TestStreamingClient_ToggleMcpServer(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.ToggleMcpServer(ctx, "my-server", false); err != nil {
		t.Errorf("ToggleMcpServer: %v", err)
	}
}

// TestStreamingClient_QueryThenReceive verifies the full Query → ReceiveResponse cycle.
func TestStreamingClient_QueryThenReceive(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientSimple(ctx, t, assistantJSON("Query result"), resultJSON())
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
// ReceiveResponse to return promptly by terminating the mock transport.
// The in-memory hanging mock blocks reading stdin until it is closed,
// which happens when Close() calls closeStdin().
func TestStreamingClient_ContextCancellation(t *testing.T) {
	ctx := context.Background()
	client, err := newMockSDKClientHanging(ctx, t)
	if err != nil {
		t.Fatalf("newMockSDKClientHanging: %v", err)
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
	_ = client.Close()

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
	ctx := context.Background()

	c1, err := newMockSDKClientSimple(ctx, t)
	if err != nil {
		t.Fatalf("first newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = c1.Close() }()

	c2, err := newMockSDKClientSimple(ctx, t)
	if err != nil {
		t.Fatalf("second newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = c2.Close() }()
}

// TestStreamingClient_ContextManagerWithException mirrors test_context_manager_with_exception:
// verifies that Close is called even when the using code panics (deferred close pattern).
func TestStreamingClient_ContextManagerWithException(t *testing.T) {
	ctx := context.Background()

	closed := false
	func() {
		client, err := newMockSDKClientSimple(ctx, t)
		if err != nil {
			t.Fatalf("newMockSDKClientSimple: %v", err)
		}
		defer func() {
			_ = client.Close()
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
	ctx := context.Background()

	client, err := newMockSDKClientWithControl(ctx, t, "success")
	if err != nil {
		t.Fatalf("newMockSDKClientWithControl: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.StopTask(ctx, "task-abc123"); err != nil {
		t.Errorf("StopTask: %v", err)
	}
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

	tr := mockTransportWithMcpStatus(t, string(b))
	ctx := context.Background()

	client, err := newMockSDKClient(ctx, t, nil, tr)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx := context.Background()

	client, err := newMockSDKClientSimple(ctx, t,
		assistantJSON("Answer"),
		resultJSON(),
		// This message appears after ResultMessage, and should NOT be yielded by ReceiveResponse.
		assistantJSON("Should not appear"),
	)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
	ctx := context.Background()
	// The mock emits assistant+result immediately after init; Query write and ReceiveResponse read overlap.
	client, err := newMockSDKClientSimple(ctx, t,
		assistantJSON("Response 1"),
		resultJSON(),
	)
	if err != nil {
		t.Fatalf("newMockSDKClientSimple: %v", err)
	}
	defer func() { _ = client.Close() }()

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
		// Broken-pipe is acceptable since the mock may have exited already.
		t.Logf("Query error (may be expected): %v", err)
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
	promptCh := make(chan map[string]any, 1)
	promptCh <- map[string]any{
		"type":    "user",
		"message": map[string]any{"role": "user", "content": "Hello"},
	}
	close(promptCh)

	tr := mockTransportWithInit(t,
		assistantJSON("Hello from stream"),
		resultJSON(),
	)

	ctx := context.Background()
	msgCh, err := mockQueryStream(ctx, t, promptCh, tr, nil)
	if err != nil {
		t.Fatalf("mockQueryStream: %v", err)
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
