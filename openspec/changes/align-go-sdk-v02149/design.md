## Context

Python SDK v0.2.140 包含 5 个 PR：
- #1204: can_use_tool 支持 string prompts（Go 已支持）
- #1205: ResultError 类型化错误
- #1206: forward_subagent_text + MCP 2.x
- #1207: parent_tool_use_id/parent_agent_id 恢复
- #1218: MCP 2.x 兼容

MCP 2.x 兼容（SdkMcpBridge + _mcp_compat）是重大变更，需要单独处理。本次提案聚焦于其他 4 项。

## Goals / Non-Goals

**Goals:**
- 新增 ResultError 类型化错误
- 新增 UserMessage.ParentAgentID 字段
- 新增 forward_subagent_text 选项
- 新增 --forward-subagent-text CLI 标志

**Non-Goals:**
- MCP 2.x 兼容（单独提案）
- _configure_can_use_tool 重构（Go 已有等价逻辑）
- _hooks_to_internal_format 重构（Go 已有等价逻辑）

## Decisions

### 1. ResultError 继承 ProcessError

Python 的 `ResultError(ProcessError)` 继承 `ProcessError`。Go 的 `ResultError` 应嵌入 `ProcessError`，保持 `errors.As` 兼容。

### 2. forward_subagent_text 作为 ClaudeAgentOptions 字段

新增 `ForwardSubagentText bool` 字段，在 `Initialize()` 请求中传递。

## Risks / Trade-offs

- ResultError 需要修改错误处理链，影响 process_query.go
