package handler

import (
	"context"
	"net/http"
	"os"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"
	"social-network-go/logger"
	"social-network-go/pb"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type CreateAdCampaignRequest struct {
	Title        string    `json:"title" binding:"required"`
	Description  string    `json:"description"`
	MediaURL     string    `json:"mediaUrl"`
	TargetURL    string    `json:"targetUrl" binding:"required"`
	AdType       string    `json:"adType" binding:"required"`       // FEED_POST, SIDEBAR
	TargetGender string    `json:"targetGender" binding:"required"` // ALL, MALE, FEMALE
	TargetMinAge int       `json:"targetMinAge"`
	TargetMaxAge int       `json:"targetMaxAge"`
	BudgetTotal  float64   `json:"budgetTotal" binding:"required,gt=0"`
	BidType      string    `json:"bidType" binding:"required"` // CPC, CPM
	BidAmount    float64   `json:"bidAmount" binding:"required,gt=0"`
	StartDate    time.Time `json:"startDate" binding:"required"`
	EndDate      time.Time `json:"endDate" binding:"required"`
}

type UpdateAdStatusRequest struct {
	Status string `json:"status" binding:"required"` // ACTIVE, PAUSED
}

type RejectAdRequest struct {
	Reason string `json:"reason"`
}

type LogAdInteractionRequest struct {
	CampaignID      string `json:"campaignId" binding:"required"`
	InteractionType string `json:"interactionType" binding:"required"` // VIEW, CLICK
}

func (h *AdminHandler) CreateAdCampaign(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	advertiserID, err := uuid.Parse(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_USER_ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	var req CreateAdCampaignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_REQUEST_BODY",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	if req.EndDate.Before(req.StartDate) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "END_DATE_BEFORE_START_DATE",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	campaign := &model.AdCampaign{
		ID:           uuid.New(),
		AdvertiserID: advertiserID,
		Title:        req.Title,
		Description:  req.Description,
		MediaURL:     req.MediaURL,
		TargetURL:    req.TargetURL,
		AdType:       req.AdType,
		TargetGender: req.TargetGender,
		TargetMinAge: req.TargetMinAge,
		TargetMaxAge: req.TargetMaxAge,
		BudgetTotal:  req.BudgetTotal,
		BudgetSpent:  0.0,
		BidType:      req.BidType,
		BidAmount:    req.BidAmount,
		StartDate:    req.StartDate,
		EndDate:      req.EndDate,
		Status:       "PENDING",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := h.svc.CreateAdCampaign(c.Request.Context(), campaign); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_CREATE_AD_CAMPAIGN",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, campaign)
}

func (h *AdminHandler) GetAdCampaigns(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	if userIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "MISSING_USER_ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	campaigns, err := h.svc.GetAdCampaigns(c.Request.Context(), userIDStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_GET_AD_CAMPAIGNS",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, campaigns)
}

func (h *AdminHandler) GetAdCampaignByID(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	userRole := c.GetHeader("X-User-Role")
	campaignIDStr := c.Param("id")

	campaign, err := h.svc.GetAdCampaignByID(c.Request.Context(), campaignIDStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "AD_CAMPAIGN_NOT_FOUND",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Verify ownership unless the requester is an Admin
	if campaign.AdvertiserID.String() != userIDStr && userRole != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "FORBIDDEN_CAMPAIGN_OWNERSHIP",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	sendSuccess(c, campaign)
}

func (h *AdminHandler) UpdateAdCampaignStatus(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	userRole := c.GetHeader("X-User-Role")
	campaignIDStr := c.Param("id")

	var req UpdateAdStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_REQUEST_BODY",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	campaign, err := h.svc.GetAdCampaignByID(c.Request.Context(), campaignIDStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "AD_CAMPAIGN_NOT_FOUND",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Verify ownership unless Admin
	if campaign.AdvertiserID.String() != userIDStr && userRole != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "FORBIDDEN_CAMPAIGN_OWNERSHIP",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Only allow PAUSED or ACTIVE toggles for approved campaigns
	if campaign.Status != "ACTIVE" && campaign.Status != "PAUSED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "CANNOT_UPDATE_STATUS_OF_UNAPPROVED_CAMPAIGN",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if req.Status != "ACTIVE" && req.Status != "PAUSED" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_TARGET_STATUS",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := h.svc.UpdateAdCampaignStatus(c.Request.Context(), campaignIDStr, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_UPDATE_STATUS",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	go h.syncAdToPostService(context.Background(), campaignIDStr)

	sendSuccess(c, gin.H{"status": req.Status})
}

func (h *AdminHandler) LogAdInteraction(c *gin.Context) {
	var req LogAdInteractionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_REQUEST_BODY",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	campaign, err := h.svc.GetAdCampaignByID(c.Request.Context(), req.CampaignID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "AD_CAMPAIGN_NOT_FOUND",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if campaign.Status != "ACTIVE" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "CAMPAIGN_NOT_ACTIVE",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	ipAddress := c.ClientIP()
	userAgent := c.Request.UserAgent()
	userIDStr := c.GetHeader("X-User-ID")

	var viewerID *uuid.UUID
	if userIDStr != "" {
		if parsed, err := uuid.Parse(userIDStr); err == nil {
			viewerID = &parsed
		}
	}

	// Anti-Fraud check: avoid double charge if same IP interacts with same campaign in short period (5 minutes)
	isDuplicate := false
	if db.RedisClient != nil {
		antiFraudKey := "ad:fraud:" + ipAddress + ":" + req.CampaignID + ":" + req.InteractionType
		exists, err := db.RedisClient.Exists(c.Request.Context(), antiFraudKey).Result()
		if err == nil && exists > 0 {
			isDuplicate = true
		} else {
			db.RedisClient.Set(c.Request.Context(), antiFraudKey, "1", 5*time.Minute)
		}
	}

	var cost float64 = 0.0
	if !isDuplicate {
		if req.InteractionType == "CLICK" && campaign.BidType == "CPC" {
			cost = campaign.BidAmount
		} else if req.InteractionType == "VIEW" && campaign.BidType == "CPM" {
			cost = campaign.BidAmount / 1000.0
		}
	}

	interaction := &model.AdInteraction{
		CampaignID:      campaign.ID,
		ViewerID:        viewerID,
		InteractionType: req.InteractionType,
		Cost:            cost,
		IPAddress:       ipAddress,
		UserAgent:       userAgent,
		CreatedAt:       time.Now(),
	}

	if err := h.svc.LogAdInteraction(c.Request.Context(), interaction); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_LOG_INTERACTION",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, gin.H{
		"logged":      true,
		"costCharged": cost,
		"isDuplicate": isDuplicate,
	})
}

// Admin Operations

func (h *AdminHandler) GetPendingAdCampaigns(c *gin.Context) {
	campaigns, err := h.svc.GetPendingAdCampaigns(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_GET_PENDING_CAMPAIGNS",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, campaigns)
}

func (h *AdminHandler) ApproveAdCampaign(c *gin.Context) {
	campaignIDStr := c.Param("id")

	campaign, err := h.svc.GetAdCampaignByID(c.Request.Context(), campaignIDStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "AD_CAMPAIGN_NOT_FOUND",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if campaign.Status != "PENDING" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "CAMPAIGN_NOT_IN_PENDING_STATUS",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := h.svc.UpdateAdCampaignStatus(c.Request.Context(), campaignIDStr, "ACTIVE"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_APPROVE_CAMPAIGN",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	go h.syncAdToPostService(context.Background(), campaignIDStr)

	sendSuccess(c, gin.H{"status": "ACTIVE"})
}

func (h *AdminHandler) RejectAdCampaign(c *gin.Context) {
	campaignIDStr := c.Param("id")

	campaign, err := h.svc.GetAdCampaignByID(c.Request.Context(), campaignIDStr)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "AD_CAMPAIGN_NOT_FOUND",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if campaign.Status != "PENDING" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "CAMPAIGN_NOT_IN_PENDING_STATUS",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := h.svc.UpdateAdCampaignStatus(c.Request.Context(), campaignIDStr, "REJECTED"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_REJECT_CAMPAIGN",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	go h.syncAdToPostService(context.Background(), campaignIDStr)

	sendSuccess(c, gin.H{"status": "REJECTED"})
}

func (h *AdminHandler) GetAdStatistics(c *gin.Context) {
	stats, err := h.svc.GetAdStatistics(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_GET_AD_STATISTICS",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, stats)
}

func (h *AdminHandler) GetActiveAdsForFeed(c *gin.Context) {
	// Public endpoint for post-service to query active ad campaigns matching user context
	// To minimize round-trip DB calls, post-service queries active ads matching demographics
	campaigns, err := h.svc.GetActiveAdCampaigns(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_GET_ACTIVE_ADS",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, campaigns)
}

func (h *AdminHandler) syncAdToPostService(ctx context.Context, campaignID string) {
	campaign, err := h.svc.GetAdCampaignByID(ctx, campaignID)
	if err != nil {
		logger.Error("Failed to fetch ad campaign %s for sync: %v", campaignID, err)
		return
	}

	postGrpcAddr := "localhost:10053"
	if val, ok := os.LookupEnv("POST_GRPC_ADDR"); ok {
		postGrpcAddr = val
	}

	conn, err := grpc.NewClient(postGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("Failed to connect to post-service gRPC at %s: %v", postGrpcAddr, err)
		return
	}
	defer conn.Close()

	client := pb.NewAdServiceClient(conn)

	syncCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	resp, err := client.SyncAd(syncCtx, &pb.AdCampaignRequest{
		Id:           campaign.ID.String(),
		Title:        campaign.Title,
		Description:  campaign.Description,
		MediaUrl:     campaign.MediaURL,
		TargetUrl:    campaign.TargetURL,
		Status:       campaign.Status,
		AdvertiserId: campaign.AdvertiserID.String(),
	})
	if err != nil {
		logger.Error("Failed to sync ad %s via gRPC: %v", campaign.ID.String(), err)
		return
	}

	if !resp.Success {
		logger.Error("post-service rejected ad sync: %s", resp.Message)
		return
	}

	logger.Info("Successfully synced ad %s (status: %s) to post-service", campaign.ID.String(), campaign.Status)
}
