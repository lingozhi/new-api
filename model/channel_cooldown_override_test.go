package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCooldownOverrideSkipsPersistentAndExistingCooldowns(t *testing.T) {
	server := useChannelCooldownTestRedis(t)
	old := setting.ChannelCooldownDisabledJSON()
	t.Cleanup(func() { require.NoError(t, setting.UpdateChannelCooldownDisabled(old)) })
	require.NoError(t, setting.UpdateChannelCooldownDisabled(`{}`))
	require.NoError(t, CooldownChannelPersistentWithoutFallback(70101, "insufficient balance", time.Hour))
	require.True(t, IsChannelCoolingDown(70101))
	require.NoError(t, setting.UpdateChannelCooldownDisabled(`{"70101":true,"70102":true}`))
	assert.False(t, IsChannelCoolingDown(70101), "override applies to an already cooling channel immediately")
	require.NoError(t, CooldownChannelPersistentWithoutFallback(70102, "insufficient balance", time.Hour))
	assert.False(t, server.Exists(persistentChannelCooldownKeyPrefix+"70102"), "new failures must not persist a cooldown")
	clearChannelCooldownsForTest()
	require.NoError(t, RestorePersistentChannelCooldowns())
	assert.False(t, IsChannelCoolingDown(70101), "a restart must not restore an opted-out channel's cooldown")
	CooldownChannelWithoutFallback(70102, "rate limited", time.Hour)
	assert.False(t, IsChannelCoolingDown(70102))
	CooldownChannelWithoutFallback(70103, "rate limited", time.Hour)
	assert.True(t, IsChannelCoolingDown(70103), "other channels retain their cooldown policy")
}

func TestCooldownOverrideKeepsChannelSelectableWithAndWithoutMemoryCache(t *testing.T) {
	for _, memory := range []bool{false, true} {
		name := "database"
		if memory {
			name = "memory"
		}
		t.Run(name, func(t *testing.T) {
			setupChannelSelectionTestDB(t)
			oldSetting := setting.ChannelCooldownDisabledJSON()
			oldHealth, oldHostMode := common.AdaptiveChannelHealthEnabled, common.UpstreamHostCircuitMode
			oldChannels, oldGroups := channelsIDM, group2model2channels
			oldCustom, oldImage := channel2advancedCustomConfig, channel2ImageRoutingConfig
			t.Cleanup(func() {
				require.NoError(t, setting.UpdateChannelCooldownDisabled(oldSetting))
				common.AdaptiveChannelHealthEnabled, common.UpstreamHostCircuitMode = oldHealth, oldHostMode
				clearChannelHealthForTest()
				ClearChannelHostCooldownsForTest()
				channelSyncLock.Lock()
				channelsIDM, group2model2channels = oldChannels, oldGroups
				channel2advancedCustomConfig, channel2ImageRoutingConfig = oldCustom, oldImage
				channelSyncLock.Unlock()
			})
			require.NoError(t, setting.UpdateChannelCooldownDisabled(`{}`))
			common.AdaptiveChannelHealthEnabled = true
			common.UpstreamHostCircuitMode = common.UpstreamHostCircuitModeEnforce
			clearChannelHealthForTest()
			ClearChannelHostCooldownsForTest()
			baseURL := "https://provider.example"
			priority := int64(10)
			for _, id := range []int{70101, 70102} {
				require.NoError(t, DB.Create(&Channel{Id: id, Type: 1, Key: "test", Status: 1, Models: "wan3.0-video", Group: "default", BaseURL: &baseURL, Priority: &priority}).Error)
				require.NoError(t, DB.Create(&Ability{ChannelId: id, Model: "wan3.0-video", Group: "default", Enabled: true, Priority: &priority}).Error)
				CooldownChannelWithoutFallback(id, "insufficient balance", time.Hour)
				key := ChannelHealthKey{ChannelID: id, Model: "wan3.0-video", Path: "/v1/tasks"}
				RecordChannelOutcome(key, ChannelOutcome{StatusCode: 503})
				RecordChannelOutcome(key, ChannelOutcome{StatusCode: 503})
				RecordChannelOutcome(key, ChannelOutcome{StatusCode: 503})
				require.False(t, IsChannelHealthAvailable(key))
			}
			RecordChannelHostFailure("provider.example", "wan3.0-video", "/v1/tasks", 70101, "transport")
			RecordChannelHostFailure("provider.example", "wan3.0-video", "/v1/tasks", 70102, "transport")
			RecordChannelHostFailure("provider.example", "wan3.0-video", "/v1/tasks", 70101, "transport")
			require.True(t, IsChannelHostCoolingDown("provider.example", "wan3.0-video", "/v1/tasks"))
			common.MemoryCacheEnabled = memory
			if memory {
				InitChannelCache()
			}
			options := ChannelSelectionOptions{RequestPath: "/v1/videos", Path: "/v1/tasks"}
			selected, err := GetRandomSatisfiedChannelWithOptions("default", "wan3.0-video", 0, options)
			require.NoError(t, err)
			require.Nil(t, selected)
			require.NoError(t, setting.UpdateChannelCooldownDisabled(`{"70101":true}`))
			selected, err = GetRandomSatisfiedChannelWithOptions("default", "wan3.0-video", 0, options)
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, 70101, selected.Id)
			assert.False(t, IsChannelRouteHostCoolingDown(selected, "wan3.0-video", "/v1/videos", "/v1/tasks"))
			assert.True(t, AcquireChannelHealthForAffinity(ChannelHealthKey{ChannelID: 70101, Model: "wan3.0-video", Path: "/v1/tasks"}))
			options.ExcludedChannelIDs = map[int]struct{}{70101: {}}
			selected, err = GetRandomSatisfiedChannelWithOptions("default", "wan3.0-video", 0, options)
			require.NoError(t, err)
			assert.Nil(t, selected, "same-request exclusions must still prevent duplicate attempts")
			require.NoError(t, DB.Model(&Channel{}).Where("id = ?", 70101).Update("status", 2).Error)
			require.NoError(t, DB.Model(&Ability{}).Where("channel_id = ?", 70101).Update("enabled", false).Error)
			if memory {
				InitChannelCache()
			}
			options.ExcludedChannelIDs = nil
			selected, err = GetRandomSatisfiedChannelWithOptions("default", "wan3.0-video", 0, options)
			require.NoError(t, err)
			assert.Nil(t, selected, "the override must not enable a disabled channel")
		})
	}
}
