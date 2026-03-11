// include_partial_messages demonstrates streaming partial (incremental) messages
// from Claude via the IncludePartialMessages option.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	opts := &claude.ClaudeAgentOptions{
		IncludePartialMessages: true,
		// Model:                  "claude-sonnet-4-5",
		MaxTurns: 2,
	}

	prompt := "Think of three jokes, then tell one"
	fmt.Printf("Prompt: %s\n\n", prompt)
	fmt.Println("==================================================")

	msgs, err := claude.Query(ctx, prompt, opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				switch b := block.(type) {
				case *claude.TextBlock:
					fmt.Print(b.Text)
				case *claude.ThinkingBlock:
					fmt.Printf("[thinking: %s]\n", b.Thinking)
				}
			}
			fmt.Println()
		case *claude.StreamEvent:
			// StreamEvent carries partial incremental data.
			fmt.Printf("[stream_event uuid=%s]\n", m.UUID)
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("\nCost: $%.6f\n", *m.TotalCostUSD)
			}
		}
	}
}
