package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MaterializedResume holds the result of MaterializeResumeSession.
// It contains a temporary CLAUDE_CONFIG_DIR laid out like ~/.claude/ so the
// CLI subprocess can resume from a session that lives in an external store.
type MaterializedResume struct {
	// ConfigDir is the temporary directory path. Point the subprocess at it
	// via CLAUDE_CONFIG_DIR env var.
	ConfigDir string
	// ResumeSessionID is the session ID to pass as --resume. When the input
	// was ContinueConversation, this is the most-recent session resolved via
	// SessionStore.ListSessions.
	ResumeSessionID string
	// Cleanup removes ConfigDir (best-effort). Call it after the subprocess exits.
	Cleanup func()
}

// ApplyMaterializedOptions returns a copy of opts repointed at a materialized
// temp config dir. Sets CLAUDE_CONFIG_DIR in Env, Resume to the materialized
// session ID, and clears ContinueConversation (already resolved to a concrete
// session ID during materialization).
func ApplyMaterializedOptions(opts ClaudeAgentOptions, m *MaterializedResume) ClaudeAgentOptions {
	env := make(map[string]string, len(opts.Env)+1)
	for k, v := range opts.Env {
		env[k] = v
	}
	env["CLAUDE_CONFIG_DIR"] = m.ConfigDir
	opts.Env = env
	opts.Resume = m.ResumeSessionID
	opts.ContinueConversation = false
	return opts
}

// MaterializeResumeSession loads a session from opts.SessionStore and writes it
// to a temporary directory laid out like ~/.claude/. Returns nil when no
// materialization is needed (no store, no resume/continue, store has no entries,
// or the resolved session ID is not a valid UUID).
//
// The caller must call m.Cleanup() after the subprocess exits.
func MaterializeResumeSession(ctx context.Context, opts *ClaudeAgentOptions) (*MaterializedResume, error) {
	store := opts.SessionStore
	if store == nil {
		return nil, nil
	}
	if opts.Resume == "" && !opts.ContinueConversation {
		return nil, nil
	}

	timeout := time.Duration(opts.LoadTimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 60 * time.Second
	}

	projectKey := ProjectKeyForDirectory(opts.CWD)

	// Resolve the session ID.
	var resolved *candidateResult
	var err error

	if opts.Resume != "" {
		if !validateUUID(opts.Resume) {
			return nil, nil
		}
		resolved, err = loadCandidate(ctx, store, projectKey, opts.Resume, timeout)
	} else {
		resolved, err = resolveContinueCandidate(ctx, store, projectKey, timeout)
	}
	if err != nil {
		return nil, fmt.Errorf("session resume materialization failed: %w", err)
	}
	if resolved == nil {
		return nil, nil
	}

	tmpBase, err := os.MkdirTemp("", "claude-resume-")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}

	projectDir := filepath.Join(tmpBase, "projects", projectKey)
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		os.RemoveAll(tmpBase)
		return nil, err
	}

	// Write main transcript.
	if err := writeJSONL(filepath.Join(projectDir, resolved.sessionID+".jsonl"), resolved.entries); err != nil {
		os.RemoveAll(tmpBase)
		return nil, err
	}

	// Copy auth files from the caller's config dir.
	copyAuthFiles(tmpBase, opts.Env)

	// Materialize subagent transcripts.
	if err := materializeSubkeys(ctx, store, tmpBase, projectDir, projectKey, resolved.sessionID, timeout); err != nil {
		os.RemoveAll(tmpBase)
		return nil, err
	}

	cleanup := func() {
		rmtreeWithRetry(tmpBase)
	}

	return &MaterializedResume{
		ConfigDir:       tmpBase,
		ResumeSessionID: resolved.sessionID,
		Cleanup:         cleanup,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

type candidateResult struct {
	sessionID string
	entries   []SessionStoreEntry
}

func loadCandidate(ctx context.Context, store SessionStore, projectKey, sessionID string, timeout time.Duration) (*candidateResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type loadResult struct {
		entries []SessionStoreEntry
		err     error
	}
	ch := make(chan loadResult, 1)
	go func() {
		entries, err := store.Load(SessionKey{ProjectKey: projectKey, SessionID: sessionID})
		ch <- loadResult{entries, err}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("SessionStore.Load() for session %s timed out", sessionID)
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("SessionStore.Load() failed: %w", r.err)
		}
		if len(r.entries) == 0 {
			return nil, nil
		}
		return &candidateResult{sessionID: sessionID, entries: r.entries}, nil
	}
}

