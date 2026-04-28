// Package sessionstores provides reference SessionStore adapters for S3, Redis,
// and Postgres.
//
// This file implements a Postgres-backed SessionStore using pgx v5.
//
// Schema (one row per transcript entry; seq orders entries within a key):
//
//	CREATE TABLE IF NOT EXISTS claude_session_store (
//	  project_key text   NOT NULL,
//	  session_id  text   NOT NULL,
//	  subpath     text   NOT NULL DEFAULT '',
//	  seq         bigserial,
//	  entry       jsonb  NOT NULL,
//	  mtime       bigint NOT NULL,
//	  PRIMARY KEY (project_key, session_id, subpath, seq)
//	);
//	CREATE INDEX IF NOT EXISTS claude_session_store_list_idx
//	  ON claude_session_store (project_key, session_id) WHERE subpath = '';
//
// The empty string is the subpath sentinel for the main transcript so the
// composite primary key is total (Postgres treats NULL as distinct in PKs).
//
// JSONB key ordering: entries are stored as jsonb, which reorders object keys
// on read-back. This is explicitly allowed by the SessionStore contract:
// Load() requires deep-equal, not byte-equal, returns.
//
// Requires pgx v5:
//
//	go get github.com/jackc/pgx/v5
//
// Usage:
//
//	pool, _ := pgxpool.New(context.TODO(), "postgresql://...")
//	store := sessionstores.NewPostgresSessionStore(pool, "claude_session_store")
//	store.CreateSchema(context.TODO()) // one-time, idempotent
//
//	msgs, _ := claude.Query(ctx, "Hello!", &claude.ClaudeAgentOptions{
//	    SessionStore: store,
//	})
//
// Retention: this adapter never deletes rows on its own. Add a scheduled
// DELETE ... WHERE mtime < $cutoff (or table partitioning by mtime) to expire
// transcripts according to your compliance requirements.
package sessionstores

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// identRe is a conservative identifier guard for the table name.
// The name is interpolated into DDL/DML (pgx cannot parameterize identifiers),
// so reject anything that isn't a plain [A-Za-z_][A-Za-z0-9_]*.
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PostgresSessionStore implements claude.SessionStore backed by Postgres.
type PostgresSessionStore struct {
	pool  *pgxpool.Pool
	table string
}

// NewPostgresSessionStore creates a new Postgres-backed SessionStore.
// The table name must match [A-Za-z_][A-Za-z0-9_]* — it is interpolated
// directly into SQL (identifiers cannot be parameterized).
func NewPostgresSessionStore(pool *pgxpool.Pool, table string) (*PostgresSessionStore, error) {
	if pool == nil {
		return nil, fmt.Errorf("PostgresSessionStore requires a non-nil pool")
	}
	if table == "" {
		table = "claude_session_store"
	}
	if !identRe.MatchString(table) {
		return nil, fmt.Errorf("table %q must match [A-Za-z_][A-Za-z0-9_]*", table)
	}
	return &PostgresSessionStore{pool: pool, table: table}, nil
}

// CreateSchema creates the table and listing index if absent. Idempotent.
// Call once at startup (or run the equivalent migration out-of-band).
// The partial index on subpath = '' keeps ListSessions cheap without
// indexing every subagent row.
func (s *PostgresSessionStore) CreateSchema(ctx context.Context) error {
	ddl := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (
			project_key text   NOT NULL,
			session_id  text   NOT NULL,
			subpath     text   NOT NULL DEFAULT '',
			seq         bigserial,
			entry       jsonb  NOT NULL,
			mtime       bigint NOT NULL,
			PRIMARY KEY (project_key, session_id, subpath, seq)
		);
		CREATE INDEX IF NOT EXISTS %s_list_idx
			ON %s (project_key, session_id) WHERE subpath = '';
	`, s.table, s.table, s.table)
	_, err := s.pool.Exec(ctx, ddl)
	return err
}

// Append inserts a batch of transcript entries in a single multi-row INSERT.
func (s *PostgresSessionStore) Append(key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}
	subpath := key.Subpath

	// Build JSONB array for unnest.
	jsonbEntries := make([]string, len(entries))
	for i, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		jsonbEntries[i] = string(b)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (project_key, session_id, subpath, entry, mtime)
		SELECT $1, $2, $3, e,
		       (EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint
		FROM unnest($4::jsonb[]) WITH ORDINALITY AS t(e, ord)
		ORDER BY ord
	`, s.table)

	_, err := s.pool.Exec(context.Background(), query,
		key.ProjectKey,
		key.SessionID,
		subpath,
		jsonbEntries,
	)
	return err
}

