package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"social-network-go/logger"
	"social-network-go/post-service/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gorm.io/gorm"
)

type SQLPostRepository struct {
	db          *gorm.DB
	neo4jDriver neo4j.DriverWithContext
}

func NewSQLPostRepository(db *gorm.DB, neo4jDriver neo4j.DriverWithContext) PostRepository {
	return &SQLPostRepository{
		db:          db,
		neo4jDriver: neo4jDriver,
	}
}

func (r *SQLPostRepository) CreatePost(ctx context.Context, postID, authorID, content, privacy string, fileIDs []string) error {
	post := model.PostEntity{
		ID:        postID,
		UserID:    authorID,
		Content:   content,
		Privacy:   privacy,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for _, fid := range fileIDs {
		post.MediaFiles = append(post.MediaFiles, model.PostMediaEntity{
			FileID:    fid,
			MediaURL:  fid,
			MediaType: "IMAGE",
			CreatedAt: time.Now(),
		})
	}

	if err := r.db.WithContext(ctx).Create(&post).Error; err != nil {
		return fmt.Errorf("failed to create post in SQL: %w", err)
	}

	// Async sync lightweight node to Neo4j graph for recommendation engine
	go r.syncPostNodeToNeo4j(context.Background(), postID, authorID)

	return nil
}

func (r *SQLPostRepository) SharePost(ctx context.Context, authorID, originalPostID, content, privacy string, postID string) error {
	post := model.PostEntity{
		ID:         postID,
		UserID:     authorID,
		Content:    content,
		Privacy:    privacy,
		OriginalID: originalPostID,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	if err := r.db.WithContext(ctx).Create(&post).Error; err != nil {
		return err
	}

	r.db.WithContext(ctx).Model(&model.PostEntity{}).Where("id = ?", originalPostID).
		UpdateColumn("share_count", gorm.Expr("share_count + ?", 1))

	// Async sync share edge and node to Neo4j
	go r.addShareEdgeNeo4j(context.Background(), authorID, originalPostID)
	go r.syncPostNodeToNeo4j(context.Background(), postID, authorID)

	return nil
}

func (r *SQLPostRepository) GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error) {
	var post model.PostEntity
	err := r.db.WithContext(ctx).Preload("MediaFiles").Where("id = ?", postID).First(&post).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, errors.New("post not found")
	} else if err != nil {
		return nil, err
	}

	return r.entityToModel(ctx, &post, currentUserID), nil
}

func (r *SQLPostRepository) GetPostsOfUser(ctx context.Context, authorUsername string, currentUserID string, skip, limit int64) ([]*model.Post, error) {
	var posts []model.PostEntity
	err := r.db.WithContext(ctx).Preload("MediaFiles").
		Where("user_id = ? OR user_id IN (SELECT id FROM users WHERE username = ?)", authorUsername, authorUsername).
		Order("created_at DESC").
		Offset(int(skip)).Limit(int(limit)).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}

	res := make([]*model.Post, len(posts))
	for i, p := range posts {
		res[i] = r.entityToModel(ctx, &p, currentUserID)
	}

	return res, nil
}

func (r *SQLPostRepository) GetAllPosts(ctx context.Context, skip, limit int64) ([]*model.Post, error) {
	var posts []model.PostEntity
	err := r.db.WithContext(ctx).Preload("MediaFiles").
		Where("privacy = ?", PostPrivacyPublic).
		Order("created_at DESC").
		Offset(int(skip)).Limit(int(limit)).
		Find(&posts).Error
	if err != nil {
		return nil, err
	}

	res := make([]*model.Post, len(posts))
	for i, p := range posts {
		res[i] = r.entityToModel(ctx, &p, "")
	}

	return res, nil
}

func (r *SQLPostRepository) GetSuggestedPosts(ctx context.Context, currentUserID string, pageType string, skip, limit int64) ([]*model.Post, error) {
	if r.neo4jDriver == nil {
		return nil, errors.New("neo4j driver is not initialized or unavailable")
	}

	candidateIDs, err := r.queryNeo4jNewsfeedCandidateIDs(ctx, currentUserID, skip, limit)
	if err != nil {
		return nil, err
	}

	if len(candidateIDs) == 0 {
		return []*model.Post{}, nil
	}

	var posts []model.PostEntity
	if err := r.db.WithContext(ctx).Preload("MediaFiles").
		Where("id IN ?", candidateIDs).
		Find(&posts).Error; err != nil {
		return nil, err
	}

	res := make([]*model.Post, len(posts))
	for i, p := range posts {
		res[i] = r.entityToModel(ctx, &p, currentUserID)
	}

	return res, nil
}

