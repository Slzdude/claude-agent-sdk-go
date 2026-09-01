## 1. MCP 扩展类型

- [ ] 1.1 mcp.go: 添加 `MCPResource` struct
  - 字段: `URI string`, `Name string`, `Description string`, `MimeType string`
  - json tag: `uri`, `name`, `description,omitempty`, `mimeType,omitempty`
  - **验收**: 可编译，json 序列化正确

- [ ] 1.2 mcp.go: 添加 `MCPResourceContent` struct
  - 字段: `URI string`, `MimeType string`, `Text string`, `Blob string`
  - json tag: `uri`, `mimeType,omitempty`, `text,omitempty`, `blob,omitempty`
  - **验收**: 可编译，json 序列化正确

- [ ] 1.3 mcp.go: 添加 `MCPPrompt` struct
  - 字段: `Name string`, `Description string`, `Arguments []MCPPromptArg`
  - json tag: `name`, `description,omitempty`, `arguments,omitempty`
  - **验收**: 可编译，json 序列化正确

- [ ] 1.4 mcp.go: 添加 `MCPPromptArg` struct
  - 字段: `Name string`, `Description string`, `Required bool`
  - json tag: `name`, `description,omitempty`, `required,omitempty`
  - **验收**: 可编译，json 序列化正确

- [ ] 1.5 mcp.go: 添加 `MCPPromptResult` struct
  - 字段: `Description string`, `Messages []MCPPromptResultMessage`
  - json tag: `description,omitempty`, `messages`
  - **验收**: 可编译，json 序列化正确

- [ ] 1.6 mcp.go: 添加 `MCPPromptResultMessage` struct
  - 字段: `Role string`, `Content map[string]any`
  - json tag: `role`, `content`
  - **验收**: 可编译，json 序列化正确

## 2. SdkMcpServer 接口扩展

- [ ] 2.1 mcp.go: `SdkMcpServer` 接口添加 `ListResources(ctx context.Context) ([]MCPResource, error)` 方法
  - 现有实现需要添加空实现: `func (s *MyServer) ListResources(ctx context.Context) ([]MCPResource, error) { return nil, nil }`
  - **验收**: 接口编译通过，现有实现不报错

- [ ] 2.2 mcp.go: `SdkMcpServer` 接口添加 `ReadResource(ctx context.Context, uri string) (MCPResourceContent, error)` 方法
  - 现有实现需要添加空实现: `func (s *MyServer) ReadResource(ctx context.Context, uri string) (MCPResourceContent, error) { return MCPResourceContent{}, nil }`
  - **验收**: 接口编译通过

- [ ] 2.3 mcp.go: `SdkMcpServer` 接口添加 `ListPrompts(ctx context.Context) ([]MCPPrompt, error)` 方法
  - 现有实现需要添加空实现
  - **验收**: 接口编译通过

- [ ] 2.4 mcp.go: `SdkMcpServer` 接口添加 `GetPrompt(ctx context.Context, name string, arguments map[string]any) (MCPPromptResult, error)` 方法
  - 现有实现需要添加空实现
  - **验收**: 接口编译通过

## 3. MCP 方法处理器

- [ ] 3.1 query_protocol.go: `handleMCPMessage` 添加 `resources/list` case
  - 调用 `server.ListResources(ctx)`
  - 响应格式: `{"mcp_response": {"jsonrpc": "2.0", "id": msgID, "result": {"resources": [...]}}}`
  - 错误处理: 返回 `-32603 internal error`
  - **验收**: 返回正确的 JSON-RPC 响应

- [ ] 3.2 query_protocol.go: `handleMCPMessage` 添加 `resources/read` case
  - 从 params 提取 `uri`
  - 调用 `server.ReadResource(ctx, uri)`
  - 响应格式: `{"mcp_response": {"jsonrpc": "2.0", "id": msgID, "result": {"contents": [...]}}}`
  - **验收**: 返回正确的 JSON-RPC 响应

- [ ] 3.3 query_protocol.go: `handleMCPMessage` 添加 `prompts/list` case
  - 调用 `server.ListPrompts(ctx)`
  - 响应格式: `{"mcp_response": {"jsonrpc": "2.0", "id": msgID, "result": {"prompts": [...]}}}`
  - **验收**: 返回正确的 JSON-RPC 响应

- [ ] 3.4 query_protocol.go: `handleMCPMessage` 添加 `prompts/get` case
  - 从 params 提取 `name` 和 `arguments`
  - 调用 `server.GetPrompt(ctx, name, arguments)`
  - 响应格式: `{"mcp_response": {"jsonrpc": "2.0", "id": msgID, "result": {...}}}`
  - **验收**: 返回正确的 JSON-RPC 响应

- [ ] 3.5 query_protocol.go: `handleMCPMessage` 添加 `ping` case
  - 返回空响应: `{"mcp_response": {"jsonrpc": "2.0", "id": msgID, "result": {}}}`
  - **验收**: 返回正确的 JSON-RPC 响应

- [ ] 3.6 query_protocol.go: 更新 `initialize` 响应
  - capabilities 添加 `resources: {}` 和 `prompts: {}`
  - **验收**: initialize 响应包含 tools、resources、prompts capabilities

## 4. 取消支持

- [ ] 4.1 query_protocol.go: `queryProto` 添加 `mcpInflight map[string]context.CancelFunc` 字段
  - 用于追踪正在执行的 MCP 请求
  - **验收**: 字段可编译

- [ ] 4.2 query_protocol.go: `handleMCPMessage` 添加 `notifications/cancelled` case
  - 从 params 提取 `requestId`
  - 查找并调用对应的 CancelFunc
  - 删除映射表条目
  - 无需响应（notification）
  - **验收**: 取消正在执行的请求

- [ ] 4.3 query_protocol.go: 为每个 MCP 请求创建可取消的 context
  - 在 `handleMCPMessage` 开头创建 `ctx, cancel := context.WithCancel(ctx)`
  - 将 cancel 存入 `mcpInflight[msgID]`
  - 请求完成后删除映射表条目
  - **验收**: 请求可通过 notifications/cancelled 取消

## 5. 测试

- [ ] 5.1 mcp_test.go: TestMCPResourcesListAndRead
  - 实现一个简单的 MCP server，返回 2 个 resources
  - 验证 resources/list 返回正确的 resources
  - 验证 resources/read 返回正确的 content

- [ ] 5.2 mcp_test.go: TestMCPPromptsListAndGet
  - 实现一个简单的 MCP server，返回 1 个 prompt
  - 验证 prompts/list 返回正确的 prompts
  - 验证 prompts/get 返回正确的 result

- [ ] 5.3 mcp_test.go: TestMCPPing
  - 验证 ping 返回空响应

- [ ] 5.4 mcp_test.go: TestMCPCancelRequest
  - 发送 notifications/cancelled 请求
  - 验证对应的请求被取消

- [ ] 5.5 mcp_test.go: TestMCPUnknownMethod
  - 发送未知方法请求
  - 验证返回 -32601 错误

## 6. 验证

- [ ] 6.1 go build ./... 编译通过
- [ ] 6.2 go vet ./... 无警告
- [ ] 6.3 go test ./... 全部通过
