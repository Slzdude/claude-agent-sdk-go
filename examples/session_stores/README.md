# SessionStore Reference Adapters

Reference implementations of `claude.SessionStore` for S3, Redis, and Postgres.
Copy the adapter you need into your own project and adapt as needed.

> **Note**: The import paths below use `github.com/anthropics/claude-agent-sdk-go`
> which is the public module path. When copying into your own project, update the
> import to match your module path.

## S3 (`s3_session_store.go`)

Uses AWS SDK v2. Transcripts stored as JSONL part files with monotonic
epoch-ms prefixes for chronological ordering.

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    sessionstores "github.com/anthropics/claude-agent-sdk-go/examples/session_stores"
)

cfg, _ := config.LoadDefaultConfig(ctx)
client := s3.NewFromConfig(cfg)
store := sessionstores.NewS3SessionStore(client, "my-bucket", "transcripts")
```

## Redis (`redis_session_store.go`)

Uses go-redis v9. Keys use `:` separator with sorted set for session index.

```go
import (
    "github.com/redis/go-redis/v9"
    sessionstores "github.com/anthropics/claude-agent-sdk-go/examples/session_stores"
)

rdb := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
store := sessionstores.NewRedisSessionStore(rdb, "transcripts")
```

## Postgres (`postgres_session_store.go`)

Uses pgx v5. One row per transcript entry with jsonb storage and bigserial
ordering.

```go
import (
    "github.com/jackc/pgx/v5/pgxpool"
    sessionstores "github.com/anthropics/claude-agent-sdk-go/examples/session_stores"
)

pool, _ := pgxpool.New(ctx, "postgresql://...")
store, _ := sessionstores.NewPostgresSessionStore(pool, "claude_session_store")
store.CreateSchema(ctx) // one-time, idempotent
```

## Usage with Claude SDK

```go
msgs, _ := claude.Query(ctx, "Hello!", &claude.ClaudeAgentOptions{
    SessionStore: store,
})
```

## Dependencies

Each adapter has its own external dependency. Add the relevant one to your
`go.mod`:

| Adapter   | Dependency                                      |
|-----------|------------------------------------------------|
| S3        | `github.com/aws/aws-sdk-go-v2` + `service/s3` |
| Redis     | `github.com/redis/go-redis/v9`                 |
| Postgres  | `github.com/jackc/pgx/v5`                      |
