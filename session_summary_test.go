package claude

import (
	"testing"
)

func TestFoldSessionSummary_SetOnceFields(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	entries := []SessionStoreEntry{
		{
			Type: "user",
			Extra: map[string]any{
				"isSidechain": false,
				"timestamp":   "2024-06-15T10:30:00.000Z",
				"cwd":         "/home/user/project",
				"message":     map[string]any{"content": "hello world"},
			},
		},
		{
			Type: "assistant",
			Extra: map[string]any{
				"isSidechain": true, // should NOT overwrite (set-once)
				"timestamp":   "2024-06-15T10:31:00.000Z",
				"cwd":         "/different/path", // should NOT overwrite (set-once)
			},
		},
	}

	result := FoldSessionSummary(nil, key, entries)

	if result.Data["is_sidechain"] != false {
		t.Errorf("is_sidechain should be false (set-once), got %v", result.Data["is_sidechain"])
	}
	if result.Data["created_at"] == nil {
		t.Error("created_at should be set")
	}
	if result.Data["cwd"] != "/home/user/project" {
		t.Errorf("cwd should be /home/user/project (set-once), got %v", result.Data["cwd"])
	}
}

func TestFoldSessionSummary_FirstPromptFiltering(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	entries := []SessionStoreEntry{
		// Meta entry - should be skipped.
		{
			Type: "user",
			Extra: map[string]any{
				"isMeta":   true,
				"message":  map[string]any{"content": "meta message"},
			},
		},
		// Slash command - should be stashed as fallback.
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": "<command-name>help</command-name>"},
			},
		},
		// Auto-generated - should be skipped.
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": "<local-command-stdout>output</local-command-stdout>"},
			},
		},
		// Real user prompt - should be selected.
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": "What is the meaning of life?"},
			},
		},
	}

	result := FoldSessionSummary(nil, key, entries)

	if result.Data["first_prompt"] != "What is the meaning of life?" {
		t.Errorf("first_prompt = %v, want 'What is the meaning of life?'", result.Data["first_prompt"])
	}
	if result.Data["command_fallback"] != "help" {
		t.Errorf("command_fallback = %v, want 'help'", result.Data["command_fallback"])
	}
}

func TestFoldSessionSummary_FirstPromptLocked(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	entries := []SessionStoreEntry{
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": "first prompt"},
			},
		},
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": "second prompt"},
			},
		},
	}

	result := FoldSessionSummary(nil, key, entries)

	if result.Data["first_prompt"] != "first prompt" {
		t.Errorf("first_prompt = %v, want 'first prompt'", result.Data["first_prompt"])
	}
}

func TestFoldSessionSummary_FirstPromptTruncation(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	longText := ""
	for i := 0; i < 300; i++ {
		longText += "a"
	}

	entries := []SessionStoreEntry{
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": longText},
			},
		},
	}

	result := FoldSessionSummary(nil, key, entries)

	prompt, ok := result.Data["first_prompt"].(string)
	if !ok {
		t.Fatal("first_prompt not set")
	}
	if len([]rune(prompt)) > 201 { // 200 chars + "…"
		t.Errorf("first_prompt too long: %d chars", len(prompt))
	}
}

func TestFoldSessionSummary_LastWinsMapping(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	entries := []SessionStoreEntry{
		{
			Type: "user",
			Extra: map[string]any{
				"customTitle": "My Session",
				"aiTitle":     "AI Title",
				"summary":     "Session summary",
				"lastPrompt":  "last user message",
				"gitBranch":   "main",
			},
		},
	}

	result := FoldSessionSummary(nil, key, entries)

	if result.Data["custom_title"] != "My Session" {
		t.Errorf("custom_title = %v", result.Data["custom_title"])
	}
	if result.Data["ai_title"] != "AI Title" {
		t.Errorf("ai_title = %v", result.Data["ai_title"])
	}
	if result.Data["summary_hint"] != "Session summary" {
		t.Errorf("summary_hint = %v", result.Data["summary_hint"])
	}
	if result.Data["last_prompt"] != "last user message" {
		t.Errorf("last_prompt = %v", result.Data["last_prompt"])
	}
	if result.Data["git_branch"] != "main" {
		t.Errorf("git_branch = %v", result.Data["git_branch"])
	}
}

func TestFoldSessionSummary_TagHandling(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	entries := []SessionStoreEntry{
		{Type: "tag", Extra: map[string]any{"tag": "v1.0"}},
		{Type: "tag", Extra: map[string]any{"tag": ""}}, // clear
	}

	result := FoldSessionSummary(nil, key, entries)

	if _, has := result.Data["tag"]; has {
		t.Error("tag should be cleared when empty")
	}
}

func TestFoldSessionSummary_IsoToEpochMs(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"2024-06-15T10:30:00.000Z", 1718447400000},
		{"2024-01-01T00:00:00.000Z", 1704067200000},
		{"invalid", 0},
		{"", 0},
	}

	for _, tt := range tests {
		got := isoToEpochMs(tt.input)
		if tt.want == 0 {
			if got != 0 {
				t.Errorf("isoToEpochMs(%q) = %d, want 0", tt.input, got)
			}
		} else if got == 0 {
			t.Errorf("isoToEpochMs(%q) = 0, want non-zero", tt.input)
		}
	}
}

func TestFoldSessionSummary_ToolResultSkipped(t *testing.T) {
	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	entries := []SessionStoreEntry{
		// User message with tool_result - should be skipped for first_prompt.
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{
					"content": []any{
						map[string]any{"type": "tool_result", "tool_use_id": "tu1"},
					},
				},
			},
		},
		// Real user message.
		{
			Type: "user",
			Extra: map[string]any{
				"message": map[string]any{"content": "real question"},
			},
		},
	}

	result := FoldSessionSummary(nil, key, entries)

	if result.Data["first_prompt"] != "real question" {
		t.Errorf("first_prompt = %v, want 'real question'", result.Data["first_prompt"])
	}
}
