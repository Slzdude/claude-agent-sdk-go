package claude

import (
	"testing"
)

// TestTextBlock verifies TextBlock creation and type assertion.
func TestTextBlock(t *testing.T) {
	b := &TextBlock{Text: "Hello, human!"}
	if b.Text != "Hello, human!" {
		t.Errorf("expected Text=%q, got %q", "Hello, human!", b.Text)
	}
	if b.contentBlockType() != "text" {
		t.Errorf("expected contentBlockType=text, got %s", b.contentBlockType())
	}
}

// TestThinkingBlock verifies ThinkingBlock creation.
func TestThinkingBlock(t *testing.T) {
	b := &ThinkingBlock{Thinking: "I'm thinking...", Signature: "sig-123"}
	if b.Thinking != "I'm thinking..." {
		t.Error("wrong Thinking")
	}
	if b.Signature != "sig-123" {
		t.Error("wrong Signature")
	}
	if b.contentBlockType() != "thinking" {
		t.Errorf("expected thinking, got %s", b.contentBlockType())
	}
}

// TestToolUseBlock verifies ToolUseBlock fields.
func TestToolUseBlock(t *testing.T) {
	b := &ToolUseBlock{
		ID:    "tool-123",
		Name:  "Read",
		Input: map[string]any{"file_path": "/test.txt"},
	}
	if b.ID != "tool-123" {
		t.Error("wrong ID")
	}
	if b.Name != "Read" {
		t.Error("wrong Name")
	}
	if b.Input["file_path"] != "/test.txt" {
		t.Error("wrong Input")
	}
	if b.contentBlockType() != "tool_use" {
		t.Errorf("expected tool_use, got %s", b.contentBlockType())
	}
}

// TestToolResultBlock verifies ToolResultBlock fields.
func TestToolResultBlock(t *testing.T) {
	isErr := false
	b := &ToolResultBlock{
		ToolUseID: "tool-123",
		Content:   "File contents here",
		IsError:   &isErr,
	}
	if b.ToolUseID != "tool-123" {
		t.Error("wrong ToolUseID")
	}
	if b.Content != "File contents here" {
		t.Error("wrong Content")
	}
	if b.IsError != nil && *b.IsError {
		t.Error("IsError should be false")
	}
	if b.contentBlockType() != "tool_result" {
		t.Errorf("expected tool_result, got %s", b.contentBlockType())
	}
}

// TestAssistantMessage verifies AssistantMessage creation.
func TestAssistantMessage(t *testing.T) {
	b := &TextBlock{Text: "Hello!"}
	m := &AssistantMessage{
		Content: []ContentBlock{b},
		Model:   "claude-opus-4-5",
	}
	if len(m.Content) != 1 {
		t.Errorf("expected 1 block, got %d", len(m.Content))
	}
	if m.Content[0].(*TextBlock).Text != "Hello!" {
		t.Error("wrong text block content")
	}
	if m.messageType() != "assistant" {
		t.Error("wrong messageType")
	}
}

// TestResultMessage verifies ResultMessage fields.
func TestResultMessage(t *testing.T) {
	cost := 0.01
	m := &ResultMessage{
		Subtype:       "success",
		DurationMs:    1500,
		DurationAPIMs: 1200,
		IsError:       false,
		NumTurns:      1,
		SessionID:     "session-123",
		TotalCostUSD:  &cost,
	}
	if m.Subtype != "success" {
		t.Error("wrong Subtype")
	}
	if *m.TotalCostUSD != 0.01 {
		t.Error("wrong TotalCostUSD")
	}
	if m.SessionID != "session-123" {
		t.Error("wrong SessionID")
	}
	if m.messageType() != "result" {
		t.Error("wrong messageType")
	}
}

// TestClaudeAgentOptions_Defaults checks zero-value defaults.
func TestClaudeAgentOptions_Defaults(t *testing.T) {
	opts := &ClaudeAgentOptions{}
	if opts.AllowedTools != nil {
		t.Error("AllowedTools should be nil by default")
	}
	if opts.SystemPrompt != nil {
		t.Error("SystemPrompt should be nil by default")
	}
	if opts.PermissionMode != "" {
		t.Error("PermissionMode should be empty by default")
	}
	if opts.ContinueConversation {
		t.Error("ContinueConversation should be false by default")
	}
}

// TestClaudeAgentOptions_WithTools checks setting tools.
func TestClaudeAgentOptions_WithTools(t *testing.T) {
	opts := &ClaudeAgentOptions{
		AllowedTools:    []string{"Read", "Write", "Edit"},
		DisallowedTools: []string{"Bash"},
	}
	if len(opts.AllowedTools) != 3 {
		t.Errorf("expected 3 allowed tools, got %d", len(opts.AllowedTools))
	}
	if len(opts.DisallowedTools) != 1 {
		t.Errorf("expected 1 disallowed tool, got %d", len(opts.DisallowedTools))
	}
}

