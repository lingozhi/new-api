package relay

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	openaichannel "github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func applySystemPromptIfNeeded(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) {
	if info == nil || request == nil {
		return
	}
	if info.ChannelSetting.SystemPrompt == "" {
		return
	}

	systemRole := request.GetSystemRoleName()

	containSystemPrompt := false
	for _, message := range request.Messages {
		if message.Role == systemRole {
			containSystemPrompt = true
			break
		}
	}
	if !containSystemPrompt {
		systemMessage := dto.Message{
			Role:    systemRole,
			Content: info.ChannelSetting.SystemPrompt,
		}
		request.Messages = append([]dto.Message{systemMessage}, request.Messages...)
		return
	}

	if !info.ChannelSetting.SystemPromptOverride {
		return
	}

	common.SetContextKey(c, constant.ContextKeySystemPromptOverride, true)
	for i, message := range request.Messages {
		if message.Role != systemRole {
			continue
		}
		if message.IsStringContent() {
			request.Messages[i].SetStringContent(info.ChannelSetting.SystemPrompt + "\n" + message.StringContent())
			return
		}
		contents := message.ParseContent()
		contents = append([]dto.MediaContent{
			{
				Type: dto.ContentTypeText,
				Text: info.ChannelSetting.SystemPrompt,
			},
		}, contents...)
		request.Messages[i].Content = contents
		return
	}
}

