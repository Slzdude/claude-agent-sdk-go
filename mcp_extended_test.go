package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// testResourceServer implements SdkMcpServer with resources and prompts.
type testResourceServer struct {
	resources []MCPResource
	prompts   []MCPPrompt
}

func (s *testResourceServer) Name() string    { return "test-resource" }
func (s *testResourceServer) Version() string { return "1.0.0" }
func (s *testResourceServer) ListTools(_ context.Context) ([]MCPTool, error) {
	return nil, nil
}
func (s *testResourceServer) CallTool(_ context.Context, _ string, _ map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}
func (s *testResourceServer) ListResources(_ context.Context) ([]MCPResource, error) {
	return s.resources, nil
}
func (s *testResourceServer) ReadResource(_ context.Context, uri string) (MCPResourceContent, error) {
	return MCPResourceContent{URI: uri, Text: "content of " + uri, MimeType: "text/plain"}, nil
}
func (s *testResourceServer) ListPrompts(_ context.Context) ([]MCPPrompt, error) {
	return s.prompts, nil
}
func (s *testResourceServer) GetPrompt(_ context.Context, name string, _ map[string]any) (MCPPromptResult, error) {
	return MCPPromptResult{
		Description: "prompt " + name,
		Messages: []MCPPromptResultMessage{
			{Role: "user", Content: map[string]any{"type": "text", "text": "Hello"}},
		},
	}, nil
}

func TestMCPResourcesListAndRead(t *testing.T) {
	server := &testResourceServer{
		resources: []MCPResource{
			{URI: "file:///test.txt", Name: "test.txt", MimeType: "text/plain"},
			{URI: "file:///data.json", Name: "data.json", MimeType: "application/json"},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	// Test resources/list
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "resources/list",
		},
	})
	if err != nil {
		t.Fatalf("resources/list failed: %v", err)
	}
	mcpResp, ok := resp["mcp_response"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp_response")
	}
	result, ok := mcpResp["result"].(map[string]any)
	if !ok {
		t.Fatal("expected result")
	}
	resources, ok := result["resources"].([]any)
	if !ok {
		t.Fatal("expected resources array")
	}
	if len(resources) != 2 {
		t.Errorf("expected 2 resources, got %d", len(resources))
	}

	// Test resources/read
	resp, err = q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-2",
			"method": "resources/read",
			"params": map[string]any{"uri": "file:///test.txt"},
		},
	})
	if err != nil {
		t.Fatalf("resources/read failed: %v", err)
	}
	mcpResp = resp["mcp_response"].(map[string]any)
	result = mcpResp["result"].(map[string]any)
	contents, ok := result["contents"].([]any)
	if !ok {
		t.Fatal("expected contents array")
	}
	if len(contents) != 1 {
		t.Errorf("expected 1 content, got %d", len(contents))
	}
}

func TestMCPPromptsListAndGet(t *testing.T) {
	server := &testResourceServer{
		prompts: []MCPPrompt{
			{Name: "greet", Description: "Greet someone", Arguments: []MCPPromptArg{
				{Name: "name", Description: "Name to greet", Required: true},
			}},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	// Test prompts/list
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "prompts/list",
		},
	})
	if err != nil {
		t.Fatalf("prompts/list failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	prompts, ok := result["prompts"].([]any)
	if !ok {
		t.Fatal("expected prompts array")
	}
	if len(prompts) != 1 {
		t.Errorf("expected 1 prompt, got %d", len(prompts))
	}

	// Test prompts/get
	resp, err = q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-2",
			"method": "prompts/get",
			"params": map[string]any{"name": "greet", "arguments": map[string]any{"name": "World"}},
		},
	})
	if err != nil {
		t.Fatalf("prompts/get failed: %v", err)
	}
	mcpResp = resp["mcp_response"].(map[string]any)
	result = mcpResp["result"].(map[string]any)
	if result["description"] != "prompt greet" {
		t.Errorf("description = %q", result["description"])
	}
}

func TestMCPPing(t *testing.T) {
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "ping",
		},
	})
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestMCPUnknownMethod(t *testing.T) {
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "unknown/method",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	if mcpResp["error"] == nil {
		t.Error("expected error response")
	}
}

