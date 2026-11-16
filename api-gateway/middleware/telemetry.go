package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// TelemetryMiddleware extracts client IP and device metadata headers
// and normalizes them on the forwarded request for downstream services.
func TelemetryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientIP := extractClientIP(c)
		if clientIP != "" {
			c.Request.Header.Set("X-Client-IP", clientIP)
			c.Set("client_ip", clientIP)
		}

		platform := strings.TrimSpace(c.GetHeader("X-Platform"))
		if platform == "" {
			platform = inferPlatformFromUA(c.GetHeader("User-Agent"))
		}
		if platform != "" {
			c.Request.Header.Set("X-Platform", platform)
		}

		c.Next()
	}
}

func extractClientIP(c *gin.Context) string {
	candidates := []string{
		c.GetHeader("CF-Connecting-IP"),
		c.GetHeader("X-Real-IP"),
		firstForwardedIP(c.GetHeader("X-Forwarded-For")),
		c.ClientIP(),
	}
	for _, raw := range candidates {
		ip := strings.TrimSpace(raw)
		if ip == "" {
			continue
		}
		if parsed := net.ParseIP(ip); parsed != nil {
			return parsed.String()
		}
	}
	return c.ClientIP()
}

func firstForwardedIP(header string) string {
	if header == "" {
		return ""
	}
	parts := strings.Split(header, ",")
	if len(parts) > 0 {
		return strings.TrimSpace(parts[0])
	}
	return ""
}

func inferPlatformFromUA(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case lower == "":
		return "unknown"
	case strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad"):
		return "web-ios"
	case strings.Contains(lower, "android"):
		return "web-android"
	case strings.Contains(lower, "mozilla") || strings.Contains(lower, "chrome") || strings.Contains(lower, "safari") || strings.Contains(lower, "firefox"):
		return "web"
	default:
		return "api"
	}
}
