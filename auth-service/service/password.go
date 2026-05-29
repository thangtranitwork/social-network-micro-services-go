package service

import (
	"context"
	"fmt"

	red "social-network-go/auth-service/redis"
	"social-network-go/exception"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/bcrypt"
)

// Update password service
// Java equivalent: UpdatePasswordServiceImpl
// Flow: send -> verify -> updatePassword
func (s *AuthService) ForgotPassword(email, clientIP string) error {
	return s.ForgotPasswordWithContinueURL(email, s.Cfg.FrontendURL+"/reset-password", clientIP)
}

func (s *AuthService) ForgotPasswordWithContinueURL(email, continueURL, clientIP string) error {
	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	// 1. Check IP rate limit before sending email
	if err := s.checkEmailRateLimit(ctx, clientIP); err != nil {
		return err
	}

	if red.RedisClient == nil {
		return exception.NewAppException(exception.UnknownError)
	}

	// Retrieve reset code if one exists in Redis
	userCodeKey := fmt.Sprintf("reset_password_code:%s", account.ID.String())
	var code uuid.UUID

	cachedCodeStr, err := red.RedisClient.Get(ctx, userCodeKey).Result()
	if err == nil {
		// Key still exists, extend TTL and keep same code
		code, err = uuid.Parse(cachedCodeStr)
		if err != nil {
			code = uuid.New()
			_ = red.RedisClient.Set(ctx, userCodeKey, code.String(), s.Cfg.PasswordResetDuration).Err()
			tokenKey := fmt.Sprintf("reset_password_token:%s", code.String())
			_ = red.RedisClient.Set(ctx, tokenKey, account.ID.String(), s.Cfg.PasswordResetDuration).Err()
		} else {
			_ = red.RedisClient.Expire(ctx, userCodeKey, s.Cfg.PasswordResetDuration).Err()
			tokenKey := fmt.Sprintf("reset_password_token:%s", code.String())
			_ = red.RedisClient.Expire(ctx, tokenKey, s.Cfg.PasswordResetDuration).Err()
		}
	} else {
		// Key expired or not found, generate new code
		code = uuid.New()
		_ = red.RedisClient.Set(ctx, userCodeKey, code.String(), s.Cfg.PasswordResetDuration).Err()
		tokenKey := fmt.Sprintf("reset_password_token:%s", code.String())
		_ = red.RedisClient.Set(ctx, tokenKey, account.ID.String(), s.Cfg.PasswordResetDuration).Err()
	}

	resetLink := fmt.Sprintf("%s?email=%s&code=%s", continueURL, email, code.String())
	go s.EmailSender.SendHTMLEmail(email, "Update Password Verification", s.EmailSender.GetResetPasswordEmailHTML(resetLink))

	return nil
}

func (s *AuthService) VerifyResetPassword(email, codeStr string) error {
	code, err := uuid.Parse(codeStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}

	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	if red.RedisClient == nil {
		return exception.NewAppException(exception.UnknownError)
	}

	// 1. Get the account ID for this token from Redis
	tokenKey := fmt.Sprintf("reset_password_token:%s", code.String())
	accountIDStr, err := red.RedisClient.Get(ctx, tokenKey).Result()
	if err != nil {
		if err == redis.Nil {
			return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
		}
		return err
	}

	if accountIDStr != account.ID.String() {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	// 2. Set reset verified key in Redis for short period
	ttl := s.Cfg.PasswordResetDuration
	err = red.RedisClient.Set(ctx, resetVerifiedKey(email, code), "1", ttl).Err()
	if err != nil {
		return err
	}

	// 3. Remove the token and user code from Redis so it cannot be used again to verify
	userCodeKey := fmt.Sprintf("reset_password_code:%s", account.ID.String())
	_ = red.RedisClient.Del(ctx, tokenKey, userCodeKey).Err()

	return nil
}

func (s *AuthService) UpdatePassword(email, codeStr, newPassword string) error {
	code, err := uuid.Parse(codeStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}

	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	if red.RedisClient == nil {
		return exception.NewAppException(exception.UnknownError)
	}

	// Verify that the code was successfully verified previously (resetVerifiedKey should exist)
	exists, err := red.RedisClient.Exists(ctx, resetVerifiedKey(email, code)).Result()
	if err != nil {
		return err
	}
	if exists == 0 {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	txAccountRepo := s.AccountRepo.WithTx(tx)

	if err := txAccountRepo.UpdatePassword(ctx, account.ID, string(hashedPassword)); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Clean up verified key from Redis
	_ = red.RedisClient.Del(ctx, resetVerifiedKey(email, code)).Err()

	return nil
}

// Backward-compatible old API: reset directly by code.
// Prefer Java-like flow: ForgotPassword -> VerifyResetPassword -> UpdatePassword.
func (s *AuthService) ResetPassword(codeStr, newPassword string) error {
	code, err := uuid.Parse(codeStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}

	ctx := context.Background()
	if red.RedisClient == nil {
		return exception.NewAppException(exception.UnknownError)
	}

	// Get account ID from token
	tokenKey := fmt.Sprintf("reset_password_token:%s", code.String())
	accountIDStr, err := red.RedisClient.Get(ctx, tokenKey).Result()
	if err != nil {
		if err == redis.Nil {
			return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
		}
		return err
	}

	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	txAccountRepo := s.AccountRepo.WithTx(tx)

	if err := txAccountRepo.UpdatePassword(ctx, accountID, string(hashedPassword)); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	// Delete from Redis
	userCodeKey := fmt.Sprintf("reset_password_code:%s", accountID.String())
	_ = red.RedisClient.Del(ctx, tokenKey, userCodeKey).Err()

	return nil
}

func (s *AuthService) ChangePassword(accountID uuid.UUID, oldPassword, newPassword string) error {
	ctx := context.Background()
	account, err := s.AccountRepo.FindByID(ctx, accountID)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(oldPassword)); err != nil {
		return exception.NewAppException(exception.InvalidPassword)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.AccountRepo.UpdatePassword(ctx, accountID, string(hashedPassword)); err != nil {
		return err
	}

	return nil
}
