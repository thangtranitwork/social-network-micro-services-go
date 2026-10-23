package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/search-service/config"
	"social-network-go/search-service/db"
	"social-network-go/search-service/model"
	"social-network-go/search-service/repository"
	syncTool "social-network-go/search-service/sync"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SearchService struct {
	Cfg         *config.Config
	Redis       *redis.Client
	UserClient  pb.UserServiceClient
	ESRepo      *repository.ESRepository
	BackfillSvc *syncTool.BackfillService
	HTTPClient  *http.Client
}

func NewSearchService(cfg *config.Config) *SearchService {
	var userClient pb.UserServiceClient
	conn, err := grpc.NewClient(
		cfg.UserGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logger.UnaryClientInterceptor()),
	)
	if err != nil {
		logger.Err(err).Warn("Failed to connect User gRPC in Search Service: %s", cfg.UserGrpcAddr)
	} else {
		userClient = pb.NewUserServiceClient(conn)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	var esRepo *repository.ESRepository
	if db.ESClient != nil {
		esRepo = repository.NewESRepository(db.ESClient)
	}

	return &SearchService{
		Cfg:         cfg,
		Redis:       rdb,
		UserClient:  userClient,
		ESRepo:      esRepo,
		BackfillSvc: syncTool.NewBackfillService(),
		HTTPClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (s *SearchService) Search(ctx context.Context, query string, currentUserID string, searchType string) (*model.SearchResults, error) {
	results := &model.SearchResults{
		USER:    []*model.User{},
		POST:    []*model.Post{},
		MESSAGE: []*model.Message{},
		GROUP:   []*model.Group{},
	}

	trimmedQuery := strings.TrimSpace(query)
	if len(trimmedQuery) < 2 {
		return results, nil
	}

	upperType := strings.ToUpper(strings.TrimSpace(searchType))
	if upperType == "" || upperType == "NOT_SET" {
		upperType = "ALL"
	}

	// 1. Primary: Search via Elasticsearch if available
	if s.ESRepo != nil {
		var esErr error
		if upperType == "ALL" || upperType == "USER" {
			blockedIDs, _ := s.GetBlockedUserIDs(ctx, currentUserID)
			if users, err := s.ESRepo.SearchUsers(ctx, trimmedQuery, blockedIDs, 15); err == nil {
				results.USER = users
			} else {
				esErr = err
				logger.WithContext(ctx).Err(err).Warn("Elasticsearch SearchUsers failed, attempting fallback")
			}
		}

		if upperType == "ALL" || upperType == "POST" {
			if posts, facets, err := s.ESRepo.SearchPostsWithFacets(ctx, trimmedQuery, currentUserID, 15); err == nil {
				results.POST = posts
				results.Facets = facets
			} else {
				esErr = err
				logger.WithContext(ctx).Err(err).Warn("Elasticsearch SearchPosts failed, attempting fallback")
			}
		}

		if upperType == "ALL" || upperType == "MESSAGE" {
			convIDs, _ := s.GetUserConversationIDs(ctx, currentUserID)
			if msgs, err := s.ESRepo.SearchMessages(ctx, trimmedQuery, currentUserID, convIDs, 15); err == nil {
				results.MESSAGE = msgs
			}
		}

		if upperType == "ALL" || upperType == "GROUP" {
			groupIDs, _ := s.GetUserJoinedGroupIDs(ctx, currentUserID)
			if groups, err := s.ESRepo.SearchGroups(ctx, trimmedQuery, currentUserID, groupIDs, 15); err == nil {
				results.GROUP = groups
			}
		}

		// If ES query succeeded (or partial results), resolve and return
		if esErr == nil || len(results.USER) > 0 || len(results.POST) > 0 {
			s.resolveUsers(ctx, results.USER)
			s.resolvePostAuthors(ctx, results.POST)
			s.enrichMediaURLs(results.USER, results.POST)
			return results, nil
		}
	}

	// 2. Fallback: Search in Neo4j if Elasticsearch is down or empty
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		if upperType == "ALL" || upperType == "USER" {
			if users, err := s.searchUsersNeo4j(ctx, session, trimmedQuery, currentUserID); err == nil {
				results.USER = users
			} else {
				logger.WithContext(ctx).Err(err).Error("Fallback Neo4j searchUsers failed")
			}
		}

		if upperType == "ALL" || upperType == "POST" {
			if posts, err := s.searchPostsNeo4j(ctx, session, trimmedQuery, currentUserID); err == nil {
				results.POST = posts
			} else {
				logger.WithContext(ctx).Err(err).Error("Fallback Neo4j searchPosts failed")
			}
		}
	}

	// 3. Resolve Authors & Media URLs
	s.resolveUsers(ctx, results.USER)
	s.resolvePostAuthors(ctx, results.POST)
	s.enrichMediaURLs(results.USER, results.POST)

	return results, nil
}

// SyncData triggers backfill from Neo4j into Elasticsearch
func (s *SearchService) SyncData(ctx context.Context) (int, int, error) {
	if s.BackfillSvc == nil {
		return 0, 0, fmt.Errorf("backfill service not initialized")
	}
	return s.BackfillSvc.SyncAllFromNeo4j(ctx)
}

// Reindex triggers zero-downtime reindexing from aliasName to targetIndex
func (s *SearchService) Reindex(ctx context.Context, aliasName, targetIndex string) error {
	if s.ESRepo == nil {
		return fmt.Errorf("elasticsearch repository not initialized")
	}
	mappingJSON, ok := db.IndexMappings[aliasName]
	if !ok {
		return fmt.Errorf("unknown index alias: %s", aliasName)
	}
	return s.ESRepo.ReindexIndex(ctx, aliasName, targetIndex, mappingJSON)
}

// Neo4j Fallback Queries
func (s *SearchService) searchUsersNeo4j(ctx context.Context, session neo4j.SessionWithContext, query string, currentUserID string) ([]*model.User, error) {
	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (u:User)
			WHERE (toLower(u.username) CONTAINS toLower($query)
			   OR toLower(u.givenName) CONTAINS toLower($query)
			   OR toLower(u.familyName) CONTAINS toLower($query))
			  AND ($currentUserID = '' OR NOT EXISTS((:User {id: $currentUserID})-[:BLOCK]-(u)))
			RETURN u.id, u.username, u.givenName, u.familyName, u.profilePictureId, u.bio
			LIMIT 15
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{
			"query":         query,
			"currentUserID": currentUserID,
		})
		if err != nil {
			return nil, err
		}

		list := []*model.User{}
		for res.Next(ctx) {
			vals := res.Record().Values
			list = append(list, &model.User{
				ID:                getStringVal(vals[0]),
				Username:          getStringVal(vals[1]),
				GivenName:         getStringVal(vals[2]),
				FamilyName:        getStringVal(vals[3]),
				ProfilePictureUrl: getStringVal(vals[4]),
				Email:             "", // Security/Privacy: Do NOT leak emails in public search results
				Bio:               getStringVal(vals[5]),
			})
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return resData.([]*model.User), nil
}

func (s *SearchService) searchPostsNeo4j(ctx context.Context, session neo4j.SessionWithContext, query string, currentUserID string) ([]*model.Post, error) {
	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL
			  AND p.privacy = 'PUBLIC'
			  AND toLower(p.content) CONTAINS toLower($query)

			OPTIONAL MATCH (viewer:User {id: $currentUserID})
			OPTIONAL MATCH (viewer)-[liked:LIKED]->(p)

			RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
			       author.id, coalesce(p.likeCount, 0), coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
			       p.files, liked IS NOT NULL
			ORDER BY p.createdAt DESC
			LIMIT 15
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{
			"query":         query,
			"currentUserID": currentUserID,
		})
		if err != nil {
			return nil, err
		}

		list := []*model.Post{}
		for res.Next(ctx) {
			vals := res.Record().Values
			p := &model.Post{
				ID:           getStringVal(vals[0]),
				Content:      getStringVal(vals[1]),
				Privacy:      getStringVal(vals[2]),
				AuthorID:     getStringVal(vals[5]),
				LikeCount:    getIntVal(vals[6]),
				CommentCount: getIntVal(vals[7]),
				ShareCount:   getIntVal(vals[8]),
				Files:        getStringSliceVal(vals[9]),
				Liked:        vals[10].(bool),
			}
			p.Images = p.Files

			if vals[3] != nil {
				p.CreatedAt = getTimeVal(vals[3])
			}
			if vals[4] != nil {
				tm := getTimeVal(vals[4])
				p.UpdatedAt = &tm
			}
			list = append(list, p)
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return resData.([]*model.Post), nil
}

func (s *SearchService) resolveUsers(ctx context.Context, users []*model.User) {
	if len(users) == 0 || s.UserClient == nil {
		return
	}

	var ids []string
	for _, u := range users {
		ids = append(ids, u.ID)
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := s.UserClient.GetUsersByIds(callCtx, &pb.UsersByIdsRequest{UserIds: ids})
	if err != nil {
		logger.Err(err).Error("Failed to GetUsersByIds in Search Service")
		return
	}

	userMap := make(map[string]*pb.UserCommonInfoResponse)
	for _, u := range resp.Users {
		userMap[u.UserId] = u
	}

	for _, u := range users {
		if detail, ok := userMap[u.ID]; ok {
			u.GivenName = detail.GivenName
			u.FamilyName = detail.FamilyName
			u.ProfilePictureUrl = detail.ProfilePictureId
			u.Username = detail.Username
		}
	}
}

func (s *SearchService) resolvePostAuthors(ctx context.Context, posts []*model.Post) {
	if len(posts) == 0 || s.UserClient == nil {
		return
	}

	var ids []string
	for _, p := range posts {
		if p.AuthorID != "" {
			ids = append(ids, p.AuthorID)
		}
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := s.UserClient.GetUsersByIds(callCtx, &pb.UsersByIdsRequest{UserIds: ids})
	if err != nil {
		logger.Err(err).Error("Failed to GetUsersByIds for post authors in Search Service")
		return
	}

	userMap := make(map[string]*pb.UserCommonInfoResponse)
	for _, u := range resp.Users {
		userMap[u.UserId] = u
	}

	for _, p := range posts {
		if detail, ok := userMap[p.AuthorID]; ok {
			p.Author = model.AuthorInfo{
				ID:                detail.UserId,
				Username:          detail.Username,
				GivenName:         detail.GivenName,
				FamilyName:        detail.FamilyName,
				ProfilePictureUrl: detail.ProfilePictureId,
			}
		} else {
			p.Author = model.AuthorInfo{ID: p.AuthorID}
		}
	}
}

func (s *SearchService) enrichMediaURLs(users []*model.User, posts []*model.Post) {
	for _, u := range users {
		if u.ProfilePictureUrl != "" && !strings.HasPrefix(u.ProfilePictureUrl, "http://") && !strings.HasPrefix(u.ProfilePictureUrl, "https://") {
			u.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, u.ProfilePictureUrl)
		}
	}
	for _, p := range posts {
		for i, f := range p.Files {
			if f != "" && !strings.HasPrefix(f, "http://") && !strings.HasPrefix(f, "https://") {
				p.Files[i] = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, f)
			}
		}
		p.Images = p.Files
		if p.Author.ProfilePictureUrl != "" && !strings.HasPrefix(p.Author.ProfilePictureUrl, "http://") && !strings.HasPrefix(p.Author.ProfilePictureUrl, "https://") {
			p.Author.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, p.Author.ProfilePictureUrl)
		}
	}
}

