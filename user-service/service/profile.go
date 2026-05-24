package service

import (
	"context"
	"time"

	"social-network-go/user-service/model"
	"social-network-go/user-service/util/validation"
)

func (s *UserService) EnsureProfile(ctx context.Context, id, email, givenName, familyName, birthdate string) (*model.User, error) {
	return s.UserRepo.EnsureProfile(ctx, id, email, givenName, familyName, birthdate)
}

func (s *UserService) GetUserProfile(ctx context.Context, usernameOrID string, currentUserID string) (*model.User, error) {
	user, err := s.UserRepo.GetUserProfile(ctx, usernameOrID, currentUserID)
	if err != nil {
		return nil, err
	}
	s.enrichUsersWithPresignedURLs(ctx, []*model.User{user})
	return user, nil
}

func (s *UserService) UpdateBio(ctx context.Context, currentUserID string, bio string) error {
	err := s.UserRepo.UpdateBio(ctx, currentUserID, bio)
	if err != nil {
		return err
	}
	s.clearCache(ctx, currentUserID)
	return nil
}

func (s *UserService) UpdateBirthdate(ctx context.Context, currentUserID string, birthdateStr string) error {
	birthdate := parseTime(birthdateStr)
	if !validation.IsValidAge(birthdate, model.MinAge) {
		return ErrInvalidAge
	}
	nextDate := time.Now().AddDate(0, 0, model.ChangeBirthdateCooldownDay).Format(time.RFC3339)

	err := s.UserRepo.UpdateBirthdate(ctx, currentUserID, birthdateStr, nextDate)
	if err != nil {
		return err
	}
	s.clearCache(ctx, currentUserID)
	return nil
}

func (s *UserService) UpdateName(ctx context.Context, currentUserID string, familyName, givenName string) error {
	if !validation.IsOnlyLettersAndSpaces(familyName) || !validation.IsOnlyLettersAndSpaces(givenName) {
		return ErrInvalidName
	}
	nextDate := time.Now().AddDate(0, 0, model.ChangeNameCooldownDay).Format(time.RFC3339)

	err := s.UserRepo.UpdateName(ctx, currentUserID, familyName, givenName, nextDate)
	if err != nil {
		return err
	}
	s.clearCache(ctx, currentUserID)
	return nil
}

func (s *UserService) UpdateUsername(ctx context.Context, currentUserID string, username string) error {
	if !validation.IsValidUsername(username) {
		return ErrInvalidUsername
	}
	nextDate := time.Now().AddDate(0, 0, model.ChangeUsernameCooldownDay).Format(time.RFC3339)

	err := s.UserRepo.UpdateUsername(ctx, currentUserID, username, nextDate)
	if err != nil {
		return err
	}
	s.clearCache(ctx, currentUserID)
	return nil
}

func (s *UserService) UpdateProfilePicture(ctx context.Context, currentUserID string, fileID string) (string, error) {
	if fileID == "" {
		return "", ErrProfilePictureRequired
	}
	err := s.UserRepo.UpdateProfilePicture(ctx, currentUserID, fileID)
	if err != nil {
		return "", err
	}
	s.clearCache(ctx, currentUserID)
	return fileID, nil
}
