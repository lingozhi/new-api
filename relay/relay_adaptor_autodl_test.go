package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaskAdaptorRegistersAutoDL(t *testing.T) {
	platform := constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeAutoDL))
	adaptor := GetTaskAdaptor(platform)
	require.NotNil(t, adaptor)
	assert.Equal(t, "autodl", adaptor.GetChannelName())
	assert.Equal(t, []string{constant.AutoDLModelMiniMaxH3, constant.AutoDLModelIndexTTS2}, adaptor.GetModelList())
}
