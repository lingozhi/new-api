package claudemessages

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relaymeta "github.com/QuantumNous/new-api/service/relayconvert/internal/meta"
)

const (
	webSearchMaxUsesLow    = 1
	webSearchMaxUsesMedium = 5
	webSearchMaxUsesHigh   = 10
)

type openRouterRequestReasoning struct {
	Enabled   bool   `json:"enabled"`
	Effort    string `json:"effort,omitempty"`
	MaxTokens int    `json:"max_tokens,omitempty"`
	Exclude   bool   `json:"exclude,omitempty"`
}

func ClaudeMessagesRequestToOpenAIChat(claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	return claudeMessagesRequestToOpenAIChat(claudeRequest, info, false)
}

func ClaudeMessagesRequestToOpenAIResponsesChat(claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error) {
	return claudeMessagesRequestToOpenAIChat(claudeRequest, info, true)
}

func claudeMessagesRequestToOpenAIChat(claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo, allowResponsesTools bool) (*dto.GeneralOpenAIRequest, error) {
	openAIRequest := dto.GeneralOpenAIRequest{
		Model:       claudeRequest.Model,
		Temperature: claudeRequest.Temperature,
		Metadata:    claudeRequest.Metadata,
	}
	if claudeRequest.MaxTokens != nil {
		openAIRequest.MaxTokens = common.GetPointer(*claudeRequest.MaxTokens)
	}
	if claudeRequest.TopP != nil {
		openAIRequest.TopP = common.GetPointer(*claudeRequest.TopP)
	}
	if claudeRequest.TopK != nil {
		openAIRequest.TopK = common.GetPointer(*claudeRequest.TopK)
	}
	if claudeRequest.Stream != nil {
		openAIRequest.Stream = common.GetPointer(*claudeRequest.Stream)
	}

	isOpenRouter := relaymeta.RelayInfoChannelType(info) == constant.ChannelTypeOpenRouter
	if isOpenRouter {
		if effort := claudeRequest.GetEfforts(); effort != "" {
			effortBytes, _ := common.Marshal(effort)
			openAIRequest.Verbosity = effortBytes
		}
		if claudeRequest.Thinking != nil {
			var reasoningConfig openRouterRequestReasoning
			if claudeRequest.Thinking.Type == "enabled" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled:   true,
					MaxTokens: claudeRequest.Thinking.GetBudgetTokens(),
				}
			} else if claudeRequest.Thinking.Type == "adaptive" {
				reasoningConfig = openRouterRequestReasoning{
					Enabled: true,
				}
			}
			reasoningJSON, err := common.Marshal(reasoningConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal reasoning: %w", err)
			}
			openAIRequest.Reasoning = reasoningJSON
		}
	} else if info != nil {
		thinkingSuffix := "-thinking"
		if strings.HasSuffix(info.OriginModelName, thinkingSuffix) &&
			!strings.HasSuffix(openAIRequest.Model, thinkingSuffix) {
			openAIRequest.Model = openAIRequest.Model + thinkingSuffix
		}
	}

	if len(claudeRequest.StopSequences) == 1 {
		openAIRequest.Stop = claudeRequest.StopSequences[0]
	} else if len(claudeRequest.StopSequences) > 1 {
		openAIRequest.Stop = claudeRequest.StopSequences
	}

	openAITools, responsesTools, nativeToolTypes, maxToolCalls, err := convertClaudeTools(claudeRequest.Tools, allowResponsesTools)
	if err != nil {
		return nil, err
	}
	openAIRequest.Tools = openAITools
	openAIRequest.ResponsesTools = responsesTools
	openAIRequest.ResponsesMaxToolCalls = maxToolCalls

	if claudeRequest.ToolChoice != nil {
		var claudeToolChoice dto.ClaudeToolChoice
		switch value := claudeRequest.ToolChoice.(type) {
		case string:
			claudeToolChoice.Type = value
		default:
			claudeToolChoice, err = common.Any2Type[dto.ClaudeToolChoice](value)
			if err != nil {
				return nil, fmt.Errorf("failed to convert Claude tool_choice: %w", err)
			}
		}

		switch strings.ToLower(strings.TrimSpace(claudeToolChoice.Type)) {
		case "auto", "none":
			openAIRequest.ToolChoice = strings.ToLower(strings.TrimSpace(claudeToolChoice.Type))
		case "any":
			openAIRequest.ToolChoice = "required"
		case "tool":
			name := strings.TrimSpace(claudeToolChoice.Name)
			if name == "" {
				return nil, fmt.Errorf("Claude tool_choice type tool requires a name")
			}
			if nativeType, ok := nativeToolTypes[name]; ok {
				openAIRequest.ResponsesToolChoice, err = common.Marshal(map[string]any{"type": nativeType})
				if err != nil {
					return nil, fmt.Errorf("failed to convert Claude tool_choice: %w", err)
				}
			} else {
				openAIRequest.ToolChoice = map[string]any{
					"type": "function",
					"function": map[string]any{
						"name": name,
					},
				}
			}
		default:
			return nil, fmt.Errorf("unsupported Claude tool_choice type %q", claudeToolChoice.Type)
		}
		openAIRequest.ParallelTooCalls = common.GetPointer(!claudeToolChoice.DisableParallelToolUse)
	}

	openAIMessages := make([]dto.Message, 0)
	if claudeRequest.System != nil {
		if claudeRequest.IsStringSystem() && claudeRequest.GetStringSystem() != "" {
			openAIMessage := dto.Message{
				Role: "system",
			}
			openAIMessage.SetStringContent(claudeRequest.GetStringSystem())
			openAIMessages = append(openAIMessages, openAIMessage)
		} else {
			systems := claudeRequest.ParseSystem()
			if len(systems) > 0 {
				openAIMessage := dto.Message{
					Role: "system",
				}
				isOpenRouterClaude := isOpenRouter && strings.HasPrefix(relaymeta.RelayInfoUpstreamModelName(info), "anthropic/claude")
				if isOpenRouterClaude {
					systemMediaMessages := make([]dto.MediaContent, 0, len(systems))
					for _, system := range systems {
						message := dto.MediaContent{
							Type:         "text",
							Text:         system.GetText(),
							CacheControl: system.CacheControl,
						}
						systemMediaMessages = append(systemMediaMessages, message)
					}
					openAIMessage.SetMediaContent(systemMediaMessages)
				} else {
					systemStr := ""
					for _, system := range systems {
						if system.Text != nil {
							systemStr += *system.Text
						}
					}
					openAIMessage.SetStringContent(systemStr)
				}
				openAIMessages = append(openAIMessages, openAIMessage)
			}
		}
	}

	for _, claudeMessage := range claudeRequest.Messages {
		openAIMessage := dto.Message{
			Role: claudeMessage.Role,
		}
		if claudeMessage.IsStringContent() {
			openAIMessage.SetStringContent(claudeMessage.GetStringContent())
		} else {
			content, err := claudeMessage.ParseContent()
			if err != nil {
				return nil, err
			}
			var toolCalls []dto.ToolCallRequest
			mediaMessages := make([]dto.MediaContent, 0, len(content))

			for _, mediaMsg := range content {
				switch mediaMsg.Type {
				case "text", "input_text":
					if message, ok := claudeMediaToOpenAI(mediaMsg); ok {
						mediaMessages = append(mediaMessages, message)
					}
				case "image":
					if message, ok := claudeMediaToOpenAI(mediaMsg); ok {
						mediaMessages = append(mediaMessages, message)
					}
				case "tool_use":
					toolCall := dto.ToolCallRequest{
						ID:   mediaMsg.Id,
						Type: "function",
						Function: dto.FunctionRequest{
							Name:      mediaMsg.Name,
							Arguments: requestToJSONString(mediaMsg.Input),
						},
					}
					toolCalls = append(toolCalls, toolCall)
				case "tool_result":
					toolName := mediaMsg.Name
					if toolName == "" {
						toolName = claudeRequest.SearchToolNameByToolCallId(mediaMsg.ToolUseId)
					}
					oaiToolMessage := dto.Message{
						Role:       "tool",
						Name:       &toolName,
						ToolCallId: mediaMsg.ToolUseId,
					}
					if mediaMsg.IsStringContent() {
						oaiToolMessage.SetStringContent(mediaMsg.GetStringContent())
					} else {
						claudeMediaContents := mediaMsg.ParseMediaContent()
						openAIMediaContents := make([]dto.MediaContent, 0, len(claudeMediaContents))
						for _, content := range claudeMediaContents {
							if converted, ok := claudeMediaToOpenAI(content); ok {
								openAIMediaContents = append(openAIMediaContents, converted)
							}
						}
						if len(openAIMediaContents) > 0 {
							oaiToolMessage.SetMediaContent(openAIMediaContents)
						} else {
							encodedJSON, _ := common.Marshal(claudeMediaContents)
							oaiToolMessage.SetStringContent(string(encodedJSON))
						}
					}
					openAIMessages = append(openAIMessages, oaiToolMessage)
				}
			}

			if len(toolCalls) > 0 {
				openAIMessage.SetToolCalls(toolCalls)
			}
			if len(mediaMessages) > 0 {
				openAIMessage.SetMediaContent(mediaMessages)
			}
		}
		if len(openAIMessage.ParseContent()) > 0 || len(openAIMessage.ToolCalls) > 0 {
			openAIMessages = append(openAIMessages, openAIMessage)
		}
	}

	openAIRequest.Messages = openAIMessages
	return &openAIRequest, nil
}

