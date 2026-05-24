package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"social-network-go/pb"
	"social-network-go/post-service/config"
	"social-network-go/post-service/db"
	"social-network-go/post-service/model"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	PostPrivacyPublic  = "PUBLIC"
	PostPrivacyFriend  = "FRIEND"
	PostPrivacyPrivate = "PRIVATE"

	PageTypeRelevant   = "RELEVANT"
	PageTypeFriendOnly = "FRIEND_ONLY"
	PageTypeTime       = "TIME"

	MaxPostContentLength    = 5000
	MaxPostAttachFiles      = 10
	MaxCommentContentLength = 1000
)

type Pageable struct {
	Skip  int64
	Limit int64
	Type  string
}

type FileClient interface {
	DeleteFiles(ctx context.Context, fileIDs []string) error
	GetPresignedURL(ctx context.Context, fileID string) (string, error)
	GetPresignedURLs(ctx context.Context, fileIDs []string) (map[string]string, error)
	GetPresignedUploadURL(ctx context.Context, filename, contentType string) (string, string, error)
}

type NotificationPublisher interface {
	Send(ctx context.Context, action string, creatorID string, receiverID string, targetID string, targetType string, shortenedContent string) error
	SendToFriends(ctx context.Context, action string, creatorID string, targetID string, targetType string, shortenedContent string) error
}

type KeywordInteractor interface {
	Interact(ctx context.Context, postID string, actorID string, score string) error
	ExtractPostKeywords(ctx context.Context, postID string, content string, isUpdate bool) error
	PostsLoaded(ctx context.Context, postIDs []string, userID string) error
}

type PostService struct {
	Cfg         *config.Config
	Redis      *redis.Client
	UserClient pb.UserServiceClient

	FileClient        FileClient
	Notification     NotificationPublisher
	KeywordInteractor KeywordInteractor
}

func NewPostService(cfg *config.Config) *PostService {
	var userClient pb.UserServiceClient

	conn, err := grpc.Dial(cfg.UserGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Warning: failed to connect User gRPC %s: %v", cfg.UserGrpcAddr, err)
	} else {
		userClient = pb.NewUserServiceClient(conn)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	return &PostService{
		Cfg:        cfg,
		Redis:      rdb,
		UserClient: userClient,
	}
}

func (s *PostService) WithIntegrations(fileClient FileClient, notification NotificationPublisher, keyword KeywordInteractor) *PostService {
	s.FileClient = fileClient
	s.Notification = notification
	s.KeywordInteractor = keyword
	return s
}

func (s *PostService) ResolveAuthor(ctx context.Context, authorID string) model.AuthorInfo {
	cacheKey := fmt.Sprintf("user:profile:%s", authorID)

	if s.Redis != nil {
		cached, err := s.Redis.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var info model.AuthorInfo
			if json.Unmarshal([]byte(cached), &info) == nil {
				return info
			}
		}
	}

	if s.UserClient != nil {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		resp, err := s.UserClient.GetCommonUserInfo(callCtx, &pb.UserRequest{UserId: authorID})
		if err == nil && resp != nil {
			info := model.AuthorInfo{
				ID:                resp.UserId,
				Username:          resp.Username,
				GivenName:         resp.GivenName,
				FamilyName:        resp.FamilyName,
				ProfilePictureUrl: resp.ProfilePictureId,
			}
			if s.Redis != nil {
				bytes, _ := json.Marshal(info)
				_ = s.Redis.Set(ctx, cacheKey, string(bytes), 10*time.Minute).Err()
			}
			return info
		}
	}

	return model.AuthorInfo{ID: authorID}
}

func (s *PostService) CreatePost(ctx context.Context, authorID, content, privacy string, fileIDs []string) (*model.Post, error) {
	content = strings.TrimSpace(content)
	
	// Filter out empty file IDs
	cleanFileIDs := make([]string, 0)
	for _, id := range fileIDs {
		if id != "" {
			cleanFileIDs = append(cleanFileIDs, id)
		}
	}
	fileIDs = cleanFileIDs

	if err := validatePostRequest(content, fileIDs); err != nil {
		return nil, err
	}
	if privacy == "" {
		privacy = PostPrivacyPublic
	}
	if !isValidPostPrivacy(privacy) {
		return nil, errors.New("INVALID_POST_PRIVACY")
	}

	postID := uuid.NewString()
	now := time.Now()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User {id: $authorID})
			CREATE (p:Post {
				id: $postID,
				content: $content,
				privacy: $privacy,
				files: $files,
				likeCount: 0,
				commentCount: 0,
				shareCount: 0,
				createdAt: datetime(),
				updatedAt: null,
				deletedAt: null
			})
			CREATE (u)-[:POSTED]->(p)
			RETURN p.id
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"authorID": authorID,
			"postID":   postID,
			"content":  content,
			"privacy":  privacy,
			"files":    fileIDs,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("USER_NOT_FOUND")
		}
		return nil, res.Err()
	})
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.ExtractPostKeywords(ctx, postID, content, false)
	}
	if s.Notification != nil {
		_ = s.Notification.SendToFriends(ctx, "POST", authorID, postID, "POST", truncateByWord(content))
	}

	post := &model.Post{
		ID:         postID,
		Content:    content,
		AuthorID:   authorID,
		Privacy:    privacy,
		Files:      make([]string, 0),
		Images:     make([]string, 0),
		SharedPost: false,
		Liked:      false,
		CreatedAt:  now,
		Author:     s.ResolveAuthor(ctx, authorID),
	}
	if len(fileIDs) > 0 {
		post.Files = fileIDs
		post.Images = fileIDs
	}

	s.enrichPostsWithPresignedURLs(ctx, []*model.Post{post})

	return post, nil
}

func (s *PostService) SharePost(ctx context.Context, authorID, originalPostID, content, privacy string) (*model.Post, error) {
	content = strings.TrimSpace(content)
	if content != "" && len(content) > MaxPostContentLength {
		return nil, errors.New("INVALID_POST_CONTENT_LENGTH")
	}
	if privacy == "" {
		privacy = PostPrivacyPublic
	}
	if !isValidPostPrivacy(privacy) {
		return nil, errors.New("INVALID_POST_PRIVACY")
	}

	original, err := s.GetPost(ctx, originalPostID, authorID)
	if err != nil {
		return nil, err
	}
	if original.Privacy != PostPrivacyPublic {
		return nil, errors.New("ONLY_PUBLIC_POST_CAN_BE_SHARED")
	}
	if err := s.ValidateBlockByIDs(ctx, authorID, original.AuthorID); err != nil {
		return nil, err
	}

	postID := uuid.NewString()
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (author:User {id: $authorID}), (origin:Post {id: $originalPostID})
			WHERE origin.deletedAt IS NULL
			CREATE (p:Post {
				id: $postID,
				content: $content,
				privacy: $privacy,
				files: [],
				likeCount: 0,
				commentCount: 0,
				shareCount: 0,
				createdAt: datetime(),
				updatedAt: null,
				deletedAt: null
			})
			CREATE (author)-[:POSTED]->(p)
			CREATE (p)-[:SHARED_FROM]->(origin)
			SET origin.shareCount = coalesce(origin.shareCount, 0) + 1
			RETURN p.id
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"authorID":       authorID,
			"originalPostID": originalPostID,
			"postID":         postID,
			"content":        content,
			"privacy":        privacy,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		return nil, res.Err()
	})
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, originalPostID, authorID, "SHARE_SCORE")
		_ = s.KeywordInteractor.ExtractPostKeywords(ctx, postID, content, false)
	}
	if s.Notification != nil {
		_ = s.Notification.Send(ctx, "SHARE", authorID, original.AuthorID, postID, "POST", truncateByWord(content))
		_ = s.Notification.SendToFriends(ctx, "POST", authorID, postID, "POST", truncateByWord(original.Content))
	}

	return s.GetPost(ctx, postID, authorID)
}

