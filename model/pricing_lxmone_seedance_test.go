package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestSeedanceProviderPricingAndBothRoutingStores(t *testing.T) {
	resetPricingEndpointTestTables(t)
	oldPrices := ratio_setting.ModelPrice2JSONString()
	t.Cleanup(func() {
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(oldPrices))
		InvalidatePricingCache()
	})
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"seedance-2.5":1.241}`))
	oldBase, newBase := "https://argolink.io", "https://lxmone.xyz"
	oldPriority, newPriority := int64(10), int64(20)
	oldChannel := &Channel{Id: 145, Name: "old", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "test-old", BaseURL: &oldBase}
	newChannel := &Channel{Id: 148, Name: "new", Type: constant.ChannelTypeOpenAI, Status: common.ChannelStatusEnabled, Key: "test-new", BaseURL: &newBase}
	newChannel.SetOtherSettings(dto.ChannelOtherSettings{LxmoneSeedanceResolutionRatios: map[string]map[string]float64{"seedance-2.5-pro": {"480p": .30 / 1.241, "720p": .50 / 1.241}}})
	require.NoError(t, DB.Create(oldChannel).Error)
	require.NoError(t, DB.Create(newChannel).Error)
	abilities := []Ability{{ChannelId: 145, Model: "seedance-2.5", Group: "default", Enabled: true, Priority: &oldPriority}, {ChannelId: 148, Model: "seedance-2.5", Group: "default", Enabled: true, Priority: &newPriority}}
	require.NoError(t, DB.Create(&abilities).Error)
	InitChannelCache()
	filtered := filterAbilitiesByRequestPathAndModel(abilities, "/v1/videos", "seedance-2.5")
	require.Len(t, filtered, 1)
	assert.Equal(t, 148, filtered[0].ChannelId)
	channelSyncLock.RLock()
	cached := filterChannelsByRequestPathAndModel([]int{145, 148}, "/v1/videos", "seedance-2.5")
	channelSyncLock.RUnlock()
	assert.Equal(t, []int{148}, cached)
	// Exhausting the new channel must not make retry selection fall back to
	// the old wire protocol. The legacy endpoint still selects the old channel.
	assert.Empty(t, filterAbilitiesByRequestPathAndModel(abilities[:1], "/v1/videos", "seedance-2.5"))
	legacy := filterAbilitiesByRequestPathAndModel(abilities, "/v1/videos/generations", "seedance-2.5")
	require.Len(t, legacy, 1)
	assert.Equal(t, 145, legacy[0].ChannelId)
	channelSyncLock.RLock()
	exhausted := filterChannelsByRequestPathAndModel([]int{145}, "/v1/videos", "seedance-2.5")
	legacyCached := filterChannelsByRequestPathAndModel([]int{145, 148}, "/v1/videos/generations", "seedance-2.5")
	channelSyncLock.RUnlock()
	assert.Empty(t, exhausted)
	assert.Equal(t, []int{145}, legacyCached)
	prices := GetPricing()
	require.Len(t, prices, 1)
	assert.Equal(t, "lxmone-seedance", prices[0].VideoProvider)
	assert.InDelta(t, .30, prices[0].VideoResolutionPrices["480p"], 1e-12)
	assert.InDelta(t, .50, prices[0].VideoResolutionPrices["720p"], 1e-12)
	assert.Zero(t, prices[0].VideoInputRatio)
	assert.Contains(t, prices[0].SupportedEndpointTypes, constant.EndpointTypeOpenAIVideo)
	assert.Contains(t, prices[0].SupportedEndpointTypes, constant.EndpointTypeSeedanceVideo)
	require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 148).Update("enabled", false).Error)
	InvalidatePricingCache()
	prices = GetPricing()
	require.Len(t, prices, 1)
	assert.Empty(t, prices[0].VideoProvider)
	assert.InDelta(t, .5621, prices[0].VideoResolutionPrices["480p"], 1e-12)
	assert.Equal(t, 1.6, prices[0].VideoInputRatio)
}
