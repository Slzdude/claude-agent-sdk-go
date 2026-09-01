package claude

import (
	"testing"
)

func TestParseUserMessage_ParentAgentID(t *testing.T) {
	raw := map[string]any{
		"type":              "user",
		"parent_agent_id":   "agent-1",
		"parent_tool_use_id": "tu-1",
		"message": map[string]any{
			"content": "hello",
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
	if um.ParentAgentID != "agent-1" {
		t.Errorf("ParentAgentID = %q", um.ParentAgentID)
	}
	if um.ParentToolUseID != "tu-1" {
		t.Errorf("ParentToolUseID = %q", um.ParentToolUseID)
	}
}

func TestParseUserMessage_NoParentAgentID(t *testing.T) {
	raw := map[string]any{
		"type": "user",
		"message": map[string]any{
			"content": "hello",
		},
	}
	msg, err := parseMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	um := msg.(*UserMessage)
	if um.ParentAgentID != "" {
		t.Errorf("ParentAgentID should be empty, got %q", um.ParentAgentID)
	}
}

func TestBuildCommand_ForwardSubagentText(t *testing.T) {
	opts := &ClaudeAgentOptions{
		ForwardSubagentText: true,
	}
	tr := &cliTransport{opts: opts, cliPath: "claude"}
	cmd := tr.buildCommand()
	found := false
	for _, arg := range cmd {
		if arg == "--forward-subagent-text" {
			found = true
		}
	}
	if !found {
		t.Error("expected --forward-subagent-text in command")
	}
}

func TestBuildCommand_ForwardSubagentText_False(t *testing.T) {
	opts := &ClaudeAgentOptions{
		ForwardSubagentText: false,
	}
	tr := &cliTransport{opts: opts, cliPath: "claude"}
	cmd := tr.buildCommand()
	for _, arg := range cmd {
		if arg == "--forward-subagent-text" {
			t.Error("--forward-subagent-text should not be in command when false")
		}
	}
}
