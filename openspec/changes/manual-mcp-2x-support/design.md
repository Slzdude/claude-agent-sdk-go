## Context

Go SDK 的 `handleMCPMessage` 手动处理 JSON-RPC，只支持 tools。Python SDK 使用 `mcp.server.Server` 支持完整 MCP 协议。需要在不引入第三方库的情况下扩展支持。

## Goals / Non-Goals

**Goals:**
- 扩展 SdkMcpServer 接口支持 resources/prompts
- 添加 MCP 方法处理器
- 添加请求取消支持
- 添加 session 状态追踪

**Non-Goals:**
- 不引入第三方 MCP 库
- 不实现 MCP client（只实现 server 端）
- 不实现 sampling/elicitation/roots 等客户端发起的方法

## Decisions

### 1. 接口方法可选

新增的 `ListResources`/`ReadResource`/`ListPrompts`/`GetPrompt` 方法通过空实现或返回错误来处理未实现的情况。用户可以选择只实现 `ListTools`/`CallTool`。

### 2. 取消通过 context 实现

`notifications/cancelled` 通过取消对应请求的 context 来实现，不需要额外的 goroutine 管理。

### 3. Session 状态简单追踪

只追踪 `initialized` 状态，用于判断是否可以处理请求。

## Risks / Trade-offs

- 手动实现 MCP 协议需要持续维护
- 新增接口方法可能影响现有用户