// Helpers
func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

func getIntVal(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return int(val)
	case int:
		return val
	default:
		return 0
	}
}

func getStringSliceVal(v interface{}) []string {
	if v == nil {
		return []string{}
	}
	slice, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	res := make([]string, len(slice))
	for i, val := range slice {
		res[i] = getStringVal(val)
	}
	return res
}

func getTimeVal(val interface{}) time.Time {
	if val == nil {
		return time.Time{}
	}
	switch v := val.(type) {
	case time.Time:
		return v
	case dbtype.LocalDateTime:
		return v.Time()
	case dbtype.Date:
		return v.Time()
	}
	return time.Time{}
}

// ---------------------------------------------------------------------
// Visual Search (Image-to-Image, Text-to-Image, and 1-Click Similar)
// ---------------------------------------------------------------------

// CheckRateLimit implements a sliding window rate limiter via Redis.
func (s *SearchService) CheckRateLimit(ctx context.Context, key string, maxRequests int, window time.Duration) bool {
	if s.Redis == nil {
		return true // Fail open if Redis is down
	}

	count, err := s.Redis.Incr(ctx, key).Result()
	if err != nil {
		logger.WithContext(ctx).Warn("Redis rate limit check error: %v", err)
		return true
	}

	if count == 1 {
		s.Redis.Expire(ctx, key, window)
	}

	return count <= int64(maxRequests)
}