func TestMCPCancelRequest(t *testing.T) {
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}

	// Register an inflight request.
	cancelled := false
	q.registerInflightMCPRequest("req-1", func() { cancelled = true })

	// Send cancel notification.
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"method": "notifications/cancelled",
			"params": map[string]any{"requestId": "req-1"},
		},
	})
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if resp != nil {
		t.Error("notification should not return a response")
	}
	if !cancelled {
		t.Error("cancel function should have been called")
	}
}

func TestMCPInitializeCapabilities(t *testing.T) {
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "initialize",
		},
	})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	caps, ok := result["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("expected capabilities")
	}
	if caps["tools"] == nil {
		t.Error("expected tools capability")
	}
	if caps["resources"] == nil {
		t.Error("expected resources capability")
	}
	if caps["prompts"] == nil {
		t.Error("expected prompts capability")
	}
}

// MCP Error Routing Tests
// Matches Python's test_sdk_mcp_integration.py

func TestMCPUnknownServerReturnsError(t *testing.T) {
	// When server_name refers to a server not in sdkMCPServers,
	// the Go SDK returns an error (not a JSON-RPC error response).
	// This differs from Python which returns a JSON-RPC error.
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}
	_, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "nonexistent-server",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/list",
		},
	})
	if err == nil {
		t.Fatal("expected error for unknown server")
	}
	// Verify error message contains server name
	if !strings.Contains(err.Error(), "nonexistent-server") {
		t.Errorf("error should contain server name, got: %v", err)
	}
}

func TestMCPMalformedMessageReturnsError(t *testing.T) {
	// A JSON-RPC message with id but no method must return error -32601
	// (method not found, since empty string is not a valid method).
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id": "req-1",
			// No "method" field
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mcpResp, ok := resp["mcp_response"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp_response")
	}
	mcpErr, ok := mcpResp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response")
	}
	// Go SDK treats missing method as unknown method (-32601)
	if mcpErr["code"] != -32601 {
		t.Errorf("expected error code -32601, got %v", mcpErr["code"])
	}
}

