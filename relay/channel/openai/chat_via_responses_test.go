package openai

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayhelper "github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newResponsesChatTestContext(t *testing.T, body string, isStream bool) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "responses-test")

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	info := relaycommon.GenRelayInfoResponses(c, &dto.OpenAIResponsesRequest{Stream: common.GetPointer(isStream)})
	info.ChannelMeta = &relaycommon.ChannelMeta{UpstreamModelName: "gpt-test"}
	info.RelayFormat = types.RelayFormatOpenAI
	info.ShouldIncludeUsage = true
	info.DisablePing = true
	return c, recorder, resp, info
}

func TestOaiResponsesToChatStreamHandlerConvertsSSEOrderAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.completed","response":{"status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, err := OaiResponsesToChatStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 2, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, got, `"role":"assistant"`)
	require.Contains(t, got, `"content":"hello"`)
	require.Contains(t, got, `"name":"lookup"`)
	require.Contains(t, got, `"arguments":"{\"q\":\"x\"}"`)
	require.Contains(t, got, `"finish_reason":"tool_calls"`)
	require.Contains(t, got, `"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5`)
	require.Contains(t, got, `data: [DONE]`)
	requireOrderedSubstrings(t, got,
		`"role":"assistant"`,
		`"content":"hello"`,
		`"name":"lookup"`,
		`"arguments":"{\"q\":\"x\"}"`,
		`"finish_reason":"tool_calls"`,
		`"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5`,
		`data: [DONE]`,
	)
}

func TestOaiResponsesToChatHandlerRejectsCompletedResponseWithoutVisibleOutput(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-test","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeEmptyResponse, apiErr.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatHandlerRejectsWhitespaceOnlyOutput(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":" \n\t "}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeEmptyResponse, apiErr.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatHandlerPreservesRefusalOutput(t *testing.T) {
	body := `{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"I cannot help with that request."}]}],"usage":{"input_tokens":2,"output_tokens":6,"total_tokens":8}}`
	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)

	usage, apiErr := OaiResponsesToChatHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), "I cannot help with that request.")
}

func TestOaiResponsesToChatBufferedStreamHandlerRejectsMissingTerminal(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"partial text"}`,
		`data: [DONE]`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, apiErr := OaiResponsesToChatBufferedStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeBadResponse, apiErr.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatBufferedStreamHandlerRejectsCompletedResponseWithoutVisibleOutput(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, apiErr := OaiResponsesToChatBufferedStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeEmptyResponse, apiErr.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatBufferedStreamHandlerRejectsWhitespaceOnlyOutput(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":" \n\t "}]}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, apiErr := OaiResponsesToChatBufferedStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, types.ErrorCodeEmptyResponse, apiErr.GetErrorCode())
	require.Empty(t, recorder.Body.String())
}

func TestOaiResponsesToChatBufferedStreamHandlerPreservesRefusalOutput(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"I cannot help with that request."}]}],"usage":{"input_tokens":2,"output_tokens":6,"total_tokens":8}}}`,
		``,
	}, "\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, apiErr := OaiResponsesToChatBufferedStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.TotalTokens)
	require.Contains(t, recorder.Body.String(), "I cannot help with that request.")
}

func TestOaiResponsesToChatStreamHandlerRejectsMissingTerminalBeforeStreaming(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "EOF"},
		{name: "bare done", body: "data: [DONE]\n\n"},
		{name: "malformed event", body: "data: {not-json}\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, resp, info := newResponsesChatTestContext(t, tt.body, true)
			info.RelayFormat = types.RelayFormatClaude

			usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

			require.Nil(t, usage)
			require.NotNil(t, apiErr)
			require.Equal(t, types.ErrorCodeBadResponse, apiErr.GetErrorCode())
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestOaiResponsesToChatStreamHandlerEmitsClaudeErrorWhenTerminalIsMissingAfterStart(t *testing.T) {
	body := `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}` + "\n\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Zero(t, usage.TotalTokens)
	require.True(t, info.UpstreamEmptyResponse)
	got := recorder.Body.String()
	require.Contains(t, got, "event: message_start")
	require.Contains(t, got, "event: error")
	require.Contains(t, got, `"type":"error"`)
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatStreamHandlerRejectsWhitespaceOnlyCompletedForOpenAI(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":" \n\t "}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Zero(t, usage.TotalTokens)
	require.True(t, info.UpstreamEmptyResponse)
	got := recorder.Body.String()
	require.Contains(t, got, `"error"`)
	require.NotContains(t, got, `"finish_reason":"stop"`)
}

func TestOaiResponsesToChatStreamHandlerPreservesRefusalDeltaForClaude(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.refusal.delta","delta":"I cannot help with that request."}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","usage":{"input_tokens":2,"output_tokens":6,"total_tokens":8}}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, 8, usage.TotalTokens)
	got := recorder.Body.String()
	require.Contains(t, got, `"type":"text_delta","text":"I cannot help with that request."`)
	require.Contains(t, got, "event: message_stop")
	require.NotContains(t, got, "event: error")
}

