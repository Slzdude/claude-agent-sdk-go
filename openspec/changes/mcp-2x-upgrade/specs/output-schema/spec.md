# MCP Output Schema 规范

## 概述

定义 Go SDK 如何支持 MCP 工具的 Output Schema 验证和 structuredContent。

## Output Schema

### 定义

Output Schema 是工具输出的 JSON Schema 定义，用于验证 `structuredContent` 字段。

```go
type MCPTool struct {
    // ... 其他字段 ...
    OutputSchema map[string]any `json:"outputSchema,omitempty"`
}
```

### 示例

```json
{
  "name": "get_weather",
  "description": "Get current weather",
  "inputSchema": {
    "type": "object",
    "properties": {
      "location": {"type": "string"}
    },
    "required": ["location"]
  },
  "outputSchema": {
    "type": "object",
    "properties": {
      "temperature": {"type": "number"},
      "condition": {"type": "string"},
      "humidity": {"type": "number"}
    },
    "required": ["temperature", "condition"]
  }
}
```

## StructuredContent

### 定义

StructuredContent 是工具调用的结构化输出，必须符合 Output Schema。

```go
type ToolResult struct {
    Content           []map[string]any `json:"content"`
    StructuredContent map[string]any   `json:"structuredContent,omitempty"`
    IsError           bool             `json:"isError"`
}
```

### 示例

```json
{
  "content": [
    {"type": "text", "text": "Temperature: 72°F, Condition: Sunny"}
  ],
  "structuredContent": {
    "temperature": 72,
    "condition": "Sunny",
    "humidity": 45
  },
  "isError": false
}
```

## 验证逻辑

### tools/call 处理器

```go
case "tools/call":
    // 1. 调用工具获取结果
    result, err := server.CallTool(ctx, toolName, toolArgs)
    
    // 2. 如果工具定义了 outputSchema
    if tool.OutputSchema != nil {
        // 2.1 检查 structuredContent 是否存在
        if result.StructuredContent == nil {
            return buildError(-32603, "Tool defines outputSchema but no structuredContent returned"), nil
        }
        
        // 2.2 验证 structuredContent 符合 schema
        // 注意：Go SDK 不实现完整的 JSON Schema 验证
        // 只检查 structuredContent 是否存在
    }
    
    // 3. 序列化结果
    b, _ := json.Marshal(result)
    var resultMap map[string]any
    _ = json.Unmarshal(b, &resultMap)
    return buildResponse(resultMap), nil
```

## 实现要求

1. **OutputSchema 是可选的**：工具可以不定义 OutputSchema
2. **StructuredContent 是可选的**：工具可以不返回 StructuredContent
3. **如果定义了 OutputSchema，StructuredContent 应该存在**：这是建议，不是强制
4. **Go SDK 不实现完整的 JSON Schema 验证**：只检查字段是否存在

## 测试用例

1. 工具定义包含 OutputSchema
2. 工具返回 StructuredContent
3. 工具定义 OutputSchema 但不返回 StructuredContent（应该返回错误）
4. 工具不定义 OutputSchema（正常返回）
5. StructuredContent 符合 Output Schema
6. StructuredContent 不符合 Output Schema（Go SDK 不验证，只检查存在性）
