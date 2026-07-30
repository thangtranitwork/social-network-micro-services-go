package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"social-network-go/logger"
	"social-network-go/notification-service/model"
)

type NotificationRepository interface {
	SaveFCMToken(userID, token, deviceType string) error
	GetFCMTokens(receiverID string) ([]string, error)
	DeleteFCMTokens(tokens []string) error
	CreateSingleNotification(event model.NotificationKafkaEvent) (string, error)
	GetFriendsIDs(creatorID string) ([]string, error)
	FetchNotificationDetails(notifID string) (*model.Notification, error)
	GetNotifications(userID string, skip, limit int) ([]model.Notification, error)
	GetUnreadCount(userID string) (int64, error)
	MarkAsRead(userID string, limit int) error
}

type notificationRepository struct {
	driver neo4j.DriverWithContext
}

func NewNotificationRepository(driver neo4j.DriverWithContext) NotificationRepository {
	return &notificationRepository{driver: driver}
}

func (r *notificationRepository) SaveFCMToken(userID, token, deviceType string) error {
	if r.driver == nil {
		logger.Warn("Neo4j driver is nil, skipping FCM token save")
		return nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userId})
		MERGE (t:FCMToken {token: $token})
		ON CREATE SET t.deviceType = $deviceType, t.createdAt = datetime(), t.updatedAt = datetime()
		ON MATCH SET t.deviceType = $deviceType, t.updatedAt = datetime()
		MERGE (u)-[:HAS_FCM_TOKEN]->(t)
	`
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, query, map[string]interface{}{
			"userId":     userID,
			"token":      token,
			"deviceType": deviceType,
		})
	})
	if err != nil {
		logger.Error("Failed to save FCM token for user %s: %v", userID, err)
		return err
	}
	return nil
}

func (r *notificationRepository) GetFCMTokens(receiverID string) ([]string, error) {
	if r.driver == nil {
		return nil, nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $receiverId})-[:HAS_FCM_TOKEN]->(t:FCMToken)
		RETURN t.token
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, map[string]interface{}{"receiverId": receiverID})
		if err != nil {
			return nil, err
		}
		var tokens []string
		for result.Next(ctx) {
			if val, ok := result.Record().Values[0].(string); ok && val != "" {
				tokens = append(tokens, val)
			}
		}
		return tokens, nil
	})
	if err != nil || res == nil {
		return nil, err
	}
	return res.([]string), nil
}

func (r *notificationRepository) DeleteFCMTokens(tokens []string) error {
	if r.driver == nil || len(tokens) == 0 {
		return nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		UNWIND $tokens AS token
		MATCH (t:FCMToken {token: token})
		DETACH DELETE t
	`
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, query, map[string]interface{}{"tokens": tokens})
	})
	if err == nil {
		logger.Info("🧹 Auto-cleaned %d stale/invalid FCM tokens from Neo4j DB", len(tokens))
	}
	return err
}

func (r *notificationRepository) CreateSingleNotification(event model.NotificationKafkaEvent) (string, error) {
	if r.driver == nil {
		return "", nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	var notifID string
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		if event.Action == "LIKE_POST" || event.Action == "LIKE_COMMENT" {
			checkQuery := `
				MATCH (creator:User {id: $creatorId})<-[:BY_USER]-(n:Notification)<-[:HAS_NOTIFICATION]-(receiver:User {id: $receiverId})
				WHERE n.action = $action
				  AND n.targetId = $targetId
				  AND n.targetType = $targetType
				RETURN n.id
				LIMIT 1
			`
			res, err := tx.Run(ctx, checkQuery, map[string]interface{}{
				"creatorId":  event.CreatorID,
				"receiverId": event.ReceiverID,
				"action":     event.Action,
				"targetId":   event.TargetID,
				"targetType": event.TargetType,
			})
			if err == nil && res.Next(ctx) {
				notifID = res.Record().Values[0].(string)
				_, updateErr := tx.Run(ctx, `
					MATCH (n:Notification {id: $id})
					SET n.sentAt = datetime(), n.isRead = false
				`, map[string]interface{}{"id": notifID})
				return nil, updateErr
			}
		}

		notifID = uuid.NewString()
		createQuery := `
			MERGE (creator:User {id: $creatorId})
			MERGE (receiver:User {id: $receiverId})
			CREATE (receiver)-[:HAS_NOTIFICATION]->(n:Notification {
				id: $id,
				action: $action,
				targetType: $targetType,
				targetId: $targetId,
				shortenedContent: $shortenedContent,
				isRead: false,
				sentAt: datetime()
			})-[:BY_USER]->(creator)
		`
		_, createErr := tx.Run(ctx, createQuery, map[string]interface{}{
			"id":               notifID,
			"creatorId":        event.CreatorID,
			"receiverId":       event.ReceiverID,
			"action":           event.Action,
			"targetType":       event.TargetType,
			"targetId":         event.TargetID,
			"shortenedContent": event.ShortenedContent,
		})
		return nil, createErr
	})
	return notifID, err
}

