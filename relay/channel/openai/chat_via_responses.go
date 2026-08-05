package openai

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/service/relayconvert"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func OaiResponsesToChatHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	var responsesResp dto.OpenAIResponsesResponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	if err := common.Unmarshal(body, &responsesResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	if oaiError := responsesResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}
	normalizeResponsesToolArguments(&responsesResp, lunaReadToolArgumentsTransform(info))
	if !responsesBridgeResponseHasVisibleOutput(&responsesResp) {
		return nil, types.NewOpenAIError(
			fmt.Errorf("responses completed without output text or a tool call"),
			types.ErrorCodeEmptyResponse,
			http.StatusBadGateway,
		)
	}

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, &responsesResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage

	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(&responsesResp)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatBufferedStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	defer service.CloseResponseBodyGracefully(resp)

	accumulator := relayconvert.NewResponsesBufferedAccumulator()
	usageAccumulator := newResponsesStreamCtx()
	var finalResponse *dto.OpenAIResponsesResponse
	var streamErr *types.NewAPIError
	seenTerminal := false

	scanner := helper.NewStreamScanner(resp.Body)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 6 || line[:5] != "data:" {
			continue
		}
		data := line[5:]
		data = strings.TrimSpace(data)
		if data == "" || data == "[DONE]" {
			if data == "[DONE]" {
				break
			}
			continue
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal buffered responses stream event: "+err.Error())
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
			break
		}
		usageAccumulator.captureResponseMetadata(streamResp)
		accumulator.ProcessEvent(&streamResp)
		switch streamResp.Type {
		case "response.completed", "response.done", "response.incomplete":
			seenTerminal = true
			finalResponse = streamResp.Response
			if finalResponse == nil && (usageAccumulator.usage != nil || len(accumulator.BuildOutput()) > 0) {
				finalResponse = &dto.OpenAIResponsesResponse{
					ID:        usageAccumulator.responseID,
					Model:     usageAccumulator.model,
					CreatedAt: int(usageAccumulator.createdAt),
					Status:    []byte(`"completed"`),
				}
			}
			if streamResp.Type == "response.incomplete" {
				if finalResponse == nil {
					finalResponse = &dto.OpenAIResponsesResponse{}
				}
				if len(finalResponse.Status) == 0 {
					finalResponse.Status = []byte(`"incomplete"`)
				}
			}
		case "response.failed", "response.error":
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					break
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
		if streamErr != nil || seenTerminal {
			break
		}
	}
	if streamErr != nil {
		return nil, streamErr
	}
	if err := scanner.Err(); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	if !seenTerminal {
		return nil, types.NewOpenAIError(
			fmt.Errorf("responses stream ended without a terminal event"),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}
	if finalResponse == nil {
		return nil, types.NewOpenAIError(
			fmt.Errorf("responses terminal event omitted its response payload"),
			types.ErrorCodeBadResponse,
			http.StatusBadGateway,
		)
	}
	if usageAccumulator.usage != nil {
		finalResponse.Usage = usageAccumulator.usage
	}
	accumulator.SupplementResponseOutput(finalResponse)
	normalizeResponsesToolArguments(finalResponse, lunaReadToolArgumentsTransform(info))
	if !responsesBridgeResponseHasVisibleOutput(finalResponse) {
		return nil, types.NewOpenAIError(
			fmt.Errorf("responses completed without output text or a tool call"),
			types.ErrorCodeEmptyResponse,
			http.StatusBadGateway,
		)
	}

	chatResult, err := relayconvert.ConvertResponse(c, info, types.RelayFormatOpenAI, finalResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	chatResp, ok := chatResult.Value.(*dto.OpenAITextResponse)
	if !ok {
		return nil, types.NewOpenAIError(fmt.Errorf("expected OpenAI chat response, got %T", chatResult.Value), types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if chatID := helper.GetResponseID(c); chatID != "" {
		chatResp.Id = chatID
	}
	usage := chatResult.Usage
	if usage == nil || usage.TotalTokens == 0 {
		text := service.ExtractOutputTextFromResponses(finalResponse)
		usage = service.ResponseText2Usage(c, text, info.UpstreamModelName, info.GetEstimatePromptTokens())
		chatResp.Usage = *usage
	}

	responseValue := any(chatResp)
	if info.RelayFormat != types.RelayFormatOpenAI {
		targetResult, err := relayconvert.ConvertResponse(c, info, info.RelayFormat, chatResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		responseValue = targetResult.Value
	}
	responseBody, err := common.Marshal(responseValue)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
	}

	service.IOCopyBytesGracefully(c, resp, responseBody)
	return usage, nil
}

func OaiResponsesToChatStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		return nil, types.NewOpenAIError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}

	defer service.CloseResponseBodyGracefully(resp)

	responseId := helper.GetResponseID(c)
	createAt := time.Now().Unix()
	state, err := relayconvert.NewResponseStreamState(types.RelayFormatOpenAIResponses, info.RelayFormat, relayconvert.ResponseStreamOptions{
		ID:                     responseId,
		Model:                  info.UpstreamModelName,
		Created:                createAt,
		IncludeUsage:           info.RelayFormat == types.RelayFormatClaude,
		ToolArgumentsTransform: lunaReadToolArgumentsTransform(info),
	})
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	streamErr := (*types.NewAPIError)(nil)
	seenTerminal := false
	seenVisibleOutput := false
	usageAccumulator := newResponsesStreamCtx()

	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo == nil {
		info.ClaudeConvertInfo = &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}
	}

	sendGeminiResponse := func(geminiResponse *dto.GeminiChatResponse) bool {
		if geminiResponse == nil {
			return true
		}
		geminiResponseStr, err := common.Marshal(geminiResponse)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeJsonMarshalFailed, http.StatusInternalServerError)
			return false
		}
		c.Render(-1, common.CustomEvent{Data: "data: " + string(geminiResponseStr)})
		_ = helper.FlushWriter(c)
		return true
	}

	sendStreamResult := func(result relayconvert.ResponseResult) bool {
		switch value := result.Value.(type) {
		case dto.ChatCompletionsStreamResponse:
			if len(value.Choices) == 0 && value.Usage == nil {
				return true
			}
			if err := helper.ObjectData(c, &value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ChatCompletionsStreamResponse:
			if value == nil || (len(value.Choices) == 0 && value.Usage == nil) {
				return true
			}
			if err := helper.ObjectData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.ClaudeResponse:
			if err := helper.ClaudeData(c, value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case *dto.ClaudeResponse:
			if value == nil {
				return true
			}
			if err := helper.ClaudeData(c, *value); err != nil {
				streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
				return false
			}
			return true
		case dto.GeminiChatResponse:
			return sendGeminiResponse(&value)
		case *dto.GeminiChatResponse:
			return sendGeminiResponse(value)
		default:
			streamErr = types.NewOpenAIError(fmt.Errorf("unsupported converted stream response type %T", result.Value), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			return false
		}
	}

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {
		if streamErr != nil {
			sr.Stop(streamErr)
			return
		}

		var streamResp dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResp); err != nil {
			logger.LogError(c, "failed to unmarshal responses stream event: "+err.Error())
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		usageAccumulator.captureResponseMetadata(streamResp)
		if usageAccumulator.usage != nil {
			state.SetUsage(relayconvert.UsageFromResponsesUsage(usageAccumulator.usage))
		}

		if streamResp.Type == "response.error" || streamResp.Type == "response.failed" {
			if streamResp.Response != nil {
				if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
					streamErr = types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
					sr.Stop(streamErr)
					return
				}
			}
			streamErr = types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}

		if responsesBridgeEventHasVisibleOutput(&streamResp) {
			seenVisibleOutput = true
		}
		isTerminal := streamResp.Type == "response.completed" ||
			streamResp.Type == "response.done" ||
			streamResp.Type == "response.incomplete"
		var terminalUsage *dto.Usage
		if isTerminal {
			seenTerminal = true
			if usageAccumulator.usage != nil {
				if streamResp.Response == nil {
					streamResp.Response = &dto.OpenAIResponsesResponse{}
				}
				streamResp.Response.Usage = usageAccumulator.usage
				terminalUsage = relayconvert.UsageFromResponsesUsage(usageAccumulator.usage)
			}
		}
		if isTerminal && !seenVisibleOutput {
			streamErr = types.NewOpenAIError(
				fmt.Errorf("responses stream terminal event contained no output text or tool call"),
				types.ErrorCodeBadResponse,
				http.StatusInternalServerError,
			)
			sr.Stop(streamErr)
			return
		}
		if isTerminal && (state.Usage() == nil || state.Usage().TotalTokens == 0) {
			state.SetUsage(service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens()))
		}

		results, err := relayconvert.ConvertStreamResponseChunk(c, info, state, &streamResp)
		if err != nil {
			streamErr = types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			sr.Stop(streamErr)
			return
		}
		for _, result := range results {
			if !sendStreamResult(result) {
				sr.Stop(streamErr)
				return
			}
		}
		if terminalUsage != nil {
			// The Chat-to-Claude hop canonicalizes usage as Chat usage; restore the
			// Responses source/details used by settlement and cache observability.
			state.SetUsage(terminalUsage)
		}
		if isTerminal {
			sr.Done()
		}
	})

	if streamErr == nil && !seenTerminal {
		streamErr = types.NewOpenAIError(
			fmt.Errorf("responses stream ended without a terminal event"),
			types.ErrorCodeBadResponse,
			http.StatusInternalServerError,
		)
		if info.StreamStatus != nil {
			info.StreamStatus.OverrideEndReasonIfNoProtocolTerminal(
				relaycommon.StreamEndReasonUpstreamFailed,
				streamErr,
				"responses_bridge_missing_terminal",
			)
			info.StreamStatus.RecordError(streamErr.Error())
		}
	}
	if streamErr != nil {
		if c.Writer.Written() {
			if !seenVisibleOutput {
				// The HTTP stream was already committed, so return a zero usage to
				// finish the handler while carrying the existing explicit empty-
				// response contract into settlement and channel-health accounting.
				info.UpstreamEmptyResponse = true
			}
			usage := responsesBridgeNonBillableUsage(info)
			if seenVisibleOutput {
				usage = responsesBridgeStreamUsage(c, info, state)
			}
			if err := writeResponsesBridgeStreamError(c, info, streamErr); err != nil {
				return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
			}
			if info.StreamStatus != nil {
				info.StreamStatus.SetProtocolTerminalEndReasonWithSource(
					relaycommon.StreamEndReasonUpstreamFailed,
					streamErr,
					"responses_bridge_error",
				)
			}
			return usage, nil
		}
		return nil, streamErr
	}

	usage := responsesBridgeStreamUsage(c, info, state)
	finalResults, err := relayconvert.FinalizeStreamResponse(c, info, state)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
	}
	for _, result := range finalResults {
		if !sendStreamResult(result) {
			return nil, streamErr
		}
	}
	if info.RelayFormat == types.RelayFormatOpenAI && info.ShouldIncludeUsage && usage != nil {
		if err := helper.ObjectData(c, helper.GenerateFinalUsageResponse(responseId, createAt, info.UpstreamModelName, *usage)); err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponse, http.StatusInternalServerError)
		}
	}

	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return usage, nil
}

