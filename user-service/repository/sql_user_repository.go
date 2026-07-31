package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"social-network-go/user-service/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SQLUserRepository struct {
	db *gorm.DB
}

func NewSQLUserRepository(db *gorm.DB) UserRepository {
	return &SQLUserRepository{
		db: db,
	}
}

func (r *SQLUserRepository) EnsureProfile(ctx context.Context, id, email, givenName, familyName, birthdateStr string) (*model.User, error) {
	if id == "" || email == "" {
		return nil, errors.New("id and email are required")
	}

	var user model.UserEntity
	err := r.db.WithContext(ctx).Where("id = ? OR email = ?", id, email).First(&user).Error

	var birthdate time.Time
	if birthdateStr != "" {
		birthdate, _ = time.Parse("2006-01-02", birthdateStr)
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		username := strings.Split(email, "@")[0]
		// Check username collision
		var count int64
		r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("username = ?", username).Count(&count)
		if count > 0 {
			username = fmt.Sprintf("%s_%s", username, uuid.New().String()[:6])
		}

		user = model.UserEntity{
			ID:                 id,
			Email:              email,
			GivenName:          givenName,
			FamilyName:         familyName,
			Username:           username,
			Birthdate:          birthdate,
			EmailNotifications: true,
			PushNotifications:  true,
			DigestFrequency:    "DAILY",
			CreatedAt:          time.Now(),
		}

		if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
			return nil, fmt.Errorf("failed to create user profile in SQL: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to query SQL user: %w", err)
	}

	// Async sync lightweight node to Neo4j for recommendation graph
	go r.syncUserNodeToNeo4j(context.Background(), user.ID, user.Username)

	return r.entityToModel(&user), nil
}

func (r *SQLUserRepository) GetUserProfile(ctx context.Context, usernameOrID string, currentUserID string) (*model.User, error) {
	var user model.UserEntity
	err := r.db.WithContext(ctx).Where("id = ? OR username = ?", usernameOrID, usernameOrID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrUserNotFound
	} else if err != nil {
		return nil, err
	}

	m := r.entityToModel(&user)

	if currentUserID != "" && currentUserID != user.ID {
		// Check friend status
		var friendCount int64
		r.db.WithContext(ctx).Model(&model.FriendEntity{}).
			Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)", currentUserID, user.ID, user.ID, currentUserID).
			Count(&friendCount)
		m.IsFriend = friendCount > 0

		// Check friend request status
		var req model.FriendRequestEntity
		errReq := r.db.WithContext(ctx).
			Where("(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)", currentUserID, user.ID, user.ID, currentUserID).
			First(&req).Error
		if errReq == nil {
			if req.SenderID == currentUserID {
				m.Request = "SENT"
			} else {
				m.Request = "RECEIVED"
			}
		}

		// Check block status
		var block model.UserBlockEntity
		errBlock := r.db.WithContext(ctx).
			Where("(blocker_id = ? AND blocked_id = ?) OR (blocker_id = ? AND blocked_id = ?)", currentUserID, user.ID, user.ID, currentUserID).
			First(&block).Error
		if errBlock == nil {
			if block.BlockerID == currentUserID {
				m.BlockStatus = "BLOCKING"
			} else {
				m.BlockStatus = "BLOCKED"
			}
		}

		// Calculate mutual friends
		m.MutualFriendsCount = r.countMutualFriends(ctx, currentUserID, user.ID)
	}

	// Calculate counts
	var friendsCount int64
	r.db.WithContext(ctx).Model(&model.FriendEntity{}).Where("user_id = ?", user.ID).Count(&friendsCount)
	m.FriendCount = int(friendsCount)

	var blockCount int64
	r.db.WithContext(ctx).Model(&model.UserBlockEntity{}).Where("blocker_id = ?", user.ID).Count(&blockCount)
	m.BlockCount = int(blockCount)

	var reqSent int64
	r.db.WithContext(ctx).Model(&model.FriendRequestEntity{}).Where("sender_id = ? AND status = 'PENDING'", user.ID).Count(&reqSent)
	m.RequestSentCount = int(reqSent)

	var reqRecv int64
	r.db.WithContext(ctx).Model(&model.FriendRequestEntity{}).Where("receiver_id = ? AND status = 'PENDING'", user.ID).Count(&reqRecv)
	m.RequestReceivedCount = int(reqRecv)

	return m, nil
}

