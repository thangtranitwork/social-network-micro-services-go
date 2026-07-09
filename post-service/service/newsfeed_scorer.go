package service

import (
	"context"
	"sort"
	"time"

	"social-network-go/post-service/model"
)

type newsfeedScoreWeights struct {
	FriendRelationship      int `json:"friendRelationship"`
	SecondDegreeOrRequested int `json:"secondDegreeOrRequested"`
	ViewForward             int `json:"viewForward"`
	ViewBackward            int `json:"viewBackward"`
	Like                    int `json:"like"`
	Comment                 int `json:"comment"`
	Share                   int `json:"share"`
	LoadedPenalty           int `json:"loadedPenalty"`
}

type NewsfeedScoreBreakdown struct {
	Recency      int `json:"recency"`
	Relationship int `json:"relationship"`
	Engagement   int `json:"engagement"`
	Loaded       int `json:"loaded"`
	Keyword      int `json:"keyword"`
	Total        int `json:"total"`
}

var defaultNewsfeedScoreWeights = newsfeedScoreWeights{
	FriendRelationship:      100,
	SecondDegreeOrRequested: 50,
	ViewForward:             2,
	ViewBackward:            1,
	Like:                    2,
	Comment:                 3,
	Share:                   5,
	LoadedPenalty:           -20,
}

func rankNewsfeedCandidates(candidates []*model.NewsfeedCandidate, now time.Time) []*model.NewsfeedCandidate {
	return rankNewsfeedCandidatesWithWeights(candidates, now, defaultNewsfeedScoreWeights)
}

func rankNewsfeedCandidatesWithWeights(candidates []*model.NewsfeedCandidate, now time.Time, weights newsfeedScoreWeights) []*model.NewsfeedCandidate {
	scores := make(map[*model.NewsfeedCandidate]NewsfeedScoreBreakdown, len(candidates))
	for _, candidate := range candidates {
		scores[candidate] = scoreNewsfeedCandidate(candidate, now, weights)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		left := scores[candidates[i]]
		right := scores[candidates[j]]
		if left.Total != right.Total {
			return left.Total > right.Total
		}
		if candidates[i].Post == nil || candidates[j].Post == nil {
			return candidates[i].Post != nil
		}
		return candidates[i].Post.CreatedAt.After(candidates[j].Post.CreatedAt)
	})

	return candidates
}

func ScoreNewsfeedCandidate(candidate *model.NewsfeedCandidate, now time.Time) NewsfeedScoreBreakdown {
	return scoreNewsfeedCandidate(candidate, now, defaultNewsfeedScoreWeights)
}

func (s *PostService) newsfeedScoreWeights(ctx context.Context) newsfeedScoreWeights {
	weights := defaultNewsfeedScoreWeights
	if s == nil || s.RuntimeCfg == nil {
		return weights
	}
	_ = s.RuntimeCfg.JSON(ctx, "newsfeed.score_weights", &weights)
	return weights
}

func scoreNewsfeedCandidate(candidate *model.NewsfeedCandidate, now time.Time, weights newsfeedScoreWeights) NewsfeedScoreBreakdown {
	if candidate == nil || candidate.Post == nil {
		return NewsfeedScoreBreakdown{}
	}

	recency := 0
	age := now.Sub(candidate.Post.CreatedAt)
	if age < 24*time.Hour {
		recency = 240 - int(age.Hours())*10
	}

	relationship := 0
	if candidate.IsFriend {
		relationship = weights.FriendRelationship
	} else if candidate.IsSecondDegreeOrRequested {
		relationship = weights.SecondDegreeOrRequested
	}
	relationship += candidate.ViewForward*weights.ViewForward + candidate.ViewBackward*weights.ViewBackward

	engagement := candidate.Post.LikeCount*weights.Like +
		candidate.Post.CommentCount*weights.Comment +
		candidate.Post.ShareCount*weights.Share
	loaded := candidate.LoadedTimes * weights.LoadedPenalty

	total := recency + relationship + engagement + loaded + candidate.KeywordScore

	return NewsfeedScoreBreakdown{
		Recency:      recency,
		Relationship: relationship,
		Engagement:   engagement,
		Loaded:       loaded,
		Keyword:      candidate.KeywordScore,
		Total:        total,
	}
}
