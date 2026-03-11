//go:build e2e

package e2e_test

// stderr_callback_test.go mirrors e2e-tests/test_stderr_callback.py.

import (
	"strings"
	"sync"
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestStderrCallbackCapturesDebugOutput tests that stderr callback receives
// debug output when --debug-to-stderr is enabled.
// Mirrors test_stderr_callback_captures_debug_output.
func TestStderrCallbackCapturesDebugOutput(t *testing.T) {
	var mu sync.Mutex
	var lines []string

	opts := &claude.ClaudeAgentOptions{
		Stderr: func(line string) {
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		},
		ExtraArgs: map[string]*string{
			"debug-to-stderr": nil, // boolean flag, no value
		},
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx, "What is 1+1?", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range ch {
	}

	mu.Lock()
	defer mu.Unlock()

	if len(lines) == 0 {
		t.Error("Should capture stderr output with debug enabled")
	}
	hasDebug := false
	for _, l := range lines {
		if strings.Contains(l, "[DEBUG]") {
			hasDebug = true
			break
		}
	}
	if !hasDebug {
		t.Errorf("Should contain [DEBUG] messages, got %d lines: %v", len(lines), lines)
	}
}

// TestStderrCallbackWithoutDebug tests that stderr callback works but receives
// no output without debug mode. Mirrors test_stderr_callback_without_debug.
func TestStderrCallbackWithoutDebug(t *testing.T) {
	var mu sync.Mutex
	var lines []string

	opts := &claude.ClaudeAgentOptions{
		Stderr: func(line string) {
			mu.Lock()
			lines = append(lines, line)
			mu.Unlock()
		},
		// No debug flag.
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx, "What is 1+1?", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	for range ch {
	}

	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 0 {
		t.Errorf("Should not capture stderr output without debug mode, got: %v", lines)
	}
}
