package service

import (
	"context"
	"strconv"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type AdminService struct{}

func NewAdminService() *AdminService {
	return &AdminService{}
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

var dayOfWeekNames = []string{"", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"}
var monthNames = []string{"", "JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER"}

func (s *AdminService) QueryWeekUserStats(ctx context.Context, week, year int) map[string]int {
	stats := map[string]int{"MONDAY": 0, "TUESDAY": 0, "WEDNESDAY": 0, "THURSDAY": 0, "FRIDAY": 0, "SATURDAY": 0, "SUNDAY": 0}
	if db.Neo4jDriver == nil {
		return stats
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (u:User) WHERE u.createdAt.week=$week AND u.createdAt.year=$year
			 RETURN u.createdAt.dayOfWeek AS dayOfWeek, count(*) AS total`,
			map[string]interface{}{"week": int64(week), "year": int64(year)})
		if err != nil { return nil, err }
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if d >= 1 && d <= 7 { stats[dayOfWeekNames[d]] = t }
		}
		return nil, nil
	})
	return stats
}

func (s *AdminService) QueryMonthUserStats(ctx context.Context, month, year int) map[string]int {
	stats := make(map[string]int)
	for i := 1; i <= 31; i++ { stats[strconv.Itoa(i)] = 0 }
	if db.Neo4jDriver == nil {
		return stats
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (u:User) WHERE u.createdAt.year=$year AND u.createdAt.month=$month
			 RETURN u.createdAt.day AS day, count(*) AS total`,
			map[string]interface{}{"month": int64(month), "year": int64(year)})
		if err != nil { return nil, err }
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			stats[strconv.Itoa(d)] = t
		}
		return nil, nil
	})
	return stats
}

func (s *AdminService) QueryYearUserStats(ctx context.Context, year int) map[string]int {
	stats := map[string]int{"JANUARY": 0, "FEBRUARY": 0, "MARCH": 0, "APRIL": 0, "MAY": 0, "JUNE": 0, "JULY": 0, "AUGUST": 0, "SEPTEMBER": 0, "OCTOBER": 0, "NOVEMBER": 0, "DECEMBER": 0}
	if db.Neo4jDriver == nil {
		return stats
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (u:User) WHERE u.createdAt.year=$year
			 RETURN u.createdAt.month AS month, count(*) AS total`,
			map[string]interface{}{"year": int64(year)})
		if err != nil { return nil, err }
		for res.Next(ctx) {
			m := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if m >= 1 && m <= 12 { stats[monthNames[m]] = t }
		}
		return nil, nil
	})
	return stats
}

func (s *AdminService) QueryWeekPostStats(ctx context.Context, week, year int) map[string]int {
	stats := map[string]int{"MONDAY": 0, "TUESDAY": 0, "WEDNESDAY": 0, "THURSDAY": 0, "FRIDAY": 0, "SATURDAY": 0, "SUNDAY": 0}
	if db.Neo4jDriver == nil {
		return stats
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (post:Post) WHERE post.createdAt.week=$week AND post.createdAt.year=$year
			 RETURN post.createdAt.dayOfWeek AS dayOfWeek, count(*) AS total`,
			map[string]interface{}{"week": int64(week), "year": int64(year)})
		if err != nil { return nil, err }
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if d >= 1 && d <= 7 { stats[dayOfWeekNames[d]] = t }
		}
		return nil, nil
	})
	return stats
}

func (s *AdminService) QueryMonthPostStats(ctx context.Context, month, year int) map[string]int {
	stats := make(map[string]int)
	for i := 1; i <= 31; i++ { stats[strconv.Itoa(i)] = 0 }
	if db.Neo4jDriver == nil {
		return stats
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (post:Post) WHERE post.createdAt.year=$year AND post.createdAt.month=$month
			 RETURN post.createdAt.day AS day, count(*) AS total`,
			map[string]interface{}{"month": int64(month), "year": int64(year)})
		if err != nil { return nil, err }
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			stats[strconv.Itoa(d)] = t
		}
		return nil, nil
	})
	return stats
}

func (s *AdminService) QueryYearPostStats(ctx context.Context, year int) map[string]int {
	stats := map[string]int{"JANUARY": 0, "FEBRUARY": 0, "MARCH": 0, "APRIL": 0, "MAY": 0, "JUNE": 0, "JULY": 0, "AUGUST": 0, "SEPTEMBER": 0, "OCTOBER": 0, "NOVEMBER": 0, "DECEMBER": 0}
	if db.Neo4jDriver == nil {
		return stats
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (post:Post) WHERE post.createdAt.year=$year
			 RETURN post.createdAt.month AS month, count(*) AS total`,
			map[string]interface{}{"year": int64(year)})
		if err != nil { return nil, err }
		for res.Next(ctx) {
			m := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if m >= 1 && m <= 12 { stats[monthNames[m]] = t }
		}
		return nil, nil
	})
	return stats
}
