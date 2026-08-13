## 1. New Types

- [ ] 1.1 types.go: Add `MessageOriginKind` type alias (9 值: human/channel/peer/task-notification/coordinator/unclassified/observer/auto-continuation/observer-activity)
- [ ] 1.2 types.go: Add `TaskNotificationOriginSubkind` type alias (2 值: scheduled-trigger/peer-send-message)
- [ ] 1.3 types.go: Add `MessageOrigin` struct (Kind required, Server/From/Name/FromSession/SenderTaskId/Body/VerifiedPeerPid/Subkind optional)
- [ ] 1.4 types.go: Add `ModelUsage` struct (InputTokens/OutputTokens/CacheReadInputTokens/CacheCreationInputTokens/WebSearchRequests/CostUSD/ContextWindow/MaxOutputTokens/CanonicalModel/Provider)
- [ ] 1.5 types.go: Add `ConversationResetMessage` struct (embeds SystemMessage, NewConversationID/UUID/SessionID)
- [ ] 1.6 types.go: Add `_SKILLS_ALL` constant = "all"

**验收**: 所有新类型可编译，json tag 正确，`Message` union 包含 `ConversationResetMessage`

## 2. New Fields

- [ ] 2.1 types.go: `UserMessage` 添加 `Origin *MessageOrigin` 字段 (json: "origin,omitempty")
- [ ] 2.2 types.go: `ResultMessage` 添加 `Origin *MessageOrigin` 字段 (json: "origin,omitempty")
- [ ] 2.3 types.go: `ResultMessage` 添加 `TerminalReason string` 字段 (json: "terminal_reason,omitempty")
- [ ] 2.4 types.go: `ClaudeAgentOptions` 添加 `ResumeSessionAt string` 字段
- [ ] 2.5 types.go: `ClaudeAgentOptions` 添加 `ResumeDropsTurn string` 字段

**验收**: 所有新字段有正确 json tag，零值不影响现有序列化

## 3. Message Parsing

- [ ] 3.1 message_parser.go: 添加 `parseOrigin(data map[string]any) *MessageOrigin` 辅助函数（检查 origin.kind 是否为 string，否则返回 nil）
- [ ] 3.2 message_parser.go: 添加 `conversation_reset` case → `ConversationResetMessage`（required: new_conversation_id, uuid, session_id）
- [ ] 3.3 message_parser.go: `parseUserMessage` 中调用 `parseOrigin` 并设置 `Origin` 字段
- [ ] 3.4 message_parser.go: `parseResultMessage` 中调用 `parseOrigin` 并设置 `Origin` 字段
- [ ] 3.5 message_parser.go: `parseResultMessage` 中解析 `terminal_reason` 字段

**验收**: `parseMessage({"type":"conversation_reset","new_conversation_id":"c1","uuid":"u1","session_id":"s1"})` 返回 `*ConversationResetMessage`；`parseMessage({"type":"user","message":...,"origin":{"kind":"task-notification"}})` 的 Origin 不为 nil

## 4. Task Lifecycle Tracking

- [ ] 4.1 query_protocol.go: 添加 `inflightTasks map[string]bool` 字段
- [ ] 4.2 query_protocol.go: 添加 `DEFERRING_TASK_TYPES` 常量 = {"local_agent", "local_workflow"}
- [ ] 4.3 query_protocol.go: 添加 `trackTaskLifecycle(message map[string]any)` 方法：
  - subtype=="task_started" && task_type in DEFERRING_TASK_TYPES → add to inflightTasks
  - subtype=="task_notification" → discard from inflightTasks
  - subtype=="task_updated" && patch.status in TerminalTaskStatuses → discard from inflightTasks
- [ ] 4.4 query_protocol.go: `Run()` 中 system 消息调用 `trackTaskLifecycle`
- [ ] 4.5 query_protocol.go: result 消息到达时，如果 `len(inflightTasks) > 0` 则不关闭 stdin

**验收**: 发送 task_started(local_agent) → result → 不关闭 stdin；发送 task_notification → result → 关闭 stdin

## 5. CLI Flags

- [ ] 5.1 transport.go: `--resume-session-at=<value>` 格式（当 `ResumeSessionAt` 非空）
- [ ] 5.2 transport.go: `--resume-drops-turn=<value>` 格式（当 `ResumeDropsTurn` 非空）

**验收**: `buildCommand()` 包含 `--resume-session-at=xxx` 和 `--resume-drops-turn=xxx`

## 6. Skills Validation

- [ ] 6.1 transport.go: 添加 `rejectNonListSkills(skills any)` — skills 必须是 []string 或 "all"
- [ ] 6.2 transport.go: 添加 `validateSkillName(name string)` — 检查非空、无代理码点、无前后空白、无括号/逗号/控制字符、不是 "*"、不以 "/" 开头、无连续反斜杠、不以反斜杠结尾
- [ ] 6.3 transport.go: `buildCommand()` 中调用验证

**验收**: `rejectNonListSkills("my-skill")` 返回 error；`validateSkillName("my(skill)")` 返回 error

## 7. Windows Safety

- [ ] 7.1 transport.go: 添加 `rejectWindowsCmdMetacharacters(optionName, value string)` — Windows 上检查 `&|<>^%!"` 和换行符
- [ ] 7.2 transport.go: `buildCommand()` 中对 resume/session_id/resume_session_at/resume_drops_turn 调用

**验收**: Windows 上 `rejectWindowsCmdMetacharacters("resume", "test&value")` 返回 error

## 8. Tests

- [ ] 8.1 message_parser_test.go: TestParseOrigin（valid/missing/invalid）
- [ ] 8.2 message_parser_test.go: TestParseConversationResetMessage（valid/missing fields）
- [ ] 8.3 message_parser_test.go: TestParseUserMessageOrigin / TestParseResultMessageOrigin
- [ ] 8.4 message_parser_test.go: TestParseResultMessageTerminalReason
- [ ] 8.5 query_protocol_test.go: TestTrackTaskLifecycle（add/discard/terminal）
- [ ] 8.6 transport_test.go: TestRejectNonListSkills / TestValidateSkillName
- [ ] 8.7 transport_test.go: TestRejectWindowsCmdMetacharacters
- [ ] 8.8 transport_test.go: TestBuildCommand_ResumeSessionAt / TestBuildCommand_ResumeDropsTurn
