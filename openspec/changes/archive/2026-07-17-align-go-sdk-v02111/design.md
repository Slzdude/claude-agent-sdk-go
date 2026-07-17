## Context

Python SDK v0.2.111 的 3 个 PR (#1081, #1082, #1083) 修复了安全性和可靠性问题。

## Goals / Non-Goals

**Goals:**
- 逐项对齐 Python SDK v0.2.111 的所有功能性变更
- 保持 Go 惯用风格（log.Printf 而非 warnings.warn）

**Non-Goals:**
- 不实现 asyncio→anyio 迁移（Go 用 goroutine）
- 不实现 `_LineFramer` 类（Go 用 bufio.Scanner 已正确处理行分割）

## Decisions

### 1. CanUseToolShadowedWarning 用 log.Printf

Go 没有 Python 的 `warnings` 模块。使用 `log.Printf("[warning] ...")` 输出，在 `processQuery` 和 `NewClaudeSDKClient` 中调用。

### 2. NDJSON 解析保持 bufio.Scanner

Go 的 `bufio.Scanner` 已按行分割。需要对齐的是 `_parse_stdout_line` 的逻辑：strip、跳过非 JSON、JSON 失败报错。

### 3. --resume=value 已在 Go 中实现

检查并确认 `transport.go` 已使用 `--resume=value` 格式。

### 4. close() 超时保护

Go 的 `sync.Mutex` 没有超时获取。改用 `TryLock` 或在 close 路径中直接设置 `closed` 标志位。

## Risks / Trade-offs

- 低风险，纯增量改进
