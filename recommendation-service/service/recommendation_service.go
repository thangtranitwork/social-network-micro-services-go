package service

import (
	"context"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"social-network-go/logger"
	"social-network-go/recommendation-service/config"
	"social-network-go/recommendation-service/db"
	"social-network-go/recommendation-service/model"
)

type RecommendationService struct {
	cfg         *config.Config
	neo4jDriver neo4j.DriverWithContext
}

func NewRecommendationService(cfg *config.Config) *RecommendationService {
	return &RecommendationService{
		cfg:         cfg,
		neo4jDriver: db.Neo4jDriver,
	}
}

func (s *RecommendationService) GetSuggestedFriends(ctx context.Context, userID string) ([]*model.RecommendedUserCandidate, error) {
	if s.neo4jDriver == nil {
		return []*model.RecommendedUserCandidate{}, nil
	}

	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (u:User {id: $userID})
			MATCH (u)-[:FRIEND]-(f:User)-[:FRIEND]-(suggested:User)
			WHERE NOT (u)-[:FRIEND]-(suggested)
			  AND NOT (u)-[:BLOCK]-(suggested)
			  AND suggested.id <> $userID
			RETURN suggested.id AS id, suggested.username AS username, COUNT(f) AS mutualCount
			ORDER BY mutualCount DESC
			LIMIT 20
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}

		var candidates []*model.RecommendedUserCandidate
		for res.Next(ctx) {
			rec := res.Record()
			idVal, _ := rec.Get("id")
			userVal, _ := rec.Get("username")
			cntVal, _ := rec.Get("mutualCount")

			c := &model.RecommendedUserCandidate{}
			if idVal != nil {
				c.UserID = idVal.(string)
			}
			if userVal != nil {
				c.Username = userVal.(string)
			}
			if cntVal != nil {
				c.MutualFriendsCount = int(cntVal.(int64))
			}
			candidates = append(candidates, c)
		}
		return candidates, nil
	})

	if err != nil {
		logger.Warn("Failed to execute GetSuggestedFriends Cypher query: %v", err)
		return []*model.RecommendedUserCandidate{}, nil
	}

	return resData.([]*model.RecommendedUserCandidate), nil
}

func (s *RecommendationService) GetSuggestedPosts(ctx context.Context, userID string) ([]*model.RecommendedPostCandidate, error) {
	if s.neo4jDriver == nil {
		return []*model.RecommendedPostCandidate{}, nil
	}

	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (viewer:User {id: $userID})
			MATCH (author:User)-[:POSTED]->(p:Post)
			WHERE NOT (viewer)-[:BLOCK]-(author)
			  AND (p.privacy = 'PUBLIC' OR (p.privacy = 'FRIEND' AND (viewer)-[:FRIEND]-(author)))
			RETURN p.id AS postID, author.id AS authorID
			ORDER BY p.createdAt DESC
			LIMIT 50
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}

		var candidates []*model.RecommendedPostCandidate
		for res.Next(ctx) {
			rec := res.Record()
			pVal, _ := rec.Get("postID")
			aVal, _ := rec.Get("authorID")

			c := &model.RecommendedPostCandidate{}
			if pVal != nil {
				c.PostID = pVal.(string)
			}
			if aVal != nil {
				c.AuthorID = aVal.(string)
			}
			candidates = append(candidates, c)
		}
		return candidates, nil
	})

	if err != nil {
		logger.Warn("Failed to execute GetSuggestedPosts Cypher query: %v", err)
		return []*model.RecommendedPostCandidate{}, nil
	}

	return resData.([]*model.RecommendedPostCandidate), nil
}
