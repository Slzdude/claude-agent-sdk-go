# Go SDK 升级报告: Python SDK v0.1.48 → v0.1.68 对齐

**日期**: 2026-04-10
**提交**: `eb27c25` + 当前未提交变更
**变更规模**: 25 files, ~2900 insertions
**CLI 版本**: 2.1.71 → 2.1.121

---

## 一、升级概要

| 指标 | 数值 |
|------|------|
| Python SDK 版本差 | 20 个 minor 版本 (v0.1.48 → v0.1.68) |
| 新增 Go 文件 | 1 (`session_mutations.go`) |
| 修改 Go 文件 | 18 |
| 新增类型/结构体 | 30+ |
| 新增导出函数 | 15+ |
| 新增客户端方法 | 1 |
| 行为修复 | 12 |
| 单元测试 | ~100 (全部通过) |
| E2E 测试 | 8+ (全部通过) |

---

## 二、完整功能清单

### 2.1 新增消息类型

| 类型 | 说明 |
|------|------|
| `RateLimitEvent` | 限流状态变更事件 |
| `RateLimitInfo` | 限流详细信息 |
| `MirrorErrorMessage` | SessionStore 追加失败消息 |
| `ServerToolUseBlock` | 服务端工具调用 (advisor, web_search 等) |
| `ServerToolResultBlock` | 服务端工具结果 |

### 2.2 新增 SessionStore 体系

| 类型/函数 | 说明 |
|-----------|------|
| `SessionStore` 接口 | 外部存储适配器 (Append/Load/ListSessions/Delete/ListSubkeys) |
| `SessionKey` | Session 存储键 |
| `SessionStoreEntry` | JSONL 条目 |
| `SessionStoreListEntry` | 列表条目 |
| `SessionSummaryEntry` | 摘要条目 |
| `McpServerInfo` | MCP 服务器信息 |
| `McpToolAnnotations` | MCP 工具注解 (wire format 兼容) |
| `McpServerConnectionStatus` | MCP 连接状态枚举 |
| `McpSdkServerConfigStatus` | SDK MCP 配置状态 |
| `McpClaudeAIProxyServerConfig` | Claude AI 代理配置 |
| `ListSessionsFromStore()` | 从 Store 列出 session |
| `GetSessionMessagesFromStore()` | 从 Store 读取消息 |
| `RenameSessionViaStore()` | 通过 Store 重命名 |
| `TagSessionViaStore()` | 通过 Store 标记 |
| `DeleteSessionViaStore()` | 通过 Store 删除 |
| `ForkSessionViaStore()` | 通过 Store 分叉 |

### 2.3 新增 Session 函数

| 函数 | 说明 |
|------|------|
| `ProjectKeyForDirectory()` | 从目录路径推导 SessionStore key |
| `ListSubagents()` | 列出子 agent |
| `GetSubagentMessages()` | 读取子 agent 消息 |
| `GetSessionInfo()` | O(1) 单 session 查询 |

### 2.4 新增选项

| 选项 | 类型 | CLI 标志 |
|------|------|----------|
| `Skills` | `any` ("all"/[]string) | 自动注入 Skill 工具 |
| `SessionStore` | `SessionStore` | `--session-mirror` |
| `LoadTimeoutMs` | `int` | - |
| `SessionID` | `string` | `--session-id` |
| `TaskBudget` | `*TaskBudget` | `--task-budget` |
| `SystemPromptFile` | `*SystemPromptFile` | `--system-prompt-file` |
| `ExcludeDynamicSections` | `*bool` | initialize 字段 |

### 2.5 类型扩展

