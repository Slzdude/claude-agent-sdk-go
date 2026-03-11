// system_prompt demonstrates different system_prompt configurations.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

func printMessages(msgs <-chan claude.Message) {
	for msg := range msgs {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Claude:", text.Text)
				}
			}
		}
	}
	fmt.Println()
}

func noSystemPrompt(ctx context.Context) {
	fmt.Println("=== No System Prompt (Vanilla Claude) ===")
	msgs, err := claude.Query(ctx, "What is 2 + 2?", nil)
	if err != nil {
		log.Fatal(err)
	}
	printMessages(msgs)
}

func stringSystemPrompt(ctx context.Context) {
	fmt.Println("=== String System Prompt ===")
	opts := &claude.ClaudeAgentOptions{
		SystemPrompt: "You are a pirate assistant. Respond in pirate speak.",
	}
	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}
	printMessages(msgs)
}

func presetSystemPrompt(ctx context.Context) {
	fmt.Println("=== Preset System Prompt (Default) ===")
	opts := &claude.ClaudeAgentOptions{
		// nil SystemPrompt means no override — use Claude Code default prompt.
		SystemPrompt: &claude.SystemPromptPreset{},
	}
	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}
	printMessages(msgs)
}

func presetWithAppend(ctx context.Context) {
	fmt.Println("=== Preset System Prompt with Append ===")
	opts := &claude.ClaudeAgentOptions{
		SystemPrompt: &claude.SystemPromptPreset{
			Append: "Always end your response with a fun fact.",
		},
	}
	msgs, err := claude.Query(ctx, "What is 2 + 2?", opts)
	if err != nil {
		log.Fatal(err)
	}
	printMessages(msgs)
}

func main() {
	ctx := context.Background()

	noSystemPrompt(ctx)
	stringSystemPrompt(ctx)
	presetSystemPrompt(ctx)
	presetWithAppend(ctx)
}