func lunaReadToolArgumentsTransform(info *relaycommon.RelayInfo) relayconvert.ToolArgumentsTransform {
	if info == nil || info.RelayFormat != types.RelayFormatClaude || info.OriginModelName != "gpt-5.6-luna" {
		return nil
	}
	return normalizeLunaReadToolArguments
}

func normalizeLunaReadToolArguments(toolName, arguments string) (string, bool) {
	if toolName != "Read" {
		return arguments, false
	}
	if !gjson.Valid(arguments) {
		return arguments, true
	}
	pages := gjson.Get(arguments, "pages")
	if !pages.Exists() || pages.Type != gjson.String || pages.String() != "" {
		return arguments, true
	}
	normalized, err := sjson.Delete(arguments, "pages")
	if err != nil {
		return arguments, true
	}
	return normalized, true
}

func normalizeResponsesToolArguments(response *dto.OpenAIResponsesResponse, transform relayconvert.ToolArgumentsTransform) {
	if response == nil || transform == nil {
		return
	}
	for i := range response.Output {
		output := &response.Output[i]
		arguments := output.ArgumentsString()
		normalized, matched := transform(output.Name, arguments)
		if !matched || normalized == arguments {
			continue
		}
		output.Arguments = []byte(normalized)
	}
}

func responsesBridgeEventHasVisibleOutput(event *dto.ResponsesStreamResponse) bool {
	if event == nil {
		return false
	}
	if (event.Type == "response.output_text.delta" || event.Type == "response.refusal.delta") &&
		strings.TrimSpace(event.Delta) != "" {
		return true
	}
	if responsesBridgeOutputIsVisible(event.Item, event.OutputIndex != nil) {
		return true
	}
	for i := range event.Output {
		if responsesBridgeOutputIsVisible(&event.Output[i], false) {
			return true
		}
	}
	return responsesBridgeResponseHasVisibleOutput(event.Response)
}

