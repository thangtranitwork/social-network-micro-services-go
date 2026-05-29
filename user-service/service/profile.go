package service

import (
	"context"
	"time"

	"social-network-go/logger"
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

	if currentUserID != "" && user.ID != currentUserID {
		go func(viewerID, targetID string) {
			ctxBg, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.UserRepo.RecordProfileView(ctxBg, viewerID, targetID)
		}(currentUserID, user.ID)
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

	// 1. Get old profile picture ID
	var oldFileID string
	if oldUser, err := s.UserRepo.GetUserProfile(ctx, currentUserID, currentUserID); err == nil && oldUser != nil {
		oldFileID = oldUser.ProfilePictureId
	}

	// 2. Update to new profile picture
	err := s.UserRepo.UpdateProfilePicture(ctx, currentUserID, fileID)
	if err != nil {
		return "", err
	}
	s.clearCache(ctx, currentUserID)

	// 3. Delete old file if it exists and is different from the new one
	if oldFileID != "" && oldFileID != fileID && s.FileClient != nil {
		go func() {
			// Run file deletion asynchronously to avoid delaying the profile update response
			logger.Info("Deleting old profile picture %s", oldFileID)
			if err := s.FileClient.DeleteFiles(context.Background(), []string{oldFileID}); err != nil {
				logger.Err(err).Warn("Failed to delete old profile picture %s", oldFileID)
			}
		}()
	}

	return fileID, nil
}
