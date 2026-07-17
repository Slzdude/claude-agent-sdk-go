## Why

golangci-lint 报告 12 个核心库问题（4 个未使用函数、1 个 ineffassign、7 个 errcheck），gofmt 报告 2 个文件格式问题。需要清理以保持代码质量。

## What Changes

- **删除死代码**: `parseMirrorErrorMessage`、`appendIfMissing` 未被调用
- **gofmt 格式化**: `types.go`、`message_parser_test.go` struct 字段对齐
- **ineffassign 修复**: `session_resume.go` 中 `claudeJSONSrc` 赋值后未使用

## Capabilities

### New Capabilities
- (无)

### Modified Capabilities
- (无)

## Impact

- `message_parser.go` — 删除 parseMirrorErrorMessage
- `transport.go` — 删除 appendIfMissing
- `types.go` — gofmt 格式化
- `message_parser_test.go` — gofmt 格式化
- `session_resume.go` — ineffassign 修复