func (s *PostService) GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (author:User)-[:POSTED]->(p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			OPTIONAL MATCH (viewer:User {id: $currentUserID})
			OPTIONAL MATCH (viewer)-[liked:LIKED]->(p)
			OPTIONAL MATCH (viewer)-[friendship:FRIEND]-(author)
			OPTIONAL MATCH (p)-[:SHARED_FROM]->(origin:Post)<-[:POSTED]-(originAuthor:User)
			OPTIONAL MATCH (viewer)-[originFriendship:FRIEND]-(originAuthor)
			OPTIONAL MATCH (viewer)-[block:BLOCK]-(originAuthor)

			WITH p, author, viewer, liked, friendship, origin, originAuthor, originFriendship, block,
			     CASE
			       WHEN origin IS NULL THEN true
			       WHEN origin.deletedAt IS NOT NULL THEN false
			       WHEN block IS NOT NULL THEN false
			       WHEN origin.privacy = 'PUBLIC' THEN true
			       WHEN origin.privacy = 'FRIEND' AND (viewer.id = originAuthor.id OR originFriendship IS NOT NULL) THEN true
			       WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true
			       ELSE false
			     END AS originCanView

			RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
			       author.id, coalesce(p.likeCount, 0), count(liked) > 0,
			       p.files, coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
			       origin.id, originAuthor.id, originCanView, friendship IS NOT NULL,
			       origin.content, origin.createdAt, origin.updatedAt, origin.privacy, origin.files
		`
		result, err := tx.Run(ctx, query, map[string]interface{}{
			"postID":        postID,
			"currentUserID": currentUserID,
		})
		if err != nil {
			return nil, err
		}
		if !result.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		p := s.postFromRecord(ctx, result.Record().Values)
		return p, result.Err()
	})
	if err != nil {
		return nil, err
	}

	post := res.(*model.Post)
	if err := s.ValidateViewPost(ctx, post, currentUserID); err != nil {
		return nil, err
	}

	if currentUserID != "" && s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, postID, currentUserID, "GET_SCORE")
	}

	s.enrichPostsWithPresignedURLs(ctx, []*model.Post{post})

	return post, nil
}

func (s *PostService) GetPostsOfUser(ctx context.Context, authorUsername string, currentUserID string, pageable Pageable) ([]*model.Post, error) {
	if err := s.ValidateBlockByUsername(ctx, currentUserID, authorUsername); err != nil {
		return nil, err
	}

	return s.queryPosts(ctx, currentUserID, `
		MATCH (author:User {username: $username})-[:POSTED]->(p:Post)
		WHERE p.deletedAt IS NULL
		OPTIONAL MATCH (viewer:User {id: $currentUserID})
		OPTIONAL MATCH (viewer)-[liked:LIKED]->(p)
		OPTIONAL MATCH (viewer)-[friendship:FRIEND]-(author)
		
		WHERE (author.username = $username AND (
			p.privacy = 'PUBLIC' OR 
			viewer.id = author.id OR 
			(p.privacy = 'FRIEND' AND friendship IS NOT NULL)
		))

		OPTIONAL MATCH (p)-[:SHARED_FROM]->(origin:Post)<-[:POSTED]-(originAuthor:User)
		OPTIONAL MATCH (viewer)-[originFriendship:FRIEND]-(originAuthor)
		OPTIONAL MATCH (viewer)-[block:BLOCK]-(originAuthor)

		WITH p, author, viewer, liked, friendship, origin, originAuthor, originFriendship, block,
			 CASE
			   WHEN origin IS NULL THEN true
			   WHEN origin.deletedAt IS NOT NULL THEN false
			   WHEN block IS NOT NULL THEN false
			   WHEN origin.privacy = 'PUBLIC' THEN true
			   WHEN origin.privacy = 'FRIEND' AND (viewer.id = originAuthor.id OR originFriendship IS NOT NULL) THEN true
			   WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true
			   ELSE false
			 END AS originCanView

		RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
		       author.id, coalesce(p.likeCount, 0), count(liked) > 0,
		       p.files, coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
		       origin.id, originAuthor.id, originCanView, friendship IS NOT NULL,
			   origin.content, origin.createdAt, origin.updatedAt, origin.privacy, origin.files
		ORDER BY p.createdAt DESC
		SKIP $skip LIMIT $limit
	`, map[string]interface{}{
		"username":      authorUsername,
		"currentUserID": currentUserID,
		"skip":          pageable.Skip,
		"limit":         normalizeLimit(pageable.Limit),
	})
}

func (s *PostService) GetAllPosts(ctx context.Context, pageable Pageable) ([]*model.Post, error) {
	return s.queryPosts(ctx, "", `
		MATCH (author:User)-[:POSTED]->(p:Post)
		WHERE p.deletedAt IS NULL
		OPTIONAL MATCH (p)-[:SHARED_FROM]->(origin:Post)<-[:POSTED]-(originAuthor:User)
		RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
		       author.id, coalesce(p.likeCount, 0), false,
		       p.files, coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
		       origin.id, originAuthor.id, true, false,
			   origin.content, origin.createdAt, origin.updatedAt, origin.privacy, origin.files
		ORDER BY p.createdAt DESC
		SKIP $skip LIMIT $limit
	`, map[string]interface{}{
		"skip":  pageable.Skip,
		"limit": normalizeLimit(pageable.Limit),
	})
}

func (s *PostService) GetSuggestedPosts(ctx context.Context, currentUserID string, pageable Pageable) ([]*model.Post, error) {
	pageType := pageable.Type
	if pageType == "" {
		pageType = PageTypeRelevant
	}

	var query string
	switch pageType {
	case PageTypeFriendOnly:
		query = `
			MATCH (viewer:User {id: $currentUserID})
			MATCH (viewer)-[:FRIEND]->(author:User)-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL AND p.privacy IN ['PUBLIC', 'FRIEND']
			  AND NOT (viewer)-[:BLOCK]-(author)
			
			OPTIONAL MATCH (viewer)-[liked:LIKED]->(p)
			OPTIONAL MATCH (p)-[:SHARED_FROM]->(origin:Post)<-[:POSTED]-(originAuthor:User)
			OPTIONAL MATCH (viewer)-[originFriendship:FRIEND]-(originAuthor)
			OPTIONAL MATCH (viewer)-[block:BLOCK]-(originAuthor)

			WITH p, author, liked, origin, originAuthor, originFriendship, block, viewer,
				 CASE
				   WHEN origin IS NULL THEN true
				   WHEN origin.deletedAt IS NOT NULL THEN false
				   WHEN block IS NOT NULL THEN false
				   WHEN origin.privacy = 'PUBLIC' THEN true
				   WHEN origin.privacy = 'FRIEND' AND (viewer.id = originAuthor.id OR originFriendship IS NOT NULL) THEN true
				   WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true
				   ELSE false
				 END AS originCanView

			ORDER BY p.createdAt DESC
			SKIP $skip LIMIT $limit

			WITH p, author, viewer, liked, origin, originAuthor, originCanView

			MERGE (viewer)-[l:LOADED]->(p)
			ON CREATE SET l.times = 1
			ON MATCH SET l.times = coalesce(l.times, 0) + 1

			RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
			       author.id, coalesce(p.likeCount, 0), liked IS NOT NULL,
			       p.files, coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
			       origin.id, originAuthor.id, originCanView, true,
				   origin.content, origin.createdAt, origin.updatedAt, origin.privacy, origin.files
		`
	case PageTypeTime:
		query = `
			MATCH (viewer:User {id: $currentUserID})
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL
			  AND NOT (viewer)-[:BLOCK]-(author)
			
			OPTIONAL MATCH (viewer)-[friendship:FRIEND]->(author)
			WHERE (
				p.privacy = 'PUBLIC' 
				OR author.id = viewer.id 
				OR (p.privacy = 'FRIEND' AND friendship IS NOT NULL)
			)
			
			OPTIONAL MATCH (viewer)-[liked:LIKED]->(p)
			OPTIONAL MATCH (p)-[:SHARED_FROM]->(origin:Post)<-[:POSTED]-(originAuthor:User)
			OPTIONAL MATCH (viewer)-[originFriendship:FRIEND]-(originAuthor)
			OPTIONAL MATCH (viewer)-[block:BLOCK]-(originAuthor)

			WITH p, author, liked, friendship, origin, originAuthor, originFriendship, block, viewer,
				 CASE
				   WHEN origin IS NULL THEN true
				   WHEN origin.deletedAt IS NOT NULL THEN false
				   WHEN block IS NOT NULL THEN false
				   WHEN origin.privacy = 'PUBLIC' THEN true
				   WHEN origin.privacy = 'FRIEND' AND (viewer.id = originAuthor.id OR originFriendship IS NOT NULL) THEN true
				   WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true
				   ELSE false
				 END AS originCanView

			RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
			       author.id, coalesce(p.likeCount, 0), liked IS NOT NULL,
			       p.files, coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
			       origin.id, originAuthor.id, originCanView, friendship IS NOT NULL,
				   origin.content, origin.createdAt, origin.updatedAt, origin.privacy, origin.files
			ORDER BY p.createdAt DESC
			SKIP $skip LIMIT $limit
		`
	default:
		// Java-like scoring (exact match)
		query = `
			MATCH (viewer:User {id: $currentUserID})
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL
			  AND NOT (viewer)-[:BLOCK]-(author)
			  AND (
				  p.privacy = 'PUBLIC' 
				  OR author.id = viewer.id 
				  OR (p.privacy = 'FRIEND' AND EXISTS((viewer)-[:FRIEND]->(author)))
			  )
			
			OPTIONAL MATCH (viewer)-[friendship:FRIEND]->(author)
			OPTIONAL MATCH path = shortestPath((viewer)-[*1..4]->(p))
			
			OPTIONAL MATCH (viewer)-[vu:VIEW_PROFILE]->(author)
			OPTIONAL MATCH (author)-[uv:VIEW_PROFILE]->(viewer)
			
			OPTIONAL MATCH (p)-[:SHARED_FROM]->(origin:Post)<-[:POSTED]-(originAuthor:User)
			OPTIONAL MATCH (viewer)-[block:BLOCK]-(originAuthor)
			OPTIONAL MATCH (viewer)-[originFriendship:FRIEND]->(originAuthor)
			
			OPTIONAL MATCH (p)-[:HAS_KEYWORDS]->(keyword:Keyword)
			OPTIONAL MATCH (viewer)-[inter:INTERACT_WITH]->(keyword)
			
			OPTIONAL MATCH (viewer)-[liked:LIKED]->(p)
			OPTIONAL MATCH (viewer)-[loaded:LOADED]->(p)

			WITH viewer, p, author, friendship, loaded,
				 CASE WHEN path IS NULL THEN NULL ELSE length(path) END AS shortestPathLength,
				 coalesce(vu.times, 0) AS viewForward,
				 coalesce(uv.times, 0) AS viewBackward,
				 origin, originAuthor, block, originFriendship,
				 liked,
				 COALESCE(SUM(inter.score), 0) AS keywordScore,
				 CASE
				   WHEN origin IS NULL THEN true
				   WHEN origin.deletedAt IS NOT NULL THEN false
				   WHEN block IS NOT NULL THEN false
				   WHEN origin.privacy = 'PUBLIC' THEN true
				   WHEN origin.privacy = 'FRIEND' AND (viewer.id = originAuthor.id OR originFriendship IS NOT NULL) THEN true
				   WHEN origin.privacy = 'PRIVATE' AND viewer.id = originAuthor.id THEN true
				   ELSE false
				 END AS originCanView,
				 CASE
					WHEN p.createdAt > datetime() - duration('P1D')
					THEN 240 - duration.between(p.createdAt, datetime()).hours * 10
					ELSE 0
				 END AS newPostScore,
				 coalesce(p.likeCount, 0) * 2 AS likeScore,
				 coalesce(p.commentCount, 0) * 3 AS commentScore,
				 coalesce(p.shareCount, 0) * 5 AS shareScore,
				 CASE WHEN loaded IS NOT NULL THEN loaded.times * (-20) ELSE 0 END AS loadedScore

			WITH p, author, viewer, origin, originAuthor, liked,
				 originCanView, shortestPathLength, viewForward, viewBackward, newPostScore, likeScore, commentScore, shareScore, friendship, loadedScore, keywordScore,
				 CASE
					 WHEN friendship IS NOT NULL THEN 100
					 WHEN (viewer)-[:FRIEND]-()-[:FRIEND]-(author) AND friendship IS NULL OR (viewer)-[:REQUEST]-(author) THEN 50
					 ELSE 0
				 END
				 + 2 * viewForward
				 + 1 * viewBackward AS relationshipScore,
				 CASE
					 WHEN shortestPathLength IS NULL OR shortestPathLength = 1 THEN 0
					 ELSE 120.0 / shortestPathLength
				 END AS pathScore

			WITH p, author, viewer, origin, originAuthor, liked, loadedScore,
				 originCanView, friendship, keywordScore,
				 pathScore + newPostScore + relationshipScore + likeScore + commentScore + shareScore + loadedScore + keywordScore AS totalScore

			ORDER BY totalScore DESC, p.createdAt DESC
			SKIP $skip LIMIT $limit

			WITH p, author, viewer, friendship, origin, originAuthor, originCanView, liked IS NOT NULL AS isLikedByMe

			MERGE (viewer)-[l:LOADED]->(p)
			ON CREATE SET l.times = 1
			ON MATCH SET l.times = coalesce(l.times, 0) + 1

			RETURN p.id, p.content, p.privacy, p.createdAt, p.updatedAt,
			       author.id, coalesce(p.likeCount, 0), isLikedByMe,
			       p.files, coalesce(p.commentCount, 0), coalesce(p.shareCount, 0),
			       origin.id, originAuthor.id, originCanView, friendship IS NOT NULL,
				   origin.content, origin.createdAt, origin.updatedAt, origin.privacy, origin.files
		`
	}

	posts, err := s.queryPosts(ctx, currentUserID, query, map[string]interface{}{
		"currentUserID": currentUserID,
		"skip":          pageable.Skip,
		"limit":         normalizeLimit(pageable.Limit),
	})
	if err == nil && pageType == PageTypeRelevant && s.KeywordInteractor != nil {
		ids := make([]string, 0, len(posts))
		for _, p := range posts {
			ids = append(ids, p.ID)
		}
		_ = s.KeywordInteractor.PostsLoaded(ctx, ids, currentUserID)
	}
	return posts, err
}

func (s *PostService) UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error {
	if !isValidPostPrivacy(privacy) {
		return errors.New("INVALID_POST_PRIVACY")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (author:User)-[:POSTED]->(p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			RETURN author.id, p.privacy
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{"postID": postID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		vals := res.Record().Values
		if getStringVal(vals[0]) != currentUserID {
			return nil, errors.New("UNAUTHORIZED")
		}
		if getStringVal(vals[1]) == privacy {
			return nil, errors.New("PRIVACY_UNCHANGED")
		}

		updateQuery := `
			MATCH (p:Post {id: $postID})
			SET p.privacy = $privacy, p.updatedAt = datetime()
			RETURN p.id
		`
		_, err = tx.Run(ctx, updateQuery, map[string]interface{}{
			"postID":  postID,
			"privacy": privacy,
		})
		return nil, err
	})
	return err
}

func (s *PostService) UpdateContent(ctx context.Context, currentUserID, postID string, content *string, newFileIDs []string, deleteOldFileIDs []string) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	var deletedFiles []string
	var finalContent string

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		getQuery := `
			MATCH (author:User)-[:POSTED]->(p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			OPTIONAL MATCH (p)-[:SHARED_FROM]->(:Post)
			RETURN author.id, p.content, p.files, count(*) > 0
		`
		res, err := tx.Run(ctx, getQuery, map[string]interface{}{"postID": postID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		vals := res.Record().Values
		if getStringVal(vals[0]) != currentUserID {
			return nil, errors.New("UNAUTHORIZED")
		}

		oldContent := getStringVal(vals[1])
		oldFiles := getStringSliceVal(vals[2])
		isShared := vals[3].(bool)

		trimmed := ""
		if content != nil {
			trimmed = strings.TrimSpace(*content)
			if len(trimmed) > MaxPostContentLength {
				return nil, errors.New("INVALID_POST_CONTENT_LENGTH")
			}
		}

		if isShared {
			if content == nil || trimmed == oldContent {
				return nil, errors.New("POST_CONTENT_UNCHANGED")
			}
			finalContent = trimmed
			_, err = tx.Run(ctx, `
				MATCH (p:Post {id: $postID})
				SET p.content = $content, p.updatedAt = datetime()
				RETURN p.id
			`, map[string]interface{}{"postID": postID, "content": finalContent})
			return nil, err
		}

		oldSet := make(map[string]bool, len(oldFiles))
		for _, id := range oldFiles {
			oldSet[id] = true
		}
		for _, id := range deleteOldFileIDs {
			if !oldSet[id] {
				return nil, errors.New("INVALID_DELETE_ATTACHMENT")
			}
		}

		remaining := make([]string, 0, len(oldFiles))
		deleteSet := make(map[string]bool, len(deleteOldFileIDs))
		for _, id := range deleteOldFileIDs {
			deleteSet[id] = true
		}
		for _, id := range oldFiles {
			if !deleteSet[id] {
				remaining = append(remaining, id)
			}
		}
		finalFiles := append(remaining, newFileIDs...)

		if len(finalFiles) > MaxPostAttachFiles {
			return nil, errors.New("INVALID_NUMBER_OF_POST_ATTACHMENTS")
		}

		if content == nil {
			finalContent = oldContent
		} else {
			finalContent = trimmed
		}
		if finalContent == "" && len(finalFiles) == 0 {
			return nil, errors.New("POST_CONTENT_AND_ATTACH_FILES_BOTH_EMPTY")
		}
		if finalContent == oldContent && stringSliceEqual(finalFiles, oldFiles) {
			return nil, errors.New("POST_CONTENT_UNCHANGED")
		}

		_, err = tx.Run(ctx, `
			MATCH (p:Post {id: $postID})
			SET p.content = $content, p.files = $files, p.updatedAt = datetime()
			RETURN p.id
		`, map[string]interface{}{
			"postID":  postID,
			"content": finalContent,
			"files":   finalFiles,
		})
		deletedFiles = deleteOldFileIDs
		return nil, err
	})
	if err != nil {
		return err
	}

	if len(deletedFiles) > 0 && s.FileClient != nil {
		_ = s.FileClient.DeleteFiles(ctx, deletedFiles)
	}
	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.ExtractPostKeywords(ctx, postID, finalContent, true)
	}
	return nil
}

func (s *PostService) LikePost(ctx context.Context, userID, postID string) error {
	post, err := s.GetPost(ctx, postID, userID)
	if err != nil {
		return err
	}
	if err := s.ValidateBlockByIDs(ctx, userID, post.AuthorID); err != nil {
		return err
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		check, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (p:Post {id: $postID})
			RETURN EXISTS((u)-[:LIKED]->(p))
		`, map[string]interface{}{"userID": userID, "postID": postID})
		if err != nil {
			return nil, err
		}
		if !check.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		if check.Record().Values[0].(bool) {
			return nil, errors.New("LIKED_POST")
		}

		_, err = tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (p:Post {id: $postID})
			MERGE (u)-[:LIKED]->(p)
			SET p.likeCount = coalesce(p.likeCount, 0) + 1
			RETURN p.id
		`, map[string]interface{}{"userID": userID, "postID": postID})
		return nil, err
	})
	if err != nil {
		return err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, postID, userID, "LIKE_SCORE")
	}
	if s.Notification != nil && userID != post.AuthorID {
		_ = s.Notification.Send(ctx, "LIKE_POST", userID, post.AuthorID, postID, "POST", "")
	}
	return nil
}

func (s *PostService) UnlikePost(ctx context.Context, userID, postID string) error {
	post, err := s.GetPost(ctx, postID, userID)
	if err != nil {
		return err
	}
	if err := s.ValidateBlockByIDs(ctx, userID, post.AuthorID); err != nil {
		return err
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		check, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (p:Post {id: $postID})
			RETURN EXISTS((u)-[:LIKED]->(p))
		`, map[string]interface{}{"userID": userID, "postID": postID})
		if err != nil {
			return nil, err
		}
		if !check.Next(ctx) || !check.Record().Values[0].(bool) {
			return nil, errors.New("NOT_LIKED_POST")
		}

		_, err = tx.Run(ctx, `
			MATCH (u:User {id: $userID})-[r:LIKED]->(p:Post {id: $postID})
			DELETE r
			SET p.likeCount = CASE WHEN coalesce(p.likeCount, 0) > 0 THEN p.likeCount - 1 ELSE 0 END
			RETURN p.id
		`, map[string]interface{}{"userID": userID, "postID": postID})
		return nil, err
	})
	return err
}

