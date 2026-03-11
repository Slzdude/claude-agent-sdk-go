// streaming demonstrates various patterns for using ClaudeSDKClient.
//
// This is a comprehensive port of the Python SDK's streaming_mode.py example.
// It covers basic streaming, multi-turn conversations, concurrent send/receive,
// interrupt, manual message handling, custom options, message stream input,
// tool use blocks, control protocol, and error handling.
//
// Usage:
//
//	go run . <example_name>   # run one example
//	go run . all              # run all examples
//
// Available examples:
//
//	basic_streaming, multi_turn_conversation, concurrent_responses,
//	with_interrupt, manual_message_handling, with_options, message_stream,
//	bash_command, control_protocol, error_handling
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// displayMessage prints a message in a standardized format.
func displayMessage(msg claude.Message) {
	switch m := msg.(type) {
	case *claude.UserMessage:
		blocks, _ := m.Content.([]claude.ContentBlock)
		for _, block := range blocks {
			switch b := block.(type) {
			case *claude.TextBlock:
				fmt.Printf("User: %s\n", b.Text)
			case *claude.ToolResultBlock:
				content := ""
				if b.Content != nil {
					content = fmt.Sprintf("%v", b.Content)
				}
				if len(content) > 100 {
					content = content[:100]
				}
				fmt.Printf("Tool Result (id: %s): %s...\n", b.ToolUseID, content)
			}
		}
	case *claude.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *claude.TextBlock:
				fmt.Printf("Claude: %s\n", b.Text)
			case *claude.ToolUseBlock:
				fmt.Printf("Tool Use: %s (id: %s)\n", b.Name, b.ID)
			}
		}
	case *claude.SystemMessage:
		// ignore system messages
	case *claude.ResultMessage:
		fmt.Println("Result ended")
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
		}
	}
}

// exampleBasicStreaming demonstrates basic streaming with receive_response.
func exampleBasicStreaming(ctx context.Context) {
	fmt.Println("=== Basic Streaming Example ===")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("User: What is 2+2?")
	if err := client.Query(ctx, "What is 2+2?"); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println()
}

// exampleMultiTurnConversation shows a multi-turn conversation using ReceiveResponse.
func exampleMultiTurnConversation(ctx context.Context) {
	fmt.Println("=== Multi-Turn Conversation Example ===")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// First turn.
	fmt.Println("User: What's the capital of France?")
	if err := client.Query(ctx, "What's the capital of France?"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	// Second turn — follow-up in same session.
	fmt.Println("\nUser: What's the population of that city?")
	if err := client.Query(ctx, "What's the population of that city?"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println()
}

// exampleConcurrentResponses handles responses while sending new messages.
func exampleConcurrentResponses(ctx context.Context) {
	fmt.Println("=== Concurrent Send/Receive Example ===")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// Start receiving all messages in a background goroutine (like Python's receive_messages()).
	msgCh := client.ReceiveMessages(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range msgCh {
			displayMessage(msg)
		}
	}()

	questions := []string{
		"What is 2 + 2?",
		"What is the square root of 144?",
		"What is 10% of 80?",
	}

	for _, q := range questions {
		fmt.Printf("\nUser: %s\n", q)
		if err := client.Query(ctx, q); err != nil {
			log.Fatal(err)
		}
		time.Sleep(3 * time.Second)
	}

	// Wait for responses and clean up.
	time.Sleep(2 * time.Second)
	client.Close()
	<-done

	fmt.Println()
}

// exampleWithInterrupt demonstrates the interrupt capability.
func exampleWithInterrupt(ctx context.Context) {
	fmt.Println("=== Interrupt Example ===")
	fmt.Println("IMPORTANT: Interrupts require active message consumption.")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("\nUser: Count from 1 to 100 slowly")
	if err := client.Query(ctx, "Count from 1 to 100 slowly, with a brief pause between each number"); err != nil {
		log.Fatal(err)
	}

	// Consume messages in background while waiting to send interrupt.
	consumeDone := make(chan struct{})
	go func() {
		defer close(consumeDone)
		for range client.ReceiveResponse(ctx) {
		}
	}()

	time.Sleep(2 * time.Second)
	fmt.Println("\n[After 2 seconds, sending interrupt...]")
	_ = client.Interrupt(ctx)

	<-consumeDone

	// Send new instruction after interrupt.
	fmt.Println("\nUser: Never mind, just tell me a quick joke")
	if err := client.Query(ctx, "Never mind, just tell me a quick joke"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println()
}

// exampleManualMessageHandling shows manual iteration with custom logic.
func exampleManualMessageHandling(ctx context.Context) {
	fmt.Println("=== Manual Message Handling Example ===")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	if err := client.Query(ctx, "List 5 programming languages and their main use cases"); err != nil {
		log.Fatal(err)
	}

	languagesFound := map[string]bool{}
	tracked := []string{"Python", "JavaScript", "Java", "C++", "Go", "Rust", "Ruby"}

	for msg := range client.ReceiveMessages(ctx) {
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Printf("Claude: %s\n", text.Text)
					for _, lang := range tracked {
						if !languagesFound[lang] {
							for i := 0; i+len(lang) <= len(text.Text); i++ {
								if text.Text[i:i+len(lang)] == lang {
									languagesFound[lang] = true
									fmt.Printf("Found language: %s\n", lang)
									break
								}
							}
						}
					}
				}
			}
		case *claude.ResultMessage:
			displayMessage(m)
			fmt.Printf("Total languages mentioned: %d\n", len(languagesFound))
			goto done
		}
	}
done:
	fmt.Println()
}

