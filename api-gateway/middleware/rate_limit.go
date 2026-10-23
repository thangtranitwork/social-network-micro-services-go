package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"social-network-go/logger"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// sendSlackRateLimitAlert sends a formatted Slack notification asynchronously when rate limit is hit
func sendSlackRateLimitAlert(webhookURL, identifier, method, path string, count int64, limit int, isBan bool) {
	if webhookURL == "" {
		return
	}

	title := "⚠️ Rate Limit Exceeded (HTTP 429)"
	color := "#FFC107" // Warning yellow
	if isBan {
		title = "🚨 Client Banned for API Spamming (HTTP 403)"
		color = "#DC3545" // Danger red
	}

	payload := map[string]any{
		"attachments": []map[string]any{
			{
				"color": color,
				"title": title,
				"fields": []map[string]any{
					{"title": "Identifier (IP/User)", "value": identifier, "short": true},
					{"title": "Endpoint", "value": method + " " + path, "short": true},
					{"title": "Request Count", "value": fmt.Sprintf("%d / %d", count, limit), "short": true},
					{"title": "Time", "value": time.Now().Format("2006-01-02 15:04:05 MST"), "short": true},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		logger.Err(err).Error("[SLACK_ALERT] Failed to send Slack webhook")
		return
	}
	defer resp.Body.Close()
}

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
			isBan := false
			// If request count exceeds the limit by 150% (e.g. limit is 100, count > 150), trigger temporary ban
			// Do not ban loopback / localhost in development
			if count > int64(float64(limit)*1.5) && identifier != "127.0.0.1" && identifier != "::1" && identifier != "localhost" {
				rdb.Set(ctx, banKey, "1", 5*time.Minute)
				logger.Warn("[RATE_LIMIT] Client %s temporarily banned for 5 minutes due to heavy API spamming (requests: %d, limit: %d)", identifier, count, limit)
				isBan = true
			}

			// Send Slack notification with 3-minute cooldown per (path + identifier) to prevent Slack flooding
			slackWebhookURL := os.Getenv("SLACK_WEBHOOK_URL")
			if slackWebhookURL != "" {
				cooldownKey := "slack_alert:rate_limit:" + c.FullPath() + ":" + identifier
				if rdb.SetNX(ctx, cooldownKey, "1", 3*time.Minute).Val() {
					method := c.Request.Method
					path := c.FullPath()
					go sendSlackRateLimitAlert(slackWebhookURL, identifier, method, path, count, limit, isBan)
				}
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

