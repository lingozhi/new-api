package oaichat

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatCompletionsRequestToResponsesRequestMapsCacheBreakpointsStably(t *testing.T) {
	system := dto.Message{Role: "system"}
	system.SetMediaContent([]dto.MediaContent{{
		Type:         dto.ContentTypeText,
		Text:         "stable system prompt",
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}})
	request := &dto.GeneralOpenAIRequest{
		Model:          "gpt-5.6-luna",
		PromptCacheKey: "claude-affinity-key",
		Messages: []dto.Message{
			system,
			{Role: "user", Content: "variable question"},
		},
	}

	first, err := ChatCompletionsRequestToResponsesRequest(request)
	require.NoError(t, err)
	second, err := ChatCompletionsRequestToResponsesRequest(request)
	require.NoError(t, err)

	firstJSON, err := common.Marshal(first)
	require.NoError(t, err)
	secondJSON, err := common.Marshal(second)
	require.NoError(t, err)
	assert.True(t, bytes.Equal(firstJSON, secondJSON), "identical requests must serialize byte-for-byte identically")
	assert.Equal(t, "claude-affinity-key", gjson.GetBytes(firstJSON, "prompt_cache_key").String())
	assert.Equal(t, "explicit", gjson.GetBytes(firstJSON, "prompt_cache_options.mode").String())
	assert.Equal(t, "system", gjson.GetBytes(firstJSON, "input.0.role").String())
	assert.Equal(t, "explicit", gjson.GetBytes(firstJSON, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "variable question", gjson.GetBytes(firstJSON, "input.1.content").String())
}

func TestChatCompletionsRequestToResponsesRequestDoesNotSendExplicitCachingToOlderModels(t *testing.T) {
	message := dto.Message{Role: "user"}
	message.SetMediaContent([]dto.MediaContent{{
		Type:         dto.ContentTypeText,
		Text:         "stable prefix",
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}})

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model:    "gpt-5.5",
		Messages: []dto.Message{message},
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(encoded, "prompt_cache_options").Exists())
	assert.False(t, gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint").Exists())
}

func TestChatCompletionsRequestToResponsesRequestInstructionsAndTools(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model: "gpt-test",
		N:     lo.ToPtr(1),
		Messages: []dto.Message{
			{Role: "system", Content: "system rules"},
			{Role: "developer", Content: "developer rules"},
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "look"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.test/a.png"}},
			}},
			assistantMessageWithTool("partial text", "call_1", "lookup", `{"q":"x"}`),
			{Role: "tool", ToolCallId: "call_1", Content: "tool result"},
		},
	}

	got, err := ChatCompletionsRequestToResponsesRequest(req)
	require.NoError(t, err)

	assert.Equal(t, "gpt-test", got.Model)
	assert.Equal(t, `"system rules\n\ndeveloper rules"`, string(got.Instructions))
	assert.Equal(t, "input_image", gjson.GetBytes(got.Input, "0.content.1.type").String())
	assert.Equal(t, "function_call", gjson.GetBytes(got.Input, "2.type").String())
	assert.Equal(t, "call_1", gjson.GetBytes(got.Input, "2.call_id").String())
	assert.Equal(t, "function_call_output", gjson.GetBytes(got.Input, "3.type").String())
}

func TestChatCompletionsRequestToResponsesRequestRejectsMultipleChoices(t *testing.T) {
	_, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-test",
		N:     lo.ToPtr(2),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "n>1")
}

func TestChatCompletionsRequestToResponsesRequestPreservesMultimodalToolOutput(t *testing.T) {
	toolResult := dto.Message{Role: "tool", ToolCallId: "call_1"}
	toolResult.SetMediaContent([]dto.MediaContent{
		{Type: dto.ContentTypeText, Text: "Rendered preview"},
		{
			Type: dto.ContentTypeImageURL,
			ImageUrl: &dto.MessageImageUrl{
				Url: "data:image/jpeg;base64,cGl4ZWxz",
			},
		},
	})

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model:    "gpt-test",
		Messages: []dto.Message{toolResult},
	})

	require.NoError(t, err)
	assert.Equal(t, "function_call_output", gjson.GetBytes(got.Input, "0.type").String())
	assert.Equal(t, "call_1", gjson.GetBytes(got.Input, "0.call_id").String())
	assert.Equal(t, "input_text", gjson.GetBytes(got.Input, "0.output.0.type").String())
	assert.Equal(t, "Rendered preview", gjson.GetBytes(got.Input, "0.output.0.text").String())
	assert.Equal(t, "input_image", gjson.GetBytes(got.Input, "0.output.1.type").String())
	assert.Equal(t, "data:image/jpeg;base64,cGl4ZWxz", gjson.GetBytes(got.Input, "0.output.1.image_url").String())
}

func assistantMessageWithTool(content string, id string, name string, args string) dto.Message {
	msg := dto.Message{Role: "assistant", Content: content}
	msg.SetToolCalls([]dto.ToolCallRequest{
		{
			ID:   id,
			Type: "function",
			Function: dto.FunctionRequest{
				Name:      name,
				Arguments: args,
			},
		},
	})
	return msg
}
