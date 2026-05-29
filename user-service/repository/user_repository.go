package repository

import (
	"context"
	"fmt"
	"sync"
	"time"

	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/user-service/db"
	"social-network-go/user-service/model"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

var (
	ErrUserNotFound                  = exception.NewAppException(exception.UserNotFound)
	ErrUsernameAlreadyExists         = exception.NewAppException(exception.UsernameAlreadyExists)
	ErrLessThan30DaysNameChange      = exception.NewAppException(exception.LessThan30DaysSinceLastNameChange)
	ErrLessThan30DaysUsernameChange  = exception.NewAppException(exception.LessThan30DaysSinceLastUsernameChange)
	ErrLessThan30DaysBirthdateChange = exception.NewAppException(exception.LessThan30DaysSinceLastBirthdateChange)
	ErrHasBlocked                    = exception.NewAppException(exception.HasBlocked)
	ErrHasBeenBlocked                = exception.NewAppException(exception.HasBeenBlocked)
	ErrFriendNotFound                = exception.NewAppException(exception.FriendNotFound)
	ErrRequestNotFound               = exception.NewAppException(exception.RequestNotFound)
	ErrCannotMakeSelfRequest         = exception.NewAppException(exception.CanNotMakeSelfRequest)
	ErrSentRequestFailed             = exception.NewAppException(exception.SendFriendRequestFailed)
	ErrAddFriendRequestReceivedLimit = exception.NewAppException(exception.AddFriendRequestReceivedLimitReached)
)

type UserRepository interface {
	EnsureProfile(ctx context.Context, id, email, givenName, familyName, birthdate string) (*model.User, error)
	GetUserProfile(ctx context.Context, usernameOrID string, currentUserID string) (*model.User, error)
	GetFriends(ctx context.Context, username string, currentUserID string) ([]*model.User, error)
	GetSuggestedFriends(ctx context.Context, currentUserID string) ([]*model.User, error)
	GetMutualFriends(ctx context.Context, currentUserID string, targetUsername string) ([]*model.User, error)
	Unfriend(ctx context.Context, currentUserID string, targetUsername string) error
	Block(ctx context.Context, currentUserID string, targetUsername string) error
	Unblock(ctx context.Context, currentUserID string, targetUsername string) error
	GetBlockedUsers(ctx context.Context, currentUserID string) ([]*model.User, error)
	SendFriendRequest(ctx context.Context, currentUserID string, targetID string, requestReceivedCount int) error
	AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error
	DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error
	GetSentRequests(ctx context.Context, currentUserID string) ([]*model.User, error)
	GetReceivedRequests(ctx context.Context, currentUserID string) ([]*model.User, error)
	UpdateBio(ctx context.Context, currentUserID string, bio string) error
	UpdateBirthdate(ctx context.Context, currentUserID string, birthdate string, nextDate string) error
	UpdateName(ctx context.Context, currentUserID string, familyName, givenName string, nextDate string) error
	UpdateUsername(ctx context.Context, currentUserID string, username string, nextDate string) error
	UpdateProfilePicture(ctx context.Context, currentUserID string, fileID string) error
	RecordProfileView(ctx context.Context, viewerID, targetID string) error
}

type Neo4jUserRepository struct {
	fallbackUsers   map[string]*model.User
	fallbackFriends map[string][]string
	fallbackBlocks  map[string][]string
	fallbackReqs    map[string][]string
	mu              sync.RWMutex
}

func NewUserRepository() UserRepository {
	return &Neo4jUserRepository{
		fallbackUsers:   make(map[string]*model.User),
		fallbackFriends: make(map[string][]string),
		fallbackBlocks:  make(map[string][]string),
		fallbackReqs:    make(map[string][]string),
	}
}

// Helpers for data conversion
func getStringVal(val interface{}) string {
	if val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return v
	case dbtype.Date:
		return v.Time().Format("2006-01-02")
	case dbtype.LocalDateTime:
		return v.Time().Format(time.RFC3339)
	case time.Time:
		return v.Format(time.RFC3339)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getInt(v interface{}) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case int64:
		return int(val)
	case int:
		return val
	default:
		return 0
	}
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}