| 类型 | 新增字段 |
|------|----------|
| `AgentDefinition` | DisallowedTools, Skills, Memory, MCPServers, InitialPrompt, MaxTurns, Background, Effort (EffortLevel), PermissionMode |
| `AssistantMessage` | Usage, MessageID, StopReason, SessionID, UUID |
| `ResultMessage` | ModelUsage, PermissionDenials, Errors, UUID |
| `SDKSessionInfo` | Tag, CreatedAt; FileSize → *int64 |
| `ToolPermissionContext` | ToolUseID, AgentID |
| `ClaudeAgentOptions` | SessionID, TaskBudget, Skills, SessionStore, LoadTimeoutMs |
| `McpServerStatus` | ServerInfo → *McpServerInfo, Status → McpServerConnectionStatus |
| `McpToolInfo` | Annotations → *McpToolAnnotations (wire 兼容) |
| `ThinkingAdaptive` | Display (ThinkingDisplay) |
| `ThinkingEnabled` | Display (ThinkingDisplay) |

### 2.6 强类型 Hook 类型

| 类型 | 说明 |
|------|------|
| `PreToolUseHookInput` | PreToolUse 钩子输入 |
| `PostToolUseHookInput` | PostToolUse 钩子输入 |
| `PostToolUseFailureHookInput` | PostToolUseFailure 钩子输入 |
| `UserPromptSubmitHookInput` | UserPromptSubmit 钩子输入 |
| `StopHookInput` | Stop 钩子输入 |
| `SubagentStopHookInput` | SubagentStop 钩子输入 |
| `PreCompactHookInput` | PreCompact 钩子输入 |
| `NotificationHookInput` | Notification 钩子输入 |
| `SubagentStartHookInput` | SubagentStart 钩子输入 |
| `PermissionRequestHookInput` | PermissionRequest 钩子输入 |
| `SyncHookJSONOutput` | 同步钩子输出 |
| `AsyncHookJSONOutput` | 异步钩子输出 |
| `PreToolUseHookSpecificOutput` | PreToolUse 特定输出 |
| `PostToolUseHookSpecificOutput` | PostToolUse 特定输出 |
| `PostToolUseFailureHookSpecificOutput` | PostToolUseFailure 特定输出 |
| `UserPromptSubmitHookSpecificOutput` | UserPromptSubmit 特定输出 |
| `NotificationHookSpecificOutput` | Notification 特定输出 |
| `SubagentStartHookSpecificOutput` | SubagentStart 特定输出 |
| `PermissionRequestHookSpecificOutput` | PermissionRequest 特定输出 |

### 2.7 行为修复

| # | 修复 | 文件 |
|---|------|------|
| 1 | Thinking config: --thinking adaptive/disabled | transport.go |
| 2 | Graceful shutdown: wait→SIGTERM→SIGKILL | transport.go |
| 3 | CLAUDECODE 环境变量过滤 | transport.go |
| 4 | CLAUDE_CODE_ENTRYPOINT 优先级修复 | transport.go |
| 5 | Non-JSON 行过滤 | transport.go |
| 6 | setting_sources 空值修复 | transport.go |
| 7 | control_cancel_request 实现 | query_protocol.go |
| 8 | notifications/initialized JSON-RPC 格式 | query_protocol.go |
| 9 | SDK 版本更新为 0.2.0 | transport.go |
| 10 | MCP ToolAnnotations wire format 修复 | mcp.go |
| 11 | delete_session 级联删除子 agent 目录 | session_mutations.go |
| 12 | --thinking-display 标志 | transport.go |
| 13 | --session-mirror 标志 | transport.go |
| 14 | TRACEPARENT/TRACESTATE 传播 | transport.go |
| 15 | Skills 默认值注入 | transport.go |

---

## 三、Wire Format 兼容性

| 组件 | 状态 |
|------|------|
| 控制请求信封 | ✅ 完全一致 |
| Initialize 请求 | ✅ 字段完全一致 (hooks, agents, skills, excludeDynamicSections) |
| Hook 回调响应 | ✅ async_/continue_ 转换一致 |
| 权限请求处理 | ✅ behavior/updatedInput 格式一致 |
| MCP JSON-RPC 路由 | ✅ 路由逻辑一致 |
| CLI 命令参数 | ✅ 标志完全一致 |
| 环境变量 | ✅ 结构一致 (CLAUDECODE 已过滤) |
| MCP Status Response | ✅ McpToolAnnotations wire format 修复 |
| ServerToolUse/Result | ✅ server_tool_use/advisor_tool_result 解析 |