func TestOaiResponsesToChatStreamHandlerEmitsClaudeErrorOnParseInterruptionAfterStart(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {not-json}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.True(t, info.UpstreamEmptyResponse)
	got := recorder.Body.String()
	require.Contains(t, got, "event: message_start")
	require.Contains(t, got, "event: error")
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatStreamHandlerKeepsVisiblePartialOutputBillableOnInterruption(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"partial answer"}`,
		`data: {not-json}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.False(t, info.UpstreamEmptyResponse)
	got := recorder.Body.String()
	require.Contains(t, got, `"type":"text_delta","text":"partial answer"`)
	require.Contains(t, got, "event: error")
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatStreamHandlerRejectsEmptyIncompleteAsClaudeStreamError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.incomplete","response":{"id":"resp_1","model":"gpt-test","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	got := recorder.Body.String()
	require.Contains(t, got, "event: message_start")
	require.Contains(t, got, "event: error")
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatStreamHandlerRejectsReasoningOnlyCompletedAsClaudeStreamError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"internal reasoning only"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	got := recorder.Body.String()
	require.Contains(t, got, "event: content_block_start")
	require.Contains(t, got, `"type":"thinking"`)
	require.Contains(t, got, "event: error")
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatStreamHandlerRejectsMalformedToolOnlyCompletedAsClaudeStreamError(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed","output":[{"type":"function_call","arguments":"{}"}]}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	got := recorder.Body.String()
	require.Contains(t, got, "event: error")
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatStreamHandlerAllowsIncompleteWithToolCall(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.incomplete","response":{"id":"resp_1","model":"gpt-test","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[{"type":"function_call","id":"fc_1","call_id":"call_1","name":"canvas_read","arguments":"{}"}],"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	got := recorder.Body.String()
	require.Contains(t, got, "event: content_block_start")
	require.Contains(t, got, `"name":"canvas_read"`)
	require.Contains(t, got, "event: message_stop")
	require.NotContains(t, got, "event: error")
}

func TestOaiResponsesToChatStreamHandlerCompletesClaudeStreamWithoutUpstreamUsage(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","model":"gpt-test","status":"completed"}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	info.RelayFormat = types.RelayFormatClaude

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	got := recorder.Body.String()
	require.Contains(t, got, `"type":"text_delta","text":"hello"`)
	require.Contains(t, got, "event: message_delta")
	require.Contains(t, got, "event: message_stop")
	require.NotContains(t, got, "event: error")
	messageDelta := got[strings.Index(got, "event: message_delta"):]
	require.Regexp(t, `"output_tokens":[1-9][0-9]*`, messageDelta)
}

func TestOaiResponsesToChatStreamHandlerEmitsClaudeErrorOnTimeoutWithoutTerminal(t *testing.T) {
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 1
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	c, recorder, resp, info := newResponsesChatTestContext(t, "", true)
	resp.Body = reader
	info.RelayFormat = types.RelayFormatClaude
	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(writer, `data: {"type":"response.created","response":{"id":"resp_1","model":"gpt-test","created_at":1710000000}}`+"\n\n")
		written <- err
	}()

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)

	require.NoError(t, <-written)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	got := recorder.Body.String()
	require.Contains(t, got, "event: message_start")
	require.Contains(t, got, "event: error")
	require.NotContains(t, got, "event: message_stop")
}