func getTimeVal(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}
	switch val := v.(type) {
	case dbtype.Date:
		return val.Time()
	case dbtype.LocalDateTime:
		return val.Time()
	case time.Time:
		return val
	case string:
		return parseTime(val)
	default:
		return time.Time{}
	}
}

func (r *Neo4jUserRepository) EnsureProfile(ctx context.Context, id, email, givenName, familyName, birthdate string) (*model.User, error) {
	username := fmt.Sprintf("user_%s", id[:8])
	if givenName == "" {
		givenName = "New"
	}
	if familyName == "" {
		familyName = "User"
	}
	birthdateStr := "1998-01-01"
	if birthdate != "" {
		// handle full ISO timestamp if passed (e.g. from JSON)
		if len(birthdate) > 10 {
			birthdate = birthdate[:10]
		}
		birthdateStr = birthdate
	}

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MERGE (u:User {id: $id})
				ON CREATE SET u.username = $username, u.email = $email, u.givenName = $givenName, u.familyName = $familyName, 
				              u.createdAt = datetime(), u.bio = "", u.friendCount = 0, u.birthdate = $birthdate,
				              u.nextChangeNameDate = datetime(), u.nextChangeBirthdateDate = datetime(), u.nextChangeUsernameDate = datetime()
				RETURN u.id
			`
			params := map[string]interface{}{
				"id":         id,
				"username":   username,
				"email":      email,
				"givenName":  givenName,
				"familyName": familyName,
				"birthdate":  birthdateStr,
			}
			result, err := tx.Run(ctx, query, params)
			if err != nil {
				return nil, err
			}
			if result.Next(ctx) {
				return result.Record().Values[0], nil
			}
			return nil, nil
		})
		if err == nil {
			logger.Info("Successfully synchronized Neo4j Profile node for ID: %s", id)
		} else {
			logger.Err(err).Error("Failed to synchronize Neo4j Profile node for ID: %s", id)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.fallbackUsers[id]; !ok {
		user := &model.User{
			ID:         id,
			Username:   username,
			Email:      email,
			GivenName:  givenName,
			FamilyName: familyName,
			Birthdate:  parseTime(birthdateStr),
			CreatedAt:  time.Now(),
		}
		r.fallbackUsers[id] = user
		r.fallbackUsers[username] = user
	}

	return r.fallbackUsers[id], nil
}

func getBool(v interface{}) bool {
	if v == nil {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}

func (r *Neo4jUserRepository) GetUserProfile(ctx context.Context, usernameOrID string, currentUserID string) (*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			if currentUserID != "" {
				queryBlock := `
					MATCH (u1:User {id: $currentID}), (u2:User)
					WHERE u2.username = $usernameOrID OR u2.id = $usernameOrID
					OPTIONAL MATCH (u1)-[:BLOCK]->(u2)
					OPTIONAL MATCH (u1)<-[:BLOCK]-(u2)
					RETURN u1.id = u2.id as isSelf, EXISTS((u1)-[:BLOCK]->(u2)) as iBlocked, EXISTS((u1)<-[:BLOCK]-(u2)) as theyBlocked
				`
				resBlock, err := tx.Run(ctx, queryBlock, map[string]interface{}{"currentID": currentUserID, "usernameOrID": usernameOrID})
				if err == nil && resBlock.Next(ctx) {
					rec := resBlock.Record()
					isSelf := rec.Values[0].(bool)
					iBlocked := rec.Values[1].(bool)
					theyBlocked := rec.Values[2].(bool)
					if !isSelf {
						if iBlocked {
							return nil, ErrHasBlocked
						}
						if theyBlocked {
							return nil, ErrHasBeenBlocked
						}
					}
				}
			}

			query := `
				MATCH (u:User)
				WHERE u.username = $usernameOrID OR u.id = $usernameOrID
				
				// Target counters
				OPTIONAL MATCH (u)-[:FRIEND]-(f)
				OPTIONAL MATCH (u)-[:BLOCK]->(b)
				OPTIONAL MATCH (u)-[:REQUEST]->(r)
				OPTIONAL MATCH (u)<-[:REQUEST]-(rc)
				
				// Viewer relationships (relative to currentUserID)
				OPTIONAL MATCH (cu:User {id: $currentUserID})
				OPTIONAL MATCH (cu)-[friendship:FRIEND]-(u)
				OPTIONAL MATCH (cu)-[requestOut:REQUEST]->(u)
				OPTIONAL MATCH (u)-[requestIn:REQUEST]->(cu)
				OPTIONAL MATCH (cu)-[blockOut:BLOCK]->(u)
				OPTIONAL MATCH (u)-[blockIn:BLOCK]->(cu)
				
				// Mutual friends
				WITH u, count(DISTINCT f) as friendCount, count(DISTINCT b) as blockCount,
				     count(DISTINCT r) as sentCount, count(DISTINCT rc) as recvCount,
				     cu, friendship, requestOut, requestIn, blockOut, blockIn
				
				WITH u, friendCount, blockCount, sentCount, recvCount, cu, friendship, requestOut, requestIn, blockOut, blockIn,
				     size([(cu)-[:FRIEND]-(mutual:User)-[:FRIEND]-(u) | mutual]) AS mutualFriendsCount
				
				// Count posts
				OPTIONAL MATCH (u)-[:POSTED]->(post:Post)
				
				RETURN u.id, u.username, u.givenName, u.familyName, u.email, u.bio, u.birthdate, 
				       friendCount, blockCount, sentCount, recvCount,
				       u.nextChangeNameDate, u.nextChangeBirthdateDate, u.nextChangeUsernameDate, u.createdAt,
				       u.profilePictureId,
				       
				       CASE WHEN friendship IS NOT NULL THEN true ELSE false END as isFriend,
				       CASE 
				           WHEN requestOut IS NOT NULL THEN 'OUT'
				           WHEN requestIn IS NOT NULL THEN 'IN'
				           ELSE 'NONE'
				       END as request,
				       CASE
				           WHEN blockOut IS NOT NULL THEN 'BLOCKED'
				           WHEN blockIn IS NOT NULL THEN 'HAS_BEEN_BLOCKED'
				           ELSE 'NORMAL'
				       END as blockStatus,
				       mutualFriendsCount,
				       count(DISTINCT post) as postCount
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"usernameOrID": usernameOrID, "currentUserID": currentUserID})
			if err != nil {
				return nil, err
			}

			if result.Next(ctx) {
				vals := result.Record().Values
				user := &model.User{
					ID:                      getStringVal(vals[0]),
					Username:                getStringVal(vals[1]),
					GivenName:               getStringVal(vals[2]),
					FamilyName:              getStringVal(vals[3]),
					Email:                   getStringVal(vals[4]),
					Bio:                     getStringVal(vals[5]),
					Birthdate:               getTimeVal(vals[6]),
					FriendCount:             getInt(vals[7]),
					BlockCount:              getInt(vals[8]),
					RequestSentCount:        getInt(vals[9]),
					RequestReceivedCount:    getInt(vals[10]),
					NextChangeNameDate:      getTimeVal(vals[11]),
					NextChangeBirthdateDate: getTimeVal(vals[12]),
					NextChangeUsernameDate:  getTimeVal(vals[13]),
					CreatedAt:               getTimeVal(vals[14]),
					ProfilePictureId:        getStringVal(vals[15]),
					IsFriend:                getBool(vals[16]),
					Request:                 getStringVal(vals[17]),
					BlockStatus:             getStringVal(vals[18]),
					MutualFriendsCount:      getInt(vals[19]),
					PostCount:               getInt(vals[20]),
				}
				return user, nil
			}
			return nil, ErrUserNotFound
		})

		if err != nil {
			return nil, err
		}
		return res.(*model.User), nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	user, ok := r.fallbackUsers[usernameOrID]
	if !ok {
		return nil, ErrUserNotFound
	}

	if currentUserID != "" && currentUserID != user.ID {
		for _, bID := range r.fallbackBlocks[currentUserID] {
			if bID == user.ID {
				return nil, ErrHasBlocked
			}
		}
		for _, bID := range r.fallbackBlocks[user.ID] {
			if bID == currentUserID {
				return nil, ErrHasBeenBlocked
			}
		}
	}

	user.FriendCount = len(r.fallbackFriends[user.ID])
	user.BlockCount = len(r.fallbackBlocks[user.ID])
	user.RequestSentCount = len(r.fallbackReqs[user.ID])

	return user, nil
}

