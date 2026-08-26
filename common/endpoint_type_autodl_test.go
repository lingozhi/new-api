package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
)

func TestAutoDLEndpointTypesFollowWorkflowModel(t *testing.T) {
	assert.Equal(t,
		[]constant.EndpointType{constant.EndpointTypeOpenAIVideo},
		GetEndpointTypesByChannelType(constant.ChannelTypeAutoDL, constant.AutoDLModelMiniMaxH3),
	)
	assert.Equal(t,
		[]constant.EndpointType{constant.EndpointTypeOpenAI},
		GetEndpointTypesByChannelType(constant.ChannelTypeAutoDL, constant.AutoDLModelIndexTTS2),
	)
	assert.Empty(t, GetEndpointTypesByChannelType(constant.ChannelTypeAutoDL, "unsupported-workflow"))
}
