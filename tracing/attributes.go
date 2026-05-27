package tracing

import (
	"encoding/json"
	"fmt"

	claude "github.com/Slzdude/claude-agent-sdk-go"
	"github.com/Slzdude/claude-agent-sdk-go/tracing/semconv"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// extractMessageAttributes extracts OTel span attributes from a message.
// outputMsgIndex is a pointer to a counter that tracks the output message index
// across the entire message stream (matches Python's output_message_index).
func extractMessageAttributes(span trace.Span, msg claude.Message, outputMsgIndex *int) {
	switch m := msg.(type) {
	case *claude.SystemMessage:
		extractSystemMessageAttributes(span, m)
	case *claude.AssistantMessage:
		extractAssistantMessageAttributes(span, m, outputMsgIndex)
	case *claude.ResultMessage:
		extractResultMessageAttributes(span, m)
	case *claude.TaskStartedMessage:
		extractTaskStartedAttributes(span, m)
	case *claude.TaskProgressMessage:
		extractTaskProgressAttributes(span, m)
	case *claude.TaskNotificationMessage:
		extractTaskNotificationAttributes(span, m)
	}
}

// extractSystemMessageAttributes extracts session_id and model from init messages.
func extractSystemMessageAttributes(span trace.Span, msg *claude.SystemMessage) {
	if msg.Data == nil {
		return
	}
	if sid, ok := msg.Data["session_id"].(string); ok && sid != "" {
		span.SetAttributes(semconv.SessionID.String(sid))
	}
	// Check both "model" and "model_name" variants
	if model := extractModelFromMap(msg.Data); model != "" {
		span.SetAttributes(semconv.LLMModelName.String(model))
	}
}

// extractAssistantMessageAttributes extracts model, usage, and output messages.
func extractAssistantMessageAttributes(span trace.Span, msg *claude.AssistantMessage, outputMsgIndex *int) {
	// Model name - check multiple locations
	if model := extractModelFromAssistant(msg); model != "" {
		span.SetAttributes(semconv.LLMModelName.String(model))
	}

	// Usage from assistant message
	if msg.Usage != nil {
		setUsageFromMap(span, msg.Usage)
	}

	// Output messages - increment index for each assistant message with content
	if len(msg.Content) > 0 {
		extractOutputMessages(span, msg.Content, *outputMsgIndex)
		*outputMsgIndex++
	}
}

// extractResultMessageAttributes extracts all terminal result attributes.
func extractResultMessageAttributes(span trace.Span, msg *claude.ResultMessage) {
	// Session ID
	if msg.SessionID != "" {
		span.SetAttributes(semconv.SessionID.String(msg.SessionID))
	}

	// Token usage
	if msg.Usage != nil {
		setUsageFromMap(span, msg.Usage)
	}

	// Model usage (may contain per-model breakdown)
	if msg.ModelUsage != nil {
		for _, v := range msg.ModelUsage {
			extractModelUsageEntry(span, v)
		}
	}

	// Cost
	if msg.TotalCostUSD != nil && *msg.TotalCostUSD > 0 {
		span.SetAttributes(semconv.LLMCostTotal.Float64(*msg.TotalCostUSD))
	}

	// Output value
	if msg.Result != "" {
		span.SetAttributes(
			semconv.OutputValue.String(msg.Result),
			semconv.OutputMimeType.String(semconv.MimeTypeText),
		)
	}

	// Status
	if msg.Subtype == "success" {
		span.SetStatus(codes.Ok, "")
	} else if msg.Subtype == "error" || msg.IsError {
		errMsg := "agent error"
		if len(msg.Errors) > 0 {
			errMsg = msg.Errors[0]
		}
		span.SetStatus(codes.Error, errMsg)
	}
}

// extractTaskStartedAttributes extracts attributes from TaskStartedMessage.
// TaskStartedMessage embeds SystemMessage, so we also extract model from Data.
func extractTaskStartedAttributes(span trace.Span, msg *claude.TaskStartedMessage) {
	if msg.SessionID != "" {
		span.SetAttributes(semconv.SessionID.String(msg.SessionID))
	}
	// Extract model from embedded SystemMessage.Data
	if msg.Data != nil {
		if model := extractModelFromMap(msg.Data); model != "" {
			span.SetAttributes(semconv.LLMModelName.String(model))
		}
	}
}

// extractTaskProgressAttributes extracts attributes from TaskProgressMessage.
func extractTaskProgressAttributes(span trace.Span, msg *claude.TaskProgressMessage) {
	// TaskProgressMessage carries usage stats for the subagent
	if msg.Usage.TotalTokens > 0 {
		span.SetAttributes(semconv.LLMTokenCountTotal.Int64(int64(msg.Usage.TotalTokens)))
	}
}

// extractTaskNotificationAttributes extracts attributes from TaskNotificationMessage.
func extractTaskNotificationAttributes(span trace.Span, msg *claude.TaskNotificationMessage) {
	if msg.SessionID != "" {
		span.SetAttributes(semconv.SessionID.String(msg.SessionID))
	}
	if msg.Usage != nil && msg.Usage.TotalTokens > 0 {
		span.SetAttributes(semconv.LLMTokenCountTotal.Int64(int64(msg.Usage.TotalTokens)))
	}
}

// extractModelFromMap extracts model name from a map, checking "model", "model_name", and "name".
func extractModelFromMap(data map[string]any) string {
	for _, key := range []string{"model", "model_name", "name"} {
		if v, ok := data[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// extractModelUsageEntry handles a single model_usage entry (may be map or list).
func extractModelUsageEntry(span trace.Span, v any) {
	switch entry := v.(type) {
	case map[string]any:
		setUsageFromMap(span, entry)
		if model := extractModelFromMap(entry); model != "" {
			span.SetAttributes(semconv.LLMModelName.String(model))
		}
	case []any:
		// List of model usage entries
		for _, item := range entry {
			if m, ok := item.(map[string]any); ok {
				setUsageFromMap(span, m)
				if model := extractModelFromMap(m); model != "" {
					span.SetAttributes(semconv.LLMModelName.String(model))
				}
			}
		}
	}
}

// extractModelFromAssistant extracts model from an AssistantMessage, checking multiple locations.
func extractModelFromAssistant(msg *claude.AssistantMessage) string {
	// Direct field
	if msg.Model != "" {
		return msg.Model
	}
	// Nested in usage
	if msg.Usage != nil {
		if model := extractModelFromMap(msg.Usage); model != "" {
			return model
		}
		// Check usage.modelUsage / usage.model_usage
		for _, key := range []string{"modelUsage", "model_usage"} {
			if mu, ok := msg.Usage[key]; ok {
				switch v := mu.(type) {
				case map[string]any:
					if model := extractModelFromMap(v); model != "" {
						return model
					}
				case map[string]string:
					if model, ok := v["model"]; ok && model != "" {
						return model
					}
				}
			}
		}
	}
	return ""
}

// setUsageFromMap extracts token counts from a usage map.
func setUsageFromMap(span trace.Span, usage map[string]any) {
	attrs := make([]attribute.KeyValue, 0, 6)

	if v := safeInt(usage, "input_tokens"); v > 0 {
		attrs = append(attrs, semconv.LLMTokenCountPrompt.Int64(int64(v)))
	}
	if v := safeInt(usage, "output_tokens"); v > 0 {
		attrs = append(attrs, semconv.LLMTokenCountCompletion.Int64(int64(v)))
	}
	total := safeInt(usage, "input_tokens") + safeInt(usage, "output_tokens")
	if total > 0 {
		attrs = append(attrs, semconv.LLMTokenCountTotal.Int64(int64(total)))
	}
	if v := safeInt(usage, "cache_read_input_tokens"); v > 0 {
		attrs = append(attrs, semconv.LLMTokenCountCacheRead.Int64(int64(v)))
	}
	// Merge cache_write and cache_creation: prefer cache_write, fall back to cache_creation
	cacheWrite := safeInt(usage, "cache_write_input_tokens")
	if cacheWrite == 0 {
		cacheWrite = safeInt(usage, "cache_creation_input_tokens")
	}
	if cacheWrite > 0 {
		attrs = append(attrs, semconv.LLMTokenCountCacheWrite.Int64(int64(cacheWrite)))
	}

	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
}

// extractOutputMessages sets llm.output_messages.N.* attributes.
func extractOutputMessages(span trace.Span, content []claude.ContentBlock, msgIndex int) {
	roleKey := semconv.OutputMessageAttr(msgIndex, semconv.OutputMessageRole)
	span.SetAttributes(roleKey.String("assistant"))

	contentIdx := 0
	toolCallIdx := 0

	for _, block := range content {
		switch b := block.(type) {
		case *claude.TextBlock:
			if b.Text != "" {
				key := semconv.OutputMessageContentAttr(msgIndex, contentIdx)
				span.SetAttributes(key.String(b.Text))
				contentIdx++
			}
		case *claude.ToolUseBlock:
			idKey := semconv.OutputMessageToolCallAttr(msgIndex, toolCallIdx, semconv.ToolCallID)
			nameKey := semconv.OutputMessageToolCallAttr(msgIndex, toolCallIdx, semconv.ToolCallFunctionName)
			span.SetAttributes(idKey.String(b.ID), nameKey.String(b.Name))

			if b.Input != nil {
				if inputJSON, err := json.Marshal(b.Input); err == nil {
					argsKey := semconv.OutputMessageToolCallAttr(msgIndex, toolCallIdx, semconv.ToolCallFunctionArgs)
					span.SetAttributes(argsKey.String(string(inputJSON)))
				}
			}
			toolCallIdx++
		}
	}
}

// safeInt safely extracts an int from a map.
func safeInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case int64:
		return int(n)
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	}
	return 0
}

// formatPromptValue formats a prompt for span input.
func formatPromptValue(prompt string) (value string, mimeType string) {
	return prompt, semconv.MimeTypeText
}

// formatPromptJSON formats a structured prompt as JSON.
func formatPromptJSON(v any) (value string, mimeType string) {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v), semconv.MimeTypeText
	}
	return string(b), semconv.MimeTypeJSON
}