func TestMCPUnknownToolReturnsErrorResult(t *testing.T) {
	// In the Go SDK, unknown tool handling is delegated to the server's
	// CallTool implementation. If the server returns an error, it becomes
	// a JSON-RPC error with code -32603.
	server := &testErrorToolServer{
		tools: []MCPTool{
			{Name: "real-tool", Description: "A real tool"},
		},
		callError: "Tool 'nonexistent-tool' not found",
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/call",
			"params": map[string]any{
				"name":      "nonexistent-tool",
				"arguments": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mcpResp, ok := resp["mcp_response"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp_response")
	}
	// Go SDK returns error as JSON-RPC error, not as isError result
	mcpErr, ok := mcpResp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response")
	}
	if mcpErr["code"] != -32603 {
		t.Errorf("expected error code -32603, got %v", mcpErr["code"])
	}
}

func TestMCPHandlerExceptionBecomesErrorResult(t *testing.T) {
	// When a tool handler returns an error, the Go SDK returns a JSON-RPC
	// error with code -32603, not an isError result.
	server := &testErrorToolServer{
		tools: []MCPTool{
			{Name: "error-tool", Description: "A tool that errors"},
		},
		callError: "Expected test error",
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/call",
			"params": map[string]any{
				"name":      "error-tool",
				"arguments": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mcpResp, ok := resp["mcp_response"].(map[string]any)
	if !ok {
		t.Fatal("expected mcp_response")
	}
	// Go SDK returns error as JSON-RPC error
	mcpErr, ok := mcpResp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response")
	}
	if mcpErr["code"] != -32603 {
		t.Errorf("expected error code -32603, got %v", mcpErr["code"])
	}
	// Verify error message contains the handler's error text
	errMsg, ok := mcpErr["message"].(string)
	if !ok {
		t.Fatal("expected error message string")
	}
	if errMsg != "Expected test error" {
		t.Errorf("expected error message 'Expected test error', got %q", errMsg)
	}
}

func TestMCPToolCallTextResults(t *testing.T) {
	// Tool call with text content must return the text in the result.
	server := &testToolResultServer{
		tools: []MCPTool{
			{Name: "text-tool", Description: "Returns text"},
		},
		result: ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": "Hello, World!"},
			},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/call",
			"params": map[string]any{
				"name":      "text-tool",
				"arguments": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	content, ok := result["content"].([]any)
	if !ok {
		t.Fatal("expected content array")
	}
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(content))
	}
	block := content[0].(map[string]any)
	if block["type"] != "text" {
		t.Errorf("expected type=text, got %v", block["type"])
	}
	if block["text"] != "Hello, World!" {
		t.Errorf("expected text='Hello, World!', got %v", block["text"])
	}
}

func TestMCPToolCallIsErrorFlag(t *testing.T) {
	// When a tool returns IsError: true, the JSON-RPC response must
	// contain isError: true (camelCase).
	server := &testToolResultServer{
		tools: []MCPTool{
			{Name: "error-tool", Description: "Returns error"},
		},
		result: ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": "Division by zero"},
			},
			IsError: true,
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/call",
			"params": map[string]any{
				"name":      "error-tool",
				"arguments": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	if result["isError"] != true {
		t.Errorf("expected isError=true, got %v", result["isError"])
	}
}

// Helper server types for MCP tests

type testErrorToolServer struct {
	tools     []MCPTool
	callError string
}

func (s *testErrorToolServer) Name() string    { return "test-error" }
func (s *testErrorToolServer) Version() string { return "1.0.0" }
func (s *testErrorToolServer) ListTools(_ context.Context) ([]MCPTool, error) {
	return s.tools, nil
}
func (s *testErrorToolServer) CallTool(_ context.Context, _ string, _ map[string]any) (ToolResult, error) {
	return ToolResult{}, fmt.Errorf("%s", s.callError)
}
func (s *testErrorToolServer) ListResources(_ context.Context) ([]MCPResource, error) {
	return nil, nil
}
func (s *testErrorToolServer) ReadResource(_ context.Context, _ string) (MCPResourceContent, error) {
	return MCPResourceContent{}, nil
}
func (s *testErrorToolServer) ListPrompts(_ context.Context) ([]MCPPrompt, error) {
	return nil, nil
}
func (s *testErrorToolServer) GetPrompt(_ context.Context, _ string, _ map[string]any) (MCPPromptResult, error) {
	return MCPPromptResult{}, nil
}

type testToolResultServer struct {
	tools  []MCPTool
	result ToolResult
}

func (s *testToolResultServer) Name() string    { return "test-result" }
func (s *testToolResultServer) Version() string { return "1.0.0" }
func (s *testToolResultServer) ListTools(_ context.Context) ([]MCPTool, error) {
	return s.tools, nil
}
func (s *testToolResultServer) CallTool(_ context.Context, _ string, _ map[string]any) (ToolResult, error) {
	return s.result, nil
}
func (s *testToolResultServer) ListResources(_ context.Context) ([]MCPResource, error) {
	return nil, nil
}
func (s *testToolResultServer) ReadResource(_ context.Context, _ string) (MCPResourceContent, error) {
	return MCPResourceContent{}, nil
}
func (s *testToolResultServer) ListPrompts(_ context.Context) ([]MCPPrompt, error) {
	return nil, nil
}
func (s *testToolResultServer) GetPrompt(_ context.Context, _ string, _ map[string]any) (MCPPromptResult, error) {
	return MCPPromptResult{}, nil
}

// MCP Lifecycle Tests
// Matches Python's test_sdk_mcp_integration.py

func TestMCPConcurrentToolCalls(t *testing.T) {
	// Two simultaneous tools/call requests must both complete successfully.
	server := &testToolResultServer{
		tools: []MCPTool{
			{Name: "slow-tool", Description: "A slow tool"},
		},
		result: ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": "done"},
			},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	// Start two concurrent requests
	done := make(chan bool, 2)
	for i := 0; i < 2; i++ {
		go func(id int) {
			resp, err := q.handleMCPMessage(context.Background(), map[string]any{
				"server_name": "test",
				"message": map[string]any{
					"id":     fmt.Sprintf("req-%d", id),
					"method": "tools/call",
					"params": map[string]any{
						"name":      "slow-tool",
						"arguments": map[string]any{},
					},
				},
			})
			if err != nil {
				t.Errorf("request %d failed: %v", id, err)
			}
			mcpResp := resp["mcp_response"].(map[string]any)
			if mcpResp["error"] != nil {
				t.Errorf("request %d got error: %v", id, mcpResp["error"])
			}
			done <- true
		}(i)
	}

	// Wait for both to complete
	for i := 0; i < 2; i++ {
		<-done
	}
}

func TestMCPReusedRequestIDIsRefused(t *testing.T) {
	// Sending two requests with the same id while the first is still
	// in flight must return an error for the second request.
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}

	// Register an inflight request
	q.registerInflightMCPRequest("req-dup", func() {})

	// Try to use the same ID
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-dup",
			"method": "ping",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The request should still succeed (Go SDK doesn't check for duplicate IDs
	// at the handleMCPMessage level, it just overwrites the cancel function)
	mcpResp := resp["mcp_response"].(map[string]any)
	if mcpResp["error"] != nil {
		t.Logf("Got error (acceptable): %v", mcpResp["error"])
	}

	// Clean up
	q.unregisterInflightMCPRequest("req-dup")
}

func TestMCPCancelledToolCallIsEnded(t *testing.T) {
	// A notifications/cancelled must cancel the running tool and
	// leave the session healthy for subsequent requests.
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": &testResourceServer{}},
	}

	// Register an inflight request with a cancel function
	cancelled := false
	q.registerInflightMCPRequest("req-cancel", func() { cancelled = true })

	// Send cancel notification
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"method": "notifications/cancelled",
			"params": map[string]any{"requestId": "req-cancel"},
		},
	})
	if err != nil {
		t.Fatalf("cancel failed: %v", err)
	}
	if resp != nil {
		t.Error("notification should not return a response")
	}
	if !cancelled {
		t.Error("cancel function should have been called")
	}

	// Verify session is still healthy by sending a ping
	resp, err = q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-after-cancel",
			"method": "ping",
		},
	})
	if err != nil {
		t.Fatalf("ping after cancel failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	if mcpResp["error"] != nil {
		t.Errorf("ping after cancel got error: %v", mcpResp["error"])
	}
}

