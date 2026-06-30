package grpc

import (
	"context"
	"encoding/json"
	"social-network-go/pb"
	"social-network-go/post-service/service"
)

type AdGrpcServer struct {
	pb.UnimplementedAdServiceServer
	postSvc *service.PostService
}

func NewAdGrpcServer(postSvc *service.PostService) *AdGrpcServer {
	return &AdGrpcServer{postSvc: postSvc}
}

// CachedAd represents the ad object stored in Redis
type CachedAd struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	MediaURL     string `json:"mediaUrl"`
	TargetURL    string `json:"targetUrl"`
	Status       string `json:"status"`
	AdvertiserID string `json:"advertiserId"`
}

func (s *AdGrpcServer) SyncAd(ctx context.Context, req *pb.AdCampaignRequest) (*pb.SyncAdResponse, error) {
	if s.postSvc.Redis == nil {
		return &pb.SyncAdResponse{Success: false, Message: "Redis client not initialized"}, nil
	}

	if req.Status == "ACTIVE" {
		ad := CachedAd{
			ID:           req.Id,
			Title:        req.Title,
			Description:  req.Description,
			MediaURL:     req.MediaUrl,
			TargetURL:    req.TargetUrl,
			Status:       req.Status,
			AdvertiserID: req.AdvertiserId,
		}

		bytes, err := json.Marshal(ad)
		if err != nil {
			return &pb.SyncAdResponse{Success: false, Message: err.Error()}, nil
		}

		err = s.postSvc.Redis.HSet(ctx, "active_ads", req.Id, string(bytes)).Err()
		if err != nil {
			return &pb.SyncAdResponse{Success: false, Message: err.Error()}, nil
		}

		return &pb.SyncAdResponse{Success: true, Message: "Ad synced to active list successfully"}, nil
	} else {
		// If status is not ACTIVE, remove it from active list
		err := s.postSvc.Redis.HDel(ctx, "active_ads", req.Id).Err()
		if err != nil {
			return &pb.SyncAdResponse{Success: false, Message: err.Error()}, nil
		}
		return &pb.SyncAdResponse{Success: true, Message: "Ad removed from active list successfully"}, nil
	}
}

func (s *AdGrpcServer) DeleteAd(ctx context.Context, req *pb.DeleteAdRequest) (*pb.DeleteAdResponse, error) {
	if s.postSvc.Redis == nil {
		return &pb.DeleteAdResponse{Success: false, Message: "Redis client not initialized"}, nil
	}

	err := s.postSvc.Redis.HDel(ctx, "active_ads", req.Id).Err()
	if err != nil {
		return &pb.DeleteAdResponse{Success: false, Message: err.Error()}, nil
	}

	return &pb.DeleteAdResponse{Success: true, Message: "Ad deleted from active list successfully"}, nil
}
