// max_budget_usd demonstrates how to use MaxBudgetUSD to control API costs.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

func withoutBudget(ctx context.Context) {
	fmt.Println("=== Without Budget Limit ===")

	msgs, err := claude.Query(ctx, "What is 2 + 2?", nil)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Claude:", text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("Total cost: $%.4f\n", *m.TotalCostUSD)
			}
			fmt.Println("Status:", m.Subtype)
		}
	}
	fmt.Println()
}

func withReasonableBudget(ctx context.Context) {
	fmt.Println("=== With Reasonable Budget ($0.10) ===")

	budget := 0.10
	opts := &claude.ClaudeAgentOptions{
		MaxBudgetUSD: &budget,
	}

	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Claude:", text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("Total cost: $%.4f\n", *m.TotalCostUSD)
			}
			fmt.Println("Status:", m.Subtype)
		}
	}
	fmt.Println()
}

func withTightBudget(ctx context.Context) {
	fmt.Println("=== With Tight Budget ($0.0001) ===")

	budget := 0.0001
	opts := &claude.ClaudeAgentOptions{
		MaxBudgetUSD: &budget,
	}

	msgs, err := claude.Query(ctx, "Read the README.md file and summarize it", opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Claude:", text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("Total cost: $%.4f\n", *m.TotalCostUSD)
			}
			fmt.Println("Status:", m.Subtype)
			if m.Subtype == "error_max_budget_usd" {
				fmt.Println("⚠  Budget limit exceeded!")
				fmt.Println("Note: The cost may exceed the budget by up to one API call's worth")
			}
		}
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	fmt.Println("This example demonstrates using MaxBudgetUSD to control API costs.")
	fmt.Println()

	withoutBudget(ctx)
	withReasonableBudget(ctx)
	withTightBudget(ctx)

	fmt.Println("Note: Budget checking happens after each API call completes,")
	fmt.Println("so the final cost may slightly exceed the specified budget.")
}
