package claude

import (
	"context"
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