func (s *PostService) DeletePost(ctx context.Context, postID, currentUserID string, isAdmin bool) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	var authorID string
	var files []string

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, `
			MATCH (author:User)-[:POSTED]->(p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			RETURN author.id, p.files
		`, map[string]interface{}{"postID": postID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		authorID = getStringVal(res.Record().Values[0])
		files = getStringSliceVal(res.Record().Values[1])

		if !isAdmin && authorID != currentUserID {
			return nil, errors.New("UNAUTHORIZED")
		}

		_, err = tx.Run(ctx, `
			MATCH (p:Post {id: $postID})
			SET p.deletedAt = datetime(), p.files = []
			RETURN p.id
		`, map[string]interface{}{"postID": postID})
		return nil, err
	})
	if err != nil {
		return err
	}

	if len(files) > 0 && s.FileClient != nil {
		_ = s.FileClient.DeleteFiles(ctx, files)
	}
	if isAdmin && s.Notification != nil && authorID != currentUserID {
		_ = s.Notification.Send(ctx, "DELETE_POST", currentUserID, authorID, postID, "POST", "")
	}
	return nil
}

func (s *PostService) ValidateViewPost(ctx context.Context, post *model.Post, viewerID string) error {
	if post == nil {
		return errors.New("POST_NOT_FOUND")
	}
	if post.AuthorID == viewerID {
		return nil
	}
	switch post.Privacy {
	case PostPrivacyPublic:
		if viewerID != "" {
			return s.ValidateBlockByIDs(ctx, viewerID, post.AuthorID)
		}
		return nil
	case PostPrivacyFriend:
		if viewerID == "" {
			return errors.New("UNAUTHORIZED")
		}
		isFriend, err := s.IsFriendByIDs(ctx, viewerID, post.AuthorID)
		if err != nil {
			return err
		}
		if !isFriend {
			return errors.New("UNAUTHORIZED")
		}
		return nil
	default:
		return errors.New("UNAUTHORIZED")
	}
}

