package oaichat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestChatCompletionsRequestToResponsesRequestMapsCacheBreakpointsStably(t *testing.T) {
	for _, model := range []string{"gpt-5.6-luna", "openai/gpt-5.6-luna"} {
		t.Run(model, func(t *testing.T) {
			system := dto.Message{Role: "system"}
			system.SetMediaContent([]dto.MediaContent{{
				Type:         dto.ContentTypeText,
				Text:         "stable system prompt",
				CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
			}})
			request := &dto.GeneralOpenAIRequest{
				Model:          model,
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
			assert.Equal(t, model, gjson.GetBytes(firstJSON, "model").String())
			assert.Equal(t, "claude-affinity-key", gjson.GetBytes(firstJSON, "prompt_cache_key").String())
			assert.Equal(t, "explicit", gjson.GetBytes(firstJSON, "prompt_cache_options.mode").String())
			assert.Equal(t, "developer", gjson.GetBytes(firstJSON, "input.0.role").String())
			assert.Equal(t, "explicit", gjson.GetBytes(firstJSON, "input.0.content.0.prompt_cache_breakpoint.mode").String())
			assert.False(t, gjson.GetBytes(firstJSON, "instructions").Exists())
			assert.Equal(t, "variable question", gjson.GetBytes(firstJSON, "input.1.content").String())
		})
	}
}

func TestChatCompletionsRequestToResponsesRequestDoesNotSendExplicitCachingToOlderModels(t *testing.T) {
	message := dto.Message{Role: "user"}
	message.SetMediaContent([]dto.MediaContent{{
		Type:         dto.ContentTypeText,
		Text:         "stable prefix",
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}})

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model:    "openai/gpt-5.2",
		Messages: []dto.Message{message},
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(encoded, "prompt_cache_options").Exists())
	assert.False(t, gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint").Exists())
}

func TestChatCompletionsRequestToResponsesRequestKeepsSystemInstructionsForOlderModels(t *testing.T) {
	system := dto.Message{Role: "system"}
	system.SetMediaContent([]dto.MediaContent{{
		Type:         dto.ContentTypeText,
		Text:         "stable system prompt",
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}})

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model:    "gpt-5.5",
		Messages: []dto.Message{system, {Role: "user", Content: "hello"}},
	})
	require.NoError(t, err)

	assert.JSONEq(t, `"stable system prompt"`, string(got.Instructions))
	assert.Equal(t, "hello", gjson.GetBytes(got.Input, "0.content").String())
	assert.Equal(t, "user", gjson.GetBytes(got.Input, "0.role").String())
	assert.Empty(t, got.PromptCacheOptions)
}

func TestChatCompletionsRequestToResponsesRequestTransfersAssistantAndRollingCacheBreakpoints(t *testing.T) {
	messages := []dto.Message{
		cachedTextMessage("system", "stable system prompt"),
		cachedTextMessage("user", "historical user 1"),
		cachedTextMessage("assistant", "historical assistant 1"),
		mediaTextMessage("user", "historical user 2", "historical user 2 tail"),
		cachedTextMessage("assistant", "historical assistant 2"),
		mediaTextMessage("user", "latest user message", "latest user tail"),
	}

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model:    "openai/gpt-5.6-luna",
		Messages: messages,
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "prompt_cache_options.mode").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "output_text", gjson.GetBytes(encoded, "input.2.content.0.type").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.3.content.0.prompt_cache_breakpoint.mode").String())
	assert.False(t, gjson.GetBytes(encoded, "input.3.content.1.prompt_cache_breakpoint").Exists())
	assert.Equal(t, "output_text", gjson.GetBytes(encoded, "input.4.content.0.type").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.5.content.0.prompt_cache_breakpoint.mode").String())
	assert.False(t, gjson.GetBytes(encoded, "input.5.content.1.prompt_cache_breakpoint").Exists())
	assert.Equal(t, 4, CountResponsesPromptCacheBreakpoints(got.Input))
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)
}

