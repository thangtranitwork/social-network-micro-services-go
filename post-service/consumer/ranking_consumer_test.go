package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/post-service/consumer"
	"social-network-go/post-service/repository"
	"social-network-go/post-service/service"
)

type mockKafkaReader struct {
	committedMsgs []kafka.Message
	commitErr     error
}

func (m *mockKafkaReader) FetchMessage(ctx context.Context) (kafka.Message, error) {
	<-ctx.Done()
	return kafka.Message{}, ctx.Err()
}

func (m *mockKafkaReader) CommitMessages(ctx context.Context, msgs ...kafka.Message) error {
	if m.commitErr != nil {
		return m.commitErr
	}
	m.committedMsgs = append(m.committedMsgs, msgs...)
	return nil
}

func (m *mockKafkaReader) Close() error {
	return nil
}

type mockKafkaWriter struct {
	writtenMsgs []kafka.Message
	writeErr    error
}

func (m *mockKafkaWriter) WriteMessages(ctx context.Context, msgs ...kafka.Message) error {
	if m.writeErr != nil {
		return m.writeErr
	}
	m.writtenMsgs = append(m.writtenMsgs, msgs...)
	return nil
}

func (m *mockKafkaWriter) Close() error {
	return nil
}

type mockConsumerPostRepo struct {
	repository.PostRepository
	resultsToReturn []repository.ViewRecordResult
	errToReturn     error
	callsCount      int
}

func (m *mockConsumerPostRepo) RecordPostsViewed(ctx context.Context, items []repository.ViewRecordItem) ([]repository.ViewRecordResult, error) {
	m.callsCount++
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	if len(m.resultsToReturn) > 0 {
		return m.resultsToReturn, nil
	}
	// Default to SUCCESS for all items
	results := make([]repository.ViewRecordResult, 0, len(items))
	for _, it := range items {
		results = append(results, repository.ViewRecordResult{
			UserID: it.UserID,
			PostID: it.PostID,
			Status: repository.ViewStatusSuccess,
		})
	}
	return results, nil
}

func makeMessage(partition int, offset int64, event *service.QualifiedViewEventMessage) kafka.Message {
	var val []byte
	if event != nil {
		if event.SchemaVersion == 0 {
			event.SchemaVersion = 1
		}
		val, _ = json.Marshal(event)
	} else {
		val = []byte("{invalid-json-payload")
	}
	key := ""
	if event != nil {
		key = event.UserID
	}
	return kafka.Message{
		Partition: partition,
		Offset:    offset,
		Key:       []byte(key),
		Value:     val,
	}
}

func TestAggregateViewItems_DeduplicationAndRetention(t *testing.T) {
	now := time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)
	retention := 48 * time.Hour

	t.Run("keeps latest timestamp for same pair", func(t *testing.T) {
		tOlder := now.Add(-2 * time.Hour)
		tNewer := now.Add(-1 * time.Hour)

		raw := []repository.ViewRecordItem{
			{UserID: "u1", PostID: "p1", ObservedAt: tOlder},
			{UserID: "u1", PostID: "p1", ObservedAt: tNewer},
		}

		res := consumer.AggregateViewItems(raw, now, retention)
		if len(res) != 1 {
			t.Fatalf("expected 1 aggregated item, got %d", len(res))
		}
		if !res[0].ObservedAt.Equal(tNewer) {
			t.Fatalf("expected newer timestamp %v, got %v", tNewer, res[0].ObservedAt)
		}
	})

	t.Run("filters out events older than retention", func(t *testing.T) {
		tExpired := now.Add(-49 * time.Hour)
		tValid := now.Add(-10 * time.Hour)

		raw := []repository.ViewRecordItem{
			{UserID: "u1", PostID: "p1", ObservedAt: tExpired},
			{UserID: "u2", PostID: "p2", ObservedAt: tValid},
		}

		res := consumer.AggregateViewItems(raw, now, retention)
		if len(res) != 1 {
			t.Fatalf("expected 1 item remaining, got %d", len(res))
		}
		if res[0].UserID != "u2" {
			t.Fatalf("expected u2, got %s", res[0].UserID)
		}
	})

	t.Run("sorts deterministically by user then post", func(t *testing.T) {
		raw := []repository.ViewRecordItem{
			{UserID: "u2", PostID: "p1", ObservedAt: now},
			{UserID: "u1", PostID: "p2", ObservedAt: now},
			{UserID: "u1", PostID: "p1", ObservedAt: now},
		}

		res := consumer.AggregateViewItems(raw, now, retention)
		if len(res) != 3 {
			t.Fatalf("expected 3 items, got %d", len(res))
		}
		if res[0].UserID != "u1" || res[0].PostID != "p1" {
			t.Fatalf("expected u1::p1 first, got %s::%s", res[0].UserID, res[0].PostID)
		}
		if res[1].UserID != "u1" || res[1].PostID != "p2" {
			t.Fatalf("expected u1::p2 second, got %s::%s", res[1].UserID, res[1].PostID)
		}
		if res[2].UserID != "u2" || res[2].PostID != "p1" {
			t.Fatalf("expected u2::p1 third, got %s::%s", res[2].UserID, res[2].PostID)
		}
	})
}

