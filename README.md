# Claude Agent SDK for Go

Go SDK for Claude Agent. See the [Claude Agent SDK documentation](https://platform.claude.com/docs/en/agent-sdk) for more information.

## Installation

```bash
go get github.com/anthropics/claude-agent-sdk-go
```

**Prerequisites:**

- Go 1.24+
- Claude Code CLI: `curl -fsSL https://claude.ai/install.sh | bash`

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    claude "github.com/anthropics/claude-agent-sdk-go"
)

func main() {
    ctx := context.Background()

    ch, err := claude.Query(ctx, "What is 2 + 2?", nil)
    if err != nil {
        log.Fatal(err)
    }

    for msg := range ch {
        fmt.Println(msg)
    }
}
```

## Basic Usage: Query()

`Query()` sends a one-shot prompt to Claude and returns a channel of response messages.

```go
import claude "github.com/anthropics/claude-agent-sdk-go"

// Simple query
ch, err := claude.Query(ctx, "Hello Claude", nil)
if err != nil {
    log.Fatal(err)
}

for msg := range ch {
    if am, ok := msg.(*claude.AssistantMessage); ok {
        for _, block := range am.Content {
            if text, ok := block.(*claude.TextBlock); ok {
                fmt.Println(text.Text)
            }
        }
    }
}

// With options
opts := &claude.ClaudeAgentOptions{
    SystemPrompt: "You are a helpful assistant",
    MaxTurns:     1,
}

ch, err = claude.Query(ctx, "Tell me a joke", opts)
```

### Using Tools

```go
opts := &claude.ClaudeAgentOptions{
    AllowedTools:   []string{"Read", "Write", "Bash"},
    PermissionMode: claude.PermissionModeAcceptEdits, // auto-accept file edits
}

ch, err := claude.Query(ctx, "Create a hello.go file", opts)
```

### Working Directory

```go
opts := &claude.ClaudeAgentOptions{
    CWD: "/path/to/project",
}
```

## ClaudeSDKClient

`ClaudeSDKClient` supports bidirectional, interactive conversations with Claude Code.

Unlike `Query()`, `ClaudeSDKClient` additionally enables **custom tools** (SDK MCP servers) and **hooks**.

```go
client, err := claude.NewClaudeSDKClient(ctx, opts)
if err != nil {
    log.Fatal(err)
}
defer client.Close()

if err := client.Query(ctx, "Hello"); err != nil {
    log.Fatal(err)
}

for msg := range client.ReceiveResponse(ctx) {
    fmt.Println(msg)
}
```

### Custom Tools (SDK MCP Servers)

Define in-process tools that Claude can call, with no separate process or IPC overhead:

```go
type MyServer struct{}

func (s *MyServer) Name() string    { return "my-tools" }
func (s *MyServer) Version() string { return "1.0.0" }

func (s *MyServer) Tools() []claude.SdkMcpTool {
    return []claude.SdkMcpTool{
        {
            Name:        "greet",
            Description: "Greet a user",
            InputSchema: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "name": map[string]any{"type": "string"},
                },
                "required": []any{"name"},
            },
        },
    }
}

func (s *MyServer) Execute(ctx context.Context, toolName string, input map[string]any) (*claude.SdkMcpToolResult, error) {
    if toolName == "greet" {
        name, _ := input["name"].(string)
        return &claude.SdkMcpToolResult{
            Content: []map[string]any{
                {"type": "text", "text": "Hello, " + name + "!"},
            },
        }, nil
    }
    return nil, fmt.Errorf("unknown tool: %s", toolName)
}

