package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ForkSession store-backed ---

func TestForkSessionViaStore_UpToMessageID(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "550e8400-e29b-41d4-a716-446655440000"}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Extra: map[string]any{"message": "hello"}},
		{Type: "assistant", UUID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", Extra: map[string]any{"response": "hi"}},
		{Type: "user", UUID: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13", Extra: map[string]any{"message": "bye"}},
	}
	store.Append(key, entries)
	result, err := ForkSessionViaStore(store, key, "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	forkKey := SessionKey{ProjectKey: "proj", SessionID: result.SessionID}
	forked, _ := store.Load(forkKey)
	if len(forked) != 2 {
		t.Errorf("expected 2 entries, got %d", len(forked))
	}
}

func TestForkSessionViaStore_FiltersProgress(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "550e8400-e29b-41d4-a716-446655440001"}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Extra: map[string]any{"message": "hello"}},
		{Type: "progress", UUID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", Extra: map[string]any{"parentUuid": "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"}},
		{Type: "assistant", UUID: "c0eebc99-9c0b-4ef8-bb6d-6bb9bd380a13", Extra: map[string]any{"parentUuid": "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", "response": "hi"}},
	}
	store.Append(key, entries)
	result, err := ForkSessionViaStore(store, key, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	forkKey := SessionKey{ProjectKey: "proj", SessionID: result.SessionID}
	forked, _ := store.Load(forkKey)
	if len(forked) != 3 {
		t.Errorf("expected 3 entries, got %d", len(forked))
	}
	for _, e := range forked {
		if e.Type == "progress" {
			t.Error("progress entries should be filtered from fork output")
		}
	}
}

func TestForkSessionViaStore_FiltersSidechain(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "550e8400-e29b-41d4-a716-446655440002"}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Extra: map[string]any{"message": "hello"}},
		{Type: "assistant", UUID: "b0eebc99-9c0b-4ef8-bb6d-6bb9bd380a12", Extra: map[string]any{"isSidechain": true, "response": "hi"}},
	}
	store.Append(key, entries)
	result, err := ForkSessionViaStore(store, key, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	forkKey := SessionKey{ProjectKey: "proj", SessionID: result.SessionID}
	forked, _ := store.Load(forkKey)
	if len(forked) != 2 {
		t.Errorf("expected 2 entries, got %d", len(forked))
	}
}

func TestForkSessionViaStore_ClearsStaleFields(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "550e8400-e29b-41d4-a716-446655440003"}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Extra: map[string]any{
			"message":                 "hello",
			"teamName":                "old-team",
			"agentName":               "old-agent",
			"slug":                    "old-slug",
			"sourceToolAssistantUUID": "old-uuid",
		}},
	}
	store.Append(key, entries)
	result, err := ForkSessionViaStore(store, key, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	forkKey := SessionKey{ProjectKey: "proj", SessionID: result.SessionID}
	forked, _ := store.Load(forkKey)
	for _, e := range forked {
		if e.Extra == nil {
			continue
		}
		for _, k := range []string{"teamName", "agentName", "slug", "sourceToolAssistantUUID"} {
			if _, ok := e.Extra[k]; ok {
				t.Errorf("field %q should be cleared in fork", k)
			}
		}
	}
}

func TestForkSessionViaStore_ContentReplacement(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "550e8400-e29b-41d4-a716-446655440004"}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Extra: map[string]any{"message": "hello"}},
		{Type: "content-replacement", Extra: map[string]any{
			"sessionId":    "550e8400-e29b-41d4-a716-446655440004",
			"replacements": []any{map[string]any{"old": "new"}},
		}},
	}
	store.Append(key, entries)
	result, err := ForkSessionViaStore(store, key, "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	forkKey := SessionKey{ProjectKey: "proj", SessionID: result.SessionID}
	forked, _ := store.Load(forkKey)
	if len(forked) != 3 {
		t.Errorf("expected 3 entries, got %d", len(forked))
	}
	found := false
	for _, e := range forked {
		if e.Type == "content-replacement" {
			found = true
			if e.UUID == "" {
				t.Error("content-replacement should have uuid")
			}
			if e.Timestamp == "" {
				t.Error("content-replacement should have timestamp")
			}
		}
	}
	if !found {
		t.Error("content-replacement entry should be in fork output")
	}
}

