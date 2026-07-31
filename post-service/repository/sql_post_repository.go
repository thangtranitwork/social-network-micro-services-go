package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"social-network-go/post-service/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"gorm.io/gorm"
)

type SQLPostRepository struct {
	db *gorm.DB
}

func NewSQLPostRepository(db *gorm.DB) PostRepository {
	return &SQLPostRepository{
		db: db,
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

	// Async sync share edge to Neo4j
	go r.addShareEdgeNeo4j(context.Background(), authorID, originalPostID)

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
	// Hybrid Query: If Neo4j driver is active, retrieve candidate Post IDs ranked by Graph Recommendation
	var candidateIDs []string
	if r.neo4jDriver != nil {
		candidateIDs = r.queryNeo4jNewsfeedCandidateIDs(ctx, currentUserID, skip, limit)
	}

	var posts []model.PostEntity
	if len(candidateIDs) > 0 {
		r.db.WithContext(ctx).Preload("MediaFiles").
			Where("id IN ?", candidateIDs).
			Find(&posts)
	} else {
		// Fallback to SQL chronological query
		r.db.WithContext(ctx).Preload("MediaFiles").
			Where("privacy = ?", PostPrivacyPublic).
			Order("created_at DESC").
			Offset(int(skip)).Limit(int(limit)).
			Find(&posts)
	}

	res := make([]*model.Post, len(posts))
	for i, p := range posts {
		res[i] = r.entityToModel(ctx, &p, currentUserID)
	}

	return res, nil
}

func (r *SQLPostRepository) GetRelevantNewsfeedCandidates(ctx context.Context, currentUserID string) ([]*model.NewsfeedCandidate, error) {
	var posts []model.PostEntity
	r.db.WithContext(ctx).Preload("MediaFiles").
		Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).
		Order("created_at DESC").Limit(50).Find(&posts)

	res := make([]*model.NewsfeedCandidate, len(posts))
	for i, p := range posts {
		res[i] = &model.NewsfeedCandidate{
			Post: r.entityToModel(ctx, &p, currentUserID),
		}
	}
	return res, nil
}

func (r *SQLPostRepository) MarkPostsLoaded(ctx context.Context, currentUserID string, postIDs []string) error {
	return nil
}

func (r *SQLPostRepository) UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error {
	return r.db.WithContext(ctx).Model(&model.PostEntity{}).
		Where("id = ? AND user_id = ?", postID, currentUserID).
		Update("privacy", privacy).Error
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

	return m
}

func (r *SQLPostRepository) queryNeo4jNewsfeedCandidateIDs(ctx context.Context, currentUserID string, skip, limit int64) []string {
	if r.neo4jDriver == nil {
		return nil
	}

	session := r.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userID})-[:FRIEND]-(f:User)-[:POSTED]->(p:Post)
		RETURN p.id AS id, p.createdAt AS createdAt
		ORDER BY p.createdAt DESC
		SKIP $skip LIMIT $limit
	`

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"userID": currentUserID,
			"skip":   skip,
			"limit":  limit,
		})
		if err != nil {
			return nil, err
		}
		var ids []string
		for res.Next(ctx) {
			rec := res.Record()
			if idVal, ok := rec.Get("id"); ok && idVal != nil {
				ids = append(ids, idVal.(string))
			}
		}
		return ids, nil
	})

	if err != nil || resData == nil {
		return nil
	}

	return resData.([]string)
}

func (r *SQLPostRepository) syncPostNodeToNeo4j(ctx context.Context, postID, authorID string) {
	if r.neo4jDriver == nil {
		return
	}
	go func(postID, authorID string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		session := r.neo4jDriver.NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(bgCtx)
		_, _ = session.ExecuteWrite(bgCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $authorID})
				MERGE (p:Post {id: $postID})
				ON CREATE SET p.createdAt = datetime()
				MERGE (u)-[:POSTED]->(p)
			`
			return tx.Run(bgCtx, query, map[string]interface{}{"postID": postID, "authorID": authorID})
		})
	}(postID, authorID)
}

func (r *SQLPostRepository) addLikeEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	go func(userID, postID string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		session := r.neo4jDriver.NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(bgCtx)
		_, _ = session.ExecuteWrite(bgCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $userID}), (p:Post {id: $postID})
				MERGE (u)-[:LIKED]->(p)
			`
			return tx.Run(bgCtx, query, map[string]interface{}{"userID": userID, "postID": postID})
		})
	}(userID, postID)
}

func (r *SQLPostRepository) removeLikeEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	go func(userID, postID string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		session := r.neo4jDriver.NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(bgCtx)
		_, _ = session.ExecuteWrite(bgCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $userID})-[r:LIKED]->(p:Post {id: $postID})
				DELETE r
			`
			return tx.Run(bgCtx, query, map[string]interface{}{"userID": userID, "postID": postID})
		})
	}(userID, postID)
}

func (r *SQLPostRepository) addCommentEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	go func(userID, postID string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		session := r.neo4jDriver.NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(bgCtx)
		_, _ = session.ExecuteWrite(bgCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $userID}), (p:Post {id: $postID})
				MERGE (u)-[:COMMENTED]->(p)
			`
			return tx.Run(bgCtx, query, map[string]interface{}{"userID": userID, "postID": postID})
		})
	}(userID, postID)
}

func (r *SQLPostRepository) addShareEdgeNeo4j(ctx context.Context, userID, postID string) {
	if r.neo4jDriver == nil {
		return
	}
	go func(userID, postID string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		session := r.neo4jDriver.NewSession(bgCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(bgCtx)
		_, _ = session.ExecuteWrite(bgCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $userID}), (p:Post {id: $postID})
				MERGE (u)-[:SHARED]->(p)
			`
			return tx.Run(bgCtx, query, map[string]interface{}{"userID": userID, "postID": postID})
		})
	}(userID, postID)
}
