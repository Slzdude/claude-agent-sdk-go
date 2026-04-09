package claude

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// -----------------------------------------------------------------------
// Public API
// -----------------------------------------------------------------------

// RenameSession renames a session by appending a custom-title entry to its JSONL file.
func RenameSession(sessionID, title, directory string) error {
	if !validateUUID(sessionID) {
		return fmt.Errorf("invalid session_id: %s", sessionID)
	}
	stripped := strings.TrimSpace(title)
	if stripped == "" {
		return errors.New("title must be non-empty")
	}

	data := fmt.Sprintf(
		`{"type":"custom-title","customTitle":%q,"sessionId":%q}`, stripped, sessionID,
	) + "\n"

	return appendToSession(sessionID, data, directory)
}

// TagSession tags a session. Pass empty tag to clear.
func TagSession(sessionID, tag, directory string) error {
	if !validateUUID(sessionID) {
		return fmt.Errorf("invalid session_id: %s", sessionID)
	}
	if tag != "" {
		sanitized := sanitizeUnicode(tag)
		sanitized = strings.TrimSpace(sanitized)
		if sanitized == "" {
			return errors.New("tag must be non-empty after sanitization (use empty string to clear)")
		}
		tag = sanitized
	}

	data := fmt.Sprintf(
		`{"type":"tag","tag":%q,"sessionId":%q}`, tag, sessionID,
	) + "\n"

	return appendToSession(sessionID, data, directory)
}

// DeleteSession permanently removes a session's JSONL file.
func DeleteSession(sessionID, directory string) error {
	if !validateUUID(sessionID) {
		return fmt.Errorf("invalid session_id: %s", sessionID)
	}
	path := findSessionFile(sessionID, directory)
	if path == "" {
		return fmt.Errorf("session %s not found", sessionID)
	}
	return os.Remove(path)
}

// ForkSessionResult holds the new session ID after forking.
type ForkSessionResult struct {
	SessionID string
}

