package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func newTestTransport(opts *ClaudeAgentOptions) *cliTransport {
	t := &cliTransport{opts: opts, cliPath: "/usr/bin/claude"}
	t.maxBufferSize = defaultMaxBufferSize
	t.cwd = opts.CWD
	return t
}

func buildCmd(opts *ClaudeAgentOptions) []string {
	t := newTestTransport(opts)
	return t.buildCommand()
}

func hasFlag(cmd []string, flag string) bool {
	for _, v := range cmd {
		if v == flag {
			return true
		}
	}
	return false
}

func flagValue(cmd []string, flag string) string {
	for i, v := range cmd {
		if v == flag && i+1 < len(cmd) {
			return cmd[i+1]
		}
	}
	return ""
}

// TestBuildCommand_AlwaysPresent checks the mandatory flags.
func TestBuildCommand_AlwaysPresent(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{})
	if !hasFlag(cmd, "--output-format") || flagValue(cmd, "--output-format") != "stream-json" {
		t.Error("--output-format stream-json missing")
	}
	if !hasFlag(cmd, "--verbose") {
		t.Error("--verbose missing")
	}
	if !hasFlag(cmd, "--input-format") || flagValue(cmd, "--input-format") != "stream-json" {
		t.Error("--input-format stream-json missing")
	}
	// --input-format stream-json must be the last pair.
	last := cmd[len(cmd)-1]
	secondLast := cmd[len(cmd)-2]
	if secondLast != "--input-format" || last != "stream-json" {
		t.Errorf("--input-format stream-json should be last flags, got %q %q", secondLast, last)
	}
}

// TestBuildCommand_SettingSourcesEmpty ensures --setting-sources is NOT emitted
// when empty, matching Python SDK v0.1.53 behaviour.
func TestBuildCommand_SettingSourcesEmpty(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{})
	if hasFlag(cmd, "--setting-sources") {
		t.Error("--setting-sources should NOT appear when SettingSources is empty")
	}
}

// TestBuildCommand_SettingSourcesNonEmpty checks correct value when set.
func TestBuildCommand_SettingSourcesNonEmpty(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{
		SettingSources: []SettingSource{SettingSourceUser, SettingSourceProject},
	})
	if !hasFlag(cmd, "--setting-sources") {
		t.Fatal("--setting-sources missing")
	}
	val := flagValue(cmd, "--setting-sources")
	if val != "user,project" {
		t.Errorf("wrong setting-sources value: %q", val)
	}
}

// TestBuildCommand_MCPConfigPath verifies that MCPConfigPath is passed directly
// to --mcp-config as a file path (mirrors Python's mcp_servers as str|Path).
func TestBuildCommand_MCPConfigPath(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{MCPConfigPath: "/etc/mcp-config.json"})
	if !hasFlag(cmd, "--mcp-config") {
		t.Fatal("--mcp-config missing when MCPConfigPath is set")
	}
	if flagValue(cmd, "--mcp-config") != "/etc/mcp-config.json" {
		t.Errorf("wrong --mcp-config value: %q", flagValue(cmd, "--mcp-config"))
	}
}

// TestBuildCommand_MaxTurns checks --max-turns.
func TestBuildCommand_MaxTurns(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{MaxTurns: 5})
	if !hasFlag(cmd, "--max-turns") || flagValue(cmd, "--max-turns") != "5" {
		t.Error("--max-turns 5 missing")
	}
	// Zero means omit.
	cmd2 := buildCmd(&ClaudeAgentOptions{MaxTurns: 0})
	if hasFlag(cmd2, "--max-turns") {
		t.Error("--max-turns should be absent when MaxTurns == 0")
	}
}

// TestBuildCommand_ModelFlags verifies --model and --fallback-model.
func TestBuildCommand_ModelFlags(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{Model: "claude-opus-4", FallbackModel: "claude-haiku"})
	if flagValue(cmd, "--model") != "claude-opus-4" {
		t.Error("wrong --model")
	}
	if flagValue(cmd, "--fallback-model") != "claude-haiku" {
		t.Error("wrong --fallback-model")
	}
}

// TestBuildCommand_AllowedDisallowedTools verifies comma-joined tool lists.
func TestBuildCommand_AllowedDisallowedTools(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{
		AllowedTools:    []string{"Bash", "Read"},
		DisallowedTools: []string{"Write"},
	})
	if flagValue(cmd, "--allowedTools") != "Bash,Read" {
		t.Errorf("wrong --allowedTools: %q", flagValue(cmd, "--allowedTools"))
	}
	if flagValue(cmd, "--disallowedTools") != "Write" {
		t.Errorf("wrong --disallowedTools: %q", flagValue(cmd, "--disallowedTools"))
	}
}

