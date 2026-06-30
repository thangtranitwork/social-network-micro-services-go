package service

import (
	"context"
	"encoding/json"
	"errors"
	"social-network-go/admin-service/db"
)

const RedisAnnouncementKey = "system_announcement"

type AnnouncementModel struct {
	Text   string `json:"text"`
	Active bool   `json:"active"`
}

func (s *AdminService) GetAnnouncement(ctx context.Context) (*AnnouncementModel, error) {
	if db.RedisClient == nil {
		return nil, errors.New("redis client not initialized")
	}

	val, err := db.RedisClient.Get(ctx, RedisAnnouncementKey).Result()
	if err != nil {
		// Key does not exist
		return &AnnouncementModel{Text: "", Active: false}, nil
	}

	var announcement AnnouncementModel
	if err := json.Unmarshal([]byte(val), &announcement); err != nil {
		return nil, err
	}

	return &announcement, nil
}

func (s *AdminService) SetAnnouncement(ctx context.Context, text string, active bool) error {
	if db.RedisClient == nil {
		return errors.New("redis client not initialized")
	}

	announcement := AnnouncementModel{
		Text:   text,
		Active: active,
	}

	data, err := json.Marshal(announcement)
	if err != nil {
		return err
	}

	return db.RedisClient.Set(ctx, RedisAnnouncementKey, string(data), 0).Err()
}

func (s *AdminService) DeleteAnnouncement(ctx context.Context) error {
	if db.RedisClient == nil {
		return errors.New("redis client not initialized")
	}

	return db.RedisClient.Del(ctx, RedisAnnouncementKey).Err()
}
