package oairesponses

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesResponseToChatCompletionsPreservesTextAndToolCalls(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		ID:        "resp_1",
		CreatedAt: 123,
		Model:     "gpt-test",
		Status:    []byte(`"completed"`),
		Output: []dto.ResponsesOutput{
			{
				Type: responsesOutputTypeMessage,
				Role: "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: "I will call a tool."},
				},
			},
			{
				Type:      responsesOutputTypeFunctionCall,
				ID:        "fc_1",
				CallId:    "call_1",
				Name:      "lookup",
				Arguments: []byte(`{"q":"x"}`),
			},
		},
		Usage: &dto.Usage{InputTokens: 3, OutputTokens: 4, TotalTokens: 7},
	}

	chat, usage, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl_1")
	require.NoError(t, err)
	require.NotNil(t, usage)

	require.Len(t, chat.Choices, 1)
	assert.Equal(t, "tool_calls", chat.Choices[0].FinishReason)
	assert.Equal(t, "I will call a tool.", chat.Choices[0].Message.StringContent())
	toolCalls := chat.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "call_1", toolCalls[0].ID)
	assert.Equal(t, "lookup", toolCalls[0].Function.Name)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Equal(t, 7, usage.TotalTokens)
}

func TestResponsesResponseToChatCompletionsPreservesReasoningSummary(t *testing.T) {
	resp := &dto.OpenAIResponsesResponse{
		ID:     "resp_1",
		Model:  "gpt-test",
		Status: []byte(`"completed"`),
		Output: []dto.ResponsesOutput{
			{
				Type: responsesOutputTypeReasoning,
				Content: []dto.ResponsesOutputContent{
					{Type: "summary_text", Text: "first summary"},
					{Type: "summary_text", Text: "\n\nsecond summary"},
				},
			},
			{
				Type: responsesOutputTypeMessage,
				Role: "assistant",
				Content: []dto.ResponsesOutputContent{
					{Type: "output_text", Text: "final"},
				},
			},
		},
	}

	chat, _, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl_1")
	require.NoError(t, err)
	assert.Equal(t, "first summary\n\nsecond summary", chat.Choices[0].Message.GetReasoningContent())
	assert.Equal(t, "final", chat.Choices[0].Message.StringContent())
}

func TestResponsesFinishReasonFromIncompleteStatus(t *testing.T) {
	tests := []struct {
		name   string
		reason string
		want   string
	}{
		{name: "max output", reason: responsesIncompleteReasonMaxTokens, want: "length"},
		{name: "content filter", reason: responsesIncompleteReasonContentFilter, want: "content_filter"},
		{name: "unknown", reason: "other", want: "length"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResponsesFinishReasonFromStatus(&dto.OpenAIResponsesResponse{
				Status:            []byte(`"incomplete"`),
				IncompleteDetails: &dto.IncompleteDetails{Reason: tt.reason},
			})
			require.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestResponsesStreamEventToChatChunksUsesOutputIndexForToolArguments(t *testing.T) {
	state := newTestResponsesStreamState()
	outputIndex := 1

	var chunks []dto.ChatCompletionsStreamResponse
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{Type: responsesEventCreated})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{Type: responsesEventOutputTextDelta, Delta: "text before tool"})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		Delta:       `{"cmd":"ls"}`,
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_1",
			CallId: "call_1",
			Name:   "exec",
		},
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type: responsesEventCompleted,
		Response: &dto.OpenAIResponsesResponse{
			Status: []byte(`"completed"`),
			Usage:  &dto.Usage{InputTokens: 1, OutputTokens: 2, TotalTokens: 3},
		},
	})...)

	require.Len(t, chunks, 4)
	assert.Equal(t, "assistant", chunks[0].Choices[0].Delta.Role)
	assert.Equal(t, "text before tool", chunks[1].Choices[0].Delta.GetContentString())
	tool := chunks[2].Choices[0].Delta.ToolCalls[0]
	require.NotNil(t, tool.Index)
	assert.Equal(t, 0, *tool.Index)
	assert.Equal(t, "call_1", tool.ID)
	assert.Equal(t, "exec", tool.Function.Name)
	assert.Equal(t, `{"cmd":"ls"}`, tool.Function.Arguments)
	require.NotNil(t, chunks[3].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *chunks[3].Choices[0].FinishReason)
	assert.Equal(t, 3, state.Usage.TotalTokens)
}

