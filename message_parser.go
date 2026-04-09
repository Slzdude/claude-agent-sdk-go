package claude

// parseMessage converts a raw JSON object (from the CLI) into a typed Message.
func parseMessage(raw map[string]any) (Message, error) {
	t := strVal(raw, "type")
	switch t {
	case "system", "":
		return parseSystemMessage(raw)
	case "user":
		return parseUserMessage(raw)
	case "assistant":
		return parseAssistantMessage(raw)
	case "task_started":
		return &TaskStartedMessage{
			TaskID:      strVal(raw, "task_id"),
			Description: strVal(raw, "description"),
			UUID:        strVal(raw, "uuid"),
			SessionID:   strVal(raw, "session_id"),
			ToolUseID:   strVal(raw, "tool_use_id"),
			TaskType:    strVal(raw, "task_type"),
		}, nil
	case "task_progress":
		p := &TaskProgressMessage{
			TaskID:       strVal(raw, "task_id"),
			Description:  strVal(raw, "description"),
			UUID:         strVal(raw, "uuid"),
			SessionID:    strVal(raw, "session_id"),
			ToolUseID:    strVal(raw, "tool_use_id"),
			LastToolName: strVal(raw, "last_tool_name"),
		}
		if u, ok := raw["usage"].(map[string]any); ok {
			if tokens, ok := u["total_tokens"].(float64); ok {
				p.Usage.TotalTokens = int(tokens)
			}
			if toolUses, ok := u["tool_uses"].(float64); ok {
				p.Usage.ToolUses = int(toolUses)
			}
			if dur, ok := u["duration_ms"].(float64); ok {
				p.Usage.DurationMs = int(dur)
			}
		}
		return p, nil
	case "task_notification":
		n := &TaskNotificationMessage{
			TaskID:     strVal(raw, "task_id"),
			Status:     TaskNotificationStatus(strVal(raw, "status")),
			OutputFile: strVal(raw, "output_file"),
			Summary:    strVal(raw, "summary"),
			UUID:       strVal(raw, "uuid"),
			SessionID:  strVal(raw, "session_id"),
			ToolUseID:  strVal(raw, "tool_use_id"),
		}
		if u, ok := raw["usage"].(map[string]any); ok {
			usage := &TaskUsage{}
			if tokens, ok := u["total_tokens"].(float64); ok {
				usage.TotalTokens = int(tokens)
			}
			if toolUses, ok := u["tool_uses"].(float64); ok {
				usage.ToolUses = int(toolUses)
			}
			if dur, ok := u["duration_ms"].(float64); ok {
				usage.DurationMs = int(dur)
			}
			n.Usage = usage
		}
		return n, nil
	case "result":
		return parseResultMessage(raw)
	case "stream_event":
		return parseStreamEvent(raw)
	case "rate_limit_event":
		return parseRateLimitEvent(raw)
	default:
		// Forward-compatible: unknown message types are silently skipped.
		return nil, nil
	}
}

func parseSystemMessage(raw map[string]any) (*SystemMessage, error) {
	m := &SystemMessage{
		Subtype: strVal(raw, "subtype"),
		Data:    raw,
	}
	return m, nil
}

func parseUserMessage(raw map[string]any) (*UserMessage, error) {
	m := &UserMessage{
		UUID:            strVal(raw, "uuid"),
		ParentToolUseID: strVal(raw, "parent_tool_use_id"),
	}
	if tr, ok := raw["tool_use_result"].(map[string]any); ok {
		m.ToolUseResult = tr
	}
	if msg, ok := raw["message"].(map[string]any); ok {
		switch cv := msg["content"].(type) {
		case string:
			m.Content = cv
		case []any:
			blocks, err := parseContentBlocks(cv)
			if err != nil {
				return nil, err
			}
			m.Content = blocks
		}
	}
	return m, nil
}

func parseAssistantMessage(raw map[string]any) (*AssistantMessage, error) {
	m := &AssistantMessage{
		ParentToolUseID: strVal(raw, "parent_tool_use_id"),
		SessionID:       strVal(raw, "session_id"),
		UUID:            strVal(raw, "uuid"),
	}
	if msg, ok := raw["message"].(map[string]any); ok {
		blocks, err := parseContentBlocks(contentArr(msg, "content"))
		if err != nil {
			return nil, err
		}
		m.Content = blocks
		m.Model = strVal(msg, "model")
		m.MessageID = strVal(msg, "id")
		m.StopReason = strVal(msg, "stop_reason")
		if e := strVal(msg, "error"); e != "" {
			m.Error = AssistantMessageErrorType(e)
		}
		if u, ok := msg["usage"].(map[string]any); ok {
			m.Usage = u
		}
	}
	return m, nil
}