// ForkSession copies a session's transcript into a new session with remapped UUIDs.
func ForkSession(sessionID, directory, upToMessageID, title string) (*ForkSessionResult, error) {
	if !validateUUID(sessionID) {
		return nil, fmt.Errorf("invalid session_id: %s", sessionID)
	}
	if upToMessageID != "" && !validateUUID(upToMessageID) {
		return nil, fmt.Errorf("invalid up_to_message_id: %s", upToMessageID)
	}

	filePath, projectDir := findSessionFileWithDir(sessionID, directory)
	if filePath == "" {
		return nil, fmt.Errorf("session %s not found", sessionID)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(content) == 0 {
		return nil, fmt.Errorf("session %s has no messages to fork", sessionID)
	}

	transcript, contentReplacements := parseForkTranscript(string(content), sessionID)

	// Filter sidechains.
	var filtered []map[string]any
	for _, e := range transcript {
		if isSidechain, _ := e["isSidechain"].(bool); !isSidechain {
			filtered = append(filtered, e)
		}
	}
	transcript = filtered
	if len(transcript) == 0 {
		return nil, fmt.Errorf("session %s has no messages to fork", sessionID)
	}

	// Truncate to upToMessageID.
	if upToMessageID != "" {
		cutoff := -1
		for i, e := range transcript {
			if strVal(e, "uuid") == upToMessageID {
				cutoff = i
				break
			}
		}
		if cutoff == -1 {
			return nil, fmt.Errorf("message %s not found in session %s", upToMessageID, sessionID)
		}
		transcript = transcript[:cutoff+1]
	}

	// UUID remapping.
	uuidMapping := make(map[string]string, len(transcript))
	for _, e := range transcript {
		uuidMapping[strVal(e, "uuid")] = newUUID()
	}

	// Index by UUID for parent resolution.
	byUUID := make(map[string]map[string]any, len(transcript))
	for _, e := range transcript {
		byUUID[strVal(e, "uuid")] = e
	}

	// Filter out progress messages from output (but keep in index for chain walk).
	var writable []map[string]any
	for _, e := range transcript {
		if strVal(e, "type") != "progress" {
			writable = append(writable, e)
		}
	}
	if len(writable) == 0 {
		return nil, fmt.Errorf("session %s has no messages to fork", sessionID)
	}

	forkedSessionID := newUUID()
	var lines []string

	for i, original := range writable {
		newUUID := uuidMapping[strVal(original, "uuid")]

		// Resolve parentUuid, skipping progress ancestors.
		var newParentUUID string
		parentID := strVal(original, "parentUuid")
		for parentID != "" {
			parent, ok := byUUID[parentID]
			if !ok {
				break
			}
			if strVal(parent, "type") != "progress" {
				newParentUUID = uuidMapping[parentID]
				break
			}
			parentID = strVal(parent, "parentUuid")
		}

		// Build forked entry.
		forked := make(map[string]any, len(original)+5)
		for k, v := range original {
			forked[k] = v
		}
		forked["uuid"] = newUUID
		forked["parentUuid"] = newParentUUID
		forked["sessionId"] = forkedSessionID
		forked["isSidechain"] = false
		forked["forkedFrom"] = map[string]any{
			"sessionId":   sessionID,
			"messageUuid": strVal(original, "uuid"),
		}

		// Remap logicalParentUuid.
		if lp := strVal(original, "logicalParentUuid"); lp != "" {
			if mapped, ok := uuidMapping[lp]; ok {
				forked["logicalParentUuid"] = mapped
			}
		}

		// Remove fields that leak source session state.
		for _, key := range []string{"teamName", "agentName", "slug", "sourceToolAssistantUUID"} {
			delete(forked, key)
		}

		b, _ := json.Marshal(forked)
		lines = append(lines, string(b))
		_ = i
	}

	// Append content-replacement entries.
	if len(contentReplacements) > 0 {
		crEntry := map[string]any{
			"type":           "content-replacement",
			"sessionId":      forkedSessionID,
			"replacements":   contentReplacements,
		}
		b, _ := json.Marshal(crEntry)
		lines = append(lines, string(b))
	}

	// Derive title.
	forkTitle := strings.TrimSpace(title)
	if forkTitle == "" {
		head := string(content)
		if len(head) > liteReadBufSize {
			head = head[:liteReadBufSize]
		}
		tail := string(content)
		if len(tail) > liteReadBufSize {
			tail = tail[len(tail)-liteReadBufSize:]
		}
		forkTitle = extractLastJSONStringField(tail, "customTitle")
		if forkTitle == "" {
			forkTitle = extractLastJSONStringField(head, "customTitle")
		}
		if forkTitle == "" {
			forkTitle = extractLastJSONStringField(tail, "aiTitle")
		}
		if forkTitle == "" {
			forkTitle = extractLastJSONStringField(head, "aiTitle")
		}
		if forkTitle == "" {
			forkTitle = extractFirstPromptFromHead(head)
		}
		if forkTitle == "" {
			forkTitle = "Forked session"
		}
		forkTitle += " (fork)"
	}

	titleEntry := fmt.Sprintf(
		`{"type":"custom-title","sessionId":%q,"customTitle":%q}`,
		forkedSessionID, forkTitle,
	)
	lines = append(lines, titleEntry)

	// Write new file with O_EXCL.
	forkPath := filepath.Join(projectDir, forkedSessionID+".jsonl")
	fd, err := os.OpenFile(forkPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to create fork file: %w", err)
	}
	defer func() { _ = fd.Close() }()
	_, err = fd.WriteString(strings.Join(lines, "\n") + "\n")
	if err != nil {
		return nil, fmt.Errorf("failed to write fork file: %w", err)
	}

	return &ForkSessionResult{SessionID: forkedSessionID}, nil
}

// -----------------------------------------------------------------------
// Internal helpers
// -----------------------------------------------------------------------

func findSessionFile(sessionID, directory string) string {
	path, _ := findSessionFileWithDir(sessionID, directory)
	return path
}

func findSessionFileWithDir(sessionID, directory string) (string, string) {
	fileName := sessionID + ".jsonl"

	tryDir := func(projectDir string) (string, string) {
		fp := filepath.Join(projectDir, fileName)
		if fi, err := os.Stat(fp); err == nil && fi.Size() > 0 {
			return fp, projectDir
		}
		return "", ""
	}

	if directory != "" {
		canonDir, err := filepath.EvalSymlinks(directory)
		if err != nil {
			canonDir = directory
		}
		canonDir = normalizeUnicode(canonDir)

		if projectDir := findProjectDir(canonDir); projectDir != "" {
			if fp, pd := tryDir(projectDir); fp != "" {
				return fp, pd
			}
		}
		for _, wt := range getWorktreePaths(canonDir) {
			if wt == canonDir {
				continue
			}
			if projectDir := findProjectDir(wt); projectDir != "" {
				if fp, pd := tryDir(projectDir); fp != "" {
					return fp, pd
				}
			}
		}
		return "", ""
	}

	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return "", ""
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if fp, pd := tryDir(filepath.Join(projectsDir, e.Name())); fp != "" {
			return fp, pd
		}
	}
	return "", ""
}