func responsesBridgeResponseHasVisibleOutput(response *dto.OpenAIResponsesResponse) bool {
	if response == nil {
		return false
	}
	for i := range response.Output {
		if responsesBridgeOutputIsVisible(&response.Output[i], false) {
			return true
		}
	}
	return false
}

func responsesBridgeOutputIsVisible(output *dto.ResponsesOutput, hasOutputIndex bool) bool {
	if output == nil {
		return false
	}
	switch output.Type {
	case "function_call", "custom_tool_call":
		return strings.TrimSpace(output.Name) != "" &&
			(hasOutputIndex || strings.TrimSpace(output.ID) != "" || strings.TrimSpace(output.CallId) != "")
	case "message":
		for _, content := range output.Content {
			if content.Type == "output_text" && strings.TrimSpace(content.Text) != "" {
				return true
			}
			if content.Type == "refusal" && strings.TrimSpace(content.Refusal) != "" {
				return true
			}
		}
	}
	return false
}

func responsesBridgeNonBillableUsage(info *relaycommon.RelayInfo) *dto.Usage {
	semantic := dto.BillingUsageSemanticOpenAI
	if info != nil && info.RelayFormat == types.RelayFormatClaude {
		semantic = dto.BillingUsageSemanticAnthropic
	}
	return &dto.Usage{UsageSemantic: semantic}
}

