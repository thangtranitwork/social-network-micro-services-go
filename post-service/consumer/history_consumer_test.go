package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/post-service/consumer"
	"social-network-go/post-service/dto"
	"social-network-go/post-service/service"
)

type mockHistoryRepo struct {
	records []dto.ViewHistoryRecord
	writeErr error
}

func (m *mockHistoryRepo) RecordViewEvent(ctx context.Context, item dto.ViewHistoryRecord, userID string, retention time.Duration) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.records = append(m.records, item)
	return nil
}

func (m *mockHistoryRepo) GetViewsByUser(ctx context.Context, userID string, month string, limit int, pageState []byte) ([]dto.ViewHistoryRecord, []byte, error) {
	return m.records, nil, nil
}

func TestHistoryConsumer_SuccessWritesToCassandraAndCommits(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	repo := &mockHistoryRepo{}

	c := consumer.NewHistoryConsumerWithReader(reader, repo)

	now := time.Now().UTC()
	ev1 := &service.QualifiedViewEventMessage{
		EventID:           "e1",
		UserID:            "u1",
		PostID:            "p1",
		ObservedAt:        now,
		VisibleDurationMs: 2500,
		VisibleRatio:      0.8,
		Surface:           "newsfeed",
		SchemaVersion:     1,
	}

	raw1, _ := json.Marshal(ev1)
	c.EnqueueRawMessageForTest(kafka.Message{
		Partition: 0,
		Offset:    10,
		Key:       []byte("u1"),
		Value:     raw1,
	})

	err := c.Flush(ctx)
	if err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}

	if len(repo.records) != 1 {
		t.Fatalf("expected 1 record written to Cassandra repo, got %d", len(repo.records))
	}
	if repo.records[0].EventID != "e1" || repo.records[0].PostID != "p1" {
		t.Fatalf("mismatched record data: %+v", repo.records[0])
	}

	if len(reader.committedMsgs) != 1 {
		t.Fatalf("expected 1 committed msg, got %d", len(reader.committedMsgs))
	}
	if reader.committedMsgs[0].Offset != 10 {
		t.Fatalf("expected committed offset 10, got %d", reader.committedMsgs[0].Offset)
	}
	if c.PendingCount() != 0 {
		t.Fatalf("expected 0 pending messages, got %d", c.PendingCount())
	}
}

func TestHistoryConsumer_WriteFailureHaltsCommit(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	repo := &mockHistoryRepo{
		writeErr: errors.New("cassandra connection timeout"),
	}

	c := consumer.NewHistoryConsumerWithReader(reader, repo)

	ev := &service.QualifiedViewEventMessage{
		EventID:       "e1",
		UserID:        "u1",
		PostID:        "p1",
		ObservedAt:    time.Now().UTC(),
		SchemaVersion: 1,
	}
	raw, _ := json.Marshal(ev)
	c.EnqueueRawMessageForTest(kafka.Message{
		Partition: 0,
		Offset:    20,
		Key:       []byte("u1"),
		Value:     raw,
	})

	err := c.Flush(ctx)
	if err == nil {
		t.Fatal("expected flush error when Cassandra write fails, got nil")
	}

	if len(reader.committedMsgs) != 0 {
		t.Fatalf("expected 0 commits on failure, got %d", len(reader.committedMsgs))
	}
	if c.PendingCount() != 1 {
		t.Fatalf("expected message to remain pending, got %d", c.PendingCount())
	}
}

func TestHistoryConsumer_ExpiredEventsResolvedWithoutCassandraWrite(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	repo := &mockHistoryRepo{}

	c := consumer.NewHistoryConsumerWithReader(reader, repo)

	// Event older than 90 days (e.g. 100 days ago)
	expiredTime := time.Now().UTC().Add(-100 * 24 * time.Hour)
	ev := &service.QualifiedViewEventMessage{
		EventID:       "e-expired",
		UserID:        "u1",
		PostID:        "p1",
		ObservedAt:    expiredTime,
		SchemaVersion: 1,
	}
	raw, _ := json.Marshal(ev)
	c.EnqueueRawMessageForTest(kafka.Message{
		Partition: 0,
		Offset:    30,
		Key:       []byte("u1"),
		Value:     raw,
	})

	err := c.Flush(ctx)
	if err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}

	// Should NOT write expired event to Cassandra
	if len(repo.records) != 0 {
		t.Fatalf("expected 0 writes to Cassandra for expired event, got %d", len(repo.records))
	}

	// But should commit offset to Kafka
	if len(reader.committedMsgs) != 1 {
		t.Fatalf("expected 1 committed msg, got %d", len(reader.committedMsgs))
	}
	if reader.committedMsgs[0].Offset != 30 {
		t.Fatalf("expected committed offset 30, got %d", reader.committedMsgs[0].Offset)
	}
}

func TestHistoryConsumer_InvalidSchemaRoutedToDLQ(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	writer := &mockKafkaWriter{}
	repo := &mockHistoryRepo{}

	c := consumer.NewHistoryConsumerWithReader(reader, repo)
	c.SetDLQWriter(writer)

	// Malformed JSON message
	c.EnqueueRawMessageForTest(kafka.Message{
		Partition: 0,
		Offset:    40,
		Value:     []byte("{invalid-json"),
	})

	err := c.Flush(ctx)
	if err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}

	if len(writer.writtenMsgs) != 1 {
		t.Fatalf("expected 1 DLQ write for malformed payload, got %d", len(writer.writtenMsgs))
	}
	if len(reader.committedMsgs) != 1 {
		t.Fatalf("expected offset to commit after DLQ ACK, got %d", len(reader.committedMsgs))
	}
}
