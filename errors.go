// Package claude provides a Go SDK for Claude Code CLI.
package claude

import (
	"fmt"
	"strings"
)

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

// normalizeResultErrors extracts string errors from a raw CLI errors field.
// The CLI emits a list of strings; tolerates a bare string and drops
// non-string or blank entries.
func normalizeResultErrors(raw any) []string {
	switch v := raw.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v != "" {
			return []string{v}
		}
		return nil
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out = append(out, s)
				}
			}
		}
		return out
	}
	return nil
}

// ResultError is returned when the CLI ends a failed run by emitting a result
// message with is_error=true and then exiting non-zero. It replaces the bare
// "exit code 1" ProcessError and carries the result's structured payload so
// callers can branch on why the run failed.
//
// It embeds ProcessError, so existing `errors.As(err, &processErr)` handlers
// keep working.
type ResultError struct {
	ProcessError
	Subtype        string         // e.g. "error_max_turns", "error_during_execution", "success"
	Errors         []string       // Error strings reported by the CLI
	Result         string         // Result text (for API failures: "API Error: ...")
	APIErrorStatus *int           // HTTP status of failing API call, if any
	TerminalReason string         // Why the run ended (e.g. "api_error", "max_turns")
	SessionID      string         // Session the result belongs to
	Data           map[string]any // Raw result message payload
}

func (e *ResultError) sdkError()  {}
func (e *ResultError) Unwrap() error { return &e.ProcessError }

// NewResultError creates a ResultError from a raw result message payload.
func NewResultError(message string, data map[string]any, exitCode int) *ResultError {
	if data == nil {
		data = map[string]any{}
	}
	err := &ResultError{
		ProcessError: ProcessError{
			Message:  message,
			ExitCode: exitCode,
		},
		Subtype:        strVal(data, "subtype"),
		Errors:         normalizeResultErrors(data["errors"]),
		Result:         strVal(data, "result"),
		TerminalReason: strVal(data, "terminal_reason"),
		SessionID:      strVal(data, "session_id"),
		Data:           data,
	}
	if aes, ok := data["api_error_status"].(float64); ok {
		v := int(aes)
		err.APIErrorStatus = &v
	}
	return err
}

// errorResultText extracts actionable error text from a result message.
// Fallback chain: errors[] → result → subtype (non-"success") → api_error_status → "unknown error".
// Matches Python's _error_result_text.
func errorResultText(message map[string]any) string {
	errors := normalizeResultErrors(message["errors"])
	if len(errors) > 0 {
		result := ""
		for i, e := range errors {
			if i > 0 {
				result += "; "
			}
			result += e
		}
		return result
	}
	if result, ok := message["result"].(string); ok {
		result = strings.TrimSpace(result)
		if result != "" {
			return result
		}
	}
	if subtype, ok := message["subtype"].(string); ok && subtype != "" && subtype != "success" {
		return subtype
	}
	if status, ok := message["api_error_status"].(float64); ok {
		return fmt.Sprintf("API error (HTTP %d)", int(status))
	}
	return "unknown error"
}
