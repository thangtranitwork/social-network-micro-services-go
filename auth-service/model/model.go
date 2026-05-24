package model

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email      string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	Password   string    `gorm:"type:varchar(255);not null"`
	Role       string    `gorm:"type:varchar(50);default:'USER';not null"`
	IsVerified bool      `gorm:"type:boolean;default:false;not null"`
	CreatedAt  time.Time `gorm:"not null"`
}

type VerifyCode struct {
	Code       uuid.UUID `gorm:"type:uuid;primaryKey"`
	AccountID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Verified   bool      `gorm:"type:boolean;default:false;not null"`
	ExpiryTime time.Time `gorm:"not null"`
}

type PasswordResetToken struct {
	Code       uuid.UUID `gorm:"type:uuid;primaryKey"`
	AccountID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Used       bool      `gorm:"type:boolean;default:false;not null"`
	ExpiryTime time.Time `gorm:"not null"`
}
