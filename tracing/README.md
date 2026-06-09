# tracing — OpenTelemetry instrumentation for Claude Agent SDK Go

Zero-intrusion tracing layer for `claude-agent-sdk-go`. Creates OpenTelemetry spans with [OpenInference](https://github.com/Arize-ai/openinference) semantic conventions, compatible with Langfuse, Arize, Phoenix, and any OTLP backend.

**Backend-agnostic** — the SDK accepts any `trace.TracerProvider` and never creates its own exporter.

## Quick Start

### Option A: Built-in `TracerProvider` field (simplest)

```go
import claude "github.com/Slzdude/claude-agent-sdk-go"

tp := setupYourTracerProvider() // your code

msgs, _ := claude.Query(ctx, "Hello", &claude.ClaudeAgentOptions{
    TracerProvider: tp,  // one line — tracing is automatic
})
```

### Option B: `tracing.TracedQuery` decorator

```go
import (
    claude "github.com/Slzdude/claude-agent-sdk-go"
    "github.com/Slzdude/claude-agent-sdk-go/tracing"
)

tp := setupYourTracerProvider()

msgs, _ := tracing.TracedQuery(ctx, "Hello",
    &claude.ClaudeAgentOptions{},
    tracing.WithTracerProvider(tp),
)
```

### Multi-turn client

```go
client, _ := claude.NewClaudeSDKClient(ctx, &claude.ClaudeAgentOptions{
    TracerProvider: tp,
})
defer client.Close()

client.Query(ctx, "Hello")
for msg := range client.ReceiveResponse(ctx) { ... }

client.Query(ctx, "Follow up")
for msg := range client.ReceiveResponse(ctx) { ... }
```

## Context Attributes

Inject metadata that appears on every span:

```go
ctx = tracing.WithSession(ctx, "session-123")        // session.id
ctx = tracing.WithUser(ctx, "user-456")              // user.id
ctx = tracing.WithMetadata(ctx, `{"env":"prod"}`)    // metadata (JSON string)
ctx = tracing.WithTags(ctx, "tag1", "tag2")          // tag.tags (string slice)

// All spans created from this context carry these attributes
msgs, _ := claude.Query(ctx, "Hello", opts)
```

## Span Hierarchy

```
ClaudeAgentSDK.Query (AGENT)
├── Skill (TOOL)              ← skill 加载
├── Bash (TOOL)               ← 命令执行
├── Read (TOOL)               ← 文件读取
└── Task (TOOL)               ← 子代理调用
    └── ClaudeAgentSDK.Task (AGENT)
        ├── Bash (TOOL)
        └── Read (TOOL)
```

## Attributes

| Attribute | Source |
|-----------|--------|
| `openinference.span.kind` | `AGENT` or `TOOL` |
| `llm.system` | `"anthropic"` |
| `llm.model_name` | ResultMessage.Model, AssistantMessage.Model |
| `gen_ai.request.model` | 同上（Langfuse 备选映射） |
| `llm.token_count.prompt` | ResultMessage.Usage.input_tokens |
| `llm.token_count.completion` | ResultMessage.Usage.output_tokens |
| `llm.token_count.total` | prompt + completion |
| `llm.token_count.prompt_details.cache_read` | 缓存读取 tokens |
| `llm.token_count.prompt_details.cache_write` | 缓存写入 tokens |
| `llm.cost.total` | ResultMessage.TotalCostUSD |
| `input.value` | 用户 prompt 或工具输入 |
| `input.mime_type` | `text/plain` 或 `application/json` |
| `output.value` | agent 结果或工具输出 |
| `output.mime_type` | 同上 |
| `gen_ai.completion` | 同 output.value（Langfuse 备选映射） |
| `session.id` | ResultMessage.SessionID 或 WithSession |
| `user.id` | WithUser |
| `metadata` | WithMetadata（JSON 字符串） |
| `tag.tags` | WithTags（字符串切片） |
| `tool.name` | 工具名称 |
| `tool.id` | 工具调用 ID |
| `tool.parameters` | 工具输入参数（JSON） |
| `agent.name` | 子代理 agent_id |
| `llm.output_messages.N.*` | 输出消息结构（角色、内容、工具调用） |

## PII Redaction

Use `AttributeFilter` to redact sensitive data before it reaches the exporter:

```go
msgs, _ := tracing.TracedQuery(ctx, prompt, opts,
    tracing.WithTracerProvider(tp),
    tracing.WithAttributeFilter(func(kv attribute.KeyValue) bool {
        // Drop input.value and output.value (may contain PII)
        if kv.Key == "input.value" || kv.Key == "output.value" {
            return false
        }
        return true
    }),
)
```

## Instrumentation Suppression

Disable tracing for specific calls:

```go
ctx = tracing.WithSuppression(ctx)
msgs, _ := claude.Query(ctx, "This call won't create any spans", opts)
```

## Backend Setup Examples

See `examples/`:
- `examples/langfuse_tracing/` — Langfuse OTLP setup
- `examples/otel_collector/` — Generic OTel collector (Jaeger, Tempo, etc.)

## What Gets Traced

| Event | Span Created |
|-------|-------------|
| `claude.Query()` call | AGENT span with input.value |
| Assistant message with text | `llm.output_messages.*` attributes on AGENT span |
| Tool use (Bash, Read, Write, etc.) | TOOL child span under AGENT |
| Tool result | output.value on TOOL span |
| Task/Agent delegation | TOOL span + nested AGENT span |
| Sub-agent tool calls | TOOL spans under nested AGENT |
| Result message | Token counts, cost, session.id on AGENT span |
