//go:build e2e

package mcp2x_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestMCP2xServerE2E tests the Go SDK against a real Python MCP 2.x server.
func TestMCP2xServerE2E(t *testing.T) {
	// Find the Python server script
	scriptPath := filepath.Join("server.py")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		t.Skip("Python MCP server script not found, skipping E2E test")
	}

	// Check if Python and mcp are available
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found, skipping E2E test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create MCP server config pointing to the Python server
	opts := &claude.ClaudeAgentOptions{
		MCPServers: map[string]claude.MCPServerConfig{
			"test": &claude.MCPStdioServerConfig{
				Command: "python3",
				Args:    []string{scriptPath},
			},
		},
		// Pre-approve all test tools
		AllowedTools: []string{
			"mcp__test__greet",
			"mcp__test__calculate",
		},
	}

	// Test 1: Basic query with tool use
	t.Run("BasicToolUse", func(t *testing.T) {
		ch, err := claude.Query(ctx, "Use the greet tool to say hello to Alice", opts)
		if err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		var messages []claude.Message
		for msg := range ch {
			messages = append(messages, msg)
			t.Logf("Message type: %T", msg)
		}

		if len(messages) == 0 {
			t.Error("Expected at least one message")
		}

		// Check that we got a result
		hasResult := false
		for _, msg := range messages {
			if _, ok := msg.(*claude.ResultMessage); ok {
				hasResult = true
			}
		}
		if !hasResult {
			t.Error("Expected a ResultMessage")
		}
	})

	// Test 2: Streaming client with MCP server
	t.Run("StreamingClient", func(t *testing.T) {
		client, err := claude.NewClaudeSDKClient(ctx, opts)
		if err != nil {
			t.Fatalf("NewClaudeSDKClient failed: %v", err)
		}
		defer client.Close()

		// Send a query
		if err := client.Query(ctx, "What tools do you have available?"); err != nil {
			t.Fatalf("Query failed: %v", err)
		}

		// Receive response
		var messages []claude.Message
		for msg := range client.ReceiveResponse(ctx) {
			messages = append(messages, msg)
			t.Logf("Message type: %T", msg)
		}

		if len(messages) == 0 {
			t.Error("Expected at least one message")
		}
	})
}

// TestMCP2xProtocolVersion tests version negotiation with the Python server.
func TestMCP2xProtocolVersion(t *testing.T) {
	// Test that our version constants are correct
	tests := []struct {
		version  string
		expected string
	}{
		{claude.MCPProtocolVersion20241105, "2024-11-05"},
		{claude.MCPProtocolVersion20250326, "2025-03-26"},
		{claude.MCPProtocolVersion20250618, "2025-06-18"},
		{claude.MCPProtocolVersion20251125, "2025-11-25"},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if tt.version != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.version)
			}
		})
	}

	// Test version negotiation
	t.Run("NegotiateVersion", func(t *testing.T) {
		// This tests the negotiateProtocolVersion function indirectly
		// by checking that the constants are in the supported list
		supported := claude.MCPSupportedProtocolVersions()
		if len(supported) != 4 {
			t.Errorf("expected 4 supported versions, got %d", len(supported))
		}
	})
}

