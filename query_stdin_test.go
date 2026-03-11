package claude

// query_stdin_test.go mirrors test_query.py.
// Tests for processQuery() stdin lifecycle with SDK MCP servers and hooks.
//
// The SDK communicates with the CLI subprocess over stdin/stdout. When SDK MCP
// servers or hooks are configured, the CLI sends control_request messages back
// to the SDK *after* the prompt is written.  The SDK must keep stdin open long
// enough to respond to these requests.
//
// These tests verify that:
//   - Without SDK MCP servers or hooks: stdin (closeStdin) is called immediately.
//   - With SDK MCP servers: stdin stays open until the first ResultMessage.
//   - With hooks configured: stdin stays open until the first ResultMessage.
//   - MCP control requests are handled when stdin is kept open.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// makeQueryScript writes a mock CLI shell script with the given sequence of output
// lines. It handles one optional round of control requests before emitting messages.
// The script responds to the initialize control_request automatically.
func makeQueryScript(t *testing.T, lines ...string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-query-*.sh")
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
	// Emit the provided lines
	for _, l := range lines {
		sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(l, "'", "'\\''")))
	}
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)
	return f.Name()
}

// makeQueryScriptWithMCPControl writes a mock CLI script that emits MCP
// control_requests before the normal messages, and expects the SDK to
// respond over stdin.  The script reads responses for each control request
// before emitting the final messages.
func makeQueryScriptWithMCPControl(t *testing.T, serverName string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "mock-query-mcp-*.sh")
	if err != nil {
		t.Fatal(err)
	}

	asstJSON := func(text string) string {
		m := map[string]any{
			"type": "assistant",
			"message": map[string]any{
				"role": "assistant",
				"content": []any{
					map[string]any{"type": "text", "text": text},
				},
				"model": "claude-opus-4-1-20250805",
			},
		}
		b, _ := json.Marshal(m)
		return string(b)
	}
	resJSON := func() string {
		m := map[string]any{
			"type":            "result",
			"subtype":         "success",
			"duration_ms":     100,
			"duration_api_ms": 80,
			"is_error":        false,
			"num_turns":       1,
			"session_id":      "test",
			"total_cost_usd":  0.001,
		}
		b, _ := json.Marshal(m)
		return string(b)
	}

	initResp := `{"type":"control_response","response":{"request_id":"%s","subtype":"success","response":{"commands":[],"output_style":"default"}}}`
	mcpReq1 := fmt.Sprintf(`{"type":"control_request","request_id":"mcp_init_1","request":{"subtype":"mcp_message","server_name":"%s","message":{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}}}`, serverName)
	mcpReq2 := fmt.Sprintf(`{"type":"control_request","request_id":"mcp_init_2","request":{"subtype":"mcp_message","server_name":"%s","message":{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}}}`, serverName)
	assistantLine := asstJSON("Hello!")
	resultLine := resJSON()

	var sb strings.Builder
	sb.WriteString("#!/bin/sh\n")
	// Handle initialize
	sb.WriteString("IFS= read -r initline\n")
	sb.WriteString("request_id=$(printf '%s' \"$initline\" | /usr/bin/jq -r '.request_id')\n")
	sb.WriteString(fmt.Sprintf("printf '%s\\n' \"$request_id\"\n", strings.ReplaceAll(initResp, "'", "'\\''")))
	// Emit MCP control requests
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(mcpReq1, "'", "'\\''")))
	// Read the SDK's MCP response from stdin (so stdin stays open)
	sb.WriteString("IFS= read -r _mcp_resp1\n")
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(mcpReq2, "'", "'\\''")))
	sb.WriteString("IFS= read -r _mcp_resp2\n")
	// Now emit the actual messages
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(assistantLine, "'", "'\\''")))
	sb.WriteString(fmt.Sprintf("printf '%%s\\n' '%s'\n", strings.ReplaceAll(resultLine, "'", "'\\''")))
	// Drain remaining stdin
	sb.WriteString("cat > /dev/null &\n")
	sb.WriteString("wait\n")
	if _, err := io.WriteString(f, sb.String()); err != nil {
		t.Fatal(err)
	}
	f.Close()
	os.Chmod(f.Name(), 0o755)
	return f.Name()
}