func TestForkSessionViaStore_CustomTitleHasUUID(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "550e8400-e29b-41d4-a716-446655440005"}
	entries := []SessionStoreEntry{
		{Type: "user", UUID: "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11", Extra: map[string]any{"message": "hello"}},
	}
	store.Append(key, entries)
	result, err := ForkSessionViaStore(store, key, "", "My Fork")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	forkKey := SessionKey{ProjectKey: "proj", SessionID: result.SessionID}
	forked, _ := store.Load(forkKey)
	for _, e := range forked {
		if e.Type == "custom-title" {
			if e.UUID == "" {
				t.Error("custom-title should have uuid")
			}
			if e.Timestamp == "" {
				t.Error("custom-title should have timestamp")
			}
			if e.Extra["customTitle"] != "My Fork" {
				t.Errorf("expected 'My Fork', got %v", e.Extra["customTitle"])
			}
		}
	}
}

// --- ForkSession filesystem ---

func TestForkSession_ClearsStaleFields(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	projectDir := filepath.Join(tmpDir, "projects", "-tmp-test")
	os.MkdirAll(projectDir, 0o755)

	sid := "550e8400-e29b-41d4-a716-446655440010"
	entry := map[string]any{
		"type": "user", "uuid": "550e8400-e29b-41d4-a716-446655440011",
		"sessionId": sid, "timestamp": "2024-01-01T00:00:00Z",
		"teamName": "old-team", "agentName": "old-agent", "slug": "old-slug", "sourceToolAssistantUUID": "old-uuid",
		"message": map[string]any{"role": "user", "content": "hello"},
	}
	b, _ := json.Marshal(entry)
	os.WriteFile(filepath.Join(projectDir, sid+".jsonl"), append(b, '\n'), 0o644)

	result, err := ForkSession(sid, "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(projectDir, result.SessionID+".jsonl"))
	var forked map[string]any
	json.Unmarshal(bytesFirstLine(data), &forked)
	for _, key := range []string{"teamName", "agentName", "slug", "sourceToolAssistantUUID"} {
		if _, ok := forked[key]; ok {
			t.Errorf("field %q should be cleared in fork", key)
		}
	}
}

func TestForkSession_ContentReplacementHasUUID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	projectDir := filepath.Join(tmpDir, "projects", "-tmp-test")
	os.MkdirAll(projectDir, 0o755)

	sid := "550e8400-e29b-41d4-a716-446655440012"
	lines := []string{
		fmt.Sprintf(`{"type":"user","uuid":"550e8400-e29b-41d4-a716-446655440013","sessionId":%q,"timestamp":"2024-01-01T00:00:00Z","message":{"role":"user","content":"hello"}}`, sid),
		fmt.Sprintf(`{"type":"content-replacement","sessionId":%q,"replacements":[{"old":"new"}]}`, sid),
	}
	os.WriteFile(filepath.Join(projectDir, sid+".jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0o644)

	result, err := ForkSession(sid, "", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(projectDir, result.SessionID+".jsonl"))
	found := false
	for _, line := range splitLinesFS(string(data)) {
		var entry map[string]any
		json.Unmarshal([]byte(line), &entry)
		if entry["type"] == "content-replacement" {
			found = true
			if entry["uuid"] == nil || entry["uuid"] == "" {
				t.Error("content-replacement should have uuid")
			}
			if entry["timestamp"] == nil || entry["timestamp"] == "" {
				t.Error("content-replacement should have timestamp")
			}
		}
	}
	if !found {
		t.Error("content-replacement entry should be in fork output")
	}
}

func splitLinesFS(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func bytesFirstLine(data []byte) []byte {
	for i, b := range data {
		if b == '\n' {
			return data[:i]
		}
	}
	return data
}