func parseResultMessage(raw map[string]any) (*ResultMessage, error) {
	m := &ResultMessage{
		Subtype:    strVal(raw, "subtype"),
		IsError:    boolVal(raw, "is_error"),
		SessionID:  strVal(raw, "session_id"),
		StopReason: strVal(raw, "stop_reason"),
		UUID:       strVal(raw, "uuid"),
	}
	switch cv := raw["result"].(type) {
	case string:
		m.Result = cv
	}
	if d, ok := raw["duration_ms"].(float64); ok {
		m.DurationMs = int(d)
	}
	if d, ok := raw["duration_api_ms"].(float64); ok {
		m.DurationAPIMs = int(d)
	}
	if n, ok := raw["num_turns"].(float64); ok {
		m.NumTurns = int(n)
	}
	if u, ok := raw["usage"].(map[string]any); ok {
		m.Usage = u
	}
	if t, ok := raw["total_cost_usd"].(float64); ok {
		m.TotalCostUSD = &t
	}
	if so, ok := raw["structured_output"]; ok {
		m.StructuredOutput = so
	}
	if mu, ok := raw["modelUsage"].(map[string]any); ok {
		m.ModelUsage = mu
	}
	if pd, ok := raw["permission_denials"].([]any); ok {
		m.PermissionDenials = pd
	}
	if errs, ok := raw["errors"].([]any); ok {
		m.Errors = make([]string, 0, len(errs))
		for _, e := range errs {
			if s, ok := e.(string); ok {
				m.Errors = append(m.Errors, s)
			}
		}
	}
	return m, nil
}

func parseStreamEvent(raw map[string]any) (*StreamEvent, error) {
	e := &StreamEvent{
		UUID:            strVal(raw, "uuid"),
		SessionID:       strVal(raw, "session_id"),
		ParentToolUseID: strVal(raw, "parent_tool_use_id"),
	}
	if ev, ok := raw["event"].(map[string]any); ok {
		e.Event = ev
	}
	return e, nil
}

func parseRateLimitEvent(raw map[string]any) (*RateLimitEvent, error) {
	infoRaw, ok := raw["rate_limit_info"].(map[string]any)
	if !ok {
		return nil, &MessageParseError{Message: "missing rate_limit_info", Data: raw}
	}
	info := RateLimitInfo{
		Status: RateLimitStatus(strVal(infoRaw, "status")),
		Raw:    infoRaw,
	}
	if v, ok := infoRaw["resetsAt"].(float64); ok {
		t := int64(v)
		info.ResetsAt = &t
	}
	if v := strVal(infoRaw, "rateLimitType"); v != "" {
		rt := RateLimitType(v)
		info.RateLimitType = &rt
	}
	if v, ok := infoRaw["utilization"].(float64); ok {
		info.Utilization = &v
	}
	if v := strVal(infoRaw, "overageStatus"); v != "" {
		os := RateLimitStatus(v)
		info.OverageStatus = &os
	}
	if v, ok := infoRaw["overageResetsAt"].(float64); ok {
		t := int64(v)
		info.OverageResetsAt = &t
	}
	if v := strVal(infoRaw, "overageDisabledReason"); v != "" {
		info.OverageDisabledReason = &v
	}
	return &RateLimitEvent{
		RateLimitInfo: info,
		UUID:          strVal(raw, "uuid"),
		SessionID:     strVal(raw, "session_id"),
	}, nil
}

func parseContentBlocks(items []any) ([]ContentBlock, error) {
	blocks := make([]ContentBlock, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}
		block, err := parseContentBlock(raw)
		if err != nil {
			return nil, err
		}
		if block == nil {
			// Forward-compatible: nil means unknown block type, skip it.
			continue
		}
		blocks = append(blocks, block)
	}
	return blocks, nil
}

func parseContentBlock(raw map[string]any) (ContentBlock, error) {
	t := strVal(raw, "type")
	switch t {
	case "text":
		return &TextBlock{Text: strVal(raw, "text")}, nil
	case "thinking":
		return &ThinkingBlock{
			Thinking: strVal(raw, "thinking"),
			Signature: func() string {
				if s, ok := raw["signature"].(string); ok {
					return s
				}
				return ""
			}(),
		}, nil
	case "tool_use":
		b := &ToolUseBlock{
			ID:   strVal(raw, "id"),
			Name: strVal(raw, "name"),
		}
		if inp, ok := raw["input"].(map[string]any); ok {
			b.Input = inp
		}
		return b, nil
	case "tool_result":
		tr := &ToolResultBlock{
			ToolUseID: strVal(raw, "tool_use_id"),
			IsError:   boolPtrVal(raw, "is_error"),
		}
		switch cv := raw["content"].(type) {
		case string:
			tr.Content = cv
		case []any:
			if len(cv) > 0 {
				if obj, ok := cv[0].(map[string]any); ok {
					tr.Content = strVal(obj, "text")
				}
			}
		}
		return tr, nil
	default:
		// Forward-compatible: unknown content block types are silently skipped.
		return nil, nil
	}
}

// ---- small helpers ----

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolVal(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func boolPtrVal(m map[string]any, key string) *bool {
	if v, ok := m[key].(bool); ok {
		return &v
	}
	return nil
}

func contentArr(m map[string]any, key string) []any {
	if v, ok := m[key].([]any); ok {
		return v
	}
	return nil
}
