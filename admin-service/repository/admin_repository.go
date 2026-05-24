package repository

import (
	"context"
	"strconv"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
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

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User)<-[:HAS_INFO]-(a:Account)
			WITH date() AS today, u, a
			RETURN count(u) AS totalUsers,
				   count(CASE WHEN a.isVerified = false THEN 1 END) AS notVerifiedUsers,
				   count(CASE WHEN date(u.createdAt) = today THEN 1 END) AS newUsersToday,
				   count(CASE WHEN u.createdAt.week = today.week AND u.createdAt.year = today.year THEN 1 END) AS newUsersThisWeek,
				   count(CASE WHEN u.createdAt.month = today.month AND u.createdAt.year = today.year THEN 1 END) AS newUsersThisMonth,
				   count(CASE WHEN u.createdAt.year = today.year THEN 1 END) AS newUsersThisYear
		`
		res, err := tx.Run(ctx, query, nil)
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			vals := res.Record().Values
			return &model.UserStatisticsResponse{
				TotalUsers:        getIntVal(vals[0]),
				NotVerifiedUsers:  getIntVal(vals[1]),
				NewUsersToday:     getIntVal(vals[2]),
				NewUsersThisWeek:  getIntVal(vals[3]),
				NewUsersThisMonth: getIntVal(vals[4]),
				NewUsersThisYear:  getIntVal(vals[5]),
			}, nil
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
				   count(CASE WHEN post.createdAt.week = today.week THEN 1 END) AS newPostsThisWeek,
				   count(CASE WHEN post.createdAt.month = today.month THEN 1 END) AS newPostsThisMonth,
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
			
			createdAtTime := time.Now()
			if val, ok := vals[3].(dbtype.Time); ok {
				createdAtTime = val.Time()
			}

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
			MATCH (u:User)<-[:HAS_INFO]-(a:Account)
			WITH u, a
			ORDER BY u.createdAt DESC
			SKIP $skip LIMIT $limit
			
			OPTIONAL MATCH (u)-[:HAS_PROFILE_PICTURE]->(pf:File)
			WITH u, a, pf
			OPTIONAL MATCH (u)-[:FRIEND]-(friend:User)
			WITH u, a, pf, count(DISTINCT friend) as friendCount
			OPTIONAL MATCH (u)-[:POSTED]->(post:Post) WHERE post.deletedAt IS NULL
			WITH u, a, pf, friendCount, count(DISTINCT post) as postCount
			OPTIONAL MATCH (u)-[:COMMENTED]->(comment:Comment)
			WITH u, a, pf, friendCount, postCount, count(DISTINCT comment) as commentCount
			OPTIONAL MATCH (u)-[:SENT_FRIEND_REQUEST]->(reqSent:User)
			WITH u, a, pf, friendCount, postCount, commentCount, count(DISTINCT reqSent) as requestSentCount
			OPTIONAL MATCH (u)<-[:SENT_FRIEND_REQUEST]-(reqRecv:User)
			WITH u, a, pf, friendCount, postCount, commentCount, requestSentCount, count(DISTINCT reqRecv) as requestReceivedCount
			OPTIONAL MATCH (u)-[:BLOCK]->(blocked:User)
			RETURN u.id, u.givenName, u.familyName, u.username, a.email, u.bio, u.birthdate, u.createdAt,
				   friendCount, postCount, pf.id AS pfId, a.isVerified AS verified,
				   commentCount, requestSentCount, requestReceivedCount, count(DISTINCT blocked) AS blockCount
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
			
			regTime := time.Now()
			if val, ok := vals[7].(dbtype.LocalDateTime); ok {
				regTime = val.Time()
			}

			pfID := formatFileURL(vals[10])

			birth := ""
			if val, ok := vals[6].(dbtype.Date); ok {
				birth = val.Time().Format("2006-01-02")
			}

			u := model.UserDetailResponse{
				ID:                getStringVal(vals[0]),
				GivenName:         getStringVal(vals[1]),
				FamilyName:        getStringVal(vals[2]),
				Username:          getStringVal(vals[3]),
				Email:             getStringVal(vals[4]),
				Bio:               getStringVal(vals[5]),
				Birthdate:         birth,
				RegistrationDate:  regTime,
				FriendCount:       getIntVal(vals[8]),
				PostCount:         getIntVal(vals[9]),
				MessageCount:      0,
				CommentCount:      getIntVal(vals[12]),
				CallCount:         0,
				RequestSentCount:  getIntVal(vals[13]),
				RequestReceivedCount: getIntVal(vals[14]),
				UploadedFileCount: 0,
				BlockCount:        getIntVal(vals[15]),
				ProfilePictureUrl: pfID,
				Verified:          vals[11] != nil && vals[11].(bool),
			}
			list = append(list, u)
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return res.([]model.UserDetailResponse), nil
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
			`MATCH (u:User) WHERE u.createdAt.week=$week AND u.createdAt.year=$year
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
			`MATCH (post:Post) WHERE post.createdAt.week=$week AND post.createdAt.year=$year
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