func (r *notificationRepository) GetFriendsIDs(creatorID string) ([]string, error) {
	if r.driver == nil {
		return nil, nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User {id: $creatorId})-[:FRIEND]-(f:User)
			RETURN f.id
		`
		result, err := tx.Run(ctx, query, map[string]interface{}{"creatorId": creatorID})
		if err != nil {
			return nil, err
		}
		var list []string
		for result.Next(ctx) {
			list = append(list, result.Record().Values[0].(string))
		}
		return list, nil
	})
	if err != nil || res == nil {
		return nil, err
	}
	return res.([]string), nil
}

func (r *notificationRepository) FetchNotificationDetails(notifID string) (*model.Notification, error) {
	if r.driver == nil {
		return nil, nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (n:Notification {id: $id})-[:BY_USER]->(creator:User)
			OPTIONAL MATCH (creator)-[:HAS_PROFILE_PICTURE]->(pf:File)
			OPTIONAL MATCH (post:Post {id: n.targetId}) WHERE n.targetType = 'POST'
			OPTIONAL MATCH (comment:Comment {id: n.targetId}) WHERE n.targetType = 'COMMENT'
			OPTIONAL MATCH (comment)-[:REPLIED]->(originalComment:Comment)
			OPTIONAL MATCH (postFromComment:Post)-[:HAS_COMMENT]-(commentWithPost:Comment)
			WHERE commentWithPost = CASE
				WHEN originalComment IS NOT NULL THEN originalComment
				ELSE comment
			END
			RETURN n.id, n.action, n.targetType, n.targetId, n.shortenedContent, n.sentAt, n.isRead,
			       creator.id, creator.username, creator.givenName, creator.familyName, pf.id,
			       post.id, postFromComment.id, comment.id, originalComment.id
		`
		result, err := tx.Run(ctx, query, map[string]interface{}{"id": notifID})
		if err != nil {
			return nil, err
		}
		if result.Next(ctx) {
			vals := result.Record().Values

			sentAtTime := time.Now()
			if val, ok := vals[5].(dbtype.LocalDateTime); ok {
				sentAtTime = val.Time()
			}

			creatorImg := ""
			if vals[11] != nil {
				creatorImg = vals[11].(string)
			}

			creatorInfo := model.CreatorInfo{
				ID:                getString(vals[7]),
				Username:          getString(vals[8]),
				GivenName:         getString(vals[9]),
				FamilyName:        getString(vals[10]),
				ProfilePictureUrl: creatorImg,
			}

			n := model.Notification{
				ID:               getString(vals[0]),
				Action:           getString(vals[1]),
				TargetType:       getString(vals[2]),
				TargetID:         getString(vals[3]),
				ShortenedContent: getString(vals[4]),
				SentAt:           sentAtTime,
				IsRead:           vals[6].(bool),
				Creator:          creatorInfo,
				Username:         creatorInfo.Username,
			}

			if n.TargetType == "POST" {
				n.PostID = getString(vals[12])
			} else if n.TargetType == "COMMENT" {
				n.PostID = getString(vals[13])
				if vals[15] != nil {
					n.CommentID = getString(vals[15])
					n.RepliedCommentID = getString(vals[14])
				} else {
					n.CommentID = getString(vals[14])
				}
			}

			return &n, nil
		}
		return nil, nil
	})

	if err != nil || res == nil {
		return nil, err
	}
	return res.(*model.Notification), nil
}

