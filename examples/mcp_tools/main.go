// mcp_tools demonstrates configuring an in-process SDK MCP server for calculator tools.
//
// This is the Go equivalent of Python SDK's create_sdk_mcp_server example.
// Instead of spawning an external process, the calculator runs inside the Go program
// and communicates with the Claude CLI via the SDK's MCP bridge.
//
// This example also demonstrates MCP 2.x features:
// - Output Schema: tools define expected output structure
// - Structured Content: tools return structured data alongside text
// - Resource Templates: server exposes dynamic resource patterns
package main

import (
	"context"
	"fmt"
	"log"
	"math"

	claude "github.com/Slzdude/claude-agent-sdk-go"
)

// CalculatorServer implements claude.SdkMcpServer with basic math operations.
// It demonstrates MCP 2.x features including Output Schema and Resource Templates.
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
	// Output schema for calculation results
	calcOutputSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"operation": map[string]any{"type": "string", "description": "The operation performed"},
			"operands":  map[string]any{"type": "array", "items": map[string]any{"type": "number"}},
			"result":    map[string]any{"type": "number", "description": "The calculation result"},
		},
	}
	return []claude.MCPTool{
		{Name: "add", Description: "Add two numbers", InputSchema: twoNums, OutputSchema: calcOutputSchema},
		{Name: "subtract", Description: "Subtract b from a", InputSchema: twoNums, OutputSchema: calcOutputSchema},
		{Name: "multiply", Description: "Multiply two numbers", InputSchema: twoNums, OutputSchema: calcOutputSchema},
		{Name: "divide", Description: "Divide a by b", InputSchema: twoNums, OutputSchema: calcOutputSchema},
		{Name: "sqrt", Description: "Calculate the square root", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"n": numProp("Number to take the square root of")},
			"required":   []string{"n"},
		}, OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operation": map[string]any{"type": "string"},
				"input":     map[string]any{"type": "number"},
				"result":    map[string]any{"type": "number"},
			},
		}},
		{Name: "power", Description: "Raise base to an exponent", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"base": numProp("Base number"), "exp": numProp("Exponent")},
			"required":   []string{"base", "exp"},
		}, OutputSchema: calcOutputSchema},
	}, nil
}

func (s *CalculatorServer) ListResources(_ context.Context) ([]claude.MCPResource, error) {
	return nil, nil
}
func (s *CalculatorServer) ReadResource(_ context.Context, _ string) (claude.MCPResourceContent, error) {
	return claude.MCPResourceContent{}, nil
}
func (s *CalculatorServer) ListPrompts(_ context.Context) ([]claude.MCPPrompt, error) {
	return nil, nil
}
func (s *CalculatorServer) GetPrompt(_ context.Context, _ string, _ map[string]any) (claude.MCPPromptResult, error) {
	return claude.MCPPromptResult{}, nil
}

// ListResourceTemplates demonstrates MCP 2.x resource templates.
func (s *CalculatorServer) ListResourceTemplates(_ context.Context) ([]claude.MCPResourceTemplate, error) {
	return []claude.MCPResourceTemplate{
		{
			URITemplate: "calc://history/{session_id}",
			Name:        "Calculation History",
			Description: "Access calculation history for a session",
			MimeType:    "application/json",
		},
	}, nil
}

func (s *CalculatorServer) CallTool(_ context.Context, name string, args map[string]any) (claude.ToolResult, error) {
	errResult := func(msg string) claude.ToolResult {
		return claude.ToolResult{Content: []map[string]any{{"type": "text", "text": msg}}, IsError: true}
	}
	// structuredResult returns both text and structured content (MCP 2.x)
	structuredResult := func(operation string, operands []float64, result float64) claude.ToolResult {
		return claude.ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": fmt.Sprintf("%s = %g", operation, result)},
			},
			StructuredContent: map[string]any{
				"operation": operation,
				"operands":  operands,
				"result":    result,
			},
		}
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
		return structuredResult(fmt.Sprintf("%g + %g", a, b), []float64{a, b}, a+b), nil
	case "subtract":
		a, b := getNum("a"), getNum("b")
		return structuredResult(fmt.Sprintf("%g - %g", a, b), []float64{a, b}, a-b), nil
	case "multiply":
		a, b := getNum("a"), getNum("b")
		return structuredResult(fmt.Sprintf("%g × %g", a, b), []float64{a, b}, a*b), nil
	case "divide":
		a, b := getNum("a"), getNum("b")
		if b == 0 {
			return errResult("Error: Division by zero is not allowed"), nil
		}
		return structuredResult(fmt.Sprintf("%g ÷ %g", a, b), []float64{a, b}, a/b), nil
	case "sqrt":
		n := getNum("n")
		if n < 0 {
			return errResult(fmt.Sprintf("Error: Cannot calculate square root of negative number %g", n)), nil
		}
		return claude.ToolResult{
			Content: []map[string]any{
				{"type": "text", "text": fmt.Sprintf("√%g = %g", n, math.Sqrt(n))},
			},
			StructuredContent: map[string]any{
				"operation": "sqrt",
				"input":     n,
				"result":    math.Sqrt(n),
			},
		}, nil
	case "power":
		base, exp := getNum("base"), getNum("exp")
		return structuredResult(fmt.Sprintf("%g^%g", base, exp), []float64{base, exp}, math.Pow(base, exp)), nil
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
		//nolint:staticcheck // AllowedTools is correct for MCP tool names (not Skills)
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
			_ = client.Close()
			log.Fatal(err)
		}

		for msg := range client.ReceiveResponse(ctx) {
			displayMessage(msg)
		}
		_ = client.Close()
	}
}
