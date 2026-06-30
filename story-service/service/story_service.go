package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/story-service/config"
	"social-network-go/story-service/db"
	"social-network-go/story-service/model"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type StoryService struct {
	Cfg        *config.Config
	Redis      *redis.Client
	UserClient pb.UserServiceClient
}

func NewStoryService(cfg *config.Config) *StoryService {
	var userClient pb.UserServiceClient
	conn, err := grpc.NewClient(
		cfg.UserGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logger.UnaryClientInterceptor()),
	)
	if err != nil {
		logger.Err(err).Warn("Failed to connect User gRPC in Story Service: %s", cfg.UserGrpcAddr)
	} else {
		userClient = pb.NewUserServiceClient(conn)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	return &StoryService{
		Cfg:        cfg,
		Redis:      rdb,
		UserClient: userClient,
	}
}

func (s *StoryService) CreateStory(ctx context.Context, authorID, mediaUrl, mediaType string) (*model.Story, error) {
	if mediaUrl == "" {
		return nil, fmt.Errorf("mediaUrl is required")
	}
	if mediaType == "" {
		mediaType = "IMAGE"
	}

	storyID := uuid.New().String()

	if db.Neo4jDriver == nil {
		return nil, fmt.Errorf("neo4j driver not initialized")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (u:User {id: $authorID})
			CREATE (s:Story {
				id: $id,
				mediaUrl: $mediaUrl,
				mediaType: $mediaType,
				createdAt: datetime()
			})
			CREATE (u)-[:POSTED_STORY]->(s)
			RETURN s.id
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{
			"authorID":  authorID,
			"id":        storyID,
			"mediaUrl":  mediaUrl,
			"mediaType": mediaType,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("user not found or story create failed")
		}
		return nil, nil
	})
	if err != nil {
		return nil, err
	}

	story := &model.Story{
		ID:        storyID,
		MediaUrl:  s.resolveMediaURL(mediaUrl),
		MediaType: mediaType,
		CreatedAt: time.Now(),
		AuthorID:  authorID,
	}

	return story, nil
}

func (s *StoryService) GetStoryFeed(ctx context.Context, userID string) ([]*model.UserStories, error) {
	if db.Neo4jDriver == nil {
		return nil, fmt.Errorf("neo4j driver not initialized")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// User stories map
	userStoriesMap := make(map[string]*model.UserStories)
	var orderedUserIDs []string

	// 1. Fetch own active stories
	ownStories, err := s.fetchOwnStories(ctx, session, userID)
	if err == nil && len(ownStories) > 0 {
		userStoriesMap[userID] = &model.UserStories{
			Stories: ownStories,
		}
		orderedUserIDs = append(orderedUserIDs, userID)
	}

	// 2. Fetch friends' active stories
	friendStoriesMap, friendIDs, err := s.fetchFriendsStories(ctx, session, userID)
	if err == nil {
		for _, fID := range friendIDs {
			userStoriesMap[fID] = &model.UserStories{
				Stories: friendStoriesMap[fID],
			}
			orderedUserIDs = append(orderedUserIDs, fID)
		}
	}

	// 3. Resolve user details using gRPC
	s.resolveUserStoriesAuthors(ctx, userStoriesMap, orderedUserIDs)

	// 4. Build output slice
	var feed []*model.UserStories
	for _, id := range orderedUserIDs {
		feed = append(feed, userStoriesMap[id])
	}

	return feed, nil
}

func (s *StoryService) DeleteStory(ctx context.Context, userID, storyID string) error {
	if db.Neo4jDriver == nil {
		return fmt.Errorf("neo4j driver not initialized")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (u:User {id: $userID})-[rel:POSTED_STORY]->(s:Story {id: $storyID})
			WITH s.id AS deletedID, s
			DETACH DELETE s
			RETURN deletedID
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{
			"userID":  userID,
			"storyID": storyID,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, fmt.Errorf("story not found or unauthorized")
		}
		return nil, nil
	})

	return err
}