func appendToSession(sessionID, data, directory string) error {
	fileName := sessionID + ".jsonl"

	tryAppend := func(dir string) bool {
		fp := filepath.Join(dir, fileName)
		f, err := os.OpenFile(fp, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			return false
		}
		defer func() { _ = f.Close() }()
		if fi, err := f.Stat(); err != nil || fi.Size() == 0 {
			return false
		}
		_, err = f.WriteString(data)
		return err == nil
	}

	if directory != "" {
		canonDir, err := filepath.EvalSymlinks(directory)
		if err != nil {
			canonDir = directory
		}
		canonDir = normalizeUnicode(canonDir)

		if projectDir := findProjectDir(canonDir); projectDir != "" && tryAppend(projectDir) {
			return nil
		}
		for _, wt := range getWorktreePaths(canonDir) {
			if wt == canonDir {
				continue
			}
			if projectDir := findProjectDir(wt); projectDir != "" && tryAppend(projectDir) {
				return nil
			}
		}
		return fmt.Errorf("session %s not found in project directory for %s", sessionID, directory)
	}

	projectsDir := getProjectsDir()
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return fmt.Errorf("session %s not found", sessionID)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if tryAppend(filepath.Join(projectsDir, e.Name())) {
			return nil
		}
	}
	return fmt.Errorf("session %s not found in any project directory", sessionID)
}

// -----------------------------------------------------------------------
// Unicode sanitization
// -----------------------------------------------------------------------

var unicodeStripRe = regexp.MustCompile(
	"[" +
		"\u200b-\u200f" + // Zero-width spaces, LTR/RTL marks
		"\u202a-\u202e" + // Directional formatting characters
		"\u2066-\u2069" + // Directional isolates
		"\ufeff" + // Byte order mark
		"\ue000-\uf8ff" + // BMP private use
		"]",
)

var formatCategories = map[string]bool{"Cf": true, "Co": true, "Cn": true}

func sanitizeUnicode(value string) string {
	current := value
	for i := 0; i < 10; i++ {
		previous := current
		// NFC normalization.
		current = norm.NFC.String(current)
		// Strip Cf/Co/Cn category characters.
		var b strings.Builder
		for _, r := range current {
			cat := getCategory(r)
			if !formatCategories[cat] {
				b.WriteRune(r)
			}
		}
		current = b.String()
		// Explicit ranges.
		current = unicodeStripRe.ReplaceAllString(current, "")
		if current == previous {
			break
		}
	}
	return current
}

func getCategory(r rune) string {
	// Go's unicode package doesn't expose category directly as a string,
	// but we can use unicode.Is() with specific ranges.
	// For practical purposes, we check the specific categories we care about.
	if isFormatRune(r) {
		return "Cf"
	}
	if isPrivateUse(r) {
		return "Co"
	}
	if isUnassigned(r) {
		return "Cn"
	}
	return "L" // Default to letter (safe).
}

func isFormatRune(r rune) bool {
	// Cf category: format characters.
	return r == '\ufeff' || // BOM
		(r >= '\u200b' && r <= '\u200f') || // zero-width + marks
		(r >= '\u202a' && r <= '\u202e') || // directional formatting
		(r >= '\u2066' && r <= '\u2069') || // directional isolates
		(r >= '\ufff9' && r <= '\ufffb') // interlinear
}

func isPrivateUse(r rune) bool {
	return (r >= '\uE000' && r <= '\uF8FF') ||
		(r >= 0xF0000 && r <= 0xFFFFF) ||
		(r >= 0x100000 && r <= 0x10FFFF)
}

func isUnassigned(r rune) bool {
	// Cn category — simplified check for common unassigned ranges.
	// Most unassigned codepoints are in supplementary planes.
	return false // Conservative: don't strip unassigned, just format/private-use.
}

// -----------------------------------------------------------------------
// Fork transcript parsing
// -----------------------------------------------------------------------

var transcriptTypes = map[string]bool{
	"user": true, "assistant": true, "attachment": true, "system": true, "progress": true,
}

func parseForkTranscript(content, sessionID string) ([]map[string]any, []any) {
	var transcript []map[string]any
	var contentReplacements []any

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entryType := strVal(entry, "type")
		if transcriptTypes[entryType] && strVal(entry, "uuid") != "" {
			transcript = append(transcript, entry)
		} else if entryType == "content-replacement" &&
			strVal(entry, "sessionId") == sessionID {
			if repl, ok := entry["replacements"].([]any); ok {
				contentReplacements = append(contentReplacements, repl...)
			}
		}
	}
	return transcript, contentReplacements
}
