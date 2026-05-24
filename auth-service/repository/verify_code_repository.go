package repository

import (
	"context"

	"social-network-go/auth-service/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type VerifyCodeRepository interface {
	WithTx(tx *gorm.DB) VerifyCodeRepository
	Create(ctx context.Context, verifyCode *model.VerifyCode) error
	FindByAccountIDAndCode(ctx context.Context, accountID, code uuid.UUID) (*model.VerifyCode, error)
	FindLatestUnverifiedByAccountID(ctx context.Context, accountID uuid.UUID) (*model.VerifyCode, error)
	Save(ctx context.Context, verifyCode *model.VerifyCode) error
	DeleteByAccountID(ctx context.Context, accountID uuid.UUID) error
}

type gormVerifyCodeRepository struct {
	db *gorm.DB
}

func NewVerifyCodeRepository(db *gorm.DB) VerifyCodeRepository {
	return &gormVerifyCodeRepository{db: db}
}

func (r *gormVerifyCodeRepository) WithTx(tx *gorm.DB) VerifyCodeRepository {
	return &gormVerifyCodeRepository{db: tx}
}

func (r *gormVerifyCodeRepository) Create(ctx context.Context, verifyCode *model.VerifyCode) error {
	return r.db.WithContext(ctx).Create(verifyCode).Error
}

func (r *gormVerifyCodeRepository) FindByAccountIDAndCode(ctx context.Context, accountID, code uuid.UUID) (*model.VerifyCode, error) {
	var verifyCode model.VerifyCode
	err := r.db.WithContext(ctx).Where("account_id = ? AND code = ?", accountID, code).First(&verifyCode).Error
	if err != nil {
		return nil, err
	}
	return &verifyCode, nil
}

func (r *gormVerifyCodeRepository) FindLatestUnverifiedByAccountID(ctx context.Context, accountID uuid.UUID) (*model.VerifyCode, error) {
	var verifyCode model.VerifyCode
	err := r.db.WithContext(ctx).Where("account_id = ? AND verified = ?", accountID, false).
		Order("expiry_time DESC").
		First(&verifyCode).Error
	if err != nil {
		return nil, err
	}
	return &verifyCode, nil
}

func (r *gormVerifyCodeRepository) Save(ctx context.Context, verifyCode *model.VerifyCode) error {
	return r.db.WithContext(ctx).Save(verifyCode).Error
}

func (r *gormVerifyCodeRepository) DeleteByAccountID(ctx context.Context, accountID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("account_id = ?", accountID).Delete(&model.VerifyCode{}).Error
}
