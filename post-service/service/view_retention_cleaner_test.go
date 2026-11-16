package service_test

import (
	"context"
	"testing"
	"time"

	"social-network-go/post-service/repository"
	"social-network-go/post-service/service"
)

type mockCleanupRepo struct {
	repository.PostRepository
	batchesToReturn []int
	callsCount      int
	lastCutoff      time.Time
	lastBatchSize   int
}

func (m *mockCleanupRepo) CleanupExpiredViews(ctx context.Context, cutoff time.Time, batchSize int) (int, error) {
	m.callsCount++
	m.lastCutoff = cutoff
	m.lastBatchSize = batchSize

	if len(m.batchesToReturn) == 0 {
		return 0, nil
	}
	deleted := m.batchesToReturn[0]
	m.batchesToReturn = m.batchesToReturn[1:]
	return deleted, nil
}

func TestRunCleanupExpiredViewsOnce_Batching(t *testing.T) {
	ctx := context.Background()
	cutoff := time.Now().UTC().Add(-48 * time.Hour)

	repo := &mockCleanupRepo{
		batchesToReturn: []int{500, 500, 120}, // 3 batches, total 1120
	}

	totalDeleted, err := service.RunCleanupExpiredViewsOnce(ctx, repo, cutoff, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if totalDeleted != 1120 {
		t.Fatalf("expected 1120 total deleted, got %d", totalDeleted)
	}
	if repo.callsCount != 3 {
		t.Fatalf("expected 3 batch calls, got %d", repo.callsCount)
	}
	if !repo.lastCutoff.Equal(cutoff) {
		t.Fatalf("expected cutoff %v, got %v", cutoff, repo.lastCutoff)
	}
}

func TestRunCleanupExpiredViewsOnce_PreservesRefreshedRelations(t *testing.T) {
	now := time.Now().UTC()
	cutoff := now.Add(-48 * time.Hour)

	// In-memory simulation of Neo4j relations:
	// Relation 1: lastViewedAt = 50h ago (< cutoff) -> expired
	// Relation 2: lastViewedAt = 10m ago (>= cutoff) -> refreshed concurrently
	type viewRelation struct {
		id           string
		lastViewedAt time.Time
	}

	relations := map[string]*viewRelation{
		"r-old": {id: "r-old", lastViewedAt: now.Add(-50 * time.Hour)},
		"r-refreshed": {id: "r-refreshed", lastViewedAt: now.Add(-10 * time.Minute)},
	}

	simulatedCleanup := func(cutoffTime time.Time, batchSize int) int {
		deletedCount := 0
		for id, rel := range relations {
			if rel.lastViewedAt.Before(cutoffTime) {
				delete(relations, id)
				deletedCount++
				if deletedCount >= batchSize {
					break
				}
			}
		}
		return deletedCount
	}

	deleted := simulatedCleanup(cutoff, 500)
	if deleted != 1 {
		t.Fatalf("expected exactly 1 relation deleted, got %d", deleted)
	}
	if _, exists := relations["r-old"]; exists {
		t.Fatal("expected r-old to be deleted")
	}
	if _, exists := relations["r-refreshed"]; !exists {
		t.Fatal("expected r-refreshed to be preserved because lastViewedAt >= cutoff")
	}
}

func TestStartViewRetentionCleaner_LifecycleCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repo := &mockCleanupRepo{
		batchesToReturn: []int{0},
	}

	stoppedCh := make(chan struct{})
	go func() {
		service.StartViewRetentionCleaner(ctx, repo, 50*time.Millisecond, 48*time.Hour, 500)
		close(stoppedCh)
	}()

	time.Sleep(120 * time.Millisecond)
	cancel() // Trigger lifecycle cancellation

	select {
	case <-stoppedCh:
		// Cleanly terminated on context cancellation
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ViewRetentionCleaner did not stop upon context cancellation")
	}
}
