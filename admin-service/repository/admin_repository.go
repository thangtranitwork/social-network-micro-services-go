package repository

import (
	"context"
	"strconv"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type AdminRepository interface {
	GetUsersStatistics(ctx context.Context) (*model.UserStatisticsResponse, error)
	GetPostsStatistics(ctx context.Context) (*model.PostStatisticsResponse, error)
	GetPostsList(ctx context.Context, skip, limit int) ([]model.PostResponse, error)
	GetUsersList(ctx context.Context, skip, limit int) ([]model.UserDetailResponse, error)

	QueryWeekUserStats(ctx context.Context, week, year int) (map[string]int, error)
	QueryMonthUserStats(ctx context.Context, month, year int) (map[string]int, error)
	QueryYearUserStats(ctx context.Context, year int) (map[string]int, error)

	QueryWeekPostStats(ctx context.Context, week, year int) (map[string]int, error)
	QueryMonthPostStats(ctx context.Context, month, year int) (map[string]int, error)
	QueryYearPostStats(ctx context.Context, year int) (map[string]int, error)

	DeletePost(ctx context.Context, postID string) error
	SuspendUser(ctx context.Context, userID string, duration time.Duration) error
	UnsuspendUser(ctx context.Context, userID string) error

	// Advertisement Repository Methods
	CreateAdCampaign(ctx context.Context, campaign *model.AdCampaign) error
	GetAdCampaigns(ctx context.Context, advertiserID string) ([]model.AdCampaign, error)
	GetAdCampaignByID(ctx context.Context, campaignID string) (*model.AdCampaign, error)
	UpdateAdCampaignStatus(ctx context.Context, campaignID string, status string) error
	GetPendingAdCampaigns(ctx context.Context) ([]model.AdCampaign, error)
	GetActiveAdCampaigns(ctx context.Context) ([]model.AdCampaign, error)
	LogAdInteraction(ctx context.Context, interaction *model.AdInteraction) error
	GetAdStatistics(ctx context.Context) (map[string]interface{}, error)
}

type Neo4jAdminRepository struct{}

func NewAdminRepository() AdminRepository {
	return &Neo4jAdminRepository{}
}

var dayOfWeekNames = []string{"", "MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"}
var monthNames = []string{"", "JANUARY", "FEBRUARY", "MARCH", "APRIL", "MAY", "JUNE", "JULY", "AUGUST", "SEPTEMBER", "OCTOBER", "NOVEMBER", "DECEMBER"}

func (r *Neo4jAdminRepository) GetUsersStatistics(ctx context.Context) (*model.UserStatisticsResponse, error) {
	if db.Neo4jDriver == nil {
		return &model.UserStatisticsResponse{
			ThisWeekStatistics:  map[string]int{},
			ThisMonthStatistics: map[string]int{},
			ThisYearStatistics:  map[string]int{},
		}, nil
	}

	var totalUsers int64
	var notVerifiedUsers int64
	if db.PostgresDB != nil {
		db.PostgresDB.Table("accounts").Count(&totalUsers)
		db.PostgresDB.Table("accounts").Where("is_verified = ?", false).Count(&notVerifiedUsers)
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User)
			WITH date() AS today, u
			RETURN count(u) AS totalUsers,
				   count(CASE WHEN date(u.createdAt) = today THEN 1 END) AS newUsersToday,
				   count(CASE WHEN u.createdAt.week = today.week AND u.createdAt.weekYear = today.weekYear THEN 1 END) AS newUsersThisWeek,
				   count(CASE WHEN u.createdAt.month = today.month AND u.createdAt.year = today.year THEN 1 END) AS newUsersThisMonth,
				   count(CASE WHEN u.createdAt.year = today.year THEN 1 END) AS newUsersThisYear
		`
		res, err := tx.Run(ctx, query, nil)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			vals := res.Record().Values
			stat := &model.UserStatisticsResponse{
				TotalUsers:        getIntVal(vals[0]),
				NotVerifiedUsers:  0,
				NewUsersToday:     getIntVal(vals[1]),
				NewUsersThisWeek:  getIntVal(vals[2]),
				NewUsersThisMonth: getIntVal(vals[3]),
				NewUsersThisYear:  getIntVal(vals[4]),
			}
			if db.PostgresDB != nil {
				stat.TotalUsers = int(totalUsers)
				stat.NotVerifiedUsers = int(notVerifiedUsers)
			}
			return stat, nil
		}
		return &model.UserStatisticsResponse{}, nil
	})
	if err != nil {
		return nil, err
	}
	return res.(*model.UserStatisticsResponse), nil
}

func (r *Neo4jAdminRepository) GetPostsStatistics(ctx context.Context) (*model.PostStatisticsResponse, error) {
	if db.Neo4jDriver == nil {
		return &model.PostStatisticsResponse{
			ThisWeekStatistics:  map[string]int{},
			ThisMonthStatistics: map[string]int{},
			ThisYearStatistics:  map[string]int{},
		}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (post:Post)
			WITH date() AS today, post
			OPTIONAL MATCH (post)-[attach:ATTACH_FILES]->(:File)
			RETURN count(DISTINCT post) AS totalPosts,
				   sum(post.likeCount) AS totalLikes,
				   sum(post.commentCount) AS totalComments,
				   count(attach) AS totalFiles,
				   sum(post.shareCount) AS totalShares,
				   count(CASE WHEN post.privacy = "PUBLIC" THEN 1 END) AS publicPostCount,
				   count(CASE WHEN post.privacy = "FRIEND" THEN 1 END) AS friendPostCount,
				   count(CASE WHEN post.privacy = "PRIVATE" THEN 1 END) AS privatePostCount,
				   count(CASE WHEN post.deletedAt IS NOT NULL THEN 1 END) AS deletedPostCount,
				   count(CASE WHEN date(post.createdAt) = today THEN 1 END) AS newPostsToday,
				   count(CASE WHEN post.createdAt.week = today.week AND post.createdAt.weekYear = today.weekYear THEN 1 END) AS newPostsThisWeek,
				   count(CASE WHEN post.createdAt.month = today.month AND post.createdAt.year = today.year THEN 1 END) AS newPostsThisMonth,
				   count(CASE WHEN post.createdAt.year = today.year THEN 1 END) AS newPostsThisYear
		`
		res, err := tx.Run(ctx, query, nil)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			vals := res.Record().Values
			getSafeInt := func(v interface{}) int {
				if v == nil {
					return 0
				}
				return int(v.(int64))
			}
			return &model.PostStatisticsResponse{
				TotalPosts:        getSafeInt(vals[0]),
				TotalLikes:        getSafeInt(vals[1]),
				TotalComments:     getSafeInt(vals[2]),
				TotalFiles:        getSafeInt(vals[3]),
				TotalShares:       getSafeInt(vals[4]),
				PublicPostCount:   getSafeInt(vals[5]),
				FriendPostCount:   getSafeInt(vals[6]),
				PrivatePostCount:  getSafeInt(vals[7]),
				DeletedPostCount:  getSafeInt(vals[8]),
				NewPostsToday:     getSafeInt(vals[9]),
				NewPostsThisWeek:  getSafeInt(vals[10]),
				NewPostsThisMonth: getSafeInt(vals[11]),
				NewPostsThisYear:  getSafeInt(vals[12]),
			}, nil
		}
		return &model.PostStatisticsResponse{}, nil
	})
	if err != nil {
		return nil, err
	}
	return res.(*model.PostStatisticsResponse), nil
}

