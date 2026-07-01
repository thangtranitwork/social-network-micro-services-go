package service

import (
	"context"
	"fmt"
	"time"

	"social-network-go/auth-service/config"
	red "social-network-go/auth-service/redis"
	"social-network-go/auth-service/repository"
	"social-network-go/exception"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type AuthService struct {
	Cfg            *config.Config
	UserClient     pb.UserServiceClient
	EmailSender    EmailSender
	TokenMgr       TokenManager
	DB             *gorm.DB
	AccountRepo    repository.AccountRepository
	VerifyCodeRepo repository.VerifyCodeRepository
	ResetTokenRepo repository.PasswordResetTokenRepository
}

func (s *AuthService) checkEmailRateLimit(ctx context.Context, clientIP string) error {
	if clientIP == "" {
		return nil
	}
	if red.RedisClient == nil {
		return nil
	}

	key := fmt.Sprintf("email_rate_limit:%s", clientIP)
	count, err := red.RedisClient.Get(ctx, key).Int()
	lookupErr := err
	if err == redis.Nil {
		lookupErr = nil
	}
	profiler.TrackCacheLookup("auth-service:cache emailRateLimit", err == nil && count > 0, lookupErr)
	if err != nil && err != redis.Nil {
		return err
	}

	if count >= s.Cfg.EmailRateLimitCount {
		return exception.NewAppException(exception.TooManyEmailRequests)
	}

	pipe := red.RedisClient.TxPipeline()
	pipe.Incr(ctx, key)
	if count == 0 {
		pipe.Expire(ctx, key, s.Cfg.EmailRateLimitWindow)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func NewAuthService(cfg *config.Config, userClient pb.UserServiceClient, gormDB *gorm.DB) *AuthService {
	emailSender := NewSMTPEmailSender(cfg)
	tokenMgr := NewJWTTokenManager(cfg, userClient)

	accountRepo := repository.NewAccountRepository(gormDB)
	verifyCodeRepo := repository.NewVerifyCodeRepository(gormDB)
	resetTokenRepo := repository.NewPasswordResetTokenRepository(gormDB)

	return &AuthService{
		Cfg:            cfg,
		UserClient:     userClient,
		EmailSender:    emailSender,
		TokenMgr:       tokenMgr,
		DB:             gormDB,
		AccountRepo:    accountRepo,
		VerifyCodeRepo: verifyCodeRepo,
		ResetTokenRepo: resetTokenRepo,
	}
}

func (s *AuthService) ValidateToken(tokenStr string) (*TokenClaims, error) {
	return s.TokenMgr.ValidateToken(tokenStr)
}

func (s *AuthService) GenerateToken(userId, email, role string, secret string, duration time.Duration) (string, error) {
	return s.TokenMgr.GenerateToken(userId, email, role, secret, duration)
}

func (s *AuthService) GetRefreshTokenDuration() time.Duration {
	return s.Cfg.RefreshTokenDuration
}

func (s *AuthService) Close() error {
	return nil
}