func (r *notificationRepository) GetNotifications(userID string, skip, limit int) ([]model.Notification, error) {
	if r.driver == nil {
		return []model.Notification{}, nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	res, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (receiver:User {id: $userId})
			MATCH (receiver)-[:HAS_NOTIFICATION]->(n:Notification)-[:BY_USER]->(creator:User)
			OPTIONAL MATCH (creator)-[:HAS_PROFILE_PICTURE]->(pf:File)
			OPTIONAL MATCH (post:Post {id: n.targetId}) WHERE n.targetType = 'POST'
			OPTIONAL MATCH (comment:Comment {id: n.targetId}) WHERE n.targetType = 'COMMENT'
			OPTIONAL MATCH (comment)-[:REPLIED]->(originalComment:Comment)
			OPTIONAL MATCH (postFromComment:Post)-[:HAS_COMMENT]-(commentWithPost:Comment)
			WHERE commentWithPost = CASE
				WHEN originalComment IS NOT NULL THEN originalComment
				ELSE comment
			END
			WITH n, creator, pf, post, comment, originalComment, postFromComment
			ORDER BY n.sentAt DESC
			SKIP $skip LIMIT $limit
			SET n.isRead = true
			RETURN n.id, n.action, n.targetType, n.targetId, n.shortenedContent, n.sentAt,
			       creator.id, creator.username, creator.givenName, creator.familyName, pf.id,
			       post.id, postFromComment.id, comment.id, originalComment.id
		`
		result, err := tx.Run(ctx, query, map[string]interface{}{
			"userId": userID,
			"skip":   int64(skip),
			"limit":  int64(limit),
		})
		if err != nil {
			return nil, err
		}
		var list []model.Notification
		for result.Next(ctx) {
			vals := result.Record().Values

			sentAtTime := time.Now()
			if val, ok := vals[5].(dbtype.LocalDateTime); ok {
				sentAtTime = val.Time()
			}

			creatorImg := ""
			if vals[10] != nil {
				creatorImg = vals[10].(string)
			}

			creatorInfo := model.CreatorInfo{
				ID:                getString(vals[6]),
				Username:          getString(vals[7]),
				GivenName:         getString(vals[8]),
				FamilyName:        getString(vals[9]),
				ProfilePictureUrl: creatorImg,
			}

			n := model.Notification{
				ID:               getString(vals[0]),
				Action:           getString(vals[1]),
				TargetType:       getString(vals[2]),
				TargetID:         getString(vals[3]),
				ShortenedContent: getString(vals[4]),
				SentAt:           sentAtTime,
				IsRead:           true,
				Creator:          creatorInfo,
				Username:         creatorInfo.Username,
			}

			if n.TargetType == "POST" {
				n.PostID = getString(vals[11])
			} else if n.TargetType == "COMMENT" {
				n.PostID = getString(vals[12])
				if vals[14] != nil {
					n.CommentID = getString(vals[14])
					n.RepliedCommentID = getString(vals[13])
				} else {
					n.CommentID = getString(vals[13])
				}
			}

			list = append(list, n)
		}
		return list, nil
	})

	if err != nil || res == nil {
		return []model.Notification{}, err
	}
	return res.([]model.Notification), nil
}

func (r *notificationRepository) GetUnreadCount(userID string) (int64, error) {
	if r.driver == nil {
		return 0, nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (receiver:User {id: $userId})-[:HAS_NOTIFICATION]->(n:Notification)
			WHERE n.isRead = false OR n.isRead IS NULL
			RETURN count(n) AS unreadCount
		`
		result, err := tx.Run(ctx, query, map[string]interface{}{"userId": userID})
		if err != nil {
			return int64(0), err
		}
		if result.Next(ctx) {
			return result.Record().Values[0].(int64), nil
		}
		return int64(0), nil
	})
	if err != nil || res == nil {
		return 0, err
	}
	return res.(int64), nil
}

func (r *notificationRepository) MarkAsRead(userID string, limit int) error {
	if r.driver == nil {
		return nil
	}
	ctx := context.Background()
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (receiver:User {id: $userId})-[:HAS_NOTIFICATION]->(n:Notification)
			WHERE n.isRead = false OR n.isRead IS NULL
			WITH n
			ORDER BY n.sentAt DESC
			LIMIT $limit
			SET n.isRead = true
			RETURN n.id
		`
		return tx.Run(ctx, query, map[string]interface{}{
			"userId": userID,
			"limit":  int64(limit),
		})
	})
	return err
}

func getString(val interface{}) string {
	if val == nil {
		return ""
	}
	return val.(string)
}