func (r *Neo4jAdminRepository) GetPostsList(ctx context.Context, skip, limit int) ([]model.PostResponse, error) {
	if db.Neo4jDriver == nil {
		return []model.PostResponse{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (post:Post)<-[:POSTED]-(author:User)
			WHERE post.deletedAt IS NULL
			OPTIONAL MATCH (post)-[:ATTACH_FILES]->(f:File)
			OPTIONAL MATCH (author)-[:HAS_PROFILE_PICTURE]->(pf:File)
			RETURN post.id, post.content, post.privacy, post.createdAt, post.likeCount, post.commentCount, post.shareCount,
				   author.id, author.username, author.givenName, author.familyName, pf.id,
				   collect(f.id) AS files
			ORDER BY post.createdAt DESC
			SKIP $skip LIMIT $limit
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"skip":  int64(skip),
			"limit": int64(limit),
		})
		if err != nil {
			return nil, err
		}
		list := make([]model.PostResponse, 0)
		for res.Next(ctx) {
			vals := res.Record().Values

			createdAtTime := getTimeVal(vals[3])

			pfID := formatFileURL(vals[11])

			author := model.CreatorInfo{
				ID:                getStringVal(vals[7]),
				Username:          getStringVal(vals[8]),
				GivenName:         getStringVal(vals[9]),
				FamilyName:        getStringVal(vals[10]),
				ProfilePictureUrl: pfID,
			}

			var files []string
			if vals[12] != nil {
				rawFiles := vals[12].([]interface{})
				for _, f := range rawFiles {
					if url := formatFileURL(f); url != "" {
						files = append(files, url)
					}
				}
			}

			p := model.PostResponse{
				ID:           getStringVal(vals[0]),
				Content:      getStringVal(vals[1]),
				Privacy:      getStringVal(vals[2]),
				CreatedAt:    createdAtTime,
				LikeCount:    getIntVal(vals[4]),
				CommentCount: getIntVal(vals[5]),
				ShareCount:   getIntVal(vals[6]),
				Author:       author,
				User:         author,
				Files:        files,
			}
			list = append(list, p)
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return res.([]model.PostResponse), nil
}

func (r *Neo4jAdminRepository) GetUsersList(ctx context.Context, skip, limit int) ([]model.UserDetailResponse, error) {
	if db.Neo4jDriver == nil {
		return []model.UserDetailResponse{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User)
			WITH u
			ORDER BY u.createdAt DESC
			SKIP $skip LIMIT $limit
			
			OPTIONAL MATCH (u)-[:HAS_PROFILE_PICTURE]->(pf:File)
			WITH u, pf
			OPTIONAL MATCH (u)-[:FRIEND]-(friend:User)
			WITH u, pf, count(DISTINCT friend) as friendCount
			OPTIONAL MATCH (u)-[:POSTED]->(post:Post) WHERE post.deletedAt IS NULL
			WITH u, pf, friendCount, count(DISTINCT post) as postCount
			OPTIONAL MATCH (u)-[:COMMENTED]->(comment:Comment)
			WITH u, pf, friendCount, postCount, count(DISTINCT comment) as commentCount
			OPTIONAL MATCH (u)-[:SENT_FRIEND_REQUEST]->(reqSent:User)
			WITH u, pf, friendCount, postCount, commentCount, count(DISTINCT reqSent) as requestSentCount
			OPTIONAL MATCH (u)<-[:SENT_FRIEND_REQUEST]-(reqRecv:User)
			WITH u, pf, friendCount, postCount, commentCount, requestSentCount, count(DISTINCT reqRecv) as requestReceivedCount
			OPTIONAL MATCH (u)-[:BLOCK]->(blocked:User)
			RETURN u.id, u.givenName, u.familyName, u.username, u.bio, u.birthdate, u.createdAt,
				   friendCount, postCount, pf.id AS pfId,
				   commentCount, requestSentCount, requestReceivedCount, count(DISTINCT blocked) AS blockCount,
				   u.suspended, u.suspendedUntil
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"skip":  int64(skip),
			"limit": int64(limit),
		})
		if err != nil {
			return nil, err
		}
		list := make([]model.UserDetailResponse, 0)
		for res.Next(ctx) {
			vals := res.Record().Values

			regTime := getTimeVal(vals[6])

			pfID := formatFileURL(vals[9])

			birth := ""
			if t := getTimeVal(vals[5]); !t.IsZero() {
				birth = t.Format("2006-01-02")
			}

			suspendedVal := false
			if vals[14] != nil {
				suspendedVal = vals[14].(bool)
			}
			suspendedUntilVal := ""
			if vals[15] != nil {
				suspendedUntilVal = getStringVal(vals[15])
			}
			if suspendedVal && suspendedUntilVal != "" {
				if untilTime, err := time.Parse(time.RFC3339, suspendedUntilVal); err == nil {
					if time.Now().After(untilTime) {
						suspendedVal = false
					}
				}
			}

			u := model.UserDetailResponse{
				ID:                   getStringVal(vals[0]),
				GivenName:            getStringVal(vals[1]),
				FamilyName:           getStringVal(vals[2]),
				Username:             getStringVal(vals[3]),
				Email:                "",
				Bio:                  getStringVal(vals[4]),
				Birthdate:            birth,
				RegistrationDate:     regTime,
				FriendCount:          getIntVal(vals[7]),
				PostCount:            getIntVal(vals[8]),
				MessageCount:         0,
				CommentCount:         getIntVal(vals[10]),
				CallCount:            0,
				RequestSentCount:     getIntVal(vals[11]),
				RequestReceivedCount: getIntVal(vals[12]),
				UploadedFileCount:    0,
				BlockCount:           getIntVal(vals[13]),
				ProfilePictureUrl:    pfID,
				Verified:             false,
				Suspended:            suspendedVal,
				SuspendedUntil:       suspendedUntilVal,
			}
			list = append(list, u)
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}

	list := res.([]model.UserDetailResponse)

	// Fetch details from Postgres
	if len(list) > 0 && db.PostgresDB != nil {
		var userIDs []string
		for _, u := range list {
			userIDs = append(userIDs, u.ID)
		}

		type AccountMini struct {
			ID         string `gorm:"column:id"`
			Email      string `gorm:"column:email"`
			IsVerified bool   `gorm:"column:is_verified"`
		}
		var accounts []AccountMini
		if err := db.PostgresDB.Table("accounts").Where("id IN ?", userIDs).Find(&accounts).Error; err == nil {
			accMap := make(map[string]AccountMini)
			for _, acc := range accounts {
				accMap[acc.ID] = acc
			}
			for i := range list {
				if acc, exists := accMap[list[i].ID]; exists {
					list[i].Email = acc.Email
					list[i].Verified = acc.IsVerified
				}
			}
		}
	}

	return list, nil
}

