package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"social-network-go/search-service/db"
)

type SearchCTREvent struct {
	Query      string `json:"query" binding:"required"`
	ResultType string `json:"resultType"` // user | post
	ResultID   string `json:"resultId" binding:"required"`
	Position   int    `json:"position"`
	Clicked    bool   `json:"clicked"`
}

func (s *SearchService) RecordSearchCTR(ctx context.Context, userID string, ev SearchCTREvent) error {
	if db.RedisClient == nil {
		return nil
	}
	normQuery := strings.ToLower(strings.TrimSpace(ev.Query))
	if normQuery == "" || ev.ResultID == "" {
		return nil
	}
	rType := strings.ToLower(strings.TrimSpace(ev.ResultType))
	if rType == "" {
		rType = "post"
	}

	pipe := db.RedisClient.Pipeline()
	queryKey := fmt.Sprintf("search:ctr:q:%s", normQuery)
	itemKey := fmt.Sprintf("%s:%s", rType, ev.ResultID)

	if ev.Clicked {
		pipe.HIncrBy(ctx, queryKey, "clicks:"+itemKey, 1)
		pipe.ZIncrBy(ctx, "search:popular_clicks:"+rType, 1, ev.ResultID)
	} else {
		pipe.HIncrBy(ctx, queryKey, "impressions:"+itemKey, 1)
	}
	pipe.Expire(ctx, queryKey, 30*24*time.Hour)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *SearchService) GetSearchCTRStats(ctx context.Context, query string) (map[string]string, error) {
	if db.RedisClient == nil {
		return map[string]string{}, nil
	}
	normQuery := strings.ToLower(strings.TrimSpace(query))
	if normQuery == "" {
		return map[string]string{}, nil
	}
	return db.RedisClient.HGetAll(ctx, fmt.Sprintf("search:ctr:q:%s", normQuery)).Result()
}