// TestClaudeAgentOptions_PermissionModes checks all permission mode constants.
func TestClaudeAgentOptions_PermissionModes(t *testing.T) {
	modes := []PermissionMode{
		PermissionModeDefault,
		PermissionModeAcceptEdits,
		PermissionModePlan,
		PermissionModeBypassPermissions,
	}
	expected := []string{"default", "acceptEdits", "plan", "bypassPermissions"}
	for i, mode := range modes {
		if string(mode) != expected[i] {
			t.Errorf("mode[%d]: expected %q, got %q", i, expected[i], mode)
		}
	}
	opts := &ClaudeAgentOptions{PermissionMode: PermissionModeBypassPermissions}
	if opts.PermissionMode != "bypassPermissions" {
		t.Error("wrong PermissionMode")
	}
}

// TestClaudeAgentOptions_SystemPromptString checks string system prompt.
func TestClaudeAgentOptions_SystemPromptString(t *testing.T) {
	opts := &ClaudeAgentOptions{
		SystemPrompt: "You are a helpful assistant.",
	}
	if opts.SystemPrompt.(string) != "You are a helpful assistant." {
		t.Error("wrong SystemPrompt string")
	}
}

// TestClaudeAgentOptions_SystemPromptPreset checks preset system prompt.
func TestClaudeAgentOptions_SystemPromptPreset(t *testing.T) {
	opts := &ClaudeAgentOptions{
		SystemPrompt: &SystemPromptPreset{Append: "Be concise."},
	}
	sp, ok := opts.SystemPrompt.(*SystemPromptPreset)
	if !ok {
		t.Fatal("SystemPrompt should be *SystemPromptPreset")
	}
	if sp.Append != "Be concise." {
		t.Errorf("wrong Append: %q", sp.Append)
	}
}

// TestClaudeAgentOptions_SessionContinuation checks session continuation.
func TestClaudeAgentOptions_SessionContinuation(t *testing.T) {
	opts := &ClaudeAgentOptions{
		ContinueConversation: true,
		Resume:               "session-123",
	}
	if !opts.ContinueConversation {
		t.Error("ContinueConversation should be true")
	}
	if opts.Resume != "session-123" {
		t.Errorf("wrong Resume: %q", opts.Resume)
	}
}

// TestClaudeAgentOptions_ModelAndPermissionTool checks model + permission tool fields.
func TestClaudeAgentOptions_ModelAndPermissionTool(t *testing.T) {
	opts := &ClaudeAgentOptions{
		Model:                    "claude-sonnet-4-5",
		PermissionPromptToolName: "CustomTool",
	}
	if opts.Model != "claude-sonnet-4-5" {
		t.Errorf("wrong Model: %q", opts.Model)
	}
	if opts.PermissionPromptToolName != "CustomTool" {
		t.Errorf("wrong PermissionPromptToolName: %q", opts.PermissionPromptToolName)
	}
}

// TestSettingSourceConstants checks SettingSource string values.
func TestSettingSourceConstants(t *testing.T) {
	if SettingSourceUser != "user" {
		t.Error("SettingSourceUser should be 'user'")
	}
	if SettingSourceProject != "project" {
		t.Error("SettingSourceProject should be 'project'")
	}
	if SettingSourceLocal != "local" {
		t.Error("SettingSourceLocal should be 'local'")
	}
}

// TestPermissionResultAllow checks PermissionResultAllow interface.
func TestPermissionResultAllow(t *testing.T) {
	r := &PermissionResultAllow{
		UpdatedInput: map[string]any{"safe_mode": true},
	}
	// Must satisfy PermissionResult interface.
	var _ PermissionResult = r
	if r.UpdatedInput["safe_mode"] != true {
		t.Error("wrong UpdatedInput")
	}
}

// TestPermissionResultDeny checks PermissionResultDeny fields.
func TestPermissionResultDeny(t *testing.T) {
	r := &PermissionResultDeny{
		Message:   "Security policy violation",
		Interrupt: true,
	}
	var _ PermissionResult = r
	if r.Message != "Security policy violation" {
		t.Error("wrong Message")
	}
	if !r.Interrupt {
		t.Error("Interrupt should be true")
	}
}

// TestAgentDefinition checks AgentDefinition fields.
func TestAgentDefinition(t *testing.T) {
	def := AgentDefinition{
		Description: "Does research",
		Tools:       []string{"Read", "Grep"},
		Model:       "claude-sonnet-4-5",
	}
	if def.Description != "Does research" {
		t.Error("wrong Description")
	}
	if len(def.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(def.Tools))
	}
}
