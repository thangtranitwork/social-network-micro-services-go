package handler

import (
	"net/http"
	"time"

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
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "admin-service"})
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
}
