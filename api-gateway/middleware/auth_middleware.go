package middleware

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"social-network-go/pb"

	"github.com/gin-gonic/gin"
)

func AuthRequired(authClient pb.AuthServiceClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		authHeader := c.GetHeader("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// Fallback for WebSockets which cannot send custom headers
		if token == "" {
			token = c.Query("token")
		}

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "UNAUTHORIZED_MISSING_TOKEN",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		// Call Auth Service via gRPC
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := authClient.ValidateToken(ctx, &pb.TokenRequest{Token: token})
		if err != nil {
			log.Printf("[AUTH] gRPC ValidateToken error: %v", err)
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"code":      503,
				"message":   "AUTH_SERVICE_UNAVAILABLE",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			c.Abort()
			return
		}

		if !resp.IsValid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "INVALID_OR_EXPIRED_TOKEN",
				"timestamp": time.Now().Format(time.RFC3339),
				"error":     resp.ErrorMessage,
			})
			c.Abort()
			return
		}

		// Inject User information into the context and headers for downstream proxying
		c.Set("user_id", resp.UserId)
		c.Set("email", resp.Email)
		c.Set("role", resp.Role)

		// Set headers for downstream services to consume securely
		c.Request.Header.Set("X-User-ID", resp.UserId)
		c.Request.Header.Set("X-User-Email", resp.Email)
		c.Request.Header.Set("X-User-Role", resp.Role)

		c.Next()
	}
}
