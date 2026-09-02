package claude

import (
	"errors"
	"testing"
)

func TestNormalizeResultErrors_String(t *testing.T) {
	result := normalizeResultErrors("single error")
	if len(result) != 1 || result[0] != "single error" {
		t.Errorf("got %v", result)
	}
}

func TestNormalizeResultErrors_StringSlice(t *testing.T) {
	result := normalizeResultErrors([]any{"error 1", "error 2", 123})
	if len(result) != 2 {
		t.Errorf("expected 2, got %d", len(result))
	}
	if result[0] != "error 1" || result[1] != "error 2" {
		t.Errorf("got %v", result)
	}
}

func TestNormalizeResultErrors_EmptyString(t *testing.T) {
	result := normalizeResultErrors("")
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestNormalizeResultErrors_Nil(t *testing.T) {
	result := normalizeResultErrors(nil)
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

func TestNormalizeResultErrors_BlankEntries(t *testing.T) {
	result := normalizeResultErrors([]any{"  ", "", "valid"})
	if len(result) != 1 || result[0] != "valid" {
		t.Errorf("got %v", result)
	}
}

func TestNewResultError(t *testing.T) {
	data := map[string]any{
		"subtype":          "error_max_turns",
		"errors":           []any{"Reached maximum number of turns"},
		"result":           "some result",
		"api_error_status": float64(429),
		"terminal_reason":  "max_turns",
		"session_id":       "s1",
	}
	err := NewResultError("test error", data, 1)
	if err.Error() != "test error (exit code: 1)" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Subtype != "error_max_turns" {
		t.Errorf("Subtype = %q", err.Subtype)
	}
	if len(err.Errors) != 1 || err.Errors[0] != "Reached maximum number of turns" {
		t.Errorf("Errors = %v", err.Errors)
	}
	if err.Result != "some result" {
		t.Errorf("Result = %q", err.Result)
	}
	if err.APIErrorStatus == nil || *err.APIErrorStatus != 429 {
		t.Errorf("APIErrorStatus = %v", err.APIErrorStatus)
	}
	if err.TerminalReason != "max_turns" {
		t.Errorf("TerminalReason = %q", err.TerminalReason)
	}
	if err.SessionID != "s1" {
		t.Errorf("SessionID = %q", err.SessionID)
	}
}

func TestNewResultError_NilData(t *testing.T) {
	err := NewResultError("test", nil, 0)
	if err.Data == nil {
		t.Error("Data should not be nil")
	}
}

func TestResultError_AsProcessError(t *testing.T) {
	err := NewResultError("test", map[string]any{"subtype": "error_max_turns"}, 1)
	var pe *ProcessError
	if !errors.As(err, &pe) {
		t.Error("ResultError should match errors.As(err, &ProcessError)")
	}
}

func TestErrorResultText_WithErrors(t *testing.T) {
	msg := map[string]any{
		"errors": []any{"tool timed out", "ENOENT: missing file"},
	}
	result := errorResultText(msg)
	if result != "tool timed out; ENOENT: missing file" {
		t.Errorf("got %q", result)
	}
}

func TestErrorResultText_WithResult(t *testing.T) {
	msg := map[string]any{
		"subtype":  "success",
		"is_error": true,
		"result":   "API Error: 429 Too Many Requests",
	}
	result := errorResultText(msg)
	if result != "API Error: 429 Too Many Requests" {
		t.Errorf("got %q", result)
	}
}

func TestErrorResultText_WithSubtype(t *testing.T) {
	msg := map[string]any{
		"subtype": "error_max_turns",
	}
	result := errorResultText(msg)
	if result != "error_max_turns" {
		t.Errorf("got %q", result)
	}
}

func TestErrorResultText_WithAPIErrorStatus(t *testing.T) {
	msg := map[string]any{
		"subtype":          "success",
		"api_error_status": float64(429),
	}
	result := errorResultText(msg)
	if result != "API error (HTTP 429)" {
		t.Errorf("got %q", result)
	}
}

func TestErrorResultText_Unknown(t *testing.T) {
	msg := map[string]any{
		"subtype": "success",
	}
	result := errorResultText(msg)
	if result != "unknown error" {
		t.Errorf("got %q", result)
	}
}

// Additional ResultError Integration Tests
// Matches Python's test_errors.py

func TestNewResultError_MalformedData(t *testing.T) {
	// ResultError with malformed data must not panic.
	data := map[string]any{
		"subtype":          123,            // not a string
		"errors":           42,             // not a list
		"result":           []any{1, 2, 3}, // not a string
		"api_error_status": "500",          // not a number
		"terminal_reason":  123,            // not a string
		"session_id":       []any{},        // not a string
	}
	err := NewResultError("test error", data, 1)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	// Should not panic, fields should default gracefully
	if err.Subtype != "" {
		t.Errorf("expected empty subtype for non-string, got %q", err.Subtype)
	}
	if err.Result != "" {
		t.Errorf("expected empty result for non-string, got %q", err.Result)
	}
	if err.TerminalReason != "" {
		t.Errorf("expected empty terminal_reason for non-string, got %q", err.TerminalReason)
	}
	if err.SessionID != "" {
		t.Errorf("expected empty session_id for non-string, got %q", err.SessionID)
	}
	if err.APIErrorStatus != nil {
		t.Errorf("expected nil api_error_status for non-number, got %v", err.APIErrorStatus)
	}
}

func TestNewResultError_MultipleErrors(t *testing.T) {
	// ResultError with multiple errors must normalize them.
	data := map[string]any{
		"errors": []any{"error 1", "error 2", "error 3"},
	}
	err := NewResultError("test", data, 1)
	if len(err.Errors) != 3 {
		t.Errorf("expected 3 errors, got %d", len(err.Errors))
	}
}

func TestNewResultError_BlankErrorsFiltered(t *testing.T) {
	// Blank errors must be filtered out.
	data := map[string]any{
		"errors": []any{"  ", "", "valid error", "  "},
	}
	err := NewResultError("test", data, 1)
	if len(err.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(err.Errors))
	}
	if err.Errors[0] != "valid error" {
		t.Errorf("expected 'valid error', got %q", err.Errors[0])
	}
}

func TestNewResultError_BareStringError(t *testing.T) {
	// A bare string errors field must be wrapped in a list.
	data := map[string]any{
		"errors": "single error string",
	}
	err := NewResultError("test", data, 1)
	if len(err.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(err.Errors))
	}
	if err.Errors[0] != "single error string" {
		t.Errorf("expected 'single error string', got %q", err.Errors[0])
	}
}

