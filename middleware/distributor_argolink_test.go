package middleware

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
)

func TestArgolinkSeedanceOnlyUsesNativeVideoRoute(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI}
	assert.True(t, channelSupportsRequestPath(channel, "/v1/videos/generations", constant.ArgolinkSeedance25Model))
	assert.True(t, channelSupportsRequestPath(channel, "/v1/videos/task_public", constant.ArgolinkSeedance25Model))
	assert.False(t, channelSupportsRequestPath(channel, "/v1/chat/completions", constant.ArgolinkSeedance25Model))
	assert.False(t, channelSupportsRequestPath(channel, "/v1/images/generations", constant.ArgolinkSeedance25Model))
	assert.False(t, channelSupportsRequestPath(channel, "/v1/videos/task_public/remix", constant.ArgolinkSeedance25Model))
}
