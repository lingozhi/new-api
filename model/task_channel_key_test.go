package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/stretchr/testify/assert"
)

func TestInitTaskPersistsSelectedChannelKeyIdentity(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		UserId:     42,
		UsingGroup: "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAutoDL,
			ChannelId:            7,
			ChannelIsMultiKey:    true,
			ChannelMultiKeyIndex: 1,
			ApiKey:               "selected-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	task := InitTask(constant.TaskPlatform("60"), relayInfo)

	assert.Equal(t, 1, task.PrivateData.ChannelMultiKeyIndex)
	assert.Equal(t, common.Sha256([]byte("selected-key")), task.PrivateData.ChannelKeyHash)
	assert.Empty(t, task.PrivateData.Key)
}

func TestInitTaskIgnoresStaleMultiKeyIndexForSingleKeyChannel(t *testing.T) {
	relayInfo := &relaycommon.RelayInfo{
		UserId:     42,
		UsingGroup: "default",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:          constant.ChannelTypeAutoDL,
			ChannelId:            7,
			ChannelIsMultiKey:    false,
			ChannelMultiKeyIndex: 9,
			ApiKey:               "single-key",
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}

	task := InitTask(constant.TaskPlatform("60"), relayInfo)

	assert.Zero(t, task.PrivateData.ChannelMultiKeyIndex)
	assert.Equal(t, common.Sha256([]byte("single-key")), task.PrivateData.ChannelKeyHash)
}
