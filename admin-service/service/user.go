package service

import (
	"context"

	"social-network-go/admin-service/model"
)

func (s *AdminService) GetUsersStatistics(ctx context.Context) (*model.UserStatisticsResponse, error) {
	return s.repo.GetUsersStatistics(ctx)
}

func (s *AdminService) GetUsersList(ctx context.Context, skip, limit int) ([]model.UserDetailResponse, error) {
	list, err := s.repo.GetUsersList(ctx, skip, limit)
	if err != nil {
		return nil, err
	}
	for i := range list {
		isOnline, lastOnline := s.GetUserOnlineStatus(ctx, list[i].ID)
		list[i].IsOnline = isOnline
		list[i].LastOnline = lastOnline
	}
	return list, nil
}

func (s *AdminService) QueryWeekUserStats(ctx context.Context, week, year int) map[string]int {
	stats, err := s.repo.QueryWeekUserStats(ctx, week, year)
	if err != nil {
		return map[string]int{}
	}
	return stats
}

func (s *AdminService) QueryMonthUserStats(ctx context.Context, month, year int) map[string]int {
	stats, err := s.repo.QueryMonthUserStats(ctx, month, year)
	if err != nil {
		return map[string]int{}
	}
	return stats
}

func (s *AdminService) QueryYearUserStats(ctx context.Context, year int) map[string]int {
	stats, err := s.repo.QueryYearUserStats(ctx, year)
	if err != nil {
		return map[string]int{}
	}
	return stats
}