func (s *StoryService) fetchOwnStories(ctx context.Context, session neo4j.SessionWithContext, userID string) ([]*model.Story, error) {
	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (cu:User {id: $userID})-[:POSTED_STORY]->(s:Story)
			WHERE s.createdAt > datetime() - duration('P1D')
			RETURN s.id, s.mediaUrl, s.mediaType, s.createdAt
			ORDER BY s.createdAt ASC
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}

		var list []*model.Story
		for res.Next(ctx) {
			vals := res.Record().Values
			story := &model.Story{
				ID:        getStringVal(vals[0]),
				MediaUrl:  s.resolveMediaURL(getStringVal(vals[1])),
				MediaType: getStringVal(vals[2]),
				AuthorID:  userID,
			}
			if vals[3] != nil {
				story.CreatedAt = getTimeVal(vals[3])
			}
			list = append(list, story)
		}
		return list, nil
	})
	if err != nil {
		return nil, err
	}
	return resData.([]*model.Story), nil
}

func (s *StoryService) fetchFriendsStories(ctx context.Context, session neo4j.SessionWithContext, userID string) (map[string][]*model.Story, []string, error) {
	type resultHelper struct {
		friendID string
		story    *model.Story
	}

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		cypher := `
			MATCH (cu:User {id: $userID})-[:FRIEND]-(friend:User)-[:POSTED_STORY]->(s:Story)
			WHERE s.createdAt > datetime() - duration('P1D')
			RETURN friend.id, s.id, s.mediaUrl, s.mediaType, s.createdAt
			ORDER BY s.createdAt ASC
		`
		res, err := tx.Run(ctx, cypher, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}

		var helpers []*resultHelper
		for res.Next(ctx) {
			vals := res.Record().Values
			friendID := getStringVal(vals[0])
			story := &model.Story{
				ID:        getStringVal(vals[1]),
				MediaUrl:  s.resolveMediaURL(getStringVal(vals[2])),
				MediaType: getStringVal(vals[3]),
				AuthorID:  friendID,
			}
			if vals[4] != nil {
				story.CreatedAt = getTimeVal(vals[4])
			}
			helpers = append(helpers, &resultHelper{friendID: friendID, story: story})
		}
		return helpers, nil
	})

	if err != nil {
		return nil, nil, err
	}

	helpers := resData.([]*resultHelper)
	storyMap := make(map[string][]*model.Story)
	var friendIDs []string
	seen := make(map[string]bool)

	for _, h := range helpers {
		if !seen[h.friendID] {
			seen[h.friendID] = true
			friendIDs = append(friendIDs, h.friendID)
		}
		storyMap[h.friendID] = append(storyMap[h.friendID], h.story)
	}

	return storyMap, friendIDs, nil
}

func (s *StoryService) resolveUserStoriesAuthors(ctx context.Context, userStoriesMap map[string]*model.UserStories, userIDs []string) {
	if len(userIDs) == 0 || s.UserClient == nil {
		return
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	resp, err := s.UserClient.GetUsersByIds(callCtx, &pb.UsersByIdsRequest{UserIds: userIDs})
	if err != nil {
		logger.Err(err).Error("Failed to GetUsersByIds in Story Service")
		// Fallback setup
		for _, id := range userIDs {
			userStoriesMap[id].User = model.AuthorInfo{ID: id}
		}
		return
	}

	userMap := make(map[string]*pb.UserCommonInfoResponse)
	for _, u := range resp.Users {
		userMap[u.UserId] = u
	}

	for _, id := range userIDs {
		if detail, ok := userMap[id]; ok {
			avatar := detail.ProfilePictureId
			if avatar != "" && !strings.HasPrefix(avatar, "http://") && !strings.HasPrefix(avatar, "https://") {
				avatar = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, avatar)
			}
			userStoriesMap[id].User = model.AuthorInfo{
				ID:                detail.UserId,
				Username:          detail.Username,
				GivenName:         detail.GivenName,
				FamilyName:        detail.FamilyName,
				ProfilePictureUrl: avatar,
			}
		} else {
			userStoriesMap[id].User = model.AuthorInfo{ID: id}
		}
	}
}

func (s *StoryService) resolveMediaURL(mediaUrl string) string {
	if mediaUrl != "" && !strings.HasPrefix(mediaUrl, "http://") && !strings.HasPrefix(mediaUrl, "https://") {
		return fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, mediaUrl)
	}
	return mediaUrl
}

// Helpers
func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
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
