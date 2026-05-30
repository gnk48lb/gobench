package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	pkgredis "gobench/pkg/redis"
	"gobench/pkg/response"
)

// RateLimitMiddleware implements sliding window rate limiting using Redis ZSet
func RateLimitMiddleware(limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := c.ClientIP()
		key := fmt.Sprintf("gobench:rate:%s:%s", clientIP, c.FullPath())
		now := time.Now().UnixNano()
		windowStart := now - window.Nanoseconds()

		ctx := c.Request.Context()

		// Use Lua script for atomic check-and-set
		script := `
			redis.call("ZREMRANGEBYSCORE", KEYS[1], "-inf", ARGV[1])
			local count = redis.call("ZCARD", KEYS[1])
			if tonumber(count) >= tonumber(ARGV[2]) then
				return 0
			end
			redis.call("ZADD", KEYS[1], ARGV[3], ARGV[4])
			redis.call("EXPIRE", KEYS[1], ARGV[5])
			return 1
		`

		windowSeconds := int(window.Seconds())
		if windowSeconds <= 0 {
			windowSeconds = 1 // Prevent 0 expiry
		}

		res, err := pkgredis.Client.Eval(ctx, script, []string{key}, windowStart, limit, now, uuid.New().String(), windowSeconds).Result()
		if err != nil {
			response.ErrorWithStatus(c, http.StatusInternalServerError, 500, "rate limiter error")
			c.Abort()
			return
		}

		allowed, _ := res.(int64)
		if allowed == 0 {
			response.ErrorWithStatus(c, http.StatusTooManyRequests, 429, "too many requests")
			c.Abort()
			return
		}

		c.Next()
	}
}
