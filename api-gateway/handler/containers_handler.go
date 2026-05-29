package handler

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

// ContainersDashboard serves the glassmorphic real-time dark Docker container management dashboard html
func ContainersDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(containersDashboardHTML))
}

//go:embed containers_dashboard.html
var containersDashboardHTML string
