package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	"social-network-go/post-service/db"
	"social-network-go/post-service/model"
)

const (
	hiddenPostsKeyPrefix   = "user:hidden_posts:"
	mutedAuthorsKeyPrefix  = "user:muted_authors:"
	authorSignalKeyPrefix  = "user:author_signal:"
	mediaWatchKeyPrefix    = "post:media_watch:"
	authorSignalWindowDays = 30 * 24 * time.Hour
)

var ErrInvalidSignalType = errors.New("unsupported negative signal type")

func (s *PostService) RecordMediaWatch(ctx context.Context, userID string, req model.MediaWatchEventRequest) error {
	if s.Redis == nil || userID == "" || req.PostID == "" {
		return nil
	}
	if req.ProgressRatio < 0 {
		req.ProgressRatio = 0
	}
	if req.ProgressRatio > 1 {
		req.ProgressRatio = 1
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"mediaId":         req.MediaID,
		"mediaType":       req.MediaType,
		"watchDurationMs": req.WatchDurationMs,
		"totalDurationMs": req.TotalDurationMs,
		"progressRatio":   req.ProgressRatio,
		"muted":           req.Muted,
		"replayCount":     req.ReplayCount,
		"updatedAt":       time.Now().UTC().Format(time.RFC3339),
	})

	pipe := s.Redis.Pipeline()
	pipe.Set(ctx, fmt.Sprintf("%s%s:%s", mediaWatchKeyPrefix, userID, req.PostID), payload, 7*24*time.Hour)

	// Deep watch boost (>= 60% watched or replayed) increases affinity with the post author
	if req.AuthorID != "" && req.AuthorID != userID {
		boost := 0
		if req.ProgressRatio >= 0.85 {
			boost += 8
		} else if req.ProgressRatio >= 0.6 {
			boost += 4
		}
		if !req.Muted && req.WatchDurationMs >= 5000 {
			boost += 2
		}
		if req.ReplayCount > 0 {
			boost += 5
		}
		if boost > 0 {
			key := authorSignalKeyPrefix + userID
			pipe.HIncrBy(ctx, key, req.AuthorID, int64(boost))
			pipe.Expire(ctx, key, authorSignalWindowDays)
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *PostService) RecordNegativeSignal(ctx context.Context, userID string, req model.NegativeSignalRequest) error {
	if userID == "" || req.PostID == "" {
		return errors.New("userId and postId are required")
	}

	switch req.SignalType {
	case model.NegativeSignalHidePost,
		model.NegativeSignalNotInterested,
		model.NegativeSignalQuickSkip,
		model.NegativeSignalMuteAuthor,
		model.NegativeSignalReport:
	default:
		return ErrInvalidSignalType
	}

	if db.SQLDB != nil {
		entity := model.PostNegativeSignalEntity{
			ID:         uuid.NewString(),
			UserID:     userID,
			PostID:     req.PostID,
			AuthorID:   req.AuthorID,
			SignalType: req.SignalType,
			CreatedAt:  time.Now(),
		}
		_ = db.SQLDB.WithContext(ctx).Create(&entity).Error
	}

	if s.Redis == nil {
		return nil
	}

	pipe := s.Redis.Pipeline()
	switch req.SignalType {
	case model.NegativeSignalHidePost, model.NegativeSignalNotInterested, model.NegativeSignalReport:
		pipe.SAdd(ctx, hiddenPostsKeyPrefix+userID, req.PostID)
		pipe.Expire(ctx, hiddenPostsKeyPrefix+userID, 90*24*time.Hour)
		if req.AuthorID != "" && req.AuthorID != userID {
			pipe.HIncrBy(ctx, authorSignalKeyPrefix+userID, req.AuthorID, -30)
			pipe.Expire(ctx, authorSignalKeyPrefix+userID, authorSignalWindowDays)
		}
	case model.NegativeSignalMuteAuthor:
		if req.AuthorID != "" && req.AuthorID != userID {
			pipe.SAdd(ctx, mutedAuthorsKeyPrefix+userID, req.AuthorID)
			pipe.Expire(ctx, mutedAuthorsKeyPrefix+userID, 365*24*time.Hour)
		}
	case model.NegativeSignalQuickSkip:
		if req.AuthorID != "" && req.AuthorID != userID {
			pipe.HIncrBy(ctx, authorSignalKeyPrefix+userID, req.AuthorID, -5)
			pipe.Expire(ctx, authorSignalKeyPrefix+userID, 14*24*time.Hour)
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *PostService) UnmuteAuthor(ctx context.Context, userID, authorID string) error {
	if s.Redis == nil || userID == "" || authorID == "" {
		return nil
	}
	return s.Redis.SRem(ctx, mutedAuthorsKeyPrefix+userID, authorID).Err()
}

func (s *PostService) applyEngagementSignalsToCandidates(ctx context.Context, userID string, candidates []*model.NewsfeedCandidate) []*model.NewsfeedCandidate {
	if s == nil || s.Redis == nil || userID == "" || len(candidates) == 0 {
		return candidates
	}

	pipe := s.Redis.Pipeline()
	hiddenCmd := pipe.SMembers(ctx, hiddenPostsKeyPrefix+userID)
	mutedCmd := pipe.SMembers(ctx, mutedAuthorsKeyPrefix+userID)
	signalCmd := pipe.HGetAll(ctx, authorSignalKeyPrefix+userID)
	_, _ = pipe.Exec(ctx)

	hiddenSet := make(map[string]bool)
	for _, id := range hiddenCmd.Val() {
		hiddenSet[id] = true
	}
	mutedSet := make(map[string]bool)
	for _, id := range mutedCmd.Val() {
		mutedSet[id] = true
	}
	authorSignals := make(map[string]int)
	for authorID, rawVal := range signalCmd.Val() {
		if v, err := strconv.Atoi(rawVal); err == nil {
			// Clamp signal contribution to [-120, +60] so it balances with recency/relationship
			if v < -120 {
				v = -120
			} else if v > 60 {
				v = 60
			}
			authorSignals[authorID] = v
		}
	}

	filtered := make([]*model.NewsfeedCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c == nil || c.Post == nil {
			continue
		}
		if hiddenSet[c.Post.ID] || mutedSet[c.Post.AuthorID] {
			continue
		}
		if score, ok := authorSignals[c.Post.AuthorID]; ok {
			c.AuthorSignalScore = score
		}
		filtered = append(filtered, c)
	}
	return filtered
}
