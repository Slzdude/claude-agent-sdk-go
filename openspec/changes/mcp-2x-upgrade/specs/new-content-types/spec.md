# MCP 新内容类型规范

## 概述

定义 Go SDK 新增的 MCP 内容类型：AudioContent 和 ResourceLink。

## AudioContent

### 定义

```go
type MCPAudioContent struct {
    Type     string `json:"type"`     // "audio"
    Data     string `json:"data"`     // base64 encoded audio
    MimeType string `json:"mimeType"` // e.g. "audio/wav", "audio/mp3"
}
```

### 使用场景

- 工具返回音频数据
- 资源包含音频内容
- 提示包含音频消息

### 示例

```json
{
  "type": "audio",
  "data": "UklGRnoGAABXQVZFZm10IBAAAAABAAEAQB8AAEAfAAABAAgAZGF0YQ==",
  "mimeType": "audio/wav"
}
```

## ResourceLink

### 定义

```go
type MCPResourceLink struct {
    Type        string `json:"type"`        // "resource_link"
    URI         string `json:"uri"`
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

### 使用场景

- 工具结果中引用资源
- 提示中引用资源
- 资源之间相互引用

### 示例

```json
{
  "type": "resource_link",
  "uri": "file:///path/to/document.pdf",
  "name": "document.pdf",
  "description": "Project documentation",
  "mimeType": "application/pdf"
}
```

## ResourceTemplate

### 定义

```go
type MCPResourceTemplate struct {
    URITemplate string `json:"uriTemplate"` // RFC 6570 URI template
    Name        string `json:"name"`
    Description string `json:"description,omitempty"`
    MimeType    string `json:"mimeType,omitempty"`
}
```

### 使用场景

- 动态资源发现
- 参数化资源访问
- 资源列表中的模板

### 示例

```json
{
  "uriTemplate": "file:///{path}",
  "name": "Local Files",
  "description": "Access local filesystem files",
  "mimeType": "application/octet-stream"
}
```

## 实现要求

1. **所有新类型必须支持 JSON 序列化**
2. **所有字段必须使用 `omitempty` 标签（可选字段）**
3. **必须在 tools/call 结果中正确处理这些类型**
4. **必须在 resources/list 响应中正确返回 ResourceTemplate**

## 测试用例

1. AudioContent 序列化/反序列化
2. ResourceLink 序列化/反序列化
3. ResourceTemplate 序列化/反序列化
4. tools/call 返回 AudioContent
5. tools/call 返回 ResourceLink
6. resources/list 返回 ResourceTemplate
