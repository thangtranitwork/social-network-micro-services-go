package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
)

type PostKeywordEvent struct {
	Event     string    `json:"event"`
	PostID    string    `json:"postId"`
	Content   string    `json:"content"`
	IsUpdate  bool      `json:"isUpdate"`
	AuthorID  string    `json:"authorId,omitempty"`
	TraceID   string    `json:"traceId,omitempty"`
	RequestID string    `json:"requestId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type KafkaKeywordPublisher struct {
	writer      *kafka.Writer
	neo4jDriver neo4j.DriverWithContext
}

func NewKafkaKeywordPublisher(kafkaAddr string, neo4jDriver neo4j.DriverWithContext) *KafkaKeywordPublisher {
	return &KafkaKeywordPublisher{
		neo4jDriver: neo4jDriver,
		writer: &kafka.Writer{
			Addr:         kafka.TCP(kafkaAddr),
			Topic:        "post-events",
			Balancer:     &kafka.LeastBytes{},
			Async:        true,
			BatchSize:    100,
			BatchTimeout: 50 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
		},
	}
}

func (p *KafkaKeywordPublisher) ExtractPostKeywords(ctx context.Context, postID string, content string, isUpdate bool) error {
	traceID, requestID := traceIDsFromContext(ctx)
	eventName := "post_created"
	if isUpdate {
		eventName = "post_updated"
	}
	event := PostKeywordEvent{
		Event:     eventName,
		PostID:    postID,
		Content:   content,
		IsUpdate:  isUpdate,
		TraceID:   traceID,
		RequestID: requestID,
		Timestamp: time.Now(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err = p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:     []byte(postID),
		Value:   payload,
		Headers: kafkaTraceHeaders(traceID, requestID),
	})
	if err != nil {
		logger.WithContext(ctx).Err(err).Error("Failed to publish keyword extraction event")
	}
	return err
}

func (p *KafkaKeywordPublisher) Interact(ctx context.Context, postID string, actorID string, score string) error {
	weight := keywordScoreWeight(score)
	if weight == 0 || postID == "" || actorID == "" {
		return nil
	}
	if p.neo4jDriver == nil {
		return fmt.Errorf("neo4j driver is not initialized")
	}

	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	session := p.neo4jDriver.NewSession(callCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(callCtx)

	_, err := session.ExecuteWrite(callCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (user:User {id: $actorID})
			MATCH (post:Post {id: $postID})-[:HAS_KEYWORDS]->(keyword:Keyword)
			MERGE (user)-[i:INTERACT_WITH]->(keyword)
			ON CREATE SET i.score = $weight, keyword.score = coalesce(keyword.score, 0) + $weight
			ON MATCH SET i.score = coalesce(i.score, 0) + $weight, keyword.score = coalesce(keyword.score, 0) + $weight
		`
		_, err := tx.Run(callCtx, query, map[string]interface{}{
			"actorID": actorID,
			"postID":  postID,
			"weight":  weight,
		})
		return nil, err
	})
	if err != nil {
		logger.WithContext(ctx).Err(err).Error("Failed to record keyword interaction for post %s", postID)
	}
	return err
}

func (p *KafkaKeywordPublisher) PostsLoaded(ctx context.Context, postIDs []string, userID string) error {
	return nil
}

func (p *KafkaKeywordPublisher) Close() error {
	return p.writer.Close()
}

func keywordScoreWeight(score string) int {
	switch score {
	case "GET_SCORE", "LIKE_SCORE":
		return 1
	case "COMMENT_SCORE":
		return 3
	case "SHARE_SCORE":
		return 5
	default:
		return 0
	}
}
