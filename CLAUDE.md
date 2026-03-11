# Workflow

```bash
# Format code
gofmt -w .

# Lint (requires golangci-lint)
golangci-lint run ./...

# Typecheck / vet
go vet ./...

# Run all unit tests
go test ./...

# Run specific test file
go test -run TestQuery ./...

# Run with verbose output
go test -v ./...

# Run with race detector
go test -race ./...

# Run e2e tests (requires ANTHROPIC_API_KEY)
go test -tags e2e -p 1 -v -timeout 300s ./e2e/

# Run a specific e2e test
go test -tags e2e -p 1 -v -timeout 300s -run TestAgentDefinition ./e2e/

# Build all packages
go build ./...

# Tidy dependencies
go mod tidy
```

# Codebase Structure

- `claude.go` - Main package entry point, public API exports
- `types.go` - Type definitions (messages, options, hooks, etc.)
- `query_protocol.go` - Query protocol and message parsing logic
- `message_parser.go` - JSON stream message parser
- `process_query.go` - Core query execution
- `sessions.go` - Session management for ClaudeSDKClient
- `transport.go` - Transport layer (subprocess CLI management)
- `hooks.go` - Hook system (PreToolUse, PostToolUse, etc.)
- `mcp.go` - MCP (Model Context Protocol) server integration
- `errors.go` - Error types
- `e2e/` - End-to-end tests (require real API key, build tag `e2e`)
- `examples/` - Example programs demonstrating SDK usage
