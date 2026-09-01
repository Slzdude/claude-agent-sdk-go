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

// TestQuery_CanUseToolHoldsStdinOpen: with only can_use_tool configured (no hooks,
// no MCP servers), stdin must remain open until the run-ending result.
func TestQuery_CanUseToolHoldsStdinOpen(t *testing.T) {
	tr := mockTransportWithInit(t, assistantLine("Hello!"), resultLine())

	permissionCalled := false
	opts := &ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx ToolPermissionContext) (PermissionResult, error) {
			permissionCalled = true
			return &PermissionResultAllow{}, nil
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
	// The permission callback was registered but won't fire without a real tool_use
	_ = permissionCalled
}

// TestQuery_StringPromptWithMCPServers: string prompt with MCP servers must
// keep stdin open until result arrives.
func TestQuery_StringPromptWithMCPServers(t *testing.T) {
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

// TestQuery_StringPromptWithoutMCPServers: string prompt without MCP servers
// must close stdin immediately and not hang.
func TestQuery_StringPromptWithoutMCPServers(t *testing.T) {
	tr := mockTransportWithInit(t, assistantLine("Hello!"), resultLine())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	msgs, err := mockQueryMessages(ctx, t, "Hello", tr, nil)
	if err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

// TestQuery_ResultWithInflightTaskKeepsStdinOpen: when a result message arrives
// but background tasks are still in flight, stdin must not be closed.
func TestQuery_ResultWithInflightTaskKeepsStdinOpen(t *testing.T) {
	// This test verifies the task lifecycle tracking behavior.
	// When task_started arrives for a deferring task type (local_agent, local_workflow),
	// the task is tracked. When result arrives with inflight tasks, stdin stays open.
	q := &queryProto{
		inflightTasks: make(map[string]bool),
	}

	// Simulate task_started for a deferring task type
	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-1",
		"task_type": "local_agent",
	})

	if len(q.inflightTasks) != 1 {
		t.Errorf("expected 1 inflight task, got %d", len(q.inflightTasks))
	}

	// Simulate task_updated with terminal status - should clear the task
	q.trackTaskLifecycle(map[string]any{
		"subtype": "task_updated",
		"task_id": "task-1",
		"patch": map[string]any{
			"status": "completed",
		},
	})

	if len(q.inflightTasks) != 0 {
		t.Errorf("expected 0 inflight tasks after completion, got %d", len(q.inflightTasks))
	}
}

// TestQuery_NonDeferringTaskTypesNotTracked: shell tasks (local_bash) must not
// be tracked as inflight, because they can run forever.
func TestQuery_NonDeferringTaskTypesNotTracked(t *testing.T) {
	q := &queryProto{
		inflightTasks: make(map[string]bool),
	}

	// Simulate task_started for a non-deferring task type
	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-shell",
		"task_type": "local_bash",
	})

	if len(q.inflightTasks) != 0 {
		t.Errorf("expected 0 inflight tasks for shell task, got %d", len(q.inflightTasks))
	}
}

// TestQuery_TaskUpdatedNonTerminalKeepsTask: task_updated with non-terminal
// status must not clear the task.
func TestQuery_TaskUpdatedNonTerminalKeepsTask(t *testing.T) {
	q := &queryProto{
		inflightTasks: make(map[string]bool),
	}

	// Add a task
	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-1",
		"task_type": "local_agent",
	})

	// Update with non-terminal status
	q.trackTaskLifecycle(map[string]any{
		"subtype": "task_updated",
		"task_id": "task-1",
		"patch": map[string]any{
			"status": "running",
		},
	})

	if len(q.inflightTasks) != 1 {
		t.Errorf("expected 1 inflight task (non-terminal), got %d", len(q.inflightTasks))
	}
}

// TestQuery_TaskNotificationClearsTask: task_notification must clear the task.
func TestQuery_TaskNotificationClearsTask(t *testing.T) {
	q := &queryProto{
		inflightTasks: make(map[string]bool),
	}

	// Add a task
	q.trackTaskLifecycle(map[string]any{
		"subtype":   "task_started",
		"task_id":   "task-1",
		"task_type": "local_agent",
	})

	// Clear via task_notification
	q.trackTaskLifecycle(map[string]any{
		"subtype": "task_notification",
		"task_id": "task-1",
	})

	if len(q.inflightTasks) != 0 {
		t.Errorf("expected 0 inflight tasks after notification, got %d", len(q.inflightTasks))
	}
}
