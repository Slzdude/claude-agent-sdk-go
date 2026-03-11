// agents demonstrates spawning subagents via the AgentDefinition API.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	opts := &claude.ClaudeAgentOptions{
		Agents: map[string]claude.AgentDefinition{
			"researcher": {
				Description: "Searches the web and summarizes findings.",
				Model:       "claude-haiku-4-5",
			},
		},
	}

	msgs, err := claude.Query(ctx,
		"Use the researcher agent to find the latest Go release and report its version.",
		opts,
	)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println(text.Text)
				}
			}
		case *claude.TaskStartedMessage:
			fmt.Printf("[task started] %s: %s\n", m.TaskID, m.Description)
		case *claude.TaskNotificationMessage:
			fmt.Printf("[task done] %s status=%s\n", m.TaskID, m.Status)
		case *claude.ResultMessage:
			_ = m
		}
	}
}
