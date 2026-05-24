package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"
	"social-network-go/admin-service/service"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

type AdminHandler struct {
	svc *service.AdminService
}

func NewAdminHandler(svc *service.AdminService) *AdminHandler {
	return &AdminHandler{svc: svc}
}

func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	return val.(string)
}

func getIntVal(val interface{}) int {
	if val == nil {
		return 0
	}
	if v, ok := val.(int64); ok {
		return int(v)
	}
	return 0
}

func formatFileURL(id interface{}) string {
	if id == nil {
		return ""
	}
	str, ok := id.(string)
	if !ok || str == "" {
		return ""
	}
	if len(str) > 4 && str[:4] == "http" {
		return str
	}
	return "http://localhost:2003/v1/files/" + str
}

func sendSuccess(c *gin.Context, body interface{}) {
	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "OK",
		"timestamp": time.Now().Format(time.RFC3339),
		"body":      body,
	})
}

func (h *AdminHandler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "admin-service"})
	})

	r.GET("/v2/statistics/users", h.GetUsersStatistics)
	r.GET("/v2/statistics/users/week", h.GetUsersWeekStatistics)
	r.GET("/v2/statistics/users/month", h.GetUsersMonthStatistics)
	r.GET("/v2/statistics/users/year", h.GetUsersYearStatistics)
	r.GET("/v2/statistics/users/online", h.GetUsersOnlineStatistics)

	r.GET("/v2/statistics/posts", h.GetPostsStatistics)
	r.GET("/v2/statistics/posts/week", h.GetPostsWeekStatistics)
	r.GET("/v2/statistics/posts/month", h.GetPostsMonthStatistics)
	r.GET("/v2/statistics/posts/year", h.GetPostsYearStatistics)
	r.GET("/v2/statistics/posts/online", h.GetUsersOnlineStatistics) // Same as users online

	r.GET("/v1/posts", h.GetPostsList)
	r.GET("/v1/users", h.GetUsersList)
}

func (h *AdminHandler) GetUsersStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	if db.Neo4jDriver == nil {
		sendSuccess(c, model.UserStatisticsResponse{ThisWeekStatistics: map[string]int{}, ThisMonthStatistics: map[string]int{}, ThisYearStatistics: map[string]int{}})
		return
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

	var stats *model.UserStatisticsResponse
	if err == nil && res != nil {
		stats = res.(*model.UserStatisticsResponse)
	} else {
		stats = &model.UserStatisticsResponse{}
	}

	stats.OnlineUsersNow = h.svc.GetOnlineUsersCount()
	stats.OnlineStatistics = h.svc.GetOnlineStatisticsLogs("")

	now := time.Now()
	_, isoWeek := now.ISOWeek()
	year := now.Year()
	month := int(now.Month())
	stats.ThisWeekStatistics = h.svc.QueryWeekUserStats(ctx, isoWeek, year)
	stats.ThisMonthStatistics = h.svc.QueryMonthUserStats(ctx, month, year)
	stats.ThisYearStatistics = h.svc.QueryYearUserStats(ctx, year)

	sendSuccess(c, stats)
}

func (h *AdminHandler) GetUsersWeekStatistics(c *gin.Context) {
	weekStr := c.DefaultQuery("week", "")
	var week, year int
	if weekStr != "" {
		fmt.Sscanf(weekStr, "%d-W%d", &year, &week)
	}
	if year == 0 || week == 0 {
		_, week = time.Now().ISOWeek()
		year = time.Now().Year()
	}
	sendSuccess(c, h.svc.QueryWeekUserStats(c.Request.Context(), week, year))
}

func (h *AdminHandler) GetUsersMonthStatistics(c *gin.Context) {
	monthStr := c.DefaultQuery("month", "")
	var year, month int
	if monthStr != "" {
		fmt.Sscanf(monthStr, "%d-%d", &year, &month)
	}
	if year == 0 || month == 0 {
		year = time.Now().Year()
		month = int(time.Now().Month())
	}
	sendSuccess(c, h.svc.QueryMonthUserStats(c.Request.Context(), month, year))
}

func (h *AdminHandler) GetUsersYearStatistics(c *gin.Context) {
	year := time.Now().Year()
	if y, err := strconv.Atoi(c.DefaultQuery("year", "")); err == nil && y > 0 {
		year = y
	}
	sendSuccess(c, h.svc.QueryYearUserStats(c.Request.Context(), year))
}

func (h *AdminHandler) GetUsersOnlineStatistics(c *gin.Context) {
	dateStr := c.Query("date")
	sendSuccess(c, h.svc.GetOnlineStatisticsLogs(dateStr))
}

