// tool_permission_callback demonstrates controlling which tools Claude can use
// and optionally modifying their inputs.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// toolUsageLog tracks all tool requests for reporting.
var toolUsageLog []map[string]any

func myPermissionCallback(
	ctx context.Context,
	toolName string,
	input map[string]any,
	permCtx claude.ToolPermissionContext,
) (claude.PermissionResult, error) {
	inputJSON, _ := json.MarshalIndent(input, "   ", "  ")

	toolUsageLog = append(toolUsageLog, map[string]any{
		"tool":  toolName,
		"input": input,
	})

	fmt.Printf("\n🔧 Tool Permission Request: %s\n", toolName)
	fmt.Printf("   Input: %s\n", inputJSON)

	// Always allow read operations.
	switch toolName {
	case "Read", "Glob", "Grep":
		fmt.Printf("   ✅ Automatically allowing %s (read-only operation)\n", toolName)
		return &claude.PermissionResultAllow{}, nil
	}

	// Deny write operations to system directories.
	if toolName == "Write" || toolName == "Edit" || toolName == "MultiEdit" {
		filePath, _ := input["file_path"].(string)
		if strings.HasPrefix(filePath, "/etc/") || strings.HasPrefix(filePath, "/usr/") {
			fmt.Printf("   ❌ Denying write to system directory: %s\n", filePath)
			return &claude.PermissionResultDeny{
				Message: fmt.Sprintf("Cannot write to system directory: %s", filePath),
			}, nil
		}
		// Redirect writes to safe directory.
		if !strings.HasPrefix(filePath, "/tmp/") && !strings.HasPrefix(filePath, "./") {
			parts := strings.Split(filePath, "/")
			safePath := "./safe_output/" + parts[len(parts)-1]
			fmt.Printf("   ⚠  Redirecting write from %s to %s\n", filePath, safePath)
			modified := make(map[string]any)
			for k, v := range input {
				modified[k] = v
			}
			modified["file_path"] = safePath
			return &claude.PermissionResultAllow{UpdatedInput: modified}, nil
		}
	}

	// Check dangerous bash commands.
	if toolName == "Bash" {
		command, _ := input["command"].(string)
		dangerous := []string{"rm -rf", "sudo", "chmod 777", "dd if=", "mkfs"}
		for _, d := range dangerous {
			if strings.Contains(command, d) {
				fmt.Printf("   ❌ Denying dangerous command: %s\n", command)
				return &claude.PermissionResultDeny{
					Message: fmt.Sprintf("Dangerous command pattern detected: %s", d),
				}, nil
			}
		}
		fmt.Printf("   ✅ Allowing bash command: %s\n", command)
		return &claude.PermissionResultAllow{}, nil
	}

	// Allow everything else.
	fmt.Printf("   ✅ Allowing tool: %s\n", toolName)
	return &claude.PermissionResultAllow{}, nil
}

func main() {
	ctx := context.Background()

	fmt.Println("============================================================")
	fmt.Println("Tool Permission Callback Example")
	fmt.Println("============================================================")
	fmt.Println("\nThis example demonstrates how to:")
	fmt.Println("1. Allow/deny tools based on type")
	fmt.Println("2. Modify tool inputs for safety")
	fmt.Println("3. Log tool usage")
	fmt.Println("============================================================")

	opts := &claude.ClaudeAgentOptions{
		CanUseTool:     myPermissionCallback,
		PermissionMode: claude.PermissionModeDefault,
		CWD:            ".",
	}

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	fmt.Println("\n📝 Sending query to Claude...")

	if err := client.Query(ctx,
		"Please do the following:\n"+
			"1. List the files in the current directory\n"+
			"2. Create a simple Go hello world script at hello.go\n"+
			"3. Tell me what you did",
	); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n📨 Receiving response...")
	messageCount := 0

	for msg := range client.ReceiveResponse(ctx) {
		messageCount++
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Printf("\n💬 Claude: %s\n", text.Text)
				}
			}
		case *claude.ResultMessage:
			fmt.Println("\n✅ Task completed!")
			fmt.Printf("   Duration: %dms\n", m.DurationMs)
			if m.TotalCostUSD != nil {
				fmt.Printf("   Cost: $%.4f\n", *m.TotalCostUSD)
			}
			fmt.Printf("   Messages processed: %d\n", messageCount)
		}
	}

	// Print tool usage summary.
	fmt.Println("\n============================================================")
	fmt.Println("Tool Usage Summary")
	fmt.Println("============================================================")
	for i, usage := range toolUsageLog {
		inputJSON, _ := json.MarshalIndent(usage["input"], "      ", "  ")
		fmt.Printf("\n%d. Tool: %s\n", i+1, usage["tool"])
		fmt.Printf("   Input: %s\n", inputJSON)
	}
}