func (s *PostService) ValidateBlockByIDs(ctx context.Context, userID, targetID string) error {
	if userID == "" || targetID == "" || userID == targetID {
		return nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (t:User {id: $targetID})
			RETURN EXISTS((u)-[:BLOCK]->(t)), EXISTS((t)-[:BLOCK]->(u))
		`, map[string]interface{}{"userID": userID, "targetID": targetID})
		if err != nil {
			return nil, err
		}
		if !result.Next(ctx) {
			return []bool{false, false}, nil
		}
		return []bool{result.Record().Values[0].(bool), result.Record().Values[1].(bool)}, nil
	})
	if err != nil {
		return err
	}
	status := res.([]bool)
	if status[0] {
		return errors.New("HAS_BLOCKED")
	}
	if status[1] {
		return errors.New("HAS_BEEN_BLOCKED")
	}
	return nil
}

func (s *PostService) ValidateBlockByUsername(ctx context.Context, userID, targetUsername string) error {
	if userID == "" || targetUsername == "" {
		return nil
	}
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (t:User {username: $targetUsername})
			RETURN EXISTS((u)-[:BLOCK]->(t)), EXISTS((t)-[:BLOCK]->(u))
		`, map[string]interface{}{"userID": userID, "targetUsername": targetUsername})
		if err != nil {
			return nil, err
		}
		if !result.Next(ctx) {
			return nil, errors.New("USER_NOT_FOUND")
		}
		return []bool{result.Record().Values[0].(bool), result.Record().Values[1].(bool)}, nil
	})
	if err != nil {
		return err
	}
	status := res.([]bool)
	if status[0] {
		return errors.New("HAS_BLOCKED")
	}
	if status[1] {
		return errors.New("HAS_BEEN_BLOCKED")
	}
	return nil
}

