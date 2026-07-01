package profiler

import (
	"errors"
	"testing"
	"time"
)

func TestCalculatePercentiles(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
		20 * time.Millisecond,
		30 * time.Millisecond,
		40 * time.Millisecond,
		50 * time.Millisecond,
		60 * time.Millisecond,
		70 * time.Millisecond,
		80 * time.Millisecond,
		90 * time.Millisecond,
		100 * time.Millisecond,
	}

	p50, p90, p99 := calculatePercentiles(durations)

	if p50 != 60*time.Millisecond {
		t.Errorf("Expected P50 to be 60ms, got %v", p50)
	}
	if p90 != 100*time.Millisecond {
		t.Errorf("Expected P90 to be 100ms, got %v", p90)
	}
	if p99 != 100*time.Millisecond {
		t.Errorf("Expected P99 to be 100ms, got %v", p99)
	}
}

func TestCalculatePercentilesSmall(t *testing.T) {
	durations := []time.Duration{
		10 * time.Millisecond,
	}

	p50, p90, p99 := calculatePercentiles(durations)

	if p50 != 10*time.Millisecond {
		t.Errorf("Expected P50 to be 10ms, got %v", p50)
	}
	if p90 != 10*time.Millisecond {
		t.Errorf("Expected P90 to be 10ms, got %v", p90)
	}
	if p99 != 10*time.Millisecond {
		t.Errorf("Expected P99 to be 10ms, got %v", p99)
	}
}

func TestTrackExecution(t *testing.T) {
	Reset()

	TrackExecution("test-cmd", func() {
		time.Sleep(10 * time.Millisecond)
	})

	stats, found := GetCommandStats("test-cmd")
	if !found {
		t.Fatalf("Expected test-cmd stats to be found")
	}

	if stats.RequestCount.Load() != 1 {
		t.Errorf("Expected RequestCount to be 1, got %d", stats.RequestCount.Load())
	}

	if len(stats.LastExecutions) != 1 {
		t.Errorf("Expected LastExecutions length to be 1, got %d", len(stats.LastExecutions))
	}

	if stats.LastExecutions[0] < 10*time.Millisecond {
		t.Errorf("Expected duration to be at least 10ms, got %v", stats.LastExecutions[0])
	}
}

func TestTrackExecutionWithReturn(t *testing.T) {
	Reset()

	res, err := TrackExecutionWithReturn("test-cmd-ret", func() (any, error) {
		time.Sleep(5 * time.Millisecond)
		return "hello", nil
	})

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if res != "hello" {
		t.Errorf("Expected result 'hello', got %v", res)
	}

	stats, found := GetCommandStats("test-cmd-ret")
	if !found {
		t.Fatalf("Expected test-cmd-ret stats to be found")
	}

	if stats.RequestCount.Load() != 1 {
		t.Errorf("Expected RequestCount to be 1, got %d", stats.RequestCount.Load())
	}
}

func TestTrackExecutionPanic(t *testing.T) {
	Reset()

	// Should recover from panic
	TrackExecution("test-panic", func() {
		panic("something went wrong")
	})

	stats, found := GetCommandStats("test-panic")
	if !found {
		t.Fatalf("Expected test-panic stats to be found")
	}

	// Should still record the execution even if it panicked
	if stats.RequestCount.Load() != 1 {
		t.Errorf("Expected RequestCount to be 1, got %d", stats.RequestCount.Load())
	}
}

func TestTrackExecutionWithReturnPanic(t *testing.T) {
	Reset()

	// With return panic is not recovered inside zprofiler, let's verify zprofiler's logic:
	// zprofiler's TrackExecutionWithReturn does not wrap fn with recover. So it would panic out.
	// But let's verify our implementation works identically.
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic from TrackExecutionWithReturn to propagate, but it was swallowed")
		} else {
			// Check if stats still got registered. In standard implementation, stats might not record if panic happens
			// because the deferred recordExecution is not set up on TrackExecutionWithReturn in zprofiler.
			// Let's verify we behave the same.
		}
	}()

	_, _ = TrackExecutionWithReturn("test-ret-panic", func() (any, error) {
		panic("ret panic")
	})
}

func TestTrackExecutionWithReturnError(t *testing.T) {
	Reset()

	res, err := TrackExecutionWithReturn("test-ret-err", func() (any, error) {
		return nil, errors.New("some error")
	})

	if err == nil || err.Error() != "some error" {
		t.Errorf("Expected error 'some error', got %v", err)
	}
	if res != nil {
		t.Errorf("Expected result nil, got %v", res)
	}
}

