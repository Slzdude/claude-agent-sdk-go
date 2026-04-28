package claude

import (
	"sort"
	"testing"
)

// RunSessionStoreConformance asserts the 14 behavioral contracts every
// SessionStore adapter must satisfy. Call this from your own tests:
//
//	func TestMyStore(t *testing.T) {
//	    store := NewMyStore()
//	    claude.RunSessionStoreConformance(t, func() claude.SessionStore { return store })
//	}
//
// Contracts 1-6 are required (append + load + isolation).
// Contracts 7-14 cover optional methods (list_sessions, delete, list_subkeys,
// list_session_summaries). All methods are required in Go's interface model,
// but implementations may return empty results for optional functionality.
func RunSessionStoreConformance(t *testing.T, makeStore func() SessionStore) {
	t.Helper()

	// Helper to create a test entry.
	e := func(extra map[string]any) SessionStoreEntry {
		entry := SessionStoreEntry{Type: "x"}
		if extra != nil {
			entry.Extra = extra
		}
		return entry
	}

	key := SessionKey{ProjectKey: "proj", SessionID: "sess"}

	// --- Required: append + load ---

	// 1. append then load returns same entries in same order
	t.Run("1_append_load_order", func(t *testing.T) {
		store := makeStore()
		entries := []SessionStoreEntry{
			e(map[string]any{"uuid": "b", "n": 1}),
			e(map[string]any{"uuid": "a", "n": 2}),
		}
		if err := store.Append(key, entries); err != nil {
			t.Fatalf("append: %v", err)
		}
		loaded, err := store.Load(key)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if len(loaded) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(loaded))
		}
		if loaded[0].Extra["uuid"] != "b" || loaded[1].Extra["uuid"] != "a" {
			t.Errorf("order mismatch: got %v", loaded)
		}
	})

	// 2. load unknown key returns nil
	t.Run("2_load_unknown", func(t *testing.T) {
		store := makeStore()
		loaded, err := store.Load(SessionKey{ProjectKey: "proj", SessionID: "nope"})
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if loaded != nil {
			t.Errorf("expected nil, got %v", loaded)
		}
		// Also check subpath.
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"uuid": "x", "n": 1})})
		loaded, err = store.Load(SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "nope"})
		if err != nil {
			t.Fatalf("load subpath: %v", err)
		}
		if loaded != nil {
			t.Errorf("expected nil for subpath, got %v", loaded)
		}
	})

	// 3. multiple append calls preserve call order
	t.Run("3_append_order", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"uuid": "z", "n": 1})})
		_ = store.Append(key, []SessionStoreEntry{
			e(map[string]any{"uuid": "a", "n": 2}),
			e(map[string]any{"uuid": "m", "n": 3}),
		})
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"uuid": "b", "n": 4})})
		loaded, err := store.Load(key)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		expected := []string{"z", "a", "m", "b"}
		if len(loaded) != len(expected) {
			t.Fatalf("expected %d entries, got %d", len(expected), len(loaded))
		}
		for i, exp := range expected {
			if loaded[i].Extra["uuid"] != exp {
				t.Errorf("entry %d: expected uuid=%s, got %v", i, exp, loaded[i].Extra["uuid"])
			}
		}
	})

	// 4. append([]) is a no-op
	t.Run("4_append_empty", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"uuid": "a", "n": 1})})
		_ = store.Append(key, []SessionStoreEntry{})
		loaded, err := store.Load(key)
		if err != nil {
			t.Fatalf("load: %v", err)
		}
		if len(loaded) != 1 {
			t.Errorf("expected 1 entry, got %d", len(loaded))
		}
	})

	// 5. subpath keys are stored independently of main
	t.Run("5_subpath_independent", func(t *testing.T) {
		store := makeStore()
		sub := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-1"}
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"uuid": "m", "n": 1})})
		_ = store.Append(sub, []SessionStoreEntry{e(map[string]any{"uuid": "s", "n": 1})})
		main, _ := store.Load(key)
		subLoad, _ := store.Load(sub)
		if len(main) != 1 || main[0].Extra["uuid"] != "m" {
			t.Errorf("main mismatch: %v", main)
		}
		if len(subLoad) != 1 || subLoad[0].Extra["uuid"] != "s" {
			t.Errorf("sub mismatch: %v", subLoad)
		}
	})

	// 6. project_key isolation
	t.Run("6_project_key_isolation", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(SessionKey{ProjectKey: "A", SessionID: "s1"}, []SessionStoreEntry{e(map[string]any{"from": "A"})})
		_ = store.Append(SessionKey{ProjectKey: "B", SessionID: "s1"}, []SessionStoreEntry{e(map[string]any{"from": "B"})})
		a, _ := store.Load(SessionKey{ProjectKey: "A", SessionID: "s1"})
		b, _ := store.Load(SessionKey{ProjectKey: "B", SessionID: "s1"})
		if len(a) != 1 || a[0].Extra["from"] != "A" {
			t.Errorf("project A mismatch: %v", a)
		}
		if len(b) != 1 || b[0].Extra["from"] != "B" {
			t.Errorf("project B mismatch: %v", b)
		}
		listA, _ := store.ListSessions("A")
		listB, _ := store.ListSessions("B")
		if len(listA) != 1 {
			t.Errorf("list A: expected 1, got %d", len(listA))
		}
		if len(listB) != 1 {
			t.Errorf("list B: expected 1, got %d", len(listB))
		}
	})

	// --- Optional: list_sessions ---

	// 7. list_sessions returns session_ids for project
	t.Run("7_list_sessions", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "a"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "b"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(SessionKey{ProjectKey: "other", SessionID: "c"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		sessions, err := store.ListSessions("proj")
		if err != nil {
			t.Fatalf("list_sessions: %v", err)
		}
		var ids []string
		for _, s := range sessions {
			ids = append(ids, s.SessionID)
		}
		sort.Strings(ids)
		if ids[0] != "a" || ids[1] != "b" {
			t.Errorf("expected [a b], got %v", ids)
		}
		// mtime must be epoch-ms; >1e12 rules out epoch-seconds.
		for _, s := range sessions {
			if s.Mtime <= 1e12 {
				t.Errorf("mtime %d looks like epoch-seconds, not milliseconds", s.Mtime)
			}
		}
		// Empty project returns empty.
		empty, _ := store.ListSessions("never-appended-project")
		if len(empty) != 0 {
			t.Errorf("expected empty, got %v", empty)
		}
	})

	// 8. list_sessions excludes subagent subpaths
	t.Run("8_list_sessions_excludes_subpaths", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "main"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "main", Subpath: "subagents/agent-1"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		sessions, _ := store.ListSessions("proj")
		if len(sessions) != 1 || sessions[0].SessionID != "main" {
			t.Errorf("expected [main], got %v", sessions)
		}
	})

	// --- Optional: delete ---

	// 9. delete main then load returns nil
	t.Run("9_delete_main", func(t *testing.T) {
		store := makeStore()
		_ = store.Delete(SessionKey{ProjectKey: "proj", SessionID: "never-written"}) // no-op
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"n": 1})})
		if err := store.Delete(key); err != nil {
			t.Fatalf("delete: %v", err)
		}
		loaded, _ := store.Load(key)
		if loaded != nil {
			t.Errorf("expected nil after delete, got %v", loaded)
		}
	})

	// 10. delete main cascades to subkeys
	t.Run("10_delete_cascades_subkeys", func(t *testing.T) {
		store := makeStore()
		sub1 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-1"}
		sub2 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-2"}
		other := SessionKey{ProjectKey: "proj", SessionID: "sess2"}
		otherProj := SessionKey{ProjectKey: "other-proj", SessionID: "sess"}
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(sub1, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(sub2, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(other, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(otherProj, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Delete(key)
		if loaded, _ := store.Load(key); loaded != nil {
			t.Errorf("main should be nil")
		}
		if loaded, _ := store.Load(sub1); loaded != nil {
			t.Errorf("sub1 should be nil")
		}
		if loaded, _ := store.Load(sub2); loaded != nil {
			t.Errorf("sub2 should be nil")
		}
		if loaded, _ := store.Load(other); loaded == nil || len(loaded) != 1 {
			t.Errorf("other should still exist")
		}
		if loaded, _ := store.Load(otherProj); loaded == nil || len(loaded) != 1 {
			t.Errorf("otherProj should still exist")
		}
	})

	// 11. delete with subpath removes only that subkey
	t.Run("11_delete_subpath_only", func(t *testing.T) {
		store := makeStore()
		sub1 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-1"}
		sub2 := SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-2"}
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(sub1, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(sub2, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Delete(sub1)
		if loaded, _ := store.Load(sub1); loaded != nil {
			t.Errorf("sub1 should be nil")
		}
		if loaded, _ := store.Load(sub2); loaded == nil || len(loaded) != 1 {
			t.Errorf("sub2 should still exist")
		}
		if loaded, _ := store.Load(key); loaded == nil || len(loaded) != 1 {
			t.Errorf("main should still exist")
		}
	})

	// --- Optional: list_subkeys ---

	// 12. list_subkeys returns subpaths
	t.Run("12_list_subkeys", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-1"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "sess", Subpath: "subagents/agent-2"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		_ = store.Append(SessionKey{ProjectKey: "proj", SessionID: "other-sess", Subpath: "subagents/agent-x"}, []SessionStoreEntry{e(map[string]any{"n": 1})})
		subkeys, err := store.ListSubkeys("proj", "sess")
		if err != nil {
			t.Fatalf("list_subkeys: %v", err)
		}
		sort.Strings(subkeys)
		expected := []string{"subagents/agent-1", "subagents/agent-2"}
		if len(subkeys) != len(expected) {
			t.Fatalf("expected %v, got %v", expected, subkeys)
		}
		for i, exp := range expected {
			if subkeys[i] != exp {
				t.Errorf("subkey %d: expected %s, got %s", i, exp, subkeys[i])
			}
		}
	})

	// 13. list_subkeys excludes main transcript
	t.Run("13_list_subkeys_excludes_main", func(t *testing.T) {
		store := makeStore()
		_ = store.Append(key, []SessionStoreEntry{e(map[string]any{"n": 1})})
		subkeys, _ := store.ListSubkeys("proj", "sess")
		if len(subkeys) != 0 {
			t.Errorf("expected empty, got %v", subkeys)
		}
		subkeys, _ = store.ListSubkeys("proj", "never-appended")
		if len(subkeys) != 0 {
			t.Errorf("expected empty for never-appended, got %v", subkeys)
		}
	})

	// --- Optional: list_session_summaries ---

	// 14. list_session_summaries returns persisted fold output
	t.Run("14_list_session_summaries", func(t *testing.T) {
		store := makeStore()
		summKey := SessionKey{ProjectKey: "proj", SessionID: "summ-sess"}
		_ = store.Append(summKey, []SessionStoreEntry{
			SessionStoreEntry{Type: "x", Timestamp: "2024-01-01T00:00:00.000Z", Extra: map[string]any{"customTitle": "first"}},
			SessionStoreEntry{Type: "x", Timestamp: "2024-01-01T00:00:01.000Z"},
		})
		_ = store.Append(summKey, []SessionStoreEntry{
			SessionStoreEntry{Type: "x", Timestamp: "2024-01-01T00:00:02.000Z", Extra: map[string]any{"customTitle": "second"}},
		})
		_ = store.Append(SessionKey{ProjectKey: "other", SessionID: "elsewhere"}, []SessionStoreEntry{
			SessionStoreEntry{Type: "x", Timestamp: "2024-01-01T00:00:00.000Z"},
		})
		summaries, err := store.ListSessionSummaries("proj")
		if err != nil {
			t.Fatalf("list_session_summaries: %v", err)
		}
		byID := make(map[string]SessionSummaryEntry)
		for _, s := range summaries {
			byID[s.SessionID] = s
		}
		if _, ok := byID["summ-sess"]; !ok {
			t.Fatalf("expected summ-sess in summaries")
		}
		summ := byID["summ-sess"]
		if summ.Mtime <= 1e12 {
			t.Errorf("mtime %d looks like epoch-seconds", summ.Mtime)
		}
		if summ.Data == nil {
			t.Errorf("data should not be nil")
		}
		// Empty project returns empty.
		empty, _ := store.ListSessionSummaries("never-appended-project")
		if len(empty) != 0 {
			t.Errorf("expected empty, got %v", empty)
		}
	})
}

// TestInMemorySessionStore_MtimeMonotonic verifies that back-to-back Append
// calls always produce distinct, strictly-increasing mtimes — matching Python's
// InMemorySessionStore._next_mtime() contract.
func TestInMemorySessionStore_MtimeMonotonic(t *testing.T) {
	store := NewInMemorySessionStore()
	key := SessionKey{ProjectKey: "proj", SessionID: "sess-mono"}
	entry := SessionStoreEntry{Type: "user", UUID: "u1"}

	const rounds = 20
	var mtimes []int64
	for i := 0; i < rounds; i++ {
		if err := store.Append(key, []SessionStoreEntry{entry}); err != nil {
			t.Fatalf("append round %d: %v", i, err)
		}
		sessions, err := store.ListSessions("proj")
		if err != nil {
			t.Fatalf("list round %d: %v", i, err)
		}
		if len(sessions) == 0 {
			t.Fatalf("round %d: no sessions returned", i)
		}
		mtimes = append(mtimes, sessions[0].Mtime)
	}
	for i := 1; i < len(mtimes); i++ {
		if mtimes[i] <= mtimes[i-1] {
			t.Errorf("mtime not strictly increasing at round %d: prev=%d cur=%d",
				i, mtimes[i-1], mtimes[i])
		}
	}
}

// TestInMemorySessionStoreConformance runs the full conformance suite
// against the built-in InMemorySessionStore.
func TestInMemorySessionStoreConformance(t *testing.T) {
	RunSessionStoreConformance(t, func() SessionStore {
		return NewInMemorySessionStore()
	})
}