// GetTextEmbedding fetches query embedding from ai-service or Redis cache.
func (s *SearchService) GetTextEmbedding(ctx context.Context, text string) ([]float32, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return nil, errors.New("empty text query")
	}

	h := sha256.Sum256([]byte(text))
	cacheKey := fmt.Sprintf("cache:vector:gemini-embedding-2:text:%s", hex.EncodeToString(h[:]))

	if s.Redis != nil {
		cachedData, err := s.Redis.Get(ctx, cacheKey).Bytes()
		if err == nil && len(cachedData) > 0 {
			var vec []float32
			if json.Unmarshal(cachedData, &vec) == nil && len(vec) > 0 {
				logger.WithContext(ctx).Info("Text embedding cache hit for query: %s", text)
				return vec, nil
			}
		}
	}

	// Call ai-service
	reqBody, _ := json.Marshal(map[string]string{"text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Cfg.AIServiceURL+"/v1/ai/embed-text", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call ai-service embed-text: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai-service embed-text returned status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Vector []float32 `json:"vector"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if len(res.Vector) == 0 {
		return nil, errors.New("ai-service returned empty embedding vector")
	}

	// Save to Redis (TTL 24 hours)
	if s.Redis != nil {
		if vecJSON, err := json.Marshal(res.Vector); err == nil {
			s.Redis.Set(ctx, cacheKey, vecJSON, 24*time.Hour)
		}
	}

	return res.Vector, nil
}

