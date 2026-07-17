package claude

import (
	"encoding/json"
	"testing"
)

func TestParseSystemMessage(t *testing.T) {
	raw := map[string]any{
		"type":    "system",
		"subtype": "init",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	sm, ok := msg.(*SystemMessage)
	if !ok {
		t.Fatalf("expected *SystemMessage, got %T", msg)
	}
	if sm.Subtype != "init" {
		t.Errorf("expected subtype 'init', got %q", sm.Subtype)
	}
}

func TestParseAssistantMessage(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "Hello, world!"},
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
		t.Fatalf("expected 1 content block, got %d", len(am.Content))
	}
	text, ok := am.Content[0].(*TextBlock)
	if !ok {
		t.Fatalf("expected *TextBlock, got %T", am.Content[0])
	}
	if text.Text != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", text.Text)
	}
}

func TestParseAssistantMessageThinking(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":      "thinking",
					"thinking":  "Let me think...",
					"signature": "sig123",
				},
				map[string]any{"type": "text", "text": "Answer"},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am := msg.(*AssistantMessage)
	if len(am.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(am.Content))
	}
	think, ok := am.Content[0].(*ThinkingBlock)
	if !ok {
		t.Fatalf("expected *ThinkingBlock, got %T", am.Content[0])
	}
	if think.Thinking != "Let me think..." {
		t.Errorf("wrong thinking text: %q", think.Thinking)
	}
	if think.Signature != "sig123" {
		t.Errorf("wrong signature: %q", think.Signature)
	}
}

func TestParseToolUseBlock(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":  "tool_use",
					"id":    "tu_123",
					"name":  "Bash",
					"input": map[string]any{"command": "echo hi"},
				},
			},
		},
	}
	msg, _ := parseMessage(raw)
	am := msg.(*AssistantMessage)
	tu, ok := am.Content[0].(*ToolUseBlock)
	if !ok {
		t.Fatalf("expected *ToolUseBlock, got %T", am.Content[0])
	}
	if tu.ID != "tu_123" || tu.Name != "Bash" {
		t.Errorf("unexpected tool_use fields: id=%q name=%q", tu.ID, tu.Name)
	}
}

