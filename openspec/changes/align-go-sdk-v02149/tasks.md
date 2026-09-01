## 1. ResultError

- [ ] 1.1 errors.go: 添加 `_normalizeResultErrors(raw any) []string` 辅助函数
  - 输入为 string → 返回 `[]string{input}`
  - 输入为 []any → 提取 string 元素，过滤空白
  - 其他 → 返回空切片
  - **验收**: `_normalizeResultErrors("err")` = `["err"]`；`_normalizeResultErrors([]any{"a", "b"})` = `["a", "b"]`

- [ ] 1.2 errors.go: 添加 `ResultError` struct
  - 嵌入 `ProcessError`
  - 字段: Subtype string, Errors []string, Result string, APIErrorStatus *int, TerminalReason string, SessionID string, Data map[string]any
  - **验收**: `ResultError` 满足 `errors.As(err, &processErr)` 为 true

- [ ] 1.3 errors.go: 添加 `NewResultError(message string, data map[string]any, exitCode int) *ResultError`
  - 从 data 提取 subtype/errors/result/api_error_status/terminal_reason/session_id
  - 调用 `_normalizeResultErrors` 处理 errors
  - **验收**: `NewResultError("msg", {"subtype":"error_max_turns","errors":["too many"]}, 1)` 的 Subtype="error_max_turns", Errors=["too many"]

- [ ] 1.4 types.go: 添加 `errorResultText(message map[string]any) string`
  - 回退链: errors[] → result(非空) → subtype(非"success") → api_error_status → "unknown error"
  - **验收**: errors=["a","b"] → "a; b"；result="API Error" → "API Error"；subtype="error_max_turns" → "error_max_turns"；api_error_status=429 → "API error (HTTP 429)"

- [ ] 1.5 process_query.go: `_lastErrorResult` 追踪
  - 收到 result 消息且 is_error=true → 设置 `_lastErrorResult = message`
  - 收到 result 消息且 is_error=false → 清除 `_lastErrorResult = nil`
  - 收到非 session_state_changed 的 system 消息 → 清除 `_lastErrorResult = nil`
  - **验收**: error result → system(session_state_changed) → ProcessError → 替换为 ResultError

- [ ] 1.6 process_query.go: ProcessError 替换为 ResultError
  - 当 `_lastErrorResult != nil` 且收到 ProcessError 时:
    - 调用 `errorResultText(_lastErrorResult)` 获取错误文本
    - 创建 `NewResultError(errorText, _lastErrorResult, exitCode)`
    - 作为 pending_error 传递给待处理的控制请求
  - **验收**: ProcessError("exit code 1") + _lastErrorResult 有值 → ResultError("Claude Code returned an error result: ...")

- [ ] 1.7 测试: TestNewResultError
- [ ] 1.8 测试: TestNormalizeResultErrors
- [ ] 1.9 测试: TestErrorResultText

## 2. UserMessage.ParentAgentID

- [ ] 2.1 types.go: `UserMessage` 添加 `ParentAgentID string` 字段 (json: "parent_agent_id,omitempty")
  - **验收**: 零值不影响现有序列化

- [ ] 2.2 message_parser.go: `parseUserMessage` 中解析 `parent_agent_id`
  - `m.ParentAgentID = strVal(raw, "parent_agent_id")`
  - **验收**: 输入 `{"type":"user","message":...,"parent_agent_id":"agent-1"}` → ParentAgentID="agent-1"

- [ ] 2.3 测试: TestParseUserMessage_ParentAgentID

## 3. ForwardSubagentText

- [ ] 3.1 types.go: `ClaudeAgentOptions` 添加 `ForwardSubagentText bool` 字段
  - **验收**: 零值不影响现有行为

- [ ] 3.2 query_protocol.go: `queryProto` 添加 `forwardSubagentText bool` 字段 + `SetForwardSubagentText(v bool)` 方法
  - **验收**: 设置后字段值正确

- [ ] 3.3 query_protocol.go: `Initialize()` 中当 forwardSubagentText 为 true 时发送 `"forwardSubagentText": true`
  - **验收**: Initialize 请求 payload 包含 forwardSubagentText: true

- [ ] 3.4 claude.go: `NewClaudeSDKClient` 中调用 `q.SetForwardSubagentText(opts.ForwardSubagentText)`
  - **验收**: ClaudeSDKClient 创建后 queryProto 的 forwardSubagentText 与 opts 一致

- [ ] 3.5 transport.go: `buildCommand()` 中当 `opts.ForwardSubagentText` 为 true 时添加 `--forward-subagent-text`
  - **验收**: buildCommand() 返回包含 "--forward-subagent-text"

- [ ] 3.6 测试: TestBuildCommand_ForwardSubagentText

## 4. 验证

- [ ] 4.1 go build ./... 编译通过
- [ ] 4.2 go vet ./... 无警告
- [ ] 4.3 golangci-lint run ./... 0 issues
- [ ] 4.4 go test ./... 全部通过
