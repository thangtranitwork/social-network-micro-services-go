package service

import (
	"context"
	"fmt"
	"time"

	"social-network-go/auth-service/model"
	red "social-network-go/auth-service/redis"
	"social-network-go/exception"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Update password service
// Java equivalent: UpdatePasswordServiceImpl
// Flow: send -> verify -> updatePassword
func (s *AuthService) ForgotPassword(email string) error {
	return s.ForgotPasswordWithContinueURL(email, s.Cfg.FrontendURL+"/reset-password")
}

func (s *AuthService) ForgotPasswordWithContinueURL(email, continueURL string) error {
	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	var resetToken *model.PasswordResetToken
	resetToken, err = s.ResetTokenRepo.FindLatestUnusedByAccountID(ctx, account.ID)
	if err != nil {
		// Token not found or error, create new one
		newResetToken := model.PasswordResetToken{
			Code:       uuid.New(),
			AccountID:  account.ID,
			Used:       false,
			ExpiryTime: time.Now().Add(s.Cfg.PasswordResetDuration),
		}
		if err := s.ResetTokenRepo.Create(ctx, &newResetToken); err != nil {
			return err
		}
		resetToken = &newResetToken
	} else {
		resetToken.ExpiryTime = time.Now().Add(s.Cfg.PasswordResetDuration)
		if err := s.ResetTokenRepo.Save(ctx, resetToken); err != nil {
			return err
		}
	}

	resetLink := fmt.Sprintf("%s?email=%s&code=%s", continueURL, email, resetToken.Code.String())
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

	resetToken, err := s.ResetTokenRepo.FindByAccountIDAndCode(ctx, account.ID, code)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if resetToken.Used || time.Now().After(resetToken.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	if red.RedisClient != nil {
		ttl := time.Until(resetToken.ExpiryTime)
		if ttl > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return red.RedisClient.Set(ctx, resetVerifiedKey(email, code), "1", ttl).Err()
		}
	}

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

	resetToken, err := s.ResetTokenRepo.FindByAccountIDAndCode(ctx, account.ID, code)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if resetToken.Used || time.Now().After(resetToken.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	if red.RedisClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		exists, err := red.RedisClient.Exists(ctx, resetVerifiedKey(email, code)).Result()
		if err != nil {
			return err
		}
		if exists == 0 {
			return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
		}
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
	txResetTokenRepo := s.ResetTokenRepo.WithTx(tx)

	if err := txAccountRepo.UpdatePassword(ctx, account.ID, string(hashedPassword)); err != nil {
		tx.Rollback()
		return err
	}
	if err := txResetTokenRepo.MarkAsUsed(ctx, code); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	if red.RedisClient != nil {
		_ = red.RedisClient.Del(context.Background(), resetVerifiedKey(email, code)).Err()
	}

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
	resetToken, err := s.ResetTokenRepo.FindByCode(ctx, code)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if resetToken.Used {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}
	if time.Now().After(resetToken.ExpiryTime) {
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
	txResetTokenRepo := s.ResetTokenRepo.WithTx(tx)

	if err := txAccountRepo.UpdatePassword(ctx, resetToken.AccountID, string(hashedPassword)); err != nil {
		tx.Rollback()
		return err
	}
	if err := txResetTokenRepo.MarkAsUsed(ctx, code); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit().Error
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
