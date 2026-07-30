package handler

import (
	"context"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"social-network-go/logger"
	"social-network-go/notification-service/config"
	"social-network-go/notification-service/model"
	"social-network-go/notification-service/service"
)

type NotificationHandler struct {
	svc    *service.NotificationService
	cfg    *config.Config
	driver neo4j.DriverWithContext
}

func NewNotificationHandler(svc *service.NotificationService, cfg *config.Config, driver neo4j.DriverWithContext) *NotificationHandler {
	return &NotificationHandler{
		svc:    svc,
		cfg:    cfg,
		driver: driver,
	}
}

func (h *NotificationHandler) HandleWebSocket(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		appEnv := strings.ToLower(os.Getenv("APP_ENV"))
		if appEnv != "production" && appEnv != "prod" && appEnv != "staging" {
			userID = c.Query("userId")
		}
	}
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized or missing userId"})
		return
	}

	conn, err := h.svc.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.Error("Failed to upgrade connection: %v", err)
		return
	}

	h.svc.RegisterClient(userID, conn)

	go func() {
		defer h.svc.UnregisterClient(userID)
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				break
			}
		}
	}()
}

func (h *NotificationHandler) GetNotifications(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	if userID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"message":   "OK",
			"timestamp": time.Now().Format(time.RFC3339),
			"body": gin.H{
				"notifications":           []interface{}{},
				"unreadNotificationCount": 0,
			},
		})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	notifications, unreadCount, err := h.svc.GetNotifications(userID, page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "OK",
		"timestamp": time.Now().Format(time.RFC3339),
		"body": gin.H{
			"notifications":           notifications,
			"unreadNotificationCount": unreadCount,
		},
	})
}

func (h *NotificationHandler) GetUnreadCount(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	if userID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"message":   "OK",
			"timestamp": time.Now().Format(time.RFC3339),
			"body":      0,
		})
		return
	}

	unreadCount, _ := h.svc.GetUnreadCount(userID)
	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "OK",
		"timestamp": time.Now().Format(time.RFC3339),
		"body":      unreadCount,
	})
}

func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	if userID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"message":   "OK",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	_ = h.svc.MarkAsRead(userID, limit)

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "OK",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (h *NotificationHandler) SaveFCMToken(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	var req model.FCMTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
		return
	}

	if userID == "" {
		userID = req.UserID
	}
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "userId or X-User-ID header is required"})
		return
	}

	if req.DeviceType == "" {
		req.DeviceType = "WEB"
	}

	err := h.svc.SaveFCMToken(userID, req.FCMToken, req.DeviceType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save FCM token: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "FCM token registered successfully",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

func (h *NotificationHandler) HealthCheck(c *gin.Context) {
	status := "UP"
	details := gin.H{}

	if h.driver == nil {
		status = "DOWN"
		details["neo4j"] = "DOWN (driver not initialized)"
	} else {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		if err := h.driver.VerifyConnectivity(ctx); err != nil {
			status = "DOWN"
			details["neo4j"] = "DOWN (" + err.Error() + ")"
		} else {
			details["neo4j"] = "UP"
		}
		cancel()
	}

	kafkaConn, err := net.DialTimeout("tcp", h.cfg.KafkaAddr, 2*time.Second)
	if err != nil {
		status = "DOWN"
		details["kafka"] = "DOWN (" + err.Error() + ")"
	} else {
		details["kafka"] = "UP"
		kafkaConn.Close()
	}

	httpStatus := http.StatusOK
	if status == "DOWN" {
		httpStatus = http.StatusServiceUnavailable
	}

	c.JSON(httpStatus, gin.H{
		"status":    status,
		"service":   "notification-service",
		"timestamp": time.Now().Format(time.RFC3339),
		"details":   details,
	})
}
