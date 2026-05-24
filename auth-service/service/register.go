package service

import (
	"context"
	"fmt"
	"time"

	"social-network-go/auth-service/model"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/pb"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// Register service
// Java equivalent: RegisterServiceImpl
func (s *AuthService) Register(email, password, givenName, familyName, birthdate string) (*model.VerifyCode, error) {
	ctx := context.Background()
	existingAccount, err := s.AccountRepo.FindByEmail(ctx, email)
	if err == nil {
		if existingAccount.IsVerified {
			return nil, exception.NewAppException(exception.AccountAlreadyExists)
		}
		return nil, exception.NewAppException(exception.EmailNotVerified)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	accountID := uuid.New()
	verifyCode := model.VerifyCode{
		Code:       uuid.New(),
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
	txVerifyCodeRepo := s.VerifyCodeRepo.WithTx(tx)

	if err := txAccountRepo.Create(ctx, &account); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := txVerifyCodeRepo.Create(ctx, &verifyCode); err != nil {
		tx.Rollback()
		return nil, err
	}
	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	verificationLink := fmt.Sprintf("%s/register?email=%s&code=%s", s.Cfg.FrontendURL, email, verifyCode.Code.String())
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

	verifyCode, err := s.VerifyCodeRepo.FindByAccountIDAndCode(ctx, account.ID, code)
	if err != nil {
		return exception.NewAppException(exception.VerificationCodeNotFound)
	}
	if verifyCode.Code != code { // defensive; normally Code == code is enough
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}
	if time.Now().After(verifyCode.ExpiryTime) {
		return exception.NewAppException(exception.VerificationCodeNotMatchedOrExpired)
	}

	tx := s.DB.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	txAccountRepo := s.AccountRepo.WithTx(tx)
	txVerifyCodeRepo := s.VerifyCodeRepo.WithTx(tx)

	if err := txAccountRepo.UpdateVerificationStatus(ctx, account.ID, true); err != nil {
		tx.Rollback()
		return err
	}
	if err := txVerifyCodeRepo.DeleteByAccountID(ctx, account.ID); err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}

	return nil
}

func (s *AuthService) ResendEmail(email string) (*model.VerifyCode, error) {
	ctx := context.Background()
	account, err := s.AccountRepo.FindByEmail(ctx, email)
	if err != nil {
		return nil, exception.NewAppException(exception.AccountNotFound)
	}
	if account.IsVerified {
		return nil, exception.NewAppException(exception.AccountVerified)
	}

	verifyCode, err := s.VerifyCodeRepo.FindLatestUnverifiedByAccountID(ctx, account.ID)
	if err != nil {
		return nil, exception.NewAppException(exception.VerificationCodeNotFound)
	}

	verifyCode.ExpiryTime = time.Now().Add(s.Cfg.VerifyEmailDuration)
	if err := s.VerifyCodeRepo.Save(ctx, verifyCode); err != nil {
		return nil, err
	}

	verificationLink := fmt.Sprintf("%s/register?email=%s&code=%s", s.Cfg.FrontendURL, email, verifyCode.Code.String())
	go s.EmailSender.SendHTMLEmail(email, "Email Verification", s.EmailSender.GetRegisterEmailHTML(verificationLink))

	return verifyCode, nil
}