func TestOaiResponsesToChatBufferedStreamHandlerReturnsJSONFromSSE(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	body := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"buffered text"}`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"lookup"}}`,
		`data: {"type":"response.function_call_arguments.delta","output_index":0,"delta":"{\"q\":\"x\"}"}`,
		`data: {"type":"response.done","response":{"model":"gpt-test","status":"completed","usage":{"input_tokens":1,"output_tokens":2,"total_tokens":3}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, false)

	usage, err := OaiResponsesToChatBufferedStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 3, usage.TotalTokens)

	got := recorder.Body.String()
	require.NotContains(t, got, `data:`)
	require.Contains(t, got, `"object":"chat.completion"`)
	require.Contains(t, got, `"content":"buffered text"`)
	require.Contains(t, got, `"name":"lookup"`)
	require.Contains(t, got, `"arguments":"{\"q\":\"x\"}"`)
	require.Contains(t, got, `"finish_reason":"tool_calls"`)
}

func TestOaiChatToResponsesStreamHandlerConvertsSSEOrderAndUsage(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q\":\"x\"}"}}]},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	usage, err := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, err)
	require.NotNil(t, usage)
	require.Equal(t, 2, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
	require.Contains(t, got, `event: response.created`)
	require.Contains(t, got, `event: response.output_text.delta`)
	require.Contains(t, got, `"delta":"hello"`)
	require.Contains(t, got, `event: response.function_call_arguments.delta`)
	require.Contains(t, got, `"delta":"{\"q\":\"x\"}"`)
	require.Contains(t, got, `event: response.completed`)
	require.Contains(t, got, `"input_tokens":2`)
	require.Contains(t, got, `"output_tokens":3`)
	require.Equal(t, []string{"response.completed"}, responsesTerminalEvents(t, got))
	requireOrderedSubstrings(t, got,
		`event: response.created`,
		`event: response.output_item.added`,
		`event: response.output_text.delta`,
		`event: response.output_item.added`,
		`event: response.function_call_arguments.delta`,
		`event: response.output_text.done`,
		`event: response.function_call_arguments.done`,
		`event: response.completed`,
	)
}

func TestOaiChatToResponsesStreamHandlerRetriesExactCompletedTerminalAfterWriteFailure(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"hello"},"finish_reason":null}]}`,
		`data: {"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		`data: {"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	writer := &failOnceOnBridgeTerminalWriter{ResponseWriter: c.Writer, needle: `"type":"response.completed"`}
	c.Writer = writer

	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.True(t, writer.failed, "fixture must fail the first response.completed data write")
	require.Equal(t, 2, usage.PromptTokens)
	require.Equal(t, 3, usage.CompletionTokens)
	require.Equal(t, 5, usage.TotalTokens)

	got := recorder.Body.String()
	require.Equal(t, []string{"response.completed"}, responsesTerminalEvents(t, got))

	var payload map[string]any
	require.NoError(t, common.UnmarshalJsonStr(responsesEventData(t, got, "response.completed"), &payload))
	response, ok := payload["response"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "chatcmpl-responses-test", response["id"])
	require.Equal(t, "completed", response["status"])
	usagePayload, ok := response["usage"].(map[string]any)
	require.True(t, ok)
	require.EqualValues(t, 2, usagePayload["input_tokens"])
	require.EqualValues(t, 3, usagePayload["output_tokens"])
	require.EqualValues(t, 5, usagePayload["total_tokens"])
}

func TestOaiChatToResponsesStreamHandlerTerminatesEarlyErrors(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	validChunk := `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}`
	tests := []struct {
		name     string
		failure  string
		wantCode string
	}{
		{
			name:     "upstream error envelope",
			failure:  `data: {"error":{"message":"provider failed","type":"server_error","code":"server_error"}}`,
			wantCode: "server_error",
		},
		{
			name:     "upstream error envelope without type",
			failure:  `data: {"error":{"message":"provider failed","code":"server_error"}}`,
			wantCode: "server_error",
		},
		{
			name:     "capacity error after protocol output",
			failure:  `data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"}}`,
			wantCode: "server_error",
		},
		{
			name:     "malformed chat chunk",
			failure:  `data: {not-json}`,
			wantCode: "bad_response_body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Join([]string{validChunk, "", tt.failure, ""}, "\n")
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			got := recorder.Body.String()
			require.Equal(t, []string{"response.failed"}, responsesTerminalEvents(t, got))
			require.NotContains(t, got, "event: error")

			var payload map[string]any
			require.NoError(t, common.UnmarshalJsonStr(responsesEventData(t, got, "response.failed"), &payload))
			response, ok := payload["response"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "failed", response["status"])
			errorPayload, ok := response["error"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, tt.wantCode, errorPayload["code"])
		})
	}
}