func (r *Neo4jUserRepository) GetFriends(ctx context.Context, username string, currentUserID string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {username: $username})-[:FRIEND]->(f:User)
				RETURN f.id, f.username, f.givenName, f.familyName, f.email, f.bio, f.birthdate, f.profilePictureId
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"username": username})
			if err != nil {
				return nil, err
			}

			var list []*model.User
			for result.Next(ctx) {
				vals := result.Record().Values
				list = append(list, &model.User{
					ID:               getStringVal(vals[0]),
					Username:         getStringVal(vals[1]),
					GivenName:        getStringVal(vals[2]),
					FamilyName:       getStringVal(vals[3]),
					Email:            getStringVal(vals[4]),
					Bio:              getStringVal(vals[5]),
					Birthdate:        getTimeVal(vals[6]),
					ProfilePictureId: getStringVal(vals[7]),
				})
			}
			return list, nil
		})
		if err == nil {
			return res.([]*model.User), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	u, ok := r.fallbackUsers[username]
	if !ok {
		return nil, ErrUserNotFound
	}

	var list []*model.User
	for _, friendID := range r.fallbackFriends[u.ID] {
		if friend, ok := r.fallbackUsers[friendID]; ok {
			list = append(list, friend)
		}
	}
	return list, nil
}

func (r *Neo4jUserRepository) GetSuggestedFriends(ctx context.Context, currentUserID string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})
				CALL {
					WITH u
					MATCH (u)-[:FRIEND]-(:User)-[:FRIEND]-(foaf:User)
					RETURN foaf
					UNION
					WITH u
					MATCH (u)-[:VIEW_PROFILE]-(foaf:User)
					RETURN foaf
					UNION
					WITH u
					MATCH (u)-[:IS_MEMBER_OF]->(:Chat)<-[:IS_MEMBER_OF]-(foaf:User)
					RETURN foaf
					UNION
					WITH u
					MATCH (u)-[:LIKED]->(:Post)<-[:POSTED]-(foaf:User)
					RETURN foaf
					UNION
					WITH u
					MATCH (u)-[:POSTED]->(:Post)<-[:LIKED]-(foaf:User)
					RETURN foaf
					UNION
					WITH u
					MATCH (u)-[:COMMENTED]->(:Comment)-[:COMMENT_OF]->(:Post)<-[:POSTED]-(foaf:User)
					RETURN foaf
					UNION
					WITH u
					MATCH (u)-[:POSTED]->(:Post)<-[:COMMENT_OF]-(:Comment)<-[:COMMENTED]-(foaf:User)
					RETURN foaf
				}
				WITH u, foaf
				WHERE foaf <> u
				  AND NOT (u)-[:FRIEND]-(foaf)
				  AND NOT (u)-[:BLOCK]-(foaf)
				  AND NOT (u)-[:REQUEST]-(foaf)
				WITH distinct u, foaf

				// 1. Calculate counts using COUNT subqueries
				WITH u, foaf,
				     COUNT { (u)-[:FRIEND]-(:User)-[:FRIEND]-(foaf) } as mutualCount,
				     COUNT { (u)-[:VIEW_PROFILE]->(foaf) } as viewOut,
				     COUNT { (u)<-[:VIEW_PROFILE]-(foaf) } as viewIn,
				     COUNT { (u)-[:IS_MEMBER_OF]->(:Chat)<-[:IS_MEMBER_OF]-(foaf) } as chatRooms,
				     COUNT { (u)-[:LIKED]->(:Post)<-[:POSTED]-(foaf) } +
				     COUNT { (u)-[:POSTED]->(:Post)<-[:LIKED]-(foaf) } +
				     COUNT { (u)-[:LIKED]->(:Post)<-[:LIKED]-(foaf) } +
				     COUNT { (u)-[:COMMENTED]->(:Comment)-[:COMMENT_OF]->(:Post)<-[:POSTED]-(foaf) } +
				     COUNT { (u)-[:POSTED]->(:Post)<-[:COMMENT_OF]-(:Comment)<-[:COMMENTED]-(foaf) } +
				     COUNT { (u)-[:COMMENTED]->(:Comment)-[:COMMENT_OF]->(:Post)<-[:COMMENT_OF]-(:Comment)<-[:COMMENTED]-(foaf) } +
				     COUNT { (u)-[:COMMENTED]->(:Comment)-[:REPLY_OF]-(:Comment)<-[:COMMENTED]-(foaf) } as interactions
				WITH u, foaf, mutualCount, viewOut, viewIn, chatRooms, interactions,
				     abs(toInteger(substring(coalesce(u.birthdate, "1998-01-01"), 0, 4)) - toInteger(substring(coalesce(foaf.birthdate, "1998-01-01"), 0, 4))) as ageDiff

				// Score calculation
				WITH foaf,
				     (mutualCount * 5) + (viewOut * 2) + (viewIn * 1) + (case when chatRooms > 0 then 30 else 0 end) + (interactions * 2) - (ageDiff * 2) as score
				RETURN foaf.id, foaf.username, foaf.givenName, foaf.familyName, foaf.email, foaf.bio, score, foaf.profilePictureId
				ORDER BY score DESC LIMIT 20
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID})
			if err != nil {
				return nil, err
			}

			var list []*model.User
			for result.Next(ctx) {
				vals := result.Record().Values
				list = append(list, &model.User{
					ID:               getStringVal(vals[0]),
					Username:         getStringVal(vals[1]),
					GivenName:        getStringVal(vals[2]),
					FamilyName:       getStringVal(vals[3]),
					Email:            getStringVal(vals[4]),
					Bio:              getStringVal(vals[5]),
					ProfilePictureId: getStringVal(vals[7]),
				})
			}
			return list, nil
		})
		if err == nil {
			return res.([]*model.User), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*model.User
	friends := r.fallbackFriends[currentUserID]
	isFriendOrSelf := func(id string) bool {
		if id == currentUserID {
			return true
		}
		for _, fID := range friends {
			if fID == id {
				return true
			}
		}
		return false
	}

	for _, u := range r.fallbackUsers {
		if u.ID != u.Username && !isFriendOrSelf(u.ID) {
			list = append(list, u)
		}
	}
	return list, nil
}

