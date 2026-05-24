package repository

import (
	"context"
	"time"

	"social-network-go/auth-service/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type AccountRepository interface {
	WithTx(tx *gorm.DB) AccountRepository
	FindByEmail(ctx context.Context, email string) (*model.Account, error)
	FindByEmailAndRole(ctx context.Context, email, role string) (*model.Account, error)
	FindByID(ctx context.Context, id uuid.UUID) (*model.Account, error)
	Create(ctx context.Context, account *model.Account) error
	Update(ctx context.Context, account *model.Account) error
	UpdateVerificationStatus(ctx context.Context, id uuid.UUID, verified bool) error
	UpdatePassword(ctx context.Context, id uuid.UUID, hashedPassword string) error
}

type gormAccountRepository struct {
	db *gorm.DB
}

func NewAccountRepository(db *gorm.DB) AccountRepository {
	return &gormAccountRepository{db: db}
}

func (r *gormAccountRepository) WithTx(tx *gorm.DB) AccountRepository {
	return &gormAccountRepository{db: tx}
}

func (r *gormAccountRepository) FindByEmail(ctx context.Context, email string) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).Where("email = ?", email).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *gormAccountRepository) FindByEmailAndRole(ctx context.Context, email, role string) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).Where("email = ? AND role = ?", email, role).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *gormAccountRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Account, error) {
	var account model.Account
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&account).Error
	if err != nil {
		return nil, err
	}
	return &account, nil
}

func (r *gormAccountRepository) Create(ctx context.Context, account *model.Account) error {
	if account.CreatedAt.IsZero() {
		account.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(account).Error
}

func (r *gormAccountRepository) Update(ctx context.Context, account *model.Account) error {
	return r.db.WithContext(ctx).Save(account).Error
}

func (r *gormAccountRepository) UpdateVerificationStatus(ctx context.Context, id uuid.UUID, verified bool) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).Update("is_verified", verified).Error
}

func (r *gormAccountRepository) UpdatePassword(ctx context.Context, id uuid.UUID, hashedPassword string) error {
	return r.db.WithContext(ctx).Model(&model.Account{}).Where("id = ?", id).Update("password", hashedPassword).Error
}