func TestResponsesStreamEventToChatChunksDoesNotDuplicatePendingArgsWithOutputIndexAndItemID(t *testing.T) {
	state := newTestResponsesStreamState()
	outputIndex := 1

	var chunks []dto.ChatCompletionsStreamResponse
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{Type: responsesEventCreated})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_1",
		Delta:       `{"q":"x"}`,
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		ItemID:      "fc_1",
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_1",
			CallId: "call_1",
			Name:   "lookup",
		},
	})...)

	require.Len(t, chunks, 2)
	tool := chunks[1].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, "call_1", tool.ID)
	assert.Equal(t, "lookup", tool.Function.Name)
	assert.Equal(t, `{"q":"x"}`, tool.Function.Arguments)
	assert.Empty(t, state.pendingArgsByOutputIndex)
	assert.Empty(t, state.pendingArgsByItemID)
}

func TestResponsesStreamEventToChatChunksDrainsItemOnlyPendingArgsWhenOutputIndexArrives(t *testing.T) {
	state := newTestResponsesStreamState()
	outputIndex := 1

	var chunks []dto.ChatCompletionsStreamResponse
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{Type: responsesEventCreated})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:   responsesEventFunctionArgsDelta,
		ItemID: "fc_1",
		Delta:  `{"q":"x"}`,
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		ItemID:      "fc_1",
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			CallId: "call_1",
			Name:   "lookup",
		},
	})...)

	require.Len(t, chunks, 2)
	tool := chunks[1].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, "call_1", tool.ID)
	assert.Equal(t, "lookup", tool.Function.Name)
	assert.Equal(t, `{"q":"x"}`, tool.Function.Arguments)
	assert.Empty(t, state.pendingArgsByOutputIndex)
	assert.Empty(t, state.pendingArgsByItemID)
}

func TestResponsesStreamEventToChatChunksCustomToolAndReasoning(t *testing.T) {
	state := newTestResponsesStreamState()
	outputIndex := 0

	chunks := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:  responsesEventReasoningTextDelta,
		Delta: "thinking",
	})
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeCustomToolCall,
			ID:     "ct_1",
			CallId: "call_custom",
			Name:   "apply_patch",
		},
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventCustomToolInputDelta,
		OutputIndex: &outputIndex,
		Delta:       "patch body",
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type: responsesEventIncomplete,
		Response: &dto.OpenAIResponsesResponse{
			IncompleteDetails: &dto.IncompleteDetails{Reason: responsesIncompleteReasonContentFilter},
		},
	})...)

	require.Len(t, chunks, 5)
	assert.Equal(t, "thinking", chunks[1].Choices[0].Delta.GetReasoningContent())
	assert.Equal(t, "apply_patch", chunks[2].Choices[0].Delta.ToolCalls[0].Function.Name)
	assert.Equal(t, "patch body", chunks[3].Choices[0].Delta.ToolCalls[0].Function.Arguments)
	require.NotNil(t, chunks[4].Choices[0].FinishReason)
	assert.Equal(t, "content_filter", *chunks[4].Choices[0].FinishReason)
}

func TestResponsesStreamEventToChatChunksUsesTerminalDoneOutput(t *testing.T) {
	state := newTestResponsesStreamState()
	chunks := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type: responsesEventDone,
		Response: &dto.OpenAIResponsesResponse{
			Status: []byte(`"completed"`),
			Output: []dto.ResponsesOutput{
				{
					Type: responsesOutputTypeMessage,
					Role: "assistant",
					Content: []dto.ResponsesOutputContent{
						{Type: "output_text", Text: "terminal text"},
					},
				},
				{
					Type:      responsesOutputTypeFunctionCall,
					ID:        "fc_1",
					CallId:    "call_1",
					Name:      "lookup",
					Arguments: []byte(`{"q":"x"}`),
				},
			},
		},
	})

	require.Len(t, chunks, 4)
	assert.Equal(t, "assistant", chunks[0].Choices[0].Delta.Role)
	assert.Equal(t, "terminal text", chunks[1].Choices[0].Delta.GetContentString())
	tool := chunks[2].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, "lookup", tool.Function.Name)
	assert.Equal(t, `{"q":"x"}`, tool.Function.Arguments)
	require.NotNil(t, chunks[3].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *chunks[3].Choices[0].FinishReason)
}

