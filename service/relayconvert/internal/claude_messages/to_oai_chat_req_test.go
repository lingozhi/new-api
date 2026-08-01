package claudemessages

import (
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeMessagesRequestToOpenAIResponsesChatPreservesMixedAssistantContent(t *testing.T) {
	text := dto.ClaudeMediaMessage{Type: "text"}
	text.SetText("I will inspect the canvas before editing it.")
	assistant := dto.ClaudeMessage{Role: "assistant"}
	assistant.SetContent([]dto.ClaudeMediaMessage{
		text,
		{
			Type:  "tool_use",
			Id:    "toolu_1",
			Name:  "canvas_read",
			Input: map[string]any{"view": "summary"},
		},
	})

	converted, err := ClaudeMessagesRequestToOpenAIResponsesChat(dto.ClaudeRequest{
		Model:    "gpt-5.6-luna",
		Messages: []dto.ClaudeMessage{assistant},
	}, &relaycommon.RelayInfo{})

	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	assert.Equal(t, "I will inspect the canvas before editing it.", converted.Messages[0].StringContent())
	toolCalls := converted.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "toolu_1", toolCalls[0].ID)
	assert.Equal(t, "canvas_read", toolCalls[0].Function.Name)
	assert.JSONEq(t, `{"view":"summary"}`, toolCalls[0].Function.Arguments)
}
