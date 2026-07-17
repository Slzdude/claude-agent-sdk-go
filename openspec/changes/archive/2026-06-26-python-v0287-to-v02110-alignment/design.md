## Context

Python SDK v0.2.87→v0.2.110 新增了 `TaskUpdatedMessage` 类型用于处理后台任务状态变更。`task_updated` 消息可能不伴随 `task_notification`，消费者需要同时监听两种消息来追踪任务生命周期。

## Goals / Non-Goals

**Goals:**
- 新增 TaskUpdatedMessage 类型和 task_updated 解析
- 新增 TerminalTaskStatuses 用于判断任务是否结束
- 充分测试覆盖

**Non-Goals:**
- asyncio→anyio 迁移 (Go 用 goroutine，不需要)
- `_swallow_done_exception` 移除 (Go 已有 panic recovery)

## Decisions

### 1. TaskUpdatedStatus 用 Go const 枚举

Go 用 `type TaskUpdatedStatus string` + const 块，与现有 `TaskNotificationStatus` 一致。

### 2. TerminalTaskStatuses 用 map[string]bool

Python 用 `frozenset`，Go 用 `map[string]bool`，支持跨类型查询（TaskNotificationStatus 和 TaskUpdatedStatus 共用）。

### 3. task_updated 解析防御性处理

Python 的 patch 可能不是 dict，status 可能是 None。Go 用类型断言防御性处理。

## Risks / Trade-offs

- 无风险，纯新增功能