func TestResponsesStreamEventToChatChunksDoesNotReplayToolFromTerminalOutput(t *testing.T) {
	state := newTestResponsesStreamState()
	outputIndex := 0

	var chunks []dto.ChatCompletionsStreamResponse
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_1",
			CallId: "call_1",
			Name:   "Skill",
		},
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_1",
		Delta:       `{"skill":"opone-canvas-agent:drama-director"}`,
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type: responsesEventCompleted,
		Response: &dto.OpenAIResponsesResponse{
			Status: []byte(`"completed"`),
			Output: []dto.ResponsesOutput{
				{
					Type:      responsesOutputTypeFunctionCall,
					ID:        "fc_1",
					CallId:    "call_1",
					Name:      "Skill",
					Arguments: []byte(`{"skill":"opone-canvas-agent:drama-director"}`),
				},
			},
		},
	})...)

	var toolNameChunks int
	var arguments strings.Builder
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}
		for _, toolCall := range chunk.Choices[0].Delta.ToolCalls {
			if toolCall.Function.Name != "" {
				toolNameChunks++
			}
			arguments.WriteString(toolCall.Function.Arguments)
		}
	}

	assert.Equal(t, 1, toolNameChunks)
	assert.Equal(t, `{"skill":"opone-canvas-agent:drama-director"}`, arguments.String())
}

func TestResponsesStreamEventToChatChunksPreservesDistinctParallelTools(t *testing.T) {
	state := newTestResponsesStreamState()
	var chunks []dto.ChatCompletionsStreamResponse
	outputs := make([]dto.ResponsesOutput, 0, 2)
	argumentsByIndex := []string{`{"view":"canvas"}`, `{"kind":"image"}`}

	for index, name := range []string{"canvas_read", "asset_list"} {
		itemID := fmt.Sprintf("fc_%d", index)
		callID := fmt.Sprintf("call_%d", index)
		chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: &index,
			Item: &dto.ResponsesOutput{
				Type:   responsesOutputTypeFunctionCall,
				ID:     itemID,
				CallId: callID,
				Name:   name,
			},
		})...)
		outputs = append(outputs, dto.ResponsesOutput{
			Type:      responsesOutputTypeFunctionCall,
			ID:        itemID,
			CallId:    callID,
			Name:      name,
			Arguments: []byte(argumentsByIndex[index]),
		})
	}
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type: responsesEventCompleted,
		Response: &dto.OpenAIResponsesResponse{
			Status: []byte(`"completed"`),
			Output: outputs,
		},
	})...)

	type observedTool struct {
		id        string
		name      string
		arguments string
	}
	observedByIndex := make(map[int]observedTool, 2)
	toolStarts := 0
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}
		for _, toolCall := range chunk.Choices[0].Delta.ToolCalls {
			require.NotNil(t, toolCall.Index)
			observed := observedByIndex[*toolCall.Index]
			if observed.id == "" {
				observed.id = toolCall.ID
			} else {
				assert.Equal(t, observed.id, toolCall.ID)
			}
			if toolCall.Function.Name != "" {
				toolStarts++
				observed.name = toolCall.Function.Name
			}
			observed.arguments += toolCall.Function.Arguments
			observedByIndex[*toolCall.Index] = observed
		}
	}

	assert.Equal(t, 2, toolStarts)
	assert.Equal(t, map[int]observedTool{
		0: {id: "call_0", name: "canvas_read", arguments: `{"view":"canvas"}`},
		1: {id: "call_1", name: "asset_list", arguments: `{"kind":"image"}`},
	}, observedByIndex)
}

