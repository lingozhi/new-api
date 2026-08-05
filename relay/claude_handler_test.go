package relay

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
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

func TestClaudeResponsesPromptCacheKeyFallbackPrefersMetadataUserID(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model:    "provider-luna-alias",
		Metadata: json.RawMessage(`{"user_id":"session-123"}`),
	}
	info := &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.6-luna",
		TokenId:         41,
	}

	first, ok := getClaudeResponsesPromptCacheKey(nil, request, info)
	require.True(t, ok)
	second, ok := getClaudeResponsesPromptCacheKey(nil, request, &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.6-luna",
		TokenId:         42,
	})
	require.True(t, ok)
	otherUser, ok := getClaudeResponsesPromptCacheKey(nil, &dto.ClaudeRequest{
		Model:    request.Model,
		Metadata: json.RawMessage(`{"user_id":"session-456"}`),
	}, info)
	require.True(t, ok)
	otherModel, ok := getClaudeResponsesPromptCacheKey(nil, request, &relaycommon.RelayInfo{
		OriginModelName: "gpt-5.6-luna-preview",
		TokenId:         info.TokenId,
	})
	require.True(t, ok)

	assert.Equal(t, first, second)
	assert.NotEqual(t, first, otherUser)
	assert.NotEqual(t, first, otherModel)
	assert.Regexp(t, `^claude:[0-9a-f]{40}$`, first)
	assert.NotContains(t, first, "session-123")
	assert.Equal(t, "metadata_user_id", info.PromptCacheKeySource)
}

func TestClaudeResponsesPromptCacheKeyPreservesAffinityPriority(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	originalEnabled := setting.Enabled
	originalRules := setting.Rules
	setting.Enabled = true
	setting.Rules = []operation_setting.ChannelAffinityRule{
		{
			Name:       "test luna affinity priority",
			ModelRegex: []string{`^gpt-5\.6-luna$`},
			PathRegex:  []string{`^/v1/messages$`},
			KeySources: []operation_setting.ChannelAffinityKeySource{
				{Type: "gjson", Path: "metadata.user_id"},
			},
			IncludeRuleName:  true,
			IncludeModelName: true,
		},
	}
	t.Cleanup(func() {
		setting.Enabled = originalEnabled
		setting.Rules = originalRules
	})

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/messages",
		strings.NewReader(`{"metadata":{"user_id":"affinity-user"}}`),
	)
	_, _ = service.GetPreferredChannelByAffinity(ctx, "gpt-5.6-luna", "default")
	expected, ok := service.GetChannelAffinityPromptCacheKey(ctx)
	require.True(t, ok)

	request := &dto.ClaudeRequest{
		Model:    "gpt-5.6-luna",
		Metadata: json.RawMessage(`{"user_id":"fallback-user"}`),
	}
	info := &relaycommon.RelayInfo{TokenId: 41}
	fallback, ok := getClaudeResponsesPromptCacheKey(nil, request, info)
	require.True(t, ok)
	actual, ok := getClaudeResponsesPromptCacheKey(ctx, request, info)
	require.True(t, ok)

	assert.Equal(t, expected, actual)
	assert.NotEqual(t, fallback, actual)
	assert.Equal(t, "affinity", info.PromptCacheKeySource)
}

func TestClaudeResponsesPromptCacheKeyFallbackUsesPositiveTokenID(t *testing.T) {
	request := &dto.ClaudeRequest{
		Model:    "gpt-5.6-luna",
		Metadata: json.RawMessage(`{"user_id":42}`),
	}

	info := &relaycommon.RelayInfo{TokenId: 101}
	first, ok := getClaudeResponsesPromptCacheKey(nil, request, info)
	require.True(t, ok)
	second, ok := getClaudeResponsesPromptCacheKey(nil, request, &relaycommon.RelayInfo{TokenId: 101})
	require.True(t, ok)
	otherToken, ok := getClaudeResponsesPromptCacheKey(nil, request, &relaycommon.RelayInfo{TokenId: 202})
	require.True(t, ok)

	assert.Equal(t, first, second)
	assert.NotEqual(t, first, otherToken)
	assert.Regexp(t, `^claude:[0-9a-f]{40}$`, first)
	assert.NotContains(t, first, "101")
	assert.Equal(t, "token_id", info.PromptCacheKeySource)
}