func (s *PostService) IsFriendByIDs(ctx context.Context, userID, targetID string) (bool, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (t:User {id: $targetID})
			RETURN EXISTS((u)-[:FRIEND]-(t))
		`, map[string]interface{}{"userID": userID, "targetID": targetID})
		if err != nil {
			return nil, err
		}
		if !result.Next(ctx) {
			return false, nil
		}
		return result.Record().Values[0].(bool), nil
	})
	if err != nil {
		return false, err
	}
	return res.(bool), nil
}

func (s *PostService) Comment(ctx context.Context, authorID, postID, content string, fileID *string) (*model.Comment, error) {
	content = strings.TrimSpace(content)
	if err := validateCommentContent(content, fileID); err != nil {
		return nil, err
	}

	post, err := s.GetPost(ctx, postID, authorID)
	if err != nil {
		return nil, err
	}

	commentID := uuid.NewString()
	files := []string{}
	if fileID != nil && *fileID != "" {
		files = append(files, *fileID)
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (author:User {id: $authorID}), (p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			CREATE (c:Comment {
				id: $commentID,
				content: $content,
				file: $file,
				likeCount: 0,
				replyCount: 0,
				createdAt: datetime(),
				updatedAt: null
			})
			CREATE (author)-[:COMMENTED]->(c)
			CREATE (c)-[:COMMENT_OF]->(p)
			SET p.commentCount = coalesce(p.commentCount, 0) + 1
			RETURN c.id
		`
		fileVal := ""
		if fileID != nil {
			fileVal = *fileID
		}
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"authorID":  authorID,
			"postID":    postID,
			"commentID": commentID,
			"content":   content,
			"file":      fileVal,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		return nil, res.Err()
	})
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, postID, authorID, "COMMENT_SCORE")
	}
	if s.Notification != nil && authorID != post.AuthorID {
		_ = s.Notification.Send(ctx, "COMMENT", authorID, post.AuthorID, commentID, "COMMENT", truncateByWord(content))
	}

	return s.GetCommentByID(ctx, commentID, authorID)
}

func (s *PostService) ReplyComment(ctx context.Context, authorID, originalCommentID, content string, fileID *string) (*model.Comment, error) {
	content = strings.TrimSpace(content)
	if err := validateCommentContent(content, fileID); err != nil {
		return nil, err
	}

	original, err := s.GetCommentByID(ctx, originalCommentID, authorID)
	if err != nil {
		return nil, err
	}
	if original.OriginalCommentID != "" {
		return nil, errors.New("CAN_NOT_REPLY_REPLIED_COMMENT")
	}

	post, err := s.GetPost(ctx, original.PostID, authorID)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateBlockByIDs(ctx, post.AuthorID, authorID); err != nil {
		return nil, err
	}

	commentID := uuid.NewString()
	fileVal := ""
	if fileID != nil {
		fileVal = *fileID
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (author:User {id: $authorID}), (origin:Comment {id: $originalCommentID}), (p:Post {id: $postID})
			CREATE (c:Comment {
				id: $commentID,
				content: $content,
				file: $file,
				likeCount: 0,
				replyCount: 0,
				createdAt: datetime(),
				updatedAt: null
			})
			CREATE (author)-[:COMMENTED]->(c)
			CREATE (c)-[:REPLY_OF]->(origin)
			CREATE (c)-[:COMMENT_OF]->(p)
			SET origin.replyCount = coalesce(origin.replyCount, 0) + 1,
			    p.commentCount = coalesce(p.commentCount, 0) + 1
			RETURN c.id
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"authorID":          authorID,
			"originalCommentID": originalCommentID,
			"postID":            original.PostID,
			"commentID":         commentID,
			"content":           content,
			"file":              fileVal,
		})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("COMMENT_NOT_FOUND")
		}
		return nil, res.Err()
	})
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, original.PostID, authorID, "COMMENT_SCORE")
	}
	if s.Notification != nil {
		if authorID != post.AuthorID {
			_ = s.Notification.Send(ctx, "COMMENT", authorID, post.AuthorID, commentID, "COMMENT", truncateByWord(content))
		}
		if authorID != original.AuthorID {
			_ = s.Notification.Send(ctx, "REPLY_COMMENT", authorID, original.AuthorID, commentID, "COMMENT", truncateByWord(original.Content))
		}
	}

	return s.GetCommentByID(ctx, commentID, authorID)
}

