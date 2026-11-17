package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"social-network-go/internal/bloom"
	"social-network-go/post-service/dto"
	"social-network-go/post-service/model"
	"social-network-go/post-service/repository"
)

type spyPostRepo struct {
	repository.PostRepository
	getPostCalls int
	postToReturn *model.Post
	errToReturn  error
}

func (s *spyPostRepo) GetPost(_ context.Context, _ string, _ string) (*model.Post, error) {
	s.getPostCalls++
	return s.postToReturn, s.errToReturn
}

func TestGetPost_BloomShortCircuit(t *testing.T) {
	ctx := context.Background()
	bf := bloom.NewMemoryBloomFilter(bloom.Config{Capacity: 1000, ErrorRate: 0.01})
	require.NoError(t, bf.Add(ctx, "post-exists"))
	require.NoError(t, bf.MarkReady(ctx))

	repo := &spyPostRepo{errToReturn: errors.New("POST_NOT_FOUND")}
	svc := &PostService{
		Repo:        repo,
		PostIDBloom: bf,
	}

	// Absent in ready Bloom filter -> short-circuits without hitting Repo
	_, err := svc.GetPost(ctx, "post-missing", "user-1")
	require.EqualError(t, err, "POST_NOT_FOUND")
	require.Equal(t, 0, repo.getPostCalls)

	// Present in Bloom filter -> proceeds to Repo
	_, err = svc.GetPost(ctx, "post-exists", "user-1")
	require.EqualError(t, err, "POST_NOT_FOUND")
	require.Equal(t, 1, repo.getPostCalls)
}

func TestGetPost_BloomNotReadyFailOpen(t *testing.T) {
	ctx := context.Background()
	bf := bloom.NewMemoryBloomFilter(bloom.Config{Capacity: 1000, ErrorRate: 0.01})

	repo := &spyPostRepo{errToReturn: errors.New("POST_NOT_FOUND")}
	svc := &PostService{
		Repo:        repo,
		PostIDBloom: bf,
	}

	_, err := svc.GetPost(ctx, "post-missing", "user-1")
	require.EqualError(t, err, "POST_NOT_FOUND")
	require.Equal(t, 1, repo.getPostCalls)
}

type mockSeenStore struct {
	markedScope string
	markedItems []string
	seenMap     map[string]bool
}

func (m *mockSeenStore) MarkSeen(_ context.Context, scope string, items []string) error {
	m.markedScope = scope
	m.markedItems = append(m.markedItems, items...)
	return nil
}

func (m *mockSeenStore) SeenSet(_ context.Context, _ string, _ []string) map[string]bool {
	return m.seenMap
}

func TestPartitionBySeen(t *testing.T) {
	ctx := context.Background()
	svc := &PostService{
		SeenBloom: &mockSeenStore{
			seenMap: map[string]bool{"p1": true, "p3": true},
		},
	}

	ranked := []*model.NewsfeedCandidate{
		{Post: &model.Post{ID: "p1"}},
		{Post: &model.Post{ID: "p2"}},
		{Post: &model.Post{ID: "p3"}},
		{Post: &model.Post{ID: "p4"}},
	}

	out := svc.partitionBySeen(ctx, "user-1", ranked)
	require.Len(t, out, 4)
	require.Equal(t, []string{"p2", "p4", "p1", "p3"}, []string{out[0].Post.ID, out[1].Post.ID, out[2].Post.ID, out[3].Post.ID})
}

func TestProcessViewEvents_RecordsSeenBloom(t *testing.T) {
	ctx := context.Background()
	store := &mockSeenStore{}
	svc := &PostService{
		SeenBloom: store,
	}

	resp, err := svc.ProcessViewEvents(ctx, "user-1", &dto.ViewEventsBatchRequest{
		Events: []dto.ViewEventDTO{
			{
				EventID:           "ev-1",
				PostID:            "post-100",
				ObservedAt:        time.Now().UTC().Add(-2 * time.Second),
				VisibleDurationMs: 2500,
				VisibleRatio:      0.8,
				Surface:           "newsfeed",
				SchemaVersion:     1,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, resp.AcceptedCount)
	require.Equal(t, "user-1", store.markedScope)
	require.Equal(t, []string{"post-100"}, store.markedItems)
}