func TestResponsesStreamEventToChatChunksBuffersAndTransformsMatchedToolArguments(t *testing.T) {
	state := newTestResponsesStreamState()
	state.ToolArgumentsTransform = func(toolName string, arguments string) (string, bool) {
		if toolName != "Read" {
			return arguments, false
		}
		return strings.Replace(arguments, `,"pages":""`, "", 1), true
	}
	outputIndex := 0

	var chunks []dto.ChatCompletionsStreamResponse
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:  responsesEventOutputTextDelta,
		Delta: "before tool",
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_read",
			CallId: "call_read",
			Name:   "Read",
		},
	})...)

	firstDelta := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
		Delta:       `{"file_path":"/tmp/sentinel.png"`,
	})
	secondDelta := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
		Delta:       `,"pages":""}`,
	})
	require.Empty(t, firstDelta)
	require.Empty(t, secondDelta)

	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDone,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
	})...)
	chunks = append(chunks, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type: responsesEventCompleted,
		Response: &dto.OpenAIResponsesResponse{
			Status: []byte(`"completed"`),
			Output: []dto.ResponsesOutput{
				{
					Type:      responsesOutputTypeFunctionCall,
					ID:        "fc_read",
					CallId:    "call_read",
					Name:      "Read",
					Arguments: []byte(`{"file_path":"/tmp/sentinel.png","pages":""}`),
				},
			},
		},
	})...)

	var text strings.Builder
	var arguments strings.Builder
	toolNameChunks := 0
	for _, chunk := range chunks {
		if len(chunk.Choices) == 0 {
			continue
		}
		text.WriteString(chunk.Choices[0].Delta.GetContentString())
		for _, toolCall := range chunk.Choices[0].Delta.ToolCalls {
			arguments.WriteString(toolCall.Function.Arguments)
			if toolCall.Function.Name != "" {
				toolNameChunks++
			}
		}
	}

	assert.Equal(t, "before tool", text.String())
	assert.Equal(t, 1, toolNameChunks)
	assert.Equal(t, `{"file_path":"/tmp/sentinel.png"}`, arguments.String())
}

func TestResponsesStreamEventToChatChunksBuffersArgumentsUntilLateToolNameArrives(t *testing.T) {
	state := newTestResponsesStreamState()
	state.ToolArgumentsTransform = func(toolName string, arguments string) (string, bool) {
		if toolName != "Read" {
			return arguments, false
		}
		return `{"safe":true}`, true
	}
	outputIndex := 0

	require.Empty(t, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
		Delta:       `{"file_path":"/tmp/sentinel.png"`,
	}))
	mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_read",
			CallId: "call_read",
		},
	})
	require.Empty(t, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
		Delta:       `,"pages":""}`,
	}))
	require.Empty(t, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDone,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
	}))

	chunks := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemDone,
		OutputIndex: &outputIndex,
		ItemID:      "fc_read",
		Item: &dto.ResponsesOutput{
			Type:      responsesOutputTypeFunctionCall,
			ID:        "fc_read",
			CallId:    "call_read",
			Name:      "Read",
			Arguments: []byte(`{"file_path":"/tmp/sentinel.png","pages":""}`),
		},
	})
	require.Len(t, chunks, 1)
	assert.Equal(t, "Read", chunks[0].Choices[0].Delta.ToolCalls[0].Function.Name)
	assert.Equal(t, `{"safe":true}`, chunks[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)
}

func TestResponsesStreamEventToChatChunksOnlyBuffersMatchedParallelTool(t *testing.T) {
	state := newTestResponsesStreamState()
	state.ToolArgumentsTransform = func(toolName string, arguments string) (string, bool) {
		if toolName != "Read" {
			return arguments, false
		}
		return `{"normalized":true}`, true
	}
	readIndex := 0
	lookupIndex := 1

	for index, item := range []dto.ResponsesOutput{
		{Type: responsesOutputTypeFunctionCall, ID: "fc_read", CallId: "call_read", Name: "Read"},
		{Type: responsesOutputTypeFunctionCall, ID: "fc_lookup", CallId: "call_lookup", Name: "lookup"},
	} {
		outputIndex := index
		mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
			Type:        responsesEventOutputItemAdded,
			OutputIndex: &outputIndex,
			Item:        &item,
		})
	}

	readDelta := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &readIndex,
		Delta:       `{"pages":""}`,
	})
	lookupDelta := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &lookupIndex,
		Delta:       `{"q":"x"}`,
	})
	require.Empty(t, readDelta)
	require.Len(t, lookupDelta, 1)
	assert.Equal(t, `{"q":"x"}`, lookupDelta[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)

	readDone := mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemDone,
		OutputIndex: &readIndex,
		Item: &dto.ResponsesOutput{
			Type:      responsesOutputTypeFunctionCall,
			ID:        "fc_read",
			CallId:    "call_read",
			Name:      "Read",
			Arguments: []byte(`{"pages":""}`),
		},
	})
	require.Len(t, readDone, 1)
	assert.Equal(t, `{"normalized":true}`, readDone[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)
}