func resolveContinueCandidate(ctx context.Context, store SessionStore, projectKey string, timeout time.Duration) (*candidateResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type listResult struct {
		sessions []SessionStoreListEntry
		err      error
	}
	ch := make(chan listResult, 1)
	go func() {
		sessions, err := store.ListSessions(projectKey)
		ch <- listResult{sessions, err}
	}()

	var sessions []SessionStoreListEntry
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("SessionStore.ListSessions() timed out")
	case r := <-ch:
		if r.err != nil {
			return nil, fmt.Errorf("SessionStore.ListSessions() failed: %w", r.err)
		}
		sessions = r.sessions
	}

	if len(sessions) == 0 {
		return nil, nil
	}

	// Sort by mtime descending.
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].Mtime > sessions[i].Mtime {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	for _, cand := range sessions {
		if !validateUUID(cand.SessionID) {
			continue
		}
		loaded, err := loadCandidate(ctx, store, projectKey, cand.SessionID, timeout)
		if err != nil {
			continue
		}
		if loaded == nil {
			continue
		}
		// Skip sidechain sessions.
		if len(loaded.entries) > 0 {
			if extra := loaded.entries[0].Extra; extra != nil {
				if isSidechain, _ := extra["isSidechain"].(bool); isSidechain {
					continue
				}
			}
		}
		return loaded, nil
	}
	return nil, nil
}

// writeJSONL writes entries as one JSON line each to path (mode 0600).
func writeJSONL(path string, entries []SessionStoreEntry) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		if _, err := f.Write(b); err != nil {
			return err
		}
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	return nil
}

// copyAuthFiles copies .credentials.json (with refreshToken redacted) and
// .claude.json from the caller's config dir to tmpBase.
func copyAuthFiles(tmpBase string, env map[string]string) {
	callerConfigDir := env["CLAUDE_CONFIG_DIR"]
	if callerConfigDir == "" {
		callerConfigDir = os.Getenv("CLAUDE_CONFIG_DIR")
	}
	sourceConfigDir := callerConfigDir
	if sourceConfigDir == "" {
		home, _ := os.UserHomeDir()
		sourceConfigDir = filepath.Join(home, ".claude")
	}

	// Copy and redact .credentials.json
	credsPath := filepath.Join(sourceConfigDir, ".credentials.json")
	credsData, err := os.ReadFile(credsPath)
	if err == nil {
		writeRedactedCredentials(string(credsData), filepath.Join(tmpBase, ".credentials.json"))
	}

	// Copy .claude.json
	claudeJSONSrc := filepath.Join(sourceConfigDir, ".claude.json")
	if callerConfigDir != "" {
		claudeJSONSrc = filepath.Join(callerConfigDir, ".claude.json")
	} else {
		home, _ := os.UserHomeDir()
		claudeJSONSrc = filepath.Join(home, ".claude.json")
	}
	copyFile(claudeJSONSrc, filepath.Join(tmpBase, ".claude.json"))
}

// writeRedactedCredentials writes credsJSON with claudeAiOauth.refreshToken removed.
// The resumed subprocess runs under a redirected CLAUDE_CONFIG_DIR; if it refreshed,
// the single-use refresh token would be consumed server-side and the new tokens written
// to a location the parent never reads back. With no refreshToken, the subprocess's
// refresh check short-circuits.
func writeRedactedCredentials(credsJSON string, dst string) {
	out := credsJSON
	var data map[string]any
	if err := json.Unmarshal([]byte(credsJSON), &data); err == nil {
		if oauth, ok := data["claudeAiOauth"].(map[string]any); ok {
			if _, has := oauth["refreshToken"]; has {
				delete(oauth, "refreshToken")
				if b, err := json.Marshal(data); err == nil {
					out = string(b)
				}
			}
		}
	}
	os.WriteFile(dst, []byte(out), 0o600)
}

func copyFile(src, dst string) {
	data, err := os.ReadFile(src)
	if err != nil {
		return
	}
	os.WriteFile(dst, data, 0o600)
}