func TestErrorResultText_PrefersErrorsOverResult(t *testing.T) {
	// errors[] must take priority over result text.
	msg := map[string]any{
		"errors": []any{"specific error"},
		"result": "generic result",
	}
	result := errorResultText(msg)
	if result != "specific error" {
		t.Errorf("expected 'specific error', got %q", result)
	}
}

func TestErrorResultText_PrefersResultOverSubtype(t *testing.T) {
	// result must take priority over subtype.
	msg := map[string]any{
		"subtype": "error_max_turns",
		"result":  "API Error: Stream idle timeout",
	}
	result := errorResultText(msg)
	if result != "API Error: Stream idle timeout" {
		t.Errorf("expected 'API Error: Stream idle timeout', got %q", result)
	}
}

func TestErrorResultText_SuccessSubtypeIgnored(t *testing.T) {
	// subtype "success" must not be used as error text.
	msg := map[string]any{
		"subtype": "success",
	}
	result := errorResultText(msg)
	if result != "unknown error" {
		t.Errorf("expected 'unknown error', got %q", result)
	}
}

func TestNewResultError_ExitCodePreserved(t *testing.T) {
	// Exit code must be preserved in the embedded ProcessError.
	err := NewResultError("test", nil, 42)
	if err.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", err.ExitCode)
	}
}

func TestNewResultError_ZeroExitCode(t *testing.T) {
	// Zero exit code must be preserved.
	err := NewResultError("test", nil, 0)
	if err.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", err.ExitCode)
	}
}
