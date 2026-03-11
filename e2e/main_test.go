//go:build e2e

// Package e2e contains end-to-end tests that make real Claude API calls.
// Run with: go test -tags e2e ./e2e/ -v
// Requires: ANTHROPIC_API_KEY environment variable.
package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

func TestMain(m *testing.M) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		panic("ANTHROPIC_API_KEY environment variable is required for e2e tests.\n" +
			"Set it before running: export ANTHROPIC_API_KEY=your-key-here")
	}
	os.Exit(m.Run())
}

// defaultCtx returns a context with a generous timeout for e2e API calls.
func defaultCtx(t *testing.T) (context.Context, context.CancelFunc) {
	t.Helper()
	return context.WithTimeout(context.Background(), 120*time.Second)
}

// ptr returns a pointer to the given string value.
func ptr(s string) *string { return &s }

// collectMessages drains a message channel into a slice.
func collectMessages(ch <-chan claude.Message) []claude.Message {
	var msgs []claude.Message
	for m := range ch {
		msgs = append(msgs, m)
	}
	return msgs
}

// requireResult asserts that msgs ends with a successful ResultMessage
// and returns it.
func requireResult(t *testing.T, msgs []claude.Message) *claude.ResultMessage {
	t.Helper()
	if len(msgs) == 0 {
		t.Fatal("no messages received")
	}
	res, ok := msgs[len(msgs)-1].(*claude.ResultMessage)
	if !ok {
		t.Fatalf("last message should be ResultMessage, got %T", msgs[len(msgs)-1])
	}
	if res.IsError {
		t.Fatalf("query failed: %s", res.Result)
	}
	return res
}
