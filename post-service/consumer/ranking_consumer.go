package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
	"social-network-go/post-service/repository"
	"social-network-go/post-service/service"
)

const (
	DefaultRankingConsumerGroupID = "post-ranking-view-consumer"
	DefaultBatchSize              = 200
	DefaultFlushInterval          = 2 * time.Second
	DefaultRankingRetention       = 48 * time.Hour
	DefaultMaxBufferCapacity      = 500
	MaxMissingPostRetries         = 3
	DefaultDLQTopic               = "post-view-events-dlq"
)

// KafkaReader abstracts Kafka message reading and committing to facilitate unit testing.
type KafkaReader interface {
	FetchMessage(ctx context.Context) (kafka.Message, error)
	CommitMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

// KafkaWriter abstracts publishing messages (e.g. to DLQ topic) with broker acknowledgement.
type KafkaWriter interface {
	WriteMessages(ctx context.Context, msgs ...kafka.Message) error
	Close() error
}

type messageEntry struct {
	msg        kafka.Message
	parsed     *service.QualifiedViewEventMessage
	isResolved bool
	isInvalid  bool
	invalidErr string
	retryCount int
}

type RankingConsumer struct {
	reader            KafkaReader
	dlqWriter         KafkaWriter
	repo              repository.PostRepository
	batchSize         int
	flushInterval     time.Duration
	retention         time.Duration
	maxBufferCapacity int

	mu          sync.Mutex
	flushMu     sync.Mutex // Serializes batch executions to guarantee sequential Neo4j writes & commits
	pendingMsgs []*messageEntry
}

func NewRankingConsumer(kafkaAddr, topic, groupID string, repo repository.PostRepository) *RankingConsumer {
	if topic == "" {
		topic = "post-view-events"
	}
	if groupID == "" {
		groupID = DefaultRankingConsumerGroupID
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaAddr},
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: 0,    // Explicit synchronous commit only
	})

	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(kafkaAddr),
		Topic:        DefaultDLQTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Async:        false, // Explicit synchronous wait for broker ACK
	}

	return &RankingConsumer{
		reader:            reader,
		dlqWriter:         dlqWriter,
		repo:              repo,
		batchSize:         DefaultBatchSize,
		flushInterval:     DefaultFlushInterval,
		retention:         DefaultRankingRetention,
		maxBufferCapacity: DefaultMaxBufferCapacity,
		pendingMsgs:       make([]*messageEntry, 0, DefaultMaxBufferCapacity),
	}
}

// NewRankingConsumerWithReader creates a consumer with a custom reader (for testing or advanced setups)
func NewRankingConsumerWithReader(reader KafkaReader, repo repository.PostRepository) *RankingConsumer {
	return &RankingConsumer{
		reader:            reader,
		repo:              repo,
		batchSize:         DefaultBatchSize,
		flushInterval:     DefaultFlushInterval,
		retention:         DefaultRankingRetention,
		maxBufferCapacity: DefaultMaxBufferCapacity,
		pendingMsgs:       make([]*messageEntry, 0, DefaultMaxBufferCapacity),
	}
}

func (c *RankingConsumer) SetDLQWriter(writer KafkaWriter) {
	c.dlqWriter = writer
}

func ValidateViewEvent(event *service.QualifiedViewEventMessage) error {
	if event == nil {
		return errors.New("event payload is nil")
	}
	if event.UserID == "" {
		return errors.New("missing userId in view event")
	}
	if event.PostID == "" {
		return errors.New("missing postId in view event")
	}
	if event.SchemaVersion != 1 {
		return errors.New("unsupported schemaVersion in view event")
	}
	if event.ObservedAt.IsZero() {
		return errors.New("missing or zero observedAt in view event")
	}
	return nil
}

func (c *RankingConsumer) SetOptions(batchSize int, flushInterval, retention time.Duration, maxBufferCapacity int) {
	c.batchSize = batchSize
	c.flushInterval = flushInterval
	c.retention = retention
	if maxBufferCapacity > 0 {
		c.maxBufferCapacity = maxBufferCapacity
	}
}

