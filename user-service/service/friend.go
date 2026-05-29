package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"social-network-go/logger"
	"social-network-go/user-service/model"
	red "social-network-go/user-service/redis"
)

func (s *UserService) GetFriends(ctx context.Context, username string, currentUserID string) ([]*model.User, error) {
	if _, err := s.GetUserProfile(ctx, username, currentUserID); err != nil {
		return nil, err
	}
	users, err := s.UserRepo.GetFriends(ctx, username, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)
	return users, nil
}

func (s *UserService) GetSuggestedFriends(ctx context.Context, currentUserID string) ([]*model.User, error) {
	cacheKey := fmt.Sprintf("user:suggested_friends:%s", currentUserID)
	if red.RedisClient != nil {
		if cachedData, err := red.RedisClient.Get(ctx, cacheKey).Result(); err == nil && cachedData != "" {
			var cachedUsers []*model.User
			if err := json.Unmarshal([]byte(cachedData), &cachedUsers); err == nil {
				logger.WithContext(ctx).Info("GetSuggestedFriends cache hit for user: %s", currentUserID)
				return cachedUsers, nil
			}
		}
	}

	users, err := s.UserRepo.GetSuggestedFriends(ctx, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)

	if red.RedisClient != nil && len(users) > 0 {
		if data, err := json.Marshal(users); err == nil {
			_ = red.RedisClient.Set(ctx, cacheKey, data, 10*time.Minute).Err()
		}
	}

	return users, nil
}

func (s *UserService) GetMutualFriends(ctx context.Context, currentUserID string, targetUsername string) ([]*model.User, error) {
	users, err := s.UserRepo.GetMutualFriends(ctx, currentUserID, targetUsername)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)
	return users, nil
}

func (s *UserService) Unfriend(ctx context.Context, currentUserID string, targetUsername string) error {
	err := s.UserRepo.Unfriend(ctx, currentUserID, targetUsername)
	if err != nil {
		return err
	}
	s.clearSuggestedFriendsCache(ctx, currentUserID)
	s.clearSuggestedFriendsCacheByUsername(ctx, targetUsername)
	return nil
}

func (s *UserService) Block(ctx context.Context, currentUserID string, targetUsername string) error {
	err := s.UserRepo.Block(ctx, currentUserID, targetUsername)
	if err != nil {
		return err
	}
	s.clearSuggestedFriendsCache(ctx, currentUserID)
	s.clearSuggestedFriendsCacheByUsername(ctx, targetUsername)
	return nil
}

func (s *UserService) Unblock(ctx context.Context, currentUserID string, targetUsername string) error {
	err := s.UserRepo.Unblock(ctx, currentUserID, targetUsername)
	if err != nil {
		return err
	}
	s.clearSuggestedFriendsCache(ctx, currentUserID)
	s.clearSuggestedFriendsCacheByUsername(ctx, targetUsername)
	return nil
}

func (s *UserService) GetBlockedUsers(ctx context.Context, currentUserID string) ([]*model.User, error) {
	users, err := s.UserRepo.GetBlockedUsers(ctx, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)
	return users, nil
}

func (s *UserService) SendFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	target, err := s.GetUserProfile(ctx, targetUsername, currentUserID)
	if err != nil {
		return err
	}
	if currentUserID == target.ID {
		return ErrCannotMakeSelfRequest
	}

	err = s.UserRepo.SendFriendRequest(ctx, currentUserID, target.ID, target.RequestReceivedCount)
	if err != nil {
		return err
	}
	s.clearSuggestedFriendsCache(ctx, currentUserID)
	s.clearSuggestedFriendsCache(ctx, target.ID)
	return nil
}

func (s *UserService) AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	err := s.UserRepo.AcceptFriendRequest(ctx, currentUserID, targetUsername)
	if err != nil {
		return err
	}
	s.clearSuggestedFriendsCache(ctx, currentUserID)
	s.clearSuggestedFriendsCacheByUsername(ctx, targetUsername)
	return nil
}

func (s *UserService) DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	err := s.UserRepo.DeleteFriendRequest(ctx, currentUserID, targetUsername)
	if err != nil {
		return err
	}
	s.clearSuggestedFriendsCache(ctx, currentUserID)
	s.clearSuggestedFriendsCacheByUsername(ctx, targetUsername)
	return nil
}

func (s *UserService) GetSentRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	users, err := s.UserRepo.GetSentRequests(ctx, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)
	return users, nil
}

func (s *UserService) GetReceivedRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	users, err := s.UserRepo.GetReceivedRequests(ctx, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)
	return users, nil
}

func (s *UserService) clearSuggestedFriendsCache(ctx context.Context, userID string) {
	if red.RedisClient == nil || userID == "" {
		return
	}
	cacheKey := fmt.Sprintf("user:suggested_friends:%s", userID)
	_ = red.RedisClient.Del(ctx, cacheKey).Err()
}

func (s *UserService) clearSuggestedFriendsCacheByUsername(ctx context.Context, username string) {
	if red.RedisClient == nil || username == "" {
		return
	}
	target, err := s.UserRepo.GetUserProfile(ctx, username, "")
	if err == nil && target != nil {
		s.clearSuggestedFriendsCache(ctx, target.ID)
	}
}