func claudeMediaToOpenAI(media dto.ClaudeMediaMessage) (dto.MediaContent, bool) {
	switch media.Type {
	case "text", "input_text":
		return dto.MediaContent{
			Type:         dto.ContentTypeText,
			Text:         media.GetText(),
			CacheControl: media.CacheControl,
		}, true
	case "image":
		if media.Source == nil {
			return dto.MediaContent{}, false
		}
		imageURL := strings.TrimSpace(media.Source.Url)
		if imageURL == "" {
			data := common.Interface2String(media.Source.Data)
			if data == "" || strings.TrimSpace(media.Source.MediaType) == "" {
				return dto.MediaContent{}, false
			}
			imageURL = fmt.Sprintf("data:%s;base64,%s", media.Source.MediaType, data)
		}
		return dto.MediaContent{
			Type:     dto.ContentTypeImageURL,
			ImageUrl: &dto.MessageImageUrl{Url: imageURL},
		}, true
	default:
		return dto.MediaContent{}, false
	}
}

func convertClaudeTools(tools any, allowResponsesTools bool) ([]dto.ToolCallRequest, []json.RawMessage, map[string]string, *uint, error) {
	if tools == nil {
		return nil, nil, nil, nil, nil
	}

	encoded, err := common.Marshal(tools)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to convert Claude tools: %w", err)
	}
	var rawTools []json.RawMessage
	if err := common.Unmarshal(encoded, &rawTools); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to convert Claude tools: %w", err)
	}

	converted := make([]dto.ToolCallRequest, 0, len(rawTools))
	responsesTools := make([]json.RawMessage, 0, 1)
	nativeToolTypes := make(map[string]string)
	var maxToolCalls *uint
	for index, rawTool := range rawTools {
		var identity struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		if err := common.Unmarshal(rawTool, &identity); err != nil {
			return nil, nil, nil, nil, fmt.Errorf("failed to convert Claude tool %d: %w", index, err)
		}

		toolType := strings.TrimSpace(identity.Type)
		switch toolType {
		case "", "custom":
			var claudeTool dto.Tool
			if err := common.Unmarshal(rawTool, &claudeTool); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("failed to convert Claude tool %d: %w", index, err)
			}
			if strings.TrimSpace(claudeTool.Name) == "" || claudeTool.InputSchema == nil {
				return nil, nil, nil, nil, fmt.Errorf("Claude tool %d requires name and input_schema", index)
			}
			converted = append(converted, dto.ToolCallRequest{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        claudeTool.Name,
					Description: claudeTool.Description,
					Parameters:  claudeTool.InputSchema,
				},
			})
		case "web_search_20250305":
			if !allowResponsesTools {
				return nil, nil, nil, nil, fmt.Errorf("Claude server tool %q requires an OpenAI Responses route", toolType)
			}
			if _, exists := nativeToolTypes["web_search"]; exists {
				return nil, nil, nil, nil, fmt.Errorf("Claude request contains multiple web_search server tools")
			}
			var webSearch dto.ClaudeWebSearchTool
			if err := common.Unmarshal(rawTool, &webSearch); err != nil {
				return nil, nil, nil, nil, fmt.Errorf("failed to convert Claude web_search tool: %w", err)
			}
			if webSearch.Name != "web_search" {
				return nil, nil, nil, nil, fmt.Errorf("Claude web_search server tool requires name web_search")
			}

			responsesTool := map[string]any{"type": "web_search"}
			if location := webSearch.UserLocation; location != nil {
				if location.Type != "" && location.Type != "approximate" {
					return nil, nil, nil, nil, fmt.Errorf("unsupported Claude web_search user_location type %q", location.Type)
				}
				userLocation := map[string]any{"type": "approximate"}
				if location.Timezone != "" {
					userLocation["timezone"] = location.Timezone
				}
				if location.Country != "" {
					userLocation["country"] = location.Country
				}
				if location.Region != "" {
					userLocation["region"] = location.Region
				}
				if location.City != "" {
					userLocation["city"] = location.City
				}
				if len(userLocation) > 0 {
					responsesTool["user_location"] = userLocation
				}
			}
			responsesToolJSON, err := common.Marshal(responsesTool)
			if err != nil {
				return nil, nil, nil, nil, fmt.Errorf("failed to convert Claude web_search tool: %w", err)
			}
			responsesTools = append(responsesTools, responsesToolJSON)
			nativeToolTypes[webSearch.Name] = "web_search"
			if webSearch.MaxUses > 0 {
				limit := uint(webSearch.MaxUses)
				maxToolCalls = &limit
			}
		default:
			return nil, nil, nil, nil, fmt.Errorf("unsupported Claude server tool type %q", toolType)
		}
	}

	return converted, responsesTools, nativeToolTypes, maxToolCalls, nil
}

func requestToJSONString(v interface{}) string {
	b, err := common.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}
