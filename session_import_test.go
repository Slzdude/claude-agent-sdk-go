package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestImportSessionToStore_MainTranscript(t *testing.T) {
	projectDir, sessionID, _ := setupSessionFile(t, []map[string]any{
		{"type": "user", "uuid": "u1", "message": "hello"},
		{"type": "assistant", "uuid": "a1", "response": "hi"},
	})

	store := NewInMemorySessionStore()
	err := ImportSessionToStore(store, sessionID, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectKey := filepath.Base(projectDir)
	key := SessionKey{ProjectKey: projectKey, SessionID: sessionID}
	entries, _ := store.Load(key)
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

func TestImportSessionToStore_Batching(t *testing.T) {
	lines := make([]map[string]any, 600)
	for i := range lines {
		lines[i] = map[string]any{"type": "user", "uuid": fmt.Sprintf("u%03d", i), "n": i}
	}
	projectDir, sessionID, _ := setupSessionFile(t, lines)

	store := NewInMemorySessionStore()
	err := ImportSessionToStore(store, sessionID, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectKey := filepath.Base(projectDir)
	key := SessionKey{ProjectKey: projectKey, SessionID: sessionID}
	entries, _ := store.Load(key)
	if len(entries) != 600 {
		t.Errorf("expected 600 entries, got %d", len(entries))
	}
}

func TestImportSessionToStore_WithSubagents(t *testing.T) {
	projectDir, sessionID, _ := setupSessionFile(t, []map[string]any{
		{"type": "user", "uuid": "u1", "message": "hello"},
	})

	// Create subagent transcript.
	subDir := filepath.Join(projectDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0o700)
	os.WriteFile(filepath.Join(subDir, "agent-abc.jsonl"), []byte(`{"type":"user","uuid":"su1","message":"sub hello"}`), 0o600)
	os.WriteFile(filepath.Join(subDir, "agent-abc.meta.json"), []byte(`{"agentType":"test","worktreePath":"/tmp/wt"}`), 0o600)

	store := NewInMemorySessionStore()
	// Pass "" as directory so findSessionFileWithDir searches all projects.
	err := ImportSessionToStore(store, sessionID, "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectKey := filepath.Base(projectDir)
	mainEntries, _ := store.Load(SessionKey{ProjectKey: projectKey, SessionID: sessionID})
	if len(mainEntries) != 1 {
		t.Errorf("expected 1 main entry, got %d", len(mainEntries))
	}
	subEntries, _ := store.Load(SessionKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
		Subpath:    "subagents/abc",
	})
	// Should have transcript entry + agent_metadata entry from .meta.json sidecar.
	if len(subEntries) != 2 {
		t.Errorf("expected 2 subagent entries (transcript + metadata), got %d", len(subEntries))
	}
}

func TestImportSessionToStore_IncludeSubagentsFalse(t *testing.T) {
	projectDir, sessionID, _ := setupSessionFile(t, []map[string]any{
		{"type": "user", "uuid": "u1", "message": "hello"},
	})
	subDir := filepath.Join(projectDir, sessionID, "subagents")
	os.MkdirAll(subDir, 0o700)
	os.WriteFile(filepath.Join(subDir, "agent-abc.jsonl"), []byte(`{"type":"user","uuid":"su1"}`), 0o600)

	store := NewInMemorySessionStore()
	err := ImportSessionToStore(store, sessionID, "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectKey := filepath.Base(projectDir)
	subEntries, _ := store.Load(SessionKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
		Subpath:    "subagents/abc",
	})
	if subEntries != nil {
		t.Error("subagent entries should not be imported when includeSubagents=false")
	}
}

func TestImportSessionToStore_NestedSubagent(t *testing.T) {
	projectDir, sessionID, _ := setupSessionFile(t, []map[string]any{
		{"type": "user", "uuid": "u1", "message": "hello"},
	})
	nestedDir := filepath.Join(projectDir, sessionID, "subagents", "workflows", "run-1")
	os.MkdirAll(nestedDir, 0o700)
	os.WriteFile(filepath.Join(nestedDir, "agent-def.jsonl"), []byte(`{"type":"user","uuid":"su2"}`), 0o600)

	store := NewInMemorySessionStore()
	err := ImportSessionToStore(store, sessionID, "", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	projectKey := filepath.Base(projectDir)
	subEntries, _ := store.Load(SessionKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
		Subpath:    "subagents/workflows/run-1/def",
	})
	if len(subEntries) != 1 {
		t.Errorf("expected 1 nested subagent entry, got %d", len(subEntries))
	}
}

func TestImportSessionToStore_InvalidUUID(t *testing.T) {
	store := NewInMemorySessionStore()
	err := ImportSessionToStore(store, "not-a-uuid", "", false)
	if err == nil {
		t.Error("expected error for invalid UUID")
	}
}
