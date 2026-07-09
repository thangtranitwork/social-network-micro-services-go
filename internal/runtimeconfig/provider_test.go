package runtimeconfig

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestProviderIntUsesCachedValue(t *testing.T) {
	provider := NewProvider(nil)
	provider.values["post.max_page_limit"] = ConfigValue{Value: "75"}

	if got := provider.Int(context.Background(), "post.max_page_limit", 100); got != 75 {
		t.Fatalf("expected cached value 75, got %d", got)
	}
}

func TestProviderIntFallsBackOnMissingOrInvalidValue(t *testing.T) {
	provider := NewProvider(nil)
	provider.values["post.max_page_limit"] = ConfigValue{Value: "not-number"}

	if got := provider.Int(context.Background(), "post.unknown", 100); got != 100 {
		t.Fatalf("expected fallback for missing key, got %d", got)
	}
	if got := provider.Int(context.Background(), "post.max_page_limit", 100); got != 100 {
		t.Fatalf("expected fallback for invalid integer, got %d", got)
	}
}

func TestProviderJSONDecodesValue(t *testing.T) {
	provider := NewProvider(nil)
	raw, err := json.Marshal(map[string]int{"like": 9})
	if err != nil {
		t.Fatal(err)
	}
	provider.values["newsfeed.score_weights"] = ConfigValue{Value: string(raw)}

	var weights struct {
		Like int `json:"like"`
	}
	if ok := provider.JSON(context.Background(), "newsfeed.score_weights", &weights); !ok {
		t.Fatal("expected JSON config to decode")
	}
	if weights.Like != 9 {
		t.Fatalf("expected decoded like weight 9, got %d", weights.Like)
	}
}

func TestRefreshParsesRuntimeConfigEnvelope(t *testing.T) {
	provider := NewProvider(fakeClient{values: map[string]string{
		"post.max_attach_files": `{"key":"post.max_attach_files","scope":"post-service","type":"INTEGER","value":"12","version":2,"updatedAt":"2026-07-09T10:00:00Z"}`,
		"broken":                `{`,
	}})
	now := time.Date(2026, 7, 9, 10, 0, 0, 0, time.UTC)
	provider.now = func() time.Time { return now }

	if err := provider.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := provider.Int(context.Background(), "post.max_attach_files", 10); got != 12 {
		t.Fatalf("expected refreshed value 12, got %d", got)
	}
	if provider.lastRefresh != now {
		t.Fatalf("expected refresh timestamp %s, got %s", now, provider.lastRefresh)
	}
	if _, ok := provider.values["broken"]; ok {
		t.Fatal("expected malformed config to be skipped")
	}
}

type fakeClient struct {
	values map[string]string
}

func (f fakeClient) HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd {
	cmd := redis.NewMapStringStringCmd(ctx)
	cmd.SetVal(f.values)
	return cmd
}
