package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// AcquireLock attempts to acquire a distributed lock, returns true if successful.
func AcquireLock(ctx context.Context, client *redis.Client, key string, value string, expiration time.Duration) (bool, error) {
	return client.SetNX(ctx, key, value, expiration).Result()
}

// ReleaseLock uses a Lua script to safely release a lock, preventing accidental deletion of another worker's lock.
func ReleaseLock(ctx context.Context, client *redis.Client, key string, value string) error {
	script := `
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`
	return client.Eval(ctx, script, []string{key}, value).Err()
}