func (r *Neo4jUserRepository) GetMutualFriends(ctx context.Context, currentUserID string, targetUsername string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $id})-[:FRIEND]-(m:User)-[:FRIEND]-(u2:User {username: $username})
				RETURN m.id, m.username, m.givenName, m.familyName, m.email, m.bio, m.profilePictureId
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
			if err != nil {
				return nil, err
			}

			var list []*model.User
			for result.Next(ctx) {
				vals := result.Record().Values
				list = append(list, &model.User{
					ID:               getStringVal(vals[0]),
					Username:         getStringVal(vals[1]),
					GivenName:        getStringVal(vals[2]),
					FamilyName:       getStringVal(vals[3]),
					Email:            getStringVal(vals[4]),
					Bio:              getStringVal(vals[5]),
					ProfilePictureId: getStringVal(vals[6]),
				})
			}
			return list, nil
		})
		if err == nil {
			return res.([]*model.User), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	target, ok := r.fallbackUsers[targetUsername]
	if !ok {
		return nil, ErrUserNotFound
	}

	myFriends := r.fallbackFriends[currentUserID]
	theirFriends := r.fallbackFriends[target.ID]

	var list []*model.User
	for _, mf := range myFriends {
		for _, tf := range theirFriends {
			if mf == tf {
				if mUser, ok := r.fallbackUsers[mf]; ok {
					list = append(list, mUser)
				}
			}
		}
	}
	return list, nil
}

