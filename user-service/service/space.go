package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"social-network-go/user-service/model"
	"social-network-go/user-service/repository"
	"social-network-go/user-service/util/validation"

	"github.com/google/uuid"
)

func (s *UserService) GetUserSpace(ctx context.Context, usernameOrID string, currentUserID string) (*model.UserSpaceConfig, error) {
	targetUser, err := s.UserRepo.GetUserProfile(ctx, usernameOrID, currentUserID)
	if err != nil {
		return nil, err
	}
	if targetUser.BlockStatus == "BLOCKED" {
		return nil, repository.ErrHasBeenBlocked
	}
	if targetUser.BlockStatus == "BLOCKING" {
		return nil, repository.ErrHasBlocked
	}

	cfg, err := s.UserRepo.GetUserSpaceConfig(ctx, targetUser.ID)
	if err != nil {
		return nil, err
	}

	s.enrichGuestbookAvatars(cfg.GuestbookEntries)
	return cfg, nil
}

func (s *UserService) UpdateUserSpace(ctx context.Context, currentUserID string, req model.UpdateSpaceConfigRequest) (*model.UserSpaceConfig, error) {
	if err := validation.ValidateSpaceConfig(req); err != nil {
		return nil, err
	}

	cfg, err := s.UserRepo.SaveUserSpaceConfig(ctx, currentUserID, req)
	if err != nil {
		return nil, err
	}

	s.clearCache(ctx, currentUserID)
	s.enrichGuestbookAvatars(cfg.GuestbookEntries)
	return cfg, nil
}

func (s *UserService) AddSpaceGuestbookEntry(ctx context.Context, targetUsernameOrID string, authorUserID string, req model.CreateSpaceGuestbookRequest) (*model.SpaceGuestbookEntry, error) {
	if err := validation.ValidateGuestbookEntry(req); err != nil {
		return nil, err
	}

	targetUser, err := s.UserRepo.GetUserProfile(ctx, targetUsernameOrID, authorUserID)
	if err != nil {
		return nil, err
	}
	if targetUser.BlockStatus == "BLOCKED" {
		return nil, repository.ErrHasBeenBlocked
	}
	if targetUser.BlockStatus == "BLOCKING" {
		return nil, repository.ErrHasBlocked
	}

	spaceCfg, err := s.UserRepo.GetUserSpaceConfig(ctx, targetUser.ID)
	if err != nil {
		return nil, err
	}
	if !spaceCfg.AllowGuestbook {
		return nil, repository.ErrGuestbookDisabled
	}

	entry := &model.SpaceGuestbookEntry{
		ID:           uuid.New().String(),
		SpaceOwnerID: targetUser.ID,
		AuthorID:     authorUserID,
		Message:      strings.TrimSpace(req.Message),
		CardColor:    req.CardColor,
		PosX:         req.PosX,
		PosY:         req.PosY,
		CreatedAt:    time.Now(),
	}

	saved, err := s.UserRepo.AddSpaceGuestbookEntry(ctx, entry)
	if err != nil {
		return nil, err
	}

	if saved.AuthorAvatarURL != "" && !strings.HasPrefix(saved.AuthorAvatarURL, "http://") && !strings.HasPrefix(saved.AuthorAvatarURL, "https://") && s.cfg != nil && s.cfg.FileServiceURL != "" {
		saved.AuthorAvatarURL = fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, saved.AuthorAvatarURL)
	}
	return saved, nil
}

func (s *UserService) DeleteSpaceGuestbookEntry(ctx context.Context, entryID string, actorUserID string) error {
	if strings.TrimSpace(entryID) == "" {
		return repository.ErrGuestbookEntryNotFound
	}
	return s.UserRepo.DeleteSpaceGuestbookEntry(ctx, entryID, actorUserID)
}

func (s *UserService) enrichGuestbookAvatars(entries []model.SpaceGuestbookEntry) {
	if len(entries) == 0 || s.cfg == nil || s.cfg.FileServiceURL == "" {
		return
	}
	for i := range entries {
		if entries[i].AuthorAvatarURL != "" && !strings.HasPrefix(entries[i].AuthorAvatarURL, "http://") && !strings.HasPrefix(entries[i].AuthorAvatarURL, "https://") {
			entries[i].AuthorAvatarURL = fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, entries[i].AuthorAvatarURL)
		}
	}
}
