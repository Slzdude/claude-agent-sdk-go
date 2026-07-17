package claude

import (
	"testing"
)

func TestParseStdoutLine_ValidJSON(t *testing.T) {
	msg, err := parseStdoutLine(`{"type":"result","subtype":"success"}`)
	if err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "result" {
		t.Errorf("type = %v", msg["type"])
	}
}

func TestParseStdoutLine_EmptyLine(t *testing.T) {
	msg, err := parseStdoutLine("")
	if err != nil {
		t.Fatal(err)
	}
	if msg != nil {
		t.Errorf("expected nil for empty line, got %v", msg)
	}
}

func TestParseStdoutLine_WhitespaceOnly(t *testing.T) {
	msg, err := parseStdoutLine("   \t  ")
	if err != nil {
		t.Fatal(err)
	}
	if msg != nil {
		t.Errorf("expected nil for whitespace, got %v", msg)
	}
}

func TestParseStdoutLine_NonJSON(t *testing.T) {
	msg, err := parseStdoutLine("[SandboxDebug] some output")
	if err != nil {
		t.Fatal(err)
	}
	if msg != nil {
		t.Errorf("expected nil for non-JSON, got %v", msg)
	}
}

func TestParseStdoutLine_InvalidJSON(t *testing.T) {
	_, err := parseStdoutLine(`{"type": invalid}`)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestParseStdoutLine_CRLF(t *testing.T) {
	msg, err := parseStdoutLine("{\"type\":\"result\"}\r")
	if err != nil {
		t.Fatal(err)
	}
	if msg["type"] != "result" {
		t.Errorf("type = %v", msg["type"])
	}
}
