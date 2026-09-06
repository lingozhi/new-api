package claude

import (
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the wire shape captured from Agent SDK 0.3.243's own JSON rejection,
// not a fabricated successful MCP response or an application-level failure.
func rejectedSDKRequest(t *testing.T) *dto.ClaudeRequest {
	t.Helper()
	var request dto.ClaudeRequest
	require.NoError(t, common.UnmarshalJsonStr(`{
		"model":"deepseek-v4-flash-vision-exp","thinking":{"type":"adaptive"},"output_config":{"effort":"max"},"max_tokens":32000,"stream":true,
		"system":"original system", "tools":[{"name":"mcp__canvas__canvas_edit","input_schema":{"type":"object"}}],
		"messages":[
			{"role":"user","content":[{"type":"text","text":"原始需求保持完整"},{"type":"image","source":{"type":"url","url":"https://example.com/avatar.png"}}]},
			{"role":"assistant","content":[{"type":"thinking","thinking":"original reasoning","signature":"sig"},{"type":"tool_use","id":"bad","name":"mcp__canvas__canvas_edit","input":{"__unparsedToolInput":{"raw":"{\"ops\":[{\"op\":\"set_params\"}}","len":36}},"cache_control":{"type":"ephemeral"}},{"type":"tool_use","id":"good","name":"read","input":{"version":27}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"bad","is_error":true,"content":"<tool_use_error>InputValidationError: mcp__canvas__canvas_edit was called with input that could not be parsed as JSON.\nYou sent (first 200 of 36 bytes): ...\nCommon causes: unescaped backslashes, control characters or truncated output. Retry with valid JSON.</tool_use_error>\n\n<system-reminder>remain</system-reminder>","cache_control":{"type":"ephemeral"}},{"type":"tool_result","tool_use_id":"good","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"pixel-data"}}]}]}
		]
	}`, &request))
	return &request
}

func TestClaudeAdaptorProjectsRejectedSDKInputAsErrorText(t *testing.T) {
	request := rejectedSDKRequest(t)
	before, err := common.DeepCopy(request)
	require.NoError(t, err)
	adaptor := Adaptor{}
	converted, err := adaptor.ConvertClaudeRequest(nil, nil, request)
	require.NoError(t, err)
	require.Same(t, request, converted)
	calls := request.Messages[1].Content.([]any)
	results := request.Messages[2].Content.([]any)
	assert.Empty(t, calls[1].(map[string]any)["input"])
	errorText := results[0].(map[string]any)["content"].(string)
	assert.Contains(t, errorText, "No tool ran.")
	assert.Contains(t, errorText, "not a valid argument example")
	assert.NotContains(t, errorText, "__unparsedToolInput")
	originalCall := before.Messages[1].Content.([]any)[1].(map[string]any)
	raw := originalCall["input"].(map[string]any)["__unparsedToolInput"].(map[string]any)["raw"].(string)
	assert.Contains(t, errorText, raw)
	assert.Contains(t, errorText, before.Messages[2].Content.([]any)[0].(map[string]any)["content"])
	// Exactly two fields change. Everything else, including reasoning, media,
	// concurrent successful calls, cache controls, model and effort, is preserved.
	calls[1].(map[string]any)["input"] = originalCall["input"]
	results[0].(map[string]any)["content"] = before.Messages[2].Content.([]any)[0].(map[string]any)["content"]
	assert.Equal(t, before, request)
	normalizeSDKRejectedToolInputs(request)
	first, err := common.Marshal(request)
	require.NoError(t, err)
	normalizeSDKRejectedToolInputs(request)
	second, err := common.Marshal(request)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestClaudeAdaptorPreservesOtherToolInputsAndFailures(t *testing.T) {
	tests := []struct {
		name   string
		change func(call, result map[string]any)
	}{
		{"legal long arguments", func(call, result map[string]any) {
			call["input"] = map[string]any{"ops": []any{map[string]any{"prompt": strings.Repeat("原文 \"quote\" \\ path\n", 1000)}}}
		}},
		{"success is not parse rejection", func(call, result map[string]any) { result["is_error"] = false }},
		{"different result id", func(call, result map[string]any) { result["tool_use_id"] = "unrelated" }},
		{"business failure", func(call, result map[string]any) { result["content"] = "VERSION_CONFLICT" }},
		{"different tool in diagnostic", func(call, result map[string]any) { call["name"] = "another_tool" }},
		{"wrapper beside real args", func(call, result map[string]any) { call["input"].(map[string]any)["ops"] = []any{} }},
		{"not SDK wrapper", func(call, result map[string]any) {
			call["input"] = map[string]any{"raw": "not a diagnostic", "len": 16}
		}},
		{"wrong wrapper type", func(call, result map[string]any) { call["input"] = map[string]any{"__unparsedToolInput": "user data"} }},
		{"missing SDK length", func(call, result map[string]any) {
			delete(call["input"].(map[string]any)["__unparsedToolInput"].(map[string]any), "len")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := rejectedSDKRequest(t)
			tt.change(request.Messages[1].Content.([]any)[1].(map[string]any), request.Messages[2].Content.([]any)[0].(map[string]any))
			before, err := common.Marshal(request)
			require.NoError(t, err)
			_, err = (&Adaptor{}).ConvertClaudeRequest(nil, nil, request)
			require.NoError(t, err)
			after, err := common.Marshal(request)
			require.NoError(t, err)
			assert.Equal(t, before, after)
		})
	}
}