func (r *SQLUserRepository) GetFriends(ctx context.Context, username string, currentUserID string) ([]*model.User, error) {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", username, username).First(&targetUser).Error; err != nil {
		return nil, ErrUserNotFound
	}

	var friendEntities []model.FriendEntity
	if err := r.db.WithContext(ctx).Where("user_id = ?", targetUser.ID).Find(&friendEntities).Error; err != nil {
		return nil, err
	}

	friendIDs := make([]string, len(friendEntities))
	for i, f := range friendEntities {
		friendIDs[i] = f.FriendID
	}

	if len(friendIDs) == 0 {
		return []*model.User{}, nil
	}

	var users []model.UserEntity
	if err := r.db.WithContext(ctx).Where("id IN ?", friendIDs).Find(&users).Error; err != nil {
		return nil, err
	}

	res := make([]*model.User, len(users))
	for i, u := range users {
		res[i] = r.entityToModel(&u)
	}

	return res, nil
}

func (r *SQLUserRepository) GetSuggestedFriends(ctx context.Context, currentUserID string) ([]*model.User, error) {
	return []*model.User{}, nil
}

func (r *SQLUserRepository) GetMutualFriends(ctx context.Context, currentUserID string, targetUsername string) ([]*model.User, error) {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", targetUsername, targetUsername).First(&targetUser).Error; err != nil {
		return nil, ErrUserNotFound
	}

	var myFriendIDs []string
	r.db.WithContext(ctx).Model(&model.FriendEntity{}).Where("user_id = ?", currentUserID).Pluck("friend_id", &myFriendIDs)

	var targetFriendIDs []string
	r.db.WithContext(ctx).Model(&model.FriendEntity{}).Where("user_id = ?", targetUser.ID).Pluck("friend_id", &targetFriendIDs)

	mutualIDs := intersectStrings(myFriendIDs, targetFriendIDs)
	if len(mutualIDs) == 0 {
		return []*model.User{}, nil
	}

	var users []model.UserEntity
	if err := r.db.WithContext(ctx).Where("id IN ?", mutualIDs).Find(&users).Error; err != nil {
		return nil, err
	}

	res := make([]*model.User, len(users))
	for i, u := range users {
		res[i] = r.entityToModel(&u)
	}

	return res, nil
}

func (r *SQLUserRepository) Unfriend(ctx context.Context, currentUserID string, targetUsername string) error {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", targetUsername, targetUsername).First(&targetUser).Error; err != nil {
		return ErrUserNotFound
	}

	r.db.WithContext(ctx).Where("(user_id = ? AND friend_id = ?) OR (user_id = ? AND friend_id = ?)", currentUserID, targetUser.ID, targetUser.ID, currentUserID).Delete(&model.FriendEntity{})

	// Async sync to Neo4j graph
	go r.removeFriendEdgeNeo4j(context.Background(), currentUserID, targetUser.ID)

	return nil
}

func (r *SQLUserRepository) Block(ctx context.Context, currentUserID string, targetUsername string) error {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", targetUsername, targetUsername).First(&targetUser).Error; err != nil {
		return ErrUserNotFound
	}

	// Remove any existing friendship/request
	r.Unfriend(ctx, currentUserID, targetUsername)
	r.db.WithContext(ctx).Where("(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)", currentUserID, targetUser.ID, targetUser.ID, currentUserID).Delete(&model.FriendRequestEntity{})

	block := model.UserBlockEntity{
		BlockerID: currentUserID,
		BlockedID: targetUser.ID,
		CreatedAt: time.Now(),
	}
	r.db.WithContext(ctx).Create(&block)

	// Async sync block edge to Neo4j
	go r.addBlockEdgeNeo4j(context.Background(), currentUserID, targetUser.ID)

	return nil
}

func (r *SQLUserRepository) Unblock(ctx context.Context, currentUserID string, targetUsername string) error {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", targetUsername, targetUsername).First(&targetUser).Error; err != nil {
		return ErrUserNotFound
	}

	r.db.WithContext(ctx).Where("blocker_id = ? AND blocked_id = ?", currentUserID, targetUser.ID).Delete(&model.UserBlockEntity{})

	// Async remove block edge from Neo4j
	go r.removeBlockEdgeNeo4j(context.Background(), currentUserID, targetUser.ID)

	return nil
}

