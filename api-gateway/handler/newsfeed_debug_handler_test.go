package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"social-network-go/api-gateway/config"
	"social-network-go/profiler"

	"github.com/gin-gonic/gin"
)

func TestNewsfeedScoreBreakdownHandlerForwardsProfilerToken(t *testing.T) {
	const token = "test-profiler-token"
	t.Setenv("PROFILER_ADMIN_TOKEN", token)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(profiler.AdminTokenHeader); got != token {
			t.Fatalf("expected profiler token %q, got %q", token, got)
		}
		if got := r.URL.Query().Get("userId"); got != "user-1" {
			t.Fatalf("expected userId to be forwarded, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"code":200,"body":[]}`))
	}))
	defer upstream.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/debug/newsfeed/score-breakdown", NewsfeedScoreBreakdownHandler(&config.Config{PostHttpAddr: upstream.URL}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/debug/newsfeed/score-breakdown?userId=user-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != `{"code":200,"body":[]}` {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}

	if os.Getenv("PROFILER_ADMIN_TOKEN") != token {
		t.Fatal("expected test env token to remain set during request")
	}
}