func (s *PostService) LikeComment(ctx context.Context, likerID, commentID string) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		check, err := tx.Run(ctx, `
			MATCH (u:User {id: $likerID}), (c:Comment {id: $commentID})
			RETURN EXISTS((u)-[:LIKED]->(c))
		`, map[string]interface{}{"likerID": likerID, "commentID": commentID})
		if err != nil {
			return nil, err
		}
		if !check.Next(ctx) {
			return nil, errors.New("COMMENT_NOT_FOUND")
		}
		if check.Record().Values[0].(bool) {
			return nil, errors.New("LIKED_COMMENT")
		}

		_, err = tx.Run(ctx, `
			MATCH (u:User {id: $likerID}), (c:Comment {id: $commentID})
			MERGE (u)-[:LIKED]->(c)
			SET c.likeCount = coalesce(c.likeCount, 0) + 1
			RETURN c.id
		`, map[string]interface{}{"likerID": likerID, "commentID": commentID})
		return nil, err
	})
	if err != nil {
		return err
	}

	if s.Notification != nil {
		_ = s.Notification.SendToFriends(ctx, "LIKE_COMMENT", likerID, commentID, "COMMENT", "")
	}
	return nil
}

func (s *PostService) UnlikeComment(ctx context.Context, likerID, commentID string) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		check, err := tx.Run(ctx, `
			MATCH (u:User {id: $likerID}), (c:Comment {id: $commentID})
			RETURN EXISTS((u)-[:LIKED]->(c))
		`, map[string]interface{}{"likerID": likerID, "commentID": commentID})
		if err != nil {
			return nil, err
		}
		if !check.Next(ctx) || !check.Record().Values[0].(bool) {
			return nil, errors.New("NOT_LIKED_COMMENT")
		}

		_, err = tx.Run(ctx, `
			MATCH (u:User {id: $likerID})-[r:LIKED]->(c:Comment {id: $commentID})
			DELETE r
			SET c.likeCount = CASE WHEN coalesce(c.likeCount, 0) > 0 THEN c.likeCount - 1 ELSE 0 END
			RETURN c.id
		`, map[string]interface{}{"likerID": likerID, "commentID": commentID})
		return nil, err
	})
	return err
}

func (s *PostService) UpdateCommentContent(ctx context.Context, currentUserID, commentID, content string) (*model.Comment, error) {
	content = strings.TrimSpace(content)
	if content == "" || len(content) > MaxCommentContentLength {
		return nil, errors.New("INVALID_COMMENT_CONTENT_LENGTH")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, `
			MATCH (author:User)-[:COMMENTED]->(c:Comment {id: $commentID})
			RETURN author.id, c.content
		`, map[string]interface{}{"commentID": commentID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("COMMENT_NOT_FOUND")
		}
		if getStringVal(res.Record().Values[0]) != currentUserID {
			return nil, errors.New("UNAUTHORIZED")
		}
		if getStringVal(res.Record().Values[1]) == content {
			return nil, errors.New("COMMENT_CONTENT_UNCHANGED")
		}
		_, err = tx.Run(ctx, `
			MATCH (c:Comment {id: $commentID})
			SET c.content = $content, c.updatedAt = datetime()
			RETURN c.id
		`, map[string]interface{}{"commentID": commentID, "content": content})
		return nil, err
	})
	if err != nil {
		return nil, err
	}
	return s.GetCommentByID(ctx, commentID, currentUserID)
}

func (s *PostService) DeleteComment(ctx context.Context, currentUserID, commentID string, isAdmin bool) error {
	comment, err := s.GetCommentByID(ctx, commentID, currentUserID)
	if err != nil {
		return err
	}
	post, err := s.GetPost(ctx, comment.PostID, currentUserID)
	if err != nil {
		return err
	}

	isPostAuthor := post.AuthorID == currentUserID
	isCommentAuthor := comment.AuthorID == currentUserID
	if !isAdmin && !isPostAuthor && !isCommentAuthor {
		return errors.New("UNAUTHORIZED")
	}

	var deletedFiles []string
	var notifyAuthorID string

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		if comment.OriginalCommentID != "" {
			deletedFiles = append(deletedFiles, comment.Files...)
			_, err := tx.Run(ctx, `
				MATCH (c:Comment {id: $commentID})-[:REPLY_OF]->(origin:Comment)
				OPTIONAL MATCH (c)-[r]-()
				DELETE r, c
				SET origin.replyCount = CASE WHEN coalesce(origin.replyCount, 0) > 0 THEN origin.replyCount - 1 ELSE 0 END
				WITH origin
				MATCH (p:Post {id: $postID})
				SET p.commentCount = CASE WHEN coalesce(p.commentCount, 0) > 0 THEN p.commentCount - 1 ELSE 0 END
				RETURN origin.id
			`, map[string]interface{}{"commentID": commentID, "postID": post.ID})
			return nil, err
		}

		res, err := tx.Run(ctx, `
			MATCH (c:Comment {id: $commentID})-[:COMMENT_OF]->(p:Post)
			OPTIONAL MATCH (reply:Comment)-[:REPLY_OF]->(c)
			RETURN collect(reply.id), collect(reply.file), c.file
		`, map[string]interface{}{"commentID": commentID})
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return nil, errors.New("COMMENT_NOT_FOUND")
		}

		replyIDs := getStringSliceVal(res.Record().Values[0])
		replyFiles := getStringSliceVal(res.Record().Values[1])
		deletedFiles = append(deletedFiles, replyFiles...)
		if f := getStringVal(res.Record().Values[2]); f != "" {
			deletedFiles = append(deletedFiles, f)
		}

		deleteCount := int64(len(replyIDs) + 1)
		_, err = tx.Run(ctx, `
			MATCH (c:Comment {id: $commentID})-[:COMMENT_OF]->(p:Post)
			OPTIONAL MATCH (reply:Comment)-[:REPLY_OF]->(c)
			DETACH DELETE reply, c
			SET p.commentCount = CASE WHEN coalesce(p.commentCount, 0) >= $deleteCount THEN p.commentCount - $deleteCount ELSE 0 END
			RETURN p.id
		`, map[string]interface{}{"commentID": commentID, "deleteCount": deleteCount})
		return nil, err
	})
	if err != nil {
		return err
	}

	if len(deletedFiles) > 0 && s.FileClient != nil {
		_ = s.FileClient.DeleteFiles(ctx, deletedFiles)
	}
	if (isAdmin || isPostAuthor) && !isCommentAuthor && s.Notification != nil {
		notifyAuthorID = comment.AuthorID
		_ = s.Notification.Send(ctx, "DELETE_COMMENT", currentUserID, notifyAuthorID, commentID, "COMMENT", "")
	}
	return nil
}

