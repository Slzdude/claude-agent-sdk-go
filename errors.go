// Package claude provides a Go SDK for Claude Code CLI.
package claude

import "fmt"

// ClaudeSDKError is the base interface for all SDK errors.
type ClaudeSDKError interface {
	error
	sdkError()
}

// CLIConnectionError is returned when unable to connect to Claude Code CLI.
type CLIConnectionError struct {
	Message string
	Cause   error
}

func (e *CLIConnectionError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("Claude Code connection error: %s: %v", e.Message, e.Cause)
	}
	return fmt.Sprintf("Claude Code connection error: %s", e.Message)
}
func (e *CLIConnectionError) Unwrap() error { return e.Cause }
func (e *CLIConnectionError) sdkError()     {}

// CLINotFoundError is returned when Claude Code CLI binary is not found.
type CLINotFoundError struct {
	CLIPath string
	Cause   error
}

func (e *CLINotFoundError) Error() string {
	if e.CLIPath != "" {
		return fmt.Sprintf("Claude Code not found at: %s", e.CLIPath)
	}
	return "Claude Code not found. Install with:\n" +
		"  npm install -g @anthropic-ai/claude-code\n\n" +
		"If already installed locally, try:\n" +
		`  export PATH="$HOME/node_modules/.bin:$PATH"` + "\n\n" +
		"Or provide the path via ClaudeAgentOptions:\n" +
		"  opts.CLIPath = \"/path/to/claude\""
}
func (e *CLINotFoundError) Unwrap() error { return e.Cause }
func (e *CLINotFoundError) sdkError()     {}

// ProcessError is returned when the CLI process exits with a non-zero status.
type ProcessError struct {
	Message  string
	ExitCode int
	Stderr   string
}

func (e *ProcessError) Error() string {
	msg := e.Message
	if e.ExitCode != 0 {
		msg = fmt.Sprintf("%s (exit code: %d)", msg, e.ExitCode)
	}
	if e.Stderr != "" {
		msg = fmt.Sprintf("%s\nError output: %s", msg, e.Stderr)
	}
	return msg
}
func (e *ProcessError) sdkError() {}

// CLIJSONDecodeError is returned when a JSON line from the CLI cannot be decoded.
type CLIJSONDecodeError struct {
	Line  string
	Cause error
}

func (e *CLIJSONDecodeError) Error() string {
	line := e.Line
	if len(line) > 100 {
		line = line[:100] + "..."
	}
	return fmt.Sprintf("Failed to decode JSON: %s", line)
}
func (e *CLIJSONDecodeError) Unwrap() error { return e.Cause }
func (e *CLIJSONDecodeError) sdkError()     {}

// MessageParseError is returned when a message from the CLI cannot be parsed.
type MessageParseError struct {
	Message string
	Data    map[string]any
}

func (e *MessageParseError) Error() string { return e.Message }
func (e *MessageParseError) sdkError()     {}
