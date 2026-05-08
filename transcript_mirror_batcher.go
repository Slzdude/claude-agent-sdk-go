package claude

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// TranscriptMirrorBatcher constants — matching Python SDK exactly.
const (
	MirrorMaxPendingEntries = 500
	MirrorMaxPendingBytes   = 1 << 20 // 1 MiB
	MirrorSendTimeout       = 60 * time.Second
	MirrorAppendMaxAttempts = 3
)

// MirrorAppendBackoff is the backoff between retry attempts.
var MirrorAppendBackoff = []time.Duration{200 * time.Millisecond, 800 * time.Millisecond}

// mirrorEntry is a buffered transcript_mirror frame.
type mirrorEntry struct {
	filePath string
	entries  []SessionStoreEntry
	bytes    int
}

// TranscriptMirrorBatcher accumulates transcript_mirror frames from the CLI
// subprocess stdout and flushes them to a SessionStore. It matches the Python
// SDK's TranscriptMirrorBatcher behavior exactly:
//   - enqueue is fire-and-forget
//   - flush is triggered on "result" messages or when thresholds are exceeded
//   - coalesces entries by file_path per flush
//   - retries on transient failures (3 attempts, backoff 0.2s/0.8s)
//   - timeout per append call (60s)
//   - errors are reported via on_error callback, never raised
type TranscriptMirrorBatcher struct {
	store       SessionStore
	projectsDir string
	onError     func(key *SessionKey, errMsg string)

	maxPendingEntries int
	maxPendingBytes   int

	mu             sync.Mutex
	pending        []mirrorEntry
	pendingEntries int
	pendingBytes   int

	flushMu sync.Mutex // serializes flush operations
}

// NewTranscriptMirrorBatcher creates a new batcher.
// projectsDir is the base directory for resolving file paths to SessionKeys.
// onError is called when store.Append fails after all retries.
// flushMode controls when entries are flushed: "batched" (default) coalesces
// and flushes per turn or threshold; "eager" flushes after every frame.
func NewTranscriptMirrorBatcher(store SessionStore, projectsDir string, onError func(key *SessionKey, errMsg string), flushMode SessionStoreFlushMode) *TranscriptMirrorBatcher {
	maxEntries := MirrorMaxPendingEntries
	maxBytes := MirrorMaxPendingBytes
	if flushMode == FlushModeEager {
		maxEntries = 0
		maxBytes = 0
	}
	return &TranscriptMirrorBatcher{
		store:             store,
		projectsDir:       projectsDir,
		onError:           onError,
		maxPendingEntries: maxEntries,
		maxPendingBytes:   maxBytes,
	}
}

// Enqueue buffers a transcript_mirror frame. If thresholds are exceeded,
// triggers an eager background flush. Fire-and-forget — never blocks.
func (b *TranscriptMirrorBatcher) Enqueue(filePath string, entries []SessionStoreEntry) {
	size := estimateEntriesSize(entries)

	b.mu.Lock()
	b.pending = append(b.pending, mirrorEntry{filePath: filePath, entries: entries, bytes: size})
	b.pendingEntries += len(entries)
	b.pendingBytes += size
	shouldFlush := b.pendingEntries > b.maxPendingEntries || b.pendingBytes > b.maxPendingBytes
	b.mu.Unlock()

	if shouldFlush {
		go b.drain()
	}
}

// Flush flushes all pending entries to the store. Called before "result"
// messages to ensure the store is up-to-date before the consumer sees results.
func (b *TranscriptMirrorBatcher) Flush() {
	b.flushMu.Lock()
	errors := b.drainLocked()
	b.flushMu.Unlock()
	b.reportErrors(errors)
}

// Close performs a final flush. Never panics — matches Python's
// close() which catches all exceptions.
func (b *TranscriptMirrorBatcher) Close() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[TranscriptMirrorBatcher] close flush failed: %v", r)
		}
	}()
	b.flushMu.Lock()
	errors := b.drainLocked()
	b.flushMu.Unlock()
	b.reportErrors(errors)
}

// drain detaches the pending buffer and flushes it. Called from eager
// background goroutines. Serialized against Flush via flushMu.
func (b *TranscriptMirrorBatcher) drain() {
	b.flushMu.Lock()
	errors := b.drainLocked()
	b.flushMu.Unlock()
	b.reportErrors(errors)
}

// drainLocked is the internal flush implementation. Caller must hold flushMu.
// Returns errors to report; caller MUST call reportErrors AFTER releasing flushMu.
// This matches Python's _drain which releases async lock BEFORE calling on_error,
// preventing deadlock if the callback calls Flush() again.
func (b *TranscriptMirrorBatcher) drainLocked() []mirrorError {
	b.mu.Lock()
	items := b.pending
	b.pending = nil
	b.pendingEntries = 0
	b.pendingBytes = 0
	b.mu.Unlock()

	if len(items) == 0 {
		return nil
	}

	return b.doFlush(items)
}

// reportErrors calls the onError callback for each failed append.
// Must be called AFTER flushMu has been released to avoid deadlock
// if the callback itself calls Flush(). Matches Python's _drain pattern.
func (b *TranscriptMirrorBatcher) reportErrors(errors []mirrorError) {
	for _, e := range errors {
		if b.onError != nil {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[TranscriptMirrorBatcher] on_error callback raised: %v", r)
					}
				}()
				b.onError(e.key, e.msg)
			}()
		}
	}
}