func (r *Neo4jAdminRepository) QueryWeekUserStats(ctx context.Context, week, year int) (map[string]int, error) {
	stats := map[string]int{"MONDAY": 0, "TUESDAY": 0, "WEDNESDAY": 0, "THURSDAY": 0, "FRIDAY": 0, "SATURDAY": 0, "SUNDAY": 0}
	if db.Neo4jDriver == nil {
		return stats, nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (u:User) WHERE u.createdAt.week=$week AND u.createdAt.weekYear=$year
			 RETURN u.createdAt.dayOfWeek AS dayOfWeek, count(*) AS total`,
			map[string]interface{}{"week": int64(week), "year": int64(year)})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if d >= 1 && d <= 7 {
				stats[dayOfWeekNames[d]] = t
			}
		}
		return nil, nil
	})
	return stats, err
}

func (r *Neo4jAdminRepository) QueryMonthUserStats(ctx context.Context, month, year int) (map[string]int, error) {
	stats := make(map[string]int)
	for i := 1; i <= 31; i++ {
		stats[strconv.Itoa(i)] = 0
	}
	if db.Neo4jDriver == nil {
		return stats, nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (u:User) WHERE u.createdAt.year=$year AND u.createdAt.month=$month
			 RETURN u.createdAt.day AS day, count(*) AS total`,
			map[string]interface{}{"month": int64(month), "year": int64(year)})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			stats[strconv.Itoa(d)] = t
		}
		return nil, nil
	})
	return stats, err
}

