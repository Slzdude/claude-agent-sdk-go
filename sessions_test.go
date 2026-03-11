package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── sanitizePath ───────────────────────────────────────────────────────────

func TestSanitizePath_Short(t *testing.T) {
	got := sanitizePath("/home/user/myproject")
	// All non-alphanumeric characters replaced with '-'
	want := "-home-user-myproject"
	if got != want {
		t.Errorf("sanitizePath = %q, want %q", got, want)
	}
}

func TestSanitizePath_ShortAllAlpha(t *testing.T) {
	got := sanitizePath("myproject")
	if got != "myproject" {
		t.Errorf("sanitizePath = %q, want %q", got, "myproject")
	}
}

func TestSanitizePath_Long(t *testing.T) {
	// Build a path that will produce a sanitized string longer than 200 chars
	longPath := "/" + strings.Repeat("a", 250)
	got := sanitizePath(longPath)
	// Should be truncated to 200 chars + "-" + hash
	if len(got) <= maxSanitizedLen {
		t.Errorf("expected long result (>%d chars), got len=%d: %q", maxSanitizedLen, len(got), got)
	}
	parts := strings.SplitN(got[(maxSanitizedLen):], "-", 2)
	if len(parts) < 2 {
		t.Errorf("expected 'prefix-hash' format, got %q", got)
	}
}

// ─── simpleHash ─────────────────────────────────────────────────────────────

func TestSimpleHash_Empty(t *testing.T) {
	// An empty string produces hash of 0 → "0"
	got := simpleHash("")
	if got != "0" {
		t.Errorf("simpleHash(\"\") = %q, want %q", got, "0")
	}
}

func TestSimpleHash_Deterministic(t *testing.T) {
	// Same input always yields same output
	h1 := simpleHash("/home/user/project")
	h2 := simpleHash("/home/user/project")
	if h1 != h2 {
		t.Errorf("simpleHash not deterministic: %q != %q", h1, h2)
	}
}

func TestSimpleHash_Distinct(t *testing.T) {
	// Different inputs should produce different hashes
	if simpleHash("abc") == simpleHash("xyz") {
		t.Error("simpleHash collision between 'abc' and 'xyz'")
	}
}

// ─── GetSessionMessages — message-level filters ──────────────────────────────

// setupSessionFile creates a temporary CLAUDE_CONFIG_DIR and writes JSONL test
// data. It returns (projectDir, sessionID, teardown).
func setupSessionFile(t *testing.T, lines []map[string]any) (projectDir string, sessionID string, cleanup func()) {
	t.Helper()
	tmpDir := t.TempDir()
	// Resolve symlinks so the sanitized project dir name matches what
	// readSessionFileContent derives when it calls filepath.EvalSymlinks on
	// the directory argument. On macOS, t.TempDir() returns /tmp/... which
	// is a symlink to /private/tmp/..., causing a mismatch without this step.
	realTmpDir := tmpDir
	if resolved, err := filepath.EvalSymlinks(tmpDir); err == nil {
		realTmpDir = resolved
	}
	sessionID = "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"
	projectDir = filepath.Join(tmpDir, "projects", sanitizePath(realTmpDir))
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(projectDir, sessionID+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	for _, line := range lines {
		b, _ := json.Marshal(line)
		_, _ = fmt.Fprintln(f, string(b))
	}

	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	return projectDir, sessionID, func() {}
}

func TestGetSessionMessages_IsMetaFilter(t *testing.T) {
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "session_id": "s1", "isMeta": true, "message": map[string]any{}},
		{"type": "assistant", "uuid": "u2", "session_id": "s1", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	// Only the assistant message (u2) should pass; the isMeta user message is skipped.
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (non-meta), got %d", len(msgs))
	}
	if msgs[0].UUID != "u2" {
		t.Errorf("expected uuid=u2, got %q", msgs[0].UUID)
	}
}

func TestGetSessionMessages_TeamNameFilter(t *testing.T) {
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "session_id": "s1", "teamName": "acme", "message": map[string]any{}},
		{"type": "assistant", "uuid": "u2", "session_id": "s1", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (non-team), got %d", len(msgs))
	}
	if msgs[0].UUID != "u2" {
		t.Errorf("expected uuid=u2, got %q", msgs[0].UUID)
	}
}

