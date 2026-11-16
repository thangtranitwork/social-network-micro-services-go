package consumer

import (
	"context"
	"encoding/json"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
	"social-network-go/post-service/dto"
	"social-network-go/post-service/repository"
	"social-network-go/post-service/service"
)

const (
	DefaultHistoryConsumerGroupID = "post-history-view-consumer"
	DefaultHistoryBatchSize       = 100
	DefaultHistoryFlushInterval   = 1 * time.Second
	DefaultHistoryRetention       = 90 * 24 * time.Hour // 90 days raw history retention
	DefaultHistoryMaxBufferCap    = 500
)

type historyMessageEntry struct {
	msg        kafka.Message
	parsed     *service.QualifiedViewEventMessage
	isResolved bool
	isInvalid  bool
	invalidErr string
}

type HistoryConsumer struct {
	reader            KafkaReader
	dlqWriter         KafkaWriter
	repo              repository.HistoryRepository
	batchSize         int
	flushInterval     time.Duration
	retention         time.Duration
	maxBufferCapacity int

	mu          sync.Mutex
	flushMu     sync.Mutex
	pendingMsgs []*historyMessageEntry
}

func NewHistoryConsumer(kafkaAddr, topic, groupID string, repo repository.HistoryRepository) *HistoryConsumer {
	if topic == "" {
		topic = "post-view-events"
	}
	if groupID == "" {
		groupID = DefaultHistoryConsumerGroupID
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaAddr},
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       10e3,
		MaxBytes:       10e6,
		CommitInterval: 0, // Explicit commit
	})

	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(kafkaAddr),
		Topic:        DefaultDLQTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireAll,
		Async:        false,
	}

	return &HistoryConsumer{
		reader:            reader,
		dlqWriter:         dlqWriter,
		repo:              repo,
		batchSize:         DefaultHistoryBatchSize,
		flushInterval:     DefaultHistoryFlushInterval,
		retention:         DefaultHistoryRetention,
		maxBufferCapacity: DefaultHistoryMaxBufferCap,
		pendingMsgs:       make([]*historyMessageEntry, 0, DefaultHistoryMaxBufferCap),
	}
}

func NewHistoryConsumerWithReader(reader KafkaReader, repo repository.HistoryRepository) *HistoryConsumer {
	return &HistoryConsumer{
		reader:            reader,
		repo:              repo,
		batchSize:         DefaultHistoryBatchSize,
		flushInterval:     DefaultHistoryFlushInterval,
		retention:         DefaultHistoryRetention,
		maxBufferCapacity: DefaultHistoryMaxBufferCap,
		pendingMsgs:       make([]*historyMessageEntry, 0, DefaultHistoryMaxBufferCap),
	}
}

func (c *HistoryConsumer) SetDLQWriter(writer KafkaWriter) {
	c.dlqWriter = writer
}

func (c *HistoryConsumer) SetOptions(batchSize int, flushInterval, retention time.Duration, maxBufferCapacity int) {
	c.batchSize = batchSize
	c.flushInterval = flushInterval
	c.retention = retention
	if maxBufferCapacity > 0 {
		c.maxBufferCapacity = maxBufferCapacity
	}
}

