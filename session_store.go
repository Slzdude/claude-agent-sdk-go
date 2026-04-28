package claude

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemorySessionStore is a reference implementation of SessionStore for testing.
type InMemorySessionStore struct {
	mu        sync.RWMutex
	data      map[string][]SessionStoreEntry
	summaries map[string]*SessionSummaryEntry
	mtimes    map[string]int64 // per-key storage write time (epoch ms)
	lastMtime int64            // strictly-monotonic counter (protected by mu)
}

// NewInMemorySessionStore creates a new in-memory session store.
func NewInMemorySessionStore() *InMemorySessionStore {
	return &InMemorySessionStore{
		data:      make(map[string][]SessionStoreEntry),
		summaries: make(map[string]*SessionSummaryEntry),
		mtimes:    make(map[string]int64),
	}
}

// nextMtime returns a strictly-monotonically-increasing timestamp in epoch ms.
// Matches Python's InMemorySessionStore._next_mtime() guarantee.
func (s *InMemorySessionStore) nextMtime() int64 {
	now := time.Now().UnixMilli()
	if now <= s.lastMtime {
		now = s.lastMtime + 1
	}
	s.lastMtime = now
	return now
}

func (s *InMemorySessionStore) keyStr(key SessionKey) string {
	parts := []string{key.ProjectKey, key.SessionID}
	if key.Subpath != "" {
		parts = append(parts, key.Subpath)
	}
	return strings.Join(parts, "/")
}

func (s *InMemorySessionStore) Append(key SessionKey, entries []SessionStoreEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.keyStr(key)
	s.data[k] = append(s.data[k], entries...)
	// Use strictly-monotonic mtime — same clock for both the list entry and
	// the summary sidecar (matching Python's InMemorySessionStore._next_mtime).
	nowMs := s.nextMtime()
	s.mtimes[k] = nowMs
	// Update summary for main transcripts (no subpath).
	if key.Subpath == "" {
		s.summaries[key.SessionID] = FoldSessionSummary(
			s.summaries[key.SessionID], key, entries,
		)
		// Stamp the sidecar with the same clock used for list_sessions so the
		// fast-path staleness check (summary.mtime vs list_sessions mtime) works.
		s.summaries[key.SessionID].Mtime = nowMs
	}
	return nil
}

func (s *InMemorySessionStore) Load(key SessionKey) ([]SessionStoreEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k := s.keyStr(key)
	entries := s.data[k]
	if entries == nil {
		return nil, nil
	}
	// Return a copy.
	result := make([]SessionStoreEntry, len(entries))
	copy(result, entries)
	return result, nil
}

func (s *InMemorySessionStore) ListSessions(projectKey string) ([]SessionStoreListEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []SessionStoreListEntry
	prefix := projectKey + "/"
	for k := range s.data {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		// Skip subpath entries.
		if strings.Contains(rest, "/") {
			continue
		}
		sid := rest
		// Return the stored mtime from when the session was last appended,
		// matching Python which stores _mtimes[k] at append time and returns
		// it from list_sessions(). Using time.Now() here would be wrong:
		// all sessions would get the same mtime, defeating sort order.
		mtime := s.mtimes[k] // 0 if never appended (shouldn't happen)
		result = append(result, SessionStoreListEntry{SessionID: sid, Mtime: mtime})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Mtime > result[j].Mtime
	})
	return result, nil
}

func (s *InMemorySessionStore) ListSessionSummaries(projectKey string) ([]SessionSummaryEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []SessionSummaryEntry
	for sid, summary := range s.summaries {
		k := projectKey + "/" + sid
		if _, exists := s.data[k]; exists {
			result = append(result, *summary)
		}
	}
	return result, nil
}

func (s *InMemorySessionStore) Delete(key SessionKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := s.keyStr(key)
	delete(s.data, k)
	delete(s.summaries, key.SessionID)
	// Cascade: delete all subkeys.
	prefix := k + "/"
	for dk := range s.data {
		if strings.HasPrefix(dk, prefix) {
			delete(s.data, dk)
		}
	}
	return nil
}

func (s *InMemorySessionStore) ListSubkeys(projectKey, sessionID string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	prefix := fmt.Sprintf("%s/%s/", projectKey, sessionID)
	var result []string
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			subpath := strings.TrimPrefix(k, prefix)
			result = append(result, subpath)
		}
	}
	return result, nil
}