// Load retrieves all entries for a session ordered by seq.
// Returns nil if no entries found.
func (s *PostgresSessionStore) Load(key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	subpath := key.Subpath

	query := fmt.Sprintf(`
		SELECT entry FROM %s
		WHERE project_key = $1 AND session_id = $2 AND subpath = $3
		ORDER BY seq
	`, s.table)

	rows, err := s.pool.Query(context.Background(), query,
		key.ProjectKey,
		key.SessionID,
		subpath,
	)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var result []claude.SessionStoreEntry
	for rows.Next() {
		var entryJSON []byte
		if err := rows.Scan(&entryJSON); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		var entry claude.SessionStoreEntry
		if err := json.Unmarshal(entryJSON, &entry); err != nil {
			continue // skip malformed entries
		}
		result = append(result, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	if len(result) == 0 {
		return nil, nil
	}
	return result, nil
}

// ListSessions returns all sessions for a project with their latest mtime.
func (s *PostgresSessionStore) ListSessions(projectKey string) ([]claude.SessionStoreListEntry, error) {
	query := fmt.Sprintf(`
		SELECT session_id, MAX(mtime) AS mtime FROM %s
		WHERE project_key = $1 AND subpath = ''
		GROUP BY session_id
	`, s.table)

	rows, err := s.pool.Query(context.Background(), query, projectKey)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var result []claude.SessionStoreListEntry
	for rows.Next() {
		var entry claude.SessionStoreListEntry
		if err := rows.Scan(&entry.SessionID, &entry.Mtime); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result = append(result, entry)
	}
	return result, rows.Err()
}

// ListSessionSummaries is not implemented for Postgres (returns
// ErrNotImplemented). Use ListSessions + per-session Load instead.
func (s *PostgresSessionStore) ListSessionSummaries(projectKey string) ([]claude.SessionSummaryEntry, error) {
	return nil, fmt.Errorf("ListSessionSummaries not implemented: %w", ErrNotImplemented)
}

// Delete removes session data. If a subpath is set, only that subpath's rows
// are removed. Otherwise, cascades to all subpaths under the session.
func (s *PostgresSessionStore) Delete(key claude.SessionKey) error {
	if key.Subpath != "" {
		query := fmt.Sprintf(`
			DELETE FROM %s
			WHERE project_key = $1 AND session_id = $2 AND subpath = $3
		`, s.table)
		_, err := s.pool.Exec(context.Background(), query,
			key.ProjectKey, key.SessionID, key.Subpath,
		)
		return err
	}

	// Cascade: main + every subpath under this (project_key, session_id).
	query := fmt.Sprintf(`
		DELETE FROM %s
		WHERE project_key = $1 AND session_id = $2
	`, s.table)
	_, err := s.pool.Exec(context.Background(), query,
		key.ProjectKey, key.SessionID,
	)
	return err
}

// ListSubkeys returns distinct subpath keys under a session.
func (s *PostgresSessionStore) ListSubkeys(projectKey, sessionID string) ([]string, error) {
	query := fmt.Sprintf(`
		SELECT DISTINCT subpath FROM %s
		WHERE project_key = $1 AND session_id = $2 AND subpath <> ''
	`, s.table)

	rows, err := s.pool.Query(context.Background(), query, projectKey, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var subpath string
		if err := rows.Scan(&subpath); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		result = append(result, subpath)
	}
	return result, rows.Err()
}

// PostgresSessionStoreFromConn creates a PostgresSessionStore from a single
// connection (for testing or simple use cases). The pool is created internally.
func PostgresSessionStoreFromConn(ctx context.Context, connString string, table string) (*PostgresSessionStore, error) {
	pool, err := pgxpool.New(ctx, connString)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	return NewPostgresSessionStore(pool, table)
}

// PostgresSessionStoreFromPool creates a PostgresSessionStore from an existing
// pgxpool.Pool with a custom table name.
func PostgresSessionStoreFromPool(pool *pgxpool.Pool, table string) (*PostgresSessionStore, error) {
	return NewPostgresSessionStore(pool, table)
}

// PostgresBatchSize is the max entries per INSERT in BatchInsert.
// For very large imports, this bounds memory usage.
const PostgresBatchSize = 500

// BatchInsert inserts entries in batches. Useful for initial data migration.
func (s *PostgresSessionStore) BatchInsert(ctx context.Context, key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	for i := 0; i < len(entries); i += PostgresBatchSize {
		end := i + PostgresBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]
		if err := s.Append(key, batch); err != nil {
			return fmt.Errorf("batch insert at offset %d: %w", i, err)
		}
	}
	return nil
}

// PostgresTxSessionStore wraps a PostgresSessionStore to operate within a
// transaction. Useful for atomic multi-session operations.
type PostgresTxSessionStore struct {
	store *PostgresSessionStore
	tx    pgx.Tx
}

// WithTx creates a transactional wrapper. Call tx.Commit() or tx.Rollback()
// on the underlying transaction when done.
func (s *PostgresSessionStore) WithTx(tx pgx.Tx) *PostgresTxSessionStore {
	return &PostgresTxSessionStore{store: s, tx: tx}
}

// Append inserts entries within the transaction.
func (t *PostgresTxSessionStore) Append(key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}
	subpath := key.Subpath

	jsonbEntries := make([]string, len(entries))
	for i, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		jsonbEntries[i] = string(b)
	}

	query := fmt.Sprintf(`
		INSERT INTO %s (project_key, session_id, subpath, entry, mtime)
		SELECT $1, $2, $3, e,
		       (EXTRACT(EPOCH FROM clock_timestamp()) * 1000)::bigint
		FROM unnest($4::jsonb[]) WITH ORDINALITY AS t(e, ord)
		ORDER BY ord
	`, t.store.table)

	_, err := t.tx.Exec(context.Background(), query,
		key.ProjectKey, key.SessionID, subpath, jsonbEntries,
	)
	return err
}

// Now returns the current time as epoch milliseconds. Utility for mtime
// calculations in adapters.
func Now() int64 {
	return time.Now().UnixMilli()
}
