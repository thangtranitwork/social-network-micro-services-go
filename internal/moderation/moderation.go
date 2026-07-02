package moderation

import "time"

const (
	TopicRequested = "content.moderation.requested"
	TopicCompleted = "content.moderation.completed"
	TopicReported  = "content.reported"

	TargetPost    = "POST"
	TargetComment = "COMMENT"

	SourcePostCreated     = "post_created"
	SourcePostUpdated     = "post_updated"
	SourceCommentCreated  = "comment_created"
	SourceCommentUpdated  = "comment_updated"
	SourceUserReport      = "user_report"
	SourceReportThreshold = "report_threshold"

	VerdictSafe        = "safe"
	VerdictNeedsReview = "needs_review"
	VerdictViolation   = "violation"

	CategorySpam       = "SPAM"
	CategoryToxic      = "TOXIC"
	CategoryHarassment = "HARASSMENT"
	CategorySexual     = "SEXUAL"
	CategoryViolence   = "VIOLENCE"
	CategoryScam       = "SCAM"
	CategoryHate       = "HATE"
	CategorySelfHarm   = "SELF_HARM"

	ActionNone         = "none"
	ActionQueue        = "queue"
	ActionAutoHide     = "auto_hide"
	ActionAdminApprove = "admin_approve"
	ActionAdminHide    = "admin_hide"
	ActionAdminDelete  = "admin_delete"
	ActionSuspendUser  = "suspend_author"
)

type RequestEvent struct {
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	AuthorID   string    `json:"authorId"`
	Content    string    `json:"content"`
	MediaIDs   []string  `json:"mediaIds"`
	Source     string    `json:"source"`
	Priority   string    `json:"priority,omitempty"`
	TraceID    string    `json:"traceId,omitempty"`
	RequestID  string    `json:"requestId,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

type CompletedEvent struct {
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	AuthorID   string    `json:"authorId,omitempty"`
	Verdict    string    `json:"verdict"`
	Categories []string  `json:"categories"`
	Confidence float64   `json:"confidence"`
	Reason     string    `json:"reason"`
	Action     string    `json:"action"`
	TraceID    string    `json:"traceId,omitempty"`
	RequestID  string    `json:"requestId,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

type ReportedEvent struct {
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	ReporterID string    `json:"reporterId"`
	Reason     string    `json:"reason"`
	TraceID    string    `json:"traceId,omitempty"`
	RequestID  string    `json:"requestId,omitempty"`
	OccurredAt time.Time `json:"occurredAt"`
}

func IsValidTargetType(targetType string) bool {
	return targetType == TargetPost || targetType == TargetComment
}

func IsValidVerdict(verdict string) bool {
	return verdict == VerdictSafe || verdict == VerdictNeedsReview || verdict == VerdictViolation
}

func NormalizeCategories(categories []string) []string {
	valid := map[string]bool{
		CategorySpam:       true,
		CategoryToxic:      true,
		CategoryHarassment: true,
		CategorySexual:     true,
		CategoryViolence:   true,
		CategoryScam:       true,
		CategoryHate:       true,
		CategorySelfHarm:   true,
	}
	seen := make(map[string]bool, len(categories))
	out := make([]string, 0, len(categories))
	for _, category := range categories {
		if valid[category] && !seen[category] {
			out = append(out, category)
			seen[category] = true
		}
	}
	return out
}