func (h *AdminHandler) GetPostsStatistics(c *gin.Context) {
	ctx := c.Request.Context()
	if db.Neo4jDriver == nil {
		sendSuccess(c, model.PostStatisticsResponse{ThisWeekStatistics: map[string]int{}, ThisMonthStatistics: map[string]int{}, ThisYearStatistics: map[string]int{}})
		return
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

	var stats *model.PostStatisticsResponse
	if err == nil && res != nil {
		stats = res.(*model.PostStatisticsResponse)
	} else {
		stats = &model.PostStatisticsResponse{}
	}

	pNow := time.Now()
	_, pWeek := pNow.ISOWeek()
	pYear := pNow.Year()
	pMonth := int(pNow.Month())
	stats.ThisWeekStatistics = h.svc.QueryWeekPostStats(ctx, pWeek, pYear)
	stats.ThisMonthStatistics = h.svc.QueryMonthPostStats(ctx, pMonth, pYear)
	stats.ThisYearStatistics = h.svc.QueryYearPostStats(ctx, pYear)

	sendSuccess(c, stats)
}

func (h *AdminHandler) GetPostsWeekStatistics(c *gin.Context) {
	weekStr := c.DefaultQuery("week", "")
	var week, year int
	if weekStr != "" {
		fmt.Sscanf(weekStr, "%d-W%d", &year, &week)
	}
	if year == 0 || week == 0 {
		_, week = time.Now().ISOWeek()
		year = time.Now().Year()
	}
	sendSuccess(c, h.svc.QueryWeekPostStats(c.Request.Context(), week, year))
}

func (h *AdminHandler) GetPostsMonthStatistics(c *gin.Context) {
	monthStr := c.DefaultQuery("month", "")
	var year, month int
	if monthStr != "" {
		fmt.Sscanf(monthStr, "%d-%d", &year, &month)
	}
	if year == 0 || month == 0 {
		year = time.Now().Year()
		month = int(time.Now().Month())
	}
	sendSuccess(c, h.svc.QueryMonthPostStats(c.Request.Context(), month, year))
}

func (h *AdminHandler) GetPostsYearStatistics(c *gin.Context) {
	year := time.Now().Year()
	if y, err := strconv.Atoi(c.DefaultQuery("year", "")); err == nil && y > 0 {
		year = y
	}
	sendSuccess(c, h.svc.QueryYearPostStats(c.Request.Context(), year))
}

func (h *AdminHandler) GetPostsList(c *gin.Context) {
	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")
	skip, _ := strconv.Atoi(skipStr)
	limit, _ := strconv.Atoi(limitStr)

	if db.Neo4jDriver == nil {
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "OK", "body": []interface{}{}})
		return
	}

	ctx := c.Request.Context()
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	postsListRes, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// FIXED: Relationship direction is (author:User)-[:POSTED]->(post:Post)
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

			authorID := getStringVal(vals[7])
			isOnline, _ := h.svc.GetUserOnlineStatus(ctx, authorID)

			author := model.CreatorInfo{
				ID:                authorID,
				Username:          getStringVal(vals[8]),
				GivenName:         getStringVal(vals[9]),
				FamilyName:        getStringVal(vals[10]),
				ProfilePictureUrl: pfID,
				IsOnline:          isOnline,
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, postsListRes)
}

func (h *AdminHandler) GetUsersList(c *gin.Context) {
	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")
	skip, _ := strconv.Atoi(skipStr)
	limit, _ := strconv.Atoi(limitStr)

	if db.Neo4jDriver == nil {
		c.JSON(http.StatusOK, gin.H{"code": 200, "message": "OK", "body": []interface{}{}})
		return
	}

	ctx := c.Request.Context()
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	usersListRes, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// FIXED: Replaced mock data with actual neo4j subqueries to fetch real counts
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

			userID := getStringVal(vals[0])
			isOnline, lastOnline := h.svc.GetUserOnlineStatus(ctx, userID)

			u := model.UserDetailResponse{
				ID:                userID,
				GivenName:         getStringVal(vals[1]),
				FamilyName:        getStringVal(vals[2]),
				Username:          getStringVal(vals[3]),
				Email:             getStringVal(vals[4]),
				Bio:               getStringVal(vals[5]),
				Birthdate:         birth,
				RegistrationDate:  regTime,
				FriendCount:       getIntVal(vals[8]),
				PostCount:         getIntVal(vals[9]),
				MessageCount:      0, // Messages are stored in MongoDB, cannot be queried directly from AdminService easily without grpc
				CommentCount:      getIntVal(vals[12]),
				CallCount:         0, // Calls are stored differently, unable to query from Neo4j
				RequestSentCount:  getIntVal(vals[13]),
				RequestReceivedCount: getIntVal(vals[14]),
				UploadedFileCount: 0, // Not explicitly tracked in User relations
				BlockCount:        getIntVal(vals[15]),
				IsOnline:          isOnline,
				LastOnline:        lastOnline,
				ProfilePictureUrl: pfID,
				Verified:          vals[11] != nil && vals[11].(bool),
			}
			list = append(list, u)
		}
		return list, nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, usersListRes)
}
