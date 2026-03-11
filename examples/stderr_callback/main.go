// stderr_callback demonstrates capturing CLI debug output via the Stderr callback.
package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	// Collect stderr messages.
	var stderrMessages []string

	stderrCallback := func(line string) {
		stderrMessages = append(stderrMessages, line)
		if strings.Contains(line, "[ERROR]") {
			fmt.Println("Error detected:", line)
		}
	}

	opts := &claude.ClaudeAgentOptions{
		Stderr: stderrCallback,
		// Enable debug output to stderr — mirrors Python's extra_args={"debug-to-stderr": None}.
		// Without this flag, the CLI produces no stderr output and the callback is never invoked.
		ExtraArgs: map[string]*string{"debug-to-stderr": nil},
	}

	fmt.Println("Running query with stderr capture...")

	msgs, err := claude.Query(ctx, "What is 2+2?", opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Response:", text.Text)
				}
			}
		}
	}

	fmt.Printf("\nCaptured %d stderr lines\n", len(stderrMessages))
	if len(stderrMessages) > 0 {
		line := stderrMessages[0]
		if len(line) > 100 {
			line = line[:100]
		}
		fmt.Println("First stderr line:", line)
	}
}
