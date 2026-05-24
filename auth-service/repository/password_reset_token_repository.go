package repository

import (
	"context"

	"social-network-go/auth-service/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type PasswordResetTokenRepository interface {
	WithTx(tx *gorm.DB) PasswordResetTokenRepository
	Create(ctx context.Context, token *model.PasswordResetToken) error
	FindLatestUnusedByAccountID(ctx context.Context, accountID uuid.UUID) (*model.PasswordResetToken, error)
	FindByAccountIDAndCode(ctx context.Context, accountID, code uuid.UUID) (*model.PasswordResetToken, error)
	FindByCode(ctx context.Context, code uuid.UUID) (*model.PasswordResetToken, error)
	Save(ctx context.Context, token *model.PasswordResetToken) error
	MarkAsUsed(ctx context.Context, code uuid.UUID) error
}

type gormPasswordResetTokenRepository struct {
	db *gorm.DB
}

func NewPasswordResetTokenRepository(db *gorm.DB) PasswordResetTokenRepository {
	return &gormPasswordResetTokenRepository{db: db}
}

func (r *gormPasswordResetTokenRepository) WithTx(tx *gorm.DB) PasswordResetTokenRepository {
	return &gormPasswordResetTokenRepository{db: tx}
}

func (r *gormPasswordResetTokenRepository) Create(ctx context.Context, token *model.PasswordResetToken) error {
	return r.db.WithContext(ctx).Create(token).Error
}

func (r *gormPasswordResetTokenRepository) FindLatestUnusedByAccountID(ctx context.Context, accountID uuid.UUID) (*model.PasswordResetToken, error) {
	var token model.PasswordResetToken
	err := r.db.WithContext(ctx).Where("account_id = ? AND used = ?", accountID, false).
		Order("expiry_time DESC").
		First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *gormPasswordResetTokenRepository) FindByAccountIDAndCode(ctx context.Context, accountID, code uuid.UUID) (*model.PasswordResetToken, error) {
	var token model.PasswordResetToken
	err := r.db.WithContext(ctx).Where("account_id = ? AND code = ?", accountID, code).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *gormPasswordResetTokenRepository) FindByCode(ctx context.Context, code uuid.UUID) (*model.PasswordResetToken, error) {
	var token model.PasswordResetToken
	err := r.db.WithContext(ctx).Where("code = ?", code).First(&token).Error
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func (r *gormPasswordResetTokenRepository) Save(ctx context.Context, token *model.PasswordResetToken) error {
	return r.db.WithContext(ctx).Save(token).Error
}

func (r *gormPasswordResetTokenRepository) MarkAsUsed(ctx context.Context, code uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&model.PasswordResetToken{}).Where("code = ?", code).Update("used", true).Error
}
