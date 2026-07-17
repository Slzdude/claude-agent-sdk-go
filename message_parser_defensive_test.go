package claude

import (
	"testing"
)

func TestParseAssistantMessage_StringContentRaisesError(t *testing.T) {
	raw := map[string]any{
		"type":    "assistant",
		"message": map[string]any{"model": "m", "content": "hi"},
	}
	_, err := parseMessage(raw)
	if err == nil {
		t.Error("expected error for string content in assistant message")
	}
}

func TestParseAssistantMessage_NonDictContentBlockRaisesError(t *testing.T) {
	raw := map[string]any{
		"type":    "assistant",
		"message": map[string]any{"model": "m", "content": []any{"oops"}},
	}
	_, err := parseMessage(raw)
	if err == nil {
		t.Error("expected error for non-dict content block")
	}
}

func TestParseUserMessage_NonDictContentBlockRaisesError(t *testing.T) {
	raw := map[string]any{
		"type":    "user",
		"message": map[string]any{"content": []any{"oops"}},
	}
	_, err := parseMessage(raw)
	if err == nil {
		t.Error("expected error for non-dict content block in user message")
	}
}

func TestParseAssistantMessage_ValidContent(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"model": "m",
			"content": []any{
				map[string]any{"type": "text", "text": "hello"},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am, ok := msg.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected *AssistantMessage, got %T", msg)
	}
	if len(am.Content) != 1 {
		t.Errorf("expected 1 content block, got %d", len(am.Content))
	}
}