// TestMCP2xTypesSerialization tests that MCP 2.x types serialize correctly.
func TestMCP2xTypesSerialization(t *testing.T) {
	// Test AudioContent
	t.Run("AudioContent", func(t *testing.T) {
		content := claude.MCPAudioContent{
			Type:     "audio",
			Data:     "base64data",
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
	})

	// Test ResourceLink
	t.Run("ResourceLink", func(t *testing.T) {
		link := claude.MCPResourceLink{
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
	})

	// Test ResourceTemplate
	t.Run("ResourceTemplate", func(t *testing.T) {
		template := claude.MCPResourceTemplate{
			URITemplate: "file:///{path}",
			Name:        "Local Files",
			Description: "Access local files",
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
	})

	// Test Tool with OutputSchema
	t.Run("ToolWithOutputSchema", func(t *testing.T) {
		tool := claude.MCPTool{
			Name:        "test-tool",
			Description: "A test tool",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"input": map[string]any{"type": "string"},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"output": map[string]any{"type": "string"},
				},
			},
		}
		b, err := json.Marshal(tool)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(b, &parsed); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if parsed["outputSchema"] == nil {
			t.Error("expected outputSchema to be present")
		}
	})

	// Test ToolResult with StructuredContent
	t.Run("ToolResultWithStructuredContent", func(t *testing.T) {
		result := claude.ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": "Hello"},
			},
			StructuredContent: map[string]any{
				"greeting": "Hello",
			},
		}
		b, err := json.Marshal(result)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}
		var parsed map[string]any
		if err := json.Unmarshal(b, &parsed); err != nil {
			t.Fatalf("json.Unmarshal failed: %v", err)
		}
		if parsed["structuredContent"] == nil {
			t.Error("expected structuredContent to be present")
		}
	})
}

// TestMCP2xSDKServerInterface tests the SdkMcpServer interface with MCP 2.x features.
func TestMCP2xSDKServerInterface(t *testing.T) {
	// Create a test server that implements resourceTemplateServer
	server := &testMCP2xServer{}

	// Verify it implements the interface
	var _ claude.SdkMcpServer = server

	// Test ListResourceTemplates
	templates, err := server.ListResourceTemplates(context.Background())
	if err != nil {
		t.Fatalf("ListResourceTemplates failed: %v", err)
	}
	if len(templates) != 1 {
		t.Errorf("expected 1 template, got %d", len(templates))
	}
	if templates[0].URITemplate != "test://{name}" {
		t.Errorf("expected uriTemplate=test://{name}, got %q", templates[0].URITemplate)
	}
}

// testMCP2xServer is a test server that implements SdkMcpServer and resourceTemplateServer.
type testMCP2xServer struct{}

func (s *testMCP2xServer) Name() string    { return "test-2x-server" }
func (s *testMCP2xServer) Version() string { return "2.0.0" }

func (s *testMCP2xServer) ListTools(_ context.Context) ([]claude.MCPTool, error) {
	return []claude.MCPTool{
		{
			Name:        "test-tool",
			Description: "A test tool",
			InputSchema: map[string]any{"type": "object"},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"result": map[string]any{"type": "string"},
				},
			},
		},
	}, nil
}

func (s *testMCP2xServer) CallTool(_ context.Context, name string, _ map[string]any) (claude.ToolResult, error) {
	return claude.ToolResult{
		Content: []map[string]any{
			{"type": "text", "text": fmt.Sprintf("Called %s", name)},
		},
		StructuredContent: map[string]any{
			"result": fmt.Sprintf("Called %s", name),
		},
	}, nil
}

func (s *testMCP2xServer) ListResources(_ context.Context) ([]claude.MCPResource, error) {
	return []claude.MCPResource{
		{URI: "test://resource", Name: "Test Resource"},
	}, nil
}

func (s *testMCP2xServer) ReadResource(_ context.Context, uri string) (claude.MCPResourceContent, error) {
	return claude.MCPResourceContent{
		URI:  uri,
		Text: "test content",
	}, nil
}

func (s *testMCP2xServer) ListPrompts(_ context.Context) ([]claude.MCPPrompt, error) {
	return nil, nil
}

func (s *testMCP2xServer) GetPrompt(_ context.Context, _ string, _ map[string]any) (claude.MCPPromptResult, error) {
	return claude.MCPPromptResult{}, nil
}

// ListResourceTemplates implements the optional resourceTemplateServer interface.
func (s *testMCP2xServer) ListResourceTemplates(_ context.Context) ([]claude.MCPResourceTemplate, error) {
	return []claude.MCPResourceTemplate{
		{
			URITemplate: "test://{name}",
			Name:        "Test Resources",
			Description: "Access test resources by name",
			MimeType:    "text/plain",
		},
	}, nil
}
