package service

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	goredis "github.com/redis/go-redis/v9"

	"social-network-go/internal/runtimeconfig"
	"social-network-go/recommendation-service/db"
)

const (
	nearbyUsersGeoKey    = "nearby:users"
	nearbyAliveKeyPrefix = "nearby:alive:"
)

var ErrMutualOptInRequired = errors.New("ENABLE_NEARBY_TO_DISCOVER")

type NearbyUserCandidate struct {
	ID                string  `json:"id"`
	UserID            string  `json:"userId"`
	Username          string  `json:"username"`
	GivenName         string  `json:"givenName"`
	FamilyName        string  `json:"familyName"`
	ProfilePictureUrl string  `json:"profilePictureUrl"`
	Avatar            string  `json:"avatar"`
	DistanceKm        float64 `json:"distanceKm"`
	DistanceLabel     string  `json:"distanceLabel"`
	Lat               float64 `json:"lat"`
	Lng               float64 `json:"lng"`
	IsFriend          bool    `json:"isFriend"`
}

func (s *RecommendationService) GetNearbyUsers(ctx context.Context, userID string, lat, lng, radiusKm float64, limit int) ([]*NearbyUserCandidate, error) {
	rtCfg := runtimeconfig.NewProvider(db.RedisClient)
	if raw, ok := rtCfg.String(ctx, "location.nearby_enabled"); ok && raw == "false" {
		return []*NearbyUserCandidate{}, nil
	}

	if db.RedisClient == nil {
		return []*NearbyUserCandidate{}, nil
	}

	if raw, ok := rtCfg.String(ctx, "location.nearby_require_mutual_optin"); ok && raw == "true" {
		alive, err := db.RedisClient.Exists(ctx, nearbyAliveKeyPrefix+userID).Result()
		if err != nil || alive == 0 {
			return nil, ErrMutualOptInRequired
		}
	}

	maxRadius := float64(rtCfg.Int(ctx, "location.nearby_max_radius_km", 10))
	if radiusKm <= 0 || radiusKm > maxRadius {
		radiusKm = maxRadius
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	locations, err := db.RedisClient.GeoSearchLocation(ctx, nearbyUsersGeoKey, &goredis.GeoSearchLocationQuery{
		GeoSearchQuery: goredis.GeoSearchQuery{
			Longitude:  lng,
			Latitude:   lat,
			Radius:     radiusKm,
			RadiusUnit: "km",
			Sort:       "ASC",
			Count:      limit * 2,
		},
		WithCoord: true,
		WithDist:  true,
	}).Result()
	if err != nil {
		return nil, err
	}

	if len(locations) == 0 {
		return []*NearbyUserCandidate{}, nil
	}

	// Verify heartbeat TTL for each candidate and clean up expired entries
	pipe := db.RedisClient.Pipeline()
	aliveCmds := make(map[string]*goredis.IntCmd, len(locations))
	for _, loc := range locations {
		if loc.Name == "" || loc.Name == userID {
			continue
		}
		aliveCmds[loc.Name] = pipe.Exists(ctx, nearbyAliveKeyPrefix+loc.Name)
	}
	_, _ = pipe.Exec(ctx)

	activeLocs := make([]goredis.GeoLocation, 0, len(locations))
	var expiredIDs []interface{}
	var candidateIDs []string
	for _, loc := range locations {
		if loc.Name == "" || loc.Name == userID {
			continue
		}
		cmd, ok := aliveCmds[loc.Name]
		if !ok || cmd.Val() == 0 {
			expiredIDs = append(expiredIDs, loc.Name)
			continue
		}
		activeLocs = append(activeLocs, loc)
		candidateIDs = append(candidateIDs, loc.Name)
	}
	if len(expiredIDs) > 0 {
		_ = db.RedisClient.ZRem(ctx, nearbyUsersGeoKey, expiredIDs...).Err()
	}
	if len(candidateIDs) == 0 {
		return []*NearbyUserCandidate{}, nil
	}

	userMap, blockedSet, friendSet := s.enrichNearbyCandidatesFromNeo4j(ctx, userID, candidateIDs)

	results := make([]*NearbyUserCandidate, 0, len(activeLocs))
	for _, loc := range activeLocs {
		if blockedSet[loc.Name] {
			continue
		}
		u, ok := userMap[loc.Name]
		if !ok {
			continue
		}
		roundedKm, label := formatRoundedDistance(loc.Dist)
		u.DistanceKm = roundedKm
		u.DistanceLabel = label
		u.Lat = loc.Latitude
		u.Lng = loc.Longitude
		u.IsFriend = friendSet[loc.Name]
		results = append(results, u)
		if len(results) >= limit {
			break
		}
	}
	return results, nil
}

func (s *RecommendationService) enrichNearbyCandidatesFromNeo4j(ctx context.Context, userID string, candidateIDs []string) (map[string]*NearbyUserCandidate, map[string]bool, map[string]bool) {
	userMap := make(map[string]*NearbyUserCandidate)
	blockedSet := make(map[string]bool)
	friendSet := make(map[string]bool)

	if s.neo4jDriver == nil || len(candidateIDs) == 0 {
		return userMap, blockedSet, friendSet
	}

	session := s.neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	_, _ = session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (c:User)
			WHERE c.id IN $candidateIDs
			OPTIONAL MATCH (u:User {id: $userID})
			RETURN c.id AS id,
			       c.username AS username,
			       coalesce(c.givenName, '') AS givenName,
			       coalesce(c.familyName, '') AS familyName,
			       coalesce(c.profilePictureUrl, '') AS profilePictureUrl,
			       CASE WHEN u IS NOT NULL AND (u)-[:BLOCK]-(c) THEN true ELSE false END AS isBlocked,
			       CASE WHEN u IS NOT NULL AND (u)-[:FRIEND]-(c) THEN true ELSE false END AS isFriend
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"userID":       userID,
			"candidateIDs": candidateIDs,
		})
		if err != nil {
			return nil, err
		}
		for res.Next(ctx) {
			rec := res.Record()
			idVal, _ := rec.Get("id")
			if idVal == nil {
				continue
			}
			idStr, _ := idVal.(string)
			if idStr == "" {
				continue
			}
			isBlocked, _ := rec.Get("isBlocked")
			if b, ok := isBlocked.(bool); ok && b {
				blockedSet[idStr] = true
				continue
			}
			isFriend, _ := rec.Get("isFriend")
			if f, ok := isFriend.(bool); ok && f {
				friendSet[idStr] = true
			}
			username, _ := rec.Get("username")
			givenName, _ := rec.Get("givenName")
			familyName, _ := rec.Get("familyName")
			pic, _ := rec.Get("profilePictureUrl")

			c := &NearbyUserCandidate{
				ID:     idStr,
				UserID: idStr,
			}
			if v, ok := username.(string); ok {
				c.Username = v
			}
			if v, ok := givenName.(string); ok {
				c.GivenName = v
			}
			if v, ok := familyName.(string); ok {
				c.FamilyName = v
			}
			if v, ok := pic.(string); ok {
				c.ProfilePictureUrl = v
				c.Avatar = v
			}
			userMap[idStr] = c
		}
		return nil, nil
	})

	return userMap, blockedSet, friendSet
}

// formatRoundedDistance obscures exact meter-level distance to prevent trilateration attacks.
func formatRoundedDistance(distKm float64) (float64, string) {
	if distKm < 0.1 {
		return 0.1, "< 100m"
	}
	if distKm < 0.5 {
		return 0.5, "< 500m"
	}
	if distKm < 1.0 {
		return 1.0, "< 1 km"
	}
	rounded := math.Round(distKm*10) / 10
	return rounded, fmt.Sprintf("~%.1f km", rounded)
}
