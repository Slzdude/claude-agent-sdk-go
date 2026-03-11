//go:build e2e

package e2e_test

// hook_events_test.go mirrors e2e-tests/test_hook_events.py.

import (
	"context"
	"sync"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestPreToolUseHookWithAdditionalContext tests PreToolUse hook returning
// additionalContext end-to-end. Mirrors test_pre_tool_use_hook_with_additional_context.
func TestPreToolUseHookWithAdditionalContext(t *testing.T) {
	var mu sync.Mutex
	var invocations []map[string]any

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPreToolUse: {
				{
					Matcher: ptr("Bash"),
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							mu.Lock()
							invocations = append(invocations, map[string]any{
								"tool_name":   input["tool_name"],
								"tool_use_id": input["tool_use_id"],
							})
							mu.Unlock()
							return map[string]any{
								"hookSpecificOutput": map[string]any{
									"hookEventName":            "PreToolUse",
									"permissionDecision":       "allow",
									"permissionDecisionReason": "Approved with context",
									"additionalContext":        "This command is running in a test environment",
								},
							}, nil
						},
					},
				},
			},
		},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "Run: echo 'test additional context'"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(invocations) == 0 {
		t.Fatal("PreToolUse hook was not invoked")
	}
	if invocations[0]["tool_use_id"] == nil {
		t.Error("tool_use_id should be present in PreToolUse input")
	}
}

// TestPostToolUseHookWithToolUseID tests PostToolUse hook receives tool_use_id.
// Mirrors test_post_tool_use_hook_with_tool_use_id.
func TestPostToolUseHookWithToolUseID(t *testing.T) {
	var mu sync.Mutex
	var invocations []map[string]any

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPostToolUse: {
				{
					Matcher: ptr("Bash"),
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							mu.Lock()
							invocations = append(invocations, map[string]any{
								"tool_name":   input["tool_name"],
								"tool_use_id": input["tool_use_id"],
							})
							mu.Unlock()
							return map[string]any{
								"hookSpecificOutput": map[string]any{
									"hookEventName":     "PostToolUse",
									"additionalContext": "Post-tool monitoring active",
								},
							}, nil
						},
					},
				},
			},
		},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "Run: echo 'test tool_use_id'"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(invocations) == 0 {
		t.Fatal("PostToolUse hook was not invoked")
	}
	if invocations[0]["tool_use_id"] == nil {
		t.Error("tool_use_id should be present in PostToolUse input")
	}
}

// TestNotificationHook tests that the Notification hook fires end-to-end.
// Mirrors test_notification_hook.
func TestNotificationHook(t *testing.T) {
	var mu sync.Mutex
	var invocations []map[string]any

	opts := &claude.ClaudeAgentOptions{
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventNotification: {
				{
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							mu.Lock()
							invocations = append(invocations, map[string]any{
								"hook_event_name":   input["hook_event_name"],
								"message":           input["message"],
								"notification_type": input["notification_type"],
							})
							mu.Unlock()
							return map[string]any{
								"hookSpecificOutput": map[string]any{
									"hookEventName":     "Notification",
									"additionalContext": "Notification received",
								},
							}, nil
						},
					},
				},
			},
		},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "Say hello in one word."); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	// Notification hooks may or may not fire. If they did, verify shape.
	mu.Lock()
	defer mu.Unlock()
	for _, inv := range invocations {
		if inv["hook_event_name"] != "Notification" {
			t.Errorf("expected Notification event, got: %v", inv["hook_event_name"])
		}
		if inv["notification_type"] == nil {
			t.Error("notification_type should be present")
		}
	}
}

// TestMultipleHooksTogether tests registering multiple hook event types.
// Mirrors test_multiple_hooks_together.
func TestMultipleHooksTogether(t *testing.T) {
	var mu sync.Mutex
	var allInvocations []string

	trackHook := func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
		mu.Lock()
		if name, ok := input["hook_event_name"].(string); ok {
			allInvocations = append(allInvocations, name)
		}
		mu.Unlock()
		return map[string]any{}, nil
	}

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventNotification: {{Hooks: []claude.HookCallback{trackHook}}},
			claude.HookEventPreToolUse:   {{Matcher: ptr("Bash"), Hooks: []claude.HookCallback{trackHook}}},
			claude.HookEventPostToolUse:  {{Matcher: ptr("Bash"), Hooks: []claude.HookCallback{trackHook}}},
		},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "Run: echo 'multi-hook test'"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()

	hasPreToolUse := false
	hasPostToolUse := false
	for _, name := range allInvocations {
		if name == "PreToolUse" {
			hasPreToolUse = true
		}
		if name == "PostToolUse" {
			hasPostToolUse = true
		}
	}
	if !hasPreToolUse {
		t.Errorf("PreToolUse hook should have fired, got: %v", allInvocations)
	}
	if !hasPostToolUse {
		t.Errorf("PostToolUse hook should have fired, got: %v", allInvocations)
	}
}
