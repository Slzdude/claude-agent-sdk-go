# MCP 2.x 升级提案

## 概述

将 Go SDK 的 MCP 实现从 `2024-11-05` 升级到支持多版本协商（`2024-11-05`, `2025-03-26`, `2025-06-18`, `2025-11-25`），并添加 MCP 2.x 新增的内容类型和功能。

## 背景

### 当前状态

Go SDK 的 MCP 实现基于 `2024-11-05` 协议版本，支持：
- `initialize` / `notifications/initialized`
- `tools/list` / `tools/call`
- `resources/list` / `resources/read`
- `prompts/list` / `prompts/get`
- `ping`
- `notifications/cancelled`

### Python SDK 分析结果

通过分析 Python SDK 源码，发现：

1. **Task System**：Python SDK **未实现** MCP Task System 协议（tasks/get, tasks/result, tasks/cancel, tasks/list）
2. **Elicitation**：Python SDK **未实现**，收到 `elicitation/create` 请求时直接返回 `-32601` 错误拒绝
3. **版本协商**：Python SDK 委托给 mcp 库，支持多版本协商
4. **能力通告**：Python SDK 委托给 mcp 库，SDK 创建的服务器只通告 `tools` 能力

### 升级范围

基于 Python SDK 的实际实现，Go SDK 的升级范围应与 Python SDK 保持一致：

**需要实现：**
1. 协议版本更新和多版本协商
2. 新内容类型（AudioContent, ResourceLink）
3. Output Schema 支持
4. ResourceTemplate 支持

**不需要实现（Python SDK 也未实现）：**
- ❌ Task System
- ❌ Elicitation
- ❌ Completions 能力
- ❌ Server-to-client 请求（sampling, roots）

## 目标

1. **协议版本协商**：支持客户端请求的协议版本，返回兼容版本
2. **新内容类型**：支持 AudioContent 和 ResourceLink
3. **Output Schema**：支持工具输出验证和 structuredContent
4. **ResourceTemplate**：支持资源模板发现
5. **完全对齐 Python SDK**：确保 Go SDK 的 MCP 实现与 Python SDK 行为一致

## 范围

### 范围内

- 更新 `initialize` 响应中的协议版本
- 添加版本协商逻辑
- 添加 AudioContent 类型
- 添加 ResourceLink 类型
- 添加 ResourceTemplate 类型
- 扩展 MCPTool 支持 OutputSchema
- 扩展 ToolResult 支持 StructuredContent
- 更新能力通告
- 添加单元测试和集成测试

### 范围外

- Task System（Python SDK 未实现）
- Elicitation（Python SDK 未实现）
- Completions 能力
- Server-to-client 请求

## 风险

1. **版本协商复杂性**：需要正确处理多个协议版本的差异
2. **向后兼容性**：确保现有用户代码不受影响
3. **CLI 兼容性**：需要验证 Claude Code CLI 是否支持新版本

## 成功标准

1. 所有现有测试通过
2. 新功能有完整的单元测试覆盖
3. 与 Python SDK 行为一致
4. 版本协商正确工作
