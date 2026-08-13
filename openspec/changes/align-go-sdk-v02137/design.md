## Context

Python SDK v0.2.121→v0.2.137 (12 files, 2197 行新增) 引入了消息来源追踪、对话重置、任务生命周期跟踪等重要功能。

## Goals / Non-Goals

**Goals:**
- 逐项对齐 Python SDK v0.2.137 的所有功能性变更
- 保持 Go 惯用风格

**Non-Goals:**
- 不实现 asyncio→anyio 迁移
- 不实现 `_LineFramer` (Go 用 bufio.Scanner)

## Decisions

### 1. MessageOrigin 用 Go struct + map[string]any

Python 用 TypedDict（total=False），Go 用 struct + json tags。未建模的字段保留在 `Extra map[string]any` 中以保持前向兼容。

### 2. ModelUsage 用 Go struct

Python 用 TypedDict，Go 用 struct with json tags。字段名保持 camelCase 以匹配 CLI wire format。

### 3. Task 生命周期跟踪

Go 的 queryProtocol 需要添加 `_inflightTasks` set 和 `_trackTaskLifecycle` 方法。

### 4. Skills 验证

Go 在 transport.go 的 `buildCommand()` 中添加验证，与 Python 的 `_apply_skills_defaults` 对齐。

## Risks / Trade-offs

- 低风险，纯增量改进