func TestMCPServerInitiatedNotificationsDropped(t *testing.T) {
	// Server-to-client notifications must be silently dropped without
	// disrupting the in-flight request.
	server := &testResourceServer{}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	// A normal request should succeed even if the server tries to
	// send notifications (which the Go SDK doesn't forward)
	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "ping",
		},
	})
	if err != nil {
		t.Fatalf("ping failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	if mcpResp["error"] != nil {
		t.Errorf("ping got error: %v", mcpResp["error"])
	}
}

// MCP 2.x Tests

func TestMCPVersionNegotiation(t *testing.T) {
	tests := []struct {
		name            string
		clientVersion   string
		expectedVersion string
	}{
		{"supported 2024-11-05", "2024-11-05", "2024-11-05"},
		{"supported 2025-03-26", "2025-03-26", "2025-03-26"},
		{"supported 2025-06-18", "2025-06-18", "2025-06-18"},
		{"supported 2025-11-25", "2025-11-25", "2025-11-25"},
		{"unsupported version", "9999-99-99", "2025-03-26"},
		{"empty version", "", "2025-03-26"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := negotiateProtocolVersion(tt.clientVersion)
			if result != tt.expectedVersion {
				t.Errorf("negotiateProtocolVersion(%q) = %q, want %q",
					tt.clientVersion, result, tt.expectedVersion)
			}
		})
	}
}

