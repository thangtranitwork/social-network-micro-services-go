package service

import (
	"context"
	"fmt"
	"time"

	"social-network-go/auth-service/model"
	red "social-network-go/auth-service/redis"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// Register service
// Java equivalent: RegisterServiceImpl
func (s *AuthService) Register(email, password, givenName, familyName, birthdate, clientIP string) (*model.VerifyCode, error) {
	ctx := context.Background()
	existingAccount, err := s.AccountRepo.FindByEmail(ctx, email)
	if err == nil {
		if existingAccount.IsVerified {
			return nil, exception.NewAppException(exception.AccountAlreadyExists)
		}
		return nil, exception.NewAppException(exception.EmailNotVerified)
	}

	// 1. Check IP rate limit before sending email
	if err := s.checkEmailRateLimit(ctx, clientIP); err != nil {
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	accountID := uuid.New()
	code := uuid.New()
	verifyCode := model.VerifyCode{
		Code:       code,
		AccountID:  accountID,
		Verified:   false,
		ExpiryTime: time.Now().Add(s.Cfg.VerifyEmailDuration),
	}
	account := model.Account{
		ID:         accountID,
		Email:      email,
		Password:   string(hashedPassword),
		Role:       "USER",
		IsVerified: false,
		CreatedAt:  time.Now(),
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	txAccountRepo := s.AccountRepo.WithTx(tx)

	if err := txAccountRepo.Create(ctx, &account); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	// 2. Save verify code to Redis
	if red.RedisClient != nil {
		key := fmt.Sprintf("verify_code:%s", accountID.String())
		err = red.RedisClient.Set(ctx, key, code.String(), s.Cfg.VerifyEmailDuration).Err()
		if err != nil {
			logger.Field("account_id", accountID).Field("error", err).Warn("Failed to save verify code to Redis")
		}
	}

	verificationLink := fmt.Sprintf("%s/register?email=%s&code=%s", s.Cfg.FrontendURL, email, code.String())
	go s.EmailSender.SendHTMLEmail(email, "Email Verification", s.EmailSender.GetRegisterEmailHTML(verificationLink))

	_, err = s.UserClient.CreateUser(ctx, &pb.CreateUserRequest{
		UserId:     accountID.String(),
		Email:      email,
		GivenName:  givenName,
		FamilyName: familyName,
		Birthdate:  birthdate,
	})
	if err != nil {
		logger.Field("account_id", accountID).Field("error", err).Warn("Failed to create User node in user-service")
	}

	return &verifyCode, nil
}

func (s *AuthService) Verify(email string, code uuid.UUID) error {
	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}
	if account.IsVerified {
		return exception.NewAppException(exception.AccountVerified)
	}

	// Look up verify code in Redis
	if red.RedisClient == nil {
		return exception.NewAppException(exception.UnknownError)
	}

	key := fmt.Sprintf("verify_code:%s", account.ID.String())
	cachedCodeStr, err := red.RedisClient.Get(ctx, key).Result()
	lookupErr := err
	if err == redis.Nil {
		lookupErr = nil
	}
	profiler.TrackCacheLookup("auth-service:cache verifyCode", err == nil && cachedCodeStr != "", lookupErr)
	if err != nil {
		if err == redis.Nil {
			return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
		}
		return err
	}

	if cachedCodeStr != code.String() {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	txAccountRepo := s.AccountRepo.WithTx(tx)

	if err := txAccountRepo.UpdateVerificationStatus(ctx, account.ID, true); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Delete from Redis since verification is successful
	_ = red.RedisClient.Del(ctx, key).Err()

	return nil
}

func (s *AuthService) ResendEmail(email, clientIP string) (*model.VerifyCode, error) {
	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, exception.NewAppException(exception.AccountNotFound)
	}
	if account.IsVerified {
		return nil, exception.NewAppException(exception.AccountVerified)
	}

	// 1. Check IP rate limit before sending email
	if err := s.checkEmailRateLimit(ctx, clientIP); err != nil {
		return nil, err
	}

	if red.RedisClient == nil {
		return nil, exception.NewAppException(exception.UnknownError)
	}

	key := fmt.Sprintf("verify_code:%s", account.ID.String())
	var code uuid.UUID

	cachedCodeStr, err := red.RedisClient.Get(ctx, key).Result()
	lookupErr := err
	if err == redis.Nil {
		lookupErr = nil
	}
	profiler.TrackCacheLookup("auth-service:cache verifyCode", err == nil && cachedCodeStr != "", lookupErr)
	if err == nil {
		// Key still exists, extend TTL and keep same code
		code, err = uuid.Parse(cachedCodeStr)
		if err != nil {
			code = uuid.New()
			_ = red.RedisClient.Set(ctx, key, code.String(), s.Cfg.VerifyEmailDuration).Err()
		} else {
			_ = red.RedisClient.Expire(ctx, key, s.Cfg.VerifyEmailDuration).Err()
		}
	} else {
		// Key expired or not found, generate new code
		code = uuid.New()
		_ = red.RedisClient.Set(ctx, key, code.String(), s.Cfg.VerifyEmailDuration).Err()
	}

	verifyCode := model.VerifyCode{
		Code:       code,
		AccountID:  account.ID,
		Verified:   false,
		ExpiryTime: time.Now().Add(s.Cfg.VerifyEmailDuration),
	}

	verificationLink := fmt.Sprintf("%s/register?email=%s&code=%s", s.Cfg.FrontendURL, email, code.String())
	go s.EmailSender.SendHTMLEmail(email, "Email Verification", s.EmailSender.GetRegisterEmailHTML(verificationLink))

	return &verifyCode, nil
}
