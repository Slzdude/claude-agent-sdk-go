# End-to-End Tests for Claude Code Go SDK

This directory contains end-to-end tests that run against the actual Claude API.

## Requirements

### API Key (REQUIRED)

Set your API key before running:

```bash
export ANTHROPIC_API_KEY="your-api-key-here"
```

## Running the Tests

```bash
# Run all e2e tests
go test -tags e2e ./e2e/ -v -timeout 300s

# Run a specific test
go test -tags e2e ./e2e/ -v -run TestSDKMCPToolExecution

# Run with parallelism limit (recommended to avoid rate limits)
go test -tags e2e ./e2e/ -v -timeout 300s -p 1
```

## Test Coverage

| Go Test File | Python Equivalent |
|---|---|
| `agents_test.go` | `test_agents_and_settings.py` |
| `dynamic_control_test.go` | `test_dynamic_control.py` |
| `hook_events_test.go` | `test_hook_events.py` |
| `hooks_test.go` | `test_hooks.py` |
| `include_partial_messages_test.go` | `test_include_partial_messages.py` |
| `sdk_mcp_tools_test.go` | `test_sdk_mcp_tools.py` |
| `stderr_callback_test.go` | `test_stderr_callback.py` |
| `structured_output_test.go` | `test_structured_output.py` |
| `tool_permissions_test.go` | `test_tool_permissions.py` |

## Cost Considerations

⚠️ These tests make real API calls to Claude, incurring costs.

- Each test typically uses 1–3 API calls
- Simple prompts are used to minimize token usage
- Use `-run TestName` to run a single test