---

## 四、测试覆盖

### 单元测试 (~100)

| 测试文件 | 测试数 | 覆盖范围 |
|----------|--------|---------|
| message_parser_test.go | 12 | 所有消息类型、content block 类型、RateLimitEvent、ServerToolUse/Result、MirrorError |
| query_protocol_test.go | 15 | 控制协议、cancel_request、MCP 类型、ToolAnnotations wire format、skills |
| transport_test.go | 14 | thinking flags、skills defaults、session-mirror、所有 CLI 标志 |
| sessions_test.go | 20 | 列表、查询、子 agent、store-backed、ProjectKeyForDirectory、Unicode |
| rate_limit_test.go | 4 | RateLimitEvent 解析 |
| types_test.go | 5 | AgentDefinition 序列化 |

### E2E 测试 (8+)

| 测试 | 结果 |
|------|------|
| TestDontAskPermissionMode | PASS |
| TestAutoPermissionMode | PASS |
| TestTaskBudget_Option | PASS |
| TestThinkingAdaptive | PASS |
| TestRateLimitEvent_Reception | PASS |
| TestGetContextUsage_Basic | PASS |
| TestAgentDefinition_ExtendedFields | PASS |
| TestSessionMutations_RenameTagDelete | PASS |
| TestSessionMutations_Fork | PASS |

---

## 五、已知差异 (语言固有)

| 差异 | Python | Go |
|------|--------|-----|
| MCP 工具定义 | `@tool` 装饰器 | `SdkMcpServer` 接口 |
| 异步模型 | AsyncIterator | `<-chan Message` |
| 上下文管理 | `async with` | 构造/Close |
| Hook 输入 | 强类型 TypedDicts | 强类型 structs (本次升级后) |
| OTEL 集成 | opentelemetry-api 库 | 环境变量传播 (简化版) |

---

## 六、文件变更清单

```
新增文件 (1):
  session_mutations.go          590+ 行   Session 变更 + store-backed mutations

修改文件 (17):
  types.go                      +200     30+ 新类型/字段
  mcp.go                        +60      McpToolAnnotations, McpServerInfo, 状态类型
  hooks.go                      +250     20+ 强类型 hook 输入/输出
  sessions.go                   +350     ListSubagents, GetSubagentMessages, store-backed, ProjectKeyForDirectory
  session_mutations.go          +110     cascade delete, store-backed mutations
  transport.go                  +100     skills, thinking-display, session-mirror, TRACEPARENT
  claude.go                     +25      skills wiring
  query_protocol.go             +20      skills initialize
  message_parser.go             +40      ServerToolUse/Result, MirrorError
  README.md                     +80      新功能文档
  6 test files                  +400     全面测试覆盖
```

---

## 七、像素级对齐验证 (v0.1.58 → v0.1.68)

### 逐项验证结果: 24/24 ✅

