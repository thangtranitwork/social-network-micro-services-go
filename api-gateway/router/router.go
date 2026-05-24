package router

import (
	"net/http"
	"time"

	"social-network-go/api-gateway/config"
	"social-network-go/api-gateway/handler"
	"social-network-go/api-gateway/middleware"
	"social-network-go/api-gateway/proxy"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/gin-gonic/gin"
)

var allowedOrigins = map[string]bool{
	"http://localhost:3000":      true,
	"http://localhost:3001":      true,
	"http://192.168.1.48:3000":  true,
	"https://pocpoc.online":     true,
	"https://www.pocpoc.online": true,
}

// SetupRoutes registers all routing policies for API Gateway
func SetupRoutes(r *gin.Engine, cfg *config.Config, authClient pb.AuthServiceClient) {
	// CORS Setup - Must use explicit origin (not wildcard) when credentials: true
	r.Use(func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if allowedOrigins[origin] {
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if origin == "" {
			// Same-origin or non-browser requests (curl, Postman, etc.)
			c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		}
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-User-ID, X-User-Email, X-User-Role, x-continue-page")
		c.Writer.Header().Set("Vary", "Origin")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "UP",
			"timestamp": time.Now().Format(time.RFC3339),
			"gateway":   "UP",
		})
	})

	// Logs & Dashboard
	r.GET("/logs", handler.LogDashboard)
	r.GET("/logs/stream", handler.StreamLogs)

	// Profiler
	r.GET("/profiler", handler.ProfilerDashboard)
	r.GET("/debug/profiler", handler.ProfilerAggregatorHandler(cfg))
	r.POST("/debug/profiler/reset", func(c *gin.Context) {
		profiler.Reset()

		services := []string{
			cfg.AuthHttpAddr,
			cfg.UserHttpAddr,
			cfg.PostHttpAddr,
			cfg.ChatHttpAddr,
			cfg.NotificationHttpAddr,
			cfg.FileHttpAddr,
			cfg.AdminHttpAddr,
		}

		client := &http.Client{Timeout: 100 * time.Millisecond}
		for _, addr := range services {
			go func(addr string) {
				_, _ = client.Post(addr+"/debug/profiler/reset", "application/json", nil)
			}(addr)
		}

		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// Public Routes (No authentication required)
	r.Any("/v1/auth/*any", proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/register/*any", proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/register", proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/forgot-password", proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/reset-password", proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/update-password", proxy.ProxyTo(cfg.AuthHttpAddr))

	// Authenticated Routes (Requires JWT Token)
	authMiddleware := middleware.AuthRequired(authClient)

	// File Service Routes
	r.GET("/v1/files/:id", proxy.ProxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/upload", authMiddleware, proxy.ProxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/upload-multiple", authMiddleware, proxy.ProxyTo(cfg.FileHttpAddr))
	r.GET("/v1/files/upload/presigned", authMiddleware, proxy.ProxyTo(cfg.FileHttpAddr))
	r.GET("/v1/files/:id/presigned", authMiddleware, proxy.ProxyTo(cfg.FileHttpAddr))
	r.DELETE("/v1/files/:id", authMiddleware, proxy.ProxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/delete-multiple", authMiddleware, proxy.ProxyTo(cfg.FileHttpAddr))

	authGroup := r.Group("")
	authGroup.Use(authMiddleware)
	{
		// Proxy to Admin Service
		authGroup.Any("/v2/statistics/*any", proxy.ProxyTo(cfg.AdminHttpAddr))
		authGroup.Any("/v2/statistics", proxy.ProxyTo(cfg.AdminHttpAddr))

		// Proxy to User Service
		authGroup.Any("/v1/users/*any", proxy.ProxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/users", func(c *gin.Context) {
			if c.Request.Method == "GET" && c.GetHeader("X-User-Role") == "ADMIN" {
				proxy.ProxyTo(cfg.AdminHttpAddr)(c)
			} else {
				proxy.ProxyTo(cfg.UserHttpAddr)(c)
			}
		})
		authGroup.Any("/v1/friends/*any", proxy.ProxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/friends", proxy.ProxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/blocks/*any", proxy.ProxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/blocks", proxy.ProxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/friend-request/*any", proxy.ProxyTo(cfg.UserHttpAddr))
		authGroup.Any("/v1/friend-request", proxy.ProxyTo(cfg.UserHttpAddr))

		// Proxy to Post & Feed Service
		authGroup.Any("/v1/posts/*any", proxy.ProxyTo(cfg.PostHttpAddr))
		authGroup.Any("/v1/posts", func(c *gin.Context) {
			if c.Request.Method == "GET" && c.GetHeader("X-User-Role") == "ADMIN" {
				proxy.ProxyTo(cfg.AdminHttpAddr)(c)
			} else {
				proxy.ProxyTo(cfg.PostHttpAddr)(c)
			}
		})
		authGroup.Any("/v1/comments/*any", proxy.ProxyTo(cfg.PostHttpAddr))
		authGroup.Any("/v1/comments", proxy.ProxyTo(cfg.PostHttpAddr))

		// Proxy to Chat Service
		authGroup.Any("/v1/chat/*any", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/chat", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/stringee/*any", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/stringee", proxy.ProxyTo(cfg.ChatHttpAddr))

		// Proxy to Notification Service
		authGroup.Any("/v1/notifications/*any", proxy.ProxyTo(cfg.NotificationHttpAddr))
		authGroup.Any("/v1/notifications", proxy.ProxyTo(cfg.NotificationHttpAddr))
	}
}