func TestChatCompletionsRequestToResponsesRequestAddsRollingBreakpointBeforeLastAssistant(t *testing.T) {
	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-terra",
		Messages: []dto.Message{
			{Role: "user", Content: "historical user 1"},
			mediaTextMessage("assistant", "historical assistant 1"),
			mediaTextMessage("user", "historical user 2", "historical user 2 tail"),
			mediaTextMessage("assistant", "historical assistant 2"),
			cachedTextMessage("user", "latest user"),
		},
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "output_text", gjson.GetBytes(encoded, "input.1.content.0.type").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.2.content.0.prompt_cache_breakpoint.mode").String())
	assert.False(t, gjson.GetBytes(encoded, "input.2.content.1.prompt_cache_breakpoint").Exists())
	assert.Equal(t, "output_text", gjson.GetBytes(encoded, "input.3.content.0.type").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.4.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, 2, CountResponsesPromptCacheBreakpoints(got.Input))
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)
}

func TestChatCompletionsRequestToResponsesRequestTransfersAssistantCacheBreakpointBackward(t *testing.T) {
	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-luna",
		Messages: []dto.Message{
			mediaTextMessage("user", "preceding user"),
			cachedTextMessage("assistant", "terminal assistant"),
		},
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "output_text", gjson.GetBytes(encoded, "input.1.content.0.type").String())
	assert.Equal(t, 1, CountResponsesPromptCacheBreakpoints(got.Input))
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)
}

func TestChatCompletionsRequestToResponsesRequestTransfersAssistantNonTextCacheControl(t *testing.T) {
	assistant := dto.Message{Role: "assistant"}
	assistant.SetMediaContent([]dto.MediaContent{
		{
			Type:         dto.ContentTypeImageURL,
			ImageUrl:     "https://example.test/image.png",
			CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
		},
		{
			Type: dto.ContentTypeText,
			Text: "assistant text",
		},
	})

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-luna",
		Messages: []dto.Message{
			{Role: "user", Content: "preceding user"},
			assistant,
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, CountResponsesPromptCacheBreakpoints(got.Input))
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.False(t, gjson.GetBytes(encoded, "input.1.content.0.prompt_cache_breakpoint").Exists())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
}

func TestChatCompletionsRequestToResponsesRequestTransfersAssistantCacheBreakpointToDeveloper(t *testing.T) {
	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-luna",
		Messages: []dto.Message{
			{Role: "user", Content: "first user"},
			cachedTextMessage("assistant", "assistant history"),
			{Role: "developer", Content: "following developer"},
			{Role: "developer", Content: "later developer"},
		},
	})
	require.NoError(t, err)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "developer", gjson.GetBytes(encoded, "input.2.role").String())
	assert.Equal(t, "input_text", gjson.GetBytes(encoded, "input.2.content.0.type").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.2.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "developer", gjson.GetBytes(encoded, "input.3.role").String())
	assert.Equal(t, "later developer", gjson.GetBytes(encoded, "input.3.content.0.text").String())
	assert.Empty(t, got.Instructions)
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)
}

func TestChatCompletionsRequestToResponsesRequestDeduplicatesTransferredBreakpoints(t *testing.T) {
	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-sol",
		Messages: []dto.Message{
			{Role: "user", Content: "first user"},
			cachedTextMessage("assistant", "first assistant"),
			cachedTextMessage("assistant", "second assistant"),
			mediaTextMessage("user", "following user"),
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 2, CountResponsesPromptCacheBreakpoints(got.Input))
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.3.content.0.prompt_cache_breakpoint.mode").String())
}

func TestChatCompletionsRequestToResponsesRequestKeepsExplicitBreakpointsForReads(t *testing.T) {
	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-sol",
		Messages: []dto.Message{
			cachedTextMessage("system", "system prompt"),
			cachedTextMessage("user", "old user 1"),
			cachedTextMessage("user", "old user 2"),
			cachedTextMessage("user", "old user 3"),
			cachedTextMessage("user", "latest user"),
			{Role: "assistant", Content: "latest assistant"},
		},
	})
	require.NoError(t, err)

	assert.Equal(t, 5, CountResponsesPromptCacheBreakpoints(got.Input))

	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.4.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.1.content.0.prompt_cache_breakpoint.mode").String())
	assertNoOutputTextPromptCacheBreakpoints(t, got.Input)
}

