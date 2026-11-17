package bloom

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/redis/go-redis/v9"
)

// RedisClient is the minimal subset of *redis.Client / redis.UniversalClient needed for RedisBloom.
type RedisClient interface {
	redis.Cmdable
	Do(ctx context.Context, args ...any) *redis.Cmd
}

// RedisBloomFilter wraps Redis 8 / RedisBloom BF.* commands over go-redis.
type RedisBloomFilter struct {
	client RedisClient
	cfg    Config
}

// NewRedisBloomFilter creates a RedisBloom-backed filter.
// Returns nil if client is nil or key is empty so callers can fail-open safely.
func NewRedisBloomFilter(client RedisClient, cfg Config) *RedisBloomFilter {
	if client == nil || strings.TrimSpace(cfg.Key) == "" {
		return nil
	}
	return &RedisBloomFilter{
		client: client,
		cfg:    cfg.withDefaults(),
	}
}

func (r *RedisBloomFilter) readyKey() string {
	return r.cfg.Key + ":ready"
}

func (r *RedisBloomFilter) Reserve(ctx context.Context) error {
	args := []any{"BF.RESERVE", r.cfg.Key, r.cfg.ErrorRate, r.cfg.Capacity}
	if r.cfg.NonScaling {
		args = append(args, "NONSCALING")
	} else if r.cfg.Expansion > 0 {
		args = append(args, "EXPANSION", r.cfg.Expansion)
	}

	err := r.client.Do(ctx, args...).Err()
	if err != nil && isAlreadyExistsErr(err) {
		return nil
	}
	if err == nil && r.cfg.TTL > 0 {
		_ = r.client.Expire(ctx, r.cfg.Key, r.cfg.TTL).Err()
	}
	return err
}

func (r *RedisBloomFilter) Add(ctx context.Context, item string) error {
	if item == "" {
		return nil
	}
	if err := r.ensureReservedOnWrite(ctx); err != nil {
		return err
	}
	err := r.client.Do(ctx, "BF.ADD", r.cfg.Key, item).Err()
	if err == nil && r.cfg.TTL > 0 {
		_ = r.client.Expire(ctx, r.cfg.Key, r.cfg.TTL).Err()
	}
	return err
}

func (r *RedisBloomFilter) AddBatch(ctx context.Context, items []string) error {
	filtered := nonEmptyStrings(items)
	if len(filtered) == 0 {
		return nil
	}
	if err := r.ensureReservedOnWrite(ctx); err != nil {
		return err
	}

	args := make([]any, 0, 2+len(filtered))
	args = append(args, "BF.MADD", r.cfg.Key)
	for _, it := range filtered {
		args = append(args, it)
	}

	err := r.client.Do(ctx, args...).Err()
	if err == nil && r.cfg.TTL > 0 {
		_ = r.client.Expire(ctx, r.cfg.Key, r.cfg.TTL).Err()
	}
	return err
}

func (r *RedisBloomFilter) Exists(ctx context.Context, item string) (bool, error) {
	if item == "" {
		return false, nil
	}
	res, err := r.client.Do(ctx, "BF.EXISTS", r.cfg.Key, item).Int64()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

func (r *RedisBloomFilter) ExistsBatch(ctx context.Context, items []string) ([]bool, error) {
	out := make([]bool, len(items))
	if len(items) == 0 {
		return out, nil
	}

	args := make([]any, 0, 2+len(items))
	args = append(args, "BF.MEXISTS", r.cfg.Key)
	for _, it := range items {
		args = append(args, it)
	}

	raw, err := r.client.Do(ctx, args...).Slice()
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(out) && i < len(raw); i++ {
		if v, ok := toInt64(raw[i]); ok && v == 1 {
			out[i] = true
		}
	}
	return out, nil
}

func (r *RedisBloomFilter) MarkReady(ctx context.Context) error {
	if r.cfg.TTL > 0 {
		return r.client.Set(ctx, r.readyKey(), "1", r.cfg.TTL).Err()
	}
	return r.client.Set(ctx, r.readyKey(), "1", 0).Err()
}

func (r *RedisBloomFilter) IsReady(ctx context.Context) (bool, error) {
	val, err := r.client.Get(ctx, r.readyKey()).Result()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return val == "1", nil
}

// MightExist executes GET <key>:ready and BF.EXISTS <key> <item> in a single pipeline round-trip.
func (r *RedisBloomFilter) MightExist(ctx context.Context, item string) bool {
	if r == nil || item == "" {
		return true
	}

	pipe := r.client.Pipeline()
	readyCmd := pipe.Get(ctx, r.readyKey())
	existsCmd := pipe.Do(ctx, "BF.EXISTS", r.cfg.Key, item)
	_, err := pipe.Exec(ctx)
	if err != nil && !errors.Is(err, redis.Nil) {
		return true
	}

	readyVal, readyErr := readyCmd.Result()
	if readyErr != nil || readyVal != "1" {
		return true
	}

	existsVal, existsErr := existsCmd.Int64()
	if existsErr != nil {
		return true
	}
	return existsVal == 1
}

func (r *RedisBloomFilter) ensureReservedOnWrite(ctx context.Context) error {
	// BF.ADD / BF.MADD auto-creates with Redis defaults if missing, but reserving once
	// ensures configured errorRate and capacity are honored when key doesn't exist yet.
	exists, err := r.client.Exists(ctx, r.cfg.Key).Result()
	if err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	return r.Reserve(ctx)
}

func isAlreadyExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "item exists") || strings.Contains(msg, "already exists")
}

func nonEmptyStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it != "" {
			out = append(out, it)
		}
	}
	return out
}

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case uint64:
		return int64(n), true
	case string:
		var parsed int64
		_, err := fmt.Sscanf(n, "%d", &parsed)
		return parsed, err == nil
	default:
		return 0, false
	}
}