func (r *Neo4jUserRepository) Unfriend(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			queryCheck := `MATCH (u1:User {id: $id})-[rel:FRIEND]-(u2:User {username: $username}) RETURN rel`
			resCheck, _ := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID, "username": targetUsername})
			if !resCheck.Next(ctx) {
				return nil, ErrFriendNotFound
			}

			query := `
				MATCH (u1:User {id: $id})-[rel:FRIEND]-(u2:User {username: $username})
				DELETE rel
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	target, ok := r.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	found := false
	myFriends := r.fallbackFriends[currentUserID]
	for i, f := range myFriends {
		if f == target.ID {
			r.fallbackFriends[currentUserID] = append(myFriends[:i], myFriends[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrFriendNotFound
	}

	theirFriends := r.fallbackFriends[target.ID]
	for i, f := range theirFriends {
		if f == currentUserID {
			r.fallbackFriends[target.ID] = append(theirFriends[:i], theirFriends[i+1:]...)
			break
		}
	}

	return nil
}

func (r *Neo4jUserRepository) Block(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $id}), (u2:User {username: $username})
				MERGE (u1)-[:BLOCK]->(u2)
				WITH u1, u2
				OPTIONAL MATCH (u1)-[f:FRIEND]-(u2)
				DELETE f
				WITH u1, u2
				OPTIONAL MATCH (u1)-[req:REQUEST]-(u2)
				DELETE req
				RETURN u1.id
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	target, ok := r.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	r.fallbackBlocks[currentUserID] = append(r.fallbackBlocks[currentUserID], target.ID)

	r.mu.Unlock()
	_ = r.Unfriend(ctx, currentUserID, targetUsername)
	r.mu.Lock()

	return nil
}

func (r *Neo4jUserRepository) Unblock(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $id})-[rel:BLOCK]->(u2:User {username: $username})
				DELETE rel
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	target, ok := r.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	myBlocks := r.fallbackBlocks[currentUserID]
	for i, bID := range myBlocks {
		if bID == target.ID {
			r.fallbackBlocks[currentUserID] = append(myBlocks[:i], myBlocks[i+1:]...)
			break
		}
	}
	return nil
}

func (r *Neo4jUserRepository) GetBlockedUsers(ctx context.Context, currentUserID string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})-[:BLOCK]->(b:User)
				RETURN b.id, b.username, b.givenName, b.familyName, b.email, b.bio, b.profilePictureId
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID})
			if err != nil {
				return nil, err
			}

			var list []*model.User
			for result.Next(ctx) {
				vals := result.Record().Values
				list = append(list, &model.User{
					ID:               getStringVal(vals[0]),
					Username:         getStringVal(vals[1]),
					GivenName:        getStringVal(vals[2]),
					FamilyName:       getStringVal(vals[3]),
					Email:            getStringVal(vals[4]),
					Bio:              getStringVal(vals[5]),
					ProfilePictureId: getStringVal(vals[6]),
				})
			}
			return list, nil
		})
		if err == nil {
			return res.([]*model.User), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*model.User
	for _, bID := range r.fallbackBlocks[currentUserID] {
		if b, ok := r.fallbackUsers[bID]; ok {
			list = append(list, b)
		}
	}
	return list, nil
}

func (r *Neo4jUserRepository) SendFriendRequest(ctx context.Context, currentUserID string, targetID string, requestReceivedCount int) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			queryCheck := `
				MATCH (u1:User {id: $id}), (u2:User {id: $targetID})
				OPTIONAL MATCH (u1)-[fr:FRIEND]-(u2)
				OPTIONAL MATCH (u1)-[req:REQUEST]-(u2)
				RETURN fr IS NOT NULL as isFriend, req IS NOT NULL as isRequested
			`
			resCheck, _ := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID, "targetID": targetID})
			if resCheck.Next(ctx) {
				rec := resCheck.Record()
				if rec.Values[0].(bool) || rec.Values[1].(bool) {
					return nil, ErrSentRequestFailed
				}
			}

			if requestReceivedCount+1 > model.MaxReceivedRequestCount {
				return nil, ErrAddFriendRequestReceivedLimit
			}

			query := `
				MATCH (u1:User {id: $id}), (u2:User {id: $targetID})
				MERGE (u1)-[:REQUEST]->(u2)
				RETURN u1.id
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "targetID": targetID})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.fallbackReqs[currentUserID] = append(r.fallbackReqs[currentUserID], targetID)
	return nil
}

