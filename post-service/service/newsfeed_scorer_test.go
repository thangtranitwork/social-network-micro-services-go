package service

import (
	"testing"
	"time"

	"social-network-go/post-service/model"
)

func TestScoreNewsfeedCandidateUsesLegacyWeights(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	candidate := &model.NewsfeedCandidate{
		Post: &model.Post{
			ID:           "post-1",
			CreatedAt:    now.Add(-3 * time.Hour),
			LikeCount:    4,
			CommentCount: 2,
			ShareCount:   1,
		},
		ViewForward:  3,
		ViewBackward: 2,
		LoadedTimes:  2,
		KeywordScore: 7,
		IsFriend:     true,
	}

	score := scoreNewsfeedCandidate(candidate, now, defaultNewsfeedScoreWeights)

	if score.Recency != 210 {
		t.Fatalf("expected recency score 210, got %d", score.Recency)
	}
	if score.Relationship != 108 {
		t.Fatalf("expected relationship score 108, got %d", score.Relationship)
	}
	if score.Engagement != 19 {
		t.Fatalf("expected engagement score 19, got %d", score.Engagement)
	}
	if score.Loaded != -40 {
		t.Fatalf("expected loaded score -40, got %d", score.Loaded)
	}
	if score.Keyword != 7 {
		t.Fatalf("expected keyword score 7, got %d", score.Keyword)
	}
	if score.Total != 304 {
		t.Fatalf("expected total score 304, got %d", score.Total)
	}
}

func TestScoreNewsfeedCandidateUsesSecondDegreeWhenNotFriend(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	candidate := &model.NewsfeedCandidate{
		Post: &model.Post{
			ID:        "post-1",
			CreatedAt: now.Add(-30 * time.Hour),
		},
		IsSecondDegreeOrRequested: true,
	}

	score := scoreNewsfeedCandidate(candidate, now, defaultNewsfeedScoreWeights)

	if score.Recency != 0 {
		t.Fatalf("expected old post recency score 0, got %d", score.Recency)
	}
	if score.Relationship != 50 {
		t.Fatalf("expected second-degree relationship score 50, got %d", score.Relationship)
	}
	if score.Total != 50 {
		t.Fatalf("expected total score 50, got %d", score.Total)
	}
}

func TestRankNewsfeedCandidatesSortsByScoreThenCreatedAt(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	newerTie := &model.NewsfeedCandidate{
		Post: &model.Post{ID: "newer-tie", CreatedAt: now.Add(-1 * time.Hour)},
	}
	olderTie := &model.NewsfeedCandidate{
		Post: &model.Post{ID: "older-tie", CreatedAt: now.Add(-1*time.Hour - time.Minute)},
	}
	highScore := &model.NewsfeedCandidate{
		Post:     &model.Post{ID: "high-score", CreatedAt: now.Add(-4 * time.Hour)},
		IsFriend: true,
	}

	ranked := rankNewsfeedCandidates([]*model.NewsfeedCandidate{olderTie, newerTie, highScore}, now)

	if ranked[0].Post.ID != "high-score" {
		t.Fatalf("expected high score candidate first, got %s", ranked[0].Post.ID)
	}
	if ranked[1].Post.ID != "newer-tie" {
		t.Fatalf("expected newer candidate to win score tie, got %s", ranked[1].Post.ID)
	}
	if ranked[2].Post.ID != "older-tie" {
		t.Fatalf("expected older tie candidate last, got %s", ranked[2].Post.ID)
	}
}

func TestPostsFromRankedCandidatesAppliesPagination(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	candidates := []*model.NewsfeedCandidate{
		{Post: &model.Post{ID: "post-1", CreatedAt: now.Add(-3 * time.Hour)}},
		{Post: &model.Post{ID: "post-2", CreatedAt: now.Add(-1 * time.Hour)}},
		{Post: &model.Post{ID: "post-3", CreatedAt: now.Add(-2 * time.Hour)}},
	}

	posts := postsFromRankedCandidates(candidates, 1, 1, now)

	if len(posts) != 1 {
		t.Fatalf("expected 1 paginated post, got %d", len(posts))
	}
	if posts[0].ID != "post-3" {
		t.Fatalf("expected second ranked post post-3, got %s", posts[0].ID)
	}
}

func TestPostsFromRankedCandidatesDeduplicatesBeforePagination(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	candidates := []*model.NewsfeedCandidate{
		{Post: &model.Post{ID: "post-1", CreatedAt: now.Add(-1 * time.Hour)}},
		{Post: &model.Post{ID: "post-1", CreatedAt: now.Add(-1 * time.Hour)}},
		{Post: &model.Post{ID: "post-2", CreatedAt: now.Add(-2 * time.Hour)}},
	}

	posts := postsFromRankedCandidates(candidates, 1, 1, now)

	if len(posts) != 1 {
		t.Fatalf("expected 1 paginated post, got %d", len(posts))
	}
	if posts[0].ID != "post-2" {
		t.Fatalf("expected duplicate post-1 to be removed before pagination, got %s", posts[0].ID)
	}
}

func TestPostIDsForLoadedSkipsAdsAndDuplicates(t *testing.T) {
	ids := postIDsForLoaded([]*model.Post{
		{ID: "post-1"},
		{ID: "post-1"},
		{ID: "ad-1", IsAd: true},
		nil,
		{ID: ""},
		{ID: "post-2"},
	})

	if len(ids) != 2 {
		t.Fatalf("expected 2 loaded post ids, got %d: %v", len(ids), ids)
	}
	if ids[0] != "post-1" || ids[1] != "post-2" {
		t.Fatalf("unexpected loaded ids: %v", ids)
	}
}
