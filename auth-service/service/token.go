package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"social-network-go/auth-service/config"
	red "social-network-go/auth-service/redis"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
)

type TokenClaims struct {
	UserId   string `json:"user_id"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	Scope    string `json:"scope"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

type TokenManager interface {
	GenerateToken(userId, email, role string, secret string, duration time.Duration) (string, error)
	ValidateToken(tokenStr string) (*TokenClaims, error)
}

type JWTTokenManager struct {
	Cfg        *config.Config
	UserClient pb.UserServiceClient
}

func NewJWTTokenManager(cfg *config.Config, userClient pb.UserServiceClient) TokenManager {
	return &JWTTokenManager{
		Cfg:        cfg,
		UserClient: userClient,
	}
}

func (m *JWTTokenManager) GenerateToken(userId, email, role string, secret string, duration time.Duration) (string, error) {
	username := ""
	cacheKey := fmt.Sprintf("user_info:%s", userId)

	if red.RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		cachedUsername, err := profiler.TrackResult("auth-service:cache token.username.get", func() (string, error) {
			return red.RedisClient.Get(ctx, cacheKey).Result()
		})
		cancel()
		lookupErr := err
		if errors.Is(err, redis.Nil) {
			lookupErr = nil
		}
		profiler.TrackCacheLookup("auth-service:cache token.username", err == nil && cachedUsername != "", lookupErr)
		if err == nil && cachedUsername != "" {
			username = cachedUsername
		}
	}

	if username == "" && m.UserClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		resp, err := profiler.TrackResult("auth-service:grpc token.getUserProfile", func() (*pb.UserCommonInfoResponse, error) {
			return m.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{UserId: userId})
		})
		if err == nil && resp != nil && resp.Username != "" {
			username = resp.Username
			if red.RedisClient != nil {
				_, _ = profiler.TrackResult("auth-service:cache token.username.set", func() (struct{}, error) {
					return struct{}{}, red.RedisClient.Set(context.Background(), cacheKey, username, 24*time.Hour).Err()
				})
			}
		} else {
			logger.Field("userId", userId).Field("error", err).Error("Failed to get username for user")
		}
	}

	claims := TokenClaims{
		UserId:   userId,
		Email:    email,
		Role:     role,
		Scope:    role,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Subject:   userId,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return profiler.TrackResult("auth-service:crypto token.sign", func() (string, error) {
		return token.SignedString([]byte(secret))
	})
}

func (m *JWTTokenManager) ValidateToken(tokenStr string) (*TokenClaims, error) {
	claims := &TokenClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return []byte(m.Cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, exception.NewAppException(exception.InvalidToken)
	}
	return claims, nil
}
