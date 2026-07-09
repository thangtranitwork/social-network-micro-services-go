package service

import (
	"context"

	"social-network-go/user-service/model"
)

func (s *UserService) minAge(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.min_age", model.MinAge)
}

func (s *UserService) maxGivenNameLength(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_given_name_length", model.MaxGivenNameLength)
}

func (s *UserService) maxFriendCount(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_friend_count", model.MaxFriendCount)
}

func (s *UserService) maxBlockCount(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_block_count", model.MaxBlockCount)
}

func (s *UserService) maxSentRequestCount(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_sent_request_count", model.MaxSentRequestCount)
}

func (s *UserService) maxFamilyNameLength(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_family_name_length", model.MaxFamilyNameLength)
}

func (s *UserService) maxUsernameLength(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_username_length", model.MaxUsernameLength)
}

func (s *UserService) maxReceivedRequestCount(ctx context.Context) int {
	return s.runtimeInt(ctx, "user.max_received_request_count", model.MaxReceivedRequestCount)
}

func (s *UserService) runtimeInt(ctx context.Context, key string, fallback int) int {
	if s == nil || s.RuntimeCfg == nil {
		return fallback
	}
	return s.RuntimeCfg.Int(ctx, key, fallback)
}
