package handler

import (
	"io"
	"net/http"
	"os"
	"time"

	"social-network-go/api-gateway/config"
	"social-network-go/logger"
	"social-network-go/profiler"

	"github.com/gin-gonic/gin"
)

func NewsfeedScoreBreakdownHandler(cfg *config.Config) gin.HandlerFunc {
	client := &http.Client{Timeout: time.Second}

	return func(c *gin.Context) {
		target := cfg.PostHttpAddr + "/debug/newsfeed/score-breakdown"
		if c.Request.URL.RawQuery != "" {
			target += "?" + c.Request.URL.RawQuery
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, target, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if token := os.Getenv("PROFILER_ADMIN_TOKEN"); token != "" {
			req.Header.Set(profiler.AdminTokenHeader, token)
		}

		resp, err := client.Do(req)
		if err != nil {
			logger.WithContext(c.Request.Context()).Err(err).Error("Newsfeed score breakdown proxy failed")
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}

		for key, values := range resp.Header {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}
}