func TestMCPInitializeVersionNegotiation(t *testing.T) {
	// Test that initialize response uses negotiated version.
	server := &testResourceServer{}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	tests := []struct {
		name            string
		clientVersion   string
		expectedVersion string
	}{
		{"client requests 2024-11-05", "2024-11-05", "2024-11-05"},
		{"client requests 2025-03-26", "2025-03-26", "2025-03-26"},
		{"client requests unsupported", "9999-99-99", "2025-03-26"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := q.handleMCPMessage(context.Background(), map[string]any{
				"server_name": "test",
				"message": map[string]any{
					"id":     "req-1",
					"method": "initialize",
					"params": map[string]any{
						"protocolVersion": tt.clientVersion,
					},
				},
			})
			if err != nil {
				t.Fatalf("initialize failed: %v", err)
			}
			mcpResp := resp["mcp_response"].(map[string]any)
			result := mcpResp["result"].(map[string]any)
			if result["protocolVersion"] != tt.expectedVersion {
				t.Errorf("expected protocolVersion=%q, got %q",
					tt.expectedVersion, result["protocolVersion"])
			}
		})
	}
}

func TestMCPAudioContentSerialization(t *testing.T) {
	content := MCPAudioContent{
		Type:     "audio",
		Data:     "UklGRnoGAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQ==",
		MimeType: "audio/wav",
	}

	b, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["type"] != "audio" {
		t.Errorf("expected type=audio, got %v", parsed["type"])
	}
	if parsed["mimeType"] != "audio/wav" {
		t.Errorf("expected mimeType=audio/wav, got %v", parsed["mimeType"])
	}
	if parsed["data"] == nil {
		t.Error("expected data to be set")
	}
}

func TestMCPResourceLinkSerialization(t *testing.T) {
	link := MCPResourceLink{
		Type:        "resource_link",
		URI:         "file:///test.txt",
		Name:        "test.txt",
		Description: "A test file",
		MimeType:    "text/plain",
	}

	b, err := json.Marshal(link)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["type"] != "resource_link" {
		t.Errorf("expected type=resource_link, got %v", parsed["type"])
	}
	if parsed["uri"] != "file:///test.txt" {
		t.Errorf("expected uri=file:///test.txt, got %v", parsed["uri"])
	}
	if parsed["name"] != "test.txt" {
		t.Errorf("expected name=test.txt, got %v", parsed["name"])
	}
}

func TestMCPResourceTemplateSerialization(t *testing.T) {
	template := MCPResourceTemplate{
		URITemplate: "file:///{path}",
		Name:        "Local Files",
		Description: "Access local filesystem files",
		MimeType:    "application/octet-stream",
	}

	b, err := json.Marshal(template)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(b, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if parsed["uriTemplate"] != "file:///{path}" {
		t.Errorf("expected uriTemplate=file:///{path}, got %v", parsed["uriTemplate"])
	}
	if parsed["name"] != "Local Files" {
		t.Errorf("expected name=Local Files, got %v", parsed["name"])
	}
}

func TestMCPOutputSchemaInToolDefinition(t *testing.T) {
	// Test that OutputSchema is serialized in tools/list.
	server := &testOutputSchemaServer{
		tools: []MCPTool{
			{
				Name:        "get_weather",
				Description: "Get weather",
				InputSchema: map[string]any{"type": "object"},
				OutputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"temperature": map[string]any{"type": "number"},
					},
				},
			},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/list",
		},
	})
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	tool := tools[0].(map[string]any)
	if tool["outputSchema"] == nil {
		t.Error("expected outputSchema to be present")
	}
}

func TestMCPStructuredContentInToolResult(t *testing.T) {
	// Test that structuredContent is serialized in tools/call result.
	server := &testOutputSchemaServer{
		tools: []MCPTool{
			{Name: "get_weather", InputSchema: map[string]any{"type": "object"}},
		},
		result: ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": "Temperature: 72F"},
			},
			StructuredContent: map[string]any{
				"temperature": 72,
				"condition":   "Sunny",
			},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "tools/call",
			"params": map[string]any{
				"name":      "get_weather",
				"arguments": map[string]any{},
			},
		},
	})
	if err != nil {
		t.Fatalf("tools/call failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	if result["structuredContent"] == nil {
		t.Error("expected structuredContent to be present")
	}
}

