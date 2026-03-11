// mcp_tools demonstrates configuring an in-process SDK MCP server for calculator tools.
//
// This is the Go equivalent of Python SDK's create_sdk_mcp_server example.
// Instead of spawning an external process, the calculator runs inside the Go program
// and communicates with the Claude CLI via the SDK's MCP bridge.
package main

import (
	"context"
	"fmt"
	"log"
	"math"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// CalculatorServer implements claude.SdkMcpServer with basic math operations.
type CalculatorServer struct{}

func (s *CalculatorServer) Name() string    { return "calculator" }
func (s *CalculatorServer) Version() string { return "2.0.0" }

func (s *CalculatorServer) ListTools(_ context.Context) ([]claude.MCPTool, error) {
	numProp := func(desc string) map[string]any {
		return map[string]any{"type": "number", "description": desc}
	}
	twoNums := map[string]any{
		"type":       "object",
		"properties": map[string]any{"a": numProp("First operand"), "b": numProp("Second operand")},
		"required":   []string{"a", "b"},
	}
	return []claude.MCPTool{
		{Name: "add", Description: "Add two numbers", InputSchema: twoNums},
		{Name: "subtract", Description: "Subtract b from a", InputSchema: twoNums},
		{Name: "multiply", Description: "Multiply two numbers", InputSchema: twoNums},
		{Name: "divide", Description: "Divide a by b", InputSchema: twoNums},
		{Name: "sqrt", Description: "Calculate the square root", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"n": numProp("Number to take the square root of")},
			"required":   []string{"n"},
		}},
		{Name: "power", Description: "Raise base to an exponent", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"base": numProp("Base number"), "exp": numProp("Exponent")},
			"required":   []string{"base", "exp"},
		}},
	}, nil
}

func (s *CalculatorServer) CallTool(_ context.Context, name string, args map[string]any) (claude.ToolResult, error) {
	text := func(msg string) claude.ToolResult {
		return claude.ToolResult{Content: []map[string]any{{"type": "text", "text": msg}}}
	}
	errResult := func(msg string) claude.ToolResult {
		return claude.ToolResult{Content: []map[string]any{{"type": "text", "text": msg}}, IsError: true}
	}
	getNum := func(key string) float64 {
		if v, ok := args[key].(float64); ok {
			return v
		}
		return 0
	}

	switch name {
	case "add":
		a, b := getNum("a"), getNum("b")
		return text(fmt.Sprintf("%g + %g = %g", a, b, a+b)), nil
	case "subtract":
		a, b := getNum("a"), getNum("b")
		return text(fmt.Sprintf("%g - %g = %g", a, b, a-b)), nil
	case "multiply":
		a, b := getNum("a"), getNum("b")
		return text(fmt.Sprintf("%g × %g = %g", a, b, a*b)), nil
	case "divide":
		a, b := getNum("a"), getNum("b")
		if b == 0 {
			return errResult("Error: Division by zero is not allowed"), nil
		}
		return text(fmt.Sprintf("%g ÷ %g = %g", a, b, a/b)), nil
	case "sqrt":
		n := getNum("n")
		if n < 0 {
			return errResult(fmt.Sprintf("Error: Cannot calculate square root of negative number %g", n)), nil
		}
		return text(fmt.Sprintf("√%g = %g", n, math.Sqrt(n))), nil
	case "power":
		base, exp := getNum("base"), getNum("exp")
		return text(fmt.Sprintf("%g^%g = %g", base, exp, math.Pow(base, exp))), nil
	default:
		return errResult(fmt.Sprintf("Unknown tool: %s", name)), nil
	}
}

func displayMessage(msg claude.Message) {
	switch m := msg.(type) {
	case *claude.AssistantMessage:
		for _, block := range m.Content {
			switch b := block.(type) {
			case *claude.TextBlock:
				fmt.Printf("Claude: %s\n", b.Text)
			case *claude.ToolUseBlock:
				fmt.Printf("Using tool: %s\n", b.Name)
				if len(b.Input) > 0 {
					fmt.Printf("  Input: %v\n", b.Input)
				}
			}
		}
	case *claude.ResultMessage:
		fmt.Println("Result ended")
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.6f\n", *m.TotalCostUSD)
		}
	}
}

func main() {
	ctx := context.Background()

	calculator := &CalculatorServer{}

	opts := &claude.ClaudeAgentOptions{
		MCPServers: map[string]claude.MCPServerConfig{
			"calc": &claude.MCPSdkServerConfig{
				Name:     "calculator",
				Instance: calculator,
			},
		},
		// Pre-approve all calculator tools so they can be used without permission prompts.
		AllowedTools: []string{
			"mcp__calc__add",
			"mcp__calc__subtract",
			"mcp__calc__multiply",
			"mcp__calc__divide",
			"mcp__calc__sqrt",
			"mcp__calc__power",
		},
	}

	prompts := []string{
		"List your tools",
		"Calculate 15 + 27",
		"What is 100 divided by 7?",
		"Calculate the square root of 144",
		"What is 2 raised to the power of 8?",
		"Calculate (12 + 8) * 3 - 10",
	}

	for _, prompt := range prompts {
		fmt.Println("==================================================")
		fmt.Printf("Prompt: %s\n", prompt)
		fmt.Println("==================================================")

		client, err := claude.NewClaudeSDKClient(ctx, opts)
		if err != nil {
			log.Fatal(err)
		}

		if err := client.Query(ctx, prompt); err != nil {
			client.Close()
			log.Fatal(err)
		}

		for msg := range client.ReceiveResponse(ctx) {
			displayMessage(msg)
		}
		client.Close()
	}
}
