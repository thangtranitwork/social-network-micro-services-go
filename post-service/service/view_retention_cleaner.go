package service

import (
	"context"
	"time"

	"social-network-go/logger"
	"social-network-go/post-service/repository"
)

const (
	DefaultViewCleanupInterval  = 1 * time.Hour
	DefaultViewCleanupBatchSize = 500
	DefaultViewCleanupTimeout   = 30 * time.Second
)

// RunCleanupExpiredViewsOnce deletes expired VIEWED relations in bounded batches up to cutoff.
func RunCleanupExpiredViewsOnce(ctx context.Context, repo repository.PostRepository, cutoff time.Time, batchSize int) (int, error) {
	if repo == nil {
		return 0, nil
	}
	if batchSize <= 0 {
		batchSize = DefaultViewCleanupBatchSize
	}

	totalDeleted := 0
	for {
		select {
		case <-ctx.Done():
			return totalDeleted, ctx.Err()
		default:
		}

		deleted, err := repo.CleanupExpiredViews(ctx, cutoff, batchSize)
		if err != nil {
			return totalDeleted, err
		}
		totalDeleted += deleted
		if deleted < batchSize {
			break
		}
	}
	return totalDeleted, nil
}

// StartViewRetentionCleaner starts a periodic background worker hooked into post-service lifecycle.
func StartViewRetentionCleaner(ctx context.Context, repo repository.PostRepository, interval, retention time.Duration, batchSize int) {
	if interval <= 0 {
		interval = DefaultViewCleanupInterval
	}
	if retention <= 0 {
		retention = 48 * time.Hour
	}
	if batchSize <= 0 {
		batchSize = DefaultViewCleanupBatchSize
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	logger.WithContext(ctx).Info("Started ViewRetentionCleaner worker (interval: %v, retention: %v, batchSize: %d)", interval, retention, batchSize)

	for {
		select {
		case <-ctx.Done():
			logger.WithContext(ctx).Info("ViewRetentionCleaner worker cancelled by lifecycle context")
			return
		case <-ticker.C:
			cleanCtx, cancel := context.WithTimeout(ctx, DefaultViewCleanupTimeout)
			cutoff := time.Now().UTC().Add(-retention)

			deleted, err := RunCleanupExpiredViewsOnce(cleanCtx, repo, cutoff, batchSize)
			cancel()

			if err != nil && ctx.Err() == nil {
				logger.WithContext(ctx).Err(err).Warn("ViewRetentionCleaner encountered error during cleanup")
			} else if deleted > 0 {
				logger.WithContext(ctx).Info("ViewRetentionCleaner purged %d expired VIEWED relations (cutoff: %s)", deleted, cutoff.Format(time.RFC3339))
			}
		}
	}
}
