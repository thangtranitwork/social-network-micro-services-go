package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"social-network-go/pb"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

type mockAuthClient struct{}

func (mockAuthClient) ValidateToken(ctx context.Context, in *pb.TokenRequest, opts ...grpc.CallOption) (*pb.TokenResponse, error) {
	return &pb.TokenResponse{
		IsValid: true,
		UserId:  "user-1",
		Email:   "user@example.com",
		Role:    "USER",
	}, nil
}

func TestAuthRequiredRejectsQueryTokenForNormalHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/private", AuthRequired(mockAuthClient{}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/private?token=query-token", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestAuthRequiredAllowsQueryTokenForSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/logs/stream", AuthRequired(mockAuthClient{}), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/logs/stream?token=query-token", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}
