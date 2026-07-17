## 1. FoldSessionSummary 对齐

- [x] 1.1 重写 `FoldSessionSummary`：移除 mtime 参数，添加 set-once 字段（is_sidechain/created_at/cwd）
- [x] 1.2 添加 first-prompt 提取逻辑：过滤 isMeta/isCompactSummary/tool_result/slash-command/auto-generated
- [x] 1.3 添加 slash-command fallback 和 first_prompt_locked 机制
- [x] 1.4 修正 last-wins 字段名映射（customTitle→custom_title 等）
- [x] 1.5 修正 tag 空值清除逻辑
- [x] 1.6 添加 isoToEpochMs、entryTextBlocks、foldFirstPrompt 辅助函数
- [x] 1.7 更新 `InMemorySessionStore.Append` 调用新签名
- [x] 1.8 编写 8 个 FoldSessionSummary 单元测试

## 2. Session Resume 实现

- [x] 2.1 创建 `session_resume.go`：`MaterializedResume` struct
- [x] 2.2 实现 `MaterializeResumeSession`：从 store 加载 + 写临时目录
- [x] 2.3 实现 `ApplyMaterializedOptions`：覆盖 env/resume
- [x] 2.4 实现 `copyAuthFiles`：复制 .credentials.json（refreshToken 脱敏）和 .claude.json
- [x] 2.5 实现 `materializeSubkeys`：递归加载子 agent transcripts + .meta.json
- [x] 2.6 实现 `isSafeSubpath`：路径安全校验（防目录穿越）
- [x] 2.7 实现 `rmtreeWithRetry`：带重试的清理
- [x] 2.8 集成到 `processQuery()`：ValidateSessionStoreOptions → MaterializeResumeSession → ApplyMaterializedOptions
- [x] 2.9 编写 10 个 session resume 单元测试

## 3. ForkSessionViaStore 增强

- [x] 3.1 添加 `upToMessageID` 参数
- [x] 3.2 添加 sidechain 过滤逻辑
- [x] 3.3 添加 `deriveTitleFromEntries` 标题自动推导

## 4. ImportSessionToStore 增强

- [x] 4.1 添加子 agent 递归导入（`importSubagentDir`）
- [x] 4.2 添加分批处理（batchSize=500）
- [x] 4.3 添加 .meta.json sidecar 导入

## 5. SessionStore 一致性测试

- [x] 5.1 创建 `session_store_conformance_test.go`：14 项契约测试
- [x] 5.2 `RunSessionStoreConformance` 可复用测试工具函数

## 6. SessionStore 预检逻辑

- [x] 6.1 创建 `session_store_validation.go`：`ValidateSessionStoreOptions`
- [x] 6.2 检查 continue_conversation + session_store 组合
- [x] 6.3 检查 session_store + enable_file_checkpointing 组合

## 7. InMemorySessionStore 修复

- [x] 7.1 mtime 从硬编码 `1000` 改为 `time.Now().UnixMilli()`

## 8. Transport 修复

- [x] 8.1 移除 `debug-to-stderr` 向后兼容检测

## 9. 最终验证

- [x] 9.1 `go build ./...` 编译通过
- [x] 9.2 `go vet ./...` 无警告
- [x] 9.3 `go test ./...` 所有测试通过（~130 个测试）
