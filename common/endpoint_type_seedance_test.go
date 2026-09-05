package common

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSeedanceCatalogEndpoints(t *testing.T) {
	for _, name := range []string{constant.ArgolinkSeedance20Model, constant.ArgolinkSeedance20FastModel, constant.ArgolinkSeedance25Model} {
		assert.Equal(t, []constant.EndpointType{constant.EndpointTypeSeedanceVideo}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, name))
	}
	endpoint, ok := GetDefaultEndpointInfo(constant.EndpointTypeSeedanceVideo)
	require.True(t, ok)
	assert.Equal(t, "/v1/videos/generations", endpoint.Path)
	assert.Equal(t, "POST", endpoint.Method)
	assert.Equal(t, []constant.EndpointType{constant.EndpointTypeOpenAI}, GetEndpointTypesByChannelType(constant.ChannelTypeOpenAI, "gpt-4o"))
}
