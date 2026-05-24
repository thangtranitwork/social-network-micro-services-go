package model

import (
	"time"

	"github.com/google/uuid"
)

type PasswordResetToken struct {
	Code       uuid.UUID `gorm:"type:uuid;primaryKey"`
	AccountID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Used       bool      `gorm:"type:boolean;default:false;not null"`
	ExpiryTime time.Time `gorm:"not null"`
}
