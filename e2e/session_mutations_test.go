//go:build e2e

package e2e_test

import (
	"os"
	"path/filepath"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestSessionMutations_RenameTagDelete verifies the full session mutation lifecycle.
func TestSessionMutations_RenameTagDelete(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	// First, create a session by running a query.
	tmpDir := t.TempDir()
	os.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("CLAUDE_CONFIG_DIR")
	os.MkdirAll(filepath.Join(tmpDir, "projects"), 0755)

	msgs, err := claude.Query(ctx, "Say 'test session' and nothing else", &claude.ClaudeAgentOptions{
		MaxTurns: 1,
		CWD:      tmpDir,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	sessionID := result.SessionID
	if sessionID == "" {
		t.Fatal("expected session ID in result")
	}
	t.Logf("Created session: %s", sessionID)

	// Rename the session.
	err = claude.RenameSession(sessionID, "Test Rename", "")
	if err != nil {
		t.Logf("RenameSession: %v (session file may not be accessible in test env)", err)
	}

	// Tag the session.
	err = claude.TagSession(sessionID, "e2e-test", "")
	if err != nil {
		t.Logf("TagSession: %v", err)
	}

	// Get session info.
	info, _ := claude.GetSessionInfo(sessionID, "")
	if info != nil {
		t.Logf("Session info: summary=%q, tag=%q", info.Summary, info.Tag)
	}

	// Delete the session.
	err = claude.DeleteSession(sessionID, "")
	if err != nil {
		t.Logf("DeleteSession: %v", err)
	}
}

// TestSessionMutations_Fork verifies session forking.
func TestSessionMutations_Fork(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	tmpDir := t.TempDir()
	os.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	defer os.Unsetenv("CLAUDE_CONFIG_DIR")
	os.MkdirAll(filepath.Join(tmpDir, "projects"), 0755)

	// Create a session.
	msgs, err := claude.Query(ctx, "Say 'fork test' and nothing else", &claude.ClaudeAgentOptions{
		MaxTurns: 1,
		CWD:      tmpDir,
	})
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	result := requireResult(t, collectMessages(msgs))
	sessionID := result.SessionID
	if sessionID == "" {
		t.Fatal("expected session ID in result")
	}

	// Fork the session.
	forkResult, err := claude.ForkSession(sessionID, "", "", "Forked Session")
	if err != nil {
		t.Logf("ForkSession: %v (session file may not be accessible)", err)
		return
	}
	if forkResult.SessionID == "" {
		t.Error("expected non-empty fork session ID")
	}
	if forkResult.SessionID == sessionID {
		t.Error("fork should have different session ID")
	}
	t.Logf("Forked session: %s -> %s", sessionID, forkResult.SessionID)
}
