package runtimeconfig

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const allConfigsKey = "runtime_config:all"

type Client interface {
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
}

type Provider struct {
	client          Client
	refreshInterval time.Duration
	now             func() time.Time

	mu          sync.RWMutex
	values      map[string]ConfigValue
	lastRefresh time.Time
}

type ConfigValue struct {
	Key       string    `json:"key"`
	Scope     string    `json:"scope"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func NewProvider(client Client) *Provider {
	return &Provider{
		client:          client,
		refreshInterval: 5 * time.Second,
		now:             time.Now,
		values:          make(map[string]ConfigValue),
	}
}

func (p *Provider) Refresh(ctx context.Context) error {
	if p == nil || p.client == nil {
		return nil
	}
	rawValues, err := p.client.HGetAll(ctx, allConfigsKey).Result()
	if err != nil {
		return err
	}

	values := make(map[string]ConfigValue, len(rawValues))
	for key, raw := range rawValues {
		var value ConfigValue
		if err := json.Unmarshal([]byte(raw), &value); err != nil {
			continue
		}
		if value.Key == "" {
			value.Key = key
		}
		values[key] = value
	}

	p.mu.Lock()
	p.values = values
	p.lastRefresh = p.now()
	p.mu.Unlock()
	return nil
}

func (p *Provider) Int(ctx context.Context, key string, fallback int) int {
	raw, ok := p.String(ctx, key)
	if !ok {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func (p *Provider) String(ctx context.Context, key string) (string, bool) {
	if p == nil {
		return "", false
	}
	_ = p.ensureFresh(ctx)

	p.mu.RLock()
	value, ok := p.values[key]
	p.mu.RUnlock()
	if !ok {
		return "", false
	}
	return value.Value, true
}

func (p *Provider) JSON(ctx context.Context, key string, target interface{}) bool {
	raw, ok := p.String(ctx, key)
	if !ok {
		return false
	}
	return json.Unmarshal([]byte(raw), target) == nil
}

func (p *Provider) ensureFresh(ctx context.Context) error {
	if p == nil || p.client == nil {
		return nil
	}

	p.mu.RLock()
	stale := p.lastRefresh.IsZero() || p.now().Sub(p.lastRefresh) >= p.refreshInterval
	p.mu.RUnlock()
	if !stale {
		return nil
	}
	return p.Refresh(ctx)
}
