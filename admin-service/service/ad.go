package service

import (
	"context"
	"social-network-go/admin-service/model"
)

func (s *AdminService) CreateAdCampaign(ctx context.Context, campaign *model.AdCampaign) error {
	return s.repo.CreateAdCampaign(ctx, campaign)
}

func (s *AdminService) GetAdCampaigns(ctx context.Context, advertiserID string) ([]model.AdCampaign, error) {
	return s.repo.GetAdCampaigns(ctx, advertiserID)
}

func (s *AdminService) GetAdCampaignByID(ctx context.Context, campaignID string) (*model.AdCampaign, error) {
	return s.repo.GetAdCampaignByID(ctx, campaignID)
}

func (s *AdminService) UpdateAdCampaignStatus(ctx context.Context, campaignID string, status string) error {
	return s.repo.UpdateAdCampaignStatus(ctx, campaignID, status)
}

func (s *AdminService) GetPendingAdCampaigns(ctx context.Context) ([]model.AdCampaign, error) {
	return s.repo.GetPendingAdCampaigns(ctx)
}

func (s *AdminService) GetActiveAdCampaigns(ctx context.Context) ([]model.AdCampaign, error) {
	return s.repo.GetActiveAdCampaigns(ctx)
}

func (s *AdminService) LogAdInteraction(ctx context.Context, interaction *model.AdInteraction) error {
	return s.repo.LogAdInteraction(ctx, interaction)
}

func (s *AdminService) GetAdStatistics(ctx context.Context) (map[string]interface{}, error) {
	return s.repo.GetAdStatistics(ctx)
}