func TestParseResultMessage(t *testing.T) {
	cost := 0.005
	raw := map[string]any{
		"type":           "result",
		"subtype":        "success",
		"is_error":       false,
		"session_id":     "sess_abc",
		"result":         "All done",
		"duration_ms":    float64(1234),
		"num_turns":      float64(3),
		"total_cost_usd": cost,
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	rm, ok := msg.(*ResultMessage)
	if !ok {
		t.Fatalf("expected *ResultMessage, got %T", msg)
	}
	if rm.Result != "All done" {
		t.Errorf("wrong result: %q", rm.Result)
	}
	if rm.DurationMs != 1234 {
		t.Errorf("wrong duration_ms: %d", rm.DurationMs)
	}
	if rm.NumTurns != 3 {
		t.Errorf("wrong num_turns: %d", rm.NumTurns)
	}
	if rm.TotalCostUSD == nil || *rm.TotalCostUSD != 0.005 {
		t.Errorf("wrong total_cost_usd: %v", rm.TotalCostUSD)
	}
}

func TestParseTaskStartedMessage(t *testing.T) {
	raw := map[string]any{
		"type":        "task_started",
		"task_id":     "task_001",
		"description": "Research task",
		"uuid":        "uuid-xyz",
		"session_id":  "sess1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tm, ok := msg.(*TaskStartedMessage)
	if !ok {
		t.Fatalf("expected *TaskStartedMessage, got %T", msg)
	}
	if tm.TaskID != "task_001" || tm.Description != "Research task" {
		t.Errorf("unexpected fields: %+v", tm)
	}
}

func TestParseTaskNotificationMessage(t *testing.T) {
	raw := map[string]any{
		"type":       "task_notification",
		"task_id":    "task_001",
		"status":     "completed",
		"summary":    "All finished",
		"uuid":       "uuid-xyz",
		"session_id": "sess1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tn, ok := msg.(*TaskNotificationMessage)
	if !ok {
		t.Fatalf("expected *TaskNotificationMessage, got %T", msg)
	}
	if tn.Status != TaskStatusCompleted {
		t.Errorf("expected completed, got %s", tn.Status)
	}
}

func TestParseTaskStartedMessage_EmbeddedSystemMessage(t *testing.T) {
	raw := map[string]any{
		"type":        "task_started",
		"task_id":     "task_001",
		"description": "Research task",
		"uuid":        "uuid-xyz",
		"session_id":  "sess1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tm, ok := msg.(*TaskStartedMessage)
	if !ok {
		t.Fatalf("expected *TaskStartedMessage, got %T", msg)
	}
	// Verify embedded SystemMessage fields are populated (matching Python SDK).
	if tm.Subtype != "task_started" {
		t.Errorf("expected Subtype 'task_started', got %q", tm.Subtype)
	}
	if tm.Data == nil {
		t.Fatal("expected Data to be populated, got nil")
	}
	if tm.Data["task_id"] != "task_001" {
		t.Errorf("wrong Data[task_id]: %v", tm.Data["task_id"])
	}
}

// TestParseTaskStartedMessage_AsSystemSubtype verifies that the Go parser
// also handles the CLI's actual wire format: type="system" + subtype="task_started".
// This matches the Python SDK's parser which dispatches on subtype within system messages.
func TestParseTaskStartedMessage_AsSystemSubtype(t *testing.T) {
	raw := map[string]any{
		"type":        "system",
		"subtype":     "task_started",
		"task_id":     "task_001",
		"description": "Research task",
		"uuid":        "uuid-xyz",
		"session_id":  "sess1",
		"task_type":   "background",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tm, ok := msg.(*TaskStartedMessage)
	if !ok {
		t.Fatalf("expected *TaskStartedMessage, got %T", msg)
	}
	if tm.Subtype != "task_started" {
		t.Errorf("expected Subtype 'task_started', got %q", tm.Subtype)
	}
	if tm.TaskID != "task_001" {
		t.Errorf("expected TaskID 'task_001', got %q", tm.TaskID)
	}
	if tm.TaskType != "background" {
		t.Errorf("expected TaskType 'background', got %q", tm.TaskType)
	}
	if tm.Data == nil {
		t.Fatal("expected Data to be populated, got nil")
	}
}

func TestParseTaskNotificationMessage_AsSystemSubtype(t *testing.T) {
	raw := map[string]any{
		"type":       "system",
		"subtype":    "task_notification",
		"task_id":    "task_001",
		"status":     "completed",
		"summary":    "All finished",
		"uuid":       "uuid-xyz",
		"session_id": "sess1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tn, ok := msg.(*TaskNotificationMessage)
	if !ok {
		t.Fatalf("expected *TaskNotificationMessage, got %T", msg)
	}
	if tn.Subtype != "task_notification" {
		t.Errorf("expected Subtype 'task_notification', got %q", tn.Subtype)
	}
	if tn.Status != TaskStatusCompleted {
		t.Errorf("expected completed, got %s", tn.Status)
	}
}

func TestParseTaskProgressMessage_EmbeddedSystemMessage(t *testing.T) {
	raw := map[string]any{
		"type":        "task_progress",
		"task_id":     "task_002",
		"description": "Working...",
		"uuid":        "uuid-abc",
		"session_id":  "sess2",
		"usage": map[string]any{
			"total_tokens": float64(1000),
			"tool_uses":    float64(5),
			"duration_ms":  float64(3000),
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tp, ok := msg.(*TaskProgressMessage)
	if !ok {
		t.Fatalf("expected *TaskProgressMessage, got %T", msg)
	}
	if tp.Subtype != "task_progress" {
		t.Errorf("expected Subtype 'task_progress', got %q", tp.Subtype)
	}
	if tp.Data == nil {
		t.Fatal("expected Data to be populated, got nil")
	}
	if tp.Usage.TotalTokens != 1000 {
		t.Errorf("expected 1000 total tokens, got %d", tp.Usage.TotalTokens)
	}
}

func TestParseTaskNotificationMessage_EmbeddedSystemMessage(t *testing.T) {
	raw := map[string]any{
		"type":       "task_notification",
		"task_id":    "task_003",
		"status":     "completed",
		"summary":    "Done",
		"uuid":       "uuid-def",
		"session_id": "sess3",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tn, ok := msg.(*TaskNotificationMessage)
	if !ok {
		t.Fatalf("expected *TaskNotificationMessage, got %T", msg)
	}
	if tn.Subtype != "task_notification" {
		t.Errorf("expected Subtype 'task_notification', got %q", tn.Subtype)
	}
	if tn.Data == nil {
		t.Fatal("expected Data to be populated, got nil")
	}
}

func TestParseMessage_EmptyType_ReturnsError(t *testing.T) {
	raw := map[string]any{"subtype": "something"}
	msg, err := parseMessage(raw)
	if err == nil {
		t.Fatal("expected error for missing type field")
	}
	if msg != nil {
		t.Errorf("expected nil message, got %v", msg)
	}
	var parseErr *MessageParseError
	if _, ok := err.(*MessageParseError); !ok {
		t.Errorf("expected *MessageParseError, got %T", err)
	}
	_ = parseErr
}

func TestParseUnknownType(t *testing.T) {
	raw := map[string]any{"type": "unknown_xyz"}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Errorf("expected no error for unknown type (forward compat), got: %v", err)
	}
	if msg != nil {
		t.Errorf("expected nil message for unknown type, got: %v", msg)
	}
}

func TestStrVal(t *testing.T) {
	m := map[string]any{"key": "value", "num": 42}
	if strVal(m, "key") != "value" {
		t.Error("expected 'value'")
	}
	if strVal(m, "num") != "" {
		t.Error("expected '' for non-string key")
	}
	if strVal(m, "missing") != "" {
		t.Error("expected '' for missing key")
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		actual  string
		minimum string
		want    bool
	}{
		{"2.1.0", "2.0.0", true},
		{"2.0.0", "2.0.0", true},
		{"1.9.9", "2.0.0", false},
		{"3.0.0", "2.0.0", true},
		{"v2.1.0", "2.0.0", true},
	}
	for _, tc := range tests {
		got := versionAtLeast(tc.actual, tc.minimum)
		if got != tc.want {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tc.actual, tc.minimum, got, tc.want)
		}
	}
}

func TestNewUUID(t *testing.T) {
	u := newUUID()
	if len(u) != 36 {
		t.Errorf("expected UUID length 36, got %d: %q", len(u), u)
	}
	// Verify format: 8-4-4-4-12
	parts := []int{8, 4, 4, 4, 12}
	segments := []string{}
	start := 0
	for _, p := range parts {
		segments = append(segments, u[start:start+p])
		start += p + 1
	}
	if len(segments) != 5 {
		t.Errorf("expected 5 UUID segments, got %d", len(segments))
	}
}

// -----------------------------------------------------------------------
// New tests: field correctness (Phase 3 — matching Python test_message_parser.py)
// -----------------------------------------------------------------------

func TestParseUserMessage_StringContent(t *testing.T) {
	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role":    "user",
			"content": "Hello from user",
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	um, ok := msg.(*UserMessage)
	if !ok {
		t.Fatalf("expected *UserMessage, got %T", msg)
	}
	if um.Content != "Hello from user" {
		t.Errorf("expected string content %q, got %v", "Hello from user", um.Content)
	}
}

func TestParseUserMessage_AllFields(t *testing.T) {
	raw := map[string]any{
		"type":               "user",
		"uuid":               "user-uuid-123",
		"parent_tool_use_id": "toolu_parent",
		"tool_use_result":    map[string]any{"output": "ok"},
		"message": map[string]any{
			"role":    "user",
			"content": "test",
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	um := msg.(*UserMessage)
	if um.UUID != "user-uuid-123" {
		t.Errorf("wrong UUID: %q", um.UUID)
	}
	if um.ParentToolUseID != "toolu_parent" {
		t.Errorf("wrong ParentToolUseID: %q", um.ParentToolUseID)
	}
	if um.ToolUseResult == nil || um.ToolUseResult["output"] != "ok" {
		t.Errorf("wrong ToolUseResult: %v", um.ToolUseResult)
	}
}

func TestParseAssistantMessage_ModelAndError(t *testing.T) {
	raw := map[string]any{
		"type":               "assistant",
		"parent_tool_use_id": "toolu_abc",
		"message": map[string]any{
			"role":  "assistant",
			"model": "claude-opus-4",
			"error": "rate_limit_error",
			"content": []any{
				map[string]any{"type": "text", "text": "Sorry"},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am := msg.(*AssistantMessage)
	if am.Model != "claude-opus-4" {
		t.Errorf("wrong Model: %q", am.Model)
	}
	if am.Error != AssistantMessageErrorType("rate_limit_error") {
		t.Errorf("wrong Error: %q", am.Error)
	}
	if am.ParentToolUseID != "toolu_abc" {
		t.Errorf("wrong ParentToolUseID: %q", am.ParentToolUseID)
	}
}

func TestParseResultMessage_StopReason(t *testing.T) {
	raw := map[string]any{
		"type":        "result",
		"subtype":     "success",
		"is_error":    false,
		"session_id":  "sess",
		"stop_reason": "end_turn",
	}
	msg, _ := parseMessage(raw)
	rm := msg.(*ResultMessage)
	if rm.StopReason != "end_turn" {
		t.Errorf("wrong StopReason: %q", rm.StopReason)
	}
}

func TestParseResultMessage_NullStopReason(t *testing.T) {
	raw := map[string]any{
		"type":        "result",
		"subtype":     "success",
		"is_error":    false,
		"session_id":  "sess",
		"stop_reason": nil,
	}
	msg, _ := parseMessage(raw)
	rm := msg.(*ResultMessage)
	if rm.StopReason != "" {
		t.Errorf("expected empty StopReason for null, got %q", rm.StopReason)
	}
}

func TestParseSystemMessage_DataPopulated(t *testing.T) {
	raw := map[string]any{
		"type":       "system",
		"subtype":    "init",
		"session_id": "sess123",
	}
	msg, _ := parseMessage(raw)
	sm := msg.(*SystemMessage)
	if sm.Data == nil {
		t.Fatal("expected Data to be populated, got nil")
	}
	if sm.Data["session_id"] != "sess123" {
		t.Errorf("wrong Data[session_id]: %v", sm.Data["session_id"])
	}
}

func TestParseStreamEvent_ParentToolUseID(t *testing.T) {
	raw := map[string]any{
		"type":               "stream_event",
		"uuid":               "ev-uuid",
		"session_id":         "sess",
		"parent_tool_use_id": "toolu_ev",
		"event":              map[string]any{"delta": "chunk"},
	}
	msg, _ := parseMessage(raw)
	se := msg.(*StreamEvent)
	if se.ParentToolUseID != "toolu_ev" {
		t.Errorf("wrong ParentToolUseID: %q", se.ParentToolUseID)
	}
}

func TestParseUnknownContentBlock(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{"type": "text", "text": "hi"},
				map[string]any{"type": "future_block_type", "data": "x"},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am := msg.(*AssistantMessage)
	// Unknown block type should be silently skipped (forward compat).
	if len(am.Content) != 1 {
		t.Errorf("expected 1 block (unknown skipped), got %d", len(am.Content))
	}
}

// -----------------------------------------------------------------------
// Tests for new fields added in Python SDK v0.1.49–v0.1.58
// -----------------------------------------------------------------------

func TestParseAssistantMessage_NewFields(t *testing.T) {
	raw := map[string]any{
		"type":               "assistant",
		"uuid":               "asst-uuid-1",
		"session_id":         "sess-1",
		"parent_tool_use_id": "toolu_abc",
		"message": map[string]any{
			"role":        "assistant",
			"model":       "claude-sonnet-4-20250514",
			"id":          "msg_123",
			"stop_reason": "end_turn",
			"usage": map[string]any{
				"input_tokens":  float64(100),
				"output_tokens": float64(50),
			},
			"content": []any{
				map[string]any{"type": "text", "text": "Hello"},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am := msg.(*AssistantMessage)
	if am.UUID != "asst-uuid-1" {
		t.Errorf("wrong UUID: %q", am.UUID)
	}
	if am.SessionID != "sess-1" {
		t.Errorf("wrong SessionID: %q", am.SessionID)
	}
	if am.MessageID != "msg_123" {
		t.Errorf("wrong MessageID: %q", am.MessageID)
	}
	if am.StopReason != "end_turn" {
		t.Errorf("wrong StopReason: %q", am.StopReason)
	}
	if am.Usage == nil {
		t.Fatal("expected Usage to be populated")
	}
	if am.Usage["input_tokens"] != float64(100) {
		t.Errorf("wrong Usage[input_tokens]: %v", am.Usage["input_tokens"])
	}
}

func TestParseResultMessage_NewFields(t *testing.T) {
	raw := map[string]any{
		"type":        "result",
		"subtype":     "success",
		"is_error":    false,
		"session_id":  "sess-1",
		"uuid":        "result-uuid-1",
		"result":      "Done",
		"duration_ms": float64(1000),
		"modelUsage": map[string]any{
			"claude-sonnet-4-20250514": map[string]any{"input_tokens": float64(200)},
		},
		"permission_denials": []any{
			map[string]any{"tool": "Bash", "reason": "dangerous"},
		},
		"errors": []any{"error 1", "error 2"},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	rm := msg.(*ResultMessage)
	if rm.UUID != "result-uuid-1" {
		t.Errorf("wrong UUID: %q", rm.UUID)
	}
	if rm.ModelUsage == nil {
		t.Fatal("expected ModelUsage to be populated")
	}
	if len(rm.PermissionDenials) != 1 {
		t.Errorf("expected 1 permission denial, got %d", len(rm.PermissionDenials))
	}
	if len(rm.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(rm.Errors))
	}
	if rm.Errors[0] != "error 1" {
		t.Errorf("wrong error[0]: %q", rm.Errors[0])
	}
}

func TestParseRateLimitEvent_AllFields(t *testing.T) {
	raw := map[string]any{
		"type": "rate_limit_event",
		"rate_limit_info": map[string]any{
			"status":                "rejected",
			"resetsAt":              float64(1700003600),
			"rateLimitType":         "five_hour",
			"utilization":           0.95,
			"overageStatus":         "allowed_warning",
			"overageResetsAt":       float64(1700007200),
			"overageDisabledReason": "budget_exceeded",
		},
		"uuid":       "rl-uuid-1",
		"session_id": "sess-1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	evt := msg.(*RateLimitEvent)
	if evt.RateLimitInfo.Status != RateLimitRejected {
		t.Errorf("wrong status: %q", evt.RateLimitInfo.Status)
	}
	if evt.RateLimitInfo.RateLimitType == nil || *evt.RateLimitInfo.RateLimitType != RateLimitTypeFiveHour {
		t.Errorf("wrong rate limit type: %v", evt.RateLimitInfo.RateLimitType)
	}
	if evt.RateLimitInfo.Utilization == nil || *evt.RateLimitInfo.Utilization != 0.95 {
		t.Errorf("wrong utilization: %v", evt.RateLimitInfo.Utilization)
	}
	if evt.UUID != "rl-uuid-1" {
		t.Errorf("wrong UUID: %q", evt.UUID)
	}
}

func TestParseRateLimitEvent_MissingInfo(t *testing.T) {
	raw := map[string]any{
		"type":       "rate_limit_event",
		"uuid":       "rl-uuid-2",
		"session_id": "sess-1",
	}
	_, err := parseMessage(raw)
	if err == nil {
		t.Error("expected error for missing rate_limit_info")
	}
}

func TestParseServerToolUseBlock(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":  "server_tool_use",
					"id":    "stu_123",
					"name":  "web_search",
					"input": map[string]any{"query": "test"},
				},
				map[string]any{"type": "text", "text": "result"},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am := msg.(*AssistantMessage)
	if len(am.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(am.Content))
	}
	stu, ok := am.Content[0].(*ServerToolUseBlock)
	if !ok {
		t.Fatalf("expected *ServerToolUseBlock, got %T", am.Content[0])
	}
	if stu.Name != ServerToolWebSearch {
		t.Errorf("wrong name: %q", stu.Name)
	}
	if stu.ID != "stu_123" {
		t.Errorf("wrong ID: %q", stu.ID)
	}
}

func TestParseServerToolResultBlock(t *testing.T) {
	raw := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{
				map[string]any{
					"type":        "advisor_tool_result",
					"tool_use_id": "stu_123",
					"content":     map[string]any{"type": "text", "text": "advice"},
				},
			},
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	am := msg.(*AssistantMessage)
	tr, ok := am.Content[0].(*ServerToolResultBlock)
	if !ok {
		t.Fatalf("expected *ServerToolResultBlock, got %T", am.Content[0])
	}
	if tr.ToolUseID != "stu_123" {
		t.Errorf("wrong tool_use_id: %q", tr.ToolUseID)
	}
}

func TestParseMirrorErrorMessage(t *testing.T) {
	raw := map[string]any{
		"type":       "system",
		"subtype":    "mirror_error",
		"error":      "connection timeout",
		"session_id": "sess-1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	me, ok := msg.(*MirrorErrorMessage)
	if !ok {
		t.Fatalf("expected *MirrorErrorMessage, got %T", msg)
	}
	if me.Error != "connection timeout" {
		t.Errorf("wrong error: %q", me.Error)
	}
}

// TestParseToolResultBlock_ContentArray verifies that a tool_result block whose
// content is an array is preserved as-is (matching Python which does
// content=block.get("content") without unwrapping).
func TestParseToolResultBlock_ContentArray(t *testing.T) {
	contentArray := []any{
		map[string]any{"type": "text", "text": "first"},
		map[string]any{"type": "text", "text": "second"},
	}
	raw := map[string]any{
		"type":        "tool_result",
		"tool_use_id": "toolu_abc123",
		"content":     contentArray,
	}
	block, err := parseContentBlock(raw)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := block.(*ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", block)
	}
	got, ok := tr.Content.([]any)
	if !ok {
		t.Fatalf("expected Content to be []any, got %T (%v)", tr.Content, tr.Content)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(got))
	}
}

// TestParseToolResultBlock_ContentString verifies that a string content is
// preserved as a string.
func TestParseToolResultBlock_ContentString(t *testing.T) {
	raw := map[string]any{
		"type":        "tool_result",
		"tool_use_id": "toolu_abc123",
		"content":     "plain text result",
	}
	block, err := parseContentBlock(raw)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := block.(*ToolResultBlock)
	if !ok {
		t.Fatalf("expected *ToolResultBlock, got %T", block)
	}
	if tr.Content != "plain text result" {
		t.Errorf("expected %q, got %v", "plain text result", tr.Content)
	}
}

func TestAgentDefinition_NewFields(t *testing.T) {
	maxTurns := 10
	bg := true
	mode := PermissionModeDontAsk
	def := AgentDefinition{
		Description:     "Agent",
		Prompt:          "Do stuff",
		Tools:           []string{"Read"},
		DisallowedTools: []string{"Bash"},
		Model:           "claude-sonnet-4-20250514",
		Skills:          []string{"coding"},
		Memory:          "user",
		MaxTurns:        &maxTurns,
		Background:      &bg,
		PermissionMode:  &mode,
	}
	b, err := json.Marshal(def)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	if m["model"] != "claude-sonnet-4-20250514" {
		t.Errorf("wrong model in JSON: %v", m["model"])
	}
	if m["disallowedTools"] == nil {
		t.Error("expected disallowedTools in JSON")
	}
	if m["maxTurns"] != float64(10) {
		t.Errorf("wrong maxTurns: %v", m["maxTurns"])
	}
}

// --- TaskUpdatedMessage parsing ---

func TestParseTaskUpdatedMessage(t *testing.T) {
	raw := map[string]any{
		"type":      "system",
		"subtype":   "task_updated",
		"task_id":   "task-123",
		"session_id": "sess-456",
		"uuid":      "uuid-789",
		"patch": map[string]any{
			"status":  "completed",
			"end_time": "2025-01-01T00:00:00Z",
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum, ok := msg.(*TaskUpdatedMessage)
	if !ok {
		t.Fatalf("expected *TaskUpdatedMessage, got %T", msg)
	}
	if tum.TaskID != "task-123" {
		t.Errorf("TaskID = %q", tum.TaskID)
	}
	if tum.Status != "completed" {
		t.Errorf("Status = %q", tum.Status)
	}
	if tum.SessionID != "sess-456" {
		t.Errorf("SessionID = %q", tum.SessionID)
	}
	if tum.UUID != "uuid-789" {
		t.Errorf("UUID = %q", tum.UUID)
	}
	if tum.Patch == nil {
		t.Fatal("Patch should not be nil")
	}
	if tum.Patch["status"] != "completed" {
		t.Errorf("Patch.status = %v", tum.Patch["status"])
	}
	if tum.Subtype != "task_updated" {
		t.Errorf("Subtype = %q", tum.Subtype)
	}
}

func TestParseTaskUpdatedMessage_MissingPatch(t *testing.T) {
	raw := map[string]any{
		"type":    "system",
		"subtype": "task_updated",
		"task_id": "task-1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum, ok := msg.(*TaskUpdatedMessage)
	if !ok {
		t.Fatalf("expected *TaskUpdatedMessage, got %T", msg)
	}
	if tum.Patch == nil {
		t.Fatal("Patch should not be nil even when missing")
	}
	if tum.Status != "" {
		t.Errorf("Status should be empty, got %q", tum.Status)
	}
}

func TestParseTaskUpdatedMessage_NonDictPatch(t *testing.T) {
	raw := map[string]any{
		"type":    "system",
		"subtype": "task_updated",
		"task_id": "task-1",
		"patch":   "invalid",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum, ok := msg.(*TaskUpdatedMessage)
	if !ok {
		t.Fatalf("expected *TaskUpdatedMessage, got %T", msg)
	}
	if tum.Patch == nil {
		t.Fatal("Patch should not be nil even when non-dict")
	}
	if len(tum.Patch) != 0 {
		t.Errorf("Patch should be empty for non-dict, got %v", tum.Patch)
	}
}

func TestParseTaskUpdatedMessage_AllStatuses(t *testing.T) {
	statuses := []string{"pending", "running", "paused", "completed", "failed", "killed"}
	for _, status := range statuses {
		raw := map[string]any{
			"type":    "system",
			"subtype": "task_updated",
			"task_id": "t1",
			"patch":   map[string]any{"status": status},
		}
		msg, err := parseMessage(raw)
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		tum := msg.(*TaskUpdatedMessage)
		if string(tum.Status) != status {
			t.Errorf("status %q: got %q", status, tum.Status)
		}
	}
}

func TestParseTaskUpdatedMessage_TerminalStatuses(t *testing.T) {
	terminal := map[string]bool{
		"completed": true,
		"failed":    true,
		"stopped":   true,
		"killed":    true,
		"pending":   false,
		"running":   false,
		"paused":    false,
	}
	for status, want := range terminal {
		got := IsTerminalTaskStatus(status)
		if got != want {
			t.Errorf("TerminalTaskStatuses[%q] = %v, want %v", status, got, want)
		}
	}
}

func TestTaskUpdatedStatusConstants(t *testing.T) {
	expected := map[TaskUpdatedStatus]string{
		TaskUpdatedPending:   "pending",
		TaskUpdatedRunning:   "running",
		TaskUpdatedPaused:    "paused",
		TaskUpdatedCompleted: "completed",
		TaskUpdatedFailed:    "failed",
		TaskUpdatedKilled:    "killed",
	}
	for status, want := range expected {
		if string(status) != want {
			t.Errorf("TaskUpdatedStatus %v = %q, want %q", status, string(status), want)
		}
	}
}

func TestParseTaskUpdatedMessage_RoundTrip(t *testing.T) {
	raw := map[string]any{
		"type":      "system",
		"subtype":   "task_updated",
		"task_id":   "t1",
		"session_id": "s1",
		"uuid":      "u1",
		"patch": map[string]any{
			"status":  "killed",
			"end_time": float64(1234567890),
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum := msg.(*TaskUpdatedMessage)
	// Verify the full patch is preserved.
	if tum.Patch["end_time"] != float64(1234567890) {
		t.Errorf("Patch.end_time = %v", tum.Patch["end_time"])
	}
	// Verify terminal status detection.
	if !IsTerminalTaskStatus(string(tum.Status)) {
		t.Errorf("killed should be terminal")
	}
}

// --- Additional tests matching Python SDK test_message_parser.py ---

func TestParseTaskUpdatedMessage_Minimal(t *testing.T) {
	// Minimal message with only task_id and patch (no uuid/session_id).
	// Mirrors the observed CLI shape where terminal completion arrives as
	// a bare task_updated patch — parsing must never raise.
	raw := map[string]any{
		"type":    "system",
		"subtype": "task_updated",
		"task_id": "b1m21w89v",
		"patch":   map[string]any{"status": "completed", "end_time": float64(1780405729183)},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum, ok := msg.(*TaskUpdatedMessage)
	if !ok {
		t.Fatalf("expected *TaskUpdatedMessage, got %T", msg)
	}
	if tum.TaskID != "b1m21w89v" {
		t.Errorf("TaskID = %q", tum.TaskID)
	}
	if tum.Status != TaskUpdatedCompleted {
		t.Errorf("Status = %q, want %q", tum.Status, TaskUpdatedCompleted)
	}
	if tum.UUID != "" {
		t.Errorf("UUID should be empty, got %q", tum.UUID)
	}
	if tum.SessionID != "" {
		t.Errorf("SessionID should be empty, got %q", tum.SessionID)
	}
}

func TestParseTaskUpdatedMessage_NonTerminalStatuses(t *testing.T) {
	// Non-terminal statuses should NOT be in TerminalTaskStatuses.
	for _, status := range []string{"pending", "running", "paused"} {
		raw := map[string]any{
			"type":    "system",
			"subtype": "task_updated",
			"task_id": "task-abc",
			"patch":   map[string]any{"status": status},
		}
		msg, err := parseMessage(raw)
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		tum := msg.(*TaskUpdatedMessage)
		if string(tum.Status) != status {
			t.Errorf("status %q: got %q", status, tum.Status)
		}
		if IsTerminalTaskStatus(status) {
			t.Errorf("status %q should NOT be terminal", status)
		}
	}
}

func TestParseTaskUpdatedMessage_PatchWithoutStatus(t *testing.T) {
	// A patch lacking 'status' is preserved verbatim; status is empty.
	raw := map[string]any{
		"type":    "system",
		"subtype": "task_updated",
		"task_id": "task-abc",
		"patch":   map[string]any{"end_time": float64(1780405729183)},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum := msg.(*TaskUpdatedMessage)
	if tum.Patch["end_time"] != float64(1780405729183) {
		t.Errorf("Patch.end_time = %v", tum.Patch["end_time"])
	}
	if tum.Status != "" {
		t.Errorf("Status should be empty, got %q", tum.Status)
	}
}

func TestParseTaskUpdatedMessage_KilledIsTerminal(t *testing.T) {
	// A task stopped via TaskStop reports status='killed' and is terminal.
	// In some kill paths no task_notification is emitted, so this task_updated
	// patch is the only terminal signal.
	raw := map[string]any{
		"type":    "system",
		"subtype": "task_updated",
		"task_id": "bs2r8eew4",
		"patch":   map[string]any{"status": "killed", "end_time": float64(1780405729183)},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	tum := msg.(*TaskUpdatedMessage)
	if tum.Status != TaskUpdatedKilled {
		t.Errorf("Status = %q, want %q", tum.Status, TaskUpdatedKilled)
	}
	if !IsTerminalTaskStatus(string(tum.Status)) {
		t.Errorf("killed should be terminal")
	}
}

func TestParseTaskUpdatedMessage_NotOtherMessageTypes(t *testing.T) {
	// task_updated should NOT parse as any other message type.
	raw := map[string]any{
		"type":    "system",
		"subtype": "task_updated",
		"task_id": "t1",
		"patch":   map[string]any{"status": "completed"},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := msg.(*TaskUpdatedMessage); !ok {
		t.Fatalf("expected *TaskUpdatedMessage, got %T", msg)
	}
	if _, ok := msg.(*TaskStartedMessage); ok {
		t.Error("should not be TaskStartedMessage")
	}
	if _, ok := msg.(*TaskProgressMessage); ok {
		t.Error("should not be TaskProgressMessage")
	}
	if _, ok := msg.(*TaskNotificationMessage); ok {
		t.Error("should not be TaskNotificationMessage")
	}
}

func TestParseTaskUpdatedMessage_NonDictPatchVariants(t *testing.T) {
	// Non-dict patches should never raise; patch falls back to empty map.
	variants := []any{"completed", []any{"completed"}, float64(42), nil}
	for _, patch := range variants {
		raw := map[string]any{
			"type":    "system",
			"subtype": "task_updated",
			"task_id": "task-abc",
			"patch":   patch,
		}
		msg, err := parseMessage(raw)
		if err != nil {
			t.Fatalf("patch %v: %v", patch, err)
		}
		tum := msg.(*TaskUpdatedMessage)
		if len(tum.Patch) != 0 {
			t.Errorf("patch %v: expected empty map, got %v", patch, tum.Patch)
		}
		if tum.Status != "" {
			t.Errorf("patch %v: expected empty status, got %q", patch, tum.Status)
		}
	}
}