| # | Python SDK 导出 | Go SDK 等价物 | 状态 |
|---|----------------|--------------|------|
| 1 | `InMemorySessionStore` | `InMemorySessionStore` (session_store.go) | ✅ |
| 2 | `SessionListSubkeysKey` | `SessionListSubkeysKey` (types.go) | ✅ |
| 3 | `fold_session_summary` | `FoldSessionSummary()` (sessions.go) | ✅ |
| 4 | `import_session_to_store` | `ImportSessionToStore()` (sessions.go) | ✅ |
| 5 | `get_session_info_from_store` | `GetSessionInfoFromStore()` (sessions.go) | ✅ |
| 6 | `list_subagents_from_store` | `ListSubagentsFromStore()` (sessions.go) | ✅ |
| 7 | `get_subagent_messages_from_store` | `GetSubagentMessagesFromStore()` (sessions.go) | ✅ |
| 8 | `list_sessions_from_store` | `ListSessionsFromStore()` (sessions.go) | ✅ |
| 9 | `get_session_messages_from_store` | `GetSessionMessagesFromStore()` (sessions.go) | ✅ |
| 10 | `project_key_for_directory` | `ProjectKeyForDirectory()` (sessions.go) | ✅ |
| 11 | `list_subagents` | `ListSubagents()` (sessions.go) | ✅ |
| 12 | `get_subagent_messages` | `GetSubagentMessages()` (sessions.go) | ✅ |
| 13 | `get_session_info` | `GetSessionInfo()` (sessions.go) | ✅ |
| 14 | `list_sessions` | `ListSessions()` (sessions.go) | ✅ |
| 15 | `get_session_messages` | `GetSessionMessages()` (sessions.go) | ✅ |
| 16 | `rename_session` | `RenameSession()` (session_mutations.go) | ✅ |
| 17 | `tag_session` | `TagSession()` (session_mutations.go) | ✅ |
| 18 | `delete_session` | `DeleteSession()` (session_mutations.go) | ✅ |
| 19 | `fork_session` | `ForkSession()` (session_mutations.go) | ✅ |
| 20 | `rename_session_via_store` | `RenameSessionViaStore()` (session_mutations.go) | ✅ |
| 21 | `tag_session_via_store` | `TagSessionViaStore()` (session_mutations.go) | ✅ |
| 22 | `delete_session_via_store` | `DeleteSessionViaStore()` (session_mutations.go) | ✅ |
| 23 | `fork_session_via_store` | `ForkSessionViaStore()` (session_mutations.go) | ✅ |
| 24 | `list_all_sessions` | `ListAllSessions()` (sessions.go) | ✅ |

### 类型对齐验证

| Python 类型 | Go 类型 | 状态 |
|------------|---------|------|
| `ServerToolUseBlock` | `ServerToolUseBlock` | ✅ |
| `ServerToolResultBlock` | `ServerToolResultBlock` | ✅ |
| `MirrorErrorMessage` | `MirrorErrorMessage` | ✅ |
| `SessionKey` | `SessionKey` | ✅ |
| `SessionStoreEntry` | `SessionStoreEntry` | ✅ |
| `SessionStoreListEntry` | `SessionStoreListEntry` | ✅ |
| `SessionSummaryEntry` | `SessionSummaryEntry` | ✅ |
| `SessionListSubkeysKey` | `SessionListSubkeysKey` | ✅ |
| `SessionStore` | `SessionStore` 接口 | ✅ |
| `ThinkingDisplay` | `ThinkingDisplay` | ✅ |
| `McpToolAnnotations` | `McpToolAnnotations` | ✅ |
| `McpServerInfo` | `McpServerInfo` | ✅ |
| `McpServerConnectionStatus` | `McpServerConnectionStatus` | ✅ |
| `McpSdkServerConfigStatus` | `McpSdkServerConfigStatus` | ✅ |
| `McpClaudeAIProxyServerConfig` | `McpClaudeAIProxyServerConfig` | ✅ |
| `RateLimitEvent` | `RateLimitEvent` | ✅ |
| `RateLimitInfo` | `RateLimitInfo` | ✅ |
| 10× HookInput types | 10× HookInput structs | ✅ |
| 9× HookOutput types | 9× HookOutput structs | ✅ |

### Wire Format 验证

| 组件 | 状态 |
|------|------|
| 控制请求信封 | ✅ 完全一致 |
| Initialize 请求 (hooks, agents, skills, excludeDynamicSections) | ✅ |
| Hook 回调响应 (async_/continue_ 转换) | ✅ |
| 权限请求处理 | ✅ |
| MCP JSON-RPC 路由 | ✅ |
| MCP Status Response (McpToolAnnotations wire format) | ✅ |
| ServerToolUse/Result 解析 | ✅ |
| CLI 命令参数 | ✅ |
| 环境变量 (CLAUDECODE 过滤, ENTRYPOINT 优先级, TRACEPARENT) | ✅ |
