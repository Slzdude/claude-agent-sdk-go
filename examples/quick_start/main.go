// quick_start demonstrates a simple one-shot query using the Claude SDK.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	msgs, err := claude.Query(ctx, "What is 2+2? Respond in one sentence.", nil)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Assistant:", text.Text)
				}
			}
		case *claude.ResultMessage:
			fmt.Printf("\nDone. Cost: $%.6f  Turns: %d\n", func() float64 {
				if m.TotalCostUSD != nil {
					return *m.TotalCostUSD
				}
				return 0
			}(), m.NumTurns)
		}
	}
}