func (s *PostService) GetComments(ctx context.Context, postID, currentUserID string, pageable Pageable) ([]*model.Comment, error) {
	post, err := s.GetPost(ctx, postID, currentUserID)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateViewPost(ctx, post, currentUserID); err != nil {
		return nil, err
	}

	pageType := pageable.Type
	if pageType == "" {
		pageType = PageTypeRelevant
	}

	order := "ORDER BY c.createdAt DESC"
	whereFriend := ""
	if pageType == PageTypeRelevant {
		order = "ORDER BY coalesce(c.likeCount, 0) DESC, c.createdAt DESC"
	} else if pageType == PageTypeFriendOnly {
		whereFriend = "AND EXISTS((viewer)-[:FRIEND]-(author))"
	}

	query := fmt.Sprintf(`
		MATCH (p:Post {id: $postID})<-[:COMMENT_OF]-(c:Comment)<-[:COMMENTED]-(author:User)
		MATCH (viewer:User {id: $currentUserID})
		WHERE p.deletedAt IS NULL %s
		OPTIONAL MATCH (viewer)-[liked:LIKED]->(c)
		RETURN c.id, c.content, c.file, c.createdAt, c.updatedAt,
		       author.id, coalesce(c.likeCount, 0), coalesce(c.replyCount, 0),
		       count(liked) > 0, p.id, ''
		%s
		SKIP $skip LIMIT $limit
	`, whereFriend, order)

	return s.queryComments(ctx, currentUserID, query, map[string]interface{}{
		"postID":        postID,
		"currentUserID": currentUserID,
		"skip":          pageable.Skip,
		"limit":         normalizeLimit(pageable.Limit),
	})
}

func (s *PostService) GetRepliedComments(ctx context.Context, originalCommentID, currentUserID string, pageable Pageable) ([]*model.Comment, error) {
	query := `
		MATCH (origin:Comment {id: $commentID})<-[:REPLY_OF]-(c:Comment)<-[:COMMENTED]-(author:User)
		OPTIONAL MATCH (:User {id: $currentUserID})-[liked:LIKED]->(c)
		OPTIONAL MATCH (origin)-[:COMMENT_OF]->(p1:Post)
		OPTIONAL MATCH (origin)-[:REPLY_OF]->(:Comment)-[:COMMENT_OF]->(p2:Post)
		WITH c, author, liked, coalesce(p1, p2) AS p
		RETURN c.id, c.content, c.file, c.createdAt, c.updatedAt,
		       author.id, coalesce(c.likeCount, 0), coalesce(c.replyCount, 0),
		       count(liked) > 0, p.id, $commentID
		ORDER BY c.createdAt ASC
		SKIP $skip LIMIT $limit
	`
	return s.queryComments(ctx, currentUserID, query, map[string]interface{}{
		"commentID":     originalCommentID,
		"currentUserID": currentUserID,
		"skip":          pageable.Skip,
		"limit":         normalizeLimit(pageable.Limit),
	})
}

func (s *PostService) GetCommentByID(ctx context.Context, commentID string, currentUserID string) (*model.Comment, error) {
	query := `
		MATCH (author:User)-[:COMMENTED]->(c:Comment {id: $commentID})
		OPTIONAL MATCH (c)-[:COMMENT_OF]->(p1:Post)
		OPTIONAL MATCH (c)-[:REPLY_OF]->(origin:Comment)
		OPTIONAL MATCH (origin)-[:COMMENT_OF]->(p2:Post)
		OPTIONAL MATCH (:User {id: $currentUserID})-[liked:LIKED]->(c)
		WITH author, c, origin, coalesce(p1, p2) AS p, liked
		RETURN c.id, c.content, c.file, c.createdAt, c.updatedAt,
		       author.id, coalesce(c.likeCount, 0), coalesce(c.replyCount, 0),
		       count(liked) > 0, p.id, coalesce(origin.id, '')
	`
	comments, err := s.queryComments(ctx, currentUserID, query, map[string]interface{}{
		"commentID":     commentID,
		"currentUserID": currentUserID,
		"skip":          int64(0),
		"limit":         int64(1),
	})
	if err != nil {
		return nil, err
	}
	if len(comments) == 0 {
		return nil, errors.New("COMMENT_NOT_FOUND")
	}
	return comments[0], nil
}

func (s *PostService) GetFilesInPostsOfUser(ctx context.Context, username, currentUserID string, pageable Pageable) ([]string, error) {
	if err := s.ValidateBlockByUsername(ctx, currentUserID, username); err != nil {
		return nil, err
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, `
			MATCH (author:User {username: $username})-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL
			UNWIND coalesce(p.files, []) AS fileID
			RETURN fileID
			SKIP $skip LIMIT $limit
		`, map[string]interface{}{
			"username": username,
			"skip":     pageable.Skip,
			"limit":    normalizeLimit(pageable.Limit),
		})
		if err != nil {
			return nil, err
		}
		files := make([]string, 0)
		for result.Next(ctx) {
			files = append(files, getStringVal(result.Record().Values[0]))
		}
		return files, result.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]string), nil
}

func (s *PostService) queryPosts(ctx context.Context, currentUserID string, query string, params map[string]interface{}) ([]*model.Post, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		posts := make([]*model.Post, 0)
		for result.Next(ctx) {
			post := s.postFromRecord(ctx, result.Record().Values)
			// Secondary validation in Go for peace of mind
			if err := s.ValidateViewPost(ctx, post, currentUserID); err == nil {
				posts = append(posts, post)
			}
		}
		return posts, result.Err()
	})
	if err != nil {
		return nil, err
	}
	posts := res.([]*model.Post)
	s.enrichPostsWithPresignedURLs(ctx, posts)
	return posts, nil
}

