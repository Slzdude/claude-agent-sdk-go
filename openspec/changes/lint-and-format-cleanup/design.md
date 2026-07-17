## Context

golangci-lint 报告核心库有 4 个 unused 函数、1 个 ineffassign、7 个 errcheck。gofmt 报告 2 个文件格式问题。errcheck（defer Close/RemoveAll）是清理操作，失败不影响功能，显式忽略即可。

## Goals / Non-Goals

**Goals:**
- 删除未使用的内部函数
- 修复 gofmt 格式问题
- 修复 ineffassign

**Non-Goals:**
- 不删除公共 API 函数（getClaudeConfigDirWithOverride, getProjectsWithOverride）
- 不修复测试文件的 errcheck（低优先级）

## Decisions

### 1. 保留公共 API 函数

`getClaudeConfigDirWithOverride` 和 `getProjectsWithOverride` 是公共 API，外部消费者可能调用。保留。

### 2. errcheck 用 `_ =` 显式忽略

清理操作的 errcheck 用 `_ =` 显式忽略，而非修改逻辑。
