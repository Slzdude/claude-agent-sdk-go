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
		"subtype": "success",
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