// Use with Claude
opts := &claude.ClaudeAgentOptions{
    McpServers: map[string]claude.McpServerConfig{
        "tools": &MyServer{},
    },
    AllowedTools: []string{"mcp__tools__greet"},
}
```

### Hooks

Hooks are functions invoked by the Claude Code application at specific points in the agent loop:

```go
func checkBashCommand(ctx context.Context, input map[string]any, toolUseID string, hookCtx *claude.HookContext) (map[string]any, error) {
    toolName, _ := input["tool_name"].(string)
    if toolName != "Bash" {
        return map[string]any{}, nil
    }
    toolInput, _ := input["tool_input"].(map[string]any)
    command, _ := toolInput["command"].(string)

    if strings.Contains(command, "foo.sh") {
        return map[string]any{
            "hookSpecificOutput": map[string]any{
                "hookEventName":              "PreToolUse",
                "permissionDecision":         "deny",
                "permissionDecisionReason":   "Command contains invalid pattern: foo.sh",
            },
        }, nil
    }
    return map[string]any{}, nil
}

matcher := "Bash"
opts := &claude.ClaudeAgentOptions{
    AllowedTools: []string{"Bash"},
    Hooks: map[claude.HookEvent][]claude.HookMatcher{
        claude.HookEventPreToolUse: {
            {Matcher: &matcher, Hooks: []claude.HookFunc{checkBashCommand}},
        },
    },
}
```

## Types

See [types.go](types.go) for complete type definitions:

- `ClaudeAgentOptions` — Configuration options
- `AssistantMessage`, `UserMessage`, `SystemMessage`, `ResultMessage` — Message types
- `TextBlock`, `ToolUseBlock`, `ToolResultBlock`, `ThinkingBlock` — Content blocks
- `HookMatcher`, `HookEvent`, `HookFunc` — Hook system types
- `SdkMcpServer`, `SdkMcpTool`, `SdkMcpToolResult` — SDK MCP server types

## Error Handling

```go
import claude "github.com/anthropics/claude-agent-sdk-go"

ch, err := claude.Query(ctx, "Hello", nil)
if err != nil {
    var cliErr *claude.CLINotFoundError
    if errors.As(err, &cliErr) {
        log.Fatal("Please install Claude Code: curl -fsSL https://claude.ai/install.sh | bash")
    }
    log.Fatal(err)
}

for msg := range ch {
    if result, ok := msg.(*claude.ResultMessage); ok && result.IsError {
        log.Printf("Query failed: %s", result.Result)
    }
}
```

See [errors.go](errors.go) for all error types:
- `CLINotFoundError` — Claude Code not installed
- `CLIConnectionError` — Connection issues
- `ProcessError` — Process failed
- `CLIJSONDecodeError` — JSON parsing issues

## Available Tools

See the [Claude Code documentation](https://docs.anthropic.com/en/docs/claude-code/settings#tools-available-to-claude) for a complete list of available tools.

## Examples

See [examples/](examples/) for working examples:

| Example | Description |
|---------|-------------|
| [quick_start/](examples/quick_start/) | Basic one-shot query |
| [streaming/](examples/streaming/) | `ClaudeSDKClient` with interactive sessions |
| [hooks/](examples/hooks/) | Pre/post tool use hooks |
| [agents/](examples/agents/) | Programmatic subagents |
| [filesystem_agents/](examples/filesystem_agents/) | Filesystem-based agents |
| [mcp_tools/](examples/mcp_tools/) | SDK MCP server with custom tools |
| [include_partial_messages/](examples/include_partial_messages/) | Streaming partial messages |
| [stderr_callback/](examples/stderr_callback/) | Capturing debug output |
| [system_prompt/](examples/system_prompt/) | Custom system prompts |
| [tool_permission_callback/](examples/tool_permission_callback/) | Tool permission callbacks |
| [tools_option/](examples/tools_option/) | Configuring allowed tools |
| [max_budget_usd/](examples/max_budget_usd/) | Cost budget limits |
| [setting_sources/](examples/setting_sources/) | Controlling settings file loading |

## End-to-End Tests

```bash
export ANTHROPIC_API_KEY=your-key-here
go test -tags e2e -p 1 -v -timeout 300s ./e2e/
```

See [e2e/README.md](e2e/README.md) for details.

## Development

```bash
# Run unit tests
go test -race ./...

# Run linter
golangci-lint run ./...

# Format code
gofmt -w .
```