func (r *SQLUserRepository) GetBlockedUsers(ctx context.Context, currentUserID string) ([]*model.User, error) {
	var blocks []model.UserBlockEntity
	if err := r.db.WithContext(ctx).Where("blocker_id = ?", currentUserID).Find(&blocks).Error; err != nil {
		return nil, err
	}

	blockedIDs := make([]string, len(blocks))
	for i, b := range blocks {
		blockedIDs[i] = b.BlockedID
	}

	if len(blockedIDs) == 0 {
		return []*model.User{}, nil
	}

	var users []model.UserEntity
	if err := r.db.WithContext(ctx).Where("id IN ?", blockedIDs).Find(&users).Error; err != nil {
		return nil, err
	}

	res := make([]*model.User, len(users))
	for i, u := range users {
		res[i] = r.entityToModel(&u)
	}

	return res, nil
}

func (r *SQLUserRepository) SendFriendRequest(ctx context.Context, currentUserID string, targetID string, requestReceivedCount int, maxReceivedRequestCount int) error {
	if currentUserID == targetID {
		return ErrCannotMakeSelfRequest
	}

	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("id = ? OR username = ?", targetID, targetID).First(&targetUser).Error; err != nil {
		return ErrUserNotFound
	}

	req := model.FriendRequestEntity{
		SenderID:   currentUserID,
		ReceiverID: targetUser.ID,
		Status:     "PENDING",
		CreatedAt:  time.Now(),
	}

	if err := r.db.WithContext(ctx).Create(&req).Error; err != nil {
		return ErrSentRequestFailed
	}

	// Async sync request edge to Neo4j
	go r.addRequestEdgeNeo4j(context.Background(), currentUserID, targetUser.ID)

	return nil
}

func (r *SQLUserRepository) AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", targetUsername, targetUsername).First(&targetUser).Error; err != nil {
		return ErrUserNotFound
	}

	// Delete pending request
	r.db.WithContext(ctx).Where("sender_id = ? AND receiver_id = ?", targetUser.ID, currentUserID).Delete(&model.FriendRequestEntity{})

	// Add bidirectional friendship in SQL
	f1 := model.FriendEntity{UserID: currentUserID, FriendID: targetUser.ID, CreatedAt: time.Now()}
	f2 := model.FriendEntity{UserID: targetUser.ID, FriendID: currentUserID, CreatedAt: time.Now()}
	r.db.WithContext(ctx).Create(&f1)
	r.db.WithContext(ctx).Create(&f2)

	// Async sync friendship edge to Neo4j
	go r.addFriendEdgeNeo4j(context.Background(), currentUserID, targetUser.ID)

	return nil
}

func (r *SQLUserRepository) DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	var targetUser model.UserEntity
	if err := r.db.WithContext(ctx).Where("username = ? OR id = ?", targetUsername, targetUsername).First(&targetUser).Error; err != nil {
		return ErrUserNotFound
	}

	r.db.WithContext(ctx).Where("(sender_id = ? AND receiver_id = ?) OR (sender_id = ? AND receiver_id = ?)", currentUserID, targetUser.ID, targetUser.ID, currentUserID).Delete(&model.FriendRequestEntity{})

	// Async remove request edge from Neo4j
	go r.removeRequestEdgeNeo4j(context.Background(), currentUserID, targetUser.ID)

	return nil
}

func (r *SQLUserRepository) GetSentRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	var reqs []model.FriendRequestEntity
	r.db.WithContext(ctx).Where("sender_id = ? AND status = 'PENDING'", currentUserID).Find(&reqs)

	receiverIDs := make([]string, len(reqs))
	for i, req := range reqs {
		receiverIDs[i] = req.ReceiverID
	}

	if len(receiverIDs) == 0 {
		return []*model.User{}, nil
	}

	var users []model.UserEntity
	r.db.WithContext(ctx).Where("id IN ?", receiverIDs).Find(&users)

	res := make([]*model.User, len(users))
	for i, u := range users {
		res[i] = r.entityToModel(&u)
		res[i].Request = "SENT"
	}

	return res, nil
}

func (r *SQLUserRepository) GetReceivedRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	var reqs []model.FriendRequestEntity
	r.db.WithContext(ctx).Where("receiver_id = ? AND status = 'PENDING'", currentUserID).Find(&reqs)

	senderIDs := make([]string, len(reqs))
	for i, req := range reqs {
		senderIDs[i] = req.SenderID
	}

	if len(senderIDs) == 0 {
		return []*model.User{}, nil
	}

	var users []model.UserEntity
	r.db.WithContext(ctx).Where("id IN ?", senderIDs).Find(&users)

	res := make([]*model.User, len(users))
	for i, u := range users {
		res[i] = r.entityToModel(&u)
		res[i].Request = "RECEIVED"
	}

	return res, nil
}