func responsesBridgeStreamUsage(c *gin.Context, info *relaycommon.RelayInfo, state *relayconvert.ResponseStreamState) *dto.Usage {
	usage := state.Usage()
	if usage == nil || usage.TotalTokens == 0 {
		usage = service.ResponseText2Usage(c, state.UsageText(), info.UpstreamModelName, info.GetEstimatePromptTokens())
		state.SetUsage(usage)
	}
	if info.RelayFormat == types.RelayFormatClaude && info.ClaudeConvertInfo != nil {
		info.ClaudeConvertInfo.Usage = usage
	}
	return usage
}

func writeResponsesBridgeStreamError(c *gin.Context, info *relaycommon.RelayInfo, streamErr *types.NewAPIError) error {
	if info.RelayFormat == types.RelayFormatClaude {
		claudeErr := streamErr.ToClaudeError()
		claudeErr.Type = "api_error"
		response := dto.ClaudeResponse{
			Type:  "error",
			Error: claudeErr,
		}
		data, err := common.Marshal(response)
		if err != nil {
			return err
		}
		return helper.ClaudeChunkData(c, response, string(data))
	}

	if err := helper.ObjectData(c, map[string]any{"error": streamErr.ToOpenAIError()}); err != nil {
		return err
	}
	if info.RelayFormat == types.RelayFormatOpenAI {
		helper.Done(c)
	}
	return nil
}
