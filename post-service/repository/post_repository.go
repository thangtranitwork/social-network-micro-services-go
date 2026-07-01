package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"social-network-go/post-service/db"
	"social-network-go/post-service/model"
	"social-network-go/profiler"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

const (
	PostPrivacyPublic  = "PUBLIC"
	PostPrivacyFriend  = "FRIEND"
	PostPrivacyPrivate = "PRIVATE"

	PageTypeRelevant   = "RELEVANT"
	PageTypeFriendOnly = "FRIEND_ONLY"
	PageTypeTime       = "TIME"
)

type PostRepository interface {
	CreatePost(ctx context.Context, postID, authorID, content, privacy string, fileIDs []string) error
	SharePost(ctx context.Context, authorID, originalPostID, content, privacy string, postID string) error
	GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error)
	GetPostsOfUser(ctx context.Context, authorUsername string, currentUserID string, skip, limit int64) ([]*model.Post, error)
	GetAllPosts(ctx context.Context, skip, limit int64) ([]*model.Post, error)
	GetSuggestedPosts(ctx context.Context, currentUserID string, pageType string, skip, limit int64) ([]*model.Post, error)
	UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error
	UpdateContent(ctx context.Context, currentUserID, postID string, content *string, newFileIDs []string, deleteOldFileIDs []string, maxPostAttachFiles int) ([]string, string, error)
	LikePost(ctx context.Context, userID, postID string) (string, error)
	UnlikePost(ctx context.Context, userID, postID string) (string, error)
	DeletePost(ctx context.Context, postID, currentUserID string, isAdmin bool) (string, []string, error)
	ValidateBlockByIDs(ctx context.Context, userID, targetID string) error
	ValidateBlockByUsername(ctx context.Context, userID, targetUsername string) error
	IsFriendByIDs(ctx context.Context, userID, targetID string) (bool, error)
	Comment(ctx context.Context, commentID, authorID, postID, content string, fileID *string) error
	ReplyComment(ctx context.Context, commentID, authorID, originalCommentID, postID, content string, fileID *string) error
	LikeComment(ctx context.Context, likerID, commentID string) error
	UnlikeComment(ctx context.Context, likerID, commentID string) error
	UpdateCommentContent(ctx context.Context, currentUserID, commentID, content string) error
	DeleteComment(ctx context.Context, currentUserID, commentID string, isAdmin bool, postAuthorID string) ([]string, error)
	GetComments(ctx context.Context, postID, currentUserID string, pageType string, skip, limit int64) ([]*model.Comment, error)
	GetRepliedComments(ctx context.Context, originalCommentID, currentUserID string, skip, limit int64) ([]*model.Comment, error)
	GetCommentByID(ctx context.Context, commentID string, currentUserID string) (*model.Comment, error)
	GetFilesInPostsOfUser(ctx context.Context, username string, skip, limit int64) ([]string, error)
}

type Neo4jPostRepository struct{}

func NewPostRepository() PostRepository {
	return &Neo4jPostRepository{}
}

func (r *Neo4jPostRepository) CreatePost(ctx context.Context, postID, authorID, content, privacy string, fileIDs []string) error {
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
	return err
}

func (r *Neo4jPostRepository) SharePost(ctx context.Context, authorID, originalPostID, content, privacy string, postID string) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
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
	return err
}

func (r *Neo4jPostRepository) GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error) {
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
		p := postFromRecord(result.Record().Values)
		return p, result.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.(*model.Post), nil
}

func (r *Neo4jPostRepository) GetPostsOfUser(ctx context.Context, authorUsername string, currentUserID string, skip, limit int64) ([]*model.Post, error) {
	return r.queryPosts(ctx, currentUserID, `
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
		"skip":          skip,
		"limit":         limit,
	})
}

func (r *Neo4jPostRepository) GetAllPosts(ctx context.Context, skip, limit int64) ([]*model.Post, error) {
	return r.queryPosts(ctx, "", `
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
		"skip":  skip,
		"limit": limit,
	})
}