func TestOaiChatToResponsesStreamHandlerMapsPreStreamServerErrorTo5xx(t *testing.T) {
	body := `data: {"error":{"message":"provider failed","type":"server_error","code":"server_error"}}` + "\n\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
	require.NotNil(t, usage)
	require.NotNil(t, apiErr)
	require.GreaterOrEqual(t, apiErr.StatusCode, http.StatusInternalServerError)
	require.False(t, types.IsPreCommitStreamCapacityError(apiErr))
	require.Zero(t, recorder.Body.Len())
}

func TestOaiChatToResponsesStreamHandlerReturnsPreCommitCapacityFailureForRetry(t *testing.T) {
	body := `data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"}}` + "\n\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Writer.Header().Set("X-Before-Stream", "keep")
	resp.Header.Set("X-Codex-Turn-State", "failed-channel-state")
	info.BeginChannelAttempt()

	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.True(t, types.IsPreCommitStreamCapacityError(apiErr))
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
	require.Equal(t, "keep", c.Writer.Header().Get("X-Before-Stream"))
	require.Empty(t, c.Writer.Header().Get("X-Codex-Turn-State"))
	require.Empty(t, c.Writer.Header().Get("Content-Type"))
	require.Empty(t, c.Writer.Header().Get("Transfer-Encoding"))
	require.False(t, info.HasSendResponse())
	require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
}

func TestOaiChatToResponsesStreamHandlerRetriesCapacityFailureAfterMetadataOnlyPrelude(t *testing.T) {
	body := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		`data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"}}`,
		``,
	}, "\n\n")
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	info.BeginChannelAttempt()

	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
	require.True(t, types.IsPreCommitStreamCapacityError(apiErr))
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
	require.False(t, info.HasSendResponse())
	require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
}

func TestOaiChatToResponsesStreamHandlerDoesNotRetryErrorsAfterMetadataOnlyPrelude(t *testing.T) {
	tests := []struct {
		name    string
		failure string
	}{
		{
			name:    "ordinary upstream error",
			failure: `data: {"error":{"message":"provider failed","type":"server_error","code":"server_error"}}`,
		},
		{
			name:    "capacity error with usage",
			failure: `data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"},"usage":{"prompt_tokens":1,"completion_tokens":0,"total_tokens":1}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Join([]string{
				`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
				tt.failure,
				``,
			}, "\n\n")
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)

			require.Nil(t, apiErr)
			require.NotNil(t, usage)
			require.True(t, c.Writer.Written())
			require.Contains(t, recorder.Body.String(), `event: response.created`)
			require.Equal(t, []string{"response.failed"}, responsesTerminalEvents(t, recorder.Body.String()))
			require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
		})
	}
}

func TestOaiChatToResponsesStreamHandlerTreatsPingAsPreCommitForCapacityRetry(t *testing.T) {
	body := `data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"}}` + "\n\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp.Header.Set("X-Codex-Turn-State", "failed-channel-state")
	info.BeginChannelAttempt()
	require.NoError(t, relayhelper.PingData(c))

	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)

	require.Nil(t, usage)
	require.NotNil(t, apiErr)
	require.True(t, types.IsPreCommitStreamCapacityError(apiErr))
	require.True(t, c.Writer.Written())
	require.Equal(t, ": PING\n\n", recorder.Body.String())
	require.NotContains(t, recorder.Body.String(), "response.failed")
	require.Equal(t, "failed-channel-state", c.Writer.Header().Get("X-Codex-Turn-State"))
	require.False(t, info.HasSendResponse())
	require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
}

