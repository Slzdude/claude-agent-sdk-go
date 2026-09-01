## Why

Python SDK v0.2.137→v0.2.149（集中在 v0.2.140 的 5 个 PR）引入了 ResultError 类型化错误、forward_subagent_text 选项、UserMessage.parent_agent_id 字段和 MCP 2.x 兼容。需要迁移到 Go SDK。

## What Changes

- **ResultError 类型**: 新错误类型替代 ProcessError，携带 subtype/errors/result/api_error_status/terminal_reason/session_id/data
- **UserMessage.parent_agent_id**: 新字段，子 agent 的父 agent ID
- **forward_subagent_text 选项**: 新选项，转发子 agent 文本/思考块
- **--forward-subagent-text CLI 标志**: 新 CLI 标志
- **MCP 2.x 兼容**: SdkMcpBridge + _mcp_compat (重大变更，单独处理)

## Capabilities

### New Capabilities
- `result-error`: ResultError 类型化错误
- `forward-subagent-text`: 子 agent 文本转发

### Modified Capabilities
- `message-parsing`: UserMessage parent_agent_id

## Impact

- `types.go` — ResultError, ClaudeAgentOptions.ForwardSubagentText
- `errors.go` — ResultError 结构体
- `message_parser.go` — parent_agent_id 解析
- `transport.go` — --forward-subagent-text CLI 标志
- `query_protocol.go` — forwardSubagentText initialize 请求
- `process_query.go` — ResultError 集成
