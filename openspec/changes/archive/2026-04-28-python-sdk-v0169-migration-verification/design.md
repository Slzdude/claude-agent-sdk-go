## Context

Go SDK 基于 Python SDK v0.1.58 初始构建。Python SDK 在 v0.1.58→v0.1.69 期间（21 文件，+4019 行）引入了完整的 SessionStore 外部存储体系。前几轮迁移已覆盖大部分功能，但像素级逐行对比发现以下差异：

1. `FoldSessionSummary` 缺少 set-once 字段、first-prompt 过滤、字段名映射
2. 无 session resume（从 store 恢复到临时目录）能力
3. `ImportSessionToStore` 缺少子 agent 导入和分批处理
4. `ForkSessionViaStore` 缺少 `upToMessageID` 参数
5. 无 SessionStore 一致性测试和预检逻辑
6. `InMemorySessionStore` mtime 硬编码
7. transport 残留 `debug-to-stderr` 检测

## Goals / Non-Goals

**Goals:**
- Go SDK 与 Python SDK v0.1.69 的所有公开 API、类型、行为完全对齐
- 所有新增/修改功能有对应单元测试
- SessionStore 实现有 14 项契约一致性测试保障
- session resume 支持从外部 store 恢复会话

**Non-Goals:**
- 不实现 `TranscriptMirrorBatcher`（Go 用 `--session-mirror` CLI 标志，子进程直接写 store）
- 不实现 macOS Keychain 凭证读取（Go 场景通常用 API key auth）
- 不实现 `_task_compat.py`（Go 用 goroutine 原生支持）
- 不回溯 v0.1.58 之前的功能差异

## Decisions

### 1. FoldSessionSummary 签名变更：移除 mtime 参数

**决策**: 从 `FoldSessionSummary(prev, key, entries, mtime)` 改为 `FoldSessionSummary(prev, key, entries)`，mtime 由适配器在 persist 后自行设置。

**理由**: 与 Python 行为一致。Python 的 fold 不设置 mtime，由适配器 stamp。这确保 mtime 使用与 `list_sessions()` 相同的时钟源（存储写入时间而非 entry 时间戳）。

**替代方案**: 保留 mtime 参数 — 会导致适配器混淆应该在 fold 前还是 fold 后设置 mtime。

### 2. Session Resume 作为独立文件

**决策**: 新建 `session_resume.go`，不合并到 `process_query.go`。

**理由**: session resume 涉及临时目录管理、auth 文件复制、subkey 物化、路径安全校验等多个关注点，独立文件更清晰。集成点仅在 `processQuery()` 中添加 3 行代码。

### 3. Go 不实现 TranscriptMirrorBatcher

**决策**: Go SDK 的 session save 路径通过 `--session-mirror` CLI 标志让子进程直接写 store，不实现 Python 的 TranscriptMirrorBatcher。

**理由**: Go 的 `processQuery` 是同步的 goroutine 模型，不需要 Python 的 async 缓冲层。CLI 子进程已内置 mirror 功能。这减少了 ~200 行代码和一个 goroutine。

**风险**: CLI 的 mirror 行为（重试、超时）不可由 SDK 控制。缓解: CLI 的 mirror 逻辑已成熟稳定。

### 4. ImportSessionToStore 子 agent 导入

**决策**: 递归扫描 `<sessionId>/subagents/**/*.jsonl`，每个子 agent 文件独立分批 append，`.meta.json` sidecar 作为 `agent_metadata` 类型 entry 追加。

**理由**: 与 Python 的 `_collect_jsonl_files` + `_append_jsonl_file_in_batches` 逻辑一致。

## Risks / Trade-offs

- **[API 变更]** `FoldSessionSummary` 和 `ForkSessionViaStore` 签名变更 → 已检查无外部调用者（仅内部使用和 InMemorySessionStore）
- **[临时目录泄漏]** session resume 创建临时目录后异常退出 → 通过 `defer cleanup()` 和 `rmtreeWithRetry` 缓解
- **[路径穿越]** subkey subpath 来自外部 store → `isSafeSubpath` 校验（拒绝空/绝对/.. /drive-prefix/NUL）
- **[refreshToken 泄漏]** 复制 credentials 到临时目录 → `writeRedactedCredentials` 删除 refreshToken
