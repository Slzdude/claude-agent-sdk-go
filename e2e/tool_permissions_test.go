//go:build e2e

package e2e_test

// tool_permissions_test.go mirrors e2e-tests/test_tool_permissions.py.

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestPermissionCallbackGetsCalled tests that can_use_tool callback is invoked
// for non-read-only commands. Mirrors test_permission_callback_gets_called.
//
// Note: Uses NewClaudeSDKClient (like the Python test uses ClaudeSDKClient).
// The touch command is used since it is not auto-allowed (not read-only).
func TestPermissionCallbackGetsCalled(t *testing.T) {
	var mu sync.Mutex
	var callbackInvocations []struct {
		toolName string
		input    map[string]any
	}

	testFile := fmt.Sprintf("/tmp/sdk_go_permission_test_%d.txt", os.Getpid())
	defer os.Remove(testFile)

	opts := &claude.ClaudeAgentOptions{
		CanUseTool: func(ctx context.Context, toolName string, input map[string]any, permCtx claude.ToolPermissionContext) (claude.PermissionResult, error) {
			mu.Lock()
			callbackInvocations = append(callbackInvocations, struct {
				toolName string
				input    map[string]any
			}{toolName, input})
			mu.Unlock()
			return &claude.PermissionResultAllow{}, nil
		},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		t.Fatalf("NewClaudeSDKClient: %v", err)
	}
	defer client.Close()

	if err := client.Query(ctx, fmt.Sprintf("Run the command: touch %s", testFile)); err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range client.ReceiveResponse(ctx) {
	}

	mu.Lock()
	defer mu.Unlock()
	foundBash := false
	for _, inv := range callbackInvocations {
		if inv.toolName == "Bash" {
			foundBash = true
			break
		}
	}
	if !foundBash {
		names := make([]string, len(callbackInvocations))
		for i, inv := range callbackInvocations {
			names[i] = inv.toolName
		}
		t.Errorf("Permission callback should have been invoked for Bash, got: %v", names)
	}
}
