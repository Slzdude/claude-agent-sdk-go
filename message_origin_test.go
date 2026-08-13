package claude

import (
	"testing"
)

func TestParseOrigin(t *testing.T) {
	// Valid origin
	raw := map[string]any{
		"origin": map[string]any{
			"kind":    "task-notification",
			"server":  "my-server",
			"subkind": "scheduled-trigger",
		},
	}
	origin := parseOrigin(raw)
	if origin == nil {
		t.Fatal("expected non-nil origin")
	}
	if origin.Kind != "task-notification" {
		t.Errorf("Kind = %q", origin.Kind)
	}
	if origin.Server != "my-server" {
		t.Errorf("Server = %q", origin.Server)
	}
	if origin.Subkind != "scheduled-trigger" {
		t.Errorf("Subkind = %q", origin.Subkind)
	}
}

func TestParseOrigin_Missing(t *testing.T) {
	raw := map[string]any{"type": "user"}
	origin := parseOrigin(raw)
	if origin != nil {
		t.Errorf("expected nil, got %v", origin)
	}
}

func TestParseOrigin_InvalidKind(t *testing.T) {
	raw := map[string]any{
		"origin": map[string]any{"kind": 123},
	}
	origin := parseOrigin(raw)
	if origin != nil {
		t.Errorf("expected nil for non-string kind, got %v", origin)
	}
}

func TestParseOrigin_PeerWithVerifiedPeerPID(t *testing.T) {
	raw := map[string]any{
		"origin": map[string]any{
			"kind":            "peer",
			"from":            "session-123",
			"name":            "Alice",
			"verifiedPeerPid": float64(12345),
		},
	}
	origin := parseOrigin(raw)
	if origin == nil {
		t.Fatal("expected non-nil origin")
	}
	if origin.Kind != "peer" {
		t.Errorf("Kind = %q", origin.Kind)
	}
	if origin.VerifiedPeerPID == nil || *origin.VerifiedPeerPID != 12345 {
		t.Errorf("VerifiedPeerPID = %v", origin.VerifiedPeerPID)
	}
}

func TestParseConversationResetMessage(t *testing.T) {
	raw := map[string]any{
		"type":                "conversation_reset",
		"new_conversation_id": "conv-abc",
		"uuid":                "u1",
		"session_id":          "s1",
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	crm, ok := msg.(*ConversationResetMessage)
	if !ok {
		t.Fatalf("expected *ConversationResetMessage, got %T", msg)
	}
	if crm.NewConversationID != "conv-abc" {
		t.Errorf("NewConversationID = %q", crm.NewConversationID)
	}
	if crm.UUID != "u1" {
		t.Errorf("UUID = %q", crm.UUID)
	}
	if crm.SessionID != "s1" {
		t.Errorf("SessionID = %q", crm.SessionID)
	}
}

func TestParseUserMessage_WithOrigin(t *testing.T) {
	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "hello",
		},
		"origin": map[string]any{
			"kind": "human",
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
	if um.Origin == nil {
		t.Fatal("expected non-nil origin")
	}
	if um.Origin.Kind != "human" {
		t.Errorf("Kind = %q", um.Origin.Kind)
	}
}

func TestParseResultMessage_WithOriginAndTerminalReason(t *testing.T) {
	raw := map[string]any{
		"type":            "result",
		"subtype":         "success",
		"session_id":      "s1",
		"duration_ms":     float64(100),
		"duration_api_ms": float64(50),
		"num_turns":       float64(1),
		"terminal_reason": "completed",
		"origin": map[string]any{
			"kind": "task-notification",
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	rm, ok := msg.(*ResultMessage)
	if !ok {
		t.Fatalf("expected *ResultMessage, got %T", msg)
	}
	if rm.TerminalReason != "completed" {
		t.Errorf("TerminalReason = %q", rm.TerminalReason)
	}
	if rm.Origin == nil || rm.Origin.Kind != "task-notification" {
		t.Errorf("Origin = %v", rm.Origin)
	}
}

func TestMessageOriginKind_Constants(t *testing.T) {
	expected := map[string]string{
		OriginKindHuman:            "human",
		OriginKindChannel:          "channel",
		OriginKindPeer:             "peer",
		OriginKindTaskNotification: "task-notification",
		OriginKindCoordinator:      "coordinator",
		OriginKindUnclassified:     "unclassified",
		OriginKindObserver:         "observer",
		OriginKindAutoContinuation: "auto-continuation",
		OriginKindObserverActivity: "observer-activity",
	}
	for kind, want := range expected {
		if kind != want {
			t.Errorf("OriginKind %q != %q", kind, want)
		}
	}
}

func TestModelUsage_Fields(t *testing.T) {
	mu := ModelUsage{
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      0.01,
	}
	if mu.InputTokens != 100 {
		t.Errorf("InputTokens = %d", mu.InputTokens)
	}
	if mu.OutputTokens != 50 {
		t.Errorf("OutputTokens = %d", mu.OutputTokens)
	}
}
