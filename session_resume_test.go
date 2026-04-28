package claude

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteJSONL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.jsonl")

	entries := []SessionStoreEntry{
		{Type: "user", UUID: "u1", Extra: map[string]any{"message": "hello"}},
		{Type: "assistant", UUID: "a1", Extra: map[string]any{"response": "hi"}},
	}

	if err := writeJSONL(path, entries); err != nil {
		t.Fatalf("writeJSONL: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var first map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("unmarshal first line: %v", err)
	}
	if first["type"] != "user" {
		t.Errorf("expected type=user, got %v", first["type"])
	}

	// Check file permissions (0600).
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("expected mode 0600, got %o", info.Mode().Perm())
	}
}

func TestWriteRedactedCredentials(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".credentials.json")

	// Input with refreshToken.
	input := `{"claudeAiOauth":{"accessToken":"at_123","refreshToken":"rt_456"},"other":"data"}`
	writeRedactedCredentials(input, dst)

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	oauth, ok := parsed["claudeAiOauth"].(map[string]any)
	if !ok {
		t.Fatal("claudeAiOauth not found")
	}
	if _, has := oauth["refreshToken"]; has {
		t.Error("refreshToken should have been removed")
	}
	if oauth["accessToken"] != "at_123" {
		t.Errorf("accessToken should be preserved, got %v", oauth["accessToken"])
	}
	if parsed["other"] != "data" {
		t.Errorf("other field should be preserved, got %v", parsed["other"])
	}
}

func TestWriteRedactedCredentials_NoOAuth(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".credentials.json")

	input := `{"someOther":"data"}`
	writeRedactedCredentials(input, dst)

	data, _ := os.ReadFile(dst)
	if string(data) != input {
		t.Errorf("expected passthrough, got %s", string(data))
	}
}

func TestWriteRedactedCredentials_Nil(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, ".credentials.json")

	writeRedactedCredentials("", dst)

	// File should be written (empty string).
	data, _ := os.ReadFile(dst)
	if len(data) != 0 {
		t.Errorf("expected empty file, got %s", string(data))
	}
}

func TestIsSafeSubpath(t *testing.T) {
	sessionDir := "/tmp/projects/proj/sess"

	tests := []struct {
		subpath string
		safe    bool
	}{
		{"subagents/agent-1", true},
		{"subagents/workflows/run-1/agent-2", true},
		{"", false},
		{"../escape", false},
		{"/absolute", false},
		{"subagents/../../../etc/passwd", false},
		{"subagents/..", false},
		{"C:\\Windows", false},
	}

	for _, tt := range tests {
		got := isSafeSubpath(tt.subpath, sessionDir)
		if got != tt.safe {
			t.Errorf("isSafeSubpath(%q, %q) = %v, want %v", tt.subpath, sessionDir, got, tt.safe)
		}
	}
}

func TestApplyMaterializedOptions(t *testing.T) {
	opts := ClaudeAgentOptions{
		Resume:              "old-resume-id",
		ContinueConversation: true,
		Env: map[string]string{
			"FOO": "bar",
		},
		Model: "claude-sonnet-4-5",
	}

	m := &MaterializedResume{
		ConfigDir:       "/tmp/claude-resume-xyz",
		ResumeSessionID: "new-session-id",
		Cleanup:         func() {},
	}

	result := ApplyMaterializedOptions(opts, m)

	if result.Resume != "new-session-id" {
		t.Errorf("Resume = %q, want %q", result.Resume, "new-session-id")
	}
	if result.ContinueConversation {
		t.Error("ContinueConversation should be false")
	}
	if result.Env["CLAUDE_CONFIG_DIR"] != "/tmp/claude-resume-xyz" {
		t.Errorf("CLAUDE_CONFIG_DIR = %q, want %q", result.Env["CLAUDE_CONFIG_DIR"], "/tmp/claude-resume-xyz")
	}
	if result.Env["FOO"] != "bar" {
		t.Errorf("FOO should be preserved")
	}
	if result.Model != "claude-sonnet-4-5" {
		t.Errorf("Model should be preserved")
	}
}

func TestMaterializeResumeSession_NoStore(t *testing.T) {
	opts := &ClaudeAgentOptions{Resume: "some-id"}
	result, err := MaterializeResumeSession(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil when no store")
	}
}

func TestMaterializeResumeSession_NoResume(t *testing.T) {
	store := NewInMemorySessionStore()
	opts := &ClaudeAgentOptions{SessionStore: store}
	result, err := MaterializeResumeSession(context.Background(), opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil when no resume/continue")
	}
}

func TestMaterializeResumeSession_WithStore(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	// Use a temp dir as CWD so ProjectKeyForDirectory produces a known key.
	tmpCWD := t.TempDir()
	projectKey := ProjectKeyForDirectory(tmpCWD)
	sessionID := "550e8400-e29b-41d4-a716-446655440000"
	key := SessionKey{ProjectKey: projectKey, SessionID: sessionID}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "u1", Extra: map[string]any{"message": map[string]any{"role": "user", "content": "hello"}}},
		{Type: "assistant", UUID: "a1", Extra: map[string]any{"message": map[string]any{"role": "assistant", "content": "hi"}}},
	}
	if err := store.Append(key, entries); err != nil {
		t.Fatalf("append: %v", err)
	}

	opts := &ClaudeAgentOptions{
		SessionStore: store,
		Resume:       sessionID,
		CWD:          tmpCWD,
		LoadTimeoutMs: 5000,
	}

	result, err := MaterializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("MaterializeResumeSession: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	defer result.Cleanup()

	if result.ResumeSessionID != sessionID {
		t.Errorf("ResumeSessionID = %q, want %q", result.ResumeSessionID, sessionID)
	}

	// Verify the temp directory structure.
	jsonlPath := filepath.Join(result.ConfigDir, "projects", projectKey, sessionID+".jsonl")
	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read jsonl: %v", err)
	}
	lines := splitLines(string(data))
	if len(lines) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lines))
	}

	// Verify cleanup works.
	result.Cleanup()
	if _, err := os.Stat(result.ConfigDir); !os.IsNotExist(err) {
		t.Error("temp dir should be removed after cleanup")
	}
}

func TestMaterializeResumeSession_ContinueConversation(t *testing.T) {
	store := NewInMemorySessionStore()
	ctx := context.Background()

	tmpCWD := t.TempDir()
	projectKey := ProjectKeyForDirectory(tmpCWD)
	sessionID := "550e8400-e29b-41d4-a716-446655440000"
	key := SessionKey{ProjectKey: projectKey, SessionID: sessionID}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "u1", Extra: map[string]any{"message": "hello"}},
	}
	store.Append(key, entries)

	opts := &ClaudeAgentOptions{
		SessionStore:         store,
		ContinueConversation: true,
		CWD:                  tmpCWD,
		LoadTimeoutMs:        5000,
	}

	result, err := MaterializeResumeSession(ctx, opts)
	if err != nil {
		t.Fatalf("MaterializeResumeSession: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for continue_conversation")
	}
	defer result.Cleanup()

	if result.ResumeSessionID != sessionID {
		t.Errorf("ResumeSessionID = %q, want %q", result.ResumeSessionID, sessionID)
	}
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range splitByNewline(s) {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func splitByNewline(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}