// materializeSubkeys loads and writes all subagent transcripts under sessionID.
func materializeSubkeys(ctx context.Context, store SessionStore, tmpBase, projectDir, projectKey, sessionID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type subkeysResult struct {
		subkeys []string
		err     error
	}
	ch := make(chan subkeysResult, 1)
	go func() {
		subkeys, err := store.ListSubkeys(projectKey, sessionID)
		ch <- subkeysResult{subkeys, err}
	}()

	var subkeys []string
	select {
	case <-ctx.Done():
		return fmt.Errorf("SessionStore.ListSubkeys() timed out")
	case r := <-ch:
		if r.err != nil {
			return fmt.Errorf("SessionStore.ListSubkeys() failed: %w", r.err)
		}
		subkeys = r.subkeys
	}

	sessionDir := filepath.Join(projectDir, sessionID)
	for _, subpath := range subkeys {
		if !isSafeSubpath(subpath, sessionDir) {
			continue
		}

		// Load subkey entries.
		subCtx, subCancel := context.WithTimeout(ctx, timeout)
		type loadResult struct {
			entries []SessionStoreEntry
			err     error
		}
		loadCh := make(chan loadResult, 1)
		go func() {
			entries, err := store.Load(SessionKey{
				ProjectKey: projectKey,
				SessionID:  sessionID,
				Subpath:    subpath,
			})
			loadCh <- loadResult{entries, err}
		}()

		var subEntries []SessionStoreEntry
		select {
		case <-subCtx.Done():
			subCancel()
			continue
		case r := <-loadCh:
			subCancel()
			if r.err != nil || len(r.entries) == 0 {
				continue
			}
			subEntries = r.entries
		}

		// Partition: agent_metadata entries describe the .meta.json sidecar;
		// everything else is a transcript line.
		var metadata []map[string]any
		var transcript []SessionStoreEntry
		for _, e := range subEntries {
			if e.Extra != nil && e.Extra["type"] == "agent_metadata" {
				metadata = append(metadata, e.Extra)
			} else {
				transcript = append(transcript, e)
			}
		}

		// Write transcript.
		subFile := filepath.Join(sessionDir, subpath+".jsonl")
		os.MkdirAll(filepath.Dir(subFile), 0o700)
		if len(transcript) > 0 {
			writeJSONL(subFile, transcript)
		}

		// Write .meta.json sidecar (last metadata entry wins).
		if len(metadata) > 0 {
			last := metadata[len(metadata)-1]
			metaContent := make(map[string]any, len(last))
			for k, v := range last {
				if k != "type" {
					metaContent[k] = v
				}
			}
			metaFile := strings.TrimSuffix(subFile, ".jsonl") + ".meta.json"
			if b, err := json.Marshal(metaContent); err == nil {
				os.WriteFile(metaFile, b, 0o600)
			}
		}
	}
	return nil
}

// isSafeSubpath rejects subpaths that are empty, absolute, contain "..", or
// escape sessionDir after resolution.
func isSafeSubpath(subpath, sessionDir string) bool {
	if subpath == "" {
		return false
	}
	// Reject absolute paths.
	if filepath.IsAbs(subpath) {
		return false
	}
	// Reject drive-prefixed paths (C:foo, UNC).
	if strings.ContainsRune(subpath, ':') {
		return false
	}
	// Reject null bytes.
	if strings.ContainsRune(subpath, '\x00') {
		return false
	}
	// Reject paths with traversal components (. or ..).
	for _, part := range strings.FieldsFunc(subpath, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if part == "." || part == ".." {
			return false
		}
	}
	// Resolve and verify it stays under sessionDir.
	target := filepath.Join(sessionDir, subpath+".jsonl")
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	absSession, err := filepath.Abs(sessionDir)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(absTarget, absSession+string(filepath.Separator)) {
		return false
	}
	return true
}

// rmtreeWithRetry removes path with retries on transient lock errors.
// On Windows, AV/indexer can briefly hold a handle on freshly-written files.
func rmtreeWithRetry(path string) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return
	}
	for i := 0; i < 4; i++ {
		if err := os.RemoveAll(path); err == nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	os.RemoveAll(path) // final attempt, ignore error
}
