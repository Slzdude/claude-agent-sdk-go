package claude

// rate_limit_test.go verifies that rate_limit_event messages are now properly
// parsed into RateLimitEvent (previously they were skipped as unknown types).

import "testing"

// TestRateLimitEvent_Parsed checks that a rate_limit_event is parsed into RateLimitEvent.
func TestRateLimitEvent_Parsed(t *testing.T) {
	data := map[string]any{
		"type": "rate_limit_event",
		"rate_limit_info": map[string]any{
			"status":         "allowed_warning",
			"resetsAt":       float64(1700000000),
			"rateLimitType":  "five_hour",
			"utilization":    0.85,
			"isUsingOverage": false,
		},
		"uuid":       "550e8400-e29b-41d4-a716-446655440000",
		"session_id": "test-session-id",
	}

	result, err := parseMessage(data)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected RateLimitEvent, got nil")
	}
	evt, ok := result.(*RateLimitEvent)
	if !ok {
		t.Fatalf("expected *RateLimitEvent, got %T", result)
	}
	if evt.RateLimitInfo.Status != RateLimitAllowedWarning {
		t.Errorf("expected status allowed_warning, got %q", evt.RateLimitInfo.Status)
	}
	if evt.UUID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("wrong UUID: %q", evt.UUID)
	}
}

// TestRateLimitEventRejected_Parsed checks that a hard rate limit is also parsed.
func TestRateLimitEventRejected_Parsed(t *testing.T) {
	data := map[string]any{
		"type": "rate_limit_event",
		"rate_limit_info": map[string]any{
			"status":                "rejected",
			"resetsAt":              float64(1700003600),
			"rateLimitType":         "seven_day",
			"isUsingOverage":        false,
			"overageStatus":         "rejected",
			"overageDisabledReason": "out_of_credits",
		},
		"uuid":       "660e8400-e29b-41d4-a716-446655440001",
		"session_id": "test-session-id",
	}

	result, err := parseMessage(data)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result == nil {
		t.Fatal("expected RateLimitEvent, got nil")
	}
	evt, ok := result.(*RateLimitEvent)
	if !ok {
		t.Fatalf("expected *RateLimitEvent, got %T", result)
	}
	if evt.RateLimitInfo.Status != RateLimitRejected {
		t.Errorf("expected status rejected, got %q", evt.RateLimitInfo.Status)
	}
}

// TestUnknownMessageType_ReturnsNil verifies forward-compatibility with new CLI types.
func TestUnknownMessageType_ReturnsNil(t *testing.T) {
	data := map[string]any{
		"type":       "some_future_event_type",
		"uuid":       "770e8400-e29b-41d4-a716-446655440002",
		"session_id": "test-session-id",
	}

	result, err := parseMessage(data)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for unknown message type, got %T", result)
	}
}

// TestKnownMessageType_StillParsed verifies that known types still work after the fix.
func TestKnownMessageType_StillParsed(t *testing.T) {
	data := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": "hello"},
			},
			"model": "claude-sonnet-4-20250514",
		},
	}

	result, err := parseMessage(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil message for known type 'assistant'")
	}
	msg, ok := result.(*AssistantMessage)
	if !ok {
		t.Fatalf("expected *AssistantMessage, got %T", result)
	}
	tb, ok := msg.Content[0].(*TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", msg.Content[0])
	}
	if tb.Text != "hello" {
		t.Errorf("expected text 'hello', got %q", tb.Text)
	}
}
