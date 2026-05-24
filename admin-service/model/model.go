package model

import "time"

type UserStatisticsResponse struct {
	TotalUsers          int                      `json:"totalUsers"`
	NotVerifiedUsers    int                      `json:"notVerifiedUsers"`
	NewUsersToday       int                      `json:"newUsersToday"`
	NewUsersThisWeek    int                      `json:"newUsersThisWeek"`
	NewUsersThisMonth   int                      `json:"newUsersThisMonth"`
	NewUsersThisYear    int                      `json:"newUsersThisYear"`
	OnlineUsersNow      int                      `json:"onlineUsersNow"`
	OnlineStatistics    []OnlineUserLog          `json:"onlineStatistics"`
	ThisWeekStatistics  map[string]int           `json:"thisWeekStatistics"`
	ThisMonthStatistics  map[string]int           `json:"thisMonthStatistics"`
	ThisYearStatistics  map[string]int           `json:"thisYearStatistics"`
}

type PostStatisticsResponse struct {
	TotalPosts          int            `json:"totalPosts"`
	TotalLikes          int            `json:"totalLikes"`
	TotalComments       int            `json:"totalComments"`
	TotalShares         int            `json:"totalShares"`
	TotalFiles          int            `json:"totalFiles"`
	PublicPostCount     int            `json:"publicPostCount"`
	FriendPostCount     int            `json:"friendPostCount"`
	PrivatePostCount    int            `json:"privatePostCount"`
	DeletedPostCount    int            `json:"deletedPostCount"`
	NewPostsToday       int            `json:"newPostsToday"`
	NewPostsThisWeek    int            `json:"newPostsThisWeek"`
	NewPostsThisMonth   int            `json:"newPostsThisMonth"`
	NewPostsThisYear    int            `json:"newPostsThisYear"`
	ThisWeekStatistics  map[string]int `json:"thisWeekStatistics"`
	ThisMonthStatistics  map[string]int `json:"thisMonthStatistics"`
	ThisYearStatistics  map[string]int `json:"thisYearStatistics"`
}

type OnlineUserLog struct {
	Timestamp   string `json:"timestamp"`
	OnlineCount int    `json:"onlineCount"`
}

type CreatorInfo struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
	IsOnline          bool   `json:"isOnline"`
}

type PostResponse struct {
	ID               string       `json:"id"`
	Content          string       `json:"content"`
	Privacy          string       `json:"privacy"`
	CreatedAt        time.Time    `json:"createdAt"`
	LikeCount        int          `json:"likeCount"`
	CommentCount     int          `json:"commentCount"`
	ShareCount       int          `json:"shareCount"`
	Author           CreatorInfo  `json:"author"`
	User             CreatorInfo  `json:"user"` // Compatibility duplicate
	Files            []string     `json:"files"`
}

type UserDetailResponse struct {
	ID                     string    `json:"id"`
	Username               string    `json:"username"`
	GivenName              string    `json:"givenName"`
	FamilyName             string    `json:"familyName"`
	Email                  string    `json:"email"`
	Bio                    string    `json:"bio"`
	Birthdate              string    `json:"birthdate"`
	RegistrationDate       time.Time `json:"registrationDate"`
	FriendCount            int       `json:"friendCount"`
	PostCount              int       `json:"postCount"`
	MessageCount           int       `json:"messageCount"`
	CommentCount           int       `json:"commentCount"`
	CallCount              int       `json:"callCount"`
	RequestSentCount       int       `json:"requestSentCount"`
	RequestReceivedCount   int       `json:"requestReceivedCount"`
	UploadedFileCount      int       `json:"uploadedFileCount"`
	BlockCount             int       `json:"blockCount"`
	IsOnline               bool      `json:"isOnline"`
	LastOnline             string    `json:"lastOnline"`
	ProfilePictureUrl      string    `json:"profilePictureUrl"`
	Verified               bool      `json:"verified"`
}
