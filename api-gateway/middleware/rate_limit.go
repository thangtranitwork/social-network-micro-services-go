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

		// Sliding window rate limiting key
		key := "rate_limit:" + c.FullPath() + ":" + identifier
		now := time.Now()
		clearBefore := now.Add(-window).UnixMilli()
		member := strconv.FormatInt(now.UnixNano(), 10)

		ctx := context.Background()

		// Run Redis commands in a pipeline
		pipe := rdb.TxPipeline()
		// Remove elements older than the window
		pipe.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(clearBefore, 10))
		// Add the current timestamp as score
		pipe.ZAdd(ctx, key, redis.Z{Score: float64(now.UnixMilli()), Member: member})
		// Get total requests in the window
		countCmd := pipe.ZCard(ctx, key)
		// Set TTL for the rate limiting key so it doesn't linger forever
		pipe.Expire(ctx, key, window)

		_, err := pipe.Exec(ctx)
		if err != nil {
			logger.Err(err).Error("[RATE_LIMIT] Redis execution error")
			c.Next() // fail open on redis errors
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
