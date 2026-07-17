## 1. Dead Code Removal

- [x] 1.1 message_parser.go: 删除 parseMirrorErrorMessage 函数
- [x] 1.2 transport.go: 删除 appendIfMissing 函数

## 2. gofmt Fixes

- [x] 2.1 gofmt -w types.go
- [x] 2.2 gofmt -w message_parser_test.go

## 3. ineffassign Fix

- [x] 3.1 session_resume.go: 修复 claudeJSONSrc 赋值后未使用

## 4. errcheck (Core Library)

- [x] 4.1 session_resume.go: defer f.Close() 显式忽略
- [x] 4.2 session_resume.go: os.RemoveAll 显式忽略
- [x] 4.3 session_mutations.go: os.RemoveAll 显式忽略
- [x] 4.4 query_protocol.go: recover() 显式忽略
- [x] 4.5 sessions.go: defer f.Close() 显式忽略

## 5. Verification

- [x] 5.1 gofmt -l . 无输出
- [x] 5.2 go vet ./... 无警告
- [x] 5.3 golangci-lint run ./... 核心库 0 errcheck/ineffassign 问题（3 个 unused 保留：2 个公共 API + 1 个 formatPromptJSON 待定）
- [x] 5.4 go test ./... 全部通过