func TestRankingConsumer_ContinuousOffsetStopsAtPendingRetry(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	repo := &mockConsumerPostRepo{
		resultsToReturn: []repository.ViewRecordResult{
			{UserID: "u1", PostID: "p1", Status: repository.ViewStatusSuccess},
			{UserID: "u1", PostID: "p2", Status: repository.ViewStatusMissingPost},
			{UserID: "u1", PostID: "p3", Status: repository.ViewStatusSuccess},
		},
	}

	c := consumer.NewRankingConsumerWithReader(reader, repo)
	now := time.Now().UTC()

	ev1 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p1", ObservedAt: now}
	ev2 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p2", ObservedAt: now}
	ev3 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p3", ObservedAt: now}

	c.EnqueueMessageForTest(makeMessage(0, 10, ev1), ev1, false)
	c.EnqueueMessageForTest(makeMessage(0, 11, ev2), ev2, false)
	c.EnqueueMessageForTest(makeMessage(0, 12, ev3), ev3, false)

	err := c.Flush(ctx)
	if err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}

	// Only offset 10 should be committed because offset 11 is missing post and pending retry!
	if len(reader.committedMsgs) != 1 {
		t.Fatalf("expected exactly 1 committed msg, got %d", len(reader.committedMsgs))
	}
	if reader.committedMsgs[0].Offset != 10 {
		t.Fatalf("expected committed offset 10, got %d", reader.committedMsgs[0].Offset)
	}

	// Remaining buffer should have offsets 11 and 12
	if c.PendingCount() != 2 {
		t.Fatalf("expected 2 pending messages, got %d", c.PendingCount())
	}
}

func TestRankingConsumer_MalformedPayloadContiguousCommit(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	repo := &mockConsumerPostRepo{}

	c := consumer.NewRankingConsumerWithReader(reader, repo)
	now := time.Now().UTC()

	ev0 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p0", ObservedAt: now}
	ev2 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p2", ObservedAt: now}

	// Message 20: valid
	c.EnqueueMessageForTest(makeMessage(0, 20, ev0), ev0, false)
	// Message 21: malformed (parsed == nil, marked resolved)
	c.EnqueueMessageForTest(makeMessage(0, 21, nil), nil, true)
	// Message 22: valid
	c.EnqueueMessageForTest(makeMessage(0, 22, ev2), ev2, false)

	err := c.Flush(ctx)
	if err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}

	// All 3 messages should be committed in sequence
	if len(reader.committedMsgs) != 3 {
		t.Fatalf("expected 3 committed msgs, got %d", len(reader.committedMsgs))
	}
	if reader.committedMsgs[0].Offset != 20 || reader.committedMsgs[1].Offset != 21 || reader.committedMsgs[2].Offset != 22 {
		t.Fatalf("expected offsets 20, 21, 22, got %v", reader.committedMsgs)
	}
	if c.PendingCount() != 0 {
		t.Fatalf("expected 0 pending messages, got %d", c.PendingCount())
	}
}

func TestRankingConsumer_FlushFailureNoCommit(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	repo := &mockConsumerPostRepo{
		errToReturn: errors.New("neo4j deadlocked transaction"),
	}

	c := consumer.NewRankingConsumerWithReader(reader, repo)
	ev := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p1", ObservedAt: time.Now().UTC()}
	c.EnqueueMessageForTest(makeMessage(0, 100, ev), ev, false)

	err := c.Flush(ctx)
	if err == nil {
		t.Fatal("expected error on failed flush, got nil")
	}

	// Offset must NOT be committed on flush failure
	if len(reader.committedMsgs) != 0 {
		t.Fatalf("expected 0 committed msgs, got %d", len(reader.committedMsgs))
	}
	// Buffer must retain the message to retry
	if c.PendingCount() != 1 {
		t.Fatalf("expected 1 pending message remaining, got %d", c.PendingCount())
	}
}