func (r *Neo4jAdminRepository) QueryYearUserStats(ctx context.Context, year int) (map[string]int, error) {
	stats := map[string]int{"JANUARY": 0, "FEBRUARY": 0, "MARCH": 0, "APRIL": 0, "MAY": 0, "JUNE": 0, "JULY": 0, "AUGUST": 0, "SEPTEMBER": 0, "OCTOBER": 0, "NOVEMBER": 0, "DECEMBER": 0}
	if db.Neo4jDriver == nil {
		return stats, nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (u:User) WHERE u.createdAt.year=$year
			 RETURN u.createdAt.month AS month, count(*) AS total`,
			map[string]interface{}{"year": int64(year)})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			m := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if m >= 1 && m <= 12 {
				stats[monthNames[m]] = t
			}
		}
		return nil, nil
	})
	return stats, err
}

func (r *Neo4jAdminRepository) QueryWeekPostStats(ctx context.Context, week, year int) (map[string]int, error) {
	stats := map[string]int{"MONDAY": 0, "TUESDAY": 0, "WEDNESDAY": 0, "THURSDAY": 0, "FRIDAY": 0, "SATURDAY": 0, "SUNDAY": 0}
	if db.Neo4jDriver == nil {
		return stats, nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (post:Post) WHERE post.createdAt.week=$week AND post.createdAt.weekYear=$year
			 RETURN post.createdAt.dayOfWeek AS dayOfWeek, count(*) AS total`,
			map[string]interface{}{"week": int64(week), "year": int64(year)})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if d >= 1 && d <= 7 {
				stats[dayOfWeekNames[d]] = t
			}
		}
		return nil, nil
	})
	return stats, err
}