func (c *HistoryConsumer) Flush(ctx context.Context) error {
	c.flushMu.Lock()
	defer c.flushMu.Unlock()

	c.mu.Lock()
	if len(c.pendingMsgs) == 0 {
		c.mu.Unlock()
		return nil
	}

	activeEntries := make([]*historyMessageEntry, len(c.pendingMsgs))
	copy(activeEntries, c.pendingMsgs)
	c.mu.Unlock()

	now := time.Now().UTC()
	cutoff := now.Add(-c.retention)

	for _, entry := range activeEntries {
		if entry.isResolved {
			continue
		}

		if entry.isInvalid || entry.parsed == nil {
			// Malformed or invalid schema: write to DLQ
			if c.dlqWriter != nil {
				reason := entry.invalidErr
				if reason == "" {
					reason = "history_invalid_schema"
				}
				dlqMsg := kafka.Message{
					Key:   entry.msg.Key,
					Value: entry.msg.Value,
					Headers: []kafka.Header{
						{Key: "dlq-reason", Value: []byte(reason)},
						{Key: "consumer-group", Value: []byte(DefaultHistoryConsumerGroupID)},
					},
				}
				if err := c.dlqWriter.WriteMessages(ctx, dlqMsg); err != nil {
					return err // Stop and backoff
				}
			}
			entry.isResolved = true
			continue
		}

		// Expired: older than 90 days retention window
		if entry.parsed.ObservedAt.Before(cutoff) {
			entry.isResolved = true
			continue
		}

		if c.repo != nil {
			rec := dto.ViewHistoryRecord{
				EventID:           entry.parsed.EventID,
				PostID:            entry.parsed.PostID,
				ObservedAt:        entry.parsed.ObservedAt,
				VisibleDurationMs: entry.parsed.VisibleDurationMs,
				VisibleRatio:      entry.parsed.VisibleRatio,
				Surface:           entry.parsed.Surface,
				SchemaVersion:     entry.parsed.SchemaVersion,
			}
			if err := c.repo.RecordViewEvent(ctx, rec, entry.parsed.UserID, c.retention); err != nil {
				logger.WithContext(ctx).Err(err).Error("failed to write view event to Cassandra history")
				return err
			}
		}
		entry.isResolved = true
	}

	// Calculate contiguous resolved prefix per partition
	msgsToCommit := make([]kafka.Message, 0)
	partitionMap := make(map[int][]*historyMessageEntry)
	for _, entry := range activeEntries {
		partitionMap[entry.msg.Partition] = append(partitionMap[entry.msg.Partition], entry)
	}

	committedSet := make(map[*historyMessageEntry]bool)
	for _, partitionEntries := range partitionMap {
		sort.Slice(partitionEntries, func(i, j int) bool {
			return partitionEntries[i].msg.Offset < partitionEntries[j].msg.Offset
		})

		for _, entry := range partitionEntries {
			if !entry.isResolved {
				break
			}
			msgsToCommit = append(msgsToCommit, entry.msg)
			committedSet[entry] = true
		}
	}

	if c.reader != nil && len(msgsToCommit) > 0 {
		if err := c.reader.CommitMessages(ctx, msgsToCommit...); err != nil {
			logger.WithContext(ctx).Err(err).Error("history consumer failed to commit offsets to Kafka")
			return err
		}
	}

	c.mu.Lock()
	remaining := make([]*historyMessageEntry, 0, len(c.pendingMsgs))
	for _, entry := range c.pendingMsgs {
		if !committedSet[entry] {
			remaining = append(remaining, entry)
		}
	}
	c.pendingMsgs = remaining
	c.mu.Unlock()

	return nil
}

func (c *HistoryConsumer) Start(ctx context.Context) error {
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	flushErrCh := make(chan error, 1)

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
			logger.WithContext(ctx).Err(err).Warn("history consumer background flush error; backing off")
			backoffAttempt++
			c.applyBackoff(ctx, backoffAttempt)
		default:
		}

		c.mu.Lock()
		bufLen := len(c.pendingMsgs)
		c.mu.Unlock()

		if bufLen >= c.maxBufferCapacity {
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
		} else if err := ValidateViewEvent(&parsedEvent); err != nil {
			isInvalid = true
			invalidReason = "invalid_schema: " + err.Error()
		} else {
			parsedPtr = &parsedEvent
		}

		entry := &historyMessageEntry{
			msg:        msg,
			parsed:     parsedPtr,
			isResolved: false,
			isInvalid:  isInvalid,
			invalidErr: invalidReason,
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

func (c *HistoryConsumer) applyBackoff(ctx context.Context, attempt int) {
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

func (c *HistoryConsumer) Close() error {
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

func (c *HistoryConsumer) PendingCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pendingMsgs)
}

func (c *HistoryConsumer) EnqueueRawMessageForTest(msg kafka.Message) {
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

	c.pendingMsgs = append(c.pendingMsgs, &historyMessageEntry{
		msg:        msg,
		parsed:     parsedPtr,
		isResolved: false,
		isInvalid:  isInvalid,
		invalidErr: invalidReason,
	})
}
