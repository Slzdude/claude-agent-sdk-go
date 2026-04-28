// Package sessionstores provides reference SessionStore adapters for S3, Redis,
// and Postgres.
//
// This file implements a Redis-backed SessionStore using go-redis v9.
//
// Key scheme (":" separator; project_key/session_id are opaque):
//
//	{prefix}:{project_key}:{session_id}             list   main transcript (JSON each)
//	{prefix}:{project_key}:{session_id}:{subpath}   list   subagent transcript
//	{prefix}:{project_key}:{session_id}:__subkeys   set    subpaths under this session
//	{prefix}:{project_key}:__sessions               zset   session_id -> mtime(ms)
//
// Requires go-redis v9:
//
//	go get github.com/redis/go-redis/v9
//
// Usage:
//
//	rdb := redis.NewClient(&redis.Options{
//	    Addr:     "localhost:6379",
//	})
//	store := sessionstores.NewRedisSessionStore(rdb, "transcripts")
//
//	msgs, _ := claude.Query(ctx, "Hello!", &claude.ClaudeAgentOptions{
//	    SessionStore: store,
//	})
//
// Retention: this adapter never expires keys on its own. Configure Redis key
// expiration on your prefix or call Delete() according to your compliance
// requirements.
package sessionstores

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

const (
	// subkeysSentinel is the reserved subpath for the per-session subkey set.
	subkeysSentinel = "__subkeys"
	// sessionsSentinel is the reserved session_id for the per-project session index.
	sessionsSentinel = "__sessions"
)

// RedisSessionStore implements claude.SessionStore backed by Redis.
type RedisSessionStore struct {
	client *redis.Client
	prefix string
	ctx    context.Context
}

// NewRedisSessionStore creates a new Redis-backed SessionStore.
// The prefix is normalized: non-empty values always end in exactly one ":".
func NewRedisSessionStore(client *redis.Client, prefix string) *RedisSessionStore {
	if prefix != "" {
		prefix = strings.TrimRight(prefix, ":") + ":"
	}
	return &RedisSessionStore{
		client: client,
		prefix: prefix,
		ctx:    context.Background(),
	}
}

// WithContext sets the base context for all Redis operations.
func (s *RedisSessionStore) WithContext(ctx context.Context) *RedisSessionStore {
	s.ctx = ctx
	return s
}

// entryKey returns the Redis key for a transcript list (main or subpath).
func (s *RedisSessionStore) entryKey(key claude.SessionKey) string {
	parts := []string{key.ProjectKey, key.SessionID}
	if key.Subpath != "" {
		parts = append(parts, key.Subpath)
	}
	return s.prefix + strings.Join(parts, ":")
}

// subkeysKey returns the Redis key for the per-session subpath set.
func (s *RedisSessionStore) subkeysKey(projectKey, sessionID string) string {
	return fmt.Sprintf("%s%s:%s:%s", s.prefix, projectKey, sessionID, subkeysSentinel)
}

// sessionsKey returns the Redis key for the per-project session index.
func (s *RedisSessionStore) sessionsKey(projectKey string) string {
	return s.prefix + projectKey + ":" + sessionsSentinel
}

// Append adds entries to the session's Redis list and updates indexes.
// Uses a MULTI/exec pipeline for atomicity.
func (s *RedisSessionStore) Append(key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}

	pipe := s.client.Pipeline()

	// RPUSH all entries.
	values := make([]interface{}, len(entries))
	for i, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		values[i] = string(b)
	}
	pipe.RPush(s.ctx, s.entryKey(key), values...)

	// Update indexes.
	if key.Subpath != "" {
		pipe.SAdd(s.ctx, s.subkeysKey(key.ProjectKey, key.SessionID), key.Subpath)
	} else {
		// Only main-transcript appends bump the session index.
		pipe.ZAdd(s.ctx, s.sessionsKey(key.ProjectKey), redis.Z{
			Score:  float64(time.Now().UnixMilli()),
			Member: key.SessionID,
		})
	}

	_, err := pipe.Exec(s.ctx)
	return err
}

// Load retrieves all entries for a session from the Redis list.
// Returns nil if the key does not exist.
func (s *RedisSessionStore) Load(key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	raw, err := s.client.LRange(s.ctx, s.entryKey(key), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("lrange: %w", err)
	}
	if len(raw) == 0 {
		return nil, nil
	}

	var result []claude.SessionStoreEntry
	for _, line := range raw {
		var entry claude.SessionStoreEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue // skip malformed entries
		}
		result = append(result, entry)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// ListSessions returns all sessions for a project from the sorted set index.
func (s *RedisSessionStore) ListSessions(projectKey string) ([]claude.SessionStoreListEntry, error) {
	pairs, err := s.client.ZRangeWithScores(s.ctx, s.sessionsKey(projectKey), 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("zrange: %w", err)
	}

	result := make([]claude.SessionStoreListEntry, 0, len(pairs))
	for _, pair := range pairs {
		sessionID, ok := pair.Member.(string)
		if !ok {
			continue
		}
		result = append(result, claude.SessionStoreListEntry{
			SessionID: sessionID,
			Mtime:     int64(pair.Score),
		})
	}
	return result, nil
}

// ListSessionSummaries is not implemented for Redis (returns
// ErrNotImplemented). Use ListSessions + per-session Load instead.
func (s *RedisSessionStore) ListSessionSummaries(projectKey string) ([]claude.SessionSummaryEntry, error) {
	return nil, fmt.Errorf("ListSessionSummaries not implemented: %w", ErrNotImplemented)
}

// Delete removes the session's data. If a subpath is set, only that subpath
// is removed. Otherwise, cascades to all subpaths.
func (s *RedisSessionStore) Delete(key claude.SessionKey) error {
	if key.Subpath != "" {
		// Targeted: remove just this subpath list and its index entry.
		pipe := s.client.Pipeline()
		pipe.Del(s.ctx, s.entryKey(key))
		pipe.SRem(s.ctx, s.subkeysKey(key.ProjectKey, key.SessionID), key.Subpath)
		_, err := pipe.Exec(s.ctx)
		return err
	}

	// Cascade: main list + every subpath list + subkey set + session-index entry.
	subkeys, err := s.client.SMembers(s.ctx, s.subkeysKey(key.ProjectKey, key.SessionID)).Result()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("smembers: %w", err)
	}

	toDelete := []string{
		s.entryKey(key),
		s.subkeysKey(key.ProjectKey, key.SessionID),
	}
	for _, sp := range subkeys {
		toDelete = append(toDelete, s.entryKey(claude.SessionKey{
			ProjectKey: key.ProjectKey,
			SessionID:  key.SessionID,
			Subpath:    sp,
		}))
	}

	pipe := s.client.Pipeline()
	pipe.Del(s.ctx, toDelete...)
	pipe.ZRem(s.ctx, s.sessionsKey(key.ProjectKey), key.SessionID)
	_, err = pipe.Exec(s.ctx)
	return err
}

// ListSubkeys returns the subpath keys under a session.
func (s *RedisSessionStore) ListSubkeys(projectKey, sessionID string) ([]string, error) {
	result, err := s.client.SMembers(s.ctx, s.subkeysKey(projectKey, sessionID)).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("smembers: %w", err)
	}
	return result, nil
}

// RedisSessionIDFromScore extracts a session ID from a Redis sorted set score.
// Utility for custom queries.
func RedisSessionIDFromScore(score float64) string {
	return strconv.FormatFloat(score, 'f', 0, 64)
}
