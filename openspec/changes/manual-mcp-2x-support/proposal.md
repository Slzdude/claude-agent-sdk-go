## Why

Go SDK 当前手动处理 MCP JSON-RPC 协议，只支持 `initialize`、`tools/list`、`tools/call`。Python SDK 使用 `mcp.server.Server` 库支持完整 MCP 协议（tools、resources、prompts、ping、取消）。需要在不引入第三方 MCP 库的情况下，扩展 Go SDK 的 MCP 支持以对齐 Python SDK。

## What Changes

- **扩展 SdkMcpServer 接口**: 添加 ListResources/ReadResource/ListPrompts/GetPrompt 可选方法
- **新增 MCP 资源/提示类型**: MCPResource, MCPResourceContent, MCPPrompt, MCPPromptResult
- **扩展 handleMCPMessage**: 添加 resources/list, resources/read, prompts/list, prompts/get, ping 处理
- **取消支持**: notifications/cancelled → 取消正在执行的请求
- **Session 状态追踪**: 记录 initialize 状态，用于协议版本协商
- **initialize 响应更新**: 声明 capabilities（tools + resources + prompts）

## Capabilities

### New Capabilities
- `mcp-extended-interface`: SdkMcpServer 扩展接口
- `mcp-method-handlers`: MCP 方法处理器
- `mcp-cancellation`: 请求取消支持
- `mcp-session-lifecycle`: Session 状态追踪

### Modified Capabilities
- (无)

## Impact

- `mcp.go` — 新类型 + 接口扩展
- `query_protocol.go` — handleMCPMessage 扩展 + 取消支持
- 新增测试文件
