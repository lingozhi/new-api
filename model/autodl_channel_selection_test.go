package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiniMaxV2SelectionUsesOnlyAutoDLWithoutMemoryCache(t *testing.T) {
	setupChannelSelectionTestDB(t)

	priority := int64(10)
	weight := uint(100)
	channels := []Channel{
		{Id: 601, Type: constant.ChannelTypeMiniMax, Key: "minimax-key", Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority, Models: "MiniMax-H3", Group: "default"},
		{Id: 602, Type: constant.ChannelTypeAutoDL, Key: "autodl-key", Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority, Models: "MiniMax-H3", Group: "default"},
	}
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "default", Model: "MiniMax-H3", ChannelId: 601, Enabled: true, Priority: &priority, Weight: weight},
		{Group: "default", Model: "MiniMax-H3", ChannelId: 602, Enabled: true, Priority: &priority, Weight: weight},
	}).Error)

	selected, err := GetChannelWithOptions("default", "MiniMax-H3", 0, ChannelSelectionOptions{
		RequestPath: "/v2/video_generation",
		Path:        "/v2/video_generation",
	})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 602, selected.Id)
}

func TestMiniMaxV2SelectionUsesOnlyAutoDLWithMemoryCache(t *testing.T) {
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	ClearChannelCacheForTest()
	t.Cleanup(func() {
		ClearChannelCacheForTest()
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	priority := int64(10)
	weight := uint(100)
	miniMax := &Channel{Id: 611, Type: constant.ChannelTypeMiniMax, Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority}
	autoDL := &Channel{Id: 612, Type: constant.ChannelTypeAutoDL, Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority}
	SetChannelCacheForTest(map[int]*Channel{611: miniMax, 612: autoDL}, map[string]map[string][]int{
		"default": {"MiniMax-H3": {611, 612}},
	})

	selected, err := GetRandomSatisfiedChannelWithOptions("default", "MiniMax-H3", 0, ChannelSelectionOptions{
		RequestPath: "/v2/video_generation",
		Path:        "/v2/video_generation",
	})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 612, selected.Id)
}

func TestNonMiniMaxRouteNeverSelectsAutoDLWithoutMemoryCache(t *testing.T) {
	setupChannelSelectionTestDB(t)

	priority := int64(10)
	weight := uint(100)
	channels := []Channel{
		{Id: 621, Type: constant.ChannelTypeAutoDL, Key: "autodl-key", Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority, Models: "MiniMax-H3", Group: "default"},
		{Id: 622, Type: constant.ChannelTypeOpenAI, Key: "openai-key", Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority, Models: "MiniMax-H3", Group: "default"},
	}
	require.NoError(t, DB.Create(&channels).Error)
	require.NoError(t, DB.Create(&[]Ability{
		{Group: "default", Model: "MiniMax-H3", ChannelId: 621, Enabled: true, Priority: &priority, Weight: weight},
		{Group: "default", Model: "MiniMax-H3", ChannelId: 622, Enabled: true, Priority: &priority, Weight: weight},
	}).Error)

	selected, err := GetChannelWithOptions("default", "MiniMax-H3", 0, ChannelSelectionOptions{
		RequestPath: "/v1/videos",
		Path:        "/v1/videos",
	})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 622, selected.Id)
}

func TestNonMiniMaxRouteNeverSelectsAutoDLWithMemoryCache(t *testing.T) {
	previousMemoryCacheEnabled := common.MemoryCacheEnabled
	common.MemoryCacheEnabled = true
	ClearChannelCacheForTest()
	t.Cleanup(func() {
		ClearChannelCacheForTest()
		common.MemoryCacheEnabled = previousMemoryCacheEnabled
	})

	priority := int64(10)
	weight := uint(100)
	autoDL := &Channel{Id: 631, Type: constant.ChannelTypeAutoDL, Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority}
	openAI := &Channel{Id: 632, Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Weight: &weight, Priority: &priority}
	SetChannelCacheForTest(map[int]*Channel{631: autoDL, 632: openAI}, map[string]map[string][]int{
		"default": {"MiniMax-H3": {631, 632}},
	})

	selected, err := GetRandomSatisfiedChannelWithOptions("default", "MiniMax-H3", 0, ChannelSelectionOptions{
		RequestPath: "/v1/videos",
		Path:        "/v1/videos",
	})

	require.NoError(t, err)
	require.NotNil(t, selected)
	assert.Equal(t, 632, selected.Id)
}