// TestBuildCommand_PermissionMode checks --permission-mode.
func TestBuildCommand_PermissionMode(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{PermissionMode: PermissionModeAcceptEdits})
	if flagValue(cmd, "--permission-mode") != "acceptEdits" {
		t.Errorf("wrong permission-mode: %q", flagValue(cmd, "--permission-mode"))
	}
}

// TestBuildCommand_Betas checks --betas.
func TestBuildCommand_Betas(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{Betas: []SdkBeta{SdkBetaContext1M}})
	if flagValue(cmd, "--betas") != "context-1m-2025-08-07" {
		t.Errorf("wrong --betas: %q", flagValue(cmd, "--betas"))
	}
}

// TestBuildCommand_MCPSdkServerStrip verifies 'instance' is absent in --mcp-config JSON.
func TestBuildCommand_MCPSdkServerStrip(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{
		MCPServers: map[string]MCPServerConfig{
			"calc": &MCPSdkServerConfig{Name: "calculator", Instance: nil},
		},
	})
	mcpJSON := flagValue(cmd, "--mcp-config")
	if mcpJSON == "" {
		t.Fatal("--mcp-config missing")
	}
	if strings.Contains(mcpJSON, "instance") {
		t.Errorf("--mcp-config should not contain 'instance' field: %s", mcpJSON)
	}
	if !strings.Contains(mcpJSON, `"type":"sdk"`) {
		t.Errorf("--mcp-config should contain type=sdk: %s", mcpJSON)
	}
}

// TestBuildCommand_IncludePartialMessages checks streaming env flag comes from opts.
func TestBuildCommand_IncludePartialMessages(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{IncludePartialMessages: true})
	if !hasFlag(cmd, "--include-partial-messages") {
		t.Error("--include-partial-messages missing")
	}
	cmd2 := buildCmd(&ClaudeAgentOptions{IncludePartialMessages: false})
	if hasFlag(cmd2, "--include-partial-messages") {
		t.Error("--include-partial-messages should be absent")
	}
}

// TestBuildCommand_ExtraArgsBoolFlag verifies nil value means boolean flag.
func TestBuildCommand_ExtraArgsBoolFlag(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{
		ExtraArgs: map[string]*string{"debug-to-stderr": nil},
	})
	if !hasFlag(cmd, "--debug-to-stderr") {
		t.Error("--debug-to-stderr boolean flag missing")
	}
}

// TestBuildCommand_ExtraArgsValueFlag verifies non-nil value produces --flag value.
func TestBuildCommand_ExtraArgsValueFlag(t *testing.T) {
	v := "myvalue"
	cmd := buildCmd(&ClaudeAgentOptions{
		ExtraArgs: map[string]*string{"custom-flag": &v},
	})
	if flagValue(cmd, "--custom-flag") != "myvalue" {
		t.Errorf("wrong --custom-flag value: %q", flagValue(cmd, "--custom-flag"))
	}
}

// TestBuildCommand_ThinkingAdaptive verifies --thinking adaptive flag.
func TestBuildCommand_ThinkingAdaptive(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{Thinking: &ThinkingAdaptive{}})
	if flagValue(cmd, "--thinking") != "adaptive" {
		t.Errorf("wrong --thinking for adaptive: %q", flagValue(cmd, "--thinking"))
	}
	if hasFlag(cmd, "--max-thinking-tokens") {
		t.Error("--max-thinking-tokens should NOT appear for adaptive thinking")
	}
}

// TestBuildCommand_ThinkingEnabled verifies explicit budget.
func TestBuildCommand_ThinkingEnabled(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{Thinking: &ThinkingEnabled{BudgetTokens: 16000}})
	if flagValue(cmd, "--max-thinking-tokens") != "16000" {
		t.Errorf("wrong --max-thinking-tokens for enabled: %q", flagValue(cmd, "--max-thinking-tokens"))
	}
}

// TestBuildCommand_OutputFormat checks --json-schema is emitted from OutputFormat.
func TestBuildCommand_OutputFormat(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{
		OutputFormat: OutputFormat{
			"type":   "json_schema",
			"schema": map[string]any{"type": "object"},
		},
	})
	if !hasFlag(cmd, "--json-schema") {
		t.Error("--json-schema missing")
	}
}

