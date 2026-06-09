# tracing — OpenTelemetry instrumentation for Claude Agent SDK Go

Zero-intrusion, decorator-pattern tracing layer for `claude-agent-sdk-go`.
Backend-agnostic — accepts any `trace.TracerProvider`. Uses OpenInference
semantic conventions for compatibility with Langfuse, Arize, Phoenix, and
any OTLP-compatible backend.

## Quick Start

```go
import (
    "github.com/Slzdude/claude-agent-sdk-go"
    "github.com/Slzdude/claude-agent-sdk-go/tracing"
)

// 1. Set up your TracerProvider (Langfuse, Jaeger, etc.)
//    See examples/langfuse_tracing/ for a complete Langfuse setup.
tp := setupYourTracerProvider()
defer tp.Shutdown(ctx)

// 2. Pass TracerProvider to the SDK — that's it.
msgs, _ := tracing.TracedQuery(ctx, "Hello", &claude.ClaudeAgentOptions{
    TracerProvider: tp,
})
for msg := range msgs { ... }
```

Or use the built-in `TracerProvider` option on `ClaudeAgentOptions`:

```go
opts := &claude.ClaudeAgentOptions{
    TracerProvider: tp,  // one-line tracing enable
}
msgs, _ := claude.Query(ctx, "Hello", opts)
```

## Span Hierarchy

```
ClaudeAgentSDK.Query (AGENT)
├── Bash (TOOL)
├── Read (TOOL)
└── Task (TOOL)
    └── ClaudeAgentSDK.Task (AGENT)
        ├── Bash (TOOL)
        └── Read (TOOL)
```

## Context Attributes

Inject metadata into every span via context:

```go
ctx = tracing.WithSession(ctx, sessionID)
ctx = tracing.WithUser(ctx, userID)
ctx = tracing.WithMetadata(ctx, `{"env":"production"}`)
ctx = tracing.WithTags(ctx, "alert", "critical")
```

## Attributes

| Attribute | Source |
|-----------|--------|
| `openinference.span.kind` | AGENT or TOOL |
| `llm.system` | "anthropic" |
| `llm.model_name` | AssistantMessage.Model, ResultMessage.Model |
| `llm.token_count.*` | ResultMessage.Usage |
| `llm.cost.total` | ResultMessage.TotalCostUSD |
| `input.value` | Prompt or tool input |
| `output.value` | Result or tool output |
| `session.id` | ResultMessage.SessionID |
| `tool.name` | ToolUseBlock.Name |
| `tool.id` | ToolUseBlock.ID |
| `agent.name` | Subagent agent_id |
| `metadata` | JSON from WithMetadata() |
| `tag.tags` | String slice from WithTags() |
| `user.id` | From WithUser() |

## Examples

- `examples/langfuse_tracing/` — Langfuse OTLP setup
- `examples/otel_collector/` — Custom OTel collector (Jaeger, Tempo, etc.)