func TestOaiChatToResponsesStreamHandlerDoesNotRetryCapacityErrorWithUsageOrOutput(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "usage",
			body: `data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"},"usage":{"prompt_tokens":4,"completion_tokens":1,"total_tokens":5}}` + "\n\n",
		},
		{
			name: "output",
			body: `data: {"error":{"message":"We're currently experiencing high demand, which may cause temporary errors.","type":"server_error","code":"server_error"},"choices":[{"index":0,"message":{"role":"assistant","content":"partial"},"finish_reason":"stop"}]}` + "\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, resp, info := newResponsesChatTestContext(t, tt.body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)

			require.NotNil(t, usage)
			require.NotNil(t, apiErr)
			require.False(t, types.IsPreCommitStreamCapacityError(apiErr))
			require.False(t, c.Writer.Written())
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestOaiChatToResponsesStreamHandlerScannerErrorAfterPartialWritesFailed(t *testing.T) {
	validChunk := `data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"content":"partial"},"finish_reason":null}]}` + "\n\n"
	c, recorder, resp, info := newResponsesChatTestContext(t, "", true)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	resp.Body = io.NopCloser(io.MultiReader(strings.NewReader(validChunk), readError{err: errors.New("upstream read failed")}))

	usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	require.Equal(t, []string{"response.failed"}, responsesTerminalEvents(t, recorder.Body.String()))
	require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
}

func TestOaiChatToResponsesStreamHandlerRejectsEmptyUpstream(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body"},
		{name: "bare done", body: "data: [DONE]\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, resp, info := newResponsesChatTestContext(t, tt.body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.NotNil(t, usage)
			require.NotNil(t, apiErr)
			require.Equal(t, types.ErrorCodeEmptyResponse, apiErr.GetErrorCode())
			require.Empty(t, responsesTerminalEvents(t, recorder.Body.String()))
		})
	}
}

func TestOaiChatToResponsesStreamHandlerTreatsPingOnlyStreamAsEmpty(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "EOF"},
		{name: "done", body: "data: [DONE]\n\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder, resp, info := newResponsesChatTestContext(t, tt.body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			require.NoError(t, relayhelper.PingData(c))

			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)

			got := recorder.Body.String()
			require.Contains(t, got, ": PING")
			require.Equal(t, []string{"response.failed"}, responsesTerminalEvents(t, got))

			var payload map[string]any
			require.NoError(t, common.UnmarshalJsonStr(responsesEventData(t, got, "response.failed"), &payload))
			response, ok := payload["response"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "failed", response["status"])
			errorPayload, ok := response["error"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, string(types.ErrorCodeEmptyResponse), errorPayload["code"])
			require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
		})
	}
}

func TestOaiChatToResponsesStreamHandlerRejectsSemanticallyEmptyChunks(t *testing.T) {
	tests := []struct {
		name       string
		chunk      string
		terminator string
	}{
		{
			name:  "empty choices then EOF",
			chunk: `{"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[]}`,
		},
		{
			name:       "empty choices then done",
			chunk:      `{"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[]}`,
			terminator: "data: [DONE]\n\n",
		},
		{
			name:  "role only then EOF",
			chunk: `{"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		},
		{
			name:       "role only then done",
			chunk:      `{"id":"chatcmpl_upstream","object":"chat.completion.chunk","created":1710000000,"model":"gpt-test","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
			terminator: "data: [DONE]\n\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := "data: " + tt.chunk + "\n\n" + tt.terminator
			c, recorder, resp, info := newResponsesChatTestContext(t, body, true)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			usage, apiErr := OaiChatToResponsesStreamHandler(c, info, resp)
			require.Nil(t, apiErr)
			require.NotNil(t, usage)

			got := recorder.Body.String()
			require.Contains(t, got, "event: response.created")
			require.Equal(t, []string{"response.failed"}, responsesTerminalEvents(t, got))

			var payload map[string]any
			require.NoError(t, common.UnmarshalJsonStr(responsesEventData(t, got, "response.failed"), &payload))
			response, ok := payload["response"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, "failed", response["status"])
			errorPayload, ok := response["error"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, string(types.ErrorCodeEmptyResponse), errorPayload["code"])
			require.Equal(t, relaycommon.StreamEndReasonUpstreamFailed, info.StreamStatus.Snapshot().EndReason)
		})
	}
}

func requireOrderedSubstrings(t *testing.T, s string, parts ...string) {
	t.Helper()

	offset := 0
	for _, part := range parts {
		idx := strings.Index(s[offset:], part)
		require.NotEqualf(t, -1, idx, "missing %q after byte offset %d", part, offset)
		offset += idx + len(part)
	}
}

type readError struct {
	err error
}

type failOnceOnBridgeTerminalWriter struct {
	gin.ResponseWriter
	needle string
	failed bool
}

func (w *failOnceOnBridgeTerminalWriter) Write(data []byte) (int, error) {
	if !w.failed && strings.Contains(string(data), w.needle) {
		w.failed = true
		return 0, io.ErrClosedPipe
	}
	return w.ResponseWriter.Write(data)
}

func (r readError) Read([]byte) (int, error) {
	return 0, r.err
}