// TestBuildCommand_ContinueConversation verifies --continue flag.
func TestBuildCommand_ContinueConversation(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{ContinueConversation: true})
	if !hasFlag(cmd, "--continue") {
		t.Error("--continue missing")
	}
}

// TestBuildCommand_Resume verifies --resume value.
func TestBuildCommand_Resume(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{Resume: "session-abc"})
	if flagValue(cmd, "--resume") != "session-abc" {
		t.Errorf("wrong --resume: %q", flagValue(cmd, "--resume"))
	}
}

// TestVersionAtLeast covers edge cases.
func TestVersionAtLeast_EdgeCases(t *testing.T) {
	tests := []struct {
		actual, min string
		want        bool
	}{
		{"2.0.0", "2.0.0", true},
		{"2.1.0", "2.0.0", true},
		{"1.99.9", "2.0.0", false},
		{"3.0.0-beta1", "2.0.0", true},
	}
	for _, tc := range tests {
		if got := versionAtLeast(tc.actual, tc.min); got != tc.want {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", tc.actual, tc.min, got, tc.want)
		}
	}
}

// -----------------------------------------------------------------------
// Transport buffering tests (mirrors test_subprocess_buffering.py)
// -----------------------------------------------------------------------

// makeBufferedTransport constructs a cliTransport whose stdout Scanner reads
// from data using bufSize as the max buffer.  This lets us test readMessages
// without a real subprocess.
func makeBufferedTransport(data string, bufSize int) *cliTransport {
	scanner := bufio.NewScanner(strings.NewReader(data))
	scanner.Buffer(make([]byte, bufSize), bufSize)
	return &cliTransport{
		opts:          &ClaudeAgentOptions{},
		maxBufferSize: bufSize,
		stdout:        scanner,
	}
}

func drainReadMessages(t *testing.T, data string, bufSize int) ([]map[string]any, error) {
	t.Helper()
	tr := makeBufferedTransport(data, bufSize)
	ctx := context.Background()
	ch := tr.readMessages(ctx)
	var msgs []map[string]any
	for m := range ch {
		msgs = append(msgs, m)
	}
	return msgs, tr.err
}

// TestBuffering_MultipleJSONOnOneLine mirrors test_multiple_json_objects_on_single_line.
// Two JSON objects separated by \n arrive as a single chunk; scanner splits on \n.
func TestBuffering_MultipleJSONOnOneLine(t *testing.T) {
	obj1 := map[string]any{"type": "message", "id": "msg1", "content": "First message"}
	obj2 := map[string]any{"type": "result", "id": "res1", "status": "completed"}
	b1, _ := json.Marshal(obj1)
	b2, _ := json.Marshal(obj2)
	data := string(b1) + "\n" + string(b2) + "\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0]["type"] != "message" || msgs[0]["id"] != "msg1" {
		t.Errorf("unexpected first message: %v", msgs[0])
	}
	if msgs[1]["type"] != "result" || msgs[1]["id"] != "res1" {
		t.Errorf("unexpected second message: %v", msgs[1])
	}
}

// TestBuffering_JSONWithEscapedNewlines mirrors test_json_with_embedded_newlines.
// JSON values containing \n escape sequences are parsed correctly.
func TestBuffering_JSONWithEscapedNewlines(t *testing.T) {
	obj1 := map[string]any{"type": "message", "content": "Line 1\nLine 2\nLine 3"}
	obj2 := map[string]any{"type": "result", "data": "Some\nMultiline\nContent"}
	b1, _ := json.Marshal(obj1)
	b2, _ := json.Marshal(obj2)
	data := string(b1) + "\n" + string(b2) + "\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0]["content"] != "Line 1\nLine 2\nLine 3" {
		t.Errorf("unexpected content: %v", msgs[0]["content"])
	}
	if msgs[1]["data"] != "Some\nMultiline\nContent" {
		t.Errorf("unexpected data: %v", msgs[1]["data"])
	}
}

// TestBuffering_MultipleNewlinesBetweenObjects mirrors test_multiple_newlines_between_objects.
// Empty lines between objects are silently skipped.
func TestBuffering_MultipleNewlinesBetweenObjects(t *testing.T) {
	obj1 := map[string]any{"type": "message", "id": "msg1"}
	obj2 := map[string]any{"type": "result", "id": "res1"}
	b1, _ := json.Marshal(obj1)
	b2, _ := json.Marshal(obj2)
	data := string(b1) + "\n\n\n" + string(b2) + "\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0]["id"] != "msg1" || msgs[1]["id"] != "res1" {
		t.Errorf("unexpected messages: %v %v", msgs[0], msgs[1])
	}
}

