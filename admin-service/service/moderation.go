package service

import (
	"context"
	"time"
)

func (s *AdminService) DeletePost(ctx context.Context, postID string) error {
	return s.repo.DeletePost(ctx, postID)
}

func (s *AdminService) SuspendUser(ctx context.Context, userID string, duration time.Duration) error {
	return s.repo.SuspendUser(ctx, userID, duration)
}

func (s *AdminService) UnsuspendUser(ctx context.Context, userID string) error {
	return s.repo.UnsuspendUser(ctx, userID)
}
