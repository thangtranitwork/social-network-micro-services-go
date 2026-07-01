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

