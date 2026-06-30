package model

import (
	"time"

	"github.com/google/uuid"
)

type AdCampaign struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AdvertiserID uuid.UUID `gorm:"type:uuid;not null;index" json:"advertiserId"`
	Title        string    `gorm:"type:varchar(255);not null" json:"title"`
	Description  string    `gorm:"type:text" json:"description"`
	MediaURL     string    `gorm:"type:varchar(512)" json:"mediaUrl"`
	TargetURL    string    `gorm:"type:varchar(512);not null" json:"targetUrl"`
	AdType       string    `gorm:"type:varchar(50);default:'FEED_POST'" json:"adType"` // FEED_POST or SIDEBAR
	TargetGender string    `gorm:"type:varchar(10);default:'ALL'" json:"targetGender"` // ALL, MALE, FEMALE
	TargetMinAge int       `gorm:"type:int;default:0" json:"targetMinAge"`
	TargetMaxAge int       `gorm:"type:int;default:100" json:"targetMaxAge"`
	BudgetTotal  float64   `gorm:"type:decimal(15,2);not null" json:"budgetTotal"`
	BudgetSpent  float64   `gorm:"type:decimal(15,2);default:0.00" json:"budgetSpent"`
	BidType      string    `gorm:"type:varchar(10);default:'CPC'" json:"bidType"` // CPC or CPM
	BidAmount    float64   `gorm:"type:decimal(10,2);not null" json:"bidAmount"`
	StartDate    time.Time `gorm:"not null" json:"startDate"`
	EndDate      time.Time `gorm:"not null" json:"endDate"`
	Status       string    `gorm:"type:varchar(20);default:'PENDING';index" json:"status"` // PENDING, ACTIVE, PAUSED, REJECTED, COMPLETED
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

type AdInteraction struct {
	ID              uint64     `gorm:"primaryKey;autoIncrement" json:"id"`
	CampaignID      uuid.UUID  `gorm:"type:uuid;not null;index" json:"campaignId"`
	ViewerID        *uuid.UUID `gorm:"type:uuid" json:"viewerId"`                        // null if visitor/not-logged-in
	InteractionType string     `gorm:"type:varchar(10);not null" json:"interactionType"` // VIEW or CLICK
	Cost            float64    `gorm:"type:decimal(10,2);not null" json:"cost"`
	IPAddress       string     `gorm:"type:varchar(45)" json:"ipAddress"`
	UserAgent       string     `gorm:"type:varchar(512)" json:"userAgent"`
	CreatedAt       time.Time  `json:"createdAt"`
}
