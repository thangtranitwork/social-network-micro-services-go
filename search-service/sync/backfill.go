package sync

import (
	"context"
	"fmt"
	"time"

	"social-network-go/logger"
	"social-network-go/search-service/db"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

type BackfillService struct{}

func NewBackfillService() *BackfillService {
	return &BackfillService{}
}

// SyncAllFromNeo4j queries Neo4j and loads users and posts into Elasticsearch
func (s *BackfillService) SyncAllFromNeo4j(ctx context.Context) (int, int, error) {
	if db.Neo4jDriver == nil {
		return 0, 0, fmt.Errorf("neo4j driver not initialized")
	}
	if db.ESClient == nil {
		return 0, 0, fmt.Errorf("elasticsearch client not initialized")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// 1. Sync Users
	userCount, err := s.syncUsers(ctx, session)
	if err != nil {
		logger.Err(err).Error("Failed to sync users to Elasticsearch")
	}

	// 2. Sync Posts
	postCount, err := s.syncPosts(ctx, session)
	if err != nil {
		logger.Err(err).Error("Failed to sync posts to Elasticsearch")
	}

	return userCount, postCount, nil
}

func (s *BackfillService) syncUsers(ctx context.Context, session neo4j.SessionWithContext) (int, error) {
	batchSize := 250
	skip := 0
	totalSynced := 0

	for {
		cypher := `
			MATCH (u:User)
			RETURN u.id, u.username, u.givenName, u.familyName, u.email, u.bio, u.profilePictureId
			SKIP $skip LIMIT $limit
		`
		resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			res, err := tx.Run(ctx, cypher, map[string]interface{}{"skip": skip, "limit": batchSize})
			if err != nil {
				return nil, err
			}

			userDocs := make([]map[string]interface{}, 0, batchSize)
			for res.Next(ctx) {
				vals := res.Record().Values
				id := getStringVal(vals[0])
				username := getStringVal(vals[1])
				givenName := getStringVal(vals[2])
				familyName := getStringVal(vals[3])
				email := getStringVal(vals[4])
				bio := getStringVal(vals[5])
				pic := getStringVal(vals[6])

				fullName := stringsJoinNonEmpty(" ", familyName, givenName)
				if fullName == "" {
					fullName = username
				}

				userDocs = append(userDocs, map[string]interface{}{
					"id":                  id,
					"username":            username,
					"given_name":          givenName,
					"family_name":         familyName,
					"full_name":           fullName,
					"email":               email,
					"bio":                 bio,
					"profile_picture_url": pic,
					"created_at":          time.Now().UTC(),
				})
			}
			return userDocs, nil
		})
		if err != nil {
			return totalSynced, err
		}

		docs := resData.([]map[string]interface{})
		if len(docs) == 0 {
			break
		}

		if err := db.BulkIndex(ctx, "users", docs); err != nil {
			return totalSynced, err
		}
		totalSynced += len(docs)
		skip += len(docs)
		if len(docs) < batchSize {
			break
		}
	}

	logger.Info("Successfully backfilled %d users to Elasticsearch in batches", totalSynced)
	return totalSynced, nil
}

func (s *BackfillService) syncPosts(ctx context.Context, session neo4j.SessionWithContext) (int, error) {
	batchSize := 250
	skip := 0
	totalSynced := 0

	for {
		cypher := `
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE p.deletedAt IS NULL AND p.privacy = 'PUBLIC'
			RETURN p.id, author.id, p.content, p.privacy, coalesce(p.likeCount, 0),
			       coalesce(p.commentCount, 0), coalesce(p.shareCount, 0), p.files, p.createdAt
			SKIP $skip LIMIT $limit
		`
		resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			res, err := tx.Run(ctx, cypher, map[string]interface{}{"skip": skip, "limit": batchSize})
			if err != nil {
				return nil, err
			}

			postDocs := make([]map[string]interface{}, 0, batchSize)
			for res.Next(ctx) {
				vals := res.Record().Values
				id := getStringVal(vals[0])
				authorID := getStringVal(vals[1])
				content := getStringVal(vals[2])
				privacy := getStringVal(vals[3])
				likeCount := getIntVal(vals[4])
				commentCount := getIntVal(vals[5])
				shareCount := getIntVal(vals[6])
				files := getStringSliceVal(vals[7])
				createdAt := getTimeVal(vals[8])
				if createdAt.IsZero() {
					createdAt = time.Now().UTC()
				}

				postDocs = append(postDocs, map[string]interface{}{
					"id":            id,
					"author_id":     authorID,
					"content":       content,
					"privacy":       privacy,
					"like_count":    likeCount,
					"comment_count": commentCount,
					"share_count":   shareCount,
					"files":         files,
					"created_at":    createdAt,
				})
			}
			return postDocs, nil
		})
		if err != nil {
			return totalSynced, err
		}

		docs := resData.([]map[string]interface{})
		if len(docs) == 0 {
			break
		}

		if err := db.BulkIndex(ctx, "posts", docs); err != nil {
			return totalSynced, err
		}
		totalSynced += len(docs)
		skip += len(docs)
		if len(docs) < batchSize {
			break
		}
	}

	logger.Info("Successfully backfilled %d posts to Elasticsearch in batches", totalSynced)
	return totalSynced, nil
}

func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	return fmt.Sprintf("%v", val)
}

func getIntVal(val interface{}) int {
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case int64:
		return int(v)
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func getStringSliceVal(val interface{}) []string {
	res := make([]string, 0)
	if val == nil {
		return res
	}
	if arr, ok := val.([]interface{}); ok {
		for _, item := range arr {
			if s, ok := item.(string); ok && s != "" {
				res = append(res, s)
			}
		}
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

func stringsJoinNonEmpty(sep string, items ...string) string {
	var clean []string
	for _, it := range items {
		if it != "" {
			clean = append(clean, it)
		}
	}
	res := ""
	for i, c := range clean {
		if i > 0 {
			res += sep
		}
		res += c
	}
	return res
}
