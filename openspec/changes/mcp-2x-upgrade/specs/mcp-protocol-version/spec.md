# MCP 协议版本协商规范

## 概述

定义 Go SDK 如何处理 MCP 协议版本协商，确保与客户端的兼容性。

## 协议版本

### 支持的版本

| 版本 | 说明 |
|------|------|
| `2024-11-05` | 初始版本，Go SDK 当前版本 |
| `2025-03-26` | 默认协商版本 |
| `2025-06-18` | 中间版本 |
| `2025-11-25` | 最新版本 |

### 版本协商逻辑

1. 客户端发送 `initialize` 请求，包含 `protocolVersion` 字段
2. 服务器检查客户端版本是否在支持列表中
3. 如果支持，回显客户端版本
4. 如果不支持，返回 `MCPDefaultNegotiatedVersion`（`2025-03-26`）

### initialize 请求格式

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "roots": {"listChanged": true},
      "sampling": {}
    },
    "clientInfo": {
      "name": "claude-code",
      "version": "1.0.0"
    }
  }
}
```

### initialize 响应格式

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-03-26",
    "capabilities": {
      "tools": {},
      "resources": {},
      "prompts": {}
    },
    "serverInfo": {
      "name": "my-server",
      "version": "1.0.0"
    }
  }
}
```

## 实现要求

1. **版本协商必须在 initialize 处理器中完成**
2. **必须回显客户端版本（如果支持）**
3. **必须返回默认版本（如果不支持）**
4. **必须在 capabilities 中通告支持的功能**

## 测试用例

1. 客户端请求 `2024-11-05` → 响应 `2024-11-05`
2. 客户端请求 `2025-03-26` → 响应 `2025-03-26`
3. 客户端请求 `2025-06-18` → 响应 `2025-06-18`
4. 客户端请求 `2025-11-25` → 响应 `2025-11-25`
5. 客户端请求 `9999-99-99` → 响应 `2025-03-26`（默认版本）