func (r *SQLPostRepository) GetRelevantNewsfeedCandidates(ctx context.Context, currentUserID string) ([]*model.NewsfeedCandidate, error) {
	if r.neo4jDriver == nil {
		return nil, errors.New("neo4j driver is not initialized or unavailable")
	}

	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	candidatesData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, getRelevantNewsfeedCandidateQuery(), map[string]interface{}{
			"currentUserID": currentUserID,
		})
		if err != nil {
			return nil, err
		}
		candidates := make([]*model.NewsfeedCandidate, 0)
		for result.Next(ctx) {
			c := newsfeedCandidateFromRecord(result.Record().Values)
			if c != nil && c.Post != nil && currentUserID != "" && c.Post.AuthorID == currentUserID {
				c.IsOwnPost = true
			}
			candidates = append(candidates, c)
		}
		return candidates, result.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query relevant newsfeed candidates from neo4j: %w", err)
	}

	candidates, ok := candidatesData.([]*model.NewsfeedCandidate)
	if !ok || len(candidates) == 0 {
		return []*model.NewsfeedCandidate{}, nil
	}

	// Hydrate post details from SQL (authoritative content, media files, SQL counts)
	postIDs := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if c != nil && c.Post != nil && c.Post.ID != "" {
			postIDs = append(postIDs, c.Post.ID)
		}
	}

	if len(postIDs) > 0 {
		var entities []model.PostEntity
		if err := r.db.WithContext(ctx).Preload("MediaFiles").
			Where("id IN ?", postIDs).
			Find(&entities).Error; err != nil {
			return nil, fmt.Errorf("failed to hydrate posts from SQL: %w", err)
		}

		entityMap := make(map[string]*model.PostEntity, len(entities))
		for i := range entities {
			entityMap[entities[i].ID] = &entities[i]
		}

		var userLikes []model.PostLikeEntity
		if currentUserID != "" {
			r.db.WithContext(ctx).Where("user_id = ? AND post_id IN ?", currentUserID, postIDs).Find(&userLikes)
		}
		likedMap := make(map[string]bool, len(userLikes))
		for _, ul := range userLikes {
			likedMap[ul.PostID] = true
		}

		validCandidates := make([]*model.NewsfeedCandidate, 0, len(candidates))
		for _, c := range candidates {
			if c == nil || c.Post == nil {
				continue
			}
			entity, ok := entityMap[c.Post.ID]
			if !ok {
				// Post exists in Neo4j but was deleted in SQL
				continue
			}
			// Enrich with authoritative SQL data
			c.Post.Content = entity.Content
			c.Post.Privacy = entity.Privacy
			c.Post.CreatedAt = entity.CreatedAt
			updatedAt := entity.UpdatedAt
			c.Post.UpdatedAt = &updatedAt
			c.Post.LikeCount = entity.LikeCount
			c.Post.CommentCount = entity.CommentCount
			c.Post.ShareCount = entity.ShareCount
			if currentUserID != "" {
				c.Post.Liked = likedMap[c.Post.ID]
			}

			if len(entity.MediaFiles) > 0 {
				files := make([]string, len(entity.MediaFiles))
				for i, mf := range entity.MediaFiles {
					files[i] = mf.FileID
				}
				c.Post.Files = files
				c.Post.Images = files
			}

			validCandidates = append(validCandidates, c)
		}
		candidates = validCandidates
	}

	return candidates, nil
}