// AggregateViewItems deduplicates raw items by (userId, postId), retaining the newest observedAt.
func AggregateViewItems(rawItems []repository.ViewRecordItem, now time.Time, retention time.Duration) []repository.ViewRecordItem {
	if len(rawItems) == 0 {
		return nil
	}

	cutoff := now.Add(-retention)
	merged := make(map[string]repository.ViewRecordItem, len(rawItems))

	for _, item := range rawItems {
		if item.UserID == "" || item.PostID == "" {
			continue
		}
		if item.ObservedAt.Before(cutoff) {
			continue
		}

		key := item.UserID + "::" + item.PostID
		existing, found := merged[key]
		if !found || item.ObservedAt.After(existing.ObservedAt) {
			merged[key] = item
		}
	}

	result := make([]repository.ViewRecordItem, 0, len(merged))
	for _, item := range merged {
		result = append(result, item)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].UserID != result[j].UserID {
			return result[i].UserID < result[j].UserID
		}
		return result[i].PostID < result[j].PostID
	})

	return result
}

// Flush executes the batch Neo4j write and strictly contiguous Kafka offset commit.
func (c *RankingConsumer) Flush(ctx context.Context) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	c.mu.Lock()
	if len(c.pendingMsgs) == 0 {
		c.mu.Unlock()
		return nil
	}

	// Snapshot active entries to process
	activeEntries := make([]*messageEntry, len(c.pendingMsgs))
	copy(activeEntries, c.pendingMsgs)
	c.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-c.retention)

	// 1. Process invalid/malformed payloads through DLQ
	for _, entry := range activeEntries {
		if entry.isResolved {
			continue
		}
		if entry.isInvalid || entry.parsed == nil {
			reason := entry.invalidErr
			if reason == "" {
				reason = "malformed_or_invalid_payload"
			}
			if err := c.publishToDLQ(ctx, entry, reason); err != nil {
				return err // Keep offset pending; Flush returns error to trigger backoff
			}
			entry.isResolved = true
		}
	}

	// 2. Filter expired events vs valid events for Neo4j update
	rawItems := make([]repository.ViewRecordItem, 0, len(activeEntries))
	for _, entry := range activeEntries {
		if entry.isResolved {
			continue
		}
		if entry.parsed.ObservedAt.Before(cutoff) {
			// Older than retention window: drop from graph update but mark resolved
			// (not an invalid payload, just legitimately outside the active ranking window)
			entry.isResolved = true
			continue
		}
		rawItems = append(rawItems, repository.ViewRecordItem{
			UserID:     entry.parsed.UserID,
			PostID:     entry.parsed.PostID,
			ObservedAt: entry.parsed.ObservedAt,
		})
	}

	aggregated := AggregateViewItems(rawItems, now, c.retention)

	if len(aggregated) > 0 && c.repo != nil {
		results, err := c.repo.RecordPostsViewed(ctx, aggregated)
		if err != nil {
			logger.WithContext(ctx).Err(err).Error("failed to record viewed posts in Neo4j, will retry with backoff")
			return err
		}

		// Map results back to entries
		statusMap := make(map[string]repository.ViewItemStatus, len(results))
		for _, res := range results {
			statusMap[res.UserID+"::"+res.PostID] = res.Status
		}

		for _, entry := range activeEntries {
			if entry.isResolved || entry.parsed == nil {
				continue
			}
			key := entry.parsed.UserID + "::" + entry.parsed.PostID
			status, ok := statusMap[key]
			if !ok {
				continue
			}

			switch status {
			case repository.ViewStatusSuccess, repository.ViewStatusPostDeleted, repository.ViewStatusMissingUser:
				entry.isResolved = true
			case repository.ViewStatusMissingPost:
				entry.retryCount++
				if entry.retryCount >= MaxMissingPostRetries {
					logger.WithContext(ctx).Warn("post %s missing in graph after %d retries; routing to DLQ", entry.parsed.PostID, entry.retryCount)
					if err := c.publishToDLQ(ctx, entry, "missing_post_max_retries"); err != nil {
						// DLQ publish failed: keep uncommitted and return error for backoff
						entry.isResolved = false
						return err
					}
					entry.isResolved = true
				} else {
					// Keep uncommitted to allow graph sync to catch up
					entry.isResolved = false
				}
			default:
				entry.isResolved = true
			}
		}
	}

	// Calculate contiguous resolved prefix per partition
	// We can only commit messages strictly in order per partition.
	msgsToCommit := make([]kafka.Message, 0)
	remainingEntries := make([]*messageEntry, 0, len(activeEntries))

	// Group entries by partition
	partitionMap := make(map[int][]*messageEntry)
	for _, entry := range activeEntries {
		partitionMap[entry.msg.Partition] = append(partitionMap[entry.msg.Partition], entry)
	}

	committedSet := make(map[*messageEntry]bool)

	for _, partitionEntries := range partitionMap {
		// Sort entries in partition by offset
		sort.Slice(partitionEntries, func(i, j int) bool {
			return partitionEntries[i].msg.Offset < partitionEntries[j].msg.Offset
		})

		for _, entry := range partitionEntries {
			if !entry.isResolved {
				// Stop at the first unresolved message in this partition.
				// Offsets cannot be committed past this point.
				break
			}
			msgsToCommit = append(msgsToCommit, entry.msg)
			committedSet[entry] = true
		}
	}

	// Commit contiguous resolved offsets to Kafka
	if c.reader != nil && len(msgsToCommit) > 0 {
		if err := c.reader.CommitMessages(ctx, msgsToCommit...); err != nil {
			logger.WithContext(ctx).Err(err).Error("failed to commit continuous offsets to Kafka; will replay on next restart")
			return err
		}
	}

	// Retain only uncommitted entries in pending buffer
	c.mu.Lock()
	for _, entry := range c.pendingMsgs {
		if !committedSet[entry] {
			remainingEntries = append(remainingEntries, entry)
		}
	}
	c.pendingMsgs = remainingEntries
	c.mu.Unlock()

	return nil
}

