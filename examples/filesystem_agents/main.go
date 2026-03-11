// filesystem_agents demonstrates loading filesystem-based agents via setting_sources.
//
// Agents can be defined in .claude/agents/ markdown files and loaded by
// specifying setting_sources=["project"]. This is distinct from inline
// AgentDefinition objects.
package main

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"runtime"

	claude "github.com/anthropics/claude-agent-sdk-go"
)

func sdkDir() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	// Two levels up from examples/filesystem_agents/ to the SDK root.
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func extractAgents(msg *claude.SystemMessage) []string {
	if msg.Subtype == "init" {
		if raw, ok := msg.Data["agents"]; ok {
			var names []string
			switch v := raw.(type) {
			case []any:
				for _, item := range v {
					switch a := item.(type) {
					case string:
						names = append(names, a)
					case map[string]any:
						if n, ok := a["name"].(string); ok {
							names = append(names, n)
						}
					}
				}
			}
			return names
		}
	}
	return nil
}

func main() {
	ctx := context.Background()

	fmt.Println("=== Filesystem Agents Example ===")
	fmt.Println("Testing: SettingSources=[\"project\"] with .claude/agents/test-agent.md")
	fmt.Println()

	opts := &claude.ClaudeAgentOptions{
		SettingSources: []claude.SettingSource{claude.SettingSourceProject},
		CWD:            sdkDir(),
	}

	msgs, err := claude.Query(ctx, "Say hello in exactly 3 words", opts)
	if err != nil {
		log.Fatal(err)
	}

	var messageTypes []string
	var agentsFound []string

	for msg := range msgs {
		switch m := msg.(type) {
		case *claude.SystemMessage:
			messageTypes = append(messageTypes, "SystemMessage")
			agentsFound = extractAgents(m)
			fmt.Printf("Init message received. Agents loaded: %v\n", agentsFound)
		case *claude.AssistantMessage:
			messageTypes = append(messageTypes, "AssistantMessage")
			for _, block := range m.Content {
				if text, ok := block.(*claude.TextBlock); ok {
					fmt.Println("Assistant:", text.Text)
				}
			}
		case *claude.ResultMessage:
			messageTypes = append(messageTypes, "ResultMessage")
			cost := 0.0
			if m.TotalCostUSD != nil {
				cost = *m.TotalCostUSD
			}
			fmt.Printf("Result: subtype=%s, cost=$%.4f\n", m.Subtype, cost)
		}
	}

	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Printf("Message types received: %v\n", messageTypes)
	fmt.Printf("Total messages: %d\n", len(messageTypes))

	hasInit := false
	hasAssistant := false
	hasResult := false
	for _, t := range messageTypes {
		switch t {
		case "SystemMessage":
			hasInit = true
		case "AssistantMessage":
			hasAssistant = true
		case "ResultMessage":
			hasResult = true
		}
	}

	fmt.Println()
	if hasInit && hasAssistant && hasResult {
		fmt.Println("SUCCESS: Received full response (init, assistant, result)")
	} else {
		fmt.Println("FAILURE: Did not receive full response")
		fmt.Printf("  - Init: %v\n", hasInit)
		fmt.Printf("  - Assistant: %v\n", hasAssistant)
		fmt.Printf("  - Result: %v\n", hasResult)
	}

	hasTestAgent := false
	for _, a := range agentsFound {
		if a == "test-agent" {
			hasTestAgent = true
		}
	}
	if hasTestAgent {
		fmt.Println("SUCCESS: test-agent is available")
	} else {
		fmt.Printf("INFO: test-agent not found in agents: %v\n", agentsFound)
		fmt.Println("(This is expected if .claude/agents/test-agent.md does not exist)")
	}
}