func (r *SQLPostRepository) MarkPostsLoaded(ctx context.Context, currentUserID string, postIDs []string) error {
	if r.neo4jDriver == nil {
		return errors.New("neo4j driver is not initialized or unavailable")
	}
	if currentUserID == "" || len(postIDs) == 0 {
		return nil
	}

	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User {id: $currentUserID})
			UNWIND $postIDs AS postID
			MATCH (p:Post {id: postID})
			MERGE (u)-[r:LOADED]->(p)
			ON CREATE SET r.times = 1
			ON MATCH SET r.times = r.times + 1
		`
		return tx.Run(ctx, query, map[string]interface{}{
			"currentUserID": currentUserID,
			"postIDs":       postIDs,
		})
	})
	return err
}

func (r *SQLPostRepository) UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error {
	err := r.db.WithContext(ctx).Model(&model.PostEntity{}).
		Where("id = ? AND user_id = ?", postID, currentUserID).
		Update("privacy", privacy).Error
	if err == nil {
		go r.syncPostNodeToNeo4j(context.Background(), postID, currentUserID)
	}
	return err
}

func (r *SQLPostRepository) UpdateContent(ctx context.Context, currentUserID, postID string, content *string, newFileIDs []string, deleteOldFileIDs []string, maxPostAttachFiles int, maxPostContentLength int) ([]string, string, error) {
	var post model.PostEntity
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", postID, currentUserID).First(&post).Error; err != nil {
		return nil, "", errors.New("post not found or unauthorized")
	}

	if content != nil {
		r.db.WithContext(ctx).Model(&post).Update("content", *content)
	}

	if len(deleteOldFileIDs) > 0 {
		r.db.WithContext(ctx).Where("post_id = ? AND file_id IN ?", postID, deleteOldFileIDs).Delete(&model.PostMediaEntity{})
	}

	for _, fid := range newFileIDs {
		r.db.WithContext(ctx).Create(&model.PostMediaEntity{
			PostID:    postID,
			FileID:    fid,
			MediaURL:  fid,
			MediaType: "IMAGE",
			CreatedAt: time.Now(),
		})
	}

	go r.syncPostNodeToNeo4j(context.Background(), postID, currentUserID)

	return deleteOldFileIDs, post.UserID, nil
}

func (r *SQLPostRepository) LikePost(ctx context.Context, userID, postID string) (string, error) {
	var count int64
	r.db.WithContext(ctx).Model(&model.PostLikeEntity{}).Where("post_id = ? AND user_id = ?", postID, userID).Count(&count)
	if count == 0 {
		r.db.WithContext(ctx).Create(&model.PostLikeEntity{
			PostID:    postID,
			UserID:    userID,
			CreatedAt: time.Now(),
		})
		r.db.WithContext(ctx).Model(&model.PostEntity{}).Where("id = ?", postID).
			UpdateColumn("like_count", gorm.Expr("like_count + ?", 1))
	}

	var post model.PostEntity
	r.db.WithContext(ctx).Select("user_id").Where("id = ?", postID).First(&post)

	// Async sync like edge to Neo4j graph
	go r.addLikeEdgeNeo4j(context.Background(), userID, postID)

	return post.UserID, nil
}

func (r *SQLPostRepository) UnlikePost(ctx context.Context, userID, postID string) (string, error) {
	res := r.db.WithContext(ctx).Where("post_id = ? AND user_id = ?", postID, userID).Delete(&model.PostLikeEntity{})
	if res.RowsAffected > 0 {
		r.db.WithContext(ctx).Model(&model.PostEntity{}).Where("id = ? AND like_count > 0", postID).
			UpdateColumn("like_count", gorm.Expr("like_count - ?", 1))
	}

	var post model.PostEntity
	r.db.WithContext(ctx).Select("user_id").Where("id = ?", postID).First(&post)

	// Async remove like edge from Neo4j graph
	go r.removeLikeEdgeNeo4j(context.Background(), userID, postID)

	return post.UserID, nil
}

func (r *SQLPostRepository) DeletePost(ctx context.Context, postID, currentUserID string, isAdmin bool) (string, []string, error) {
	var post model.PostEntity
	query := r.db.WithContext(ctx).Where("id = ?", postID)
	if !isAdmin {
		query = query.Where("user_id = ?", currentUserID)
	}

	if err := query.Preload("MediaFiles").First(&post).Error; err != nil {
		return "", nil, errors.New("post not found or unauthorized")
	}

	deletedFileIDs := make([]string, len(post.MediaFiles))
	for i, m := range post.MediaFiles {
		deletedFileIDs[i] = m.FileID
	}

	r.db.WithContext(ctx).Delete(&post)

	go r.markPostDeletedNeo4j(context.Background(), postID)

	return post.UserID, deletedFileIDs, nil
}

func (r *SQLPostRepository) ValidateBlockByIDs(ctx context.Context, userID, targetID string) error {
	return nil
}

func (r *SQLPostRepository) ValidateBlockByUsername(ctx context.Context, userID, targetUsername string) error {
	return nil
}

func (r *SQLPostRepository) IsFriendByIDs(ctx context.Context, userID, targetID string) (bool, error) {
	return true, nil
}

func (r *SQLPostRepository) Comment(ctx context.Context, commentID, authorID, postID, content string, fileID *string) error {
	comment := model.CommentEntity{
		ID:        commentID,
		PostID:    postID,
		UserID:    authorID,
		Content:   content,
		FileID:    fileID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := r.db.WithContext(ctx).Create(&comment).Error; err != nil {
		return err
	}

	r.db.WithContext(ctx).Model(&model.PostEntity{}).Where("id = ?", postID).
		UpdateColumn("comment_count", gorm.Expr("comment_count + ?", 1))

	// Async sync comment edge to Neo4j graph
	go r.addCommentEdgeNeo4j(context.Background(), authorID, postID)

	return nil
}

func (r *SQLPostRepository) ReplyComment(ctx context.Context, commentID, authorID, originalCommentID, postID, content string, fileID *string) error {
	comment := model.CommentEntity{
		ID:        commentID,
		PostID:    postID,
		UserID:    authorID,
		ParentID:  originalCommentID,
		Content:   content,
		FileID:    fileID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := r.db.WithContext(ctx).Create(&comment).Error; err != nil {
		return err
	}

	r.db.WithContext(ctx).Model(&model.CommentEntity{}).Where("id = ?", originalCommentID).
		UpdateColumn("reply_count", gorm.Expr("reply_count + ?", 1))

	r.db.WithContext(ctx).Model(&model.PostEntity{}).Where("id = ?", postID).
		UpdateColumn("comment_count", gorm.Expr("comment_count + ?", 1))

	return nil
}

func (r *SQLPostRepository) LikeComment(ctx context.Context, likerID, commentID string) error {
	var count int64
	r.db.WithContext(ctx).Model(&model.CommentLikeEntity{}).Where("comment_id = ? AND user_id = ?", commentID, likerID).Count(&count)
	if count == 0 {
		r.db.WithContext(ctx).Create(&model.CommentLikeEntity{
			CommentID: commentID,
			UserID:    likerID,
			CreatedAt: time.Now(),
		})
		r.db.WithContext(ctx).Model(&model.CommentEntity{}).Where("id = ?", commentID).
			UpdateColumn("like_count", gorm.Expr("like_count + ?", 1))
	}
	return nil
}

func (r *SQLPostRepository) UnlikeComment(ctx context.Context, likerID, commentID string) error {
	res := r.db.WithContext(ctx).Where("comment_id = ? AND user_id = ?", commentID, likerID).Delete(&model.CommentLikeEntity{})
	if res.RowsAffected > 0 {
		r.db.WithContext(ctx).Model(&model.CommentEntity{}).Where("id = ? AND like_count > 0", commentID).
			UpdateColumn("like_count", gorm.Expr("like_count - ?", 1))
	}
	return nil
}

func (r *SQLPostRepository) UpdateCommentContent(ctx context.Context, currentUserID, commentID, content string) error {
	return r.db.WithContext(ctx).Model(&model.CommentEntity{}).
		Where("id = ? AND user_id = ?", commentID, currentUserID).
		Update("content", content).Error
}

func (r *SQLPostRepository) DeleteComment(ctx context.Context, currentUserID, commentID string, isAdmin bool, postAuthorID string) ([]string, error) {
	var comment model.CommentEntity
	if err := r.db.WithContext(ctx).Where("id = ?", commentID).First(&comment).Error; err != nil {
		return nil, errors.New("comment not found")
	}

	r.db.WithContext(ctx).Delete(&comment)
	r.db.WithContext(ctx).Model(&model.PostEntity{}).Where("id = ? AND comment_count > 0", comment.PostID).
		UpdateColumn("comment_count", gorm.Expr("comment_count - ?", 1))

	return []string{}, nil
}

func (r *SQLPostRepository) GetComments(ctx context.Context, postID, currentUserID string, pageType string, skip, limit int64) ([]*model.Comment, error) {
	var comments []model.CommentEntity
	err := r.db.WithContext(ctx).Where("post_id = ? AND (parent_id IS NULL OR parent_id = '')", postID).
		Order("created_at ASC").Offset(int(skip)).Limit(int(limit)).Find(&comments).Error
	if err != nil {
		return nil, err
	}

	res := make([]*model.Comment, len(comments))
	for i, c := range comments {
		res[i] = r.commentEntityToModel(ctx, &c, currentUserID)
	}

	return res, nil
}

func (r *SQLPostRepository) GetRepliedComments(ctx context.Context, originalCommentID, currentUserID string, skip, limit int64) ([]*model.Comment, error) {
	var comments []model.CommentEntity
	err := r.db.WithContext(ctx).Where("parent_id = ?", originalCommentID).
		Order("created_at ASC").Offset(int(skip)).Limit(int(limit)).Find(&comments).Error
	if err != nil {
		return nil, err
	}

	res := make([]*model.Comment, len(comments))
	for i, c := range comments {
		res[i] = r.commentEntityToModel(ctx, &c, currentUserID)
	}

	return res, nil
}

func (r *SQLPostRepository) GetCommentByID(ctx context.Context, commentID string, currentUserID string) (*model.Comment, error) {
	var comment model.CommentEntity
	if err := r.db.WithContext(ctx).Where("id = ?", commentID).First(&comment).Error; err != nil {
		return nil, errors.New("comment not found")
	}

	return r.commentEntityToModel(ctx, &comment, currentUserID), nil
}

func (r *SQLPostRepository) GetFilesInPostsOfUser(ctx context.Context, username string, skip, limit int64) ([]string, error) {
	var media []model.PostMediaEntity
	r.db.WithContext(ctx).Joins("JOIN posts ON posts.id = post_media.post_id").
		Where("posts.user_id = ?", username).
		Offset(int(skip)).Limit(int(limit)).Find(&media)

	res := make([]string, len(media))
	for i, m := range media {
		res[i] = m.FileID
	}
	return res, nil
}

// Helpers
func (r *SQLPostRepository) entityToModel(ctx context.Context, p *model.PostEntity, currentUserID string) *model.Post {
	updatedAt := p.UpdatedAt
	m := &model.Post{
		ID:           p.ID,
		AuthorID:     p.UserID,
		Content:      p.Content,
		LikeCount:    p.LikeCount,
		CommentCount: p.CommentCount,
		ShareCount:   p.ShareCount,
		CreatedAt:    p.CreatedAt,
		UpdatedAt:    &updatedAt,
		Privacy:      p.Privacy,
	}

	for _, mf := range p.MediaFiles {
		m.Files = append(m.Files, mf.FileID)
	}

	if currentUserID != "" {
		var likeCount int64
		r.db.WithContext(ctx).Model(&model.PostLikeEntity{}).Where("post_id = ? AND user_id = ?", p.ID, currentUserID).Count(&likeCount)
		m.Liked = likeCount > 0
	}

	return m
}

func (r *SQLPostRepository) commentEntityToModel(ctx context.Context, c *model.CommentEntity, currentUserID string) *model.Comment {
	updatedAt := c.UpdatedAt
	m := &model.Comment{
		ID:        c.ID,
		PostID:    c.PostID,
		AuthorID:  c.UserID,
		Content:   c.Content,
		LikeCount: c.LikeCount,
		CreatedAt: c.CreatedAt,
		UpdatedAt: &updatedAt,
	}

	if currentUserID != "" {
		var likeCount int64
		r.db.WithContext(ctx).Model(&model.CommentLikeEntity{}).Where("comment_id = ? AND user_id = ?", c.ID, currentUserID).Count(&likeCount)
		m.Liked = likeCount > 0
	}

	if c.FileID != nil && *c.FileID != "" {
		m.FileUrl = *c.FileID
		m.Files = []string{*c.FileID}
	}

	return m
}

func (r *SQLPostRepository) queryNeo4jNewsfeedCandidateIDs(ctx context.Context, currentUserID string, skip, limit int64) ([]string, error) {
	if r.neo4jDriver == nil {
		return nil, errors.New("neo4j driver is not initialized or unavailable")
	}
	if currentUserID == "" {
		return nil, nil
	}
	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (viewer:User {id: $viewerID})
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL
			  AND NOT (viewer)-[:BLOCK]-(author)
			  AND (
				  p.privacy = 'PUBLIC'
				  OR author.id = viewer.id
				  OR (p.privacy = 'FRIEND' AND EXISTS((viewer)-[:FRIEND]-(author)))
			  )
			RETURN p.id AS postID
			ORDER BY p.createdAt DESC
			SKIP $skip LIMIT $limit
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"viewerID": currentUserID,
			"skip":     skip,
			"limit":    limit,
		})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			rec := res.Record()
			if val, ok := rec.Get("postID"); ok && val != nil {
				ids = append(ids, fmt.Sprintf("%v", val))
			}
		}
		return ids, nil
	})
	if err != nil {
		return nil, err
	}
	if resData == nil {
		return nil, nil
	}
	return resData.([]string), nil
}

func (r *SQLPostRepository) syncPostNodeToNeo4j(ctx context.Context, postID, authorID string) {
	if r.neo4jDriver == nil {
		return
	}

	var post model.PostEntity
	if err := r.db.WithContext(ctx).Preload("MediaFiles").First(&post, "id = ?", postID).Error; err != nil {
		logger.Warn("[Neo4j Sync] Failed to query post %s from SQL: %v", postID, err)
		return
	}

	var fileIDs []string
	for _, m := range post.MediaFiles {
		fileIDs = append(fileIDs, m.FileID)
	}

	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MERGE (u:User {id: $authorID})
			MERGE (p:Post {id: $postID})
			ON CREATE SET 
				p.content = $content,
				p.privacy = $privacy,
				p.files = $files,
				p.likeCount = $likeCount,
				p.commentCount = $commentCount,
				p.shareCount = $shareCount,
				p.createdAt = datetime($createdAt)
			ON MATCH SET 
				p.content = $content,
				p.privacy = $privacy,
				p.files = $files
			MERGE (u)-[:POSTED]->(p)
		`
		return tx.Run(ctx, query, map[string]interface{}{
			"authorID":     authorID,
			"postID":       postID,
			"content":      post.Content,
			"privacy":      post.Privacy,
			"files":        fileIDs,
			"likeCount":    post.LikeCount,
			"commentCount": post.CommentCount,
			"shareCount":   post.ShareCount,
			"createdAt":    post.CreatedAt.Format(time.RFC3339),
		})
	})
	if err != nil {
		logger.Warn("[Neo4j Sync] Error syncing post node %s to Neo4j: %v", postID, err)
	} else {
		logger.Info("[Neo4j Sync] Successfully synced post %s to Neo4j graph", postID)
	}
}