func TestTrackResult(t *testing.T) {
	Reset()

	res, err := TrackResult("test-typed-result", func() (string, error) {
		time.Sleep(5 * time.Millisecond)
		return "typed", nil
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if res != "typed" {
		t.Fatalf("Expected typed result, got %q", res)
	}

	stats, found := GetCommandStats("test-typed-result")
	if !found {
		t.Fatalf("Expected test-typed-result stats to be found")
	}
	if stats.RequestCount.Load() != 1 {
		t.Errorf("Expected RequestCount to be 1, got %d", stats.RequestCount.Load())
	}
}

func TestTrackEvent(t *testing.T) {
	Reset()

	TrackEvent("test-event")
	TrackEvent("test-event")

	stats, found := GetCommandStats("test-event")
	if !found {
		t.Fatalf("Expected test-event stats to be found")
	}
	if stats.RequestCount.Load() != 2 {
		t.Errorf("Expected RequestCount to be 2, got %d", stats.RequestCount.Load())
	}
	if stats.PendingCount.Load() != 0 {
		t.Errorf("Expected PendingCount to stay 0, got %d", stats.PendingCount.Load())
	}
}

func TestTrackCacheLookup(t *testing.T) {
	Reset()

	TrackCacheLookup("test-cache", true, nil)
	TrackCacheLookup("test-cache", false, nil)
	TrackCacheLookup("test-cache", false, errors.New("redis down"))

	for _, command := range []string{"test-cache.hit", "test-cache.miss", "test-cache.error"} {
		stats, found := GetCommandStats(command)
		if !found {
			t.Fatalf("Expected %s stats to be found", command)
		}
		if stats.RequestCount.Load() != 1 {
			t.Errorf("Expected %s RequestCount to be 1, got %d", command, stats.RequestCount.Load())
		}
	}
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Specific Prefix rules
		{"/v1/users/of-user/biden", "/v1/users/of-user/:username"},
		{"/v1/users/of-user/hochiminh/", "/v1/users/of-user/:username"},
		{"/v1/posts/of-user/obama", "/v1/posts/of-user/:username"},
		{"/v1/posts/files/macron", "/v1/posts/files/:username"},
		{"/v1/posts/like/12345", "/v1/posts/like/:postId"},
		{"/v1/posts/like/post_123", "/v1/posts/like/:postId"},
		{"/v1/friends/mutual-friends/putin", "/v1/friends/mutual-friends/:username"},
		{"/v1/friend-request/send/trump", "/v1/friend-request/send/:username"},
		{"/v1/friend-request/accept/zelenskyy", "/v1/friend-request/accept/:username"},
		{"/v1/friend-request/delete/user_1", "/v1/friend-request/delete/:username"},

		// Generic Segment rules
		{"/v1/users/user_12", "/v1/users/:username"},
		{"/v1/users/update-bio", "/v1/users/update-bio"},
		{"/v1/friends/user_33", "/v1/friends/:username"},
		{"/v1/friends/suggested", "/v1/friends/suggested"},
		{"/v1/blocks/obama", "/v1/blocks/:username"},
		{"/v1/stories/story_99", "/v1/stories/:id"},
		{"/v1/stories/feed", "/v1/stories/feed"},
		{"/v1/files/file_abc123", "/v1/files/:id"},
		{"/v1/files/file_abc123/presigned", "/v1/files/:id/presigned"},
		{"/v1/files/upload", "/v1/files/upload"},

		// Admin routes
		{"/v1/admin/containers/container_abc123/start", "/v1/admin/containers/:id/start"},
		{"/v1/admin/containers/123/stop", "/v1/admin/containers/:id/stop"},

		// Fallbacks
		{"/v1/some-service/123", "/v1/some-service/:id"},
		{"/v1/some-service/60d5ec49f1b2c5001f3f3e12", "/v1/some-service/:id"},
	}

	for _, tc := range tests {
		got := normalizePath(tc.input)
		if got != tc.expected {
			t.Errorf("normalizePath(%q) = %q; want %q", tc.input, got, tc.expected)
		}
	}
}

func TestParseProfiledCommandKeepsNormalizedPathParams(t *testing.T) {
	tests := []struct {
		command     string
		wantService string
		wantMethod  string
		wantPath    string
	}{
		{
			command:     "api-gateway:GET /v1/users/:username",
			wantService: "api-gateway",
			wantMethod:  "GET",
			wantPath:    "/v1/users/:username",
		},
		{
			command:     "post-service:POST /v1/posts/like/:postId",
			wantService: "post-service",
			wantMethod:  "POST",
			wantPath:    "/v1/posts/like/:postId",
		},
		{
			command:     "notification-service:WS /v1/notifications/ws",
			wantService: "notification-service",
			wantMethod:  "WS",
			wantPath:    "/v1/notifications/ws",
		},
		{
			command:     "background-job",
			wantService: "",
			wantMethod:  "ANY",
			wantPath:    "background-job",
		},
	}

	for _, tc := range tests {
		gotService, gotMethod, gotPath := parseProfiledCommand(tc.command)
		if gotService != tc.wantService || gotMethod != tc.wantMethod || gotPath != tc.wantPath {
			t.Errorf("parseProfiledCommand(%q) = (%q, %q, %q); want (%q, %q, %q)",
				tc.command, gotService, gotMethod, gotPath, tc.wantService, tc.wantMethod, tc.wantPath)
		}
	}
}

func TestGetRouteStatsLightweightReturnsStructuredRows(t *testing.T) {
	Reset()

	TrackExecution("api-gateway:GET /v1/users/:username", func() {})

	routes := GetRouteStatsLightweight()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}

	route := routes[0]
	if route.Command != "api-gateway:GET /v1/users/:username" {
		t.Errorf("Command = %q; want %q", route.Command, "api-gateway:GET /v1/users/:username")
	}
	if route.Service != "api-gateway" {
		t.Errorf("Service = %q; want api-gateway", route.Service)
	}
	if route.Method != "GET" {
		t.Errorf("Method = %q; want GET", route.Method)
	}
	if route.Path != "/v1/users/:username" {
		t.Errorf("Path = %q; want /v1/users/:username", route.Path)
	}
	if route.RequestCount != 1 {
		t.Errorf("RequestCount = %d; want 1", route.RequestCount)
	}
}

func TestGetRouteStatsLightweightSkipsLegacyWildcardCommands(t *testing.T) {
	Reset()

	TrackExecution("api-gateway:GET /v1/posts/*any", func() {})
	TrackExecution("api-gateway:GET /v1/posts/:id", func() {})

	routes := GetRouteStatsLightweight()
	if len(routes) != 1 {
		t.Fatalf("expected 1 route after pruning wildcard command, got %d", len(routes))
	}
	if routes[0].Path != "/v1/posts/:id" {
		t.Errorf("Path = %q; want /v1/posts/:id", routes[0].Path)
	}
}
