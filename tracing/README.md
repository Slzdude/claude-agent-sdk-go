# tracing — OpenTelemetry instrumentation for Claude Agent SDK Go

Zero-intrusion, decorator-pattern tracing layer for `claude-agent-sdk-go`. Creates OpenTelemetry spans with OpenInference semantic conventions, compatible with Langfuse and any OTLP backend.

## Quick Start

### Langfuse

```go
import (
    "github.com/Slzdude/claude-agent-sdk-go/tracing"
    "github.com/Slzdude/claude-agent-sdk-go/tracing/langfuse"
)

// Setup
tp, _ := langfuse.SetupLangfuse(ctx, langfuse.LangfuseConfig{})
defer tp.Shutdown(ctx)

// Use TracedQuery instead of claude.Query
msgs, _ := tracing.TracedQuery(ctx, "Hello", nil, tracing.WithTracerProvider(tp))
for msg := range msgs { ... }
```

### Custom OTel backend

```go
import "go.opentelemetry.io/otel"

tp := setupYourTracerProvider()
otel.SetTracerProvider(tp)

msgs, _ := tracing.TracedQuery(ctx, "Hello", nil)
for msg := range msgs { ... }
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

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `LANGFUSE_PUBLIC_KEY` | Langfuse public key | — |
| `LANGFUSE_SECRET_KEY` | Langfuse secret key | — |
| `LANGFUSE_HOST` | Langfuse instance URL | `https://cloud.langfuse.com` |
| `OTEL_SERVICE_NAME` | Service name for traces | `claude-agent-app` |
