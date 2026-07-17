## 1. 新增类型

- [x] 1.1 types.go: 添加 TaskUpdatedStatus 枚举 (6 值)
- [x] 1.2 types.go: 添加 TerminalTaskStatuses map
- [x] 1.3 types.go: 添加 TaskUpdatedMessage struct

## 2. 新增解析

- [x] 2.1 message_parser.go: 添加 task_updated 解析 (防御性处理)
- [x] 2.2 message_parser.go: parseSystemMessage 路由 task_updated

## 3. 测试

- [x] 3.1 新增 TestTaskUpdatedMessageParsing 测试
- [x] 3.2 新增 TestTerminalTaskStatuses 测试
- [x] 3.3 新增 TestTaskUpdatedStatusConstants 测试
- [x] 3.4 运行完整测试套件确认无回归
