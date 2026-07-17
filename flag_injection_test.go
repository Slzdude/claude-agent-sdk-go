package claude

import (
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
