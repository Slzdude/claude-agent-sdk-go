# MCP 2.x 升级任务清单

## 阶段 1：类型定义 (mcp.go)

### 1.1 添加版本常量

**任务：** 在 `mcp.go` 中添加 MCP 协议版本常量和协商逻辑

**验收标准：**
- [x] 常量定义正确，值与 MCP 规范一致
- [x] `MCPSupportedProtocolVersions` 包含所有 4 个版本
- [x] `MCPDefaultNegotiatedVersion` 设置为 `2025-03-26`
- [x] 编译通过，无错误

---

### 1.2 添加新内容类型

**任务：** 在 `mcp.go` 中添加 AudioContent、ResourceLink、ResourceTemplate 结构体

**验收标准：**
- [x] 结构体定义正确，字段类型匹配 MCP 规范
- [x] JSON 标签正确（camelCase，omitempty）
- [x] 编译通过，无错误
- [x] 单元测试验证 JSON 序列化/反序列化

---

### 1.3 扩展现有类型

**任务：** 扩展 `MCPTool` 和 `ToolResult` 结构体

**验收标准：**
- [x] 新字段添加正确，类型匹配 MCP 规范
- [x] JSON 标签正确（omitempty）
- [x] 编译通过，无错误
- [x] 现有代码不受影响（向后兼容）
- [x] 单元测试验证新字段序列化

---

## 阶段 2：协议处理 (query_protocol.go)

### 2.1 版本协商

**任务：** 实现版本协商逻辑并更新 initialize 处理器

**验收标准：**
- [x] `negotiateProtocolVersion()` 函数实现正确
- [x] 客户端请求 `2024-11-05` → 响应 `2024-11-05`
- [x] 客户端请求 `2025-03-26` → 响应 `2025-03-26`
- [x] 客户端请求 `2025-06-18` → 响应 `2025-06-18`
- [x] 客户端请求 `2025-11-25` → 响应 `2025-11-25`
- [x] 客户端请求 `9999-99-99` → 响应 `2025-03-26`（默认版本）
- [x] 客户端请求空字符串 → 响应 `2025-03-26`（默认版本）
- [x] 单元测试覆盖所有版本组合

---

### 2.2 Output Schema 支持

**任务：** 更新 `tools/call` 处理器支持 Output Schema 验证

**验收标准：**
- [x] `tools/call` 正确处理 `structuredContent` 字段
- [x] `structuredContent` 正确序列化到响应中
- [x] 不定义 `outputSchema` 时行为不变
- [x] 定义 `outputSchema` 但不返回 `structuredContent` 时不报错（Go SDK 不强制验证）
- [x] 单元测试验证 `structuredContent` 序列化

---

### 2.3 ResourceTemplate 支持

**任务：** 更新 `resources/list` 处理器支持 ResourceTemplate

**验收标准：**
- [x] `resources/list` 正确返回 `resources` 数组
- [x] 如果服务器实现 `ListResourceTemplates()`，返回 `resourceTemplates` 数组
- [x] 如果服务器不实现 `ListResourceTemplates()`，不返回 `resourceTemplates`
- [x] `ResourceTemplate` 序列化正确（uriTemplate, name, description, mimeType）
- [x] 单元测试验证 ResourceTemplate 返回

---

### 2.4 能力通告更新

**任务：** 更新 `initialize` 响应中的 capabilities

**验收标准：**
- [x] 默认 capabilities 包含 `tools`、`resources`、`prompts`
- [x] 如果服务器支持 ResourceTemplate，`resources` 包含 `templates: true`
- [x] 如果服务器不支持 ResourceTemplate，`resources` 不包含 `templates`
- [x] 单元测试验证 capabilities 动态设置

---

## 阶段 3：测试

### 3.1 版本协商测试

**任务：** 在 `mcp_extended_test.go` 中添加版本协商测试

**验收标准：**
- [x] 测试覆盖所有 4 个支持的版本
- [x] 测试不支持的版本返回默认版本
- [x] 测试空字符串返回默认版本
- [x] 所有测试通过

---

### 3.2 新内容类型测试

**任务：** 在 `mcp_extended_test.go` 中添加新内容类型测试

**验收标准：**
- [x] AudioContent 序列化/反序列化正确
- [x] ResourceLink 序列化/反序列化正确
- [x] ResourceTemplate 序列化/反序列化正确
- [x] 所有测试通过

---

### 3.3 Output Schema 测试

**任务：** 在 `mcp_extended_test.go` 中添加 Output Schema 测试

**验收标准：**
- [x] 工具定义包含 OutputSchema 时正确序列化
- [x] tools/call 返回 structuredContent 时正确处理
- [x] 不定义 OutputSchema 时行为不变
- [x] 所有测试通过

---

### 3.4 ResourceTemplate 测试

**任务：** 在 `mcp_extended_test.go` 中添加 ResourceTemplate 测试

**验收标准：**
- [x] resources/list 返回 ResourceTemplate 时正确处理
- [x] capabilities 动态包含 templates
- [x] 不支持 ResourceTemplate 时不返回
- [x] 所有测试通过

---

### 3.5 向后兼容性测试

**任务：** 运行所有现有测试确保通过

**验收标准：**
- [x] `go test ./... -count=1` 全部通过
- [x] `go vet ./...` 无错误
- [x] `golangci-lint run` 无问题
- [x] 不使用新功能时行为与之前完全一致

---

## 阶段 4：文档和示例

### 4.1 文档更新

**任务：** 更新 README 中的 MCP 相关文档

**验收标准：**
- [ ] README 包含 MCP 2.x 功能说明
- [ ] 文档描述版本协商行为
- [ ] 文档描述新内容类型
- [ ] 文档描述 Output Schema 支持

---

### 4.2 示例更新

**任务：** 更新 mcp_tools 示例使用新功能

**验收标准：**
- [ ] 示例展示 Output Schema 使用
- [ ] 示例展示 ResourceTemplate 使用
- [ ] 示例可以正常运行

---

## 依赖关系

```
阶段 1.1 (版本常量) ✅
    ↓
阶段 1.2 (新内容类型) ✅
    ↓
阶段 1.3 (扩展类型) ✅
    ↓
阶段 2.1 (版本协商) ✅
    ↓
阶段 2.2 (Output Schema) ✅
    ↓
阶段 2.3 (ResourceTemplate) ✅
    ↓
阶段 2.4 (能力通告) ✅
    ↓
阶段 3 (测试) ✅
    ↓
阶段 4 (文档) ⏳
```

## 总体验收标准

1. **功能完整性**：所有 MCP 2.x 功能实现（版本协商、新内容类型、Output Schema、ResourceTemplate）✅
2. **向后兼容性**：所有现有测试通过，不使用新功能时行为不变 ✅
3. **测试覆盖**：新功能有完整的单元测试覆盖 ✅
4. **代码质量**：`go vet`、`golangci-lint` 无问题 ✅
5. **文档完整**：README 和示例更新 ⏳
6. **Python SDK 对齐**：与 Python SDK 行为一致（不实现 Task System 和 Elicitation）✅
