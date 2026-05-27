// Package semconv defines OpenInference semantic convention constants
// for LLM observability span attributes.
package semconv

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"
)

// Span-level attributes
const (
	SpanKindKey = attribute.Key("openinference.span.kind")

	LLMSystem    = attribute.Key("llm.system")
	LLMModelName = attribute.Key("llm.model_name")

	// Token counts
	LLMTokenCountPrompt         = attribute.Key("llm.token_count.prompt")
	LLMTokenCountCompletion     = attribute.Key("llm.token_count.completion")
	LLMTokenCountTotal          = attribute.Key("llm.token_count.total")
	LLMTokenCountCacheRead      = attribute.Key("llm.token_count.prompt_details.cache_read")
	LLMTokenCountCacheWrite     = attribute.Key("llm.token_count.prompt_details.cache_write")
	LLMTokenCountCacheCreation  = attribute.Key("llm.token_count.prompt_details.cache_creation")

	// Cost
	LLMCostTotal = attribute.Key("llm.cost.total")

	// Input/Output
	InputValue    = attribute.Key("input.value")
	InputMimeType = attribute.Key("input.mime_type")
	OutputValue   = attribute.Key("output.value")
	OutputMimeType = attribute.Key("output.mime_type")

	// Session
	SessionID = attribute.Key("session.id")

	// Tool
	ToolName       = attribute.Key("tool.name")
	ToolID         = attribute.Key("tool.id")
	ToolParameters = attribute.Key("tool.parameters")

	// Agent
	AgentName = attribute.Key("agent.name")

	// Output messages (indexed)
	// Usage pattern: llm.output_messages.N.message_role, llm.output_messages.N.message_content.M, etc.
	OutputMessagePrefix    = "llm.output_messages"
	OutputMessageRole      = ".message_role"
	OutputMessageContent   = ".message_content"
	OutputMessageToolCalls = ".message_tool_calls"
	ToolCallID             = ".tool_call_id"
	ToolCallFunctionName   = ".tool_call_function_name"
	ToolCallFunctionArgs   = ".tool_call_function_arguments_json"

	// Error
	ErrorType = attribute.Key("error.type")
)

// OutputMessageAttr returns the attribute key for an indexed output message field.
func OutputMessageAttr(index int, suffix string) attribute.Key {
	return attribute.Key(OutputMessagePrefix + "." + itoa(index) + suffix)
}

// OutputMessageContentAttr returns the attribute key for a specific content block.
func OutputMessageContentAttr(msgIndex, contentIndex int) attribute.Key {
	return attribute.Key(OutputMessagePrefix + "." + itoa(msgIndex) + OutputMessageContent + "." + itoa(contentIndex))
}

// OutputMessageToolCallAttr returns the attribute key for a specific tool call field.
func OutputMessageToolCallAttr(msgIndex, callIndex int, suffix string) attribute.Key {
	return attribute.Key(OutputMessagePrefix + "." + itoa(msgIndex) + OutputMessageToolCalls + "." + itoa(callIndex) + suffix)
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