func (r *Neo4jUserRepository) AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $id})<-[req:REQUEST]-(u2:User {username: $username})
				DELETE req
				WITH u1, u2
				MERGE (u1)-[:FRIEND]-(u2)
				RETURN u1.id
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
			if err == nil && !res.Next(ctx) {
				return nil, ErrRequestNotFound
			}
			return nil, err
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	target, ok := r.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	found := false
	reqs := r.fallbackReqs[target.ID]
	for i, rID := range reqs {
		if rID == currentUserID {
			r.fallbackReqs[target.ID] = append(reqs[:i], reqs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrRequestNotFound
	}

	r.fallbackFriends[currentUserID] = append(r.fallbackFriends[currentUserID], target.ID)
	r.fallbackFriends[target.ID] = append(r.fallbackFriends[target.ID], currentUserID)

	return nil
}

func (r *Neo4jUserRepository) DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $id})-[req:REQUEST]-(u2:User {username: $username})
				DELETE req
				RETURN u1.id
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
			if err == nil && !res.Next(ctx) {
				query2 := `
					MATCH (u1:User {id: $id})<-[req:REQUEST]-(u2:User {username: $username})
					DELETE req
					RETURN u1.id
				`
				res2, err2 := tx.Run(ctx, query2, map[string]interface{}{"id": currentUserID, "username": targetUsername})
				if err2 == nil && !res2.Next(ctx) {
					return nil, ErrRequestNotFound
				}
				return nil, err2
			}
			return nil, err
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	target, ok := r.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	found := false
	reqs := r.fallbackReqs[currentUserID]
	for i, rID := range reqs {
		if rID == target.ID {
			r.fallbackReqs[currentUserID] = append(reqs[:i], reqs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		reqs2 := r.fallbackReqs[target.ID]
		for i, rID := range reqs2 {
			if rID == currentUserID {
				r.fallbackReqs[target.ID] = append(reqs2[:i], reqs2[i+1:]...)
				found = true
				break
			}
		}
	}

	if !found {
		return ErrRequestNotFound
	}
	return nil
}

func (r *Neo4jUserRepository) GetSentRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})-[:REQUEST]->(target:User)
				RETURN target.id, target.username, target.givenName, target.familyName, target.email, target.bio, target.profilePictureId
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID})
			if err != nil {
				return nil, err
			}

			var list []*model.User
			for result.Next(ctx) {
				vals := result.Record().Values
				list = append(list, &model.User{
					ID:               getStringVal(vals[0]),
					Username:         getStringVal(vals[1]),
					GivenName:        getStringVal(vals[2]),
					FamilyName:       getStringVal(vals[3]),
					Email:            getStringVal(vals[4]),
					Bio:              getStringVal(vals[5]),
					ProfilePictureId: getStringVal(vals[6]),
				})
			}
			return list, nil
		})
		if err == nil {
			return res.([]*model.User), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*model.User
	for _, rID := range r.fallbackReqs[currentUserID] {
		if target, ok := r.fallbackUsers[rID]; ok {
			list = append(list, target)
		}
	}
	return list, nil
}

