package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"social-network-go/fcm-service/service"
	"social-network-go/pb"
)

type FCMHandler struct {
	svc *service.FCMService
}

func NewFCMHandler(svc *service.FCMService) *FCMHandler {
	return &FCMHandler{svc: svc}
}

// REST Push Handler
type PushRequest struct {
	ReceiverID string            `json:"receiverId" binding:"required"`
	Title      string            `json:"title" binding:"required"`
	Body       string            `json:"body" binding:"required"`
	Data       map[string]string `json:"data,omitempty"`
}

func (h *FCMHandler) SendPush(c *gin.Context) {
	var req PushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.svc.SendDirectPush(req.ReceiverID, req.Title, req.Body, req.Data)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "FCM push dispatched successfully",
	})
}

// gRPC Server Implementation
func (h *FCMHandler) SendPushNotification(ctx context.Context, req *pb.SendPushRequest) (*pb.SendPushResponse, error) {
	if req == nil || req.ReceiverId == "" {
		return &pb.SendPushResponse{Success: false, Message: "receiver_id is required"}, nil
	}

	err := h.svc.SendDirectPush(req.ReceiverId, req.Title, req.Body, req.Data)
	if err != nil {
		return &pb.SendPushResponse{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	return &pb.SendPushResponse{
		Success: true,
		Message: "Push notification dispatched via gRPC",
	}, nil
}
