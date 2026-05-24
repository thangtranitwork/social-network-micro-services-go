package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"social-network-go/user-service/db"
	"social-network-go/user-service/model"
	red "social-network-go/user-service/redis"
	"social-network-go/user-service/util/validation"
	"social-network-go/exception"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
)

var (
	ErrUserNotFound                   = exception.NewAppException(exception.UserNotFound)
	ErrUsernameAlreadyExists          = exception.NewAppException(exception.UsernameAlreadyExists)
	ErrInvalidUsername                = exception.NewAppException(exception.InvalidUsername)
	ErrInvalidAge                     = exception.NewAppException(exception.AgeMustBeAtLeast16)
	ErrInvalidName                    = exception.NewAppException(exception.InvalidGivenNameLength) // fallback or match to Name
	ErrCooldownNotFinished            = exception.NewAppException(exception.LessThan30DaysSinceLastNameChange)
	ErrUnauthorized                   = exception.NewAppException(exception.Unauthorized)
	ErrProfilePictureRequired         = exception.NewAppException(exception.ProfilePictureRequired)
	ErrLessThan30DaysNameChange       = exception.NewAppException(exception.LessThan30DaysSinceLastNameChange)
	ErrLessThan30DaysUsernameChange   = exception.NewAppException(exception.LessThan30DaysSinceLastUsernameChange)
	ErrLessThan30DaysBirthdateChange  = exception.NewAppException(exception.LessThan30DaysSinceLastBirthdateChange)
	ErrHasBlocked                     = exception.NewAppException(exception.HasBlocked)
	ErrHasBeenBlocked                 = exception.NewAppException(exception.HasBeenBlocked)
	ErrFriendNotFound                 = exception.NewAppException(exception.FriendNotFound)
	ErrRequestNotFound                = exception.NewAppException(exception.RequestNotFound)
	ErrCannotMakeSelfRequest          = exception.NewAppException(exception.CanNotMakeSelfRequest)
	ErrSentRequestFailed              = exception.NewAppException(exception.SentAddFriendRequestFailed)
	ErrAddFriendRequestSentLimit      = exception.NewAppException(exception.AddFriendRequestSentLimitReached)
	ErrAddFriendRequestReceivedLimit  = exception.NewAppException(exception.AddFriendRequestReceivedLimitReached)
)

// helper for neo4j nullable properties
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

type FileClient interface {
	DeleteFiles(ctx context.Context, fileIDs []string) error
	GetPresignedURL(ctx context.Context, fileID string) (string, error)
	GetPresignedURLs(ctx context.Context, fileIDs []string) (map[string]string, error)
	GetPresignedUploadURL(ctx context.Context, filename, contentType string) (string, string, error)
}

type UserService struct {
	FileClient FileClient
	// In-memory fallback database for robustness when Neo4j is not running
	fallbackUsers   map[string]*model.User // Key: User ID or Username
	fallbackFriends map[string][]string    // Key: User ID -> List of User IDs
	fallbackBlocks  map[string][]string    // Key: User ID -> List of User IDs
	fallbackReqs    map[string][]string    // Key: User ID -> List of User IDs
	mu              sync.RWMutex
}

func NewUserService() *UserService {
	s := &UserService{
		fallbackUsers:   make(map[string]*model.User),
		fallbackFriends: make(map[string][]string),
		fallbackBlocks:  make(map[string][]string),
		fallbackReqs:    make(map[string][]string),
	}

	return s
}