func (r *Neo4jUserRepository) GetReceivedRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})<-[:REQUEST]-(sender:User)
				RETURN sender.id, sender.username, sender.givenName, sender.familyName, sender.email, sender.bio, sender.profilePictureId
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"id": currentUserID})
			if err != nil {
				return nil, err
			}

			var list []*model.User
			for result.Next(ctx) {
				vals := result.Record().Values
				list = append(list, &model.User{
					ID:               getStringVal(vals[0]),
					Username:         getStringVal(vals[1]),
					GivenName:        getStringVal(vals[2]),
					FamilyName:       getStringVal(vals[3]),
					Email:            getStringVal(vals[4]),
					Bio:              getStringVal(vals[5]),
					ProfilePictureId: getStringVal(vals[6]),
				})
			}
			return list, nil
		})
		if err == nil {
			return res.([]*model.User), nil
		}
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*model.User
	for senderID, reqs := range r.fallbackReqs {
		for _, targetID := range reqs {
			if targetID == currentUserID {
				if sender, ok := r.fallbackUsers[senderID]; ok {
					list = append(list, sender)
				}
			}
		}
	}
	return list, nil
}

func (r *Neo4jUserRepository) UpdateBio(ctx context.Context, currentUserID string, bio string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})
				SET u.bio = $bio
				RETURN u.id
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "bio": bio})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if u, ok := r.fallbackUsers[currentUserID]; ok {
		u.Bio = bio
	}
	return nil
}

