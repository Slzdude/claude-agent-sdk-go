package claude

import (
	"context"
	"encoding/json"
	"testing"
)

// marshalLines converts a slice of maps to a slice of JSON strings.
func marshalLines(msgs []map[string]any) []string {
	lines := make([]string, 0, len(msgs))
	for _, m := range msgs {
		b, _ := json.Marshal(m)
		lines = append(lines, string(b))
	}
	return lines
}

// TestIntegration_ParsesAssistantAndResult verifies the full parse pipeline.
func TestIntegration_ParsesAssistantAndResult(t *testing.T) {
	messages := []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"role":  "assistant",
				"model": "claude-opus-4-5",
				"content": []map[string]any{
					{"type": "text", "text": "2 + 2 equals 4"},
				},
			},
		},
		{
			"type":            "result",
			"subtype":         "success",
			"duration_ms":     1000,
			"duration_api_ms": 800,
			"is_error":        false,
			"num_turns":       1,
			"session_id":      "test-session",
			"total_cost_usd":  0.001,
		},
	}

	tr := mockTransportLines(t, marshalLines(messages)...)
	q := newQueryProto(tr, &ClaudeAgentOptions{})
	defer func() { _ = tr.close() }()

	rawCh := q.Run(context.Background())

	var parsed []Message
	for raw := range rawCh {
		msg, err := parseMessage(raw)
		if err != nil || msg == nil {
			continue
		}
		parsed = append(parsed, msg)
	}

	if len(parsed) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(parsed))
	}

	assistantMsg, ok := parsed[0].(*AssistantMessage)
	if !ok {
		t.Fatalf("expected *AssistantMessage, got %T", parsed[0])
	}
	if len(assistantMsg.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(assistantMsg.Content))
	}
	text, ok := assistantMsg.Content[0].(*TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", assistantMsg.Content[0])
	}
	if text.Text != "2 + 2 equals 4" {
		t.Errorf("wrong text: %q", text.Text)
	}

	result, ok := parsed[1].(*ResultMessage)
	if !ok {
		t.Fatalf("expected *ResultMessage, got %T", parsed[1])
	}
	if result.SessionID != "test-session" {
		t.Errorf("wrong SessionID: %q", result.SessionID)
	}
	if result.TotalCostUSD == nil || *result.TotalCostUSD != 0.001 {
		t.Errorf("wrong TotalCostUSD: %v", result.TotalCostUSD)
	}
}

// TestIntegration_ParsesToolUseBlock verifies tool_use blocks are parsed.
func TestIntegration_ParsesToolUseBlock(t *testing.T) {
	messages := []map[string]any{
		{
			"type": "assistant",
			"message": map[string]any{
				"role":  "assistant",
				"model": "claude-opus-4-5",
				"content": []map[string]any{
					{"type": "text", "text": "Let me read that file."},
					{"type": "tool_use", "id": "tool-123", "name": "Read", "input": map[string]any{"file_path": "/test.txt"}},
				},
			},
		},
		{
			"type":     "result",
			"subtype":  "success",
			"is_error": false,
		},
	}

	tr := mockTransportLines(t, marshalLines(messages)...)
	q := newQueryProto(tr, &ClaudeAgentOptions{})
	defer func() { _ = tr.close() }()

	rawCh := q.Run(context.Background())

	var parsed []Message
	for raw := range rawCh {
		msg, _ := parseMessage(raw)
		if msg != nil {
			parsed = append(parsed, msg)
		}
	}

	if len(parsed) < 1 {
		t.Fatal("expected at least 1 message")
	}
	am, ok := parsed[0].(*AssistantMessage)
	if !ok {
		t.Fatalf("expected AssistantMessage, got %T", parsed[0])
	}
	if len(am.Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(am.Content))
	}
	toolBlock, ok := am.Content[1].(*ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", am.Content[1])
	}
	if toolBlock.Name != "Read" {
		t.Errorf("wrong tool name: %q", toolBlock.Name)
	}
	if toolBlock.Input["file_path"] != "/test.txt" {
		t.Errorf("wrong input: %v", toolBlock.Input)
	}
}