func chatCompletionsViaResponses(c *gin.Context, info *relaycommon.RelayInfo, adaptor channel.Adaptor, request *dto.GeneralOpenAIRequest) (*dto.Usage, *types.NewAPIError) {
	chatJSON, err := common.Marshal(request)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	chatJSON, err = materializeResponsesOnlyRequestFields(chatJSON, request)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	if len(info.ParamOverride) > 0 {
		chatJSON, err = relaycommon.ApplyParamOverrideWithRelayInfo(chatJSON, info)
		if err != nil {
			return nil, newAPIErrorFromParamOverride(err)
		}
	}
	if err := helper.ValidateUnifiedImagePayload(info, chatJSON); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}

	overriddenChatReq, err := parseChatRequestForResponses(chatJSON)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeChannelParamOverrideInvalid, types.ErrOptionWithSkipRetry())
	}

	result, err := service.ConvertRequestVia(c, info, overriddenChatReq, types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	responsesReq, ok := result.Value.(*dto.OpenAIResponsesRequest)
	if !ok {
		return nil, types.NewError(fmt.Errorf("expected OpenAI responses request, got %T", result.Value), types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	savedRelayMode := info.RelayMode
	savedRequestURLPath := info.RequestURLPath
	defer func() {
		info.RelayMode = savedRelayMode
		info.RequestURLPath = savedRequestURLPath
	}()

	info.RelayMode = relayconstant.RelayModeResponses
	info.RequestURLPath = "/v1/responses"

	convertedRequest, err := adaptor.ConvertOpenAIResponsesRequest(c, info, *responsesReq)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	relaycommon.AppendRequestConversionFromRequest(info, convertedRequest)

	jsonData, err := common.Marshal(convertedRequest)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}

	jsonData, err = relaycommon.RemoveDisabledFields(jsonData, info.ChannelOtherSettings, info.ChannelSetting.PassThroughBodyEnabled)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	if err := helper.ValidateUnifiedImagePayload(info, jsonData); err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	body, size, closer, err := relaycommon.NewOutboundJSONBody(jsonData)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeConvertRequestFailed, types.ErrOptionWithSkipRetry())
	}
	defer closer.Close()
	jsonData = nil
	info.UpstreamRequestBodySize = size
	var requestBody io.Reader = body

	var httpResp *http.Response
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeDoRequestFailed, http.StatusInternalServerError)
	}
	if resp == nil {
		return nil, types.NewOpenAIError(nil, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	statusCodeMappingStr := c.GetString("status_code_mapping")

	httpResp = resp.(*http.Response)
	clientStream := info.IsStream
	upstreamStream := isResponsesEventStreamContentType(httpResp.Header.Get("Content-Type"))
	info.IsStream = clientStream || upstreamStream
	if httpResp.StatusCode != http.StatusOK {
		newApiErr := service.RelayErrorHandler(c.Request.Context(), httpResp, false)
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}

	if upstreamStream && clientStream {
		usage, newApiErr := openaichannel.OaiResponsesToChatStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}
	if upstreamStream {
		info.IsStream = false
		usage, newApiErr := openaichannel.OaiResponsesToChatBufferedStreamHandler(c, info, httpResp)
		if newApiErr != nil {
			service.ResetStatusCode(newApiErr, statusCodeMappingStr)
			return nil, newApiErr
		}
		return usage, nil
	}

	usage, newApiErr := openaichannel.OaiResponsesToChatHandler(c, info, httpResp)
	if newApiErr != nil {
		service.ResetStatusCode(newApiErr, statusCodeMappingStr)
		return nil, newApiErr
	}
	return usage, nil
}

func materializeResponsesOnlyRequestFields(chatJSON []byte, request *dto.GeneralOpenAIRequest) ([]byte, error) {
	if request == nil || (len(request.ResponsesTools) == 0 && len(request.ResponsesToolChoice) == 0 && request.ResponsesMaxToolCalls == nil) {
		return chatJSON, nil
	}

	var body map[string]json.RawMessage
	if err := common.Unmarshal(chatJSON, &body); err != nil {
		return nil, err
	}
	if len(request.ResponsesTools) > 0 {
		var tools []json.RawMessage
		if rawTools, ok := body["tools"]; ok {
			if err := common.Unmarshal(rawTools, &tools); err != nil {
				return nil, fmt.Errorf("invalid intermediate tools: %w", err)
			}
		}
		tools = append(tools, request.ResponsesTools...)
		encodedTools, err := common.Marshal(tools)
		if err != nil {
			return nil, err
		}
		body["tools"] = encodedTools
	}
	if len(request.ResponsesToolChoice) > 0 {
		body["tool_choice"] = append(json.RawMessage(nil), request.ResponsesToolChoice...)
	}
	if request.ResponsesMaxToolCalls != nil {
		encodedLimit, err := common.Marshal(*request.ResponsesMaxToolCalls)
		if err != nil {
			return nil, err
		}
		body["max_tool_calls"] = encodedLimit
	}
	return common.Marshal(body)
}

func parseChatRequestForResponses(chatJSON []byte) (*dto.GeneralOpenAIRequest, error) {
	var body map[string]json.RawMessage
	if err := common.Unmarshal(chatJSON, &body); err != nil {
		return nil, err
	}

	responsesTools := make([]json.RawMessage, 0)
	if rawTools, ok := body["tools"]; ok {
		var tools []json.RawMessage
		if err := common.Unmarshal(rawTools, &tools); err != nil {
			return nil, fmt.Errorf("invalid overridden tools: %w", err)
		}
		chatTools := make([]json.RawMessage, 0, len(tools))
		for _, rawTool := range tools {
			var identity struct {
				Type string `json:"type"`
			}
			if err := common.Unmarshal(rawTool, &identity); err != nil {
				return nil, fmt.Errorf("invalid overridden tool: %w", err)
			}
			if identity.Type == "function" {
				chatTools = append(chatTools, rawTool)
			} else {
				responsesTools = append(responsesTools, rawTool)
			}
		}
		encodedTools, err := common.Marshal(chatTools)
		if err != nil {
			return nil, err
		}
		body["tools"] = encodedTools
	}

	var responsesToolChoice json.RawMessage
	if rawChoice, ok := body["tool_choice"]; ok && len(rawChoice) > 0 && string(rawChoice) != "null" {
		var identity struct {
			Type string `json:"type"`
		}
		if common.Unmarshal(rawChoice, &identity) == nil && identity.Type != "" && identity.Type != "function" {
			responsesToolChoice = append(json.RawMessage(nil), rawChoice...)
			delete(body, "tool_choice")
		}
	}

	var responsesMaxToolCalls *uint
	if rawLimit, ok := body["max_tool_calls"]; ok {
		if string(rawLimit) != "null" {
			var limit uint
			if err := common.Unmarshal(rawLimit, &limit); err != nil {
				return nil, fmt.Errorf("invalid max_tool_calls: %w", err)
			}
			responsesMaxToolCalls = &limit
		}
		delete(body, "max_tool_calls")
	}

	cleanedJSON, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	var request dto.GeneralOpenAIRequest
	if err := common.Unmarshal(cleanedJSON, &request); err != nil {
		return nil, err
	}
	request.ResponsesTools = responsesTools
	request.ResponsesToolChoice = responsesToolChoice
	request.ResponsesMaxToolCalls = responsesMaxToolCalls
	return &request, nil
}

func isResponsesEventStreamContentType(contentType string) bool {
	return strings.Contains(strings.ToLower(contentType), "text/event-stream")
}
