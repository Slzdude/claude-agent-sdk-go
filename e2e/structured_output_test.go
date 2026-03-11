//go:build e2e

package e2e_test

// structured_output_test.go mirrors e2e-tests/test_structured_output.py.

import (
	"os"
	"testing"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

// TestSimpleStructuredOutput tests structured output with file counting.
// Mirrors test_simple_structured_output.
func TestSimpleStructuredOutput(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_count":      map[string]any{"type": "number"},
			"has_tests":       map[string]any{"type": "boolean"},
			"test_file_count": map[string]any{"type": "number"},
		},
		"required": []any{"file_count", "has_tests"},
	}

	// Use the Go SDK root (.. from e2e/) which has *.go source files and *_test.go test files.
	opts := &claude.ClaudeAgentOptions{
		OutputFormat:   claude.OutputFormat{"type": "json_schema", "schema": schema},
		PermissionMode: claude.PermissionModeAcceptEdits,
		CWD:            "..",
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx,
		"Count how many Go source files (*.go, excluding test files) are in the current directory and check if there are any test files (*_test.go). Use tools to explore the filesystem.",
		opts,
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	result := requireResult(t, msgs)

	if result.StructuredOutput == nil {
		t.Fatal("No structured output in result")
	}
	output, ok := result.StructuredOutput.(map[string]any)
	if !ok {
		t.Fatalf("structured_output should be map, got %T", result.StructuredOutput)
	}
	if _, ok := output["file_count"]; !ok {
		t.Error("missing file_count in structured output")
	}
	if _, ok := output["has_tests"]; !ok {
		t.Error("missing has_tests in structured output")
	}

	fileCount, _ := output["file_count"].(float64)
	if fileCount <= 0 {
		t.Errorf("should find Go source files in SDK root, got file_count=%v", fileCount)
	}
}

// TestNestedStructuredOutput tests structured output with nested objects.
// Mirrors test_nested_structured_output.
func TestNestedStructuredOutput(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"analysis": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"word_count":      map[string]any{"type": "number"},
					"character_count": map[string]any{"type": "number"},
				},
				"required": []any{"word_count", "character_count"},
			},
			"words": map[string]any{
				"type":  "array",
				"items": map[string]any{"type": "string"},
			},
		},
		"required": []any{"analysis", "words"},
	}

	opts := &claude.ClaudeAgentOptions{
		OutputFormat:   claude.OutputFormat{"type": "json_schema", "schema": schema},
		PermissionMode: claude.PermissionModeAcceptEdits,
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx,
		"Analyze this text: 'Hello world'. Provide word count, character count, and list of words.",
		opts,
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	result := requireResult(t, msgs)

	if result.StructuredOutput == nil {
		t.Fatal("No structured output in result")
	}
	output, ok := result.StructuredOutput.(map[string]any)
	if !ok {
		t.Fatalf("structured_output should be map, got %T", result.StructuredOutput)
	}

	analysis, ok := output["analysis"].(map[string]any)
	if !ok {
		t.Fatalf("analysis should be map, got %T", output["analysis"])
	}
	words, ok := output["words"].([]any)
	if !ok {
		t.Fatalf("words should be []any, got %T", output["words"])
	}

	wordCount, _ := analysis["word_count"].(float64)
	if wordCount != 2 {
		t.Errorf("word_count should be 2, got %v", wordCount)
	}
	charCount, _ := analysis["character_count"].(float64)
	if charCount != 11 {
		t.Errorf("character_count should be 11, got %v", charCount)
	}
	if len(words) != 2 {
		t.Errorf("words should have 2 elements, got %d", len(words))
	}
}

// TestStructuredOutputWithEnum tests structured output with enum constraints.
// Mirrors test_structured_output_with_enum.
func TestStructuredOutputWithEnum(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"has_tests": map[string]any{"type": "boolean"},
			"test_framework": map[string]any{
				"type": "string",
				"enum": []any{"pytest", "unittest", "nose", "unknown"},
			},
			"test_count": map[string]any{"type": "number"},
		},
		"required": []any{"has_tests", "test_framework"},
	}

	// Use the Go SDK root (.. from e2e/) which has *_test.go files using the "testing" standard library.
	opts := &claude.ClaudeAgentOptions{
		OutputFormat:   claude.OutputFormat{"type": "json_schema", "schema": schema},
		PermissionMode: claude.PermissionModeAcceptEdits,
		CWD:            "..",
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx,
		"Search for test files (*_test.go) in the current directory. Determine which test framework is being used and count how many test files exist. Use Grep to search for framework imports.",
		opts,
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	result := requireResult(t, msgs)

	if result.StructuredOutput == nil {
		t.Fatal("No structured output in result")
	}
	output, ok := result.StructuredOutput.(map[string]any)
	if !ok {
		t.Fatalf("structured_output should be map, got %T", result.StructuredOutput)
	}

	// Go uses the "testing" standard library.
	framework, _ := output["test_framework"].(string)
	validFrameworks := map[string]bool{"pytest": true, "unittest": true, "nose": true, "unknown": true}
	if !validFrameworks[framework] {
		t.Errorf("test_framework should be one of pytest/unittest/nose/unknown, got: %q", framework)
	}

	hasTests, _ := output["has_tests"].(bool)
	if !hasTests {
		t.Error("has_tests should be true")
	}
	// The enum only has 'pytest/unittest/nose/unknown'; Go's standard testing package
	// doesn't match any, so we accept 'unknown' as a valid answer.
	_ = framework // already checked above
}

// TestStructuredOutputWithTools tests structured output when agent uses tools.
// Mirrors test_structured_output_with_tools.
func TestStructuredOutputWithTools(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_count": map[string]any{"type": "number"},
			"has_readme": map[string]any{"type": "boolean"},
		},
		"required": []any{"file_count", "has_readme"},
	}

	opts := &claude.ClaudeAgentOptions{
		OutputFormat:   claude.OutputFormat{"type": "json_schema", "schema": schema},
		PermissionMode: claude.PermissionModeAcceptEdits,
		CWD:            os.TempDir(),
	}

	ctx, cancel := defaultCtx(t)
	defer cancel()

	ch, err := claude.Query(ctx,
		"Count how many files are in the current directory and check if there's a README file. Use tools as needed.",
		opts,
	)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	msgs := collectMessages(ch)
	result := requireResult(t, msgs)

	if result.StructuredOutput == nil {
		t.Fatal("No structured output in result")
	}
	output, ok := result.StructuredOutput.(map[string]any)
	if !ok {
		t.Fatalf("structured_output should be map, got %T", result.StructuredOutput)
	}
	if _, ok := output["file_count"]; !ok {
		t.Error("missing file_count")
	}
	if _, ok := output["has_readme"]; !ok {
		t.Error("missing has_readme")
	}
	fileCount, _ := output["file_count"].(float64)
	if fileCount < 0 {
		t.Errorf("file_count should be non-negative, got %v", fileCount)
	}
}
