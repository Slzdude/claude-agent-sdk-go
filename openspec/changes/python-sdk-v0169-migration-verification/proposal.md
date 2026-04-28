## Why

Go SDK 最初基于 Python SDK v0.1.58 构建。Python SDK 在 v0.1.58→v0.1.69 期间新增了 4019 行代码（21 个文件），引入了 SessionStore 外部存储体系、server tool 解析、thinking display 控制、skills 自动配置等关键功能。虽然前几轮迁移已覆盖大部分功能，但通过逐行像素级对比发现仍有多处实现差异和缺漏需要补齐，以确保 Go SDK 与 Python SDK 功能完全对齐。

## What Changes

- **SessionStore 外部存储完整对齐**: 修复 `FoldSessionSummary` 的 set-once 字段（`is_sidechain`/`created_at`/`cwd`）、first-prompt 提取过滤（slash-command/meta/tool_result）、last-wins 字段名映射（camelCase→snake_case）、tag 空值清除
- **Session Resume 实现**: 新增 `session_resume.go`，支持从 SessionStore 加载会话并写入临时 `CLAUDE_CONFIG_DIR`，包括 auth 文件复制（refreshToken 脱敏）、subkey 物化、路径安全校验
- **ImportSessionToStore 增强**: 补齐子 agent 递归导入、分批处理（batchSize=500）、`.meta.json` sidecar 写入
- **ForkSessionViaStore 增强**: 补齐 `upToMessageID` 参数支持、sidechain 过滤、标题自动推导
- **SessionStore 一致性测试**: 新增 `RunSessionStoreConformance` 14 项契约测试工具
- **SessionStore 预检逻辑**: 新增 `ValidateSessionStoreOptions` 检查不兼容选项组合
- **InMemorySessionStore 修复**: mtime 从硬编码 `1000` 改为 `time.Now().UnixMilli()`
- **Transport 修复**: 移除 `debug-to-stderr` 向后兼容检测，与 Python v0.1.60+ 行为一致

## Capabilities

### New Capabilities
- `session-resume`: 从 SessionStore 加载会话到临时目录，支持 CLI 子进程 resume。包括 auth 文件复制、subkey 物化、路径安全校验、cleanup 重试
- `session-store-conformance`: 14 项 SessionStore 契约一致性测试工具，支持自定义 store 实现验证
- `session-store-validation`: SessionStore 选项组合预检，快速报错不兼容配置

### Modified Capabilities
- `fold-session-summary`: 增量摘要推导逻辑全面对齐 Python——新增 set-once 字段（is_sidechain/created_at/cwd）、first-prompt 过滤（meta/slash-command/tool_result/auto-generated）、last-wins 字段名映射到 snake_case
- `import-session-to-store`: 增强为支持子 agent 递归导入、分批 append、.meta.json sidecar
- `fork-session-via-store`: 新增 upToMessageID 参数、sidechain 过滤、标题自动推导
- `in-memory-session-store`: mtime 改为真实 epoch-ms 时间戳

## Impact

- **新增文件**: `session_resume.go`, `session_store_validation.go`, `session_resume_test.go`, `session_store_conformance_test.go`, `session_summary_test.go`
- **修改文件**: `sessions.go` (FoldSessionSummary 重写, ImportSessionToStore 增强), `session_mutations.go` (ForkSessionViaStore), `session_store.go` (mtime 修复), `process_query.go` (集成 session resume + validation), `transport.go` (移除 debug-to-stderr)
- **API 变更**: `FoldSessionSummary` 签名变更（移除 mtime 参数）, `ForkSessionViaStore` 签名变更（新增 upToMessageID）, `ImportSessionToStore` 签名变更（新增 includeSubagents 变参）
- **测试**: 新增 ~30 个测试用例覆盖所有新增/修改功能
