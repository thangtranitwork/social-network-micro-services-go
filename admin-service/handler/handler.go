package handler

import (
	"context"
	"net/http"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/service"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func sendSuccess(c *gin.Context, body interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "OK",
		"timestamp": time.Now().Format(time.RFC3339),
		"body":      body,
	})
}

func (h *AdminHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		status := "UP"
		details := gin.H{}

		// Check Neo4j
		if db.Neo4jDriver == nil {
			if gin.Mode() == gin.TestMode {
				details["neo4j"] = "UP (mocked)"
			} else {
				status = "DOWN"
				details["neo4j"] = "DOWN (driver not initialized)"
			}
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.Neo4jDriver.VerifyConnectivity(ctx); err != nil {
				status = "DOWN"
				details["neo4j"] = "DOWN (" + err.Error() + ")"
			} else {
				details["neo4j"] = "UP"
			}
			cancel()
		}

		// Check Redis
		if db.RedisClient == nil {
			if gin.Mode() == gin.TestMode {
				details["redis"] = "UP (mocked)"
			} else {
				status = "DOWN"
				details["redis"] = "DOWN (client not initialized)"
			}
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.RedisClient.Ping(ctx).Err(); err != nil {
				status = "DOWN"
				details["redis"] = "DOWN (" + err.Error() + ")"
			} else {
				details["redis"] = "UP"
			}
			cancel()
		}

		httpStatus := http.StatusOK
		if status == "DOWN" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"service":   "admin-service",
			"timestamp": time.Now().Format(time.RFC3339),
			"details":   details,
		})
	})

	r.GET("/v2/statistics/users", h.GetUsersStatistics)
	r.GET("/v2/statistics/users/week", h.GetUsersWeekStatistics)
	r.GET("/v2/statistics/users/month", h.GetUsersMonthStatistics)
	r.GET("/v2/statistics/users/year", h.GetUsersYearStatistics)
	r.GET("/v2/statistics/users/online", h.GetUsersOnlineStatistics)

	r.GET("/v2/statistics/posts", h.GetPostsStatistics)
	r.GET("/v2/statistics/posts/week", h.GetPostsWeekStatistics)
	r.GET("/v2/statistics/posts/month", h.GetPostsMonthStatistics)
	r.GET("/v2/statistics/posts/year", h.GetPostsYearStatistics)
	r.GET("/v2/statistics/posts/online", h.GetUsersOnlineStatistics) // Same as users online

	r.GET("/v1/posts", h.GetPostsList)
	r.GET("/v1/users", h.GetUsersList)

	// Docker Container Monitor Routes
	r.GET("/v1/admin/containers", h.GetContainers)
	r.GET("/v1/admin/containers/stats/ws", h.StreamContainersStats)
	r.POST("/v1/admin/containers/:id/start", h.StartContainer)
	r.POST("/v1/admin/containers/:id/stop", h.StopContainer)
	r.POST("/v1/admin/containers/:id/restart", h.RestartContainer)
	r.GET("/v1/admin/containers/:id/logs/ws", h.StreamContainerLogs)

	// Content Moderation Routes
	r.DELETE("/v1/admin/posts/:id", h.DeletePost)
	r.POST("/v1/admin/users/:id/suspend", h.SuspendUser)
	r.POST("/v1/admin/users/:id/unsuspend", h.UnsuspendUser)
}
