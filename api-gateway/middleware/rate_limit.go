package middleware

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"social-network-go/logger"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

func RateLimiter(rdb *redis.Client, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rdb == nil {
			c.Next()
			return
		}

		// Identify by User ID if authenticated, else IP address
		identifier := c.GetString("user_id")
		if identifier == "" {
			identifier = c.ClientIP()
		}

		// Sliding window rate limiting keys
		key := "rate_limit:" + c.FullPath() + ":" + identifier
		banKey := "rate_limit_ban:" + identifier
		now := time.Now()
		clearBefore := now.Add(-window).UnixMilli()
		member := strconv.FormatInt(now.UnixNano(), 10)

		ctx := context.Background()

		// Run Redis commands in a pipeline (checks ban state and processes window in 1 RTT)
		pipe := rdb.TxPipeline()
		banCheckCmd := pipe.Exists(ctx, banKey)
		pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(clearBefore, 10))
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixMilli()), Member: member})
		countCmd := pipe.ZCard(ctx, key)
		pipe.Expire(ctx, key, window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			logger.Err(err).Error("[RATE_LIMIT] Redis execution error")
			c.Next() // fail open on redis errors
			return
		}

		// If client is already banned, block the request immediately
		if banCheckCmd.Val() > 0 {
			c.JSON(http.StatusForbidden, gin.H{
				"code":      403,
				"message":   "IP_TEMPORARILY_BANNED_DUE_TO_SPAM",
				"timestamp": now.Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		count := countCmd.Val()

		// Add RateLimit headers
		c.Writer.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
		remaining := int64(limit) - count
		if remaining < 0 {
			remaining = 0
		}
		c.Writer.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > int64(limit) {
			// If request count exceeds the limit by 150% (e.g. limit is 100, count > 150), trigger temporary ban
			if count > int64(float64(limit)*1.5) {
				rdb.Set(ctx, banKey, "1", 5*time.Minute)
				logger.Warn("[RATE_LIMIT] Client %s temporarily banned for 5 minutes due to heavy API spamming (requests: %d, limit: %d)", identifier, count, limit)
			}

			c.JSON(http.StatusTooManyRequests, gin.H{
				"code":      429,
				"message":   "TOO_MANY_REQUESTS",
				"timestamp": now.Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		c.Next()
	}
}