func TestClaudeResponsesPromptCacheKeyFallbackSkipsMissingIdentifier(t *testing.T) {
	tests := []struct {
		name    string
		request *dto.ClaudeRequest
		info    *relaycommon.RelayInfo
	}{
		{name: "nil request", info: &relaycommon.RelayInfo{TokenId: 1}},
		{name: "missing metadata and token", request: &dto.ClaudeRequest{Model: "gpt-5.6-luna"}, info: &relaycommon.RelayInfo{}},
		{name: "blank metadata and zero token", request: &dto.ClaudeRequest{Model: "gpt-5.6-luna", Metadata: json.RawMessage(`{"user_id":"  "}`)}, info: &relaycommon.RelayInfo{}},
		{name: "negative token", request: &dto.ClaudeRequest{Model: "gpt-5.6-luna"}, info: &relaycommon.RelayInfo{TokenId: -1}},
		{name: "missing model", request: &dto.ClaudeRequest{Metadata: json.RawMessage(`{"user_id":"session-123"}`)}, info: &relaycommon.RelayInfo{TokenId: 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, ok := getClaudeResponsesPromptCacheKey(nil, tt.request, tt.info)

			assert.False(t, ok)
			assert.Empty(t, key)
			if tt.info != nil {
				assert.Equal(t, "none", tt.info.PromptCacheKeySource)
			}
		})
	}
}

func TestClaudeLunaResponsesCompatibilityPreservesRequestControls(t *testing.T) {
	maxTokens := uint(32000)
	stream := true
	temperature := 0.2
	topP := 0.8
	claudeRequest := &dto.ClaudeRequest{
		Model:       "gpt-5.6-luna",
		System:      "You are the canvas agent.",
		Messages:    []dto.ClaudeMessage{{Role: "user", Content: "Build a scene."}},
		MaxTokens:   &maxTokens,
		Stream:      &stream,
		Temperature: &temperature,
		TopP:        &topP,
		Tools: []any{
			dto.Tool{
				Name:        "canvas_read",
				Description: "Read the current canvas",
				InputSchema: map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
			dto.ClaudeWebSearchTool{
				Type:    "web_search_20250305",
				Name:    "web_search",
				MaxUses: 3,
				UserLocation: &dto.ClaudeWebSearchUserLocation{
					Type:     "approximate",
					Timezone: "Asia/Singapore",
					Country:  "SG",
					City:     "Singapore",
				},
			},
		},
		ToolChoice: dto.ClaudeToolChoice{
			Type:                   "tool",
			Name:                   "canvas_read",
			DisableParallelToolUse: true,
		},
		Metadata:     json.RawMessage(`{"user_id":"canvas-agent"}`),
		OutputConfig: json.RawMessage(`{"effort":"max"}`),
	}

	chatRequest, err := service.ClaudeToOpenAIResponsesRequest(*claudeRequest, &relaycommon.RelayInfo{})
	require.NoError(t, err)
	applyClaudeResponsesEffort(claudeRequest, chatRequest)
	chatJSON, err := common.Marshal(chatRequest)
	require.NoError(t, err)
	assert.NotContains(t, string(chatJSON), "web_search")
	assert.NotContains(t, string(chatJSON), "max_tool_calls")
	chatJSON, err = materializeResponsesOnlyRequestFields(chatJSON, chatRequest)
	require.NoError(t, err)
	chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, dto.ChannelOtherSettings{}, false)
	require.NoError(t, err)
	assert.Contains(t, string(chatJSON), "web_search")
	assert.Contains(t, string(chatJSON), "max_tool_calls")
	roundTrippedChatRequest, err := parseChatRequestForResponses(chatJSON)
	require.NoError(t, err)

	responsesResult, err := service.ConvertRequestVia(
		nil,
		&relaycommon.RelayInfo{},
		roundTrippedChatRequest,
		types.RelayFormatOpenAI,
		types.RelayFormatOpenAIResponses,
	)
	require.NoError(t, err)
	responsesRequest, ok := responsesResult.Value.(*dto.OpenAIResponsesRequest)
	require.True(t, ok)

	assert.Equal(t, "gpt-5.6-luna", responsesRequest.Model)
	require.NotNil(t, responsesRequest.MaxOutputTokens)
	require.NotNil(t, responsesRequest.Stream)
	require.NotNil(t, responsesRequest.Temperature)
	require.NotNil(t, responsesRequest.TopP)
	assert.Equal(t, maxTokens, *responsesRequest.MaxOutputTokens)
	assert.Equal(t, stream, *responsesRequest.Stream)
	assert.Equal(t, temperature, *responsesRequest.Temperature)
	assert.Equal(t, topP, *responsesRequest.TopP)
	assert.JSONEq(t, `"You are the canvas agent."`, string(responsesRequest.Instructions))
	assert.JSONEq(t, `[{"role":"user","content":"Build a scene."}]`, string(responsesRequest.Input))
	assert.JSONEq(t, `[{
		"type":"function",
		"name":"canvas_read",
		"description":"Read the current canvas",
		"parameters":{"type":"object","properties":{}}
	},{
		"type":"web_search",
		"user_location":{
			"type":"approximate",
			"timezone":"Asia/Singapore",
			"country":"SG",
			"city":"Singapore"
		}
	}]`, string(responsesRequest.Tools))
	require.NotNil(t, responsesRequest.MaxToolCalls)
	assert.Equal(t, uint(3), *responsesRequest.MaxToolCalls)
	assert.JSONEq(t, `{"type":"function","name":"canvas_read"}`, string(responsesRequest.ToolChoice))
	assert.JSONEq(t, `false`, string(responsesRequest.ParallelToolCalls))
	assert.JSONEq(t, `{"user_id":"canvas-agent"}`, string(responsesRequest.Metadata))
	require.NotNil(t, responsesRequest.Reasoning)
	assert.Equal(t, "max", responsesRequest.Reasoning.Effort)
}