// doFlush coalesces entries by file_path and sends each batch to the store.
// Returns a list of (key, error) pairs for failures. Errors are reported
// AFTER releasing the lock so a slow onError callback cannot block subsequent
// drains — matching Python SDK's _do_flush + _drain exactly.
func (b *TranscriptMirrorBatcher) doFlush(items []mirrorEntry) []mirrorError {
	// Coalesce by file_path — dict preserves first-seen order in Python;
	// Go map order is random but each path is independent so this is fine.
	byPath := make(map[string][]SessionStoreEntry)
	for _, item := range items {
		byPath[item.filePath] = append(byPath[item.filePath], item.entries...)
	}

	var errors []mirrorError
	for filePath, entries := range byPath {
		if len(entries) == 0 {
			// Avoid creating phantom keys in adapters that touch storage
			// on append([]) — nothing to write.
			continue
		}
		key := FilePathToSessionKey(filePath, b.projectsDir)
		if key == nil {
			log.Printf(
				"[SessionStore] dropping mirror frame: filePath %s is not "+
					"under %s -- subprocess CLAUDE_CONFIG_DIR likely differs "+
					"from parent (custom env / container?)",
				filePath, b.projectsDir,
			)
			continue
		}

		var lastErr error
		succeeded := false
		timedOut := false
		for attempt := 0; attempt < MirrorAppendMaxAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(MirrorAppendBackoff[attempt-1])
			}

			// Run store.Append in a goroutine with timeout via select.
			// This matches Python's asyncio.wait_for() which cancels the
			// coroutine on timeout.
			type appendResult struct {
				err error
			}
			ch := make(chan appendResult, 1)
			go func() {
				ch <- appendResult{err: b.store.Append(*key, entries)}
			}()

			select {
			case r := <-ch:
				if r.err == nil {
					succeeded = true
				} else {
					lastErr = r.err
					// Log each attempt failure at debug level (matching Python).
					log.Printf(
						"[TranscriptMirrorBatcher] append attempt %d/%d failed for %s: %v",
						attempt+1, MirrorAppendMaxAttempts, filePath, r.err,
					)
				}
			case <-time.After(MirrorSendTimeout):
				// Don't retry on timeout: the in-flight call may still land
				// — a retry would launch a concurrent duplicate.
				// Also keeps worst-case lock hold at ~send_timeout rather
				// than ~3×send_timeout + backoff.
				lastErr = fmt.Errorf("store.append timed out after %s", MirrorSendTimeout)
				timedOut = true
				log.Printf(
					"[TranscriptMirrorBatcher] append timed out after %.1fs for %s — not retrying",
					MirrorSendTimeout.Seconds(), filePath,
				)
			}
			if succeeded || timedOut {
				break
			}
		}

		if !succeeded {
			log.Printf("[TranscriptMirrorBatcher] flush failed for %s: %v", filePath, lastErr)
			errors = append(errors, mirrorError{key: key, msg: fmt.Sprintf("%v", lastErr)})
		}
	}
	return errors
}

// mirrorError holds a failed append's key and error message for deferred reporting.
type mirrorError struct {
	key *SessionKey
	msg string
}

// FilePathToSessionKey derives a SessionKey from an absolute transcript file path.
// Main transcripts: <projects_dir>/<project_key>/<session_id>.jsonl
// Subagent transcripts: <projects_dir>/<project_key>/<session_id>/subagents/.../agent-<id>.jsonl
// Returns nil if file_path is not under projects_dir or has unrecognized shape.
func FilePathToSessionKey(filePath, projectsDir string) *SessionKey {
	rel, err := filepath.Rel(projectsDir, filePath)
	if err != nil {
		return nil
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 || parts[0] == ".." || filepath.IsAbs(rel) {
		return nil
	}
	if len(parts) < 2 {
		return nil
	}

	projectKey := parts[0]
	second := parts[1]

	// Main transcript: <project_key>/<session_id>.jsonl
	if len(parts) == 2 && strings.HasSuffix(second, ".jsonl") {
		sessID := strings.TrimSuffix(second, ".jsonl")
		return &SessionKey{ProjectKey: projectKey, SessionID: sessID}
	}

	// Subagent transcript: <project_key>/<session_id>/subagents/.../agent-<id>.jsonl
	if len(parts) >= 4 {
		subpathParts := parts[2:]
		last := subpathParts[len(subpathParts)-1]
		if strings.HasSuffix(last, ".jsonl") {
			subpathParts[len(subpathParts)-1] = strings.TrimSuffix(last, ".jsonl")
		}
		return &SessionKey{
			ProjectKey: projectKey,
			SessionID:  second,
			Subpath:    strings.Join(subpathParts, "/"),
		}
	}

	return nil
}

// estimateEntriesSize approximates the wire size of entries.
// Matches Python's len(json.dumps(entries)) — one stringify per frame.
func estimateEntriesSize(entries []SessionStoreEntry) int {
	b, _ := json.Marshal(entries)
	return len(b)
}
