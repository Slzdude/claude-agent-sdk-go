package claude

import (
	"runtime"
	"strings"
	"testing"
)

func TestBuildCommand_ResumeEqualsFormat(t *testing.T) {
	opts := &ClaudeAgentOptions{
		Resume: "abc123",
	}
	cmd := buildTestCommand(opts)
	found := false
	for _, arg := range cmd {
		if arg == "--resume=abc123" {
			found = true
		}
		if arg == "--resume" {
			t.Error("--resume should not be a separate arg")
		}
	}
	if !found {
		t.Error("expected --resume=abc123 in command")
	}
}

func TestBuildCommand_ResumeDashLeading(t *testing.T) {
	opts := &ClaudeAgentOptions{
		Resume: "--evil",
	}
	cmd := buildTestCommand(opts)
	for _, arg := range cmd {
		if arg == "--evil" {
			t.Error("--evil should not appear as standalone arg")
		}
		if arg == "--resume" {
			t.Error("--resume should not be a separate arg")
		}
	}
	if !hasArgPrefix(cmd, "--resume=--evil") {
		t.Error("expected --resume=--evil in command")
	}
}

func TestBuildCommand_SessionIDEqualsFormat(t *testing.T) {
	opts := &ClaudeAgentOptions{
		SessionID: "550e8400-e29b-41d4-a716-446655440000",
	}
	cmd := buildTestCommand(opts)
	found := false
	for _, arg := range cmd {
		if strings.HasPrefix(arg, "--session-id=") {
			found = true
		}
		if arg == "--session-id" {
			t.Error("--session-id should not be a separate arg")
		}
	}
	if !found {
		t.Error("expected --session-id= in command")
	}
}

func buildTestCommand(opts *ClaudeAgentOptions) []string {
	t := &cliTransport{opts: opts, cliPath: "claude"}
	return t.buildCommand()
}

func hasArgPrefix(cmd []string, prefix string) bool {
	for _, arg := range cmd {
		if strings.HasPrefix(arg, prefix) {
			return true
		}
	}
	return false
}

// Windows Metacharacter Rejection Tests
// Matches Python's test_bad_resume_values_raise_on_windows

// TestRejectWindowsCmdMetacharacters_Resume verifies that resume values with
// cmd.exe metacharacters are rejected on Windows (CVE-2024-27980).
// On non-Windows, the function is a no-op.
// Matches Python's test_bad_resume_values_raise_on_windows.
func TestRejectWindowsCmdMetacharacters_Resume(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test (cmd.exe metacharacter rejection)")
	}
	metachars := []string{
		"test&value",
		"test|value",
		"test<value",
		"test>value",
		"test^value",
		"test%value",
		"test!value",
		`test"value`,
		"test\nvalue",
		"test\rvalue",
	}
	for _, val := range metachars {
		t.Run(val, func(t *testing.T) {
			defer func() {
				if r := recover(); r == nil {
					t.Errorf("expected panic for metacharacter in %q", val)
				}
			}()
			rejectWindowsCmdMetacharacters("resume", val)
		})
	}
}

// TestRejectWindowsCmdMetacharacters_SessionID verifies session_id rejection on Windows.
func TestRejectWindowsCmdMetacharacters_SessionID(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for metacharacter")
		}
	}()
	rejectWindowsCmdMetacharacters("session_id", "test&value")
}

// TestRejectWindowsCmdMetacharacters_ResumeSessionAt verifies resume_session_at rejection on Windows.
func TestRejectWindowsCmdMetacharacters_ResumeSessionAt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for metacharacter")
		}
	}()
	rejectWindowsCmdMetacharacters("resume_session_at", "test&value")
}

// TestRejectWindowsCmdMetacharacters_ResumeDropsTurn verifies resume_drops_turn rejection on Windows.
func TestRejectWindowsCmdMetacharacters_ResumeDropsTurn(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for metacharacter")
		}
	}()
	rejectWindowsCmdMetacharacters("resume_drops_turn", "test&value")
}

// TestRejectWindowsCmdMetacharacters_NonWindows verifies no-op on non-Windows.
func TestRejectWindowsCmdMetacharacters_NonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Non-Windows only test")
	}
	// Should not panic on non-Windows
	rejectWindowsCmdMetacharacters("resume", "test&value")
	rejectWindowsCmdMetacharacters("session_id", "test|value")
	rejectWindowsCmdMetacharacters("resume_session_at", "test<value")
	rejectWindowsCmdMetacharacters("resume_drops_turn", "test>value")
}

// Extra Args Dash-Leading Security Tests
// Matches Python's test_dash_leading_value_uses_equals_form

func TestBuildCommand_ExtraArgsDashLeading(t *testing.T) {
	// Extra args with dash-leading values must use = form
	opts := &ClaudeAgentOptions{
		ExtraArgs: map[string]*string{
			"custom-flag": strPtr("--evil"),
		},
	}
	cmd := buildTestCommand(opts)
	found := false
	for _, arg := range cmd {
		if strings.HasPrefix(arg, "--custom-flag=--evil") {
			found = true
		}
		if arg == "--evil" {
			t.Error("--evil should not appear as standalone arg")
		}
	}
	if !found {
		t.Error("expected --custom-flag=--evil in command")
	}
}

func TestBuildCommand_ExtraArgsOrdinaryValue(t *testing.T) {
	// Extra args with ordinary values keep two-token form
	opts := &ClaudeAgentOptions{
		ExtraArgs: map[string]*string{
			"custom-flag": strPtr("normal-value"),
		},
	}
	cmd := buildTestCommand(opts)
	found := false
	for i, arg := range cmd {
		if arg == "--custom-flag" && i+1 < len(cmd) && cmd[i+1] == "normal-value" {
			found = true
		}
	}
	if !found {
		t.Error("expected --custom-flag normal-value in command")
	}
}

func TestBuildCommand_ExtraArgsNilValue(t *testing.T) {
	// Extra args with nil value are boolean flags
	opts := &ClaudeAgentOptions{
		ExtraArgs: map[string]*string{
			"verbose": nil,
		},
	}
	cmd := buildTestCommand(opts)
	found := false
	for _, arg := range cmd {
		if arg == "--verbose" {
			found = true
		}
	}
	if !found {
		t.Error("expected --verbose in command")
	}
}

func strPtr(s string) *string {
	return &s
}