func TestGetSessionMessages_SidechainFilter(t *testing.T) {
	lines := []map[string]any{
		// isSidechain:true is the correct field (matches Python SDK _is_visible_message).
		{"type": "user", "uuid": "u1", "session_id": "s1", "isSidechain": true, "message": map[string]any{}},
		{"type": "assistant", "uuid": "u2", "session_id": "s1", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	// Sidechain message u1 should be filtered out.
	if len(msgs) != 1 || msgs[0].UUID != "u2" {
		t.Fatalf("expected 1 non-sidechain message, got %v", msgs)
	}
}

func TestGetSessionMessages_LimitOffset(t *testing.T) {
	// Use parentUuid chain to form a real 4-message conversation thread.
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "session_id": "s1", "message": map[string]any{}},
		{"type": "assistant", "uuid": "u2", "parentUuid": "u1", "session_id": "s1", "message": map[string]any{}},
		{"type": "user", "uuid": "u3", "parentUuid": "u2", "session_id": "s1", "message": map[string]any{}},
		{"type": "assistant", "uuid": "u4", "parentUuid": "u3", "session_id": "s1", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Fatalf("baseline: expected 4 messages, got %d", len(msgs))
	}

	// Apply limit=2, offset=1 manually by calling GetSessionMessages with the
	// correct project directory derived from CLAUDE_CONFIG_DIR.
	tmpDir := os.Getenv("CLAUDE_CONFIG_DIR")
	msgs2, err := GetSessionMessages(sid, tmpDir, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs2) != 2 {
		t.Fatalf("limit=2 offset=1: expected 2 messages, got %d", len(msgs2))
	}
	if msgs2[0].UUID != "u2" || msgs2[1].UUID != "u3" {
		t.Errorf("unexpected UUIDs: %v %v", msgs2[0].UUID, msgs2[1].UUID)
	}
}

func TestGetSessionMessages_OnlyReturnsUserAssistant(t *testing.T) {
	// Chain: user → system → assistant (result type is not a transcript entry type and is dropped at parse).
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "session_id": "s1", "message": map[string]any{}},
		{"type": "system", "uuid": "s0", "parentUuid": "u1", "session_id": "s1", "message": map[string]any{}},
		{"type": "result", "uuid": "r0", "parentUuid": "s0", "session_id": "s1", "message": map[string]any{}},
		{"type": "assistant", "uuid": "u2", "parentUuid": "s0", "session_id": "s1", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected only user+assistant messages (2), got %d", len(msgs))
	}
}

// ─── ListSessions ─────────────────────────────────────────────────────────────