// GetImageEmbedding fetches image query embedding from ai-service or Redis cache.
func (s *SearchService) GetImageEmbedding(ctx context.Context, imageBytes []byte) ([]float32, error) {
	if len(imageBytes) == 0 {
		return nil, errors.New("empty image bytes")
	}

	h := sha256.Sum256(imageBytes)
	cacheKey := fmt.Sprintf("cache:vector:gemini-embedding-2:sha256:%s", hex.EncodeToString(h[:]))

	if s.Redis != nil {
		cachedData, err := s.Redis.Get(ctx, cacheKey).Bytes()
		if err == nil && len(cachedData) > 0 {
			var vec []float32
			if json.Unmarshal(cachedData, &vec) == nil && len(vec) > 0 {
				logger.WithContext(ctx).Info("Image embedding cache hit (SHA-256: %s)", hex.EncodeToString(h[:8]))
				return vec, nil
			}
		}
	}

	// Prepare multipart request to ai-service
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("image", "query.jpg")
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(imageBytes); err != nil {
		return nil, err
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.Cfg.AIServiceURL+"/v1/ai/embed-image", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := s.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call ai-service embed-image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ai-service embed-image returned status %d: %s", resp.StatusCode, string(b))
	}

	var res struct {
		Vector []float32 `json:"vector"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil, err
	}
	if len(res.Vector) == 0 {
		return nil, errors.New("ai-service returned empty embedding vector")
	}

	// Save to Redis (TTL 7 days)
	if s.Redis != nil {
		if vecJSON, err := json.Marshal(res.Vector); err == nil {
			s.Redis.Set(ctx, cacheKey, vecJSON, 7*24*time.Hour)
		}
	}

	return res.Vector, nil
}

// GetViewerFriendIDs fetches friends from Redis cache or Neo4j.
func (s *SearchService) GetViewerFriendIDs(ctx context.Context, viewerID string) ([]string, error) {
	if viewerID == "" {
		return []string{}, nil
	}

	cacheKey := fmt.Sprintf("cache:user:friends:%s", viewerID)
	if s.Redis != nil {
		val, err := s.Redis.Get(ctx, cacheKey).Result()
		if err == nil && val != "" {
			var ids []string
			if json.Unmarshal([]byte(val), &ids) == nil {
				return ids, nil
			}
		}
	}

	if db.Neo4jDriver == nil {
		return []string{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `MATCH (u:User {id: $userID})-[:FRIEND]-(f:User) RETURN f.id AS friendID`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": viewerID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("friendID"); ok && v != nil {
				ids = append(ids, fmt.Sprintf("%v", v))
			}
		}
		return ids, nil
	})
	if err != nil {
		return []string{}, err
	}

	friendIDs := resData.([]string)
	if s.Redis != nil && len(friendIDs) > 0 {
		if data, err := json.Marshal(friendIDs); err == nil {
			s.Redis.Set(ctx, cacheKey, data, 5*time.Minute)
		}
	}
	return friendIDs, nil
}

// GetBlockedUserIDs fetches bidirectional blocked user IDs for the viewer from Neo4j
func (s *SearchService) GetBlockedUserIDs(ctx context.Context, viewerID string) ([]string, error) {
	if viewerID == "" || db.Neo4jDriver == nil {
		return []string{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `MATCH (u:User {id: $userID})-[:BLOCK]-(b:User) RETURN b.id AS blockedID`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": viewerID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("blockedID"); ok && v != nil {
				ids = append(ids, fmt.Sprintf("%v", v))
			}
		}
		return ids, nil
	})
	if err != nil {
		return []string{}, err
	}
	return resData.([]string), nil
}

// GetUserConversationIDs fetches conversation IDs that the user is a participant of
func (s *SearchService) GetUserConversationIDs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" || db.Neo4jDriver == nil {
		return []string{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `MATCH (u:User {id: $userID})-[:PARTICIPANT_IN|MEMBER_OF]->(c:Conversation) RETURN c.id AS convID`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("convID"); ok && v != nil {
				ids = append(ids, fmt.Sprintf("%v", v))
			}
		}
		return ids, nil
	})
	if err != nil {
		return []string{}, err
	}
	return resData.([]string), nil
}

// GetUserJoinedGroupIDs fetches group IDs that the user is a member of
func (s *SearchService) GetUserJoinedGroupIDs(ctx context.Context, userID string) ([]string, error) {
	if userID == "" || db.Neo4jDriver == nil {
		return []string{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `MATCH (u:User {id: $userID})-[:MEMBER_OF]->(g:Group) RETURN g.id AS groupID`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			rec := res.Record()
			if v, ok := rec.Get("groupID"); ok && v != nil {
				ids = append(ids, fmt.Sprintf("%v", v))
			}
		}
		return ids, nil
	})
	if err != nil {
		return []string{}, err
	}
	return resData.([]string), nil
}

// SearchByImage performs Image-to-Image visual search.
func (s *SearchService) SearchByImage(ctx context.Context, imageBytes []byte, currentUserID string, limit int) ([]*model.VisualPostSearchResult, error) {
	if s.ESRepo == nil {
		return nil, errors.New("elasticsearch repository unavailable")
	}

	vec, err := s.GetImageEmbedding(ctx, imageBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image embedding: %w", err)
	}

	friendIDs, _ := s.GetViewerFriendIDs(ctx, currentUserID)
	allowedAuthors := append(friendIDs, currentUserID)

	// Cosine threshold for Image-to-Image: default 0.50 (customizable)
	minRawCosine := 0.50
	hits, err := s.ESRepo.SearchSimilarImagesWithFilter(ctx, vec, allowedAuthors, "", minRawCosine, limit, true)
	if err != nil {
		return nil, fmt.Errorf("kNN image search failed: %w", err)
	}

	return s.verifyAndEnrichVisualResults(ctx, hits, currentUserID)
}

// SearchImageByText performs Text-to-Image cross-modal search.
func (s *SearchService) SearchImageByText(ctx context.Context, queryText string, currentUserID string, limit int) ([]*model.VisualPostSearchResult, error) {
	if s.ESRepo == nil {
		return nil, errors.New("elasticsearch repository unavailable")
	}

	vec, err := s.GetTextEmbedding(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("failed to generate text embedding: %w", err)
	}

	friendIDs, _ := s.GetViewerFriendIDs(ctx, currentUserID)
	allowedAuthors := append(friendIDs, currentUserID)

	// Cross-modal Text-to-Image cosine threshold: default 0.20
	minRawCosine := 0.20
	hits, err := s.ESRepo.SearchSimilarImagesWithFilter(ctx, vec, allowedAuthors, "", minRawCosine, limit, false)
	if err != nil {
		return nil, fmt.Errorf("kNN visual text search failed: %w", err)
	}

	return s.verifyAndEnrichVisualResults(ctx, hits, currentUserID)
}

// SearchSimilarByFileID performs 1-click similar search from an existing fileID without calling Gemini.
func (s *SearchService) SearchSimilarByFileID(ctx context.Context, fileID string, currentUserID string, limit int) ([]*model.VisualPostSearchResult, error) {
	if s.ESRepo == nil {
		return nil, errors.New("elasticsearch repository unavailable")
	}
	if fileID == "" {
		return nil, errors.New("empty fileID")
	}

	// 1. Authorization: Verify viewer has permission to view the source post containing fileID
	if db.Neo4jDriver != nil && currentUserID != "" {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (viewer:User {id: $viewerID})
				MATCH (author:User)-[:POSTED]->(p:Post)
				WHERE $fileID IN p.files
				  AND p.deletedAt IS NULL
				  AND NOT (viewer)-[:BLOCK]-(author)
				  AND (
					p.privacy = 'PUBLIC'
					OR author.id = viewer.id
					OR (p.privacy = 'FRIEND' AND EXISTS((viewer)-[:FRIEND]-(author)))
				  )
				RETURN p.id AS postID
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{
				"viewerID": currentUserID,
				"fileID":   fileID,
			})
			if err != nil {
				return nil, err
			}
			if !res.Next(ctx) {
				return nil, errors.New("forbidden or not found")
			}
			return res.Record().Values[0], nil
		})
		if err != nil || resData == nil {
			return nil, errors.New("SOURCE_FILE_NOT_FOUND_OR_FORBIDDEN")
		}
	}

	// 2. Retrieve existing vector from Elasticsearch
	vec, sourcePostID, _, err := s.ESRepo.GetImageVectorByFileID(ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve image vector from ES: %w", err)
	}

	friendIDs, _ := s.GetViewerFriendIDs(ctx, currentUserID)
	allowedAuthors := append(friendIDs, currentUserID)

	// 3. Search similar images excluding the source post
	minRawCosine := 0.50
	hits, err := s.ESRepo.SearchSimilarImagesWithFilter(ctx, vec, allowedAuthors, sourcePostID, minRawCosine, limit, true)
	if err != nil {
		return nil, fmt.Errorf("kNN similar search failed: %w", err)
	}

	return s.verifyAndEnrichVisualResults(ctx, hits, currentUserID)
}

// verifyAndEnrichVisualResults performs Neo4j final defense-in-depth and enriches author/files.
func (s *SearchService) verifyAndEnrichVisualResults(ctx context.Context, hits []*model.ImageSearchResult, viewerID string) ([]*model.VisualPostSearchResult, error) {
	if len(hits) == 0 {
		return []*model.VisualPostSearchResult{}, nil
	}

	candidatePostIDs := make([]string, 0, len(hits))
	for _, h := range hits {
		candidatePostIDs = append(candidatePostIDs, h.PostID)
	}

	if db.Neo4jDriver == nil {
		return []*model.VisualPostSearchResult{}, nil
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (viewer:User {id: $viewerID})
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE p.id IN $candidatePostIDs
			  AND p.deletedAt IS NULL
			  AND NOT (viewer)-[:BLOCK]-(author)
			  AND (
				p.privacy = 'PUBLIC'
				OR author.id = viewer.id
				OR (p.privacy = 'FRIEND' AND EXISTS((viewer)-[:FRIEND]-(author)))
			  )
			RETURN p.id AS postID, author.id AS authorID, p.content AS content,
			       p.privacy AS privacy, p.files AS files, coalesce(p.likeCount, 0) AS likeCount,
			       coalesce(p.commentCount, 0) AS commentCount, coalesce(p.shareCount, 0) AS shareCount,
			       p.createdAt AS createdAt
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"viewerID":         viewerID,
			"candidatePostIDs": candidatePostIDs,
		})
		if err != nil {
			return nil, err
		}

		postMap := make(map[string]*model.Post)
		for res.Next(ctx) {
			rec := res.Record()
			p := &model.Post{
				ID:           getStringVal(rec.Values[0]),
				AuthorID:     getStringVal(rec.Values[1]),
				Content:      getStringVal(rec.Values[2]),
				Privacy:      getStringVal(rec.Values[3]),
				Files:        getStringSliceVal(rec.Values[4]),
				LikeCount:    getIntVal(rec.Values[5]),
				CommentCount: getIntVal(rec.Values[6]),
				ShareCount:   getIntVal(rec.Values[7]),
				CreatedAt:    getTimeVal(rec.Values[8]),
			}
			p.Images = p.Files
			postMap[p.ID] = p
		}
		return postMap, nil
	})
	if err != nil {
		return nil, fmt.Errorf("Neo4j defense-in-depth check failed: %w", err)
	}

	postMap := resData.(map[string]*model.Post)
	if len(postMap) == 0 {
		return []*model.VisualPostSearchResult{}, nil
	}

	// Enrich Authors and Media URLs
	postsToEnrich := make([]*model.Post, 0, len(postMap))
	for _, p := range postMap {
		postsToEnrich = append(postsToEnrich, p)
	}
	s.resolvePostAuthors(ctx, postsToEnrich)
	s.enrichMediaURLs(nil, postsToEnrich)

	// Combine into final results preserving order from ES hits
	finalResults := make([]*model.VisualPostSearchResult, 0, len(hits))
	for _, hit := range hits {
		if post, exists := postMap[hit.PostID]; exists {
			finalResults = append(finalResults, &model.VisualPostSearchResult{
				Post:           post,
				MatchedFileID:  hit.FileID,
				RawCosine:      hit.RawCosine,
				RelevanceLabel: hit.RelevanceLabel,
			})
		}
	}

	return finalResults, nil
}
