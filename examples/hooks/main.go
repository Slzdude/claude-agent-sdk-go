// hooks demonstrates registering hook callbacks for tool events.
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

func main() {
	ctx := context.Background()

	matcher := "Bash"
	opts := &claude.ClaudeAgentOptions{
		Hooks: map[claude.HookEvent][]claude.HookMatcher{
			claude.HookEventPreToolUse: {
				{
					Matcher: &matcher,
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							cmd, _ := input["command"].(string)
							fmt.Printf("[hook] PreToolUse Bash: %q\n", cmd)
							// Return nil to allow the tool use to proceed unchanged.
							return nil, nil
						},
					},
				},
			},
			claude.HookEventPostToolUse: {
				{
					Matcher: &matcher,
					Hooks: []claude.HookCallback{
						func(ctx context.Context, input map[string]any, toolUseID string) (map[string]any, error) {
							fmt.Println("[hook] PostToolUse Bash completed")
							return nil, nil
						},
					},
				},
			},
		},
	}

	msgs, err := claude.Query(ctx, "Run `echo hello` in bash.", opts)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		if result, ok := msg.(*claude.ResultMessage); ok {
			fmt.Printf("Result: %s\n", result.Result)
		}
	}
}
