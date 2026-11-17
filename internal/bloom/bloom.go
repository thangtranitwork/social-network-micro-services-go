package bloom

import (
	"context"
	"strings"
	"time"
)

const (
	DefaultErrorRate = 0.001 // 0.1% false positive rate
	DefaultCapacity  = 100000
	DefaultExpansion = 2
)

// Config configures a Bloom filter instance.
type Config struct {
	Key        string
	ErrorRate  float64
	Capacity   uint64
	TTL        time.Duration
	Expansion  uint64
	NonScaling bool
}

func (c Config) withDefaults() Config {
	if c.ErrorRate <= 0 || c.ErrorRate >= 1 {
		c.ErrorRate = DefaultErrorRate
	}
	if c.Capacity == 0 {
		c.Capacity = DefaultCapacity
	}
	if c.Expansion == 0 {
		c.Expansion = DefaultExpansion
	}
	return c
}

// Filter defines a common Bloom filter contract backed by RedisBloom or in-memory bitset.
type Filter interface {
	Reserve(ctx context.Context) error
	Add(ctx context.Context, item string) error
	AddBatch(ctx context.Context, items []string) error
	Exists(ctx context.Context, item string) (bool, error)
	ExistsBatch(ctx context.Context, items []string) ([]bool, error)
	MarkReady(ctx context.Context) error
	IsReady(ctx context.Context) (bool, error)
}

// MightExist is a fail-open helper for cache-penetration guards.
// It returns false ONLY when the filter is non-nil, marked ready, and definitively
// reports the item is absent. On nil filter, unready filter, or any error, it returns true
// so callers safely fall back to the underlying datastore.
func MightExist(ctx context.Context, f Filter, item string) bool {
	if f == nil || item == "" {
		return true
	}
	if checker, ok := f.(interface {
		MightExist(ctx context.Context, item string) bool
	}); ok {
		return checker.MightExist(ctx, item)
	}

	ready, err := f.IsReady(ctx)
	if err != nil || !ready {
		return true
	}
	exists, err := f.Exists(ctx, item)
	if err != nil {
		return true
	}
	return exists
}

// NormalizeEmail normalizes email keys consistently before hashing/lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// NormalizeUsername normalizes username keys consistently before hashing/lookup.
func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}