func (c *RankingConsumer) publishToDLQ(ctx context.Context, entry *messageEntry, reason string) error {
	if c.dlqWriter == nil {
		logger.WithContext(ctx).Warn("no DLQ writer configured; dropping message for reason: %s", reason)
		return nil
	}

	dlqMsg := kafka.Message{
		Key:   entry.msg.Key,
		Value: entry.msg.Value,
		Headers: []kafka.Header{
			{Key: "dlq-reason", Value: []byte(reason)},
			{Key: "original-topic", Value: []byte(entry.msg.Topic)},
			{Key: "original-partition", Value: []byte(fmt.Sprintf("%d", entry.msg.Partition))},
			{Key: "original-offset", Value: []byte(fmt.Sprintf("%d", entry.msg.Offset))},
			{Key: "dlq-timestamp", Value: []byte(time.Now().UTC().Format(time.RFC3339))},
		},
	}

	if err := c.dlqWriter.WriteMessages(ctx, dlqMsg); err != nil {
		logger.WithContext(ctx).Err(err).Error("failed to publish message to DLQ broker (partition %d offset %d reason %s)", entry.msg.Partition, entry.msg.Offset, reason)
		return err
	}

	logger.WithContext(ctx).Info("successfully published message to DLQ (partition %d offset %d reason %s)", entry.msg.Partition, entry.msg.Offset, reason)
	return nil
}