func TestRankingConsumer_MaxMissingPostRetriesDLQ(t *testing.T) {
	ctx := context.Background()

	t.Run("DLQ success unblocks offset and commits", func(t *testing.T) {
		reader := &mockKafkaReader{}
		writer := &mockKafkaWriter{}
		repo := &mockConsumerPostRepo{
			resultsToReturn: []repository.ViewRecordResult{
				{UserID: "u1", PostID: "p-missing", Status: repository.ViewStatusMissingPost},
			},
		}

		c := consumer.NewRankingConsumerWithReader(reader, repo)
		c.SetDLQWriter(writer)

		now := time.Now().UTC()
		ev := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p-missing", ObservedAt: now, SchemaVersion: 1}
		c.EnqueueMessageForTest(makeMessage(0, 50, ev), ev, false)

		// Flush 1 (retry 1): not committed
		_ = c.Flush(ctx)
		if len(reader.committedMsgs) != 0 || len(writer.writtenMsgs) != 0 {
			t.Fatalf("expected 0 commits/DLQ writes on attempt 1")
		}

		// Flush 2 (retry 2): not committed
		_ = c.Flush(ctx)
		if len(reader.committedMsgs) != 0 || len(writer.writtenMsgs) != 0 {
			t.Fatalf("expected 0 commits/DLQ writes on attempt 2")
		}

		// Flush 3 (retry 3): reaches MaxMissingPostRetries (3) -> publishes DLQ with ACK -> committed!
		err := c.Flush(ctx)
		if err != nil {
			t.Fatalf("unexpected flush error on attempt 3: %v", err)
		}

		if len(writer.writtenMsgs) != 1 {
			t.Fatalf("expected 1 DLQ write on max retries, got %d", len(writer.writtenMsgs))
		}
		if len(reader.committedMsgs) != 1 {
			t.Fatalf("expected 1 commit after 3 retries (DLQ unblock), got %d", len(reader.committedMsgs))
		}
		if reader.committedMsgs[0].Offset != 50 {
			t.Fatalf("expected committed offset 50, got %d", reader.committedMsgs[0].Offset)
		}
		if c.PendingCount() != 0 {
			t.Fatalf("expected 0 pending messages, got %d", c.PendingCount())
		}
	})

	t.Run("DLQ failure halts commit and keeps offset pending", func(t *testing.T) {
		reader := &mockKafkaReader{}
		writer := &mockKafkaWriter{
			writeErr: errors.New("kafka dlq broker unavailable"),
		}
		repo := &mockConsumerPostRepo{
			resultsToReturn: []repository.ViewRecordResult{
				{UserID: "u1", PostID: "p-missing", Status: repository.ViewStatusMissingPost},
			},
		}

		c := consumer.NewRankingConsumerWithReader(reader, repo)
		c.SetDLQWriter(writer)

		now := time.Now().UTC()
		ev := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p-missing", ObservedAt: now, SchemaVersion: 1}
		c.EnqueueMessageForTest(makeMessage(0, 60, ev), ev, false)

		// Flush 1 & 2
		_ = c.Flush(ctx)
		_ = c.Flush(ctx)

		// Flush 3: triggers DLQ write which fails
		err := c.Flush(ctx)
		if err == nil {
			t.Fatalf("expected flush error when DLQ broker fails, got nil")
		}

		// Offset MUST NOT be committed because DLQ failed to acknowledge
		if len(reader.committedMsgs) != 0 {
			t.Fatalf("expected 0 commits when DLQ fails, got %d", len(reader.committedMsgs))
		}
		// Message MUST remain pending to retry
		if c.PendingCount() != 1 {
			t.Fatalf("expected 1 pending message remaining in buffer, got %d", c.PendingCount())
		}
	})
}

