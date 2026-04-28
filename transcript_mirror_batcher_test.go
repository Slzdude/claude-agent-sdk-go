package claude

import (
	"fmt"
	"testing"
)

func TestFilePathToSessionKey_MainTranscript(t *testing.T) {
	projectsDir := "/home/user/.claude/projects"
	filePath := "/home/user/.claude/projects/proj-abc/550e8400-e29b-41d4-a716-446655440000.jsonl"

	key := FilePathToSessionKey(filePath, projectsDir)
	if key == nil {
		t.Fatal("expected non-nil key")
	}
	if key.ProjectKey != "proj-abc" {
		t.Errorf("ProjectKey = %q, want %q", key.ProjectKey, "proj-abc")
	}
	if key.SessionID != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("SessionID = %q", key.SessionID)
	}
	if key.Subpath != "" {
		t.Errorf("Subpath should be empty, got %q", key.Subpath)
	}
}

func TestFilePathToSessionKey_SubagentTranscript(t *testing.T) {
	projectsDir := "/home/user/.claude/projects"
	filePath := "/home/user/.claude/projects/proj-abc/550e8400/subagents/agent-1.jsonl"

	key := FilePathToSessionKey(filePath, projectsDir)
	if key == nil {
		t.Fatal("expected non-nil key")
	}
	if key.ProjectKey != "proj-abc" {
		t.Errorf("ProjectKey = %q", key.ProjectKey)
	}
	if key.SessionID != "550e8400" {
		t.Errorf("SessionID = %q", key.SessionID)
	}
	if key.Subpath != "subagents/agent-1" {
		t.Errorf("Subpath = %q, want %q", key.Subpath, "subagents/agent-1")
	}
}

func TestFilePathToSessionKey_NestedSubagent(t *testing.T) {
	projectsDir := "/home/user/.claude/projects"
	filePath := "/home/user/.claude/projects/proj-abc/550e8400/subagents/workflows/run-1/agent-2.jsonl"

	key := FilePathToSessionKey(filePath, projectsDir)
	if key == nil {
		t.Fatal("expected non-nil key")
	}
	if key.Subpath != "subagents/workflows/run-1/agent-2" {
		t.Errorf("Subpath = %q", key.Subpath)
	}
}

func TestFilePathToSessionKey_OutsideProjectsDir(t *testing.T) {
	projectsDir := "/home/user/.claude/projects"
	filePath := "/tmp/other/session.jsonl"

	key := FilePathToSessionKey(filePath, projectsDir)
	if key != nil {
		t.Errorf("expected nil for path outside projects dir, got %v", key)
	}
}

func TestFilePathToSessionKey_TooShort(t *testing.T) {
	projectsDir := "/home/user/.claude/projects"
	filePath := "/home/user/.claude/projects/only-one-part.jsonl"

	key := FilePathToSessionKey(filePath, projectsDir)
	if key != nil {
		t.Errorf("expected nil for too-short path, got %v", key)
	}
}

func TestTranscriptMirrorBatcher_EnqueueAndFlush(t *testing.T) {
	store := NewInMemorySessionStore()
	var errors []string
	batcher := NewTranscriptMirrorBatcher(store, "/tmp/projects", func(key *SessionKey, errMsg string) {
		errors = append(errors, errMsg)
	})

	// Enqueue some entries.
	batcher.Enqueue("/tmp/projects/proj/sess.jsonl", []SessionStoreEntry{
		{Type: "user", UUID: "u1", Extra: map[string]any{"message": "hello"}},
	})
	batcher.Enqueue("/tmp/projects/proj/sess.jsonl", []SessionStoreEntry{
		{Type: "assistant", UUID: "a1", Extra: map[string]any{"response": "hi"}},
	})

	// Before flush, store should be empty.
	entries, _ := store.Load(SessionKey{ProjectKey: "proj", SessionID: "sess"})
	if len(entries) != 0 {
		t.Errorf("store should be empty before flush, got %d entries", len(entries))
	}

	// Flush.
	batcher.Flush()

	// After flush, store should have entries.
	entries, _ = store.Load(SessionKey{ProjectKey: "proj", SessionID: "sess"})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after flush, got %d", len(entries))
	}
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %v", errors)
	}
}

func TestTranscriptMirrorBatcher_CoalesceByPath(t *testing.T) {
	store := NewInMemorySessionStore()
	batcher := NewTranscriptMirrorBatcher(store, "/tmp/projects", func(key *SessionKey, errMsg string) {})

	// Enqueue to same path multiple times.
	for i := 0; i < 5; i++ {
		batcher.Enqueue("/tmp/projects/proj/sess.jsonl", []SessionStoreEntry{
			{Type: "user", UUID: fmt.Sprintf("u%d", i)},
		})
	}
	batcher.Flush()

	// Should have 5 entries coalesced into one append.
	entries, _ := store.Load(SessionKey{ProjectKey: "proj", SessionID: "sess"})
	if len(entries) != 5 {
		t.Errorf("expected 5 entries, got %d", len(entries))
	}
}

func TestTranscriptMirrorBatcher_ErrorReporting(t *testing.T) {
	store := NewInMemorySessionStore()
	var errors []string
	batcher := NewTranscriptMirrorBatcher(store, "/tmp/projects", func(key *SessionKey, errMsg string) {
		errors = append(errors, errMsg)
	})

	// Enqueue to a path that will produce a valid key but the store will handle fine.
	batcher.Enqueue("/tmp/projects/proj/sess.jsonl", []SessionStoreEntry{
		{Type: "user", UUID: "u1"},
	})
	batcher.Flush()

	// No errors expected for valid operations.
	if len(errors) != 0 {
		t.Errorf("expected no errors, got %v", errors)
	}
}

func TestTranscriptMirrorBatcher_InvalidPath(t *testing.T) {
	store := NewInMemorySessionStore()
	var errors []string
	batcher := NewTranscriptMirrorBatcher(store, "/tmp/projects", func(key *SessionKey, errMsg string) {
		errors = append(errors, errMsg)
	})

	// Enqueue to a path outside projects dir — should be dropped silently.
	batcher.Enqueue("/tmp/other/session.jsonl", []SessionStoreEntry{
		{Type: "user", UUID: "u1"},
	})
	batcher.Flush()

	// No entries and no errors (just a log warning).
	entries, _ := store.Load(SessionKey{ProjectKey: "proj", SessionID: "sess"})
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestTranscriptMirrorBatcher_Close(t *testing.T) {
	store := NewInMemorySessionStore()
	batcher := NewTranscriptMirrorBatcher(store, "/tmp/projects", func(key *SessionKey, errMsg string) {})

	batcher.Enqueue("/tmp/projects/proj/sess.jsonl", []SessionStoreEntry{
		{Type: "user", UUID: "u1"},
	})
	batcher.Close()

	entries, _ := store.Load(SessionKey{ProjectKey: "proj", SessionID: "sess"})
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after close, got %d", len(entries))
	}
}
