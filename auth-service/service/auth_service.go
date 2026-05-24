package service

import (
	"time"

	"social-network-go/auth-service/config"
	"social-network-go/auth-service/repository"
	"social-network-go/pb"

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