func TestRankingConsumer_InvalidPayloadDLQBetweenValidEvents(t *testing.T) {
	ctx := context.Background()

	t.Run("invalid payload routed to DLQ allowing contiguous commit of valid events", func(t *testing.T) {
		reader := &mockKafkaReader{}
		writer := &mockKafkaWriter{}
		repo := &mockConsumerPostRepo{}

		c := consumer.NewRankingConsumerWithReader(reader, repo)
		c.SetDLQWriter(writer)

		now := time.Now().UTC()
		evValid1 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p1", ObservedAt: now, SchemaVersion: 1}
		evValid2 := &service.QualifiedViewEventMessage{UserID: "u3", PostID: "p3", ObservedAt: now, SchemaVersion: 1}

		// Event 1 (offset 100): Valid
		c.EnqueueRawMessageForTest(makeMessage(0, 100, evValid1))
		// Event 2 (offset 101): Valid JSON but missing UserID (invalid schema)
		invalidMsg := kafka.Message{
			Partition: 0,
			Offset:    101,
			Key:       []byte(""),
			Value:     []byte(`{"postId":"p2","schemaVersion":1,"observedAt":"` + now.Format(time.RFC3339) + `"}`),
		}
		c.EnqueueRawMessageForTest(invalidMsg)
		// Event 3 (offset 102): Valid
		c.EnqueueRawMessageForTest(makeMessage(0, 102, evValid2))

		err := c.Flush(ctx)
		if err != nil {
			t.Fatalf("unexpected flush error: %v", err)
		}

		// DLQ writer must receive exactly the invalid message at offset 101
		if len(writer.writtenMsgs) != 1 {
			t.Fatalf("expected exactly 1 DLQ write, got %d", len(writer.writtenMsgs))
		}

		// Offsets 100, 101, 102 must be committed strictly contiguously in order!
		if len(reader.committedMsgs) != 3 {
			t.Fatalf("expected 3 committed msgs, got %d", len(reader.committedMsgs))
		}
		if reader.committedMsgs[0].Offset != 100 || reader.committedMsgs[1].Offset != 101 || reader.committedMsgs[2].Offset != 102 {
			t.Fatalf("expected contiguous offsets 100, 101, 102, got %v", reader.committedMsgs)
		}

		if c.PendingCount() != 0 {
			t.Fatalf("expected 0 pending messages, got %d", c.PendingCount())
		}
	})

	t.Run("DLQ failure on invalid payload halts contiguous commit at that offset", func(t *testing.T) {
		reader := &mockKafkaReader{}
		writer := &mockKafkaWriter{
			writeErr: errors.New("dlq broker down"),
		}
		repo := &mockConsumerPostRepo{}

		c := consumer.NewRankingConsumerWithReader(reader, repo)
		c.SetDLQWriter(writer)

		now := time.Now().UTC()
		evValid1 := &service.QualifiedViewEventMessage{UserID: "u1", PostID: "p1", ObservedAt: now, SchemaVersion: 1}
		evValid2 := &service.QualifiedViewEventMessage{UserID: "u3", PostID: "p3", ObservedAt: now, SchemaVersion: 1}

		c.EnqueueRawMessageForTest(makeMessage(0, 200, evValid1))
		invalidMsg := kafka.Message{
			Partition: 0,
			Offset:    201,
			Key:       []byte(""),
			Value:     []byte(`{"postId":"p2","schemaVersion":1}`),
		}
		c.EnqueueRawMessageForTest(invalidMsg)
		c.EnqueueRawMessageForTest(makeMessage(0, 202, evValid2))

		err := c.Flush(ctx)
		if err == nil {
			t.Fatal("expected flush error when DLQ write fails, got nil")
		}

		// Because offset 201 failed DLQ publish, offsets must NOT commit past 201
		// Offset 200 could be committed or remain pending, but 201 and 202 MUST NOT be committed
		for _, msg := range reader.committedMsgs {
			if msg.Offset >= 201 {
				t.Fatalf("offset %d committed prematurely despite DLQ failure!", msg.Offset)
			}
		}
		if c.PendingCount() == 0 {
			t.Fatalf("expected pending messages to remain in buffer after failure")
		}
	})
}

func TestRankingConsumer_DistinguishExpiredVsInvalid(t *testing.T) {
	ctx := context.Background()
	reader := &mockKafkaReader{}
	writer := &mockKafkaWriter{}
	repo := &mockConsumerPostRepo{}

	c := consumer.NewRankingConsumerWithReader(reader, repo)
	c.SetDLQWriter(writer)

	now := time.Now().UTC()
	// Expired event: 72 hours ago (retention is 48h)
	expiredTime := now.Add(-72 * time.Hour)
	evExpired := &service.QualifiedViewEventMessage{
		UserID:        "u-old",
		PostID:        "p-old",
		ObservedAt:    expiredTime,
		SchemaVersion: 1,
	}

	c.EnqueueRawMessageForTest(makeMessage(0, 300, evExpired))

	err := c.Flush(ctx)
	if err != nil {
		t.Fatalf("unexpected flush error: %v", err)
	}

	// Expired event is valid data, so it should NOT be sent to DLQ
	if len(writer.writtenMsgs) != 0 {
		t.Fatalf("expired event should NOT be sent to DLQ, got %d writes", len(writer.writtenMsgs))
	}

	// But it should be safely marked resolved and committed to progress the partition offset
	if len(reader.committedMsgs) != 1 {
		t.Fatalf("expected expired event offset 300 to be committed, got %d", len(reader.committedMsgs))
	}
	if reader.committedMsgs[0].Offset != 300 {
		t.Fatalf("expected committed offset 300, got %d", reader.committedMsgs[0].Offset)
	}
}