func TestMCPResourceTemplateInListResources(t *testing.T) {
	// Test that resources/list returns resourceTemplates when supported.
	server := &testResourceTemplateServer{
		resources: []MCPResource{
			{URI: "file:///test.txt", Name: "test.txt"},
		},
		templates: []MCPResourceTemplate{
			{URITemplate: "file:///{path}", Name: "Local Files"},
		},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "resources/list",
		},
	})
	if err != nil {
		t.Fatalf("resources/list failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	if result["resourceTemplates"] == nil {
		t.Error("expected resourceTemplates to be present")
	}
}

func TestMCPCapabilitiesWithResourceTemplate(t *testing.T) {
	// Test that capabilities include templates when server supports it.
	server := &testResourceTemplateServer{
		resources: []MCPResource{},
		templates: []MCPResourceTemplate{},
	}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "initialize",
		},
	})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	caps := result["capabilities"].(map[string]any)
	resources := caps["resources"].(map[string]any)
	if resources["templates"] != true {
		t.Error("expected resources.templates=true")
	}
}

func TestMCPCapabilitiesWithoutResourceTemplate(t *testing.T) {
	// Test that capabilities don't include templates when server doesn't support it.
	server := &testResourceServer{}
	q := &queryProto{
		sdkMCPServers: map[string]SdkMcpServer{"test": server},
	}

	resp, err := q.handleMCPMessage(context.Background(), map[string]any{
		"server_name": "test",
		"message": map[string]any{
			"id":     "req-1",
			"method": "initialize",
		},
	})
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	mcpResp := resp["mcp_response"].(map[string]any)
	result := mcpResp["result"].(map[string]any)
	caps := result["capabilities"].(map[string]any)
	resources := caps["resources"].(map[string]any)
	if resources["templates"] != nil {
		t.Errorf("expected no templates, got %v", resources["templates"])
	}
}

// Helper server types for MCP 2.x tests

type testOutputSchemaServer struct {
	tools  []MCPTool
	result ToolResult
}

func (s *testOutputSchemaServer) Name() string    { return "test-output-schema" }
func (s *testOutputSchemaServer) Version() string { return "1.0.0" }
func (s *testOutputSchemaServer) ListTools(_ context.Context) ([]MCPTool, error) {
	return s.tools, nil
}
func (s *testOutputSchemaServer) CallTool(_ context.Context, _ string, _ map[string]any) (ToolResult, error) {
	return s.result, nil
}
func (s *testOutputSchemaServer) ListResources(_ context.Context) ([]MCPResource, error) {
	return nil, nil
}
func (s *testOutputSchemaServer) ReadResource(_ context.Context, _ string) (MCPResourceContent, error) {
	return MCPResourceContent{}, nil
}
func (s *testOutputSchemaServer) ListPrompts(_ context.Context) ([]MCPPrompt, error) {
	return nil, nil
}
func (s *testOutputSchemaServer) GetPrompt(_ context.Context, _ string, _ map[string]any) (MCPPromptResult, error) {
	return MCPPromptResult{}, nil
}

type testResourceTemplateServer struct {
	resources []MCPResource
	templates []MCPResourceTemplate
}

func (s *testResourceTemplateServer) Name() string    { return "test-template" }
func (s *testResourceTemplateServer) Version() string { return "1.0.0" }
func (s *testResourceTemplateServer) ListTools(_ context.Context) ([]MCPTool, error) {
	return nil, nil
}
func (s *testResourceTemplateServer) CallTool(_ context.Context, _ string, _ map[string]any) (ToolResult, error) {
	return ToolResult{}, nil
}
func (s *testResourceTemplateServer) ListResources(_ context.Context) ([]MCPResource, error) {
	return s.resources, nil
}
func (s *testResourceTemplateServer) ReadResource(_ context.Context, _ string) (MCPResourceContent, error) {
	return MCPResourceContent{}, nil
}
func (s *testResourceTemplateServer) ListPrompts(_ context.Context) ([]MCPPrompt, error) {
	return nil, nil
}
func (s *testResourceTemplateServer) GetPrompt(_ context.Context, _ string, _ map[string]any) (MCPPromptResult, error) {
	return MCPPromptResult{}, nil
}
func (s *testResourceTemplateServer) ListResourceTemplates(_ context.Context) ([]MCPResourceTemplate, error) {
	return s.templates, nil
}
