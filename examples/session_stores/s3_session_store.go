// Package sessionstores provides reference SessionStore adapters for S3, Redis,
// and Postgres. These are reference implementations — copy them into your own
// project and adapt as needed.
//
// This file implements an S3-backed SessionStore using the AWS SDK v2.
//
// Transcripts are stored as JSONL part files:
//
//	s3://{bucket}/{prefix}{project_key}/{session_id}/part-{epochMs13}-{rand6}.jsonl
//
// Each Append() writes a new part; Load() lists, sorts, and concatenates them.
// The 13-digit zero-padded epoch-ms prefix means lexical key order ==
// chronological order. A per-instance monotonic millisecond counter orders
// same-instance same-ms appends; the random hex suffix disambiguates concurrent
// instances.
//
// Requires the AWS SDK v2:
//
//	go get github.com/aws/aws-sdk-go-v2
//	go get github.com/aws/aws-sdk-go-v2/config
//	go get github.com/aws/aws-sdk-go-v2/service/s3
//
// Usage:
//
//	cfg, _ := config.LoadDefaultConfig(context.TODO())
//	client := s3.NewFromConfig(cfg)
//	store := sessionstores.NewS3SessionStore(client, "my-claude-sessions", "transcripts")
//
//	msgs, _ := claude.Query(ctx, "Hello!", &claude.ClaudeAgentOptions{
//	    SessionStore: store,
//	})
//
// Retention: this adapter never deletes objects on its own. Configure an S3
// lifecycle policy on the bucket/prefix to expire transcripts according to your
// compliance requirements. Delete() is implemented but only invoked when you
// call DeleteSessionViaStore() from the SDK.
package sessionstores

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// S3LoadConcurrency is the max parallel GetObject calls during Load().
const S3LoadConcurrency = 16

var partMtimeRe = regexp.MustCompile(`/part-(\d{13})-[0-9a-f]{6}\.jsonl$`)

// S3SessionStore implements claude.SessionStore backed by S3.
type S3SessionStore struct {
	client   *s3.Client
	bucket   string
	prefix   string
	mu       sync.Mutex
	lastMs   int64
	ctx      context.Context
}

// NewS3SessionStore creates a new S3-backed SessionStore.
// The prefix is normalized: non-empty values always end in exactly one "/".
func NewS3SessionStore(client *s3.Client, bucket, prefix string) *S3SessionStore {
	if prefix != "" {
		prefix = strings.TrimRight(prefix, "/") + "/"
	}
	return &S3SessionStore{
		client: client,
		bucket: bucket,
		prefix: prefix,
		ctx:    context.Background(),
	}
}

// WithContext sets the base context for all S3 operations.
func (s *S3SessionStore) WithContext(ctx context.Context) *S3SessionStore {
	s.ctx = ctx
	return s
}

// keyPrefix returns the S3 key prefix for a session (or subpath). Always ends in "/".
func (s *S3SessionStore) keyPrefix(key claude.SessionKey) string {
	parts := []string{key.ProjectKey, key.SessionID}
	if key.Subpath != "" {
		parts = append(parts, key.Subpath)
	}
	return s.prefix + strings.Join(parts, "/") + "/"
}

// projectPrefix returns the S3 key prefix for a project. Always ends in "/".
func (s *S3SessionStore) projectPrefix(projectKey string) string {
	return s.prefix + projectKey + "/"
}

// nextPartName generates a part file name with monotonic epoch-ms prefix.
func (s *S3SessionStore) nextPartName() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UnixMilli()
	if now <= s.lastMs {
		now = s.lastMs + 1
	}
	s.lastMs = now

	randBytes := make([]byte, 3)
	_, _ = rand.Read(randBytes)
	return fmt.Sprintf("part-%013d-%s.jsonl", now, hex.EncodeToString(randBytes))
}

// Append writes a new JSONL part file to S3.
func (s *S3SessionStore) Append(key claude.SessionKey, entries []claude.SessionStoreEntry) error {
	if len(entries) == 0 {
		return nil
	}
	objectKey := s.keyPrefix(key) + s.nextPartName()

	var buf bytes.Buffer
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}

	_, err := s.client.PutObject(s.ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(objectKey),
		Body:        bytes.NewReader(buf.Bytes()),
		ContentType: aws.String("application/x-ndjson"),
	})
	return err
}

// Load lists all part files for a session, sorts them chronologically, and
// concatenates the entries. Returns nil if no parts found.
func (s *S3SessionStore) Load(key claude.SessionKey) ([]claude.SessionStoreEntry, error) {
	prefix := s.keyPrefix(key)

	// List part files directly under this prefix only. Without Delimiter,
	// S3 recurses into subpaths (e.g. subagents/*), so a main-transcript
	// load would mix in subagent entries.
	var keys []string
	var continuationToken *string
	for {
		out, err := s.client.ListObjectsV2(s.ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			Delimiter:         aws.String("/"),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range out.Contents {
			k := aws.ToString(obj.Key)
			// Keep only direct children (no '/' after prefix).
			if !strings.Contains(k[len(prefix):], "/") {
				keys = append(keys, k)
			}
		}
		if out.NextContinuationToken == nil {
			break
		}
		continuationToken = out.NextContinuationToken
	}

	if len(keys) == 0 {
		return nil, nil
	}

	// 13-digit epochMs prefix is fixed-width, so lexical == chronological.
	sort.Strings(keys)

	// Bounded-parallel GetObject.
	type result struct {
		idx  int
		body string
	}
	results := make([]result, len(keys))
	sem := make(chan struct{}, S3LoadConcurrency)
	var wg sync.WaitGroup

	for i, objectKey := range keys {
		wg.Add(1)
		go func(idx int, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			out, err := s.client.GetObject(s.ctx, &s3.GetObjectInput{
				Bucket: aws.String(s.bucket),
				Key:    aws.String(key),
			})
			if err != nil {
				return
			}
			defer out.Body.Close()
			data, err := io.ReadAll(out.Body)
			if err != nil {
				return
			}
			results[idx] = result{idx: idx, body: string(data)}
		}(i, objectKey)
	}
	wg.Wait()

	var allEntries []claude.SessionStoreEntry
	for _, r := range results {
		if r.body == "" {
			continue
		}
		for _, line := range strings.Split(r.body, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var entry claude.SessionStoreEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				continue // skip malformed lines
			}
			allEntries = append(allEntries, entry)
		}
	}

	if len(allEntries) == 0 {
		return nil, nil
	}
	return allEntries, nil
}

