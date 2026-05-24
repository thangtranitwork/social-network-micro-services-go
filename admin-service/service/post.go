package service

import (
	"context"

	"social-network-go/admin-service/model"
)

func (s *AdminService) GetPostsStatistics(ctx context.Context) (*model.PostStatisticsResponse, error) {
	return s.repo.GetPostsStatistics(ctx)
}

func (s *AdminService) GetPostsList(ctx context.Context, skip, limit int) ([]model.PostResponse, error) {
	return s.repo.GetPostsList(ctx, skip, limit)
}

func (s *AdminService) QueryWeekPostStats(ctx context.Context, week, year int) map[string]int {
	stats, err := s.repo.QueryWeekPostStats(ctx, week, year)
	if err != nil {
		return map[string]int{}
	}
	return stats
}

func (s *AdminService) QueryMonthPostStats(ctx context.Context, month, year int) map[string]int {
	stats, err := s.repo.QueryMonthPostStats(ctx, month, year)
	if err != nil {
		return map[string]int{}
	}
	return stats
}

func (s *AdminService) QueryYearPostStats(ctx context.Context, year int) map[string]int {
	stats, err := s.repo.QueryYearPostStats(ctx, year)
	if err != nil {
		return map[string]int{}
	}
	return stats
}