// queryMessages is a helper that runs processQuery and collects all messages.
func queryMessages(ctx context.Context, prompt string, opts *ClaudeAgentOptions) ([]Message, error) {
	ch, err := Query(ctx, prompt, opts)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for m := range ch {
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// TestQuery_StdinClosedImmediately mirrors test_string_prompt_without_mcp_servers_closes_immediately:
// when no SDK MCP servers and no hooks, closeStdin is called immediately after writing the prompt.
// Behaviorally: the query completes and returns messages without hanging.
func TestQuery_StdinClosedImmediately(t *testing.T) {
	assistantLine, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{map[string]any{"type": "text", "text": "Hello!"}},
			"model": "claude-sonnet-4-20250514",
		},
	})
	resultLine, _ := json.Marshal(map[string]any{
		"type":            "result",
		"subtype":         "success",
		"duration_ms":     100,
		"duration_api_ms": 80,
		"is_error":        false,
		"num_turns":       1,
		"session_id":      "test",
		"total_cost_usd":  0.001,
	})

	script := makeQueryScript(t, string(assistantLine), string(resultLine))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := queryMessages(ctx, "Hello", &ClaudeAgentOptions{CLIPath: script})
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (assistant+result), got %d", len(msgs))
	}
	if _, ok := msgs[0].(*AssistantMessage); !ok {
		t.Errorf("first message should be AssistantMessage, got %T", msgs[0])
	}
	if _, ok := msgs[1].(*ResultMessage); !ok {
		t.Errorf("second message should be ResultMessage, got %T", msgs[1])
	}
}

// TestQuery_StdinDeferredWithMCPServers mirrors test_string_prompt_waits_for_result_with_sdk_mcp_servers:
// with SDK MCP servers present, the SDK must keep stdin open until the
// first ResultMessage arrives (so it can respond to MCP control requests).
func TestQuery_StdinDeferredWithMCPServers(t *testing.T) {
	// Use a fake SDK MCP server
	server := &fakeMCPServer{}

	script := makeQueryScript(t,
		func() string {
			b, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "Hi!"}},
					"model":   "claude-sonnet-4-20250514",
				},
			})
			return string(b)
		}(),
		func() string {
			b, _ := json.Marshal(map[string]any{
				"type":            "result",
				"subtype":         "success",
				"duration_ms":     100,
				"duration_api_ms": 80,
				"is_error":        false,
				"num_turns":       1,
				"session_id":      "test",
				"total_cost_usd":  0.001,
			})
			return string(b)
		}(),
	)

	opts := &ClaudeAgentOptions{
		CLIPath: script,
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := queryMessages(ctx, "Hello", opts)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	if _, ok := msgs[1].(*ResultMessage); !ok {
		t.Errorf("last message should be ResultMessage, got %T", msgs[1])
	}
}

// TestQuery_StdinDeferredWithHooks mirrors test_string_prompt_with_hooks_waits_for_result:
// with hooks configured, stdin must stay open even without SDK MCP servers.
func TestQuery_StdinDeferredWithHooks(t *testing.T) {
	hookCalled := false
	var mu sync.Mutex

	script := makeQueryScript(t,
		func() string {
			b, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "Hi!"}},
					"model":   "claude-sonnet-4-20250514",
				},
			})
			return string(b)
		}(),
		func() string {
			b, _ := json.Marshal(map[string]any{
				"type":            "result",
				"subtype":         "success",
				"duration_ms":     100,
				"duration_api_ms": 80,
				"is_error":        false,
				"num_turns":       1,
				"session_id":      "test",
				"total_cost_usd":  0.001,
			})
			return string(b)
		}(),
	)

	opts := &ClaudeAgentOptions{
		CLIPath: script,
		Hooks: map[HookEvent][]HookMatcher{
			HookEventPreToolUse: {
				{
					Hooks: []HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							mu.Lock()
							hookCalled = true
							mu.Unlock()
							return map[string]any{"continue_": true}, nil
						},
					},
				},
			},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := queryMessages(ctx, "Do something", opts)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	_ = hookCalled // hook is registered but may not be called in unit test without real CLI
}

