//go:build e2e

package e2e_test

// hooks_test.go mirrors e2e-tests/test_hooks.py.

import (
	"context"
	"sync"
	"testing"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

// TestHookWithPermissionDecisionAndReason tests hooks returning permissionDecision
// and reason fields end-to-end. Mirrors test_hook_with_permission_decision_and_reason.
func TestHookWithPermissionDecisionAndReason(t *testing.T) {
	var mu sync.Mutex
	var hookInvocations []string

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Bash", "Write"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPreToolUse: {
				{
				Matcher: ptr("Bash"),
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							toolName, _ := input["tool_name"].(string)
							mu.Lock()
							hookInvocations = append(hookInvocations, toolName)
							mu.Unlock()

							if toolName == "Bash" {
								return map[string]any{
									"reason":        "Bash commands are blocked in this test for safety",
									"systemMessage": "⚠️ Command blocked by hook",
									"hookSpecificOutput": map[string]any{
										"hookEventName":             "PreToolUse",
										"permissionDecision":        "deny",
										"permissionDecisionReason":  "Security policy: Bash blocked",
									},
								}, nil
							}
							return map[string]any{
								"reason": "Tool approved by security review",
								"hookSpecificOutput": map[string]any{
									"hookEventName":             "PreToolUse",
									"permissionDecision":        "allow",
									"permissionDecisionReason":  "Tool passed security checks",
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

	if err := client.Query(ctx, "Run this bash command: echo 'hello'"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, name := range hookInvocations {
		if name == "Bash" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("hook should have been invoked for Bash tool, got: %v", hookInvocations)
	}
}

// TestHookWithContinueAndStopReason tests hooks returning continue_=False and
// stopReason fields end-to-end. Mirrors test_hook_with_continue_and_stop_reason.
func TestHookWithContinueAndStopReason(t *testing.T) {
	var mu sync.Mutex
	var hookInvocations []string

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPostToolUse: {
				{
				Matcher: ptr("Bash"),
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							toolName, _ := input["tool_name"].(string)
							mu.Lock()
							hookInvocations = append(hookInvocations, toolName)
							mu.Unlock()
							return map[string]any{
								"continue_":     false,
								"stopReason":    "Execution halted by test hook for validation",
								"reason":        "Testing continue and stopReason fields",
								"systemMessage": "🛑 Test hook stopped execution",
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

	if err := client.Query(ctx, "Run: echo 'test message'"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, name := range hookInvocations {
		if name == "Bash" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PostToolUse hook should have been invoked, got: %v", hookInvocations)
	}
}

// TestHookWithAdditionalContext tests hooks returning hookSpecificOutput with
// additionalContext field end-to-end. Mirrors test_hook_with_additional_context.
func TestHookWithAdditionalContext(t *testing.T) {
	var mu sync.Mutex
	var hookInvocations []string

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Bash"},
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPostToolUse: {
				{
					Matcher: ptr("Bash"),
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							mu.Lock()
							hookInvocations = append(hookInvocations, "context_added")
							mu.Unlock()
							return map[string]any{
								"systemMessage": "Additional context provided by hook",
								"reason":        "Hook providing monitoring feedback",
								"suppressOutput": false,
								"hookSpecificOutput": map[string]any{
									"hookEventName":    "PostToolUse",
									"additionalContext": "The command executed successfully with hook monitoring",
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

	if err := client.Query(ctx, "Run: echo 'testing hooks'"); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(hookInvocations) == 0 {
		t.Error("PostToolUse hook should have been invoked")
	}
}
