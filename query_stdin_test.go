package claude

// query_stdin_test.go mirrors test_query.py.
// Tests for processQuery() stdin lifecycle with SDK MCP servers and hooks.
//
// Uses in-memory mock transports (cross-platform, no shell scripts).

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// mockQueryMessages runs mockProcessQuery and collects all messages.
func mockQueryMessages(ctx context.Context, t *testing.T, prompt string, tr *cliTransport, opts *ClaudeAgentOptions) ([]Message, error) {
	t.Helper()
	if opts == nil {
		opts = &ClaudeAgentOptions{}
	}
	ch, err := mockProcessQuery(ctx, t, prompt, tr, opts)
	if err != nil {
		return nil, err
	}
	var msgs []Message
	for m := range ch {
		msgs = append(msgs, m)
	}
	return msgs, nil
}

// assistantLine returns a marshalled assistant message JSON string.
func assistantLine(text string) string {
	b, _ := json.Marshal(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role":    "assistant",
			"content": []any{map[string]any{"type": "text", "text": text}},
			"model":   "claude-sonnet-4-20250514",
		},
	})
	return string(b)
}

// resultLine returns a marshalled result message JSON string.
func resultLine() string {
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
}

// TestQuery_StdinClosedImmediately: without SDK MCP servers or hooks, the query
// completes and returns messages without hanging.
func TestQuery_StdinClosedImmediately(t *testing.T) {
	tr := mockTransportWithInit(t, assistantLine("Hello!"), resultLine())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := mockQueryMessages(ctx, t, "Hello", tr, nil)
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

// TestQuery_StdinDeferredWithMCPServers: with SDK MCP servers present, stdin
// stays open until ResultMessage arrives.
func TestQuery_StdinDeferredWithMCPServers(t *testing.T) {
	server := &fakeMCPServer{}

	tr := mockTransportWithInit(t, assistantLine("Hi!"), resultLine())

	opts := &ClaudeAgentOptions{
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := mockQueryMessages(ctx, t, "Hello", tr, opts)
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

// TestQuery_StdinDeferredWithHooks: with hooks configured, stdin must stay open.
func TestQuery_StdinDeferredWithHooks(t *testing.T) {
	hookCalled := false
	var mu sync.Mutex

	tr := mockTransportWithInit(t, assistantLine("Hi!"), resultLine())

	opts := &ClaudeAgentOptions{
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

	msgs, err := mockQueryMessages(ctx, t, "Do something", tr, opts)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
	_ = hookCalled // hook registered but won't fire without real tool_use block
}

// TestQuery_MCPControlRequestsHandled: MCP control_requests arriving after the
// user message are handled because stdin remains open.
func TestQuery_MCPControlRequestsHandled(t *testing.T) {
	server := &fakeMCPServer{}

	tr := mockTransportWithMCP(t, "fake")

	opts := &ClaudeAgentOptions{
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	msgs, err := mockQueryMessages(ctx, t, "Greet Alice", tr, opts)
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

// TestQuery_AsyncIterableWithMCPServers: QueryStream path also defers stdin
// close when SDK MCP servers are present.
func TestQuery_AsyncIterableWithMCPServers(t *testing.T) {
	server := &fakeMCPServer{}

	tr := mockTransportWithInit(t, assistantLine("Hi from stream!"), resultLine())

	promptCh := make(chan map[string]any, 1)
	promptCh <- map[string]any{
		"type":       "user",
		"message":    map[string]any{"role": "user", "content": "Hello"},
		"session_id": "default",
	}
	close(promptCh)

	opts := &ClaudeAgentOptions{
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := mockQueryStream(ctx, t, promptCh, tr, opts)
	if err != nil {
		t.Fatalf("mockQueryStream: %v", err)
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

// TestQuery_AsyncIterableMCPControlRequests: MCP control requests are handled
// in the async-iterable (QueryStream) path.
func TestQuery_AsyncIterableMCPControlRequests(t *testing.T) {
	server := &fakeMCPServer{}

	tr := mockTransportWithMCP(t, "fake")

	promptCh := make(chan map[string]any, 1)
	promptCh <- map[string]any{
		"type":       "user",
		"message":    map[string]any{"role": "user", "content": "Hello"},
		"session_id": "default",
	}
	close(promptCh)

	opts := &ClaudeAgentOptions{
		MCPServers: map[string]MCPServerConfig{
			"fake": &MCPSdkServerConfig{Instance: server},
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	ch, err := mockQueryStream(ctx, t, promptCh, tr, opts)
	if err != nil {
		t.Fatalf("mockQueryStream: %v", err)
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
