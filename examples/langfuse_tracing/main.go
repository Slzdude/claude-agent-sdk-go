// Example: Langfuse tracing for Claude Agent SDK Go.
//
// Prerequisites:
//
//	export LANGFUSE_PUBLIC_KEY="pk-lf-..."
//	export LANGFUSE_SECRET_KEY="sk-lf-..."
//	export LANGFUSE_HOST="https://cloud.langfuse.com"  # optional
package main

import (
	"context"
	"fmt"
	"log"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing"
	"github.com/Slzdude/claude-agent-sdk-go/tracing/langfuse"
)

func main() {
	ctx := context.Background()

	// Setup Langfuse exporter
	tp, err := langfuse.SetupLangfuse(ctx, langfuse.LangfuseConfig{
		// PublicKey and SecretKey are read from LANGFUSE_PUBLIC_KEY
		// and LANGFUSE_SECRET_KEY env vars by default.
	})
	if err != nil {
		log.Fatalf("Failed to setup Langfuse: %v", err)
	}
	defer func() {
		if err := tp.Shutdown(ctx); err != nil {
			log.Printf("Shutdown error: %v", err)
		}
	}()

	// Use TracedQuery instead of claude.Query
	opts := &claude.ClaudeAgentOptions{
		PermissionMode: claude.PermissionModeBypassPermissions,
	}

	msgs, err := tracing.TracedQuery(ctx, "What is 2+2? Reply with just the number.", opts,
		tracing.WithTracerProvider(tp),
	)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Response:", text.Text)
				}
			}
		case *claude.ResultMessage:
			fmt.Printf("Done! Session: %s, Cost: $%.4f\n", m.SessionID, *m.TotalCostUSD)
		}
	}
}