// Start runs the consumer fetch and flush loop.
func (c *RankingConsumer) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	flushErrCh := make(chan error, 1)

	// Background ticker flusher
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.Flush(ctx); err != nil {
					select {
					case flushErrCh <- err:
					default:
					}
				}
			}
		}
	}()

	backoffAttempt := 0

	for {
		select {
		case <-ctx.Done():
			_ = c.Flush(context.Background())
			return ctx.Err()
		case err := <-flushErrCh:
			logger.WithContext(ctx).Err(err).Warn("background flush encountered error; applying backoff")
			backoffAttempt++
			c.applyBackoff(ctx, backoffAttempt)
		default:
		}

		// Enforce hard buffer cap: pause fetching when buffer is full
		c.mu.Lock()
		bufLen := len(c.pendingMsgs)
		c.mu.Unlock()

		if bufLen >= c.maxBufferCapacity {
			logger.WithContext(ctx).Warn("consumer buffer reached hard cap (%d/%d); pausing fetch to flush", bufLen, c.maxBufferCapacity)
			if err := c.Flush(ctx); err != nil {
				backoffAttempt++
				c.applyBackoff(ctx, backoffAttempt)
			} else {
				backoffAttempt = 0
			}
			continue
		}

		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			logger.WithContext(ctx).Err(err).Warn("kafka fetch message error, retrying...")
			time.Sleep(200 * time.Millisecond)
			continue
		}

		var parsedEvent service.QualifiedViewEventMessage
		var parsedPtr *service.QualifiedViewEventMessage
		var isInvalid bool
		var invalidReason string

		if err := json.Unmarshal(msg.Value, &parsedEvent); err != nil {
			isInvalid = true
			invalidReason = "malformed_json: " + err.Error()
			logger.WithContext(ctx).Err(err).Warn("malformed view event payload at partition %d offset %d", msg.Partition, msg.Offset)
		} else if err := ValidateViewEvent(&parsedEvent); err != nil {
			isInvalid = true
			invalidReason = "invalid_schema: " + err.Error()
			logger.WithContext(ctx).Warn("invalid view event schema at partition %d offset %d: %v", msg.Partition, msg.Offset, err)
		} else {
			parsedPtr = &parsedEvent
		}

		entry := &messageEntry{
			msg:        msg,
			parsed:     parsedPtr,
			isResolved: false,
			isInvalid:  isInvalid,
			invalidErr: invalidReason,
			retryCount: 0,
		}

		c.mu.Lock()
		c.pendingMsgs = append(c.pendingMsgs, entry)
		shouldFlush := len(c.pendingMsgs) >= c.batchSize
		c.mu.Unlock()

		if shouldFlush {
			if err := c.Flush(ctx); err != nil {
				backoffAttempt++
				c.applyBackoff(ctx, backoffAttempt)
			} else {
				backoffAttempt = 0
			}
		}
	}
}

func (c *RankingConsumer) applyBackoff(ctx context.Context, attempt int) {
	if attempt > 6 {
		attempt = 6
	}
	base := 200 * time.Millisecond
	sleepDuration := base * time.Duration(1<<attempt)
	jitter := time.Duration(rand.Int63n(int64(sleepDuration / 2)))
	totalSleep := sleepDuration + jitter

	select {
	case <-time.After(totalSleep):
	case <-ctx.Done():
	}
}

func (c *RankingConsumer) Close() error {
	var firstErr error
	if c.reader != nil {
		if err := c.reader.Close(); err != nil {
			firstErr = err
		}
	}
	if c.dlqWriter != nil {
		if err := c.dlqWriter.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// PendingCount returns the number of pending messages in buffer (for tests and metrics)
func (c *RankingConsumer) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pendingMsgs)
}

// EnqueueMessageForTest adds a message directly into the pending queue (for deterministic unit testing)
func (c *RankingConsumer) EnqueueMessageForTest(msg kafka.Message, parsed *service.QualifiedViewEventMessage, isResolved bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var isInvalid bool
	var invalidErr string
	if parsed == nil {
		isInvalid = true
		invalidErr = "nil_or_malformed_payload"
	} else {
		if parsed.SchemaVersion == 0 {
			parsed.SchemaVersion = 1
		}
		if err := ValidateViewEvent(parsed); err != nil {
			isInvalid = true
			invalidErr = err.Error()
		}
	}
	c.pendingMsgs = append(c.pendingMsgs, &messageEntry{
		msg:        msg,
		parsed:     parsed,
		isResolved: isResolved,
		isInvalid:  isInvalid,
		invalidErr: invalidErr,
		retryCount: 0,
	})
}

// EnqueueRawMessageForTest decodes the message directly to test real parsing and validation paths
func (c *RankingConsumer) EnqueueRawMessageForTest(msg kafka.Message) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var parsedEvent service.QualifiedViewEventMessage
	var parsedPtr *service.QualifiedViewEventMessage
	var isInvalid bool
	var invalidReason string

	if err := json.Unmarshal(msg.Value, &parsedEvent); err != nil {
		isInvalid = true
		invalidReason = "malformed_json: " + err.Error()
	} else if err := ValidateViewEvent(&parsedEvent); err != nil {
		isInvalid = true
		invalidReason = "invalid_schema: " + err.Error()
	} else {
		parsedPtr = &parsedEvent
	}

	c.pendingMsgs = append(c.pendingMsgs, &messageEntry{
		msg:        msg,
		parsed:     parsedPtr,
		isResolved: false,
		isInvalid:  isInvalid,
		invalidErr: invalidReason,
		retryCount: 0,
	})
}
