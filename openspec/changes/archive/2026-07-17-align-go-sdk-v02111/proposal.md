## Why

Python SDK v0.2.111 包含 3 个重要修复：can_use_tool 遮蔽警告、shielded close 防止僵尸子进程、NDJSON 行帧重组修复大行丢失。需要逐项迁移到 Go SDK。

## What Changes

- **CanUseToolShadowedWarning**: 当 `can_use_tool` 被 `allowed_tools` 或 `bypassPermissions` 遮蔽时输出警告
- **Shielded close**: `close()` 中写锁获取添加超时，`_ACTIVE_CHILDREN` 仅在成功回收后丢弃
- **NDJSON line framing**: stdout 解析改为行帧模式（strip + 跳过非 JSON + JSON 失败报错）
- **--resume=value 格式**: 安全修复，防止 flag 注入
- **message_parser 防御**: content block 非 map 时返回错误而非 panic

## Capabilities

### New Capabilities
- `can-use-tool-shadowed-warning`: can_use_tool 遮蔽检测和警告

### Modified Capabilities
- `ndjson-parsing`: stdout 解析改为行帧模式
- `transport-close`: close 超时保护和子进程回收逻辑
- `cli-flag-format`: --resume/--session-id 使用 = 格式

## Impact

- `types.go` — 新增 CanUseToolShadowedWarning 逻辑
- `transport.go` — close 超时、--resume=value、NDJSON 解析
- `message_parser.go` — content block 防御
- `process_query.go` / `claude.go` — 警告集成
- 新增测试