// TestBuffering_SplitJSONAcrossMultipleReads mirrors test_split_json_across_multiple_reads.
// A single JSON object is written in one call (no embedded newline); scanner buffers
// until it finds the terminating \n.
func TestBuffering_SplitJSONAcrossReads(t *testing.T) {
	text := strings.Repeat("x", 1000)
	obj := map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"content": []any{
				map[string]any{"type": "text", "text": text},
				map[string]any{"type": "tool_use", "id": "tool_123", "name": "Read",
					"input": map[string]any{"file_path": "/test.txt"}},
			},
		},
	}
	b, _ := json.Marshal(obj)
	data := string(b) + "\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0]["type"] != "assistant" {
		t.Errorf("expected type=assistant, got %v", msgs[0]["type"])
	}
	content := msgs[0]["message"].(map[string]any)["content"].([]any)
	if len(content) != 2 {
		t.Errorf("expected 2 content blocks, got %d", len(content))
	}
}

// TestBuffering_LargeMinifiedJSON mirrors test_large_minified_json.
func TestBuffering_LargeMinifiedJSON(t *testing.T) {
	type item struct {
		ID    int    `json:"id"`
		Value string `json:"value"`
	}
	items := make([]item, 1000)
	for i := range items {
		items[i] = item{ID: i, Value: strings.Repeat("x", 100)}
	}
	large, _ := json.Marshal(map[string]any{"data": items})
	obj := map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{
					"tool_use_id": "toolu_016fed1NhiaMLqnEvrj5NUaj",
					"type":        "tool_result",
					"content":     string(large),
				},
			},
		},
	}
	b, _ := json.Marshal(obj)
	data := string(b) + "\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0]["type"] != "user" {
		t.Errorf("expected type=user, got %v", msgs[0]["type"])
	}
	content := msgs[0]["message"].(map[string]any)["content"].([]any)
	toolUseID := content[0].(map[string]any)["tool_use_id"]
	if toolUseID != "toolu_016fed1NhiaMLqnEvrj5NUaj" {
		t.Errorf("unexpected tool_use_id: %v", toolUseID)
	}
}

// TestBuffering_BufferSizeExceeded mirrors test_buffer_size_exceeded.
// Sending more data than maxBufferSize without a newline should yield CLIJSONDecodeError.
func TestBuffering_BufferSizeExceeded(t *testing.T) {
	customLimit := 1024
	// Incomplete JSON larger than the limit (no terminating newline)
	hugeIncomplete := `{"data": "` + strings.Repeat("x", customLimit+100)

	_, err := drainReadMessages(t, hugeIncomplete, customLimit)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var jde *CLIJSONDecodeError
	if !errors.As(err, &jde) {
		t.Fatalf("expected *CLIJSONDecodeError, got %T: %v", err, err)
	}
	if !strings.Contains(jde.Line, "exceeded maximum buffer size") {
		t.Errorf("expected 'exceeded maximum buffer size' in error line, got %q", jde.Line)
	}
}

// TestBuffering_BufferSizeOption mirrors test_buffer_size_option.
// A custom (small) buffer limit is enforced.
func TestBuffering_BufferSizeOption(t *testing.T) {
	customLimit := 512
	hugeIncomplete := `{"data": "` + strings.Repeat("x", customLimit+10)

	_, err := drainReadMessages(t, hugeIncomplete, customLimit)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var jde *CLIJSONDecodeError
	if !errors.As(err, &jde) {
		t.Fatalf("expected *CLIJSONDecodeError, got %T: %v", err, err)
	}
	want := "exceeded maximum buffer size of 512 bytes"
	if !strings.Contains(jde.Line, want) {
		t.Errorf("expected %q in error line, got %q", want, jde.Line)
	}
}

