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
	if s.neo4jDriver == nil || userID == "" {
		return []*model.RecommendedUserCandidate{}, nil
	}

	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	resData, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		// 1. Friends-of-friends recommendation (2nd degree)
		query := `
			MATCH (u:User {id: $userID})
			MATCH (u)-[:FRIEND]-(f:User)-[:FRIEND]-(suggested:User)
			WHERE NOT (u)-[:FRIEND]-(suggested)
			  AND NOT (u)-[:BLOCK]-(suggested)
			  AND suggested.id <> $userID
			RETURN suggested.id AS id, 
			       suggested.username AS username, 
			       coalesce(suggested.givenName, '') AS givenName,
			       coalesce(suggested.familyName, '') AS familyName,
			       coalesce(suggested.profilePictureUrl, '') AS profilePictureUrl,
			       COUNT(f) AS mutualCount
			ORDER BY mutualCount DESC
			LIMIT 20
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{"userID": userID})
		if err != nil {
			return nil, err
		}

		candidates := make([]*model.RecommendedUserCandidate, 0)
		seen := make(map[string]bool)

		for res.Next(ctx) {
			rec := res.Record()
			idVal, _ := rec.Get("id")
			userVal, _ := rec.Get("username")
			givenVal, _ := rec.Get("givenName")
			familyVal, _ := rec.Get("familyName")
			picVal, _ := rec.Get("profilePictureUrl")
			cntVal, _ := rec.Get("mutualCount")

			idStr := ""
			if idVal != nil {
				idStr = idVal.(string)
			}
			if idStr == "" || seen[idStr] {
				continue
			}
			seen[idStr] = true

			c := &model.RecommendedUserCandidate{
				ID:     idStr,
				UserID: idStr,
			}
			if userVal != nil {
				c.Username = userVal.(string)
			}
			if givenVal != nil {
				c.GivenName = givenVal.(string)
			}
			if familyVal != nil {
				c.FamilyName = familyVal.(string)
			}
			if picVal != nil {
				c.ProfilePictureUrl = picVal.(string)
				c.Avatar = picVal.(string)
			}
			if cntVal != nil {
				c.MutualFriendsCount = int(cntVal.(int64))
			}
			candidates = append(candidates, c)
		}

		// 2. Fallback: If less than 10 candidates (e.g. new user or disconnected graph), suggest other non-friend users
		if len(candidates) < 10 {
			fallbackLimit := 15 - len(candidates)
			fallbackQuery := `
				MATCH (suggested:User)
				WHERE suggested.id <> $userID
				  AND NOT EXISTS((:User {id: $userID})-[:FRIEND]-(suggested))
				  AND NOT EXISTS((:User {id: $userID})-[:BLOCK]-(suggested))
				RETURN suggested.id AS id, 
				       suggested.username AS username, 
				       coalesce(suggested.givenName, '') AS givenName,
				       coalesce(suggested.familyName, '') AS familyName,
				       coalesce(suggested.profilePictureUrl, '') AS profilePictureUrl,
				       0 AS mutualCount
				LIMIT $limit
			`
			fres, ferr := tx.Run(ctx, fallbackQuery, map[string]interface{}{
				"userID": userID,
				"limit":  fallbackLimit,
			})
			if ferr == nil {
				for fres.Next(ctx) {
					rec := fres.Record()
					idVal, _ := rec.Get("id")
					userVal, _ := rec.Get("username")
					givenVal, _ := rec.Get("givenName")
					familyVal, _ := rec.Get("familyName")
					picVal, _ := rec.Get("profilePictureUrl")

					idStr := ""
					if idVal != nil {
						idStr = idVal.(string)
					}
					if idStr == "" || seen[idStr] {
						continue
					}
					seen[idStr] = true

					c := &model.RecommendedUserCandidate{
						ID:                 idStr,
						UserID:             idStr,
						MutualFriendsCount: 0,
					}
					if userVal != nil {
						c.Username = userVal.(string)
					}
					if givenVal != nil {
						c.GivenName = givenVal.(string)
					}
					if familyVal != nil {
						c.FamilyName = familyVal.(string)
					}
					if picVal != nil {
						c.ProfilePictureUrl = picVal.(string)
						c.Avatar = picVal.(string)
					}
					candidates = append(candidates, c)
				}
			}
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