// exampleWithOptions shows configuring the client with ClaudeAgentOptions.
func exampleWithOptions(ctx context.Context) {
	fmt.Println("=== Custom Options Example ===")

	opts := &claude.ClaudeAgentOptions{
		AllowedTools: []string{"Read", "Write"},
		SystemPrompt: "You are a helpful coding assistant.",
	}

	client, err := claude.NewClaudeSDKClient(ctx, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("User: Create a simple hello.txt file with a greeting message")
	if err := client.Query(ctx, "Create a simple hello.txt file with a greeting message"); err != nil {
		log.Fatal(err)
	}

	var toolUses []string
	for msg := range client.ReceiveResponse(ctx) {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tu, ok := block.(*claude.ToolUseBlock); ok {
					toolUses = append(toolUses, tu.Name)
				}
			}
		}
		displayMessage(msg)
	}
	if len(toolUses) > 0 {
		fmt.Printf("Tools used: %v\n", toolUses)
	}

	fmt.Println()
}

// exampleMessageStream demonstrates sending a stream of raw user messages
// (equivalent to Python's async iterable prompt / QueryStream).
func exampleMessageStream(ctx context.Context) {
	fmt.Println("=== Message Stream Input Example ===")

	// Build the message channel before opening the session.
	promptCh := make(chan map[string]any, 10)
	go func() {
		defer close(promptCh)

		msgs := []string{
			"Hello! I have multiple questions.",
			"First, what's the capital of Japan?",
			"Second, what's 15% of 200?",
		}
		for _, text := range msgs {
			fmt.Printf("User: %s\n", text)
			promptCh <- map[string]any{
				"type":    "user",
				"message": map[string]any{"role": "user", "content": text},
			}
		}
	}()

	msgCh, err := claude.QueryStream(ctx, promptCh, nil)
	if err != nil {
		log.Fatal(err)
	}

	resultCount := 0
	for msg := range msgCh {
		displayMessage(msg)
		if _, ok := msg.(*claude.ResultMessage); ok {
			resultCount++
		}
	}
	fmt.Printf("Received %d ResultMessage(s)\n", resultCount)

	fmt.Println()
}

// exampleBashCommand shows handling ToolUseBlock and ToolResultBlock.
func exampleBashCommand(ctx context.Context) {
	fmt.Println("=== Bash Command Example ===")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	fmt.Println("User: Run a bash echo command")
	if err := client.Query(ctx, "Run a bash echo command that says 'Hello from bash!'"); err != nil {
		log.Fatal(err)
	}

	msgTypes := map[string]int{}

	for msg := range client.ReceiveMessages(ctx) {
		switch m := msg.(type) {
		case *claude.UserMessage:
			msgTypes["UserMessage"]++
			blocks, _ := m.Content.([]claude.ContentBlock)
			for _, block := range blocks {
				switch b := block.(type) {
				case *claude.TextBlock:
					fmt.Printf("User: %s\n", b.Text)
				case *claude.ToolResultBlock:
					content := fmt.Sprintf("%v", b.Content)
					if len(content) > 100 {
						content = content[:100]
					}
					fmt.Printf("Tool Result (id: %s): %s...\n", b.ToolUseID, content)
				}
			}
		case *claude.AssistantMessage:
			msgTypes["AssistantMessage"]++
			for _, block := range m.Content {
				switch b := block.(type) {
				case *claude.TextBlock:
					fmt.Printf("Claude: %s\n", b.Text)
				case *claude.ToolUseBlock:
					fmt.Printf("Tool Use: %s (id: %s)\n", b.Name, b.ID)
					if b.Name == "Bash" {
						if cmd, ok := b.Input["command"].(string); ok {
							fmt.Printf("  Command: %s\n", cmd)
						}
					}
				}
			}
		case *claude.ResultMessage:
			msgTypes["ResultMessage"]++
			displayMessage(m)
			goto done2
		}
	}
done2:
	fmt.Printf("\nMessage types received: ")
	first := true
	for t := range msgTypes {
		if !first {
			fmt.Print(", ")
		}
		fmt.Print(t)
		first = false
	}
	fmt.Println()

	fmt.Println()
}

