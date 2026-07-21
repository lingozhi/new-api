package model

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/go-redis/redis/v8"
)

const persistentChannelCooldownKeyPrefix = "channel:cooldown:persistent:"

type persistentChannelCooldown struct {
	Reason        string `json:"reason"`
	ExpiresUnixMs int64  `json:"expires_unix_ms"`
}

// CooldownChannelPersistentWithoutFallback applies the cooldown locally first,
// then persists it when Redis is enabled. Redis failures never weaken the local
// protection or prevent the request from failing over to another channel.
func CooldownChannelPersistentWithoutFallback(channelId int, reason string, duration time.Duration) error {
	expires := time.Now().Add(duration)
	setChannelCooldownUntil(channelId, reason, expires, true)

	if !common.RedisEnabled {
		return nil
	}
	if common.RDB == nil {
		return errors.New("redis is unavailable")
	}

	record := persistentChannelCooldown{
		Reason:        reason,
		ExpiresUnixMs: expires.UnixMilli(),
	}
	payload, err := common.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal persistent channel cooldown: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	key := persistentChannelCooldownKeyPrefix + strconv.Itoa(channelId)
	ttl := time.Until(expires)
	if ttl <= 0 {
		return nil
	}
	const preserveLongerCooldownScript = `
local current_ttl = redis.call('PTTL', KEYS[1])
local new_ttl = tonumber(ARGV[2])
if current_ttl == -1 or current_ttl >= new_ttl then
  return 0
end
redis.call('PSETEX', KEYS[1], new_ttl, ARGV[1])
return 1
`
	if err := common.RDB.Eval(ctx, preserveLongerCooldownScript, []string{key}, string(payload), ttl.Milliseconds()).Err(); err != nil {
		return fmt.Errorf("persist channel cooldown: %w", err)
	}
	return nil
}

// RestorePersistentChannelCooldowns restores only cooldowns explicitly written
// by CooldownChannelPersistentWithoutFallback. Transient health signals remain
// process-local and naturally reset after a deploy.
func RestorePersistentChannelCooldowns() error {
	if !common.RedisEnabled {
		return nil
	}
	if common.RDB == nil {
		return errors.New("redis is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var cursor uint64
	var restoreErr error
	for {
		keys, nextCursor, err := common.RDB.Scan(ctx, cursor, persistentChannelCooldownKeyPrefix+"*", 100).Result()
		if err != nil {
			return errors.Join(restoreErr, fmt.Errorf("scan persistent channel cooldowns: %w", err))
		}
		for _, key := range keys {
			channelId, err := strconv.Atoi(strings.TrimPrefix(key, persistentChannelCooldownKeyPrefix))
			if err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("parse persistent channel cooldown key %q: %w", key, err))
				continue
			}

			payload, err := common.RDB.Get(ctx, key).Result()
			if errors.Is(err, redis.Nil) {
				continue
			}
			if err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("read persistent channel cooldown %d: %w", channelId, err))
				continue
			}
			var record persistentChannelCooldown
			if err := common.UnmarshalJsonStr(payload, &record); err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("decode persistent channel cooldown %d: %w", channelId, err))
				continue
			}
			expires := time.UnixMilli(record.ExpiresUnixMs)
			if !time.Now().Before(expires) {
				if err := common.RDB.Del(ctx, key).Err(); err != nil {
					restoreErr = errors.Join(restoreErr, fmt.Errorf("delete expired channel cooldown %d: %w", channelId, err))
				}
				continue
			}
			setChannelCooldownUntil(channelId, record.Reason, expires, true)
		}

		cursor = nextCursor
		if cursor == 0 {
			return restoreErr
		}
	}
}