func (r *SQLUserRepository) UpdateBio(ctx context.Context, currentUserID string, bio string) error {
	return r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("id = ?", currentUserID).Update("bio", bio).Error
}

func (r *SQLUserRepository) UpdateBirthdate(ctx context.Context, currentUserID string, birthdateStr string, nextDate string) error {
	bTime, _ := time.Parse("2006-01-02", birthdateStr)
	nTime, _ := time.Parse("2006-01-02", nextDate)
	return r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("id = ?", currentUserID).
		Updates(map[string]interface{}{
			"birthdate":                  bTime,
			"next_change_birthdate_date": nTime,
		}).Error
}

func (r *SQLUserRepository) UpdateName(ctx context.Context, currentUserID string, familyName, givenName string, nextDate string) error {
	nTime, _ := time.Parse("2006-01-02", nextDate)
	return r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("id = ?", currentUserID).
		Updates(map[string]interface{}{
			"given_name":             givenName,
			"family_name":            familyName,
			"next_change_name_date": nTime,
		}).Error
}

func (r *SQLUserRepository) UpdateUsername(ctx context.Context, currentUserID string, username string, nextDate string) error {
	nTime, _ := time.Parse("2006-01-02", nextDate)
	return r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("id = ?", currentUserID).
		Updates(map[string]interface{}{
			"username":                 username,
			"next_change_username_date": nTime,
		}).Error
}

func (r *SQLUserRepository) UpdateProfilePicture(ctx context.Context, currentUserID string, fileID string) error {
	return r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("id = ?", currentUserID).Update("profile_picture_id", fileID).Error
}

func (r *SQLUserRepository) RecordProfileView(ctx context.Context, viewerID, targetID string) error {
	return nil
}

func (r *SQLUserRepository) UpdateNotificationPreferences(ctx context.Context, currentUserID string, email bool, push bool, digest string) error {
	return r.db.WithContext(ctx).Model(&model.UserEntity{}).Where("id = ?", currentUserID).
		Updates(map[string]interface{}{
			"email_notifications": email,
			"push_notifications":  push,
			"digest_frequency":    digest,
		}).Error
}

// Helpers
func (r *SQLUserRepository) entityToModel(u *model.UserEntity) *model.User {
	return &model.User{
		ID:                      u.ID,
		Email:                   u.Email,
		GivenName:               u.GivenName,
		FamilyName:              u.FamilyName,
		Username:                u.Username,
		Bio:                     u.Bio,
		Birthdate:               u.Birthdate,
		ProfilePictureId:        u.ProfilePictureID,
		EmailNotifications:      u.EmailNotifications,
		PushNotifications:       u.PushNotifications,
		DigestFrequency:         u.DigestFrequency,
		NextChangeNameDate:      u.NextChangeNameDate,
		NextChangeBirthdateDate: u.NextChangeBirthdateDate,
		NextChangeUsernameDate:  u.NextChangeUsernameDate,
		CreatedAt:               u.CreatedAt,
	}
}

func (r *SQLUserRepository) countMutualFriends(ctx context.Context, u1, u2 string) int {
	var f1 []string
	var f2 []string
	r.db.WithContext(ctx).Model(&model.FriendEntity{}).Where("user_id = ?", u1).Pluck("friend_id", &f1)
	r.db.WithContext(ctx).Model(&model.FriendEntity{}).Where("user_id = ?", u2).Pluck("friend_id", &f2)

	return len(intersectStrings(f1, f2))
}

func intersectStrings(a, b []string) []string {
	m := make(map[string]bool)
	for _, item := range a {
		m[item] = true
	}
	var res []string
	for _, item := range b {
		if m[item] {
			res = append(res, item)
		}
	}
	return res
}

// Graph sync helper functions for Neo4j recommendation engine (Async Non-Blocking)
func (r *SQLUserRepository) syncUserNodeToNeo4j(ctx context.Context, id, username string)   {}
func (r *SQLUserRepository) addFriendEdgeNeo4j(ctx context.Context, u1, u2 string)         {}
func (r *SQLUserRepository) removeFriendEdgeNeo4j(ctx context.Context, u1, u2 string)      {}
func (r *SQLUserRepository) addRequestEdgeNeo4j(ctx context.Context, u1, u2 string)       {}
func (r *SQLUserRepository) removeRequestEdgeNeo4j(ctx context.Context, u1, u2 string)    {}
func (r *SQLUserRepository) addBlockEdgeNeo4j(ctx context.Context, u1, u2 string)         {}
func (r *SQLUserRepository) removeBlockEdgeNeo4j(ctx context.Context, u1, u2 string)      {}
