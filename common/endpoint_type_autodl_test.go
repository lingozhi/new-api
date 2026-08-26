package common

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutoDLEndpointTypesFollowWorkflowModel(t *testing.T) {
	assert.Equal(t,
		[]constant.EndpointType{constant.EndpointTypeOpenAIVideo},
		GetEndpointTypesByChannelType(constant.ChannelTypeAutoDL, constant.AutoDLModelMiniMaxH3),
	)
	assert.Equal(t,
		[]constant.EndpointType{constant.EndpointTypeAudioSpeech},
		GetEndpointTypesByChannelType(constant.ChannelTypeAutoDL, constant.AutoDLModelIndexTTS2),
	)
	assert.Empty(t, GetEndpointTypesByChannelType(constant.ChannelTypeAutoDL, "unsupported-workflow"))
}

func TestAudioSpeechEndpointHasOfficialDefaultRoute(t *testing.T) {
	info, ok := GetDefaultEndpointInfo(constant.EndpointTypeAudioSpeech)
	require.True(t, ok)
	assert.Equal(t, EndpointInfo{Path: constant.AutoDLAudioSpeechPath, Method: http.MethodPost}, info)
}
