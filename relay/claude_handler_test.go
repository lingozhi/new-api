package relay

import (
	"encoding/json"
	"testing"

	"github.com/QuantumNous/new-api/dto"
	"github.com/stretchr/testify/assert"
)

func TestApplyClaudeResponsesEffortPreservesMax(t *testing.T) {
	claudeRequest := &dto.ClaudeRequest{
		OutputConfig: json.RawMessage(`{"effort":"max"}`),
	}
	openAIRequest := &dto.GeneralOpenAIRequest{}

	applyClaudeResponsesEffort(claudeRequest, openAIRequest)

	assert.Equal(t, "max", openAIRequest.ReasoningEffort)
}

func TestApplyClaudeResponsesEffortKeepsExistingValueWhenMissing(t *testing.T) {
	claudeRequest := &dto.ClaudeRequest{}
	openAIRequest := &dto.GeneralOpenAIRequest{
		ReasoningEffort: "high",
	}

	applyClaudeResponsesEffort(claudeRequest, openAIRequest)

	assert.Equal(t, "high", openAIRequest.ReasoningEffort)
}
