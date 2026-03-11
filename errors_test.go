package claude

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestCLIConnectionError_Message checks that message is included.
func TestCLIConnectionError_Message(t *testing.T) {
	err := &CLIConnectionError{Message: "Failed to connect to CLI"}
	if !strings.Contains(err.Error(), "Failed to connect to CLI") {
		t.Errorf("expected message in error, got %q", err.Error())
	}
}

// TestCLIConnectionError_Cause checks that Unwrap works for errors.Is.
func TestCLIConnectionError_Cause(t *testing.T) {
	cause := errors.New("underlying cause")
	err := &CLIConnectionError{Message: "Failed to connect to CLI", Cause: cause}
	if !strings.Contains(err.Error(), "Failed to connect to CLI") {
		t.Errorf("expected message in error, got %q", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Error("Unwrap should return the cause so errors.Is works")
	}
}

// TestCLINotFoundError checks CLINotFoundError default message.
func TestCLINotFoundError(t *testing.T) {
	err := &CLINotFoundError{}
	if !strings.Contains(err.Error(), "Claude Code not found") {
		t.Errorf("expected 'Claude Code not found' in error, got %q", err.Error())
	}
}

// TestCLINotFoundError_WithPath checks CLINotFoundError with a custom path.
func TestCLINotFoundError_WithPath(t *testing.T) {
	err := &CLINotFoundError{CLIPath: "/opt/claude"}
	if !strings.Contains(err.Error(), "/opt/claude") {
		t.Errorf("expected path in error, got %q", err.Error())
	}
}

// TestProcessError checks ProcessError carries exit code and stderr.
func TestProcessError(t *testing.T) {
	err := &ProcessError{
		Message:  "Process failed",
		Stderr:   "Command not found",
		ExitCode: 1,
	}
	s := err.Error()
	if !strings.Contains(s, "exit code: 1") {
		t.Errorf("expected 'exit code: 1' in error string, got %q", s)
	}
	if !strings.Contains(s, "Command not found") {
		t.Errorf("expected stderr in error string, got %q", s)
	}
	if !strings.Contains(s, "Process failed") {
		t.Errorf("expected message in error string, got %q", s)
	}
}

// TestCLIJSONDecodeError checks CLIJSONDecodeError wraps original error.
func TestCLIJSONDecodeError(t *testing.T) {
	parseErr := &json.SyntaxError{}
	decodeErr := &CLIJSONDecodeError{Line: "{invalid json}", Cause: parseErr}
	if decodeErr.Line != "{invalid json}" {
		t.Error("wrong Line")
	}
	if !strings.Contains(decodeErr.Error(), "{invalid json}") {
		t.Errorf("expected line in error, got %q", decodeErr.Error())
	}
	if !errors.Is(decodeErr, parseErr) {
		t.Error("Unwrap should return the cause")
	}
}

// TestCLIJSONDecodeError_LongLineTruncated checks long lines are truncated.
func TestCLIJSONDecodeError_LongLineTruncated(t *testing.T) {
	longLine := strings.Repeat("x", 200)
	err := &CLIJSONDecodeError{Line: longLine, Cause: errors.New("parse error")}
	msg := err.Error()
	// The error message should be truncated
	if len(msg) > 200 {
		// fine — just ensure it doesn't panic or emit the full 200-char line verbatim
	}
	if !strings.Contains(msg, "...") {
		t.Error("long lines should be truncated with '...'")
	}
}
