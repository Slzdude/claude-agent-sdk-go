## Why

Python SDK v0.2.121→v0.2.137 引入了消息来源追踪（MessageOrigin）、对话重置消息、任务生命周期跟踪、模型使用量类型化、resume 选项扩展、技能验证等重要功能。需要逐项迁移到 Go SDK。

## What Changes

- **MessageOrigin 类型体系**: 9 种消息来源类型 + TypedDict + 解析
- **ConversationResetMessage**: 新消息类型，对话重置事件
- **ModelUsage TypedDict**: 替代 `map[string]any`，类型安全的模型使用量
- **ResultMessage 新字段**: `origin`, `terminal_reason`, `model_usage` 类型细化
- **UserMessage 新字段**: `origin`
- **ClaudeAgentOptions 新字段**: `resume_session_at`, `resume_drops_turn`
- **Task 生命周期跟踪**: Query 跟踪 in-flight tasks，延迟 stdin 关闭
- **Skills 验证**: 参数类型校验 + 名称格式校验
- **Windows 安全**: 命令行元字符检测

## Capabilities

### New Capabilities
- `message-origin`: 消息来源追踪系统
- `conversation-reset`: 对话重置消息
- `task-lifecycle`: 任务生命周期跟踪
- `skills-validation`: 技能参数验证

### Modified Capabilities
- `model-usage`: ModelUsage 类型化
- `resume-options`: resume_session_at/resume_drops_turn
- `message-parsing`: origin/terminal_reason 解析

## Impact

- `types.go` — 5 个新类型 + 6 个新字段
- `message_parser.go` — origin 解析 + conversation_reset 解析
- `query_protocol.go` — task 生命周期跟踪
- `transport.go` — skills 验证 + Windows 安全 + 新 CLI 标志
