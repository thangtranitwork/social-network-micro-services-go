package router

import (
	"context"
	"net/http"
	"os"
	"time"

	"social-network-go/api-gateway/config"
	"social-network-go/api-gateway/handler"
	"social-network-go/api-gateway/middleware"
	"social-network-go/api-gateway/proxy"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

var allowedOrigins = map[string]bool{
	"http://localhost:3000":     true,
	"http://localhost:3001":     true,
	"http://localhost:10000":    true,
	"http://192.168.1.48:3000":  true,
	"http://192.168.1.48:10000": true,
	"https://pocpoc.online":     true,
	"https://www.pocpoc.online": true,
}

// SetupRoutes registers all routing policies for API Gateway
func SetupRoutes(r *gin.Engine, cfg *config.Config, authClient pb.AuthServiceClient, rdb *redis.Client) {
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
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With, X-User-ID, X-User-Email, X-User-Role, x-continue-page, X-Trace-ID, X-Request-ID, x-trace-id, x-request-id")
		c.Writer.Header().Set("Vary", "Origin")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Rate limit definitions
	publicRateLimit := middleware.RateLimiter(rdb, 100, 1*time.Minute)
	authRateLimit := middleware.RateLimiter(rdb, 300, 1*time.Minute)
	authMiddleware := middleware.AuthRequired(authClient)

	adminOnly := func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists || role != "ADMIN" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":      403,
				"message":   "FORBIDDEN_ADMIN_ONLY",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			c.Abort()
			return
		}
		c.Next()
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "UP",
			"timestamp": time.Now().Format(time.RFC3339),
			"gateway":   "UP",
		})
	})

	// Public HTML Dashboards (Authentication is checked inside JS using localStorage token)
	r.GET("/logs", handler.LogDashboard)
	r.GET("/containers", handler.ContainersDashboard)
	r.GET("/profiler", handler.ProfilerDashboard)
	r.GET("/monitor", handler.MonitorDashboard)
	r.GET("/monitor/health", handler.CheckHealth(cfg))

	// Protected Admin Observability APIs
	adminObsGroup := r.Group("")
	adminObsGroup.Use(authMiddleware, adminOnly)
	{
		adminObsGroup.GET("/logs/stream", handler.StreamLogs)
		adminObsGroup.GET("/logs/search", handler.SearchLogs)
		adminObsGroup.GET("/debug/profiler", handler.ProfilerAggregatorHandler(cfg))
		adminObsGroup.POST("/debug/profiler/reset", func(c *gin.Context) {
			serviceQuery := c.Query("service")

			services := map[string]string{
				"auth-service":         cfg.AuthHttpAddr,
				"user-service":         cfg.UserHttpAddr,
				"post-service":         cfg.PostHttpAddr,
				"chat-service":         cfg.ChatHttpAddr,
				"notification-service": cfg.NotificationHttpAddr,
				"file-service":         cfg.FileHttpAddr,
				"admin-service":        cfg.AdminHttpAddr,
				"ai-service":           cfg.AIHttpAddr,
				"search-service":       cfg.SearchHttpAddr,
				"story-service":        cfg.StoryHttpAddr,
			}

			client := &http.Client{Timeout: 500 * time.Millisecond}
			resetRemote := func(addr string) gin.H {
				ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
				defer cancel()

				req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr+"/debug/profiler/reset", nil)
				if err != nil {
					return gin.H{"ok": false, "error": err.Error()}
				}
				if token := os.Getenv("PROFILER_ADMIN_TOKEN"); token != "" {
					req.Header.Set(profiler.AdminTokenHeader, token)
				}
				resp, err := client.Do(req)
				if err != nil {
					return gin.H{"ok": false, "error": err.Error()}
				}
				defer resp.Body.Close()
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					return gin.H{"ok": false, "status": resp.StatusCode}
				}
				return gin.H{"ok": true, "status": resp.StatusCode}
			}

			if serviceQuery != "" {
				if serviceQuery == "api-gateway" {
					profiler.Reset()
					c.JSON(http.StatusOK, gin.H{"status": "success", "services": gin.H{"api-gateway": gin.H{"ok": true}}})
				} else if addr, exists := services[serviceQuery]; exists {
					result := resetRemote(addr)
					status := http.StatusOK
					if ok, _ := result["ok"].(bool); !ok {
						status = http.StatusBadGateway
					}
					c.JSON(status, gin.H{"status": "success", "services": gin.H{serviceQuery: result}})
				} else {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid service name"})
				}
				return
			} else {
				// Reset all
				profiler.Reset()
				results := gin.H{"api-gateway": gin.H{"ok": true}}
				status := http.StatusOK
				for name, addr := range services {
					result := resetRemote(addr)
					results[name] = result
					if ok, _ := result["ok"].(bool); !ok {
						status = http.StatusBadGateway
					}
				}
				c.JSON(status, gin.H{"status": "success", "services": results})
				return
			}
		})
	}

	// Public Routes (No authentication required)
	r.POST("/v1/auth/login", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.POST("/v1/auth/login-admin", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.POST("/v1/auth/refresh", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.POST("/v1/auth/refresh-admin", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.POST("/v1/auth/forgot-password", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.POST("/v1/auth/reset-password", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.GET("/v1/auth/google/login", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.GET("/v1/auth/google/callback", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/register/*any", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/register", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/forgot-password", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/reset-password", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.Any("/v1/update-password", publicRateLimit, proxy.ProxyTo(cfg.AuthHttpAddr))
	r.GET("/v1/announcement", publicRateLimit, proxy.ProxyTo(cfg.AdminHttpAddr))

	// Authenticated Routes (Requires JWT Token)

	// File Service Routes
	r.GET("/v1/files/:id", publicRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/upload", authMiddleware, authRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/upload-multiple", authMiddleware, authRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))
	r.GET("/v1/files/upload/presigned", authMiddleware, authRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))
	r.GET("/v1/files/:id/presigned", authMiddleware, authRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))
	r.DELETE("/v1/files/:id", authMiddleware, authRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))
	r.POST("/v1/files/delete-multiple", authMiddleware, authRateLimit, proxy.ProxyTo(cfg.FileHttpAddr))

	authGroup := r.Group("")
	authGroup.Use(authMiddleware, authRateLimit)
	{
		// Proxy to Auth Service (Protected routes)
		authGroup.DELETE("/v1/auth/logout", proxy.ProxyTo(cfg.AuthHttpAddr))
		authGroup.DELETE("/v1/auth/logout-user", proxy.ProxyTo(cfg.AuthHttpAddr))
		authGroup.DELETE("/v1/auth/logout-admin", proxy.ProxyTo(cfg.AuthHttpAddr))
		authGroup.POST("/v1/auth/change-password", proxy.ProxyTo(cfg.AuthHttpAddr))
		authGroup.Any("/v1/auth/2fa/*any", proxy.ProxyTo(cfg.AuthHttpAddr))
		authGroup.Any("/v1/auth/2fa", proxy.ProxyTo(cfg.AuthHttpAddr))

		// Proxy to Admin Service (Restricted to Admin role only)

		adminGroup := authGroup.Group("")
		adminGroup.Use(adminOnly)
		{
			adminGroup.Any("/v2/statistics/*any", proxy.ProxyTo(cfg.AdminHttpAddr))
			adminGroup.Any("/v2/statistics", proxy.ProxyTo(cfg.AdminHttpAddr))
			adminGroup.Any("/v1/admin/*any", proxy.ProxyTo(cfg.AdminHttpAddr))
			adminGroup.Any("/v1/admin", proxy.ProxyTo(cfg.AdminHttpAddr))
		}

		// Proxy to Admin Service for Advertisers
		authGroup.Any("/v1/ads/*any", proxy.ProxyTo(cfg.AdminHttpAddr))
		authGroup.Any("/v1/ads", proxy.ProxyTo(cfg.AdminHttpAddr))

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
		authGroup.Any("/v1/reports/*any", proxy.ProxyTo(cfg.PostHttpAddr))
		authGroup.Any("/v1/reports", proxy.ProxyTo(cfg.PostHttpAddr))

		// Proxy to Chat Service
		authGroup.Any("/v1/chat/*any", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/chat", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.POST("/v1/stringee/create-token", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/call/*any", proxy.ProxyTo(cfg.ChatHttpAddr))
		authGroup.Any("/v1/call", proxy.ProxyTo(cfg.ChatHttpAddr))

		// Proxy to Notification Service
		authGroup.Any("/v1/notifications/*any", proxy.ProxyTo(cfg.NotificationHttpAddr))
		authGroup.Any("/v1/notifications", proxy.ProxyTo(cfg.NotificationHttpAddr))

		// Proxy to Search Service
		authGroup.Any("/v1/search/*any", proxy.ProxyTo(cfg.SearchHttpAddr))
		authGroup.Any("/v1/search", proxy.ProxyTo(cfg.SearchHttpAddr))

		// Proxy to Story Service
		authGroup.Any("/v1/stories/*any", proxy.ProxyTo(cfg.StoryHttpAddr))
		authGroup.Any("/v1/stories", proxy.ProxyTo(cfg.StoryHttpAddr))
	}
}
