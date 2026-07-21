package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func useChannelCooldownTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = client
	t.Cleanup(func() {
		clearChannelCooldownsForTest()
		require.NoError(t, client.Close())
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})
	return server
}

func TestChannelCooldownSkipsChannelUntilExpiry(t *testing.T) {
	clearChannelCooldownsForTest()

	CooldownChannel(17, "Insufficient account balance", time.Minute)

	if !IsChannelCoolingDown(17) {
		t.Fatalf("expected channel 17 to be cooling down")
	}
	if IsChannelCoolingDown(29) {
		t.Fatalf("expected channel 29 to remain available")
	}
}

func TestChannelCooldownExpires(t *testing.T) {
	clearChannelCooldownsForTest()

	CooldownChannel(17, "Insufficient account balance", -time.Second)

	if IsChannelCoolingDown(17) {
		t.Fatalf("expected expired cooldown to be cleared")
	}
}

func TestChannelCooldownCannotBeShortenedByConcurrentFailure(t *testing.T) {
	clearChannelCooldownsForTest()
	t.Cleanup(clearChannelCooldownsForTest)

	CooldownChannel(17, "stream_capacity", 15*time.Minute)
	_, firstExpiry, cooling := GetChannelCooldown(17)
	require.True(t, cooling)

	CooldownChannel(17, "retryable_transient", 5*time.Minute)
	reason, secondExpiry, cooling := GetChannelCooldown(17)
	require.True(t, cooling)
	assert.Equal(t, firstExpiry, secondExpiry)
	assert.Equal(t, "stream_capacity", reason)
}

func TestChannelCooldownTracksStrictFallbackWindowSeparately(t *testing.T) {
	for _, strictFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("strict_first=%t", strictFirst), func(t *testing.T) {
			clearChannelCooldownsForTest()
			t.Cleanup(clearChannelCooldownsForTest)

			startedAt := time.Now()
			if strictFirst {
				CooldownChannelWithoutFallback(17, "stream_capacity", 15*time.Minute)
				CooldownChannel(17, "stream_unstable", time.Hour)
			} else {
				CooldownChannel(17, "stream_unstable", time.Hour)
				CooldownChannelWithoutFallback(17, "stream_capacity", 15*time.Minute)
			}

			state := getChannelCooldownState(17)
			require.True(t, state.active)
			assert.False(t, state.allowFallback)
			assert.WithinDuration(t, startedAt.Add(time.Hour), state.expires, time.Second)
			assert.WithinDuration(t, startedAt.Add(15*time.Minute), state.fallbackBlockedUntil, time.Second)
		})
	}
}

func TestExpiredStrictCooldownDoesNotBlockLongerFallbackEligibleCooldown(t *testing.T) {
	clearChannelCooldownsForTest()
	t.Cleanup(clearChannelCooldownsForTest)

	CooldownChannel(17, "stream_unstable", time.Hour)
	CooldownChannelWithoutFallback(17, "expired_capacity", -time.Second)

	state := getChannelCooldownState(17)
	require.True(t, state.active)
	assert.True(t, state.allowFallback)
}

func TestGetChannelCooldownReportsActiveStrictReason(t *testing.T) {
	clearChannelCooldownsForTest()
	t.Cleanup(clearChannelCooldownsForTest)

	startedAt := time.Now()
	CooldownChannel(17, "stream_unstable", time.Hour)
	CooldownChannelWithoutFallback(17, "stream_capacity", 15*time.Minute)

	reason, expires, cooling := GetChannelCooldown(17)
	require.True(t, cooling)
	assert.Equal(t, "stream_capacity", reason)
	assert.WithinDuration(t, startedAt.Add(15*time.Minute), time.Unix(expires, 0), time.Second)
}

func TestPersistentStrictChannelCooldownRestoresAfterProcessRestart(t *testing.T) {
	useChannelCooldownTestRedis(t)
	clearChannelCooldownsForTest()

	startedAt := time.Now()
	require.NoError(t, CooldownChannelPersistentWithoutFallback(57, "upstream_rate_limit", 2*time.Hour))
	clearChannelCooldownsForTest()
	assert.False(t, IsChannelCoolingDown(57), "clearing process memory should simulate a replacement instance")

	require.NoError(t, RestorePersistentChannelCooldowns())
	reason, expires, cooling := GetChannelCooldown(57)
	require.True(t, cooling)
	assert.Equal(t, "upstream_rate_limit", reason)
	assert.False(t, IsChannelCoolingFallbackAllowed(57))
	assert.WithinDuration(t, startedAt.Add(2*time.Hour), time.Unix(expires, 0), time.Second)
}

func TestTransientChannelCooldownDoesNotRestoreAfterProcessRestart(t *testing.T) {
	useChannelCooldownTestRedis(t)
	clearChannelCooldownsForTest()

	CooldownChannelWithoutFallback(57, "stream_capacity", 15*time.Minute)
	clearChannelCooldownsForTest()

	require.NoError(t, RestorePersistentChannelCooldowns())
	assert.False(t, IsChannelCoolingDown(57), "transient stream state must remain process-local")
}

func TestPersistentChannelCooldownCannotBeShortenedInRedis(t *testing.T) {
	useChannelCooldownTestRedis(t)
	clearChannelCooldownsForTest()

	startedAt := time.Now()
	require.NoError(t, CooldownChannelPersistentWithoutFallback(78, "upstream_rate_limit", 2*time.Hour))
	require.NoError(t, CooldownChannelPersistentWithoutFallback(78, "account_unavailable", 30*time.Minute))
	clearChannelCooldownsForTest()

	require.NoError(t, RestorePersistentChannelCooldowns())
	reason, expires, cooling := GetChannelCooldown(78)
	require.True(t, cooling)
	assert.Equal(t, "upstream_rate_limit", reason)
	assert.WithinDuration(t, startedAt.Add(2*time.Hour), time.Unix(expires, 0), time.Second)
}

func TestPersistentChannelCooldownFailsOpenWhenRedisIsUnavailable(t *testing.T) {
	clearChannelCooldownsForTest()
	oldRedisEnabled, oldRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = nil
	t.Cleanup(func() {
		clearChannelCooldownsForTest()
		common.RedisEnabled = oldRedisEnabled
		common.RDB = oldRDB
	})

	err := CooldownChannelPersistentWithoutFallback(91, "balance_exhausted", 30*time.Minute)
	require.Error(t, err)
	assert.True(t, IsChannelCoolingDown(91), "Redis failure must not disable local protection")
	assert.False(t, IsChannelCoolingFallbackAllowed(91))
}
