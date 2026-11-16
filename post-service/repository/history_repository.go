package repository

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"
	"social-network-go/post-service/dto"
)

type HistoryRepository interface {
	RecordViewEvent(ctx context.Context, item dto.ViewHistoryRecord, userID string, retention time.Duration) error
	GetViewsByUser(ctx context.Context, userID string, month string, limit int, pageState []byte) ([]dto.ViewHistoryRecord, []byte, error)
}

type CassandraHistoryRepository struct {
	session *gocql.Session
}

func NewCassandraHistoryRepository(session *gocql.Session) *CassandraHistoryRepository {
	return &CassandraHistoryRepository{
		session: session,
	}
}

func (r *CassandraHistoryRepository) RecordViewEvent(ctx context.Context, item dto.ViewHistoryRecord, userID string, retention time.Duration) error {
	if r.session == nil {
		return errors.New("cassandra session is not initialized")
	}
	if userID == "" || item.PostID == "" || item.EventID == "" {
		return errors.New("invalid view event record: missing required keys")
	}

	month := item.ObservedAt.UTC().Format("2006-01")
	now := time.Now().UTC()
	expireAt := item.ObservedAt.Add(retention)
	ttlSeconds := int(expireAt.Sub(now).Seconds())

	// If event is already beyond retention window, skip writing to avoid generating unnecessary tombstones
	if ttlSeconds <= 0 {
		return nil
	}

	query := `
		INSERT INTO views_by_user_month (
			user_id, month, observed_at, event_id, post_id,
			visible_duration_ms, visible_ratio, surface, schema_version
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		USING TTL ?
	`
	return r.session.Query(query,
		userID,
		month,
		item.ObservedAt.UTC(),
		item.EventID,
		item.PostID,
		item.VisibleDurationMs,
		item.VisibleRatio,
		item.Surface,
		item.SchemaVersion,
		ttlSeconds,
	).WithContext(ctx).Exec()
}

func (r *CassandraHistoryRepository) GetViewsByUser(ctx context.Context, userID string, month string, limit int, pageState []byte) ([]dto.ViewHistoryRecord, []byte, error) {
	if r.session == nil {
		return nil, nil, errors.New("cassandra session is not initialized")
	}
	if userID == "" {
		return nil, nil, errors.New("userID is required")
	}
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	cqlQuery := `
		SELECT event_id, post_id, observed_at, visible_duration_ms, visible_ratio, surface, schema_version
		FROM views_by_user_month
		WHERE user_id = ? AND month = ?
		LIMIT ?
	`

	q := r.session.Query(cqlQuery, userID, month, limit).WithContext(ctx).PageSize(limit)
	if len(pageState) > 0 {
		q = q.PageState(pageState)
	}

	iter := q.Iter()
	var records []dto.ViewHistoryRecord

	var (
		eventID           string
		postID            string
		observedAt        time.Time
		visibleDurationMs int
		visibleRatio      float64
		surface           string
		schemaVersion     int
	)

	for iter.Scan(&eventID, &postID, &observedAt, &visibleDurationMs, &visibleRatio, &surface, &schemaVersion) {
		records = append(records, dto.ViewHistoryRecord{
			EventID:           eventID,
			PostID:            postID,
			ObservedAt:        observedAt,
			VisibleDurationMs: visibleDurationMs,
			VisibleRatio:      visibleRatio,
			Surface:           surface,
			SchemaVersion:     schemaVersion,
		})
	}

	if err := iter.Close(); err != nil {
		return nil, nil, err
	}

	nextPageState := iter.PageState()
	return records, nextPageState, nil
}