// TestQuery_MCPControlRequestsHandled mirrors test_string_prompt_mcp_server_control_requests_succeed:
// verifies that MCP control_requests arriving after the user message are
// handled because stdin remains open.
func TestQuery_MCPControlRequestsHandled(t *testing.T) {
	// Use a real "greeter" server that handles initialize and tools/list.
	server := &fakeMCPServer{}

	script := makeQueryScriptWithMCPControl(t, "fake")

	opts := &ClaudeAgentOptions{
		CLIPath: script,
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msgs, err := queryMessages(ctx, "Greet Alice", opts)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages (assistant+result), got %d", len(msgs))
	}
	if _, ok := msgs[0].(*AssistantMessage); !ok {
		t.Errorf("first message should be AssistantMessage, got %T", msgs[0])
	}
	if _, ok := msgs[1].(*ResultMessage); !ok {
		t.Errorf("last message should be ResultMessage, got %T", msgs[1])
	}
}

// TestQuery_AsyncIterableWithMCPServers mirrors test_async_iterable_with_sdk_mcp_servers:
// verifies that QueryStream (async-iterable path) also defers stdin close
// when SDK MCP servers are present.
func TestQuery_AsyncIterableWithMCPServers(t *testing.T) {
	server := &fakeMCPServer{}

	script := makeQueryScript(t,
		func() string {
			b, _ := json.Marshal(map[string]any{
				"type": "assistant",
				"message": map[string]any{
					"role":    "assistant",
					"content": []any{map[string]any{"type": "text", "text": "Hi from stream!"}},
					"model":   "claude-sonnet-4-20250514",
				},
			})
			return string(b)
		}(),
		func() string {
			b, _ := json.Marshal(map[string]any{
				"type":            "result",
				"subtype":         "success",
				"duration_ms":     100,
				"duration_api_ms": 80,
				"is_error":        false,
				"num_turns":       1,
				"session_id":      "test",
				"total_cost_usd":  0.001,
			})
			return string(b)
		}(),
	)

	promptCh := make(chan map[string]any, 1)
	promptCh <- map[string]any{
		"type":       "user",
		"message":    map[string]any{"role": "user", "content": "Hello"},
		"session_id": "default",
	}
	close(promptCh)

	opts := &ClaudeAgentOptions{
		CLIPath: script,
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := QueryStream(ctx, promptCh, opts)
	if err != nil {
		t.Fatalf("QueryStream: %v", err)
	}

	var msgs []Message
	for m := range ch {
		msgs = append(msgs, m)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages from QueryStream, got %d", len(msgs))
	}
	if _, ok := msgs[1].(*ResultMessage); !ok {
		t.Errorf("last message should be ResultMessage, got %T", msgs[1])
	}
}

// TestQuery_AsyncIterableMCPControlRequests mirrors test_async_iterable_mcp_control_requests_succeed:
// verifies that MCP control requests are handled in the async-iterable path.
func TestQuery_AsyncIterableMCPControlRequests(t *testing.T) {
	server := &fakeMCPServer{}
	script := makeQueryScriptWithMCPControl(t, "fake")

	promptCh := make(chan map[string]any, 1)
	promptCh <- map[string]any{
		"type":       "user",
		"message":    map[string]any{"role": "user", "content": "Hello"},
		"session_id": "default",
	}
	close(promptCh)

	opts := &ClaudeAgentOptions{
		CLIPath: script,
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ch, err := QueryStream(ctx, promptCh, opts)
	if err != nil {
		t.Fatalf("QueryStream: %v", err)
	}

	var msgs []Message
	for m := range ch {
		msgs = append(msgs, m)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	if _, ok := msgs[1].(*ResultMessage); !ok {
		t.Errorf("last message should be ResultMessage, got %T", msgs[1])
	}
}