func (s *PostService) postFromRecord(ctx context.Context, vals []interface{}) *model.Post {
	p := &model.Post{
		ID:           getStringVal(vals[0]),
		Content:      getStringVal(vals[1]),
		Privacy:      getStringVal(vals[2]),
		AuthorID:     getStringVal(vals[5]),
		LikeCount:    getIntVal(vals[6]),
		Liked:        false,
		Files:        make([]string, 0),
		Images:       make([]string, 0),
		CommentCount: getIntVal(vals[9]),
		ShareCount:   getIntVal(vals[10]),
	}
	p.Files = getStringSliceVal(vals[8])
	p.Images = p.Files

	if len(vals) > 7 {
		if v, ok := vals[7].(bool); ok {
			p.Liked = v
		}
	}
	if vals[3] != nil {
		switch v := vals[3].(type) {
		case dbtype.LocalDateTime:
			p.CreatedAt = v.Time()
		case time.Time:
			p.CreatedAt = v
		}
	}
	if vals[4] != nil {
		switch v := vals[4].(type) {
		case dbtype.LocalDateTime:
			t := v.Time()
			p.UpdatedAt = &t
		case time.Time:
			p.UpdatedAt = &v
		}
	}
	p.Author = s.ResolveAuthor(ctx, p.AuthorID)

	// Original post info
	if len(vals) > 11 && vals[11] != nil && getStringVal(vals[11]) != "" {
		p.SharedPost = true
		p.OriginalPostID = getStringVal(vals[11])
		p.OriginalAuthorID = getStringVal(vals[12])
		p.OriginalPostCanView = vals[13].(bool)

		if p.OriginalPostCanView {
			origCreatedAt := time.Time{}
			if vals[16] != nil {
				switch v := vals[16].(type) {
				case dbtype.LocalDateTime:
					origCreatedAt = v.Time()
				case time.Time:
					origCreatedAt = v
				}
			}
			var origUpdatedAt *time.Time
			if vals[17] != nil {
				switch v := vals[17].(type) {
				case dbtype.LocalDateTime:
					t := v.Time()
					origUpdatedAt = &t
				case time.Time:
					origUpdatedAt = &v
				}
			}

			p.OriginalPost = &model.Post{
				ID:        p.OriginalPostID,
				Content:   getStringVal(vals[15]),
				CreatedAt: origCreatedAt,
				UpdatedAt: origUpdatedAt,
				Privacy:   getStringVal(vals[18]),
				Files:     make([]string, 0),
				Images:    make([]string, 0),
				AuthorID:  p.OriginalAuthorID,
				Author:    s.ResolveAuthor(ctx, p.OriginalAuthorID),
			}
			p.OriginalPost.Files = getStringSliceVal(vals[19])
			p.OriginalPost.Images = p.OriginalPost.Files
		}
	}
	return p
}

func (s *PostService) queryComments(ctx context.Context, currentUserID string, query string, params map[string]interface{}) ([]*model.Comment, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		comments := make([]*model.Comment, 0)
		for result.Next(ctx) {
			vals := result.Record().Values
			fileID := getStringVal(vals[2])
			files := []string{}
			if fileID != "" {
				files = append(files, fileID)
			}
			c := &model.Comment{
				ID:                getStringVal(vals[0]),
				Content:           getStringVal(vals[1]),
				Files:             files,
				AuthorID:          getStringVal(vals[5]),
				LikeCount:         getIntVal(vals[6]),
				ReplyCount:        getIntVal(vals[7]),
				Liked:             false,
				PostID:            getStringVal(vals[9]),
				OriginalCommentID: getStringVal(vals[10]),
			}
			if fileID != "" {
				c.FileUrl = fileID
			}
			if v, ok := vals[8].(bool); ok {
				c.Liked = v
			}
			if vals[3] != nil {
				switch v := vals[3].(type) {
				case dbtype.LocalDateTime:
					c.CreatedAt = v.Time()
				case time.Time:
					c.CreatedAt = v
				}
			}
			if vals[4] != nil {
				switch v := vals[4].(type) {
				case dbtype.LocalDateTime:
					t := v.Time()
					c.UpdatedAt = &t
				case time.Time:
					c.UpdatedAt = &v
				}
			}
			c.Author = s.ResolveAuthor(ctx, c.AuthorID)
			comments = append(comments, c)
		}
		return comments, result.Err()
	})
	if err != nil {
		return nil, err
	}
	comments := res.([]*model.Comment)
	s.enrichCommentsWithPresignedURLs(ctx, comments)
	return comments, nil
}

func validatePostRequest(content string, files []string) error {
	if strings.TrimSpace(content) == "" && len(files) == 0 {
		return errors.New("POST_CONTENT_AND_ATTACH_FILES_BOTH_EMPTY")
	}
	if len(content) > MaxPostContentLength {
		return errors.New("INVALID_POST_CONTENT_LENGTH")
	}
	if len(files) > MaxPostAttachFiles {
		return errors.New("INVALID_NUMBER_OF_POST_ATTACHMENTS")
	}
	return nil
}

func validateCommentContent(content string, fileID *string) error {
	hasNoAttachment := fileID == nil || strings.TrimSpace(*fileID) == ""
	if strings.TrimSpace(content) == "" && hasNoAttachment {
		return errors.New("COMMENT_CONTENT_AND_ATTACH_FILE_BOTH_EMPTY")
	}
	if len(content) > MaxCommentContentLength {
		return errors.New("INVALID_COMMENT_CONTENT_LENGTH")
	}
	return nil
}

func isValidPostPrivacy(privacy string) bool {
	return privacy == PostPrivacyPublic || privacy == PostPrivacyFriend || privacy == PostPrivacyPrivate
}

func normalizeLimit(limit int64) int64 {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, v := range a {
		counts[v]++
	}
	for _, v := range b {
		if counts[v] == 0 {
			return false
		}
		counts[v]--
	}
	return true
}

func truncateByWord(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 120 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= 120 {
		return s
	}
	return string(runes[:120]) + "..."
}

func (s *PostService) enrichPostsWithPresignedURLs(ctx context.Context, posts []*model.Post) {
	if s.FileClient == nil || len(posts) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)
	
	addFileID := func(id string) {
		if id != "" && !fileIDSet[id] {
			fileIDs = append(fileIDs, id)
			fileIDSet[id] = true
		}
	}

	for _, post := range posts {
		for _, fileID := range post.Files {
			addFileID(fileID)
		}
		addFileID(post.Author.ProfilePictureUrl)
		if post.OriginalPost != nil {
			for _, fileID := range post.OriginalPost.Files {
				addFileID(fileID)
			}
			addFileID(post.OriginalPost.Author.ProfilePictureUrl)
		}
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for posts: %v", err)
		return
	}

	for _, post := range posts {
		for i, fileID := range post.Files {
			if url, ok := urls[fileID]; ok {
				post.Files[i] = url
			}
		}
		post.Images = post.Files
		if url, ok := urls[post.Author.ProfilePictureUrl]; ok {
			post.Author.ProfilePictureUrl = url
		}
		if post.OriginalPost != nil {
			for i, fileID := range post.OriginalPost.Files {
				if url, ok := urls[fileID]; ok {
					post.OriginalPost.Files[i] = url
				}
			}
			post.OriginalPost.Images = post.OriginalPost.Files
			if url, ok := urls[post.OriginalPost.Author.ProfilePictureUrl]; ok {
				post.OriginalPost.Author.ProfilePictureUrl = url
			}
		}
	}
}

func (s *PostService) enrichCommentsWithPresignedURLs(ctx context.Context, comments []*model.Comment) {
	if s.FileClient == nil || len(comments) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)

	addFileID := func(id string) {
		if id != "" && !fileIDSet[id] {
			fileIDs = append(fileIDs, id)
			fileIDSet[id] = true
		}
	}

	for _, comment := range comments {
		for _, fileID := range comment.Files {
			addFileID(fileID)
		}
		addFileID(comment.Author.ProfilePictureUrl)
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for comments: %v", err)
		return
	}

	for _, comment := range comments {
		for i, fileID := range comment.Files {
			if url, ok := urls[fileID]; ok {
				comment.Files[i] = url
			}
		}
		if url, ok := urls[comment.FileUrl]; ok {
			comment.FileUrl = url
		}
		if url, ok := urls[comment.Author.ProfilePictureUrl]; ok {
			comment.Author.ProfilePictureUrl = url
		}
	}
}
