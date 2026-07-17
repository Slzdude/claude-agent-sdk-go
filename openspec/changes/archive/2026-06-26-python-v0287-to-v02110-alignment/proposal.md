## Why

Python SDK v0.2.87→v0.2.110 引入了 `TaskUpdatedMessage` 类型和 `task_updated` 解析，以及 asyncio→anyio 的内部迁移。Go SDK 需要补齐新类型和解析，并充分测试。

## What Changes

- **新增类型**: `TaskUpdatedStatus` (6 值枚举)、`TERMINAL_TASK_STATUSES` (4 值集合)、`TaskUpdatedMessage` struct
- **新增解析**: `task_updated` subtype 在 message_parser.go 中解析为 `TaskUpdatedMessage`
- **测试**: 新增 TaskUpdatedMessage 解析测试、TerminalTaskStatuses 验证测试

## Capabilities

### New Capabilities
- `task-updated-message`: 新增 TaskUpdatedMessage 类型和 task_updated 解析

### Modified Capabilities
- (无)

## Impact

- `types.go` — 新增 3 个类型/常量
- `message_parser.go` — 新增 task_updated 解析
- 新增测试文件覆盖新功能
