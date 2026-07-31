package events

const (
	TopicUserEvents = "social.user.events"
	TopicPostEvents = "social.post.events"

	EventTypeUserCreated      = "USER_CREATED"
	EventTypeFriendshipAdded  = "FRIENDSHIP_ADDED"
	EventTypeFriendshipRemoved = "FRIENDSHIP_REMOVED"
	EventTypeUserBlocked      = "USER_BLOCKED"

	EventTypePostCreated = "POST_CREATED"
	EventTypePostLiked   = "POST_LIKED"
	EventTypePostUnliked = "POST_UNLIKED"
)

type GraphUserEvent struct {
	EventType string `json:"eventType"`
	UserID    string `json:"userId"`
	Username  string `json:"username,omitempty"`
	TargetID  string `json:"targetId,omitempty"` // For friend or block
}

type GraphPostEvent struct {
	EventType string `json:"eventType"`
	PostID    string `json:"postId"`
	UserID    string `json:"userId"`
	Privacy   string `json:"privacy,omitempty"`
}