func (s *UserService) EnsureProfile(ctx context.Context, id, email string) (*model.User, error) {
	username := fmt.Sprintf("user_%s", id[:8])
	givenName := "New"
	familyName := "User"

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MERGE (u:User {id: $id})
				ON CREATE SET u.username = $username, u.email = $email, u.givenName = $givenName, u.familyName = $familyName, 
				              u.createdAt = datetime(), u.bio = "", u.friendCount = 0, u.birthdate = "1998-01-01",
				              u.nextChangeNameDate = datetime(), u.nextChangeBirthdateDate = datetime(), u.nextChangeUsernameDate = datetime()
				RETURN u.id
			`
			params := map[string]interface{}{
				"id":         id,
				"username":   username,
				"email":      email,
				"givenName":  givenName,
				"familyName": familyName,
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
			log.Printf("Successfully synchronized Neo4j Profile node for ID: %s", id)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.fallbackUsers[id]; !ok {
		user := &model.User{
			ID:         id,
			Username:   username,
			Email:      email,
			GivenName:  givenName,
			FamilyName: familyName,
			Birthdate:  parseTime("1998-01-01"),
			CreatedAt:  time.Now(),
		}
		s.fallbackUsers[id] = user
		s.fallbackUsers[username] = user
	}

	return s.fallbackUsers[id], nil
}

func (s *UserService) WithIntegrations(fileClient FileClient) *UserService {
	s.FileClient = fileClient
	return s
}

func (s *UserService) GetUserProfile(ctx context.Context, usernameOrID string, currentUserID string) (*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Check block status if currentUserID is provided
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
				OPTIONAL MATCH (u)-[:FRIEND]-(f)
				OPTIONAL MATCH (u)-[:BLOCK]->(b)
				OPTIONAL MATCH (u)-[:REQUEST]->(r)
				OPTIONAL MATCH (u)<-[:REQUEST]-(rc)
				RETURN u.id, u.username, u.givenName, u.familyName, u.email, u.bio, u.birthdate, 
				       count(DISTINCT f) as friendCount, count(DISTINCT b) as blockCount, 
				       count(DISTINCT r) as sentCount, count(DISTINCT rc) as recvCount,
				       u.nextChangeNameDate, u.nextChangeBirthdateDate, u.nextChangeUsernameDate, u.createdAt,
				       u.profilePictureId
			`
			result, err := tx.Run(ctx, query, map[string]interface{}{"usernameOrID": usernameOrID})
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
				}
				return user, nil
			}
			return nil, ErrUserNotFound
		})

		if err != nil {
			return nil, err
		}
		user := res.(*model.User)
		s.enrichUsersWithPresignedURLs(ctx, []*model.User{user})
		return user, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	user, ok := s.fallbackUsers[usernameOrID]
	if !ok {
		return nil, ErrUserNotFound
	}

	if currentUserID != "" && currentUserID != user.ID {
		// Basic block check in fallback
		for _, bID := range s.fallbackBlocks[currentUserID] {
			if bID == user.ID {
				return nil, ErrHasBlocked
			}
		}
		for _, bID := range s.fallbackBlocks[user.ID] {
			if bID == currentUserID {
				return nil, ErrHasBeenBlocked
			}
		}
	}

	user.FriendCount = len(s.fallbackFriends[user.ID])
	user.BlockCount = len(s.fallbackBlocks[user.ID])
	user.RequestSentCount = len(s.fallbackReqs[user.ID])

	return user, nil
}

func (s *UserService) GetFriends(ctx context.Context, username string, currentUserID string) ([]*model.User, error) {
	// Java checks if current user is blocked by target or vice versa
	if _, err := s.GetUserProfile(ctx, username, currentUserID); err != nil {
		return nil, err
	}

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
			users := res.([]*model.User)
			s.enrichUsersWithPresignedURLs(ctx, users)
			return users, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.fallbackUsers[username]
	if !ok {
		return nil, ErrUserNotFound
	}

	var list []*model.User
	for _, friendID := range s.fallbackFriends[u.ID] {
		if friend, ok := s.fallbackUsers[friendID]; ok {
			list = append(list, friend)
		}
	}
	return list, nil
}

func (s *UserService) GetSuggestedFriends(ctx context.Context, currentUserID string) ([]*model.User, error) {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $id})-[:FRIEND]-(f:User)-[:FRIEND]-(foaf:User)
				WHERE NOT (u)-[:FRIEND]-(foaf) AND NOT (u)-[:BLOCK]-(foaf) AND NOT (u)-[:REQUEST]-(foaf) AND u <> foaf
				RETURN foaf.id, foaf.username, foaf.givenName, foaf.familyName, foaf.email, foaf.bio, count(f) as mutualCount, foaf.profilePictureId
				ORDER BY mutualCount DESC LIMIT 20
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
			users := res.([]*model.User)
			s.enrichUsersWithPresignedURLs(ctx, users)
			return users, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.User
	friends := s.fallbackFriends[currentUserID]
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

	for _, u := range s.fallbackUsers {
		if u.ID != u.Username && !isFriendOrSelf(u.ID) {
			list = append(list, u)
		}
	}
	return list, nil
}

func (s *UserService) GetMutualFriends(ctx context.Context, currentUserID string, targetUsername string) ([]*model.User, error) {
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
			users := res.([]*model.User)
			s.enrichUsersWithPresignedURLs(ctx, users)
			return users, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	target, ok := s.fallbackUsers[targetUsername]
	if !ok {
		return nil, ErrUserNotFound
	}

	myFriends := s.fallbackFriends[currentUserID]
	theirFriends := s.fallbackFriends[target.ID]

	var list []*model.User
	for _, mf := range myFriends {
		for _, tf := range theirFriends {
			if mf == tf {
				if mUser, ok := s.fallbackUsers[mf]; ok {
					list = append(list, mUser)
				}
			}
		}
	}
	return list, nil
}

