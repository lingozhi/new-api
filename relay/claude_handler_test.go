package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
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
