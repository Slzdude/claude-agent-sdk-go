//go:build e2e

package e2e_test

import (
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestRateLimitEvent_Reception verifies that RateLimitEvent messages can be
// received (or at minimum, that the type is handled without errors).
func TestRateLimitEvent_Reception(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	msgs, err := claude.Query(ctx, "Say hello in one word", &claude.ClaudeAgentOptions{
		MaxTurns: 1,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	if result.IsError {
		t.Fatalf("query failed: %s", result.Result)
	}
	// Note: RateLimitEvent may or may not appear depending on rate limits.
	// This test primarily verifies the parser doesn't crash on the type.
}

// TestGetContextUsage_Basic verifies GetContextUsage works on a connected client.
func TestGetContextUsage_Basic(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, &claude.ClaudeAgentOptions{
		MaxTurns: 1,
	})
	if err != nil {
		t.Fatalf("NewClaudeSDKClient failed: %v", err)
	}
	defer client.Close()

	// Send a prompt to establish context.
	if err := client.Query(ctx, "Say hello"); err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		_ = msg
	}

	// Now query context usage.
	usage, err := client.GetContextUsage(ctx)
	if err != nil {
		t.Fatalf("GetContextUsage failed: %v", err)
	}
	if usage.TotalTokens <= 0 {
		t.Errorf("expected positive TotalTokens, got %d", usage.TotalTokens)
	}
	if usage.Model == "" {
		t.Error("expected non-empty Model")
	}
	t.Logf("Context usage: %d/%d tokens (%.1f%%), model=%s",
		usage.TotalTokens, usage.MaxTokens, usage.Percentage, usage.Model)
}

// TestDontAskPermissionMode verifies that "dontAsk" permission mode is accepted.
func TestDontAskPermissionMode(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	msgs, err := claude.Query(ctx, "Say hello in one word", &claude.ClaudeAgentOptions{
		MaxTurns:        1,
		PermissionMode:  claude.PermissionModeDontAsk,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	if result.IsError {
		t.Fatalf("query failed: %s", result.Result)
	}
}

// TestAutoPermissionMode verifies that "auto" permission mode is accepted.
func TestAutoPermissionMode(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	msgs, err := claude.Query(ctx, "Say hello in one word", &claude.ClaudeAgentOptions{
		MaxTurns:        1,
		PermissionMode:  claude.PermissionModeAuto,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	if result.IsError {
		t.Fatalf("query failed: %s", result.Result)
	}
}

// TestTaskBudget_Option verifies that TaskBudget option is accepted by the CLI.
func TestTaskBudget_Option(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	msgs, err := claude.Query(ctx, "Say hello in one word", &claude.ClaudeAgentOptions{
		MaxTurns:   1,
		TaskBudget: &claude.TaskBudget{Total: 50000},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	if result.IsError {
		t.Fatalf("query failed: %s", result.Result)
	}
}

// TestThinkingAdaptive verifies that ThinkingAdaptive works correctly.
func TestThinkingAdaptive(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	msgs, err := claude.Query(ctx, "What is 2+2?", &claude.ClaudeAgentOptions{
		MaxTurns: 1,
		Thinking: &claude.ThinkingAdaptive{},
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	if result.IsError {
		t.Fatalf("query failed: %s", result.Result)
	}
}

// TestAgentDefinition_ExtendedFields verifies that extended AgentDefinition fields work.
func TestAgentDefinition_ExtendedFields(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	maxTurns := 2
	bg := false
	client, err := claude.NewClaudeSDKClient(ctx, &claude.ClaudeAgentOptions{
		MaxTurns: 1,
		Agents: map[string]claude.AgentDefinition{
			"test-agent": {
				Description: "A test agent",
				Prompt:      "You are a helpful assistant. Respond briefly.",
				Tools:       []string{"Read"},
				Model:       "sonnet",
				MaxTurns:    &maxTurns,
				Background:  &bg,
			},
		},
	})
	if err != nil {
		t.Fatalf("NewClaudeSDKClient failed: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "Hello"); err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		_ = msg
	}
}