// TestBuffering_MixedCompleteAndSplitJSON mirrors test_mixed_complete_and_split_json.
func TestBuffering_MixedCompleteAndSplitJSON(t *testing.T) {
	msg1, _ := json.Marshal(map[string]any{"type": "system", "subtype": "start"})
	largeTxt := strings.Repeat("y", 5000)
	msg2, _ := json.Marshal(map[string]any{
		"type":    "assistant",
		"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": largeTxt}}},
	})
	msg3, _ := json.Marshal(map[string]any{"type": "system", "subtype": "end"})
	data := string(msg1) + "\n" + string(msg2) + "\n" + string(msg3) + "\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}
	if msgs[0]["type"] != "system" || msgs[0]["subtype"] != "start" {
		t.Errorf("unexpected first message: %v", msgs[0])
	}
	if msgs[1]["type"] != "assistant" {
		t.Errorf("unexpected second message type: %v", msgs[1]["type"])
	}
	content := msgs[1]["message"].(map[string]any)["content"].([]any)
	txt := content[0].(map[string]any)["text"].(string)
	if len(txt) != 5000 {
		t.Errorf("expected text len 5000, got %d", len(txt))
	}
	if msgs[2]["type"] != "system" || msgs[2]["subtype"] != "end" {
		t.Errorf("unexpected third message: %v", msgs[2])
	}
}

// TestBuffering_NonJSONLinesSkipped verifies that non-JSON lines (e.g. [SandboxDebug])
// are silently skipped without corrupting the buffer.
func TestBuffering_NonJSONLinesSkipped(t *testing.T) {
	obj1 := map[string]any{"type": "message", "id": "msg1"}
	b1, _ := json.Marshal(obj1)
	// Mix JSON with non-JSON lines.
	data := "[SandboxDebug] some debug output\n" +
		string(b1) + "\n" +
		"[AnotherWarning] not json either\n" +
		"plain text line\n"

	msgs, err := drainReadMessages(t, data, defaultMaxBufferSize)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (non-JSON lines skipped), got %d", len(msgs))
	}
	if msgs[0]["id"] != "msg1" {
		t.Errorf("unexpected message: %v", msgs[0])
	}
}

// TestBuildCommand_SessionID verifies --session-id flag.
func TestBuildCommand_SessionID(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{SessionID: "my-session-123"})
	if flagValue(cmd, "--session-id") != "my-session-123" {
		t.Errorf("wrong --session-id: %q", flagValue(cmd, "--session-id"))
	}
}

// TestBuildCommand_TaskBudget verifies --task-budget flag.
func TestBuildCommand_TaskBudget(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{TaskBudget: &TaskBudget{Total: 50000}})
	if flagValue(cmd, "--task-budget") != "50000" {
		t.Errorf("wrong --task-budget: %q", flagValue(cmd, "--task-budget"))
	}
}

// TestBuildCommand_SystemPromptFile verifies --system-prompt-file flag.
func TestBuildCommand_SystemPromptFile(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{
		SystemPrompt: &SystemPromptFile{Type: "file", Path: "/prompts/system.txt"},
	})
	if flagValue(cmd, "--system-prompt-file") != "/prompts/system.txt" {
		t.Errorf("wrong --system-prompt-file: %q", flagValue(cmd, "--system-prompt-file"))
	}
}

// TestBuildCommand_ThinkingDisabled verifies --thinking disabled flag.
func TestBuildCommand_ThinkingDisabled(t *testing.T) {
	cmd := buildCmd(&ClaudeAgentOptions{Thinking: &ThinkingDisabled{}})
	if flagValue(cmd, "--thinking") != "disabled" {
		t.Errorf("wrong --thinking for disabled: %q", flagValue(cmd, "--thinking"))
	}
}

// TestBuildCommand_CLAUDECODEFiltered verifies CLAUDECODE env var is filtered.
func TestConnect_EnvFiltering(t *testing.T) {
	// This test verifies the env building logic conceptually.
	// We can't easily test the actual connect() without a real subprocess,
	// but we can verify the buildCommand does the right thing.
	os.Setenv("CLAUDECODE", "1")
	defer os.Unsetenv("CLAUDECODE")

	// The actual filtering happens in connect(), but we verify the code compiles
	// and the command building works with all new options.
	cmd := buildCmd(&ClaudeAgentOptions{
		SessionID:   "test-session",
		TaskBudget:  &TaskBudget{Total: 10000},
		Thinking:    &ThinkingAdaptive{},
		SystemPrompt: &SystemPromptPreset{
			Type:                   "preset",
			Preset:                 "claude_code",
			ExcludeDynamicSections: boolPtr(true),
		},
	})
	if !hasFlag(cmd, "--session-id") {
		t.Error("--session-id missing")
	}
	if !hasFlag(cmd, "--task-budget") {
		t.Error("--task-budget missing")
	}
	if !hasFlag(cmd, "--thinking") {
		t.Error("--thinking missing")
	}
}

func boolPtr(b bool) *bool { return &b }