func TestClaudeLunaPromptCacheSystemBreakpointSurvivesResponsesConversion(t *testing.T) {
	system := dto.ClaudeMediaMessage{
		Type:         dto.ContentTypeText,
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}
	system.SetText("stable system prompt")
	secondSystem := dto.ClaudeMediaMessage{
		Type:         dto.ContentTypeText,
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}
	secondSystem.SetText("stable tool instructions")
	claudeRequest := dto.ClaudeRequest{
		Model:    "gpt-5.6-luna",
		System:   []dto.ClaudeMediaMessage{system, secondSystem},
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "variable question"}},
		Metadata: json.RawMessage(`{"user_id":"session-123"}`),
		Tools: []map[string]any{{
			"name":          "lookup",
			"description":   "Look up a value",
			"input_schema":  map[string]any{"type": "object"},
			"cache_control": map[string]any{"type": "ephemeral"},
		}},
	}
	info := &relaycommon.RelayInfo{OriginModelName: "gpt-5.6-luna", TokenId: 41}

	chatRequest, err := service.ClaudeToOpenAIResponsesRequest(claudeRequest, info)
	require.NoError(t, err)
	cacheKey, ok := getClaudeResponsesPromptCacheKey(nil, &claudeRequest, info)
	require.True(t, ok)
	chatRequest.PromptCacheKey = cacheKey
	chatJSON, err := common.Marshal(chatRequest)
	require.NoError(t, err)
	chatJSON, err = materializeResponsesOnlyRequestFields(chatJSON, chatRequest)
	require.NoError(t, err)
	chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, dto.ChannelOtherSettings{}, false)
	require.NoError(t, err)
	roundTrippedChatRequest, err := parseChatRequestForResponses(chatJSON)
	require.NoError(t, err)

	responsesResult, err := service.ConvertRequestVia(
		nil,
		info,
		roundTrippedChatRequest,
		types.RelayFormatOpenAI,
		types.RelayFormatOpenAIResponses,
	)
	require.NoError(t, err)
	responsesRequest, ok := responsesResult.Value.(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	encoded, err := common.Marshal(responsesRequest)
	require.NoError(t, err)

	assert.NotEmpty(t, gjson.GetBytes(encoded, "prompt_cache_key").String())
	assert.Equal(t, "developer", gjson.GetBytes(encoded, "input.0.role").String())
	assert.Equal(t, "stable system prompt", gjson.GetBytes(encoded, "input.0.content.0.text").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.0.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "input.0.content.1.prompt_cache_breakpoint.mode").String())
	assert.Equal(t, "explicit", gjson.GetBytes(encoded, "prompt_cache_options.mode").String())
	assert.False(t, gjson.GetBytes(encoded, "tools.0.cache_control").Exists())
	assert.False(t, gjson.GetBytes(encoded, "tools.0.prompt_cache_breakpoint").Exists())
	assert.False(t, gjson.GetBytes(encoded, "instructions").Exists())
	assert.Equal(t, 2, info.PromptCacheBreakpointCount)
}

func TestClaudeOlderModelCacheControlKeepsResponsesInstructions(t *testing.T) {
	system := dto.ClaudeMediaMessage{
		Type:         dto.ContentTypeText,
		CacheControl: json.RawMessage(`{"type":"ephemeral"}`),
	}
	system.SetText("stable system prompt")
	claudeRequest := dto.ClaudeRequest{
		Model:    "gpt-5.5",
		System:   []dto.ClaudeMediaMessage{system},
		Messages: []dto.ClaudeMessage{{Role: "user", Content: "variable question"}},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName:            "gpt-5.5",
		TokenId:                    41,
		PromptCacheBreakpointCount: 99,
	}

	chatRequest, err := service.ClaudeToOpenAIResponsesRequest(claudeRequest, info)
	require.NoError(t, err)
	chatJSON, err := common.Marshal(chatRequest)
	require.NoError(t, err)
	chatJSON, err = materializeResponsesOnlyRequestFields(chatJSON, chatRequest)
	require.NoError(t, err)
	chatJSON, err = relaycommon.RemoveDisabledFields(chatJSON, dto.ChannelOtherSettings{}, false)
	require.NoError(t, err)
	roundTrippedChatRequest, err := parseChatRequestForResponses(chatJSON)
	require.NoError(t, err)

	responsesResult, err := service.ConvertRequestVia(
		nil,
		info,
		roundTrippedChatRequest,
		types.RelayFormatOpenAI,
		types.RelayFormatOpenAIResponses,
	)
	require.NoError(t, err)
	responsesRequest, ok := responsesResult.Value.(*dto.OpenAIResponsesRequest)
	require.True(t, ok)
	encoded, err := common.Marshal(responsesRequest)
	require.NoError(t, err)

	assert.Equal(t, "stable system prompt", gjson.GetBytes(encoded, "instructions").String())
	assert.Equal(t, "user", gjson.GetBytes(encoded, "input.0.role").String())
	assert.False(t, gjson.GetBytes(encoded, "prompt_cache_options").Exists())
	assert.Zero(t, info.PromptCacheBreakpointCount)
}

