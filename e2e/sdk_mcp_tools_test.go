//go:build e2e

package e2e_test

// sdk_mcp_tools_test.go mirrors e2e-tests/test_sdk_mcp_tools.py.
//
// Go replaces Python's @tool decorator + create_sdk_mcp_server() with a
// simpleServer struct that implements SdkMcpServer directly.

import (
	"context"
	"fmt"
	"sync"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// simpleServer is a minimal in-process MCP server for e2e testing.
type simpleServer struct {
	name    string
	version string
	tools   []claude.MCPTool
	impls   map[string]func(context.Context, map[string]any) (claude.ToolResult, error)
	execMu  sync.Mutex
	execLog []string
}

func (s *simpleServer) Name() string    { return s.name }
func (s *simpleServer) Version() string { return s.version }
func (s *simpleServer) ListTools(_ context.Context) ([]claude.MCPTool, error) {
	return s.tools, nil
}
func (s *simpleServer) CallTool(ctx context.Context, name string, args map[string]any) (claude.ToolResult, error) {
	s.execMu.Lock()
	s.execLog = append(s.execLog, name)
	s.execMu.Unlock()
	if fn, ok := s.impls[name]; ok {
		return fn(ctx, args)
	}
	return claude.ToolResult{}, fmt.Errorf("unknown tool: %s", name)
}

// newSimpleServer creates a test server with echo and greet tools.
func newSimpleServer(name, version string) *simpleServer {
	srv := &simpleServer{
		name:    name,
		version: version,
		impls:   make(map[string]func(context.Context, map[string]any) (claude.ToolResult, error)),
	}
	srv.tools = []claude.MCPTool{
		{
			Name:        "echo",
			Description: "Echo back the input text",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
				"required":   []any{"text"},
			},
		},
		{
			Name:        "greet",
			Description: "Greet a person by name",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"name": map[string]any{"type": "string"}},
				"required":   []any{"name"},
			},
		},
	}
	srv.impls["echo"] = func(_ context.Context, args map[string]any) (claude.ToolResult, error) {
		text, _ := args["text"].(string)
		return claude.ToolResult{Content: []map[string]any{{"type": "text", "text": "Echo: " + text}}}, nil
	}
	srv.impls["greet"] = func(_ context.Context, args map[string]any) (claude.ToolResult, error) {
		name, _ := args["name"].(string)
		return claude.ToolResult{Content: []map[string]any{{"type": "text", "text": "Hello, " + name + "!"}}}, nil
	}
	return srv
}

// TestSDKMCPToolExecution tests that SDK MCP tools can be called with allowed_tools.
// Mirrors test_sdk_mcp_tool_execution.
func TestSDKMCPToolExecution(t *testing.T) {
	srv := newSimpleServer("test", "1.0.0")
	// Only expose echo tool.
	srv.tools = srv.tools[:1]

	opts := &claude.ClaudeAgentOptions{
		MCPServers:   map[string]claude.MCPServerConfig{"test": &claude.MCPSdkServerConfig{Instance: srv}},
		AllowedTools: []string{"mcp__test__echo"},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx, "Call the mcp__test__echo tool with any text", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	requireResult(t, msgs)

	srv.execMu.Lock()
	defer srv.execMu.Unlock()
	found := false
	for _, name := range srv.execLog {
		if name == "echo" {
			found = true
		}
	}
	if !found {
		t.Error("Echo tool function was not executed")
	}
}

// TestSDKMCPPermissionEnforcement tests that disallowed_tools prevents SDK MCP
// tool execution. Mirrors test_sdk_mcp_permission_enforcement.
func TestSDKMCPPermissionEnforcement(t *testing.T) {
	srv := newSimpleServer("test", "1.0.0")

	opts := &claude.ClaudeAgentOptions{
		MCPServers:      map[string]claude.MCPServerConfig{"test": &claude.MCPSdkServerConfig{Instance: srv}},
		DisallowedTools: []string{"mcp__test__echo"},
		AllowedTools:    []string{"mcp__test__greet"},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx,
		"First use the greet tool to greet 'Alice'. After that completes, use the echo tool to echo 'test'. Do these one at a time, not in parallel.",
		opts,
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	requireResult(t, msgs)

	srv.execMu.Lock()
	defer srv.execMu.Unlock()
	echoRan := false
	greetRan := false
	for _, name := range srv.execLog {
		if name == "echo" {
			echoRan = true
		}
		if name == "greet" {
			greetRan = true
		}
	}
	if echoRan {
		t.Error("Disallowed echo tool was executed")
	}
	if !greetRan {
		t.Error("Allowed greet tool was not executed")
	}
}

// TestSDKMCPMultipleTools tests that multiple SDK MCP tools can be called.
// Mirrors test_sdk_mcp_multiple_tools.
func TestSDKMCPMultipleTools(t *testing.T) {
	srv := newSimpleServer("multi", "1.0.0")

	opts := &claude.ClaudeAgentOptions{
		MCPServers:   map[string]claude.MCPServerConfig{"multi": &claude.MCPSdkServerConfig{Instance: srv}},
		AllowedTools: []string{"mcp__multi__echo", "mcp__multi__greet"},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx,
		"Call mcp__multi__echo with text='test' and mcp__multi__greet with name='Bob'",
		opts,
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	requireResult(t, msgs)

	srv.execMu.Lock()
	defer srv.execMu.Unlock()
	echoRan := false
	greetRan := false
	for _, name := range srv.execLog {
		if name == "echo" {
			echoRan = true
		}
		if name == "greet" {
			greetRan = true
		}
	}
	if !echoRan {
		t.Error("Echo tool was not executed")
	}
	if !greetRan {
		t.Error("Greet tool was not executed")
	}
}

// TestSDKMCPWithoutPermissions tests SDK MCP tool behavior without explicit
// allowed_tools. Mirrors test_sdk_mcp_without_permissions.
func TestSDKMCPWithoutPermissions(t *testing.T) {
	srv := newSimpleServer("noperm", "1.0.0")
	// Only expose echo tool.
	srv.tools = srv.tools[:1]

	opts := &claude.ClaudeAgentOptions{
		MCPServers: map[string]claude.MCPServerConfig{"noperm": &claude.MCPSdkServerConfig{Instance: srv}},
		// No AllowedTools specified.
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx, "Call the mcp__noperm__echo tool", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	requireResult(t, msgs)

	srv.execMu.Lock()
	defer srv.execMu.Unlock()
	for _, name := range srv.execLog {
		if name == "echo" {
			t.Error("SDK MCP tool was executed without permission")
		}
	}
}
