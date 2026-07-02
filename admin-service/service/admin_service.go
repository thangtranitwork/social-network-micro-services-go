package service

import (
	"context"
	"sync"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"
	"social-network-go/admin-service/repository"
)

type AdminService struct {
	repo           repository.AdminRepository
	moderationRepo repository.ModerationRepository
	moderation     moderationStore
}

func NewAdminService(repo repository.AdminRepository) *AdminService {
	return &AdminService{
		repo:           repo,
		moderationRepo: repository.NewModerationRepository(db.PostgresDB),
		moderation: moderationStore{
			items:   make(map[string]*model.ModerationQueueItem),
			audits:  make([]model.ModerationAuditLog, 0),
			reports: make(map[string]map[string]bool),
		},
	}
}

type moderationStore struct {
	mu      sync.RWMutex
	items   map[string]*model.ModerationQueueItem
	audits  []model.ModerationAuditLog
	reports map[string]map[string]bool
}

func (s *AdminService) GetOnlineUsersCount() int {
	if db.RedisClient == nil {
		return 0
	}
	val, err := db.RedisClient.Get(context.Background(), "online_user_count").Int()
	if err != nil {
		return 0
	}
	return val
}

func (s *AdminService) GetUserOnlineStatus(ctx context.Context, userID string) (bool, string) {
	if db.RedisClient == nil || userID == "" {
		return false, time.Now().Format(time.RFC3339)
	}
	counterKey := "user_online_counter:" + userID
	val, err := db.RedisClient.Get(ctx, counterKey).Int()
	isOnline := err == nil && val > 0

	lastOnlineKey := "last_online:" + userID
	lastOnlineStr, err := db.RedisClient.Get(ctx, lastOnlineKey).Result()
	if err != nil || lastOnlineStr == "" {
		lastOnlineStr = time.Now().Format(time.RFC3339)
	}

	return isOnline, lastOnlineStr
}

func (s *AdminService) GetOnlineStatisticsLogs(dayStr string) []model.OnlineUserLog {
	targetDay := time.Now()
	if dayStr != "" {
		if t, err := time.Parse("2006-01-02", dayStr); err == nil {
			targetDay = t
		}
	}
	onlineNow := s.GetOnlineUsersCount()
	currentHour := time.Now().Hour()
	isSameDay := targetDay.Format("2006-01-02") == time.Now().Format("2006-01-02")

	var logs []model.OnlineUserLog
	for i := 0; i < 24; i++ {
		count := 0
		if isSameDay && i == currentHour {
			count = onlineNow
		}
		logTime := time.Date(targetDay.Year(), targetDay.Month(), targetDay.Day(), i, 0, 0, 0, time.Local)
		logs = append(logs, model.OnlineUserLog{
			Timestamp:   logTime.Format("2006-01-02T15:04:05Z"),
			OnlineCount: count,
		})
	}
	return logs
}
