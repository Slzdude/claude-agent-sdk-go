package claude

import (
	"bufio"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// GetSessionInfo returns metadata for a single session by ID.
// Returns nil if not found, is a sidechain, or has no extractable summary.
func GetSessionInfo(sessionID, directory string) (*SDKSessionInfo, error) {
	if !validateUUID(sessionID) {
		return nil, nil
	}
	fileName := sessionID + ".jsonl"

	readInfo := func(projectDir, projectPath string) *SDKSessionInfo {
		lite, err := readSessionLite(filepath.Join(projectDir, fileName))
		if err != nil {
			return nil
		}
		return parseSessionInfoFromLite(sessionID, lite, projectPath)
	}

	if directory != "" {
		canonDir, err := filepath.EvalSymlinks(directory)
		if err != nil {
			canonDir = directory
		}
		canonDir = normalizeUnicode(canonDir)

		if projectDir := findProjectDir(canonDir); projectDir != "" {
			if info := readInfo(projectDir, canonDir); info != nil {
				return info, nil
			}
		}
		for _, wt := range getWorktreePaths(canonDir) {
			if wt == canonDir {
				continue
			}
			if projectDir := findProjectDir(wt); projectDir != "" {
				if info := readInfo(projectDir, wt); info != nil {
					return info, nil
				}
			}
		}
		return nil, nil
	}

	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		projectDir := filepath.Join(projectsDir, e.Name())
		if info := readInfo(projectDir, ""); info != nil {
			return info, nil
		}
	}
	return nil, nil
}

const (
	liteReadBufSize = 65536
	maxSanitizedLen = 200
)

var (
	uuidRE            = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	skipFirstPromptRE = regexp.MustCompile(`(?s)^(?:<local-command-stdout>|<session-start-hook>|<tick>|<goal>|\[Request interrupted by user[^\]]*\]|\s*<ide_opened_file>[\s\S]*</ide_opened_file>\s*|\s*<ide_selection>[\s\S]*</ide_selection>\s*)$`)
	commandNameRE     = regexp.MustCompile(`<command-name>(.*?)</command-name>`)
	sanitizeRE        = regexp.MustCompile(`[^a-zA-Z0-9]`)
)

