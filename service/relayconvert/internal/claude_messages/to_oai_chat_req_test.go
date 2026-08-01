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
	content := converted.Messages[0].ParseContent()
	require.Len(t, content, 1)
	assert.Equal(t, dto.ContentTypeText, content[0].Type)
	assert.Equal(t, "I will inspect the canvas before editing it.", content[0].Text)
	toolCalls := converted.Messages[0].ParseToolCalls()
	require.Len(t, toolCalls, 1)
	assert.Equal(t, "toolu_1", toolCalls[0].ID)
	assert.Equal(t, "canvas_read", toolCalls[0].Function.Name)
	assert.JSONEq(t, `{"view":"summary"}`, toolCalls[0].Function.Arguments)
}

func TestClaudeMessagesRequestToOpenAIResponsesChatPreservesMultimodalToolResult(t *testing.T) {
	text := dto.ClaudeMediaMessage{Type: "text"}
	text.SetText("The preview contains the latest canvas result.")
	toolResult := dto.ClaudeMediaMessage{
		Type:      "tool_result",
		ToolUseId: "toolu_read_1",
		Content: []dto.ClaudeMediaMessage{
			text,
			{
				Type: "image",
				Source: &dto.ClaudeMessageSource{
					Type:      "base64",
					MediaType: "image/jpeg",
					Data:      "cGl4ZWxz",
				},
			},
		},
	}
	user := dto.ClaudeMessage{Role: "user"}
	user.SetContent([]dto.ClaudeMediaMessage{toolResult})

	converted, err := ClaudeMessagesRequestToOpenAIResponsesChat(dto.ClaudeRequest{
		Model:    "gpt-5.6-luna",
		Messages: []dto.ClaudeMessage{user},
	}, &relaycommon.RelayInfo{})

	require.NoError(t, err)
	require.Len(t, converted.Messages, 1)
	message := converted.Messages[0]
	assert.Equal(t, "tool", message.Role)
	assert.Equal(t, "toolu_read_1", message.ToolCallId)
	content := message.ParseContent()
	require.Len(t, content, 2)
	assert.Equal(t, dto.ContentTypeText, content[0].Type)
	assert.Equal(t, "The preview contains the latest canvas result.", content[0].Text)
	assert.Equal(t, dto.ContentTypeImageURL, content[1].Type)
	assert.Equal(t, "data:image/jpeg;base64,cGl4ZWxz", content[1].GetImageMedia().Url)
}
