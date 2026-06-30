package repository

import (
	"context"
	"errors"
	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"

	"gorm.io/gorm"
)

func (r *Neo4jAdminRepository) CreateAdCampaign(ctx context.Context, campaign *model.AdCampaign) error {
	if db.PostgresDB == nil {
		return errors.New("database connection not available")
	}
	return db.PostgresDB.WithContext(ctx).Create(campaign).Error
}

func (r *Neo4jAdminRepository) GetAdCampaigns(ctx context.Context, advertiserID string) ([]model.AdCampaign, error) {
	if db.PostgresDB == nil {
		return nil, errors.New("database connection not available")
	}
	var campaigns []model.AdCampaign
	err := db.PostgresDB.WithContext(ctx).
		Where("advertiser_id = ?", advertiserID).
		Order("created_at DESC").
		Find(&campaigns).Error
	return campaigns, err
}

func (r *Neo4jAdminRepository) GetAdCampaignByID(ctx context.Context, campaignID string) (*model.AdCampaign, error) {
	if db.PostgresDB == nil {
		return nil, errors.New("database connection not available")
	}
	var campaign model.AdCampaign
	err := db.PostgresDB.WithContext(ctx).
		Where("id = ?", campaignID).
		First(&campaign).Error
	if err != nil {
		return nil, err
	}
	return &campaign, nil
}

func (r *Neo4jAdminRepository) UpdateAdCampaignStatus(ctx context.Context, campaignID string, status string) error {
	if db.PostgresDB == nil {
		return errors.New("database connection not available")
	}
	return db.PostgresDB.WithContext(ctx).
		Model(&model.AdCampaign{}).
		Where("id = ?", campaignID).
		Update("status", status).Error
}

func (r *Neo4jAdminRepository) GetPendingAdCampaigns(ctx context.Context) ([]model.AdCampaign, error) {
	if db.PostgresDB == nil {
		return nil, errors.New("database connection not available")
	}
	var campaigns []model.AdCampaign
	err := db.PostgresDB.WithContext(ctx).
		Where("status = ?", "PENDING").
		Order("created_at ASC").
		Find(&campaigns).Error
	return campaigns, err
}

func (r *Neo4jAdminRepository) GetActiveAdCampaigns(ctx context.Context) ([]model.AdCampaign, error) {
	if db.PostgresDB == nil {
		return nil, errors.New("database connection not available")
	}
	var campaigns []model.AdCampaign
	err := db.PostgresDB.WithContext(ctx).
		Where("status = ? AND budget_spent < budget_total AND start_date <= NOW() AND end_date >= NOW()", "ACTIVE").
		Find(&campaigns).Error
	return campaigns, err
}

func (r *Neo4jAdminRepository) LogAdInteraction(ctx context.Context, interaction *model.AdInteraction) error {
	if db.PostgresDB == nil {
		return errors.New("database connection not available")
	}

	return db.PostgresDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Create interaction record
		if err := tx.Create(interaction).Error; err != nil {
			return err
		}

		// 2. Charge budget
		if interaction.Cost > 0 {
			err := tx.Model(&model.AdCampaign{}).
				Where("id = ?", interaction.CampaignID).
				UpdateColumn("budget_spent", gorm.Expr("budget_spent + ?", interaction.Cost)).Error
			if err != nil {
				return err
			}

			// 3. Check if total budget has been spent, if so auto-complete it
			var campaign model.AdCampaign
			if err := tx.Where("id = ?", interaction.CampaignID).First(&campaign).Error; err == nil {
				if campaign.BudgetSpent >= campaign.BudgetTotal {
					tx.Model(&model.AdCampaign{}).
						Where("id = ?", interaction.CampaignID).
						Update("status", "COMPLETED")
				}
			}
		}

		return nil
	})
}

func (r *Neo4jAdminRepository) GetAdStatistics(ctx context.Context) (map[string]interface{}, error) {
	if db.PostgresDB == nil {
		return nil, errors.New("database connection not available")
	}

	var stats struct {
		TotalCampaigns  int64   `gorm:"column:total_campaigns"`
		ActiveCampaigns int64   `gorm:"column:active_campaigns"`
		TotalRevenue    float64 `gorm:"column:total_revenue"`
	}

	err := db.PostgresDB.WithContext(ctx).
		Table("ad_campaigns").
		Select("COUNT(*) as total_campaigns, " +
			"COUNT(CASE WHEN status = 'ACTIVE' THEN 1 END) as active_campaigns, " +
			"COALESCE(SUM(budget_spent), 0) as total_revenue").
		Scan(&stats).Error

	if err != nil {
		return nil, err
	}

	var interactionStats struct {
		TotalViews  int64 `gorm:"column:total_views"`
		TotalClicks int64 `gorm:"column:total_clicks"`
	}

	err = db.PostgresDB.WithContext(ctx).
		Table("ad_interactions").
		Select("COUNT(CASE WHEN interaction_type = 'VIEW' THEN 1 END) as total_views, " +
			"COUNT(CASE WHEN interaction_type = 'CLICK' THEN 1 END) as total_clicks").
		Scan(&interactionStats).Error

	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"totalCampaigns":  stats.TotalCampaigns,
		"activeCampaigns": stats.ActiveCampaigns,
		"totalRevenue":    stats.TotalRevenue,
		"totalViews":      interactionStats.TotalViews,
		"totalClicks":     interactionStats.TotalClicks,
	}

	return result, nil
}
