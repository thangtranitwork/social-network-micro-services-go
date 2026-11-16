package service

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"social-network-go/admin-service/db"
)

type SearchQueryCTRItem struct {
	Query       string  `json:"query"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	CTRPercent  float64 `json:"ctrPercent"`
}

type PopularSearchClickItem struct {
	ResultID string  `json:"resultId"`
	Clicks   float64 `json:"clicks"`
}

func (s *AdminService) GetTelemetryAndSearchAnalytics(ctx context.Context) map[string]interface{} {
	userSummary := map[string]interface{}{
		"totalDeviceSessions": 0,
		"nearbyActiveUsers":   0,
		"platforms":           []interface{}{},
		"cities":              []interface{}{},
	}

	userURL := os.Getenv("USER_HTTP_ADDR")
	if userURL == "" {
		userURL = "http://localhost:10082"
	}
	userURL = strings.TrimRight(userURL, "/")
	if !strings.HasPrefix(userURL, "http://") && !strings.HasPrefix(userURL, "https://") {
		userURL = "http://" + userURL
	}
	client := &http.Client{Timeout: 3 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userURL+"/v1/internal/analytics/telemetry", nil)
	if err == nil {
		if resp, err := client.Do(req); err == nil {
			defer resp.Body.Close()
			var parsed struct {
				Body map[string]interface{} `json:"body"`
			}
			if json.NewDecoder(resp.Body).Decode(&parsed) == nil && parsed.Body != nil {
				userSummary = parsed.Body
			}
		}
	}

	topQueries := []SearchQueryCTRItem{}
	zeroClickQueries := []SearchQueryCTRItem{}
	popularPosts := []PopularSearchClickItem{}
	popularUsers := []PopularSearchClickItem{}

	if db.RedisClient != nil {
		// 1. Scan search:ctr:q:* keys
		var cursor uint64
		scanned := 0
		for {
			keys, nextCursor, err := db.RedisClient.Scan(ctx, cursor, "search:ctr:q:*", 50).Result()
			if err != nil {
				break
			}
			for _, k := range keys {
				queryText := strings.TrimPrefix(k, "search:ctr:q:")
				fields, err := db.RedisClient.HGetAll(ctx, k).Result()
				if err != nil {
					continue
				}
				var imps, clicks int64
				for field, valStr := range fields {
					val, _ := strconv.ParseInt(valStr, 10, 64)
					if strings.HasPrefix(field, "clicks:") {
						clicks += val
					} else if strings.HasPrefix(field, "impressions:") {
						imps += val
					}
				}
				totalViews := imps + clicks
				ctr := 0.0
				if totalViews > 0 {
					ctr = float64(clicks) * 100.0 / float64(totalViews)
				}
				item := SearchQueryCTRItem{
					Query:       queryText,
					Impressions: imps,
					Clicks:      clicks,
					CTRPercent:  float64(int(ctr*10)) / 10.0,
				}
				topQueries = append(topQueries, item)
				if imps > 0 && clicks == 0 {
					zeroClickQueries = append(zeroClickQueries, item)
				}
			}
			scanned += len(keys)
			cursor = nextCursor
			if cursor == 0 || scanned >= 200 {
				break
			}
		}

		sort.Slice(topQueries, func(i, j int) bool {
			return (topQueries[i].Clicks + topQueries[i].Impressions) > (topQueries[j].Clicks + topQueries[j].Impressions)
		})
		if len(topQueries) > 20 {
			topQueries = topQueries[:20]
		}
		sort.Slice(zeroClickQueries, func(i, j int) bool {
			return zeroClickQueries[i].Impressions > zeroClickQueries[j].Impressions
		})
		if len(zeroClickQueries) > 15 {
			zeroClickQueries = zeroClickQueries[:15]
		}

		// 2. Popular clicked items
		if zs, err := db.RedisClient.ZRevRangeWithScores(ctx, "search:popular_clicks:post", 0, 9).Result(); err == nil {
			for _, z := range zs {
				if id, ok := z.Member.(string); ok {
					popularPosts = append(popularPosts, PopularSearchClickItem{ResultID: id, Clicks: z.Score})
				}
			}
		}
		if zs, err := db.RedisClient.ZRevRangeWithScores(ctx, "search:popular_clicks:user", 0, 9).Result(); err == nil {
			for _, z := range zs {
				if id, ok := z.Member.(string); ok {
					popularUsers = append(popularUsers, PopularSearchClickItem{ResultID: id, Clicks: z.Score})
				}
			}
		}
	}

	return map[string]interface{}{
		"telemetry": userSummary,
		"searchCtr": map[string]interface{}{
			"topQueries":        topQueries,
			"zeroClickQueries":  zeroClickQueries,
			"popularPostClicks": popularPosts,
			"popularUserClicks": popularUsers,
		},
	}
}