func TestFinalizeResponsesToChatStreamTransformsMatchedToolWithoutDoneEvent(t *testing.T) {
	state := newTestResponsesStreamState()
	state.ToolArgumentsTransform = func(toolName string, arguments string) (string, bool) {
		if toolName != "Read" {
			return arguments, false
		}
		return `{"finalized":true}`, true
	}
	outputIndex := 0

	mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_read",
			CallId: "call_read",
			Name:   "Read",
		},
	})
	require.Empty(t, mustStreamChunks(t, state, &dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		Delta:       `{"pages":""}`,
	}))

	chunks := FinalizeResponsesToChatStream(state)
	require.Len(t, chunks, 2)
	assert.Equal(t, `{"finalized":true}`, chunks[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)
	require.NotNil(t, chunks[1].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *chunks[1].Choices[0].FinishReason)
}

func TestFinalizeResponsesToChatStreamFlushesPendingDeltaOnlyArguments(t *testing.T) {
	state := newTestResponsesStreamState()
	outputIndex := 2
	_, err := ResponsesStreamEventToChatChunks(&dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		Delta:       `{"pending":true}`,
	}, state)
	require.NoError(t, err)

	chunks := FinalizeResponsesToChatStream(state)
	require.Len(t, chunks, 3)
	tool := chunks[1].Choices[0].Delta.ToolCalls[0]
	assert.Equal(t, "call_output_2", tool.ID)
	assert.Equal(t, `{"pending":true}`, tool.Function.Arguments)
	require.NotNil(t, chunks[2].Choices[0].FinishReason)
	assert.Equal(t, "tool_calls", *chunks[2].Choices[0].FinishReason)
}

func TestResponsesStreamEventToChatChunksFailedEventReturnsError(t *testing.T) {
	_, err := ResponsesStreamEventToChatChunks(&dto.ResponsesStreamResponse{Type: responsesEventFailed}, newTestResponsesStreamState())
	require.Error(t, err)
}

func TestResponsesBufferedAccumulatorSupplementsEmptyTerminalOutput(t *testing.T) {
	acc := NewResponsesBufferedAccumulator()
	outputIndex := 1
	acc.ProcessEvent(&dto.ResponsesStreamResponse{Type: responsesEventOutputTextDelta, Delta: "buffered text"})
	acc.ProcessEvent(&dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_1",
			CallId: "call_1",
			Name:   "lookup",
		},
	})
	acc.ProcessEvent(&dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		Delta:       `{"q":"x"}`,
	})

	resp := &dto.OpenAIResponsesResponse{
		Status: []byte(`"completed"`),
		Model:  "gpt-test",
	}
	acc.SupplementResponseOutput(resp)

	chat, _, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl_1")
	require.NoError(t, err)
	assert.Equal(t, "buffered text", chat.Choices[0].Message.StringContent())
	toolCalls := chat.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
}

func TestResponsesBufferedAccumulatorDoesNotDuplicatePendingArgsWithOutputIndexAndItemID(t *testing.T) {
	acc := NewResponsesBufferedAccumulator()
	outputIndex := 1
	acc.ProcessEvent(&dto.ResponsesStreamResponse{
		Type:        responsesEventFunctionArgsDelta,
		OutputIndex: &outputIndex,
		ItemID:      "fc_1",
		Delta:       `{"q":"x"}`,
	})
	acc.ProcessEvent(&dto.ResponsesStreamResponse{
		Type:        responsesEventOutputItemAdded,
		OutputIndex: &outputIndex,
		ItemID:      "fc_1",
		Item: &dto.ResponsesOutput{
			Type:   responsesOutputTypeFunctionCall,
			ID:     "fc_1",
			CallId: "call_1",
			Name:   "lookup",
		},
	})

	resp := &dto.OpenAIResponsesResponse{
		Status: []byte(`"completed"`),
		Model:  "gpt-test",
	}
	acc.SupplementResponseOutput(resp)

	chat, _, err := ResponsesResponseToChatCompletionsResponse(resp, "chatcmpl_1")
	require.NoError(t, err)
	toolCalls := chat.Choices[0].Message.ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, `{"q":"x"}`, toolCalls[0].Function.Arguments)
	assert.Empty(t, acc.pendingByOutputIndex)
	assert.Empty(t, acc.pendingByItemID)
}

func newTestResponsesStreamState() *ResponsesToChatStreamState {
	state := NewResponsesToChatStreamState("gpt-test", false)
	state.ID = "chatcmpl_test"
	state.Created = 123
	return state
}

func mustStreamChunks(t *testing.T, state *ResponsesToChatStreamState, event *dto.ResponsesStreamResponse) []dto.ChatCompletionsStreamResponse {
	t.Helper()
	chunks, err := ResponsesStreamEventToChatChunks(event, state)
	require.NoError(t, err)
	return chunks
}
