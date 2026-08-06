package oaichat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
)

// OpenAI limits new cache writes per request, not the number of historical
// breakpoints that may be replayed for cache reads. Keep the local replay
// window within the documented read window while letting the upstream enforce
// its four-write limit.
const maxResponsesPromptCacheReadBreakpoints = 50

func normalizeChatImageURLToString(v any) any {
	switch vv := v.(type) {
	case string:
		return vv
	case map[string]any:
		if url := common.Interface2String(vv["url"]); url != "" {
			return url
		}
		return v
	case dto.MessageImageUrl:
		if vv.Url != "" {
			return vv.Url
		}
		return v
	case *dto.MessageImageUrl:
		if vv != nil && vv.Url != "" {
			return vv.Url
		}
		return v
	default:
		return v
	}
}

func chatContentPartsToResponses(parts []dto.MediaContent, role string, explicitPromptCaching bool) ([]map[string]any, bool) {
	contentParts := make([]map[string]any, 0, len(parts))
	hasCacheBreakpoint := false
	for _, part := range parts {
		var converted map[string]any
		cacheable := false
		switch part.Type {
		case dto.ContentTypeText:
			textType := "input_text"
			cacheable = role != "assistant"
			if role == "assistant" {
				textType = "output_text"
			}
			converted = map[string]any{"type": textType, "text": part.Text}
		case dto.ContentTypeImageURL:
			converted = map[string]any{"type": "input_image", "image_url": normalizeChatImageURLToString(part.ImageUrl)}
			cacheable = true
		case dto.ContentTypeInputAudio:
			converted = map[string]any{"type": "input_audio", "input_audio": part.InputAudio}
		case dto.ContentTypeFile:
			converted = map[string]any{"type": "input_file", "file": part.File}
			cacheable = true
		case dto.ContentTypeVideoUrl:
			converted = map[string]any{"type": "input_video", "video_url": part.VideoUrl}
		default:
			converted = map[string]any{"type": part.Type}
		}
		if explicitPromptCaching && role != "assistant" && cacheable && hasChatCacheControl(part.CacheControl) {
			converted["prompt_cache_breakpoint"] = map[string]string{"mode": "explicit"}
			hasCacheBreakpoint = true
		}
		contentParts = append(contentParts, converted)
	}
	return contentParts, hasCacheBreakpoint
}

func supportsResponsesExplicitPromptCaching(model string) bool {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	if separator := strings.LastIndexByte(normalizedModel, '/'); separator >= 0 {
		normalizedModel = normalizedModel[separator+1:]
	}
	return strings.HasPrefix(normalizedModel, "gpt-5.6")
}

func hasChatCacheControl(cacheControl json.RawMessage) bool {
	return len(cacheControl) > 0 && strings.TrimSpace(string(cacheControl)) != "null"
}

func chatMessageHasInputText(message *dto.Message) bool {
	if message == nil || message.Content == nil {
		return false
	}
	if message.IsStringContent() {
		return strings.TrimSpace(message.StringContent()) != ""
	}
	for _, part := range message.ParseContent() {
		if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
			return true
		}
	}
	return false
}

func addPromptCacheBreakpointToFirstInputText(parts []map[string]any) bool {
	for _, part := range parts {
		if part["type"] != "input_text" {
			continue
		}
		part["prompt_cache_breakpoint"] = map[string]string{"mode": "explicit"}
		return true
	}
	return false
}

func convertChatResponseFormatToResponsesText(reqFormat *dto.ResponseFormat) json.RawMessage {
	if reqFormat == nil || strings.TrimSpace(reqFormat.Type) == "" {
		return nil
	}

	format := map[string]any{
		"type": reqFormat.Type,
	}

	if reqFormat.Type == "json_schema" && len(reqFormat.JsonSchema) > 0 {
		var chatSchema map[string]any
		if err := common.Unmarshal(reqFormat.JsonSchema, &chatSchema); err == nil {
			for key, value := range chatSchema {
				if key == "type" {
					continue
				}
				format[key] = value
			}

			if nested, ok := format["json_schema"].(map[string]any); ok {
				for key, value := range nested {
					if _, exists := format[key]; !exists {
						format[key] = value
					}
				}
				delete(format, "json_schema")
			}
		} else {
			format["json_schema"] = reqFormat.JsonSchema
		}
	}

	textRaw, _ := common.Marshal(map[string]any{
		"format": format,
	})
	return textRaw
}

func ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error) {
	if req == nil {
		return nil, errors.New("request is nil")
	}
	if req.Model == "" {
		return nil, errors.New("model is required")
	}
	if lo.FromPtrOr(req.N, 1) > 1 {
		return nil, fmt.Errorf("n>1 is not supported in responses compatibility mode")
	}

	var instructionsParts []string
	inputItems := make([]map[string]any, 0, len(req.Messages))
	systemInputItemIndexes := make(map[int]struct{})
	explicitPromptCaching := supportsResponsesExplicitPromptCaching(req.Model)
	breakpointHostMessageIndexes := make(map[int]struct{})
	if explicitPromptCaching {
		// Responses cannot mark output_text; move assistant boundaries to nearby input messages.
		lastAssistantMessageIndex := -1
		for messageIndex := range req.Messages {
			message := &req.Messages[messageIndex]
			if strings.TrimSpace(message.Role) != "assistant" {
				continue
			}
			lastAssistantMessageIndex = messageIndex
			hasCacheControl := false
			for _, part := range message.ParseContent() {
				if hasChatCacheControl(part.CacheControl) {
					hasCacheControl = true
					break
				}
			}
			if !hasCacheControl {
				continue
			}

			hostIndex := -1
			for candidateIndex := messageIndex + 1; candidateIndex < len(req.Messages); candidateIndex++ {
				role := strings.TrimSpace(req.Messages[candidateIndex].Role)
				if role != "user" && role != "developer" {
					continue
				}
				if chatMessageHasInputText(&req.Messages[candidateIndex]) {
					hostIndex = candidateIndex
					break
				}
			}
			if hostIndex < 0 {
				for candidateIndex := messageIndex - 1; candidateIndex >= 0; candidateIndex-- {
					role := strings.TrimSpace(req.Messages[candidateIndex].Role)
					if role != "user" && role != "developer" && role != "system" {
						continue
					}
					if chatMessageHasInputText(&req.Messages[candidateIndex]) {
						hostIndex = candidateIndex
						break
					}
				}
			}
			if hostIndex >= 0 {
				breakpointHostMessageIndexes[hostIndex] = struct{}{}
			}
		}

		if len(req.Messages) > 2 && lastAssistantMessageIndex >= 0 {
			for messageIndex := lastAssistantMessageIndex - 1; messageIndex >= 0; messageIndex-- {
				role := strings.TrimSpace(req.Messages[messageIndex].Role)
				if role != "user" && role != "developer" {
					continue
				}
				if chatMessageHasInputText(&req.Messages[messageIndex]) {
					breakpointHostMessageIndexes[messageIndex] = struct{}{}
					break
				}
			}
		}
	}

	instructionInputStarted := false
	for messageIndex, msg := range req.Messages {
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			continue
		}
		_, breakpointHost := breakpointHostMessageIndexes[messageIndex]

		if role == "tool" || role == "function" {
			callID := strings.TrimSpace(msg.ToolCallId)
			output := chatToolOutputToResponses(&msg)

			if callID == "" {
				inputItems = append(inputItems, map[string]any{
					"role":    "user",
					"content": fmt.Sprintf("[tool_output_missing_call_id] %v", output),
				})
				continue
			}

			inputItems = append(inputItems, map[string]any{
				"type":    "function_call_output",
				"call_id": callID,
				"output":  output,
			})
			continue
		}

		// Prefer mapping system/developer messages into `instructions`.
		if role == "system" || role == "developer" {
			if msg.Content == nil {
				continue
			}
			keepInInput := instructionInputStarted || breakpointHost
			if msg.IsStringContent() && !keepInInput {
				if s := strings.TrimSpace(msg.StringContent()); s != "" {
					instructionsParts = append(instructionsParts, s)
				}
				continue
			}
			parts := msg.ParseContent()
			convertedParts, hasBreakpoint := chatContentPartsToResponses(parts, role, explicitPromptCaching)
			if breakpointHost && addPromptCacheBreakpointToFirstInputText(convertedParts) {
				hasBreakpoint = true
			}
			if hasBreakpoint || keepInInput {
				inputRole := role
				if role == "system" {
					inputRole = "developer"
					systemInputItemIndexes[len(inputItems)] = struct{}{}
				}
				inputItems = append(inputItems, map[string]any{
					"role":    inputRole,
					"content": convertedParts,
				})
				instructionInputStarted = true
				continue
			}
			var sb strings.Builder
			for _, part := range parts {
				if part.Type == dto.ContentTypeText && strings.TrimSpace(part.Text) != "" {
					if sb.Len() > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(part.Text)
				}
			}
			if s := strings.TrimSpace(sb.String()); s != "" {
				instructionsParts = append(instructionsParts, s)
			}
			continue
		}

		item := map[string]any{
			"role": role,
		}

		if msg.Content == nil {
			item["content"] = ""
			inputItems = append(inputItems, item)

			if role == "assistant" {
				for _, tc := range msg.ParseToolCalls() {
					if strings.TrimSpace(tc.ID) == "" {
						continue
					}
					if tc.Type != "" && tc.Type != "function" {
						continue
					}
					name := strings.TrimSpace(tc.Function.Name)
					if name == "" {
						continue
					}
					inputItems = append(inputItems, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			continue
		}

		if msg.IsStringContent() {
			if breakpointHost {
				contentParts, _ := chatContentPartsToResponses(msg.ParseContent(), role, explicitPromptCaching)
				addPromptCacheBreakpointToFirstInputText(contentParts)
				item["content"] = contentParts
			} else {
				item["content"] = msg.StringContent()
			}
			inputItems = append(inputItems, item)

			if role == "assistant" {
				for _, tc := range msg.ParseToolCalls() {
					if strings.TrimSpace(tc.ID) == "" {
						continue
					}
					if tc.Type != "" && tc.Type != "function" {
						continue
					}
					name := strings.TrimSpace(tc.Function.Name)
					if name == "" {
						continue
					}
					inputItems = append(inputItems, map[string]any{
						"type":      "function_call",
						"call_id":   tc.ID,
						"name":      name,
						"arguments": tc.Function.Arguments,
					})
				}
			}
			continue
		}

		parts := msg.ParseContent()
		contentParts, _ := chatContentPartsToResponses(parts, role, explicitPromptCaching)
		if breakpointHost {
			addPromptCacheBreakpointToFirstInputText(contentParts)
		}
		item["content"] = contentParts
		inputItems = append(inputItems, item)

		if role == "assistant" {
			for _, tc := range msg.ParseToolCalls() {
				if strings.TrimSpace(tc.ID) == "" {
					continue
				}
				if tc.Type != "" && tc.Type != "function" {
					continue
				}
				name := strings.TrimSpace(tc.Function.Name)
				if name == "" {
					continue
				}
				inputItems = append(inputItems, map[string]any{
					"type":      "function_call",
					"call_id":   tc.ID,
					"name":      name,
					"arguments": tc.Function.Arguments,
				})
			}
		}
	}

	type promptCacheBreakpointPosition struct {
		system bool
		part   map[string]any
	}
	breakpoints := make([]promptCacheBreakpointPosition, 0, maxResponsesPromptCacheReadBreakpoints+1)
	for itemIndex, item := range inputItems {
		content, ok := item["content"].([]map[string]any)
		if !ok {
			continue
		}
		_, system := systemInputItemIndexes[itemIndex]
		for _, part := range content {
			breakpoint, exists := part["prompt_cache_breakpoint"]
			if !exists || breakpoint == nil {
				continue
			}
			breakpoints = append(breakpoints, promptCacheBreakpointPosition{
				system: system,
				part:   part,
			})
		}
	}

	remainingBreakpointCount := len(breakpoints)
	if remainingBreakpointCount > maxResponsesPromptCacheReadBreakpoints {
		// Keep the newest boundary and the latest system boundary, then fill from the suffix.
		keep := make([]bool, len(breakpoints))
		keptCount := 0
		for index := len(breakpoints) - 1; index >= 0; index-- {
			if breakpoints[index].system {
				keep[index] = true
				keptCount++
				break
			}
		}
		for index := len(breakpoints) - 1; index >= 0 && keptCount < maxResponsesPromptCacheReadBreakpoints; index-- {
			if keep[index] {
				continue
			}
			keep[index] = true
			keptCount++
		}
		for index, breakpoint := range breakpoints {
			if !keep[index] {
				delete(breakpoint.part, "prompt_cache_breakpoint")
			}
		}
		remainingBreakpointCount = keptCount
	}
	hasCacheBreakpoint := remainingBreakpointCount > 0

	inputRaw, err := common.Marshal(inputItems)
	if err != nil {
		return nil, err
	}

	var instructionsRaw json.RawMessage
	if len(instructionsParts) > 0 {
		instructions := strings.Join(instructionsParts, "\n\n")
		instructionsRaw, _ = common.Marshal(instructions)
	}

	var toolsRaw json.RawMessage
	if req.Tools != nil || len(req.ResponsesTools) > 0 {
		tools := make([]map[string]any, 0, len(req.Tools)+len(req.ResponsesTools))
		for _, tool := range req.Tools {
			switch tool.Type {
			case "function":
				tools = append(tools, map[string]any{
					"type":        "function",
					"name":        tool.Function.Name,
					"description": tool.Function.Description,
					"parameters":  tool.Function.Parameters,
				})
			default:
				// Best-effort: keep original tool shape for unknown types.
				var m map[string]any
				if b, err := common.Marshal(tool); err == nil {
					_ = common.Unmarshal(b, &m)
				}
				if len(m) == 0 {
					m = map[string]any{"type": tool.Type}
				}
				tools = append(tools, m)
			}
		}
		for _, rawTool := range req.ResponsesTools {
			var nativeTool map[string]any
			if err := common.Unmarshal(rawTool, &nativeTool); err != nil {
				return nil, fmt.Errorf("invalid Responses-native tool: %w", err)
			}
			if strings.TrimSpace(common.Interface2String(nativeTool["type"])) == "" {
				return nil, errors.New("Responses-native tool is missing type")
			}
			tools = append(tools, nativeTool)
		}
		toolsRaw, _ = common.Marshal(tools)
	}

	var toolChoiceRaw json.RawMessage
	if len(req.ResponsesToolChoice) > 0 {
		toolChoiceRaw = append(json.RawMessage(nil), req.ResponsesToolChoice...)
	} else if req.ToolChoice != nil {
		switch v := req.ToolChoice.(type) {
		case string:
			toolChoiceRaw, _ = common.Marshal(v)
		default:
			var m map[string]any
			if b, err := common.Marshal(v); err == nil {
				_ = common.Unmarshal(b, &m)
			}
			if m == nil {
				toolChoiceRaw, _ = common.Marshal(v)
			} else if t, _ := m["type"].(string); t == "function" {
				// Chat: {"type":"function","function":{"name":"..."}}
				// Responses: {"type":"function","name":"..."}
				if name, ok := m["name"].(string); ok && name != "" {
					toolChoiceRaw, _ = common.Marshal(map[string]any{
						"type": "function",
						"name": name,
					})
				} else if fn, ok := m["function"].(map[string]any); ok {
					if name, ok := fn["name"].(string); ok && name != "" {
						toolChoiceRaw, _ = common.Marshal(map[string]any{
							"type": "function",
							"name": name,
						})
					} else {
						toolChoiceRaw, _ = common.Marshal(v)
					}
				} else {
					toolChoiceRaw, _ = common.Marshal(v)
				}
			} else {
				toolChoiceRaw, _ = common.Marshal(v)
			}
		}
	}

	var parallelToolCallsRaw json.RawMessage
	if req.ParallelTooCalls != nil {
		parallelToolCallsRaw, _ = common.Marshal(*req.ParallelTooCalls)
	}

	textRaw := convertChatResponseFormatToResponsesText(req.ResponseFormat)

	maxOutputTokens := lo.FromPtrOr(req.MaxTokens, uint(0))
	maxCompletionTokens := lo.FromPtrOr(req.MaxCompletionTokens, uint(0))
	if maxCompletionTokens > maxOutputTokens {
		maxOutputTokens = maxCompletionTokens
	}
	// OpenAI Responses API rejects max_output_tokens < 16 when explicitly provided.
	//if maxOutputTokens > 0 && maxOutputTokens < 16 {
	//	maxOutputTokens = 16
	//}

	var topP *float64
	if req.TopP != nil {
		topP = common.GetPointer(lo.FromPtr(req.TopP))
	}

	out := &dto.OpenAIResponsesRequest{
		Model:             req.Model,
		Input:             inputRaw,
		Instructions:      instructionsRaw,
		Stream:            req.Stream,
		Temperature:       req.Temperature,
		Text:              textRaw,
		ToolChoice:        toolChoiceRaw,
		Tools:             toolsRaw,
		TopP:              topP,
		User:              req.User,
		ParallelToolCalls: parallelToolCallsRaw,
		Store:             req.Store,
		Metadata:          req.Metadata,
		MaxToolCalls:      req.ResponsesMaxToolCalls,
	}
	if req.PromptCacheKey != "" {
		out.PromptCacheKey, _ = common.Marshal(req.PromptCacheKey)
	}
	if hasCacheBreakpoint {
		out.PromptCacheOptions, _ = common.Marshal(map[string]string{"mode": "explicit"})
	}
	if req.MaxTokens != nil || req.MaxCompletionTokens != nil {
		out.MaxOutputTokens = lo.ToPtr(maxOutputTokens)
	}

	if req.ReasoningEffort != "" {
		out.Reasoning = &dto.Reasoning{
			Effort:  req.ReasoningEffort,
			Summary: "detailed",
		}
	}

	return out, nil
}

func CountResponsesPromptCacheBreakpoints(input json.RawMessage) int {
	var items []struct {
		Content json.RawMessage `json:"content"`
	}
	if err := common.Unmarshal(input, &items); err != nil {
		return 0
	}

	count := 0
	for _, item := range items {
		var parts []struct {
			PromptCacheBreakpoint json.RawMessage `json:"prompt_cache_breakpoint"`
		}
		if err := common.Unmarshal(item.Content, &parts); err != nil {
			continue
		}
		for _, part := range parts {
			if len(part.PromptCacheBreakpoint) > 0 && string(part.PromptCacheBreakpoint) != "null" {
				count++
			}
		}
	}
	return count
}

func chatToolOutputToResponses(message *dto.Message) any {
	if message == nil || message.Content == nil {
		return ""
	}
	if message.IsStringContent() {
		return message.StringContent()
	}

	parts := message.ParseContent()
	output := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case dto.ContentTypeText:
			output = append(output, map[string]any{
				"type": "input_text",
				"text": part.Text,
			})
		case dto.ContentTypeImageURL:
			output = append(output, map[string]any{
				"type":      "input_image",
				"image_url": normalizeChatImageURLToString(part.ImageUrl),
			})
		case dto.ContentTypeFile:
			output = append(output, map[string]any{
				"type": "input_file",
				"file": part.File,
			})
		default:
			encoded, err := common.Marshal(part)
			if err != nil {
				encoded = []byte(fmt.Sprintf("%v", part))
			}
			output = append(output, map[string]any{
				"type": "input_text",
				"text": string(encoded),
			})
		}
	}
	if len(output) > 0 {
		return output
	}
	if encoded, err := common.Marshal(message.Content); err == nil {
		return string(encoded)
	}
	return fmt.Sprintf("%v", message.Content)
}
