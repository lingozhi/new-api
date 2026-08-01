package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyClaudeResponsesEffortPreservesMax(t *testing.T) {
	claudeRequest := &dto.ClaudeRequest{
		OutputConfig: json.RawMessage(`{"effort":"max"}`),
	}
	openAIRequest := &dto.GeneralOpenAIRequest{
		Model: "gpt-5.6-luna",
	}

	applyClaudeResponsesEffort(claudeRequest, openAIRequest)

	assert.Equal(t, "max", openAIRequest.ReasoningEffort)
	result, err := service.ConvertRequestVia(
		nil,
		&relaycommon.RelayInfo{},
		openAIRequest,
		types.RelayFormatOpenAI,
		types.RelayFormatOpenAIResponses,
	)
	require.NoError(t, err)
	responsesRequest, ok := result.Value.(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	require.NotNil(t, responsesRequest.Reasoning)
	assert.Equal(t, "max", responsesRequest.Reasoning.Effort)
}

func TestApplyClaudeResponsesEffortKeepsExistingValueWhenMissing(t *testing.T) {
	claudeRequest := &dto.ClaudeRequest{}
	openAIRequest := &dto.GeneralOpenAIRequest{
		ReasoningEffort: "high",
	}

	applyClaudeResponsesEffort(claudeRequest, openAIRequest)

	assert.Equal(t, "high", openAIRequest.ReasoningEffort)
}

func TestShouldClaudeRequestUseResponsesForcesLunaToolReasoningWhenPolicyDisabled(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalPolicy := settings.ChatCompletionsToResponsesPolicy
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: false}
	t.Cleanup(func() {
		settings.ChatCompletionsToResponsesPolicy = originalPolicy
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.6-luna",
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	request := &dto.ClaudeRequest{
		Model:        "provider-luna-upstream-alias",
		Tools:        []any{map[string]any{"name": "canvas_read"}},
		OutputConfig: json.RawMessage(`{"effort":"max"}`),
	}

	assert.True(t, shouldClaudeRequestUseResponses(info, request))
}

func TestShouldClaudeRequestUseResponsesDoesNotForceOtherRequests(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalPolicy := settings.ChatCompletionsToResponsesPolicy
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: false}
	t.Cleanup(func() {
		settings.ChatCompletionsToResponsesPolicy = originalPolicy
	})

	tests := []struct {
		name   string
		model  string
		tools  any
		effort string
	}{
		{name: "no tools", model: "gpt-5.6-luna", effort: "max"},
		{name: "effort none", model: "gpt-5.6-luna", tools: []any{map[string]any{"name": "canvas_read"}}, effort: "none"},
		{name: "other GPT 5.6 model", model: "gpt-5.6-sol", tools: []any{map[string]any{"name": "canvas_read"}}, effort: "max"},
		{name: "responses compact endpoint alias", model: "gpt-5.6-luna-openai-compact", tools: []any{map[string]any{"name": "canvas_read"}}, effort: "max"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				OriginModelName: tt.model,
				ChannelMeta:     &relaycommon.ChannelMeta{},
			}
			request := &dto.ClaudeRequest{
				Model:        tt.model,
				Tools:        tt.tools,
				OutputConfig: json.RawMessage(`{"effort":"` + tt.effort + `"}`),
			}

			assert.False(t, shouldClaudeRequestUseResponses(info, request))
		})
	}
}

func TestShouldRouteClaudeRequestViaResponsesOverridesPassThroughForLunaCompatibility(t *testing.T) {
	settings := model_setting.GetGlobalSettings()
	originalPolicy := settings.ChatCompletionsToResponsesPolicy
	settings.ChatCompletionsToResponsesPolicy = model_setting.ChatCompletionsToResponsesPolicy{Enabled: false}
	t.Cleanup(func() {
		settings.ChatCompletionsToResponsesPolicy = originalPolicy
	})

	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.6-luna",
		ChannelMeta:     &relaycommon.ChannelMeta{},
	}
	request := &dto.ClaudeRequest{
		Model:        "provider-luna-upstream-alias",
		Tools:        []any{map[string]any{"name": "canvas_read"}},
		OutputConfig: json.RawMessage(`{"effort":"max"}`),
	}

	assert.True(t, shouldRouteClaudeRequestViaResponses(info, request, true, true))
}