func (r *Neo4jPostRepository) GetSuggestedPosts(ctx context.Context, currentUserID string, pageType string, skip, limit int64) ([]*model.Post, error) {
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
					 originCanView, viewForward, viewBackward, newPostScore, likeScore, commentScore, shareScore, friendship, loadedScore, keywordScore,
					 CASE
						 WHEN friendship IS NOT NULL THEN 100
						 WHEN (viewer)-[:FRIEND]-()-[:FRIEND]-(author) AND friendship IS NULL OR (viewer)-[:REQUEST]-(author) THEN 50
						 ELSE 0
					 END
					 + 2 * viewForward
					 + 1 * viewBackward AS relationshipScore

				WITH p, author, viewer, origin, originAuthor, liked, loadedScore,
					 originCanView, friendship, keywordScore,
					 newPostScore + relationshipScore + likeScore + commentScore + shareScore + loadedScore + keywordScore AS totalScore

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

	return profiler.TrackResult(fmt.Sprintf("post-service:query newsfeed.neo4j.%s", pageType), func() ([]*model.Post, error) {
		return r.queryPosts(ctx, currentUserID, query, map[string]interface{}{
			"currentUserID": currentUserID,
			"skip":          skip,
			"limit":         limit,
		})
	})
}

func (r *Neo4jPostRepository) UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error {
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

func (r *Neo4jPostRepository) UpdateContent(ctx context.Context, currentUserID, postID string, content *string, newFileIDs []string, deleteOldFileIDs []string, maxPostAttachFiles int) ([]string, string, error) {
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
			if len(trimmed) > 5000 { // MaxPostContentLength
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

		if len(finalFiles) > maxPostAttachFiles {
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
	return deletedFiles, finalContent, err
}

func (r *Neo4jPostRepository) LikePost(ctx context.Context, userID, postID string) (string, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		check, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (author:User)-[:POSTED]->(p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			OPTIONAL MATCH (u)-[liked:LIKED]->(p)
			OPTIONAL MATCH (u)-[friendship:FRIEND]-(author)
			OPTIONAL MATCH (u)-[block:BLOCK]-(author)
			WITH u, author, p, liked,
			     CASE
			       WHEN author.id = u.id THEN true
			       WHEN block IS NOT NULL THEN false
			       WHEN p.privacy = 'PUBLIC' THEN true
			       WHEN p.privacy = 'FRIEND' AND friendship IS NOT NULL THEN true
			       ELSE false
			     END AS canView
			RETURN author.id, liked IS NOT NULL, canView
		`, map[string]interface{}{"userID": userID, "postID": postID})
		if err != nil {
			return nil, err
		}
		if !check.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		vals := check.Record().Values
		authorID := getStringVal(vals[0])
		if liked, ok := vals[1].(bool); ok && liked {
			return nil, errors.New("LIKED_POST")
		}
		if canView, ok := vals[2].(bool); !ok || !canView {
			return nil, errors.New("UNAUTHORIZED")
		}

		_, err = tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (p:Post {id: $postID})
			MERGE (u)-[liked:LIKED]->(p)
			ON CREATE SET liked.createdAt = datetime(), p.likeCount = coalesce(p.likeCount, 0) + 1
			RETURN p.id
		`, map[string]interface{}{"userID": userID, "postID": postID})
		return authorID, err
	})
	if err != nil {
		return "", err
	}
	return res.(string), nil
}

func (r *Neo4jPostRepository) UnlikePost(ctx context.Context, userID, postID string) (string, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		check, err := tx.Run(ctx, `
			MATCH (u:User {id: $userID}), (author:User)-[:POSTED]->(p:Post {id: $postID})
			WHERE p.deletedAt IS NULL
			OPTIONAL MATCH (u)-[liked:LIKED]->(p)
			OPTIONAL MATCH (u)-[friendship:FRIEND]-(author)
			OPTIONAL MATCH (u)-[block:BLOCK]-(author)
			WITH u, author, p, liked,
			     CASE
			       WHEN author.id = u.id THEN true
			       WHEN block IS NOT NULL THEN false
			       WHEN p.privacy = 'PUBLIC' THEN true
			       WHEN p.privacy = 'FRIEND' AND friendship IS NOT NULL THEN true
			       ELSE false
			     END AS canView
			RETURN author.id, liked IS NOT NULL, canView
		`, map[string]interface{}{"userID": userID, "postID": postID})
		if err != nil {
			return nil, err
		}
		if !check.Next(ctx) {
			return nil, errors.New("POST_NOT_FOUND")
		}
		vals := check.Record().Values
		authorID := getStringVal(vals[0])
		if canView, ok := vals[2].(bool); !ok || !canView {
			return nil, errors.New("UNAUTHORIZED")
		}
		if liked, ok := vals[1].(bool); !ok || !liked {
			return nil, errors.New("NOT_LIKED_POST")
		}

		_, err = tx.Run(ctx, `
			MATCH (u:User {id: $userID})-[r:LIKED]->(p:Post {id: $postID})
			DELETE r
			SET p.likeCount = CASE WHEN coalesce(p.likeCount, 0) > 0 THEN p.likeCount - 1 ELSE 0 END
			RETURN p.id
		`, map[string]interface{}{"userID": userID, "postID": postID})
		return authorID, err
	})
	if err != nil {
		return "", err
	}
	return res.(string), nil
}

