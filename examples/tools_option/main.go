// tools_option demonstrates controlling which tools Claude can use.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

func toolsArrayExample(ctx context.Context) {
	fmt.Println("=== Tools Array Example ===")
	fmt.Println("Setting Tools=[\"Read\", \"Glob\", \"Grep\"]")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{
		Tools:    []string{"Read", "Glob", "Grep"},
		MaxTurns: 1,
	}

	msgs, err := claude.Query(ctx, "What tools do you have available? Just list them briefly.", opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.SystemMessage:
			if m.Subtype == "init" {
				if tools, ok := m.Data["tools"]; ok {
					fmt.Printf("Tools from system message: %v\n\n", tools)
				}
			}
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Claude:", text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("\nCost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
	fmt.Println()
}

func toolsEmptyArrayExample(ctx context.Context) {
	fmt.Println("=== Tools Empty Array Example ===")
	fmt.Println("Setting Tools=[] (disables all built-in tools)")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{
		Tools:    []string{},
		MaxTurns: 1,
	}

	msgs, err := claude.Query(ctx, "What tools do you have available? Just list them briefly.", opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.SystemMessage:
			if m.Subtype == "init" {
				if tools, ok := m.Data["tools"]; ok {
					fmt.Printf("Tools from system message: %v\n\n", tools)
				}
			}
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Claude:", text.Text)
				}
			}
		case *claude.ResultMessage:
			if m.TotalCostUSD != nil {
				fmt.Printf("\nCost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
	fmt.Println()
}

func toolsPresetExample(ctx context.Context) {
	fmt.Println("=== Tools Preset Example ===")
	fmt.Println("Setting Tools=ToolsPreset (uses default Claude Code tools)")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{
		Tools:    &claude.ToolsPreset{},
		MaxTurns: 1,
	}

	msgs, err := claude.Query(ctx, "What tools do you have available? Just list them briefly.", opts)
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
				fmt.Printf("\nCost: $%.4f\n", *m.TotalCostUSD)
			}
		}
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	toolsArrayExample(ctx)
	toolsEmptyArrayExample(ctx)
	toolsPresetExample(ctx)
}
