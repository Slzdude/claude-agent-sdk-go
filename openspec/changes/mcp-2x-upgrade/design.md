# MCP 2.x 升级设计

## 架构概述

```
┌─────────────────────────────────────────────────────────────────────┐
│                    MCP 2.x 升级架构                                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌──────────────┐                                                  │
│  │ mcp.go       │  类型定义层                                       │
│  │              │  ├── 新增: MCPAudioContent                       │
│  │              │  ├── 新增: MCPResourceLink                       │
│  │              │  ├── 新增: MCPResourceTemplate                   │
│  │              │  ├── 扩展: MCPTool.OutputSchema                  │
│  │              │  └── 扩展: ToolResult.StructuredContent           │
│  └──────────────┘                                                  │
│         │                                                          │
│         ▼                                                          │
│  ┌──────────────┐                                                  │
│  │ query_       │  协议处理层                                       │
│  │ protocol.go  │  ├── 更新: initialize 响应                       │
│  │              │  ├── 新增: 版本协商逻辑                          │
│  │              │  ├── 更新: tools/call (Output Schema 验证)       │
│  │              │  └── 更新: resources/list (ResourceTemplate)     │
│  └──────────────┘                                                  │
│         │                                                          │
│         ▼                                                          │
│  ┌──────────────┐                                                  │
│  │ SdkMcpServer │  服务器接口层                                     │
│  │ 接口         │  ├── 保持不变                                    │
│  │              │  └── Output Schema 通过 ToolResult 传递          │
│  └──────────────┘                                                  │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

## 详细设计

### 1. 协议版本协商

#### 1.1 版本常量

```go
// 支持的协议版本
const (
    MCPProtocolVersion20241105 = "2024-11-05"
    MCPProtocolVersion20250326 = "2025-03-26"
    MCPProtocolVersion20250618 = "2025-06-18"
    MCPProtocolVersion20251125 = "2025-11-25"
)

// 默认协商版本（当客户端版本不在支持列表中时使用）
const MCPDefaultNegotiatedVersion = MCPProtocolVersion20250326

// 支持的版本列表
var MCPSupportedProtocolVersions = []string{
    MCPProtocolVersion20241105,
    MCPProtocolVersion20250326,
    MCPProtocolVersion20250618,
    MCPProtocolVersion20251125,
}
```

#### 1.2 版本协商逻辑

```go
func negotiateProtocolVersion(clientVersion string) string {
    for _, v := range MCPSupportedProtocolVersions {
        if v == clientVersion {
            return clientVersion // 回显客户端版本
        }
    }
    return MCPDefaultNegotiatedVersion // 返回默认版本
}
```

#### 1.3 initialize 响应更新

```go
case "initialize":
    clientVersion := strVal(params, "protocolVersion")
    negotiatedVersion := negotiateProtocolVersion(clientVersion)
    
    return buildResponse(map[string]any{
        "protocolVersion": negotiatedVersion,
        "capabilities": map[string]any{
            "tools":     map[string]any{},
            "resources": map[string]any{},
            "prompts":   map[string]any{},
        },
        "serverInfo": map[string]any{
            "name":    serverName,
            "version": "1.0.0",
        },
    }), nil