func (r *Neo4jUserRepository) UpdateBirthdate(ctx context.Context, currentUserID string, birthdate string, nextDate string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			queryCheck := `MATCH (u:User {id: $id}) RETURN u.nextChangeBirthdateDate`
			resCheck, err := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID})
			if err == nil && resCheck.Next(ctx) {
				next := getTimeVal(resCheck.Record().Values[0])
				if time.Now().Before(next) {
					return nil, ErrLessThan30DaysBirthdateChange
				}
			}

			query := `
				MATCH (u:User {id: $id})
				SET u.birthdate = $birthdate, u.nextChangeBirthdateDate = $nextDate
				RETURN u.id
			`
			return tx.Run(ctx, query, map[string]interface{}{
				"id":        currentUserID,
				"birthdate": birthdate,
				"nextDate":  nextDate,
			})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.fallbackUsers[currentUserID]; ok {
		if time.Now().Before(u.NextChangeBirthdateDate) {
			return ErrLessThan30DaysBirthdateChange
		}
		u.Birthdate = parseTime(birthdate)
		u.NextChangeBirthdateDate = parseTime(nextDate)
	}
	return nil
}

func (r *Neo4jUserRepository) UpdateName(ctx context.Context, currentUserID string, familyName, givenName string, nextDate string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			queryCheck := `MATCH (u:User {id: $id}) RETURN u.nextChangeNameDate`
			resCheck, err := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID})
			if err == nil && resCheck.Next(ctx) {
				next := getTimeVal(resCheck.Record().Values[0])
				if time.Now().Before(next) {
					return nil, ErrLessThan30DaysNameChange
				}
			}

			query := `
				MATCH (u:User {id: $id})
				SET u.familyName = $familyName, u.givenName = $givenName, u.nextChangeNameDate = $nextDate
				RETURN u.id
			`
			return tx.Run(ctx, query, map[string]interface{}{
				"id":         currentUserID,
				"familyName": familyName,
				"givenName":  givenName,
				"nextDate":   nextDate,
			})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.fallbackUsers[currentUserID]; ok {
		if time.Now().Before(u.NextChangeNameDate) {
			return ErrLessThan30DaysNameChange
		}
		u.FamilyName = familyName
		u.GivenName = givenName
		u.NextChangeNameDate = parseTime(nextDate)
	}
	return nil
}

func (r *Neo4jUserRepository) UpdateUsername(ctx context.Context, currentUserID string, username string, nextDate string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			queryExist := `MATCH (u:User {username: $username}) RETURN u.id`
			resExist, _ := tx.Run(ctx, queryExist, map[string]interface{}{"username": username})
			if resExist.Next(ctx) {
				return nil, ErrUsernameAlreadyExists
			}

			queryCheck := `MATCH (u:User {id: $id}) RETURN u.nextChangeUsernameDate`
			resCheck, err := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID})
			if err == nil && resCheck.Next(ctx) {
				next := getTimeVal(resCheck.Record().Values[0])
				if time.Now().Before(next) {
					return nil, ErrLessThan30DaysUsernameChange
				}
			}

			query := `
				MATCH (u:User {id: $id})
				SET u.username = $username, u.nextChangeUsernameDate = $nextDate
				RETURN u.id
			`
			return tx.Run(ctx, query, map[string]interface{}{
				"id":       currentUserID,
				"username": username,
				"nextDate": nextDate,
			})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.fallbackUsers[currentUserID]; ok {
		if _, exists := r.fallbackUsers[username]; exists {
			return ErrUsernameAlreadyExists
		}
		if time.Now().Before(u.NextChangeUsernameDate) {
			return ErrLessThan30DaysUsernameChange
		}
		delete(r.fallbackUsers, u.Username)
		u.Username = username
		u.NextChangeUsernameDate = parseTime(nextDate)
		r.fallbackUsers[username] = u
	}
	return nil
}

func (r *Neo4jUserRepository) UpdateProfilePicture(ctx context.Context, currentUserID string, fileID string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})
				SET u.profilePictureId = $fileId
				RETURN u.id
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "fileId": fileID})
		})
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.fallbackUsers[currentUserID]; ok {
		u.ProfilePictureId = fileID
	}
	return nil
}

func (r *Neo4jUserRepository) RecordProfileView(ctx context.Context, viewerID, targetID string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $viewerID}), (u2:User {id: $targetID})
				MERGE (u1)-[v:VIEW_PROFILE]->(u2)
				ON CREATE SET v.createdAt = datetime(), v.times = 1
				ON MATCH SET v.times = coalesce(v.times, 0) + 1, v.updatedAt = datetime()
				RETURN u1.id
			`
			return tx.Run(ctx, query, map[string]interface{}{"viewerID": viewerID, "targetID": targetID})
		})
		return err
	}
	return nil
}
