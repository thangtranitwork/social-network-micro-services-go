package service

import (
	"context"

	"social-network-go/user-service/model"
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
	users, err := s.UserRepo.GetSuggestedFriends(ctx, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, users)
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
	return s.UserRepo.Unfriend(ctx, currentUserID, targetUsername)
}

func (s *UserService) Block(ctx context.Context, currentUserID string, targetUsername string) error {
	err := s.UserRepo.Block(ctx, currentUserID, targetUsername)
	if err != nil {
		return err
	}
	return nil
}

func (s *UserService) Unblock(ctx context.Context, currentUserID string, targetUsername string) error {
	return s.UserRepo.Unblock(ctx, currentUserID, targetUsername)
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

	return s.UserRepo.SendFriendRequest(ctx, currentUserID, target.ID, target.RequestReceivedCount)
}

func (s *UserService) AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	return s.UserRepo.AcceptFriendRequest(ctx, currentUserID, targetUsername)
}

func (s *UserService) DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	return s.UserRepo.DeleteFriendRequest(ctx, currentUserID, targetUsername)
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
