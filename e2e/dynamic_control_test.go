//go:build e2e

package e2e_test

// dynamic_control_test.go mirrors e2e-tests/test_dynamic_control.py.

import (
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestSetPermissionMode tests that permission mode can be changed dynamically.
// Mirrors test_set_permission_mode.
func TestSetPermissionMode(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, &claude.ClaudeAgentOptions{
		PermissionMode: claude.PermissionModeDefault,
	})
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	// Change permission mode to acceptEdits.
	if err := client.SetPermissionMode(ctx, claude.PermissionModeAcceptEdits); err != nil {
		t.Fatalf("SetPermissionMode(acceptEdits): %v", err)
	}

	if err := client.Query(ctx, "What is 2+2? Just respond with the number."); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	// Change back to default.
	if err := client.SetPermissionMode(ctx, claude.PermissionModeDefault); err != nil {
		t.Fatalf("SetPermissionMode(default): %v", err)
	}

	if err := client.Query(ctx, "What is 3+3? Just respond with the number."); err != nil {
		t.Fatalf("Query 2: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}
}

// TestSetModel tests that the model can be changed dynamically during a session.
// Mirrors test_set_model.
func TestSetModel(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	// Default model.
	if err := client.Query(ctx, "What is 1+1? Just the number."); err != nil {
		t.Fatalf("Query 1: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	// Switch to Haiku.
	if err := client.SetModel(ctx, ptr("claude-3-5-haiku-20241022")); err != nil {
		t.Fatalf("SetModel(haiku): %v", err)
	}

	if err := client.Query(ctx, "What is 2+2? Just the number."); err != nil {
		t.Fatalf("Query 2: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	// Reset to default (nil = server default).
	if err := client.SetModel(ctx, nil); err != nil {
		t.Fatalf("SetModel(nil): %v", err)
	}

	if err := client.Query(ctx, "What is 3+3? Just the number."); err != nil {
		t.Fatalf("Query 3: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}
}

// TestInterrupt tests that an interrupt can be sent during a session.
// Mirrors test_interrupt.
func TestInterrupt(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, "Count from 1 to 100 slowly."); err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Fire and forget the interrupt; may or may not take effect in time.
	_ = client.Interrupt(ctx)

	// Consume remaining messages.
	for range client.ReceiveResponse(ctx) {
	}
}
