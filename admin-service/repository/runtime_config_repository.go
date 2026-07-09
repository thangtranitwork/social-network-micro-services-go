package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"social-network-go/admin-service/model"
)

var (
	ErrRuntimeConfigNotFound        = errors.New("runtime config not found")
	ErrRuntimeConfigVersionConflict = errors.New("runtime config version conflict")
	ErrRuntimeConfigNotEditable     = errors.New("runtime config is not editable")
	ErrRuntimeConfigAlreadyExists   = errors.New("runtime config already exists")
)

type RuntimeConfigListFilter struct {
	Scope    string
	Category string
	Query    string
}

type RuntimeConfigRepository interface {
	List(ctx context.Context, filter RuntimeConfigListFilter) ([]model.RuntimeConfig, error)
	Get(ctx context.Context, key string) (*model.RuntimeConfig, error)
	SeedDefaults(ctx context.Context, configs []model.RuntimeConfig) error
	Create(ctx context.Context, config model.RuntimeConfig, updatedBy, reason string) (*model.RuntimeConfig, error)
	UpdateValue(ctx context.Context, key, value string, expectedVersion int64, updatedBy, reason string) (*model.RuntimeConfig, error)
	ResetValue(ctx context.Context, key string, updatedBy, reason string) (*model.RuntimeConfig, error)
}

type GormRuntimeConfigRepository struct {
	db *gorm.DB
}

func NewRuntimeConfigRepository(db *gorm.DB) RuntimeConfigRepository {
	if db == nil {
		return nil
	}
	return &GormRuntimeConfigRepository{db: db}
}

func (r *GormRuntimeConfigRepository) List(ctx context.Context, filter RuntimeConfigListFilter) ([]model.RuntimeConfig, error) {
	query := r.db.WithContext(ctx).Model(&model.RuntimeConfig{})
	if filter.Scope = strings.TrimSpace(filter.Scope); filter.Scope != "" {
		query = query.Where("scope = ?", filter.Scope)
	}
	if filter.Category = strings.TrimSpace(filter.Category); filter.Category != "" {
		query = query.Where("category = ?", filter.Category)
	}
	if filter.Query = strings.TrimSpace(filter.Query); filter.Query != "" {
		like := "%" + strings.ToLower(filter.Query) + "%"
		query = query.Where("LOWER(key) LIKE ? OR LOWER(description) LIKE ?", like, like)
	}

	var configs []model.RuntimeConfig
	if err := query.Order("scope ASC, category ASC, key ASC").Find(&configs).Error; err != nil {
		return nil, err
	}
	return configs, nil
}

func (r *GormRuntimeConfigRepository) Get(ctx context.Context, key string) (*model.RuntimeConfig, error) {
	var config model.RuntimeConfig
	err := r.db.WithContext(ctx).First(&config, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrRuntimeConfigNotFound
	}
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *GormRuntimeConfigRepository) SeedDefaults(ctx context.Context, configs []model.RuntimeConfig) error {
	if len(configs) == 0 {
		return nil
	}
	now := time.Now()
	for i := range configs {
		if configs[i].CreatedAt.IsZero() {
			configs[i].CreatedAt = now
		}
		if configs[i].UpdatedAt.IsZero() {
			configs[i].UpdatedAt = now
		}
		if configs[i].Version == 0 {
			configs[i].Version = 1
		}
		configs[i].IsEditable = true
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoNothing: true,
	}).Create(&configs).Error
}

func (r *GormRuntimeConfigRepository) Create(ctx context.Context, config model.RuntimeConfig, updatedBy, reason string) (*model.RuntimeConfig, error) {
	now := time.Now()
	config.CreatedAt = now
	config.UpdatedAt = now
	config.UpdatedBy = updatedBy
	config.Version = 1
	config.IsEditable = true

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&model.RuntimeConfig{}).Where("key = ?", config.Key).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRuntimeConfigAlreadyExists
		}
		if err := tx.Create(&config).Error; err != nil {
			return err
		}
		audit := model.RuntimeConfigAudit{
			ID:        uuid.NewString(),
			Key:       config.Key,
			NewValue:  config.Value,
			Version:   config.Version,
			UpdatedBy: updatedBy,
			Reason:    reason,
			CreatedAt: now,
		}
		return tx.Create(&audit).Error
	})
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *GormRuntimeConfigRepository) UpdateValue(ctx context.Context, key, value string, expectedVersion int64, updatedBy, reason string) (*model.RuntimeConfig, error) {
	return r.update(ctx, key, value, expectedVersion, updatedBy, reason, false)
}

func (r *GormRuntimeConfigRepository) ResetValue(ctx context.Context, key string, updatedBy, reason string) (*model.RuntimeConfig, error) {
	return r.update(ctx, key, "", 0, updatedBy, reason, true)
}

func (r *GormRuntimeConfigRepository) update(ctx context.Context, key, value string, expectedVersion int64, updatedBy, reason string, reset bool) (*model.RuntimeConfig, error) {
	var updated model.RuntimeConfig
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var config model.RuntimeConfig
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&config, "key = ?", key).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrRuntimeConfigNotFound
		}
		if err != nil {
			return err
		}
		if !config.IsEditable {
			return ErrRuntimeConfigNotEditable
		}
		if !reset && expectedVersion != config.Version {
			return ErrRuntimeConfigVersionConflict
		}

		oldValue := config.Value
		if reset {
			value = config.DefaultValue
		}
		config.Value = value
		config.Version++
		config.UpdatedBy = updatedBy
		config.UpdatedAt = time.Now()

		if err := tx.Save(&config).Error; err != nil {
			return err
		}
		audit := model.RuntimeConfigAudit{
			ID:        uuid.NewString(),
			Key:       config.Key,
			OldValue:  oldValue,
			NewValue:  config.Value,
			Version:   config.Version,
			UpdatedBy: updatedBy,
			Reason:    reason,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return err
		}
		updated = config
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}