// ListSessions lists all sessions under a project key by scanning part file
// keys and deriving session IDs and mtimes from the key structure.
func (s *S3SessionStore) ListSessions(projectKey string) ([]claude.SessionStoreListEntry, error) {
	prefix := s.projectPrefix(projectKey)
	sessions := make(map[string]int64)
	var continuationToken *string

	for {
		out, err := s.client.ListObjectsV2(s.ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range out.Contents {
			k := aws.ToString(obj.Key)
			rest := k[len(prefix):]
			slash := strings.Index(rest, "/")
			if slash == -1 {
				continue
			}
			// Main-transcript parts only (one level under session_id).
			if strings.Contains(rest[slash+1:], "/") {
				continue
			}
			sessionID := rest[:slash]
			var mtime int64
			if m := partMtimeRe.FindStringSubmatch(k); m != nil {
				fmt.Sscanf(m[1], "%d", &mtime)
			} else if obj.LastModified != nil {
				mtime = obj.LastModified.UnixMilli()
			}
			if mtime > sessions[sessionID] {
				sessions[sessionID] = mtime
			}
		}
		if out.NextContinuationToken == nil {
			break
		}
		continuationToken = out.NextContinuationToken
	}

	result := make([]claude.SessionStoreListEntry, 0, len(sessions))
	for sid, mtime := range sessions {
		result = append(result, claude.SessionStoreListEntry{
			SessionID: sid,
			Mtime:     mtime,
		})
	}
	return result, nil
}

// ListSessionSummaries is not implemented for S3 (returns
// ErrNotImplemented). Use ListSessions + per-session Load instead.
func (s *S3SessionStore) ListSessionSummaries(projectKey string) ([]claude.SessionSummaryEntry, error) {
	return nil, fmt.Errorf("ListSessionSummaries not implemented: %w", ErrNotImplemented)
}

// Delete removes all objects under the session's key prefix.
// If a subpath is set, only that subpath's objects are removed.
func (s *S3SessionStore) Delete(key claude.SessionKey) error {
	prefix := s.keyPrefix(key)
	directOnly := key.Subpath != ""
	var continuationToken *string

	for {
		input := &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		}
		if directOnly {
			input.Delimiter = aws.String("/")
		}
		out, err := s.client.ListObjectsV2(s.ctx, input)
		if err != nil {
			return fmt.Errorf("list objects for delete: %w", err)
		}
		var toDelete []s3types.ObjectIdentifier
		for _, obj := range out.Contents {
			k := aws.ToString(obj.Key)
			if directOnly && strings.Contains(k[len(prefix):], "/") {
				continue
			}
			toDelete = append(toDelete, s3types.ObjectIdentifier{Key: obj.Key})
		}
		if len(toDelete) > 0 {
			_, err := s.client.DeleteObjects(s.ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(s.bucket),
				Delete: &s3types.Delete{
					Objects: toDelete,
					Quiet:   aws.Bool(true),
				},
			})
			if err != nil {
				return fmt.Errorf("delete objects: %w", err)
			}
		}
		if out.NextContinuationToken == nil {
			break
		}
		continuationToken = out.NextContinuationToken
	}
	return nil
}

// ListSubkeys lists subpath keys under a session by scanning S3 keys.
func (s *S3SessionStore) ListSubkeys(projectKey, sessionID string) ([]string, error) {
	prefix := s.keyPrefix(claude.SessionKey{
		ProjectKey: projectKey,
		SessionID:  sessionID,
	})
	subkeys := make(map[string]bool)
	var continuationToken *string

	for {
		out, err := s.client.ListObjectsV2(s.ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuationToken,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range out.Contents {
			k := aws.ToString(obj.Key)
			rel := k[len(prefix):]
			parts := strings.Split(rel, "/")
			if len(parts) >= 2 {
				subpath := strings.Join(parts[:len(parts)-1], "/")
				if subpath != "" {
					subkeys[subpath] = true
				}
			}
		}
		if out.NextContinuationToken == nil {
			break
		}
		continuationToken = out.NextContinuationToken
	}

	result := make([]string, 0, len(subkeys))
	for sp := range subkeys {
		// Defense-in-depth: drop traversal segments.
		valid := true
		for _, seg := range strings.Split(sp, "/") {
			if seg == ".." || seg == "." || seg == "" {
				valid = false
				break
			}
		}
		if valid {
			result = append(result, sp)
		}
	}
	return result, nil
}
