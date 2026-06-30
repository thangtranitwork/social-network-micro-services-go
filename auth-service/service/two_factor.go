package service

import (
	"context"
	"github.com/google/uuid"
	"github.com/pquerna/otp/totp"
	"social-network-go/exception"
	"time"
)

func (s *AuthService) Generate2FA(userID uuid.UUID, email string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "SocialNetwork",
		AccountName: email,
	})
	if err != nil {
		return "", "", exception.NewAppException(exception.NewAppError(500, "2FA_GENERATE_ERROR", "Could not generate 2FA secret"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account, err := s.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return "", "", exception.NewAppException(exception.AccountNotFound)
	}

	account.TwoFactorSecret = key.Secret()
	err = s.AccountRepo.Update(ctx, account)
	if err != nil {
		return "", "", exception.NewAppException(exception.NewAppError(500, "DB_ERROR", "Failed to update account"))
	}

	return key.Secret(), key.URL(), nil
}

func (s *AuthService) Verify2FA(userID uuid.UUID, code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account, err := s.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	valid := totp.Validate(code, account.TwoFactorSecret)
	if !valid {
		return exception.NewAppException(exception.NewAppError(400, "INVALID_2FA_CODE", "Invalid 2FA code"))
	}

	account.IsTwoFactorEnabled = true
	err = s.AccountRepo.Update(ctx, account)
	if err != nil {
		return exception.NewAppException(exception.NewAppError(500, "DB_ERROR", "Failed to update account"))
	}

	return nil
}

func (s *AuthService) Disable2FA(userID uuid.UUID, code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account, err := s.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return exception.NewAppException(exception.AccountNotFound)
	}

	if account.IsTwoFactorEnabled {
		valid := totp.Validate(code, account.TwoFactorSecret)
		if !valid {
			return exception.NewAppException(exception.NewAppError(400, "INVALID_2FA_CODE", "Invalid 2FA code"))
		}
	}

	account.IsTwoFactorEnabled = false
	account.TwoFactorSecret = ""
	err = s.AccountRepo.Update(ctx, account)
	if err != nil {
		return exception.NewAppException(exception.NewAppError(500, "DB_ERROR", "Failed to update account"))
	}

	return nil
}

func (s *AuthService) Get2FAStatus(userID uuid.UUID) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	account, err := s.AccountRepo.FindByID(ctx, userID)
	if err != nil {
		return false, exception.NewAppException(exception.AccountNotFound)
	}

	return account.IsTwoFactorEnabled, nil
}
