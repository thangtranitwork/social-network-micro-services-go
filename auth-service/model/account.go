package model

import (
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email              string    `gorm:"type:varchar(255);uniqueIndex;not null"`
	Password           string    `gorm:"type:varchar(255);not null"`
	Role               string    `gorm:"type:varchar(50);default:'USER';not null"`
	IsVerified         bool      `gorm:"type:boolean;default:false;not null"`
	TwoFactorSecret    string    `gorm:"type:varchar(255);default:null"`
	IsTwoFactorEnabled bool      `gorm:"type:boolean;default:false"`
	CreatedAt          time.Time `gorm:"not null"`
}