// ListSessions returns sessions from the given project directory (and worktrees
// if requested), sorted newest-first. Pass limit <= 0 for no limit.
// Pass offset > 0 to skip the first N sessions (after sorting).
func ListSessions(directory string, includeWorktrees bool, limit int, offset int) ([]SDKSessionInfo, error) {
	if directory == "" {
		directory = "."
	}
	canonDir, err := filepath.EvalSymlinks(directory)
	if err != nil {
		canonDir = directory
	}
	canonDir = normalizeUnicode(canonDir)

	var all []SDKSessionInfo
	dirs := []string{canonDir}
	if includeWorktrees {
		if wts := getWorktreePaths(canonDir); len(wts) > 0 {
			dirs = append(dirs, wts...)
		}
	}
	for _, d := range dirs {
		projectDir := findProjectDir(d)
		if projectDir == "" {
			continue
		}
		sessions := readSessionsFromDir(projectDir, d)
		all = append(all, sessions...)
	}
	all = deduplicateSessions(all)
	sortSessionsByMtime(all)
	if offset > 0 {
		if offset >= len(all) {
			return nil, nil
		}
		all = all[offset:]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// ListAllSessions scans every project directory under ~/.claude/projects/ and
// returns sessions sorted newest-first. Pass limit<=0 for no limit.
// Pass offset > 0 to skip the first N sessions (after sorting).
// This mirrors Python SDK's list_sessions(directory=None) behaviour.
func ListAllSessions(limit int, offset int) ([]SDKSessionInfo, error) {
	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		// If the directory doesn't exist, return empty (not an error).
		return nil, nil
	}
	var all []SDKSessionInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(projectsDir, e.Name())
		all = append(all, readSessionsFromDir(dir, "")...)
	}
	all = deduplicateSessions(all)
	sortSessionsByMtime(all)
	if offset > 0 {
		if offset >= len(all) {
			return nil, nil
		}
		all = all[offset:]
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// GetSessionMessages returns the user/assistant messages from a session JSONL file.
// Pass offset=0 and limit<=0 for all messages from the start.
// When directory is empty, searches all project directories (mirrors Python SDK
// get_session_messages(session_id, directory=None) behaviour).
//
// The conversation is reconstructed via parentUuid chain links (same algorithm as
// the Python SDK and VS Code IDE): this correctly handles forked, rewound, and
// compacted sessions by returning only the main conversation thread.
func GetSessionMessages(sessionID string, directory string, limit int, offset int) ([]SessionMessage, error) {
	// Invalid UUID → return empty list, not error (matches Python SDK behaviour).
	if !validateUUID(sessionID) {
		return nil, nil
	}

	content, err := readSessionFileContent(sessionID, directory)
	if err != nil || content == "" {
		return nil, nil
	}

	entries := parseTranscriptEntries(content)
	chain := buildConversationChain(entries)
	var msgs []SessionMessage
	for _, e := range chain {
		if !isVisibleMessage(e) {
			continue
		}
		msgs = append(msgs, transcriptEntryToSessionMessage(e))
	}

	// Apply offset / limit.
	if offset > 0 {
		if offset >= len(msgs) {
			return nil, nil
		}
		msgs = msgs[offset:]
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

// ProjectKeyForDirectory derives the SessionStore project key from a directory path.
// This is the bridge between filesystem paths and SessionStore keys.
func ProjectKeyForDirectory(directory string) string {
	if directory == "" {
		directory = "."
	}
	canonDir, err := filepath.EvalSymlinks(directory)
	if err != nil {
		canonDir = directory
	}
	return sanitizePath(normalizeUnicode(canonDir))
}

// SubagentInfo describes a subagent transcript.
type SubagentInfo struct {
	AgentID   string `json:"agent_id"`
	AgentType string `json:"agent_type,omitempty"`
	FilePath  string `json:"-"`
}

// ListSubagents returns the subagent IDs for a session.
// Recursively scans <sessionId>/subagents/**/agent-*.jsonl files, including
// nested subdirectories like workflows/<runId>/.
func ListSubagents(sessionID, directory string) ([]SubagentInfo, error) {
	if !validateUUID(sessionID) {
		return nil, nil
	}
	dir := resolveSubagentsDir(sessionID, directory)
	if dir == "" {
		return nil, nil
	}
	var result []SubagentInfo
	collectAgentFiles(dir, &result)
	return result, nil
}

// collectAgentFiles recursively collects agent-*.jsonl files from a directory tree.
func collectAgentFiles(baseDir string, result *[]SubagentInfo) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		fullPath := filepath.Join(baseDir, name)
		if e.IsDir() {
			collectAgentFiles(fullPath, result)
			continue
		}
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		agentID := strings.TrimPrefix(strings.TrimSuffix(name, ".jsonl"), "agent-")
		agentType := ""
		if data, err := os.ReadFile(fullPath); err == nil {
			lines := strings.SplitN(string(data), "\n", 2)
			if len(lines) > 0 {
				var entry map[string]any
				if json.Unmarshal([]byte(lines[0]), &entry) == nil {
					agentType = strVal(entry, "agentType")
				}
			}
		}
		*result = append(*result, SubagentInfo{
			AgentID:   agentID,
			AgentType: agentType,
			FilePath:  fullPath,
		})
	}
}

// GetSubagentMessages returns messages from a subagent's transcript.
// Searches nested subdirectories (e.g. workflows/<runId>/) to find the agent file.
func GetSubagentMessages(sessionID, agentID, directory string, limit, offset int) ([]SessionMessage, error) {
	if !validateUUID(sessionID) {
		return nil, nil
	}
	dir := resolveSubagentsDir(sessionID, directory)
	if dir == "" {
		return nil, nil
	}

	// Search for the agent file recursively (matches Python's _collect_agent_files).
	var agentFilePath string
	var agents []SubagentInfo
	collectAgentFiles(dir, &agents)
	for _, a := range agents {
		if a.AgentID == agentID {
			agentFilePath = a.FilePath
			break
		}
	}
	if agentFilePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(agentFilePath)
	if err != nil {
		return nil, nil
	}
	entries := parseTranscriptEntries(string(data))
	chain := buildSubagentChain(entries)
	var msgs []SessionMessage
	for _, e := range chain {
		t := strVal(e, "type")
		if t != "user" && t != "assistant" {
			continue
		}
		msgs = append(msgs, transcriptEntryToSessionMessage(e))
	}
	if offset > 0 {
		if offset >= len(msgs) {
			return nil, nil
		}
		msgs = msgs[offset:]
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

func resolveSubagentsDir(sessionID, directory string) string {
	fileName := sessionID + ".jsonl"
	tryDir := func(projectDir string) string {
		subDir := filepath.Join(projectDir, sessionID, "subagents")
		if fi, err := os.Stat(subDir); err == nil && fi.IsDir() {
			return subDir
		}
		return ""
	}
	if directory != "" {
		canonDir, err := filepath.EvalSymlinks(directory)
		if err != nil {
			canonDir = directory
		}
		canonDir = normalizeUnicode(canonDir)
		if projectDir := findProjectDir(canonDir); projectDir != "" {
			if d := tryDir(projectDir); d != "" {
				return d
			}
		}
		for _, wt := range getWorktreePaths(canonDir) {
			if wt == canonDir {
				continue
			}
			if projectDir := findProjectDir(wt); projectDir != "" {
				if d := tryDir(projectDir); d != "" {
					return d
				}
			}
		}
		return ""
	}
	// Check if session file exists in any project dir, then look for subagents.
	projectsDir := getProjectsDir()
	ents, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		projectDir := filepath.Join(projectsDir, e.Name())
		fp := filepath.Join(projectDir, fileName)
		if _, err := os.Stat(fp); err == nil {
			if d := tryDir(projectDir); d != "" {
				return d
			}
		}
	}
	return ""
}

func buildSubagentChain(entries []map[string]any) []map[string]any {
	if len(entries) == 0 {
		return nil
	}
	byUUID := make(map[string]map[string]any, len(entries))
	entryPos := make(map[string]int, len(entries))
	for i, e := range entries {
		uid := strVal(e, "uuid")
		byUUID[uid] = e
		entryPos[uid] = i
	}
	// Find leaf: last entry that is user or assistant.
	var best map[string]any
	bestPos := -1
	for i := len(entries) - 1; i >= 0; i-- {
		t := strVal(entries[i], "type")
		if t == "user" || t == "assistant" {
			best = entries[i]
			bestPos = i
			break
		}
	}
	if best == nil {
		return nil
	}
	_ = bestPos
	// Walk from best leaf to root.
	chain := make([]map[string]any, 0, 64)
	seen := make(map[string]bool)
	cur := best
	for cur != nil {
		uid := strVal(cur, "uuid")
		if seen[uid] {
			break
		}
		seen[uid] = true
		chain = append(chain, cur)
		parent := strVal(cur, "parentUuid")
		if parent == "" {
			break
		}
		cur = byUUID[parent]
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// Store-backed session operations ----------------------------------------------------------------

// storeListLoadConcurrency is the upper bound on concurrent store.Load() calls
// in ListSessionsFromStore. Prevents exhausting adapter connection pools.
const storeListLoadConcurrency = 16

// ListSessionsFromStore lists sessions from a SessionStore.
// Supports offset pagination and gap-fill with stale sidecar detection,
// matching Python SDK's list_sessions_from_store.
func ListSessionsFromStore(store SessionStore, projectKey string, limit, offset int) ([]SDKSessionInfo, error) {
	// Try summary-backed fast path with gap-fill.
	if summaries, err := store.ListSessionSummaries(projectKey); err == nil {
		// Also call list_sessions for gap-fill (stale sidecar detection).
		var listing []SessionStoreListEntry
		knownMtimes := make(map[string]int64)
		if ls, lsErr := store.ListSessions(projectKey); lsErr == nil {
			listing = ls
			for _, e := range ls {
				knownMtimes[e.SessionID] = e.Mtime
			}
		}

		type slot struct {
			mtime int64
			info  *SDKSessionInfo
			sid   string
		}
		var slots []slot
		freshIDs := make(map[string]bool)

		for _, s := range summaries {
			sid := s.SessionID
			if len(listing) > 0 {
				known, exists := knownMtimes[sid]
				if !exists {
					// Summary for a session list_sessions no longer reports — drop.
					continue
				}
				if s.Mtime < known {
					// Stale sidecar — let gap-fill re-fold from source.
					continue
				}
			}
			info := summaryEntryToSDKInfo(s, "")
			if info == nil {
				freshIDs[sid] = true
				continue
			}
			slots = append(slots, slot{mtime: s.Mtime, info: info, sid: sid})
			freshIDs[sid] = true
		}

		// Add placeholder slots for sessions missing from summaries (gap-fill).
		if len(listing) > 0 {
			for _, e := range listing {
				if !freshIDs[e.SessionID] {
					slots = append(slots, slot{mtime: e.Mtime, sid: e.SessionID})
				}
			}
		}

		// Sort by mtime descending, then apply offset/limit BEFORE loading.
		sort.Slice(slots, func(i, j int) bool {
			return slots[i].mtime > slots[j].mtime
		})
		page := slots
		if offset > 0 && offset < len(page) {
			page = page[offset:]
		} else if offset >= len(page) {
			page = nil
		}
		if limit > 0 && len(page) > limit {
			page = page[:limit]
		}

		// Load gap-fill sessions concurrently with bounded parallelism.
		results := make([]SDKSessionInfo, 0, len(page))
		var mu sync.Mutex
		sem := make(chan struct{}, storeListLoadConcurrency)
		var wg sync.WaitGroup

		for _, sl := range page {
			if sl.info != nil {
				results = append(results, *sl.info)
				continue
			}
			// Gap-fill: load from store.
			wg.Add(1)
			go func(sid string, mtime int64) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				storeEntries, err := store.Load(SessionKey{ProjectKey: projectKey, SessionID: sid})
				if err != nil || len(storeEntries) == 0 {
					return
				}
				info := extractSessionInfoFromStoreEntries(sid, storeEntries, mtime)
				if info != nil {
					mu.Lock()
					results = append(results, *info)
					mu.Unlock()
				}
			}(sl.sid, sl.mtime)
		}
		wg.Wait()

		sortSessionsByMtime(results)
		return results, nil
	}

	// Fallback to list_sessions + per-session load with bounded concurrency.
	entries, err := store.ListSessions(projectKey)
	if err != nil {
		return nil, err
	}

	// Apply offset/limit before loading.
	page := entries
	if offset > 0 && offset < len(page) {
		page = page[offset:]
	} else if offset >= len(page) {
		return nil, nil
	}
	if limit > 0 && len(page) > limit {
		page = page[:limit]
	}

	results := make([]SDKSessionInfo, 0, len(page))
	var mu sync.Mutex
	sem := make(chan struct{}, storeListLoadConcurrency)
	var wg sync.WaitGroup

	for _, e := range page {
		wg.Add(1)
		go func(e SessionStoreListEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			storeEntries, err := store.Load(SessionKey{ProjectKey: projectKey, SessionID: e.SessionID})
			if err != nil || len(storeEntries) == 0 {
				return
			}
			info := extractSessionInfoFromStoreEntries(e.SessionID, storeEntries, e.Mtime)
			if info != nil {
				mu.Lock()
				results = append(results, *info)
				mu.Unlock()
			}
		}(e)
	}
	wg.Wait()

	sortSessionsByMtime(results)
	return results, nil
}

// summaryEntryToSDKInfo converts a SessionSummaryEntry to SDKSessionInfo.
// projectPath is used as a fallback for cwd when the summary doesn't have one.
// Returns nil if the summary indicates a sidechain or empty session.
// Matches Python SDK's summary_entry_to_sdk_info exactly.
func summaryEntryToSDKInfo(s SessionSummaryEntry, projectPath string) *SDKSessionInfo {
	if s.Data == nil {
		return nil
	}
	if s.Data["is_sidechain"] == true {
		return nil
	}

	// Resolve first_prompt: only use if locked (matching Python's
	// first_prompt_locked check), otherwise fall back to command_fallback.
	var firstPrompt string
	if s.Data["first_prompt_locked"] == true {
		if fp, ok := s.Data["first_prompt"].(string); ok {
			firstPrompt = fp
		}
	}
	if firstPrompt == "" {
		if fb, ok := s.Data["command_fallback"].(string); ok && fb != "" {
			firstPrompt = fb
		}
	}

	// Resolve custom_title (custom_title || ai_title).
	customTitle := ""
	if ct, ok := s.Data["custom_title"].(string); ok && ct != "" {
		customTitle = ct
	} else if at, ok := s.Data["ai_title"].(string); ok && at != "" {
		customTitle = at
	}

	// Resolve summary (custom_title || last_prompt || summary_hint || first_prompt).
	summary := customTitle
	if summary == "" {
		if lp, ok := s.Data["last_prompt"].(string); ok && lp != "" {
			summary = lp
		}
	}
	if summary == "" {
		if sh, ok := s.Data["summary_hint"].(string); ok && sh != "" {
			summary = sh
		}
	}
	if summary == "" {
		summary = firstPrompt
	}
	if summary == "" {
		return nil
	}

	info := &SDKSessionInfo{
		SessionID:    s.SessionID,
		Summary:      summary,
		LastModified: s.Mtime,
		CustomTitle:  customTitle,
		FirstPrompt:  firstPrompt,
	}
	if tag, ok := s.Data["tag"].(string); ok && tag != "" {
		info.Tag = tag
	}
	if ca, ok := s.Data["created_at"].(int64); ok && ca > 0 {
		info.CreatedAt = &ca
	}
	if gb, ok := s.Data["git_branch"].(string); ok && gb != "" {
		info.GitBranch = gb
	}
	// cwd: summary value wins, then projectPath fallback (matching Python).
	if cwd, ok := s.Data["cwd"].(string); ok && cwd != "" {
		info.CWD = cwd
	} else if projectPath != "" {
		info.CWD = projectPath
	}
	return info
}

func extractSessionInfoFromStoreEntries(sessionID string, entries []SessionStoreEntry, mtime int64) *SDKSessionInfo {
	if len(entries) == 0 {
		return nil
	}
	// Serialize entries to JSONL, then call the same parser as the disk path.
	// Matches Python's get_session_info_from_store which does:
	//   lines = [json.dumps(e) for e in entries]
	//   return _parse_session_info_from_lite(session_id, _build_lite_from_content("\n".join(lines)))
	//
	// IMPORTANT: parseSessionInfoFromLite matches patterns like `{"type":"tag"` anchored at
	// the start of a line, so we must put "type" first in the serialized JSON.
	var sb strings.Builder
	for _, e := range entries {
		// Serialize remaining fields (excluding type/uuid/timestamp — handled separately).
		rest := make(map[string]any, len(e.Extra))
		for k, v := range e.Extra {
			if k != "type" && k != "uuid" && k != "timestamp" {
				rest[k] = v
			}
		}
		restB, err := json.Marshal(rest)
		if err != nil {
			continue
		}
		// Build JSON with "type" first, then optionally uuid/timestamp, then rest.
		// This ensures parseSessionInfoFromLite's line-prefix checks work correctly.
		sb.WriteByte('{')
		sb.WriteString(`"type":`)
		typeB, _ := json.Marshal(e.Type)
		sb.Write(typeB)
		if e.UUID != "" {
			sb.WriteString(`,"uuid":`)
			uuidB, _ := json.Marshal(e.UUID)
			sb.Write(uuidB)
		}
		if e.Timestamp != "" {
			sb.WriteString(`,"timestamp":`)
			tsB, _ := json.Marshal(e.Timestamp)
			sb.Write(tsB)
		}
		// Append remaining fields from rest (strip leading '{').
		if len(restB) > 2 { // not empty {}
			sb.WriteByte(',')
			sb.Write(restB[1 : len(restB)-1]) // strip outer braces
		}
		sb.WriteByte('}')
		sb.WriteByte('\n')
	}
	content := sb.String()
	if content == "" {
		return nil
	}
	lite := sessionFileInfoFromContent(content, mtime, int64(len(content)))
	return parseSessionInfoFromLite(sessionID, lite, "")
}

// sessionFileInfoFromContent builds a sessionFileInfo from an in-memory JSONL
// string (e.g. serialized store entries), using the same head/tail logic as
// readSessionLite. Used by extractSessionInfoFromStoreEntries.
func sessionFileInfoFromContent(content string, mtime, size int64) *sessionFileInfo {
	head := content
	if len(head) > liteReadBufSize {
		head = head[:liteReadBufSize]
	}
	tail := content
	if len(tail) > liteReadBufSize {
		tail = tail[len(tail)-liteReadBufSize:]
	}
	return &sessionFileInfo{mtime: mtime, size: size, head: head, tail: tail}
}

// GetSessionMessagesFromStore loads session messages from a SessionStore.
func GetSessionMessagesFromStore(store SessionStore, key SessionKey, limit, offset int) ([]SessionMessage, error) {
	entries, err := store.Load(key)
	if err != nil || len(entries) == 0 {
		return nil, nil
	}
	// Convert store entries to transcript format, filtering by type+uuid
	// (matching Python's _filter_transcript_entries).
	var transcript []map[string]any
	for _, e := range entries {
		if !transcriptEntryTypes[e.Type] || e.UUID == "" {
			continue
		}
		m := map[string]any{
			"type": e.Type,
			"uuid": e.UUID,
		}
		for k, v := range e.Extra {
			m[k] = v
		}
		transcript = append(transcript, m)
	}
	chain := buildConversationChain(transcript)
	var msgs []SessionMessage
	for _, e := range chain {
		if !isVisibleMessage(e) {
			continue
		}
		msgs = append(msgs, transcriptEntryToSessionMessage(e))
	}
	if offset > 0 {
		if offset >= len(msgs) {
			return nil, nil
		}
		msgs = msgs[offset:]
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

// GetSessionInfoFromStore loads session info from a SessionStore.
func GetSessionInfoFromStore(store SessionStore, key SessionKey) (*SDKSessionInfo, error) {
	entries, err := store.Load(key)
	if err != nil || len(entries) == 0 {
		return nil, nil
	}
	info := extractSessionInfoFromStoreEntries(key.SessionID, entries, 0)
	return info, nil
}

// ListSubagentsFromStore lists subagent transcripts from a SessionStore.
func ListSubagentsFromStore(store SessionStore, projectKey, sessionID string) ([]SubagentInfo, error) {
	subkeys, err := store.ListSubkeys(projectKey, sessionID)
	if err != nil {
		return nil, err
	}
	var result []SubagentInfo
	seen := make(map[string]bool)
	for _, sub := range subkeys {
		// Only consider subagent keys (matching Python's "subagents/" prefix filter).
		if !strings.HasPrefix(sub, "subagents/") {
			continue
		}
		// Get last path component — handles nested paths like
		// "subagents/workflows/run-1/agent-abc" (matches Python's rsplit("/",1)[-1]).
		last := sub[strings.LastIndex(sub, "/")+1:]
		if !strings.HasPrefix(last, "agent-") {
			continue
		}
		agentID := strings.TrimPrefix(last, "agent-")
		if seen[agentID] {
			continue
		}
		seen[agentID] = true
		result = append(result, SubagentInfo{AgentID: agentID})
	}
	return result, nil
}

// GetSubagentMessagesFromStore loads subagent messages from a SessionStore.
// Filters out agent_metadata entries (matching Python's explicit filter).
func GetSubagentMessagesFromStore(store SessionStore, key SessionKey, limit, offset int) ([]SessionMessage, error) {
	entries, err := store.Load(key)
	if err != nil || len(entries) == 0 {
		return nil, nil
	}
	var transcript []map[string]any
	for _, e := range entries {
		// Filter out agent_metadata entries (synthetic metadata from mirror hook).
		if e.Type == "agent_metadata" {
			continue
		}
		// Filter by type+uuid (matching Python's _filter_transcript_entries).
		if !transcriptEntryTypes[e.Type] || e.UUID == "" {
			continue
		}
		m := map[string]any{"type": e.Type, "uuid": e.UUID}
		for k, v := range e.Extra {
			m[k] = v
		}
		transcript = append(transcript, m)
	}
	chain := buildSubagentChain(transcript)
	var msgs []SessionMessage
	for _, e := range chain {
		t := strVal(e, "type")
		if t != "user" && t != "assistant" {
			continue
		}
		msgs = append(msgs, transcriptEntryToSessionMessage(e))
	}
	if offset > 0 {
		if offset >= len(msgs) {
			return nil, nil
		}
		msgs = msgs[offset:]
	}
	if limit > 0 && len(msgs) > limit {
		msgs = msgs[:limit]
	}
	return msgs, nil
}

// foldLastWinsFields maps JSONL entry keys → summary data keys.
var foldLastWinsFields = map[string]string{
	"customTitle": "custom_title",
	"aiTitle":     "ai_title",
	"lastPrompt":  "last_prompt",
	"summary":     "summary_hint",
	"gitBranch":   "git_branch",
}

// skipFirstPromptRe matches auto-generated patterns that should not be used as first_prompt.
var skipFirstPromptRe = regexp.MustCompile(
	`^(?:<local-command-stdout>|<session-start-hook>|<tick>|<goal>|` +
		`\[Request interrupted by user[^\]]*\]|` +
		`\s*<ide_opened_file>[\s\S]*</ide_opened_file>\s*$|` +
		`\s*<ide_selection>[\s\S]*</ide_selection>\s*$)`,
)

// commandNameRe matches slash-command names in user messages.
var commandNameRe = regexp.MustCompile(`<command-name>(.*?)</command-name>`)

// FoldSessionSummary incrementally derives session metadata from entries.
// Stores can call this inside Append() to maintain a SessionSummaryEntry sidecar.
// The prev parameter is the previous summary (nil for first call).
// mtime is NOT set by the fold — the adapter stamps it after persisting.
// Returns the updated summary.
func FoldSessionSummary(prev *SessionSummaryEntry, key SessionKey, entries []SessionStoreEntry) *SessionSummaryEntry {
	if len(entries) == 0 {
		return prev
	}
	var s *SessionSummaryEntry
	if prev != nil {
		cp := *prev
		cp.Data = make(map[string]any, len(prev.Data))
		for k, v := range prev.Data {
			cp.Data[k] = v
		}
		s = &cp
	} else {
		s = &SessionSummaryEntry{
			SessionID: key.SessionID,
			Data:      make(map[string]any),
		}
	}

	for _, e := range entries {
		if e.Extra == nil {
			continue
		}
		// Set-once: is_sidechain.
		if _, has := s.Data["is_sidechain"]; !has {
			s.Data["is_sidechain"] = e.Extra["isSidechain"] == true
		}
		// Set-once: created_at (first parseable ISO timestamp → epoch ms).
		if _, has := s.Data["created_at"]; !has {
			if ts, ok := e.Extra["timestamp"].(string); ok && ts != "" {
				if ms := isoToEpochMs(ts); ms > 0 {
					s.Data["created_at"] = ms
				}
			}
		}
		// Set-once: cwd (first non-empty).
		if _, has := s.Data["cwd"]; !has {
			if cwd, ok := e.Extra["cwd"].(string); ok && cwd != "" {
				s.Data["cwd"] = cwd
			}
		}

		// First-prompt extraction with filtering.
		foldFirstPrompt(s.Data, e)

		// Last-wins fields with key mapping.
		for src, dst := range foldLastWinsFields {
			if v, ok := e.Extra[src].(string); ok && v != "" {
				s.Data[dst] = v
			}
		}

		// Tag (last-wins, empty means cleared).
		if e.Type == "tag" {
			if tag, ok := e.Extra["tag"].(string); ok && tag != "" {
				s.Data["tag"] = tag
			} else {
				delete(s.Data, "tag")
			}
		}
	}
	return s
}

// foldFirstPrompt extracts the first real user prompt from an entry,
// skipping meta, compact-summary, tool_result, slash-command, and auto-generated messages.
func foldFirstPrompt(data map[string]any, entry SessionStoreEntry) {
	if data["first_prompt_locked"] == true {
		return
	}
	if entry.Type != "user" {
		return
	}
	if entry.Extra["isMeta"] == true || entry.Extra["isCompactSummary"] == true {
		return
	}
	// Skip tool_result-carrying user messages.
	if msg, ok := entry.Extra["message"].(map[string]any); ok {
		if content, ok := msg["content"].([]any); ok {
			for _, block := range content {
				if b, ok := block.(map[string]any); ok && b["type"] == "tool_result" {
					return
				}
			}
		}
	}

	for _, text := range entryTextBlocks(entry) {
		result := strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
		if result == "" {
			continue
		}
		// Slash commands → stash as fallback.
		if cmdMatch := commandNameRe.FindStringSubmatch(result); cmdMatch != nil {
			if _, has := data["command_fallback"]; !has {
				data["command_fallback"] = cmdMatch[1]
			}
			continue
		}
		// Skip auto-generated patterns.
		if skipFirstPromptRe.MatchString(result) {
			continue
		}
		// Truncate to 200 runes.
		runes := []rune(result)
		if len(runes) > 200 {
			result = string(runes[:200]) + "…"
		}
		data["first_prompt"] = result
		data["first_prompt_locked"] = true
		return
	}
}

// entryTextBlocks extracts text strings from a user entry's message content.
func entryTextBlocks(entry SessionStoreEntry) []string {
	msg, ok := entry.Extra["message"].(map[string]any)
	if !ok {
		return nil
	}
	content := msg["content"]
	switch c := content.(type) {
	case string:
		return []string{c}
	case []any:
		var texts []string
		for _, block := range c {
			if b, ok := block.(map[string]any); ok && b["type"] == "text" {
				if t, ok := b["text"].(string); ok {
					texts = append(texts, t)
				}
			}
		}
		return texts
	}
	return nil
}

// isoToEpochMs parses an ISO-8601 timestamp string to Unix epoch milliseconds.
func isoToEpochMs(ts string) int64 {
	// Handle trailing Z.
	s := ts
	if strings.HasSuffix(s, "Z") {
		s = strings.TrimSuffix(s, "Z") + "+00:00"
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.UnixMilli()
}

// ImportSessionToStore replays a local session transcript into a SessionStore.
// If includeSubagents is true (recommended), subagent transcripts under
// <sessionId>/subagents/ are also imported with appropriate subpath keys.
func ImportSessionToStore(store SessionStore, sessionID, directory string, includeSubagents ...bool) error {
	if !validateUUID(sessionID) {
		return fmt.Errorf("invalid session_id: %s", sessionID)
	}

	doSubagents := true
	if len(includeSubagents) > 0 {
		doSubagents = includeSubagents[0]
	}

	// Find the session file path to derive project key from parent dir name.
	filePath, projectDir := findSessionFileWithDir(sessionID, directory)
	if filePath == "" {
		return fmt.Errorf("session %s not found", sessionID)
	}

	// Derive project key from the on-disk project directory name — matches
	// TranscriptMirrorBatcher's key derivation so imported sessions are
	// indistinguishable from live-mirrored ones.
	projectKey := filepath.Base(projectDir)

	// Import main transcript.
	if err := importJSONLFileToStore(store, filePath, SessionKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
	}); err != nil {
		return err
	}

	if !doSubagents {
		return nil
	}

	// Import subagent transcripts: <projectDir>/<sessionId>/subagents/**/*.jsonl
	subagentsDir := filepath.Join(projectDir, sessionID, "subagents")
	return importSubagentDir(store, subagentsDir, projectKey, sessionID, "")
}

// importSubagentDir recursively imports subagent JSONL files from a directory.
func importSubagentDir(store SessionStore, dir, projectKey, sessionID, relPrefix string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // subagents dir may not exist
	}
	for _, e := range entries {
		name := e.Name()
		fullPath := filepath.Join(dir, name)
		if e.IsDir() {
			// Recurse into nested dirs (e.g. workflows/<runId>/).
			nestedPrefix := name
			if relPrefix != "" {
				nestedPrefix = relPrefix + "/" + name
			}
			if err := importSubagentDir(store, fullPath, projectKey, sessionID, nestedPrefix); err != nil {
				return err
			}
			continue
		}
		if !strings.HasPrefix(name, "agent-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		// Derive subpath: subagents/agent-{id} or subagents/workflows/run-1/agent-{id}
		agentID := strings.TrimPrefix(strings.TrimSuffix(name, ".jsonl"), "agent-")
		subpath := "subagents/" + agentID
		if relPrefix != "" {
			subpath = "subagents/" + relPrefix + "/" + agentID
		}
		if err := importJSONLFileToStore(store, fullPath, SessionKey{
			ProjectKey: projectKey,
			SessionID:  sessionID,
			Subpath:    subpath,
		}); err != nil {
			return err
		}

		// Import .meta.json sidecar (matching Python's import_session_to_store).
		// The on-disk .jsonl does NOT contain agent_metadata entries — those are only
		// sent to live mirrors and persisted in the .meta.json sidecar. Import the
		// sidecar so materialize_resume_session() can recreate it and resumed subagents
		// keep their agentType/worktreePath.
		metaPath := strings.TrimSuffix(fullPath, ".jsonl") + ".meta.json"
		if metaBytes, err := os.ReadFile(metaPath); err == nil {
			var meta map[string]any
			if json.Unmarshal(metaBytes, &meta) == nil {
				// Python: meta_entry = {"type": "agent_metadata"}; meta_entry.update(meta)
				meta["type"] = "agent_metadata"
				metaEntry := SessionStoreEntry{
					Type:      "agent_metadata",
					UUID:      strVal(meta, "uuid"),
					Timestamp: strVal(meta, "timestamp"),
					Extra:     meta,
				}
				_ = store.Append(SessionKey{
					ProjectKey: projectKey,
					SessionID:  sessionID,
					Subpath:    subpath,
				}, []SessionStoreEntry{metaEntry})
			}
		}
	}
	return nil
}

// importJSONLFileToStore streams a JSONL file line-by-line and appends entries to
// the store in batches. Matches Python's _append_jsonl_file_in_batches which flushes
// at both MAX_PENDING_ENTRIES (500) and MAX_PENDING_BYTES (1 MiB) to bound memory.
func importJSONLFileToStore(store SessionStore, filePath string, key SessionKey) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	const batchSize = MirrorMaxPendingEntries // 500, matches Python MAX_PENDING_ENTRIES
	var entries []SessionStoreEntry
	nbytes := 0
	scanner := bufio.NewScanner(f)
	// Allow scanning lines up to 10 MiB (single large tool-result entries can be big).
	scanner.Buffer(make([]byte, 64*1024), 10<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var raw map[string]any
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		entry := SessionStoreEntry{
			Type:      strVal(raw, "type"),
			UUID:      strVal(raw, "uuid"),
			Timestamp: strVal(raw, "timestamp"),
			Extra:     raw,
		}
		entries = append(entries, entry)
		nbytes += len(line)
		if len(entries) >= batchSize || nbytes >= MirrorMaxPendingBytes {
			if err := store.Append(key, entries); err != nil {
				return err
			}
			entries = entries[:0]
			nbytes = 0
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(entries) > 0 {
		return store.Append(key, entries)
	}
	return nil
}

// readSessionFileContent finds and reads the raw JSONL content for a session.
// When directory is "", searches all project directories.
// When directory is set, tries that directory then falls back to its git worktrees.
func readSessionFileContent(sessionID string, directory string) (string, error) {
	fileName := sessionID + ".jsonl"

	if directory == "" {
		// Search all project directories.
		projectsDir := getProjectsDir()
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			return "", nil
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			fp := filepath.Join(projectsDir, e.Name(), fileName)
			if data, err := os.ReadFile(fp); err == nil {
				return string(data), nil
			}
		}
		return "", nil
	}

	canonDir, err := filepath.EvalSymlinks(directory)
	if err != nil {
		canonDir = directory
	}
	canonDir = normalizeUnicode(canonDir)

	// Try the exact/prefix-matched project directory.
	if projectDir := findProjectDir(canonDir); projectDir != "" {
		fp := filepath.Join(projectDir, fileName)
		if data, err := os.ReadFile(fp); err == nil {
			return string(data), nil
		}
	}

	// Fallback: try git worktree paths.
	for _, wt := range getWorktreePaths(canonDir) {
		if wt == canonDir {
			continue
		}
		if projectDir := findProjectDir(wt); projectDir != "" {
			fp := filepath.Join(projectDir, fileName)
			if data, err := os.ReadFile(fp); err == nil {
				return string(data), nil
			}
		}
	}
	return "", nil
}

// transcriptEntryTypes are the entry types that carry uuid+parentUuid chain links.
var transcriptEntryTypes = map[string]bool{
	"user": true, "assistant": true, "progress": true, "system": true, "attachment": true,
}

// parseTranscriptEntries parses JSONL content into transcript entries.
// Only keeps entries with a uuid that are one of the transcript message types.
func parseTranscriptEntries(content string) []map[string]any {
	var entries []map[string]any
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if !transcriptEntryTypes[strVal(entry, "type")] {
			continue
		}
		if strVal(entry, "uuid") == "" {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

// buildConversationChain reconstructs the main conversation thread by following
// parentUuid links. Returns entries in chronological order (root → leaf).
// This mirrors _build_conversation_chain() in the Python SDK.
func buildConversationChain(entries []map[string]any) []map[string]any {
	if len(entries) == 0 {
		return nil
	}

	// Index by uuid for O(1) parent lookup; track file order for tie-breaking.
	byUUID := make(map[string]map[string]any, len(entries))
	entryPos := make(map[string]int, len(entries))
	for i, e := range entries {
		uid := strVal(e, "uuid")
		byUUID[uid] = e
		entryPos[uid] = i
	}

	// Find terminal nodes: no other entry's parentUuid points to them.
	referenced := make(map[string]bool, len(entries))
	for _, e := range entries {
		if p := strVal(e, "parentUuid"); p != "" {
			referenced[p] = true
		}
	}
	var terminals []map[string]any
	for _, e := range entries {
		if !referenced[strVal(e, "uuid")] {
			terminals = append(terminals, e)
		}
	}

	// Walk back from each terminal to find the nearest user/assistant leaf.
	var leaves []map[string]any
	for _, term := range terminals {
		cur := term
		seen := make(map[string]bool)
		for cur != nil {
			uid := strVal(cur, "uuid")
			if seen[uid] {
				break
			}
			seen[uid] = true
			t := strVal(cur, "type")
			if t == "user" || t == "assistant" {
				leaves = append(leaves, cur)
				break
			}
			parent := strVal(cur, "parentUuid")
			if parent == "" {
				break
			}
			cur = byUUID[parent]
		}
	}
	if len(leaves) == 0 {
		return nil
	}

	// Prefer main-chain leaves: not sidechain, not teamName, not isMeta.
	mainLeaves := make([]map[string]any, 0, len(leaves))
	for _, leaf := range leaves {
		if isSidechain, _ := leaf["isSidechain"].(bool); isSidechain {
			continue
		}
		if _, hasTeam := leaf["teamName"]; hasTeam {
			continue
		}
		if isMeta, _ := leaf["isMeta"].(bool); isMeta {
			continue
		}
		mainLeaves = append(mainLeaves, leaf)
	}
	pickFrom := mainLeaves
	if len(pickFrom) == 0 {
		pickFrom = leaves
	}

	// Pick the leaf with the highest file position.
	best := pickFrom[0]
	bestPos := entryPos[strVal(best, "uuid")]
	for _, leaf := range pickFrom[1:] {
		if pos := entryPos[strVal(leaf, "uuid")]; pos > bestPos {
			best = leaf
			bestPos = pos
		}
	}

	// Walk from best leaf to root via parentUuid, then reverse.
	chain := make([]map[string]any, 0, 64)
	seen := make(map[string]bool)
	cur := best
	for cur != nil {
		uid := strVal(cur, "uuid")
		if seen[uid] {
			break
		}
		seen[uid] = true
		chain = append(chain, cur)
		parent := strVal(cur, "parentUuid")
		if parent == "" {
			break
		}
		cur = byUUID[parent]
	}
	// Reverse to chronological order.
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}

// isVisibleMessage returns true if the entry should be included in returned messages.
// Matches Python SDK _is_visible_message(): includes isCompactSummary messages.
func isVisibleMessage(e map[string]any) bool {
	t := strVal(e, "type")
	if t != "user" && t != "assistant" {
		return false
	}
	if isMeta, _ := e["isMeta"].(bool); isMeta {
		return false
	}
	if isSidechain, _ := e["isSidechain"].(bool); isSidechain {
		return false
	}
	_, hasTeam := e["teamName"]
	return !hasTeam
}

// transcriptEntryToSessionMessage converts a raw entry to a SessionMessage.
// `sessionId` in the JSONL is camelCase; `parent_tool_use_id` is always nil
// (sidechain entries are already filtered out by isVisibleMessage).
func transcriptEntryToSessionMessage(e map[string]any) SessionMessage {
	t := strVal(e, "type")
	if t != "user" && t != "assistant" {
		t = "user"
	}
	message, _ := e["message"].(map[string]any)
	return SessionMessage{
		Type:      t,
		UUID:      strVal(e, "uuid"),
		SessionID: strVal(e, "sessionId"), // camelCase key in JSONL
		Message:   message,
	}
}

// -----------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------

func getClaudeConfigDir() string {
	if d := os.Getenv("CLAUDE_CONFIG_DIR"); d != "" {
		return normalizeUnicode(d)
	}
	home, _ := os.UserHomeDir()
	return normalizeUnicode(filepath.Join(home, ".claude"))
}

// getClaudeConfigDirWithOverride is like getClaudeConfigDir but checks
// envOverride["CLAUDE_CONFIG_DIR"] before os.Getenv. This lets callers
// that pass a custom CLAUDE_CONFIG_DIR to a subprocess resolve the same
// directory the subprocess writes to.
func getClaudeConfigDirWithOverride(envOverride map[string]string) string {
	if envOverride != nil {
		if d, ok := envOverride["CLAUDE_CONFIG_DIR"]; ok && d != "" {
			return normalizeUnicode(d)
		}
	}
	return getClaudeConfigDir()
}

func getProjectsDir() string {
	return filepath.Join(getClaudeConfigDir(), "projects")
}

// getProjectsWithOverride returns the projects directory, consulting
// envOverride["CLAUDE_CONFIG_DIR"] before os.Getenv.
func getProjectsWithOverride(envOverride map[string]string) string {
	return filepath.Join(getClaudeConfigDirWithOverride(envOverride), "projects")
}

func getProjectDir(projectPath string) string {
	return filepath.Join(getProjectsDir(), sanitizePath(projectPath))
}

func findProjectDir(projectPath string) string {
	exact := getProjectDir(projectPath)
	if fi, err := os.Stat(exact); err == nil && fi.IsDir() {
		return exact
	}

	sanitized := sanitizePath(projectPath)
	if len(sanitized) <= maxSanitizedLen {
		return "" // short path: exact match failure means no sessions
	}

	// Long paths: prefix-scan to handle hash mismatches between Node/Bun
	prefix := sanitized[:maxSanitizedLen]
	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), prefix+"-") {
			return filepath.Join(projectsDir, e.Name())
		}
	}
	return ""
}

// sanitizePath converts a path to a safe directory name, hashing long paths.
func sanitizePath(name string) string {
	sanitized := sanitizeRE.ReplaceAllString(name, "-")
	if len(sanitized) <= maxSanitizedLen {
		return sanitized
	}
	h := simpleHash(name)
	return fmt.Sprintf("%s-%s", sanitized[:maxSanitizedLen], h)
}

// simpleHash is a port of the JS simpleHash (32-bit int, base36).
// Emulates JS's `h = (h << 5) - h + charCode; h |= 0` (signed 32-bit truncation).
func simpleHash(s string) string {
	var h int32
	for _, ch := range s {
		// Use int64 arithmetic then truncate to int32 to match JS `|= 0`.
		h = int32(int64(h<<5) - int64(h) + int64(int32(ch)))
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		return "0"
	}
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	var out []byte
	n := uint32(h)
	for n > 0 {
		out = append([]byte{digits[n%36]}, out...)
		n /= 36
	}
	_ = fnv.New32() // ensure import used
	return string(out)
}

func normalizeUnicode(s string) string {
	// NFC normalization matches Python's unicodedata.normalize("NFC", ...).
	return norm.NFC.String(filepath.Clean(s))
}

func validateUUID(s string) bool {
	return uuidRE.MatchString(s)
}

// parseSessionInfoFromLite builds SDKSessionInfo from a lite session read.
// Returns nil for sidechain sessions or metadata-only sessions.
func parseSessionInfoFromLite(sessionID string, lite *sessionFileInfo, projectPath string) *SDKSessionInfo {
	head, tail := lite.head, lite.tail

	// Check for sidechain.
	firstLine := head
	if nl := strings.IndexByte(firstLine, '\n'); nl >= 0 {
		firstLine = firstLine[:nl]
	}
	if strings.Contains(firstLine, `"isSidechain":true`) || strings.Contains(firstLine, `"isSidechain": true`) {
		return nil
	}

	// Title: customTitle > aiTitle > lastPrompt > firstPrompt.
	customTitle := extractLastJSONStringField(tail, "customTitle")
	if customTitle == "" {
		customTitle = extractLastJSONStringField(head, "customTitle")
	}
	aiTitle := extractLastJSONStringField(tail, "aiTitle")
	if aiTitle == "" {
		aiTitle = extractLastJSONStringField(head, "aiTitle")
	}
	firstPrompt := extractFirstPromptFromHead(head)
	lastPrompt := extractLastJSONStringField(tail, "lastPrompt")
	summary := customTitle
	if summary == "" {
		summary = aiTitle
	}
	if summary == "" {
		summary = lastPrompt
	}
	if summary == "" {
		summary = firstPrompt
	}
	if summary == "" {
		return nil // skip metadata-only sessions
	}

	gitBranch := extractLastJSONStringField(tail, "gitBranch")
	if gitBranch == "" {
		gitBranch = extractJSONStringField(head, "gitBranch")
	}
	cwd := extractJSONStringField(head, "cwd")
	if cwd == "" {
		cwd = projectPath
	}

	// Tag extraction — scope to {"type":"tag"} lines.
	// The last tag entry wins (even if empty string, which means cleared).
	var tag string
	foundTag := false
	for _, line := range strings.Split(tail, "\n") {
		if strings.HasPrefix(line, `{"type":"tag"`) {
			tag = extractLastJSONStringField(line, "tag")
			foundTag = true
		}
	}
	if !foundTag {
		tag = ""
	}

	// CreatedAt from first entry's timestamp (epoch ms).
	var createdAt *int64
	ts := extractJSONStringField(firstLine, "timestamp")
	if ts != "" {
		ts = strings.TrimSuffix(ts, "Z")
		ts = strings.TrimSuffix(ts, "+00:00")
		if t, err := time.Parse("2006-01-02T15:04:05.000Z07:00", ts+"Z"); err == nil {
			ms := t.UnixMilli()
			createdAt = &ms
		} else if t, err := time.Parse(time.RFC3339, ts+"Z"); err == nil {
			ms := t.UnixMilli()
			createdAt = &ms
		}
	}

	return &SDKSessionInfo{
		SessionID:    sessionID,
		Summary:      summary,
		LastModified: lite.mtime,
		FileSize:     &lite.size,
		CustomTitle:  customTitle,
		FirstPrompt:  firstPrompt,
		GitBranch:    gitBranch,
		CWD:          cwd,
		Tag:          tag,
		CreatedAt:    createdAt,
	}
}

// readSessionsFromDir reads .jsonl session files from a project directory.
func readSessionsFromDir(projectDir string, projectPath string) []SDKSessionInfo {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return nil
	}

	var results []SDKSessionInfo
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		sessionID := strings.TrimSuffix(name, ".jsonl")
		if !validateUUID(sessionID) {
			continue
		}

		filePath := filepath.Join(projectDir, name)
		info, err := readSessionLite(filePath)
		if err != nil {
			continue
		}

		sessionInfo := parseSessionInfoFromLite(sessionID, info, projectPath)
		if sessionInfo != nil {
			results = append(results, *sessionInfo)
		}
	}
	return results
}

type sessionFileInfo struct {
	mtime int64
	size  int64
	head  string
	tail  string
}

func readSessionLite(filePath string) (*sessionFileInfo, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := fi.Size()
	mtime := fi.ModTime().UnixMilli()

	headBuf := make([]byte, liteReadBufSize)
	n, err := f.Read(headBuf)
	if err != nil || n == 0 {
		return nil, fmt.Errorf("empty or unreadable file")
	}
	head := string(headBuf[:n])

	var tail string
	if size <= liteReadBufSize {
		tail = head
	} else {
		tailOffset := size - liteReadBufSize
		if _, err := f.Seek(tailOffset, 0); err != nil {
			tail = head
		} else {
			tailBuf := make([]byte, liteReadBufSize)
			tn, _ := f.Read(tailBuf)
			tail = string(tailBuf[:tn])
		}
	}

	return &sessionFileInfo{mtime: mtime, size: size, head: head, tail: tail}, nil
}

// extractJSONStringField finds the first "key":"value" pair in text.
func extractJSONStringField(text, key string) string {
	patterns := []string{`"` + key + `":"`, `"` + key + `": "`}
	for _, pattern := range patterns {
		idx := strings.Index(text, pattern)
		if idx < 0 {
			continue
		}
		start := idx + len(pattern)
		i := start
		for i < len(text) {
			if text[i] == '\\' {
				i += 2
				continue
			}
			if text[i] == '"' {
				return unescapeJSONString(text[start:i])
			}
			i++
		}
	}
	return ""
}

// extractLastJSONStringField finds the LAST occurrence of "key":"value" in text.
func extractLastJSONStringField(text, key string) string {
	patterns := []string{`"` + key + `":"`, `"` + key + `": "`}
	var lastValue string
	for _, pattern := range patterns {
		from := 0
		for {
			idx := strings.Index(text[from:], pattern)
			if idx < 0 {
				break
			}
			abs := from + idx
			start := abs + len(pattern)
			i := start
			for i < len(text) {
				if text[i] == '\\' {
					i += 2
					continue
				}
				if text[i] == '"' {
					lastValue = unescapeJSONString(text[start:i])
					break
				}
				i++
			}
			from = i + 1
		}
	}
	return lastValue
}

func unescapeJSONString(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var decoded string
	if err := json.Unmarshal([]byte(`"`+s+`"`), &decoded); err == nil {
		return decoded
	}
	return s
}

// extractFirstPromptFromHead extracts the first meaningful user prompt.
func extractFirstPromptFromHead(head string) string {
	var commandFallback string
	lines := strings.Split(head, "\n")
	for _, line := range lines {
		if !strings.Contains(line, `"type":"user"`) && !strings.Contains(line, `"type": "user"`) {
			continue
		}
		if strings.Contains(line, `"tool_result"`) {
			continue
		}
		if strings.Contains(line, `"isMeta":true`) || strings.Contains(line, `"isMeta": true`) {
			continue
		}
		if strings.Contains(line, `"isCompactSummary":true`) || strings.Contains(line, `"isCompactSummary": true`) {
			continue
		}

		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry["type"] != "user" {
			continue
		}
		message, _ := entry["message"].(map[string]any)
		if message == nil {
			continue
		}

		var texts []string
		switch c := message["content"].(type) {
		case string:
			texts = []string{c}
		case []any:
			for _, item := range c {
				if block, ok := item.(map[string]any); ok {
					if block["type"] == "text" {
						if t, ok := block["text"].(string); ok {
							texts = append(texts, t)
						}
					}
				}
			}
		}

		for _, raw := range texts {
			result := strings.TrimSpace(strings.ReplaceAll(raw, "\n", " "))
			if result == "" {
				continue
			}
			if m := commandNameRE.FindStringSubmatch(result); m != nil {
				if commandFallback == "" {
					commandFallback = m[1]
				}
				continue
			}
			if skipFirstPromptRE.MatchString(result) {
				continue
			}
			if len([]rune(result)) > 200 {
				runes := []rune(result)
				result = string(runes[:200]) + "…"
			}
			return result
		}
	}
	if commandFallback != "" {
		return commandFallback
	}
	return ""
}

func deduplicateSessions(sessions []SDKSessionInfo) []SDKSessionInfo {
	byID := make(map[string]SDKSessionInfo, len(sessions))
	for _, s := range sessions {
		if existing, ok := byID[s.SessionID]; !ok || s.LastModified > existing.LastModified {
			byID[s.SessionID] = s
		}
	}
	out := make([]SDKSessionInfo, 0, len(byID))
	for _, s := range byID {
		out = append(out, s)
	}
	return out
}

func sortSessionsByMtime(sessions []SDKSessionInfo) {
	// Simple insertion sort (session counts are typically small)
	for i := 1; i < len(sessions); i++ {
		for j := i; j > 0 && sessions[j].LastModified > sessions[j-1].LastModified; j-- {
			sessions[j], sessions[j-1] = sessions[j-1], sessions[j]
		}
	}
}

func getWorktreePaths(cwd string) []string {
	// Use `git worktree list --porcelain` to enumerate additional worktrees.
	// The first entry is the main worktree (already included), so we skip it.
	out, err := exec.Command("git", "-C", cwd, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	var paths []string
	first := true
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			path := strings.TrimPrefix(line, "worktree ")
			path = strings.TrimSpace(path)
			// NFC-normalize to match Python's unicodedata.normalize("NFC", ...)
			// call in _get_worktree_paths. On macOS HFS+, git may return NFD
			// (decomposed) paths that must be normalized before directory-name
			// computation (sanitizePath / simpleHash) to match the on-disk key.
			path = normalizeUnicode(path)
			if first {
				first = false
				continue // skip main worktree
			}
			if path != "" {
				paths = append(paths, path)
			}
		}
	}
	_ = time.Now()            // keep time import
	_ = unicode.IsLetter('a') // keep unicode import
	return paths
}
