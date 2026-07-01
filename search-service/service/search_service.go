package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/search-service/config"
	"social-network-go/search-service/db"
	"social-network-go/search-service/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type SearchService struct {
	Cfg        *config.Config
	Redis      *redis.Client
	UserClient pb.UserServiceClient
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

	return &SearchService{
		Cfg:        cfg,
		Redis:      rdb,
		UserClient: userClient,
	}
}

func (s *SearchService) Search(ctx context.Context, query string, currentUserID string) (*model.SearchResults, error) {
	results := &model.SearchResults{
		USER: []*model.User{},
		POST: []*model.Post{},
	}

	trimmedQuery := strings.TrimSpace(query)
	if len(trimmedQuery) < 2 {
		return results, nil
	}

	if db.Neo4jDriver == nil {
		return results, fmt.Errorf("neo4j driver not initialized")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// 1. Search Users in Neo4j
	users, err := s.searchUsers(ctx, session, query)
	if err == nil {
		results.USER = users
	} else {
		logger.WithContext(ctx).Err(err).Error("Failed to search users in Neo4j")
	}

	// 2. Search Posts in Neo4j
	posts, err := s.searchPosts(ctx, session, query, currentUserID)
	if err == nil {
		results.POST = posts
	} else {
		logger.WithContext(ctx).Err(err).Error("Failed to search posts in Neo4j")
	}

	// 3. Resolve Authors & Media URLs
	s.resolveUsers(ctx, results.USER)
	s.resolvePostAuthors(ctx, results.POST)
	s.enrichMediaURLs(results.USER, results.POST)

	return results, nil
}

func (s *SearchService) searchUsers(ctx context.Context, session neo4j.SessionWithContext, query string) ([]*model.User, error) {
	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (u:User)
			WHERE toLower(u.username) CONTAINS toLower($query)
			   OR toLower(u.givenName) CONTAINS toLower($query)
			   OR toLower(u.familyName) CONTAINS toLower($query)
			RETURN u.id, u.username, u.givenName, u.familyName, u.profilePictureId, u.email, u.bio
			LIMIT 15
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{"query": query})
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
				Email:             getStringVal(vals[5]),
				Bio:               getStringVal(vals[6]),
			})
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return resData.([]*model.User), nil
}

func (s *SearchService) searchPosts(ctx context.Context, session neo4j.SessionWithContext, query string, currentUserID string) ([]*model.Post, error) {
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