func (r *Neo4jPostRepository) DeletePost(ctx context.Context, postID, currentUserID string, isAdmin bool) (string, []string, error) {
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
	return authorID, files, err
}

func (r *Neo4jPostRepository) ValidateBlockByIDs(ctx context.Context, userID, targetID string) error {
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

func (r *Neo4jPostRepository) ValidateBlockByUsername(ctx context.Context, userID, targetUsername string) error {
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

func (r *Neo4jPostRepository) IsFriendByIDs(ctx context.Context, userID, targetID string) (bool, error) {
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

func (r *Neo4jPostRepository) Comment(ctx context.Context, commentID, authorID, postID, content string, fileID *string) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
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
	return err
}

func (r *Neo4jPostRepository) ReplyComment(ctx context.Context, commentID, authorID, originalCommentID, postID, content string, fileID *string) error {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
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
		fileVal := ""
		if fileID != nil {
			fileVal = *fileID
		}
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"authorID":          authorID,
			"originalCommentID": originalCommentID,
			"postID":            postID,
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
	return err
}

func (r *Neo4jPostRepository) LikeComment(ctx context.Context, likerID, commentID string) error {
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
	return err
}

func (r *Neo4jPostRepository) UnlikeComment(ctx context.Context, likerID, commentID string) error {
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

func (r *Neo4jPostRepository) UpdateCommentContent(ctx context.Context, currentUserID, commentID, content string) error {
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
	return err
}

func (r *Neo4jPostRepository) DeleteComment(ctx context.Context, currentUserID, commentID string, isAdmin bool, postAuthorID string) ([]string, error) {
	comment, err := r.GetCommentByID(ctx, commentID, currentUserID)
	if err != nil {
		return nil, err
	}

	isPostAuthor := postAuthorID == currentUserID
	isCommentAuthor := comment.AuthorID == currentUserID
	if !isAdmin && !isPostAuthor && !isCommentAuthor {
		return nil, errors.New("UNAUTHORIZED")
	}

	var deletedFiles []string
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
			`, map[string]interface{}{"commentID": commentID, "postID": comment.PostID})
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
	return deletedFiles, err
}

func (r *Neo4jPostRepository) GetComments(ctx context.Context, postID, currentUserID string, pageType string, skip, limit int64) ([]*model.Comment, error) {
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

	return r.queryComments(ctx, currentUserID, query, map[string]interface{}{
		"postID":        postID,
		"currentUserID": currentUserID,
		"skip":          skip,
		"limit":         limit,
	})
}

func (r *Neo4jPostRepository) GetRepliedComments(ctx context.Context, originalCommentID, currentUserID string, skip, limit int64) ([]*model.Comment, error) {
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
	return r.queryComments(ctx, currentUserID, query, map[string]interface{}{
		"commentID":     originalCommentID,
		"currentUserID": currentUserID,
		"skip":          skip,
		"limit":         limit,
	})
}

func (r *Neo4jPostRepository) GetCommentByID(ctx context.Context, commentID string, currentUserID string) (*model.Comment, error) {
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
	comments, err := r.queryComments(ctx, currentUserID, query, map[string]interface{}{
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

func (r *Neo4jPostRepository) GetFilesInPostsOfUser(ctx context.Context, username string, skip, limit int64) ([]string, error) {
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
			"skip":     skip,
			"limit":    limit,
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

func (r *Neo4jPostRepository) queryPosts(ctx context.Context, currentUserID string, query string, params map[string]interface{}) ([]*model.Post, error) {
	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, params)
		if err != nil {
			return nil, err
		}
		posts := make([]*model.Post, 0)
		for result.Next(ctx) {
			post := postFromRecord(result.Record().Values)
			posts = append(posts, post)
		}
		return posts, result.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]*model.Post), nil
}

func (r *Neo4jPostRepository) queryComments(ctx context.Context, currentUserID string, query string, params map[string]interface{}) ([]*model.Comment, error) {
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
			comments = append(comments, c)
		}
		return comments, result.Err()
	})
	if err != nil {
		return nil, err
	}
	return res.([]*model.Comment), nil
}

func postFromRecord(vals []interface{}) *model.Post {
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
			}
			p.OriginalPost.Files = getStringSliceVal(vals[19])
			p.OriginalPost.Images = p.OriginalPost.Files
		}
	}
	return p
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