func (s *UserService) Unfriend(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Check if they are friends
			queryCheck := `MATCH (u1:User {id: $id})-[r:FRIEND]-(u2:User {username: $username}) RETURN r`
			resCheck, _ := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID, "username": targetUsername})
			if !resCheck.Next(ctx) {
				return nil, ErrFriendNotFound
			}

			query := `
				MATCH (u1:User {id: $id})-[r:FRIEND]-(u2:User {username: $username})
				DELETE r
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
		})
		if err != nil {
			return err
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	found := false
	myFriends := s.fallbackFriends[currentUserID]
	for i, f := range myFriends {
		if f == target.ID {
			s.fallbackFriends[currentUserID] = append(myFriends[:i], myFriends[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrFriendNotFound
	}

	theirFriends := s.fallbackFriends[target.ID]
	for i, f := range theirFriends {
		if f == currentUserID {
			s.fallbackFriends[target.ID] = append(theirFriends[:i], theirFriends[i+1:]...)
			break
		}
	}

	return nil
}

func (s *UserService) Block(ctx context.Context, currentUserID string, targetUsername string) error {
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
		if err == nil {
			return nil
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	s.fallbackBlocks[currentUserID] = append(s.fallbackBlocks[currentUserID], target.ID)
	
	s.mu.Unlock()
	_ = s.Unfriend(ctx, currentUserID, targetUsername)
	s.mu.Lock()

	return nil
}

func (s *UserService) Unblock(ctx context.Context, currentUserID string, targetUsername string) error {
	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u1:User {id: $id})-[r:BLOCK]->(u2:User {username: $username})
				DELETE r
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "username": targetUsername})
		})
		if err == nil {
			return nil
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	myBlocks := s.fallbackBlocks[currentUserID]
	for i, bID := range myBlocks {
		if bID == target.ID {
			s.fallbackBlocks[currentUserID] = append(myBlocks[:i], myBlocks[i+1:]...)
			break
		}
	}
	return nil
}

func (s *UserService) GetBlockedUsers(ctx context.Context, currentUserID string) ([]*model.User, error) {
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
			users := res.([]*model.User)
			s.enrichUsersWithPresignedURLs(ctx, users)
			return users, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.User
	for _, bID := range s.fallbackBlocks[currentUserID] {
		if b, ok := s.fallbackUsers[bID]; ok {
			list = append(list, b)
		}
	}
	return list, nil
}

func (s *UserService) SendFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	target, err := s.GetUserProfile(ctx, targetUsername, currentUserID)
	if err != nil {
		return err
	}

	if currentUserID == target.ID {
		return ErrCannotMakeSelfRequest
	}

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Check if already friends or requested
			queryCheck := `
				MATCH (u1:User {id: $id}), (u2:User {id: $targetID})
				OPTIONAL MATCH (u1)-[r:FRIEND]-(u2)
				OPTIONAL MATCH (u1)-[req:REQUEST]-(u2)
				RETURN r IS NOT NULL as isFriend, req IS NOT NULL as isRequested
			`
			resCheck, _ := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID, "targetID": target.ID})
			if resCheck.Next(ctx) {
				rec := resCheck.Record()
				if rec.Values[0].(bool) || rec.Values[1].(bool) {
					return nil, ErrSentRequestFailed
				}
			}

			// Check limits
			if target.RequestReceivedCount+1 > model.MaxReceivedRequestCount {
				return nil, ErrAddFriendRequestReceivedLimit
			}

			query := `
				MATCH (u1:User {id: $id}), (u2:User {id: $targetID})
				MERGE (u1)-[:REQUEST]->(u2)
				RETURN u1.id
			`
			return tx.Run(ctx, query, map[string]interface{}{"id": currentUserID, "targetID": target.ID})
		})
		if err != nil {
			return err
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.fallbackReqs[currentUserID] = append(s.fallbackReqs[currentUserID], target.ID)
	return nil
}

func (s *UserService) AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
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
		if err != nil {
			return err
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	// Find the request (it should be in target's sent requests)
	found := false
	reqs := s.fallbackReqs[target.ID]
	for i, rID := range reqs {
		if rID == currentUserID {
			s.fallbackReqs[target.ID] = append(reqs[:i], reqs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return ErrRequestNotFound
	}

	s.fallbackFriends[currentUserID] = append(s.fallbackFriends[currentUserID], target.ID)
	s.fallbackFriends[target.ID] = append(s.fallbackFriends[target.ID], currentUserID)

	return nil
}

func (s *UserService) DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
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
				// Try the other direction (deleting received request)
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
		if err != nil {
			return err
		}
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	target, ok := s.fallbackUsers[targetUsername]
	if !ok {
		return ErrUserNotFound
	}

	found := false
	reqs := s.fallbackReqs[currentUserID]
	for i, rID := range reqs {
		if rID == target.ID {
			s.fallbackReqs[currentUserID] = append(reqs[:i], reqs[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		// Try other direction
		reqs2 := s.fallbackReqs[target.ID]
		for i, rID := range reqs2 {
			if rID == currentUserID {
				s.fallbackReqs[target.ID] = append(reqs2[:i], reqs2[i+1:]...)
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

func (s *UserService) GetSentRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
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
			users := res.([]*model.User)
			s.enrichUsersWithPresignedURLs(ctx, users)
			return users, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.User
	for _, rID := range s.fallbackReqs[currentUserID] {
		if target, ok := s.fallbackUsers[rID]; ok {
			list = append(list, target)
		}
	}
	return list, nil
}

func (s *UserService) GetReceivedRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
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
			users := res.([]*model.User)
			s.enrichUsersWithPresignedURLs(ctx, users)
			return users, nil
		}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var list []*model.User
	for senderID, reqs := range s.fallbackReqs {
		for _, targetID := range reqs {
			if targetID == currentUserID {
				if sender, ok := s.fallbackUsers[senderID]; ok {
					list = append(list, sender)
				}
			}
		}
	}
	return list, nil
}

func (s *UserService) clearCache(ctx context.Context, userID string) {
	if red.RedisClient == nil {
		return
	}

	// Key used by auth-service for token generation (username cache)
	authKey := fmt.Sprintf("user_info:%s", userID)
	// Key used by post-service for profile enrichment
	postKey := fmt.Sprintf("user:profile:%s", userID)

	_ = red.RedisClient.Del(ctx, authKey, postKey).Err()
}

func (s *UserService) UpdateBio(ctx context.Context, currentUserID string, bio string) error {
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
		if err == nil {
			s.clearCache(ctx, currentUserID)
			return nil
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if u, ok := s.fallbackUsers[currentUserID]; ok {
		u.Bio = bio
		s.clearCache(ctx, currentUserID)
	}
	return nil
}

func (s *UserService) UpdateBirthdate(ctx context.Context, currentUserID string, birthdateStr string) error {
	birthdate := parseTime(birthdateStr)
	if !validation.IsValidAge(birthdate, model.MinAge) {
		return ErrInvalidAge
	}

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Check cooldown
			queryCheck := `MATCH (u:User {id: $id}) RETURN u.nextChangeBirthdateDate`
			resCheck, err := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID})
			if err == nil && resCheck.Next(ctx) {
				nextDate := getTimeVal(resCheck.Record().Values[0])
				if time.Now().Before(nextDate) {
					return nil, ErrLessThan30DaysBirthdateChange
				}
			}

			query := `
				MATCH (u:User {id: $id})
				SET u.birthdate = $birthdate, u.nextChangeBirthdateDate = $nextDate
				RETURN u.id
			`
			nextDate := time.Now().AddDate(0, 0, model.ChangeBirthdateCooldownDay)
			return tx.Run(ctx, query, map[string]interface{}{
				"id":        currentUserID,
				"birthdate": birthdateStr,
				"nextDate":  nextDate.Format(time.RFC3339),
			})
		})
		if err != nil {
			return err
		}
		s.clearCache(ctx, currentUserID)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.fallbackUsers[currentUserID]; ok {
		if time.Now().Before(u.NextChangeBirthdateDate) {
			return ErrLessThan30DaysBirthdateChange
		}
		u.Birthdate = birthdate
		u.NextChangeBirthdateDate = time.Now().AddDate(0, 0, model.ChangeBirthdateCooldownDay)
		s.clearCache(ctx, currentUserID)
	}
	return nil
}

func (s *UserService) UpdateName(ctx context.Context, currentUserID string, familyName, givenName string) error {
	if !validation.IsOnlyLettersAndSpaces(familyName) || !validation.IsOnlyLettersAndSpaces(givenName) {
		return ErrInvalidName
	}

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Check cooldown
			queryCheck := `MATCH (u:User {id: $id}) RETURN u.nextChangeNameDate`
			resCheck, err := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID})
			if err == nil && resCheck.Next(ctx) {
				nextDate := getTimeVal(resCheck.Record().Values[0])
				if time.Now().Before(nextDate) {
					return nil, ErrLessThan30DaysNameChange
				}
			}

			query := `
				MATCH (u:User {id: $id})
				SET u.familyName = $familyName, u.givenName = $givenName, u.nextChangeNameDate = $nextDate
				RETURN u.id
			`
			nextDate := time.Now().AddDate(0, 0, model.ChangeNameCooldownDay)
			return tx.Run(ctx, query, map[string]interface{}{
				"id":         currentUserID,
				"familyName": familyName,
				"givenName":  givenName,
				"nextDate":   nextDate.Format(time.RFC3339),
			})
		})
		if err != nil {
			return err
		}
		s.clearCache(ctx, currentUserID)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.fallbackUsers[currentUserID]; ok {
		if time.Now().Before(u.NextChangeNameDate) {
			return ErrLessThan30DaysNameChange
		}
		u.FamilyName = familyName
		u.GivenName = givenName
		u.NextChangeNameDate = time.Now().AddDate(0, 0, model.ChangeNameCooldownDay)
		s.clearCache(ctx, currentUserID)
	}
	return nil
}

func (s *UserService) UpdateUsername(ctx context.Context, currentUserID string, username string) error {
	if !validation.IsValidUsername(username) {
		return ErrInvalidUsername
	}

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Check if exists
			queryExist := `MATCH (u:User {username: $username}) RETURN u.id`
			resExist, _ := tx.Run(ctx, queryExist, map[string]interface{}{"username": username})
			if resExist.Next(ctx) {
				return nil, ErrUsernameAlreadyExists
			}

			// Check cooldown
			queryCheck := `MATCH (u:User {id: $id}) RETURN u.nextChangeUsernameDate`
			resCheck, err := tx.Run(ctx, queryCheck, map[string]interface{}{"id": currentUserID})
			if err == nil && resCheck.Next(ctx) {
				nextDate := getTimeVal(resCheck.Record().Values[0])
				if time.Now().Before(nextDate) {
					return nil, ErrLessThan30DaysUsernameChange
				}
			}

			query := `
				MATCH (u:User {id: $id})
				SET u.username = $username, u.nextChangeUsernameDate = $nextDate
				RETURN u.id
			`
			nextDate := time.Now().AddDate(0, 0, model.ChangeUsernameCooldownDay)
			return tx.Run(ctx, query, map[string]interface{}{
				"id":       currentUserID,
				"username": username,
				"nextDate": nextDate.Format(time.RFC3339),
			})
		})
		if err != nil {
			return err
		}
		s.clearCache(ctx, currentUserID)
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.fallbackUsers[currentUserID]; ok {
		if _, exists := s.fallbackUsers[username]; exists {
			return ErrUsernameAlreadyExists
		}
		if time.Now().Before(u.NextChangeUsernameDate) {
			return ErrLessThan30DaysUsernameChange
		}
		delete(s.fallbackUsers, u.Username)
		u.Username = username
		u.NextChangeUsernameDate = time.Now().AddDate(0, 0, model.ChangeUsernameCooldownDay)
		s.fallbackUsers[username] = u
		s.clearCache(ctx, currentUserID)
	}
	return nil
}

func (s *UserService) UpdateProfilePicture(ctx context.Context, currentUserID string, fileID string) (string, error) {
	if fileID == "" {
		return "", ErrProfilePictureRequired
	}

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
		if err != nil {
			return "", err
		}
		s.clearCache(ctx, currentUserID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.fallbackUsers[currentUserID]; ok {
		u.ProfilePictureId = fileID
		s.clearCache(ctx, currentUserID)
	}
	return fileID, nil
}

func (s *UserService) enrichUsersWithPresignedURLs(ctx context.Context, users []*model.User) {
	if s.FileClient == nil || len(users) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)

	for _, u := range users {
		if u.ProfilePictureId != "" && !fileIDSet[u.ProfilePictureId] {
			fileIDs = append(fileIDs, u.ProfilePictureId)
			fileIDSet[u.ProfilePictureId] = true
		}
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for users: %v", err)
		return
	}

	for _, u := range users {
		if url, ok := urls[u.ProfilePictureId]; ok {
			u.ProfilePictureId = url
		}
	}
}
