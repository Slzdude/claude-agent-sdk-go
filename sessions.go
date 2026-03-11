package claude

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

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
func ListSessions(directory string, includeWorktrees bool, limit int) ([]SDKSessionInfo, error) {
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
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// ListAllSessions scans every project directory under ~/.claude/projects/ and
// returns sessions sorted newest-first. Pass limit<=0 for no limit.
// This mirrors Python SDK's list_sessions(directory=None) behaviour.
func ListAllSessions(limit int) ([]SDKSessionInfo, error) {
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

func getProjectsDir() string {
	return filepath.Join(getClaudeConfigDir(), "projects")
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

		// Check for sidechain sessions
		firstLine := info.head
		if nl := strings.IndexByte(firstLine, '\n'); nl >= 0 {
			firstLine = firstLine[:nl]
		}
		if strings.Contains(firstLine, `"isSidechain":true`) || strings.Contains(firstLine, `"isSidechain": true`) {
			continue
		}

		customTitle := extractLastJSONStringField(info.tail, "customTitle")
		firstPrompt := extractFirstPromptFromHead(info.head)
		summary := customTitle
		if summary == "" {
			summary = extractLastJSONStringField(info.tail, "summary")
		}
		if summary == "" {
			summary = firstPrompt
		}

		if summary == "" {
			continue // skip metadata-only sessions
		}

		gitBranch := extractLastJSONStringField(info.tail, "gitBranch")
		if gitBranch == "" {
			gitBranch = extractJSONStringField(info.head, "gitBranch")
		}
		cwd := extractJSONStringField(info.head, "cwd")
		if cwd == "" {
			cwd = projectPath
		}

		results = append(results, SDKSessionInfo{
			SessionID:    sessionID,
			Summary:      summary,
			LastModified: info.mtime,
			FileSize:     info.size,
			CustomTitle:  customTitle,
			FirstPrompt:  firstPrompt,
			GitBranch:    gitBranch,
			CWD:          cwd,
		})
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
	defer f.Close()

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