func (r *SQLPostRepository) addLikeEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MERGE (u:User {id: $userID})
			MERGE (p:Post {id: $postID})
			MERGE (u)-[:LIKED]->(p)
			SET p.likeCount = coalesce(p.likeCount, 0) + 1
		`
		return tx.Run(ctx, query, map[string]interface{}{"userID": userID, "postID": postID})
	})
}

func (r *SQLPostRepository) removeLikeEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User {id: $userID})-[r:LIKED]->(p:Post {id: $postID})
			DELETE r
			SET p.likeCount = CASE WHEN coalesce(p.likeCount, 0) > 0 THEN p.likeCount - 1 ELSE 0 END
		`
		return tx.Run(ctx, query, map[string]interface{}{"userID": userID, "postID": postID})
	})
}

func (r *SQLPostRepository) addCommentEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (p:Post {id: $postID})
			SET p.commentCount = coalesce(p.commentCount, 0) + 1
		`
		return tx.Run(ctx, query, map[string]interface{}{"postID": postID})
	})
}

func (r *SQLPostRepository) addShareEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (p:Post {id: $postID})
			SET p.shareCount = coalesce(p.shareCount, 0) + 1
		`
		return tx.Run(ctx, query, map[string]interface{}{"postID": postID})
	})
}

func (r *SQLPostRepository) markPostDeletedNeo4j(ctx context.Context, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)
	_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (p:Post {id: $postID})
			SET p.deletedAt = datetime()
		`
		return tx.Run(ctx, query, map[string]interface{}{"postID": postID})
	})
}

// SyncAllPostsToNeo4j scans recent posts from SQL and merges them to Neo4j.
func (r *SQLPostRepository) SyncAllPostsToNeo4j(ctx context.Context) {
	if r.neo4jDriver == nil {
		return
	}
	var posts []model.PostEntity
	if err := r.db.WithContext(ctx).Preload("MediaFiles").Order("created_at DESC").Limit(500).Find(&posts).Error; err != nil {
		logger.Warn("[Neo4j Initial Sync] Failed to query posts: %v", err)
		return
	}
	syncedCount := 0
	for _, p := range posts {
		r.syncPostNodeToNeo4j(ctx, p.ID, p.UserID)
		syncedCount++
	}
	logger.Info("[Neo4j Initial Sync] Completed sync of %d posts to Neo4j graph", syncedCount)
}