func TestListSessions_LimitApplied(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	// Create two separate project dirs, each with one session.
	for i := 0; i < 3; i++ {
		projDir := filepath.Join(tmpDir, "projects", fmt.Sprintf("proj%d", i))
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
		sid := fmt.Sprintf("aaaaaaaa-0000-0000-0000-00000000000%d", i)
		f, err := os.Create(filepath.Join(projDir, sid+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		_ = f.Close()
	}

	all, err := ListSessions(tmpDir, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	// All discovered sessions
	total := len(all)
	if total == 0 {
		t.Skip("no sessions discovered — directory layout may differ")
	}

	limited, err := ListSessions(tmpDir, false, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) > 1 {
		t.Errorf("limit=1 returned %d sessions", len(limited))
	}
}

// ─── helper ──────────────────────────────────────────────────────────────────

// getSessionMsgsFromEnv calls GetSessionMessages using CLAUDE_CONFIG_DIR as the directory.
func getSessionMsgsFromEnv(t *testing.T, sessionID string) ([]SessionMessage, error) {
	t.Helper()
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		t.Fatal("CLAUDE_CONFIG_DIR not set")
	}
	return GetSessionMessages(sessionID, dir, 0, 0)
}

// ─── ListAllSessions ─────────────────────────────────────────────────────────

func TestListAllSessions_ReturnsSessionsFromAllProjects(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	// Create sessions in two different project directories, each with minimal content.
	for i := 0; i < 2; i++ {
		projDir := filepath.Join(tmpDir, "projects", fmt.Sprintf("proj%d", i))
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
		sid := fmt.Sprintf("aaaaaaaa-0000-0000-0000-%012d", i)
		f, err := os.Create(filepath.Join(projDir, sid+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(map[string]any{
			"type": "user", "uuid": sid,
			"message": map[string]any{"role": "user", "content": "test prompt"},
		})
		_, _ = fmt.Fprintln(f, string(b))
		_ = f.Close()
	}

	all, err := ListAllSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) < 2 {
		t.Errorf("expected at least 2 sessions, got %d", len(all))
	}
}

func TestListAllSessions_LimitApplied(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	for i := 0; i < 3; i++ {
		projDir := filepath.Join(tmpDir, "projects", fmt.Sprintf("projA%d", i))
		if err := os.MkdirAll(projDir, 0o755); err != nil {
			t.Fatal(err)
		}
		sid := fmt.Sprintf("bbbbbbbb-0000-0000-0000-%012d", i)
		f, _ := os.Create(filepath.Join(projDir, sid+".jsonl"))
		b, _ := json.Marshal(map[string]any{
			"type": "user", "uuid": sid,
			"message": map[string]any{"role": "user", "content": "test"},
		})
		_, _ = fmt.Fprintln(f, string(b))
		_ = f.Close()
	}

	limited, err := ListAllSessions(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) > 1 {
		t.Errorf("limit=1 should return at most 1 session, got %d", len(limited))
	}
}

func TestListAllSessions_EmptyProjectsDir(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)
	// No projects directory at all.
	all, err := ListAllSessions(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Errorf("expected empty list, got %d sessions", len(all))
	}
}

// ─── GetSessionMessages — empty-directory (searches all projects) ─────────────

func TestGetSessionMessages_EmptyDirectory_FindsSession(t *testing.T) {
	// Two messages in a parentUuid chain to verify chain reconstruction.
	lines := []map[string]any{
		{"type": "user", "uuid": "u1", "session_id": "s1", "message": map[string]any{}},
		{"type": "assistant", "uuid": "u2", "parentUuid": "u1", "session_id": "s1", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)

	// Call with empty directory — should scan all projects.
	msgs, err := GetSessionMessages(sid, "", 0, 0)
	if err != nil {
		t.Fatalf("GetSessionMessages with empty directory: %v", err)
	}
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages, got %d", len(msgs))
	}
}

func TestGetSessionMessages_EmptyDirectory_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	// Python SDK returns empty list (not error) when session is not found.
	msgs, err := GetSessionMessages("00000000-0000-0000-0000-000000000000", "", 0, 0)
	if err != nil {
		t.Errorf("expected nil error for missing session, got: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty list for missing session, got %d messages", len(msgs))
	}
}

// ─── parentUuid chain reconstruction ─────────────────────────────────────────

// TestGetSessionMessages_ParentUuidChain verifies that the chain algorithm
// returns only the main conversation thread via parentUuid links.
func TestGetSessionMessages_ParentUuidChain(t *testing.T) {
	// Three-turn conversation with proper parentUuid links.
	lines := []map[string]any{
		{"type": "user", "uuid": "a", "message": map[string]any{"content": "hello"}},
		{"type": "assistant", "uuid": "b", "parentUuid": "a", "message": map[string]any{}},
		{"type": "user", "uuid": "c", "parentUuid": "b", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages in chain, got %d", len(msgs))
	}
	if msgs[0].UUID != "a" || msgs[1].UUID != "b" || msgs[2].UUID != "c" {
		t.Errorf("unexpected chain order: %v %v %v", msgs[0].UUID, msgs[1].UUID, msgs[2].UUID)
	}
}

// TestGetSessionMessages_ParentUuidPicksBestLeaf verifies that when multiple
// branches exist, the highest-indexed leaf (main thread) is selected.
func TestGetSessionMessages_ParentUuidPicksBestLeaf(t *testing.T) {
	// Two branches off the same root:
	//   root (user:r) → branch_a (assistant:a) [first branch]
	//   root (user:r) → branch_b (assistant:b) [later branch — should be picked]
	lines := []map[string]any{
		{"type": "user", "uuid": "r", "message": map[string]any{}},
		{"type": "assistant", "uuid": "a", "parentUuid": "r", "message": map[string]any{}},
		{"type": "assistant", "uuid": "b", "parentUuid": "r", "message": map[string]any{}},
	}
	_, sid, _ := setupSessionFile(t, lines)
	msgs, err := getSessionMsgsFromEnv(t, sid)
	if err != nil {
		t.Fatal(err)
	}
	// Should pick branch_b (latest in file) → chain [r, b]
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[1].UUID != "b" {
		t.Errorf("expected best leaf to be 'b', got %q", msgs[1].UUID)
	}
}

// TestGetSessionMessages_InvalidUUID mirrors Python's _validate_uuid check:
// an invalid session ID returns an empty list without error.
func TestGetSessionMessages_InvalidUUID(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", tmpDir)

	msgs, err := GetSessionMessages("not-a-uuid", tmpDir, 0, 0)
	if err != nil {
		t.Errorf("expected nil error for invalid UUID, got: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected empty list for invalid UUID, got %d messages", len(msgs))
	}
}