```

### 2. 新内容类型

#### 2.1 AudioContent

```go
// MCPAudioContent represents audio content in MCP responses.
type MCPAudioContent struct {
    Type     string `json:"type"`     // "audio"
    Data     string `json:"data"`     // base64 encoded audio
    MimeType string `json:"mimeType"` // e.g. "audio/wav"
}
```

#### 2.2 ResourceLink

```go
// MCPResourceLink represents a link to a resource.
type MCPResourceLink struct {
    Type        string `json:"type"`        // "resource_link"
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

#### 2.3 ResourceTemplate

```go
// MCPResourceTemplate represents a resource template with URI pattern.
type MCPResourceTemplate struct {
    URITemplate string `json:"uriTemplate"` // RFC 6570 URI template
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

### 3. Output Schema 支持

#### 3.1 MCPTool 扩展

```go
type MCPTool struct {
    Name         string           `json:"name"`
    Description  string           `json:"description,omitempty"`
    InputSchema  map[string]any   `json:"inputSchema"`
    OutputSchema map[string]any   `json:"outputSchema,omitempty"` // 新增
    Annotations  *ToolAnnotations `json:"annotations,omitempty"`
    Meta         map[string]any   `json:"_meta,omitempty"`
}
```

#### 3.2 ToolResult 扩展

```go
type ToolResult struct {
    Content           []map[string]any `json:"content"`
    StructuredContent map[string]any   `json:"structuredContent,omitempty"` // 新增
    IsError           bool             `json:"isError"`
}
```

#### 3.3 Output Schema 验证

在 `tools/call` 处理器中，如果工具定义了 `outputSchema`，验证 `structuredContent`：

```go
case "tools/call":
    // ... 调用工具 ...
    
    // 如果工具定义了 outputSchema，验证 structuredContent
    if tool.OutputSchema != nil && result.StructuredContent == nil {
        // 返回错误：需要 structuredContent
    }
```

### 4. ResourceTemplate 支持

#### 4.1 SdkMcpServer 接口扩展

```go
type SdkMcpServer interface {
    // 现有方法...
    ListResources(ctx context.Context) ([]MCPResource, error)
    ReadResource(ctx context.Context, uri string) (MCPResourceContent, error)
    
    // 新增方法（可选，通过类型断言检查）
    // ListResourceTemplates(ctx context.Context) ([]MCPResourceTemplate, error)
}
```

#### 4.2 resources/list 响应更新

```go
case "resources/list":
    resources, err := server.ListResources(ctx)
    if err != nil {
        return buildError(-32603, err.Error()), nil
    }
    
    result := map[string]any{"resources": resources}
    
    // 检查是否支持 ResourceTemplate
    if templateServer, ok := server.(interface{
        ListResourceTemplates(context.Context) ([]MCPResourceTemplate, error)
    }); ok {
        templates, err := templateServer.ListResourceTemplates(ctx)
        if err == nil {
            result["resourceTemplates"] = templates
        }
    }
    
    b, _ := json.Marshal(result)
    var resultMap map[string]any
    _ = json.Unmarshal(b, &resultMap)
    return buildResponse(resultMap), nil
```

### 5. 能力通告更新

```go
case "initialize":
    // ... 版本协商 ...
    
    capabilities := map[string]any{
        "tools":     map[string]any{},
        "resources": map[string]any{},
        "prompts":   map[string]any{},
    }
    
    // 检查是否支持 ResourceTemplate
    if _, ok := server.(interface{
        ListResourceTemplates(context.Context) ([]MCPResourceTemplate, error)
    }); ok {
        capabilities["resources"].(map[string]any)["templates"] = true
    }
```

## 文件变更

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `mcp.go` | 修改 | 添加新类型，扩展现有类型 |
| `query_protocol.go` | 修改 | 更新 initialize、tools/call、resources/list 处理器 |
| `mcp_test.go` | 新增 | 新功能的单元测试 |
| `mcp_extended_test.go` | 修改 | 更新现有测试 |

## 向后兼容性

1. **新字段都是可选的**：所有新增字段都使用 `omitempty` 标签
2. **版本协商透明**：客户端请求旧版本时返回旧版本
3. **接口不变**：SdkMcpServer 接口保持不变，新功能通过类型断言支持
4. **默认行为不变**：不设置新功能时行为与之前完全一致

## 测试策略

1. **单元测试**：每个新类型和函数都有对应的单元测试
2. **版本协商测试**：测试各种版本组合的协商结果
3. **Output Schema 测试**：测试 schema 验证逻辑
4. **ResourceTemplate 测试**：测试模板列表和能力通告
5. **向后兼容性测试**：确保现有测试全部通过
