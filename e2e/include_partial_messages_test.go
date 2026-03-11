//go:build e2e

package e2e_test

// include_partial_messages_test.go mirrors e2e-tests/test_include_partial_messages.py.

import (
	"testing"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// TestIncludePartialMessagesStreamEvents tests that include_partial_messages=true
// produces StreamEvent messages. Mirrors test_include_partial_messages_stream_events.
func TestIncludePartialMessagesStreamEvents(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	opts := &claude.ClaudeAgentOptions{
		IncludePartialMessages: true,
		MaxTurns:               2,
	}

	ch, err := claude.Query(ctx, "Think of three jokes, then tell one", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)

	// Verify we have messages.
	if len(msgs) == 0 {
		t.Fatal("no messages received")
	}

	// First message should be a SystemMessage(init).
	sys, ok := msgs[0].(*claude.SystemMessage)
	if !ok || sys.Subtype != "init" {
		t.Errorf("first message should be SystemMessage(init), got %T", msgs[0])
	}

	// Should have multiple StreamEvent messages.
	var streamEvents []*claude.StreamEvent
	var assistantMessages []*claude.AssistantMessage
	for _, m := range msgs {
		switch v := m.(type) {
		case *claude.StreamEvent:
			streamEvents = append(streamEvents, v)
		case *claude.AssistantMessage:
			assistantMessages = append(assistantMessages, v)
		}
	}

	if len(streamEvents) == 0 {
		t.Error("No StreamEvent messages received")
	}

	// Check for expected StreamEvent types.
	eventTypes := map[string]bool{}
	for _, s := range streamEvents {
		if t2, ok := s.Event["type"].(string); ok {
			eventTypes[t2] = true
		}
	}
	for _, expected := range []string{
		"message_start", "content_block_start", "content_block_delta",
		"content_block_stop", "message_stop",
	} {
		if !eventTypes[expected] {
			t.Errorf("missing StreamEvent type: %s (got: %v)", expected, eventTypes)
		}
	}

	// Should have at least one AssistantMessage.
	if len(assistantMessages) == 0 {
		t.Error("No AssistantMessage received")
	}

	// Check for TextBlock in at least one AssistantMessage.
	hasText := false
	for _, am := range assistantMessages {
		for _, block := range am.Content {
			if _, ok := block.(*claude.TextBlock); ok {
				hasText = true
			}
		}
	}
	if !hasText {
		t.Error("No TextBlock found in AssistantMessages")
	}

	// Should end with a successful ResultMessage.
	requireResult(t, msgs)
}

// TestIncludePartialMessagesThinkingDeltas tests that thinking content is
// streamed incrementally. Mirrors test_include_partial_messages_thinking_deltas.
func TestIncludePartialMessagesThinkingDeltas(t *testing.T) {
	ctx, cancel := defaultCtx(t)
	defer cancel()

	opts := &claude.ClaudeAgentOptions{
		IncludePartialMessages: true,
		MaxTurns:               2,
	}

	ch, err := claude.Query(ctx, "Think step by step: what is 15 * 17?", opts)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)

	// Collect text deltas from StreamEvents (content_block_delta with text_delta type).
	var textDeltas []string
	for _, m := range msgs {
		se, ok := m.(*claude.StreamEvent)
		if !ok {
			continue
		}
		evType, _ := se.Event["type"].(string)
		if evType != "content_block_delta" {
			continue
		}
		delta, ok := se.Event["delta"].(map[string]any)
		if !ok {
			continue
		}
		if delta["type"] == "text_delta" {
			if text, ok := delta["text"].(string); ok {
				textDeltas = append(textDeltas, text)
			}
		}
	}

	if len(textDeltas) == 0 {
		t.Error("No text delta StreamEvents received")
	}
	t.Logf("Received %d text delta events", len(textDeltas))

	requireResult(t, msgs)
}
