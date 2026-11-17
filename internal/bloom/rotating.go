package bloom

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	DefaultRotatingTTL          = 7 * 24 * time.Hour
	DefaultRotatingLookbackDays = 3
)

// RotatingStore manages daily-bucketed RedisBloom filters per scope (e.g. per user seen posts).
type RotatingStore struct {
	client       RedisClient
	prefix       string
	errorRate    float64
	capacity     uint64
	ttl          time.Duration
	lookbackDays int
	nowFn        func() time.Time
}

// RotatingConfig configures a time-bucketed RedisBloom store.
type RotatingConfig struct {
	Prefix       string
	ErrorRate    float64
	Capacity     uint64
	TTL          time.Duration
	LookbackDays int
}

// NewRotatingStore creates a daily-bucketed Bloom filter store.
// Returns nil if client is nil or prefix is empty.
func NewRotatingStore(client RedisClient, cfg RotatingConfig) *RotatingStore {
	if client == nil || strings.TrimSpace(cfg.Prefix) == "" {
		return nil
	}
	if cfg.ErrorRate <= 0 || cfg.ErrorRate >= 1 {
		cfg.ErrorRate = 0.01
	}
	if cfg.Capacity == 0 {
		cfg.Capacity = 5000
	}
	if cfg.TTL <= 0 {
		cfg.TTL = DefaultRotatingTTL
	}
	if cfg.LookbackDays <= 0 {
		cfg.LookbackDays = DefaultRotatingLookbackDays
	}
	return &RotatingStore{
		client:       client,
		prefix:       cfg.Prefix,
		errorRate:    cfg.ErrorRate,
		capacity:     cfg.Capacity,
		ttl:          cfg.TTL,
		lookbackDays: cfg.LookbackDays,
		nowFn:        func() time.Time { return time.Now().UTC() },
	}
}

func (s *RotatingStore) bucketKey(scope string, day time.Time) string {
	return fmt.Sprintf("%s:%s:%s", s.prefix, scope, day.UTC().Format("2006-01-02"))
}

// MarkSeen records items into today's bucket for the given scope.
func (s *RotatingStore) MarkSeen(ctx context.Context, scope string, items []string) error {
	if s == nil || scope == "" {
		return nil
	}
	filtered := nonEmptyStrings(items)
	if len(filtered) == 0 {
		return nil
	}
	f := NewRedisBloomFilter(s.client, Config{
		Key:       s.bucketKey(scope, s.nowFn()),
		ErrorRate: s.errorRate,
		Capacity:  s.capacity,
		TTL:       s.ttl,
	})
	return f.AddBatch(ctx, filtered)
}

// SeenSet returns the subset of items present in any of the recent lookbackDays buckets.
// On Redis error it fails open (returns empty map).
func (s *RotatingStore) SeenSet(ctx context.Context, scope string, items []string) map[string]bool {
	seen := make(map[string]bool)
	if s == nil || scope == "" || len(items) == 0 {
		return seen
	}

	filtered := nonEmptyStrings(items)
	if len(filtered) == 0 {
		return seen
	}

	now := s.nowFn()
	for d := 0; d < s.lookbackDays; d++ {
		day := now.AddDate(0, 0, -d)
		f := NewRedisBloomFilter(s.client, Config{
			Key:       s.bucketKey(scope, day),
			ErrorRate: s.errorRate,
			Capacity:  s.capacity,
			TTL:       s.ttl,
		})
		hits, err := f.ExistsBatch(ctx, filtered)
		if err != nil {
			continue
		}
		for i, hit := range hits {
			if hit {
				seen[filtered[i]] = true
			}
		}
	}
	return seen
}