func (r *Neo4jAdminRepository) QueryMonthPostStats(ctx context.Context, month, year int) (map[string]int, error) {
	stats := make(map[string]int)
	for i := 1; i <= 31; i++ {
		stats[strconv.Itoa(i)] = 0
	}
	if db.Neo4jDriver == nil {
		return stats, nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (post:Post) WHERE post.createdAt.year=$year AND post.createdAt.month=$month
			 RETURN post.createdAt.day AS day, count(*) AS total`,
			map[string]interface{}{"month": int64(month), "year": int64(year)})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			d := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			stats[strconv.Itoa(d)] = t
		}
		return nil, nil
	})
	return stats, err
}

func (r *Neo4jAdminRepository) QueryYearPostStats(ctx context.Context, year int) (map[string]int, error) {
	stats := map[string]int{"JANUARY": 0, "FEBRUARY": 0, "MARCH": 0, "APRIL": 0, "MAY": 0, "JUNE": 0, "JULY": 0, "AUGUST": 0, "SEPTEMBER": 0, "OCTOBER": 0, "NOVEMBER": 0, "DECEMBER": 0}
	if db.Neo4jDriver == nil {
		return stats, nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)
	_, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx,
			`MATCH (post:Post) WHERE post.createdAt.year=$year
			 RETURN post.createdAt.month AS month, count(*) AS total`,
			map[string]interface{}{"year": int64(year)})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			m := int(res.Record().Values[0].(int64))
			t := int(res.Record().Values[1].(int64))
			if m >= 1 && m <= 12 {
				stats[monthNames[m]] = t
			}
		}
		return nil, nil
	})
	return stats, err
}

func (r *Neo4jAdminRepository) DeletePost(ctx context.Context, postID string) error {
	if db.Neo4jDriver == nil {
		return nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (p:Post {id: $postID})
			SET p.deletedAt = datetime()
			RETURN p
		`
		_, err := tx.Run(ctx, query, map[string]interface{}{"postID": postID})
		return nil, err
	})
	return err
}

func (r *Neo4jAdminRepository) SuspendUser(ctx context.Context, userID string, duration time.Duration) error {
	until := time.Now().Add(duration)

	// Dual write 1: Update Neo4j User Node
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $userID})
				SET u.suspended = true, u.suspendedUntil = $until
				RETURN u
			`
			_, err := tx.Run(ctx, query, map[string]interface{}{
				"userID": userID,
				"until":  until.Format(time.RFC3339),
			})
			return nil, err
		})
		if err != nil {
			return err
		}
	}

	// Dual write 2: Set Redis key auth:suspended:user:<id> with TTL
	if db.RedisClient != nil {
		redisKey := "auth:suspended:user:" + userID
		err := db.RedisClient.Set(ctx, redisKey, "true", duration).Err()
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Neo4jAdminRepository) UnsuspendUser(ctx context.Context, userID string) error {
	// Dual write 1: Update Neo4j User Node
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $userID})
				SET u.suspended = false, u.suspendedUntil = ""
				RETURN u
			`
			_, err := tx.Run(ctx, query, map[string]interface{}{
				"userID": userID,
			})
			return nil, err
		})
		if err != nil {
			return err
		}
	}

	// Dual write 2: Delete Redis key auth:suspended:user:<id>
	if db.RedisClient != nil {
		redisKey := "auth:suspended:user:" + userID
		err := db.RedisClient.Del(ctx, redisKey).Err()
		if err != nil {
			return err
		}
	}

	return nil
}