func TestClaudeLunaResponsesCompatibilityRejectsMalformedTools(t *testing.T) {
	_, err := service.ConvertRequest(
		nil,
		&relaycommon.RelayInfo{},
		types.RelayFormatOpenAI,
		&dto.ClaudeRequest{
			Model: "gpt-5.6-luna",
			Tools: make(chan int),
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "tools")
}

func TestClaudeLunaResponsesCompatibilityPreservesNativeWebSearchChoice(t *testing.T) {
	chatRequest, err := service.ClaudeToOpenAIResponsesRequest(
		dto.ClaudeRequest{
			Model: "gpt-5.6-luna",
			Tools: []any{dto.ClaudeWebSearchTool{
				Type: "web_search_20250305",
				Name: "web_search",
			}},
			ToolChoice: dto.ClaudeToolChoice{
				Type: "tool",
				Name: "web_search",
			},
		},
		&relaycommon.RelayInfo{},
	)
	require.NoError(t, err)
	assert.Nil(t, chatRequest.ToolChoice)
	assert.JSONEq(t, `{"type":"web_search"}`, string(chatRequest.ResponsesToolChoice))
}

func TestClaudeLunaResponsesCompatibilityHonorsToolDisableOverride(t *testing.T) {
	chatRequest, err := service.ClaudeToOpenAIResponsesRequest(
		dto.ClaudeRequest{
			Model: "gpt-5.6-luna",
			Tools: []any{dto.ClaudeWebSearchTool{
				Type:    "web_search_20250305",
				Name:    "web_search",
				MaxUses: 3,
			}},
			ToolChoice: dto.ClaudeToolChoice{Type: "tool", Name: "web_search"},
		},
		&relaycommon.RelayInfo{},
	)
	require.NoError(t, err)
	chatJSON, err := common.Marshal(chatRequest)
	require.NoError(t, err)
	chatJSON, err = materializeResponsesOnlyRequestFields(chatJSON, chatRequest)
	require.NoError(t, err)
	chatJSON, err = relaycommon.ApplyParamOverride(chatJSON, map[string]any{
		"tools":       []any{},
		"tool_choice": "none",
	}, nil)
	require.NoError(t, err)
	overridden, err := parseChatRequestForResponses(chatJSON)
	require.NoError(t, err)
	assert.Empty(t, overridden.ResponsesTools)
	assert.Empty(t, overridden.ResponsesToolChoice)
	assert.Equal(t, "none", overridden.ToolChoice)
}

func TestClaudeChatCompatibilityRejectsServerWebSearch(t *testing.T) {
	_, err := service.ConvertRequest(
		nil,
		&relaycommon.RelayInfo{},
		types.RelayFormatOpenAI,
		&dto.ClaudeRequest{
			Model: "other-openai-chat-model",
			Tools: []any{dto.ClaudeWebSearchTool{
				Type: "web_search_20250305",
				Name: "web_search",
			}},
		},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "requires an OpenAI Responses route")
}

func TestClaudeLunaResponsesCompatibilityRejectsUnknownServerTool(t *testing.T) {
	_, err := service.ConvertRequest(
		nil,
		&relaycommon.RelayInfo{},
		types.RelayFormatOpenAI,
		&dto.ClaudeRequest{
			Model: "gpt-5.6-luna",
			Tools: []any{map[string]any{
				"type": "computer_20250124",
				"name": "computer",
			}},
		},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported Claude server tool type")
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

func TestShouldClaudeRequestUseResponsesForcesLunaNativeWebSearchWithoutEffort(t *testing.T) {
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
		Model: "provider-luna-upstream-alias",
		Tools: []any{dto.ClaudeWebSearchTool{
			Type: "web_search_20250305",
			Name: "web_search",
		}},
	}

	assert.True(t, shouldClaudeRequestUseResponses(info, request))
	assert.True(t, shouldRouteClaudeRequestViaResponses(info, request, true, true))
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