func TestChatCompletionsRequestToResponsesRequestKeepsHistoricalReadWindow(t *testing.T) {
	messages := []dto.Message{cachedTextMessage("system", "system prompt")}
	for index := 0; index < 52; index++ {
		messages = append(messages, cachedTextMessage("user", fmt.Sprintf("user turn %d", index)))
	}
	messages = append(messages, dto.Message{Role: "assistant", Content: "latest assistant"})

	got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
		Model:    "gpt-5.6-sol",
		Messages: messages,
	})
	require.NoError(t, err)

	assert.Equal(t, 50, CountResponsesPromptCacheBreakpoints(got.Input))
	encoded, err := common.Marshal(got)
	require.NoError(t, err)
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.False(t, gjson.GetBytes(encoded, "input.1.content.0.prompt_cache_breakpoint").Exists())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.52.content.0.prompt_cache_breakpoint.mode").String())
}

func TestChatCompletionsRequestToResponsesRequestRollingCacheBreakpointEligibility(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		messages []dto.Message
	}{
		{
			name:  "older model",
			model: "gpt-5.5",
			messages: []dto.Message{
				{Role: "user", Content: "first user"},
				cachedTextMessage("assistant", "assistant history"),
				{Role: "user", Content: "latest user"},
			},
		},
		{
			name:  "short request",
			model: "gpt-5.6-luna",
			messages: []dto.Message{
				mediaTextMessage("assistant", "assistant history"),
				{Role: "user", Content: "latest user"},
			},
		},
		{
			name:  "no stable user before last assistant",
			model: "gpt-5.6-luna",
			messages: []dto.Message{
				{Role: "assistant", Content: "first assistant"},
				{Role: "assistant", Content: "terminal assistant"},
				{Role: "user", Content: "latest user"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ChatCompletionsRequestToResponsesRequest(&dto.GeneralOpenAIRequest{
				Model:    tt.model,
				Messages: tt.messages,
			})
			require.NoError(t, err)

			assert.Zero(t, CountResponsesPromptCacheBreakpoints(got.Input))
			assert.Empty(t, got.PromptCacheOptions)
			assertNoOutputTextPromptCacheBreakpoints(t, got.Input)
		})
	}
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

func cachedTextMessage(role string, text string) dto.Message {
	message := dto.Message{Role: role}
	message.SetMediaContent([]dto.MediaContent{{
		Type:         dto.ContentTypeText,
		Text:         text,
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}})
	return message
}

func mediaTextMessage(role string, texts ...string) dto.Message {
	parts := make([]dto.MediaContent, 0, len(texts))
	for _, text := range texts {
		parts = append(parts, dto.MediaContent{Type: dto.ContentTypeText, Text: text})
	}
	message := dto.Message{Role: role}
	message.SetMediaContent(parts)
	return message
}

func assertNoOutputTextPromptCacheBreakpoints(t *testing.T, input json.RawMessage) {
	t.Helper()
	var items []struct {
		Content json.RawMessage `json:"content"`
		Output  json.RawMessage `json:"output"`
	}
	require.NoError(t, common.Unmarshal(input, &items))
	for _, item := range items {
		for _, rawParts := range []json.RawMessage{item.Content, item.Output} {
			var parts []struct {
				Type                  string          `json:"type"`
				PromptCacheBreakpoint json.RawMessage `json:"prompt_cache_breakpoint"`
			}
			if common.Unmarshal(rawParts, &parts) != nil {
				continue
			}
			for _, part := range parts {
				if part.Type == "output_text" {
					assert.Empty(t, part.PromptCacheBreakpoint)
				}
			}
		}
	}
}