// exampleControlProtocol demonstrates server info retrieval and interrupt capability.
func exampleControlProtocol(ctx context.Context) {
	fmt.Println("=== Control Protocol Example ===")
	fmt.Println("Shows server info retrieval and interrupt capability")
	fmt.Println()

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 1. Get server initialization info.
	fmt.Println("1. Getting server info...")
	serverInfo := client.GetServerInfo()
	if serverInfo != nil {
		fmt.Println("✓ Server info retrieved successfully!")
		if cmds, ok := serverInfo["commands"].([]any); ok {
			fmt.Printf("  - Available commands: %d\n", len(cmds))
		}
		if style, ok := serverInfo["output_style"].(string); ok {
			fmt.Printf("  - Output style: %s\n", style)
		}
	} else {
		fmt.Println("✗ No server info available")
	}

	// 2. Test interrupt.
	fmt.Println("\n2. Testing interrupt capability...")
	fmt.Println("User: Count from 1 to 20 slowly")
	if err := client.Query(ctx, "Count from 1 to 20 slowly, pausing between each number"); err != nil {
		log.Fatal(err)
	}

	interruptDone := make(chan struct{})
	go func() {
		defer close(interruptDone)
		for msg := range client.ReceiveResponse(ctx) {
			if m, ok := msg.(*claude.AssistantMessage); ok {
				for _, block := range m.Content {
					if text, ok := block.(*claude.TextBlock); ok {
						excerpt := text.Text
						if len(excerpt) > 50 {
							excerpt = excerpt[:50]
						}
						fmt.Printf("Claude: %s...\n", excerpt)
						break
					}
				}
			}
		}
	}()

	time.Sleep(2 * time.Second)
	fmt.Println("\n[Sending interrupt after 2 seconds...]")
	if err := client.Interrupt(ctx); err != nil {
		fmt.Printf("✗ Interrupt failed: %v\n", err)
	} else {
		fmt.Println("✓ Interrupt sent successfully")
	}
	<-interruptDone

	// Send new query after interrupt.
	fmt.Println("\nUser: Just say 'Hello!'")
	if err := client.Query(ctx, "Just say 'Hello!'"); err != nil {
		log.Fatal(err)
	}
	for msg := range client.ReceiveResponse(ctx) {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Printf("Claude: %s\n", text.Text)
				}
			}
		}
	}

	fmt.Println()
}

// exampleErrorHandling demonstrates graceful error and timeout handling.
func exampleErrorHandling(ctx context.Context) {
	fmt.Println("=== Error Handling Example ===")

	client, err := claude.NewClaudeSDKClient(ctx, nil)
	if err != nil {
		var connErr *claude.CLIConnectionError
		if errors.As(err, &connErr) {
			fmt.Printf("Connection error: %v\n", connErr)
		} else {
			fmt.Printf("Unexpected error: %v\n", err)
		}
		return
	}
	defer client.Close()

	fmt.Println("User: Run a bash sleep command for 60 seconds not in the background")
	if err := client.Query(ctx, "Run a bash sleep command for 60 seconds not in the background"); err != nil {
		log.Fatal(err)
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	messages := 0
	for msg := range client.ReceiveResponse(timeoutCtx) {
		messages++
		switch m := msg.(type) {
		case *claude.AssistantMessage:
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					excerpt := text.Text
					if len(excerpt) > 50 {
						excerpt = excerpt[:50]
					}
					fmt.Printf("Claude: %s...\n", excerpt)
				}
			}
		case *claude.ResultMessage:
			displayMessage(m)
		}
	}

	if timeoutCtx.Err() != nil {
		fmt.Printf("\nResponse timeout after 10 seconds - demonstrating graceful handling\n")
		fmt.Printf("Received %d messages before timeout\n", messages)
	}

	fmt.Println()
}

func main() {
	ctx := context.Background()

	type example struct {
		name string
		fn   func(context.Context)
	}

	examples := []example{
		{"basic_streaming", exampleBasicStreaming},
		{"multi_turn_conversation", exampleMultiTurnConversation},
		{"concurrent_responses", exampleConcurrentResponses},
		{"with_interrupt", exampleWithInterrupt},
		{"manual_message_handling", exampleManualMessageHandling},
		{"with_options", exampleWithOptions},
		{"message_stream", exampleMessageStream},
		{"bash_command", exampleBashCommand},
		{"control_protocol", exampleControlProtocol},
		{"error_handling", exampleErrorHandling},
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run . <example_name>")
		fmt.Println("\nAvailable examples:")
		fmt.Println("  all - Run all examples")
		for _, e := range examples {
			fmt.Printf("  %s\n", e.name)
		}
		os.Exit(0)
	}

	name := os.Args[1]

	if name == "all" {
		for _, e := range examples {
			e.fn(ctx)
			fmt.Println("--------------------------------------------------")
		}
		return
	}

	for _, e := range examples {
		if e.name == name {
			e.fn(ctx)
			return
		}
	}

	fmt.Printf("Error: Unknown example %q\n\nAvailable examples:\n  all\n", name)
	for _, e := range examples {
		fmt.Printf("  %s\n", e.name)
	}
	os.Exit(1)
}
