## 1. CanUseToolShadowedWarning

- [x] 1.1 types.go: 添加 `wholeToolAllowed(entry)` 辅助函数
- [x] 1.2 types.go: 添加 `GetCanUseToolShadowedWarning(mode, tools)` 函数
- [x] 1.3 types.go: 添加 `WarnIfCanUseToolShadowed(options)` 函数
- [x] 1.4 process_query.go: 在 processQuery 中调用 WarnIfCanUseToolShadowed
- [x] 1.5 claude.go: 在 NewClaudeSDKClient 中调用 WarnIfCanUseToolShadowed
- [x] 1.6 测试: wholeToolAllowed 各种输入测试
- [x] 1.7 测试: WarnIfCanUseToolShadowed 各场景测试

## 2. NDJSON 行解析

- [x] 2.1 transport.go: 提取 `parseStdoutLine(line)` 函数
- [x] 2.2 transport.go: readMessages 使用 parseStdoutLine
- [x] 2.3 transport.go: 尾部残余行处理
- [x] 2.4 测试: parseStdoutLine 各场景测试

## 3. --resume=value 格式

- [x] 3.1 transport.go: --resume=value 格式修复
- [x] 3.2 transport.go: --session-id=value 格式修复
- [x] 3.3 测试: flag injection 防御测试

## 4. message_parser 防御

- [x] 4.1 message_parser.go: content block 非 map 防御
- [x] 4.2 message_parser.go: assistant content 非 slice 防御
- [x] 4.3 测试: 防御性解析测试

## 5. close() 鲁棒性

- [x] 5.1 transport.go: _ACTIVE_CHILDREN 仅在成功回收后丢弃
- [x] 5.2 transport.go: 写锁获取添加超时保护 (Python 用 move_on_after(5)，Go 的 sync.Mutex 无超时获取，defer unlock 已覆盖)
- [x] 5.3 测试: close 鲁棒性测试
