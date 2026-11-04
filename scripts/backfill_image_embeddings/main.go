package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/segmentio/kafka-go"
)

type Checkpoint struct {
	LastProcessedPostID string    `json:"lastProcessedPostId"`
	ProcessedCount      int       `json:"processedCount"`
	UpdatedAt           time.Time `json:"updatedAt"`
}

type BackfillPostEvent struct {
	Event     string    `json:"event"`
	PostID    string    `json:"postId"`
	AuthorID  string    `json:"authorId"`
	Privacy   string    `json:"privacy"`
	Files     []string  `json:"files"`
	Timestamp time.Time `json:"timestamp"`
}

const (
	checkpointFile = "scripts/backfill_checkpoint.json"
	defaultThrottle = 1000 * time.Millisecond // 1 post / sec (60 posts / min)
)

func main() {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	neo4jURI := getEnv("NEO4J_URI", "neo4j://localhost:7687")
	neo4jUser := getEnv("NEO4J_USER", "neo4j")
	neo4jPass := getEnv("NEO4J_PASS", "password")
	kafkaAddr := getEnv("KAFKA_ADDR", "localhost:9092")

	log.Printf("Connecting to Neo4j at %s...", neo4jURI)
	driver, err := neo4j.NewDriverWithContext(neo4jURI, neo4j.BasicAuth(neo4jUser, neo4jPass, ""))
	if err != nil {
		log.Fatalf("Failed to create Neo4j driver: %v", err)
	}
	defer driver.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := driver.VerifyConnectivity(ctx); err != nil {
		cancel()
		log.Fatalf("Neo4j connectivity failed: %v", err)
	}
	cancel()

	log.Printf("Connecting to Kafka at %s (topic: post-events)...", kafkaAddr)
	writer := &kafka.Writer{
		Addr:         kafka.TCP(kafkaAddr),
		Topic:        "post-events",
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 50 * time.Millisecond,
		WriteTimeout: 2 * time.Second,
	}
	defer writer.Close()

	// Load checkpoint
	checkpoint := loadCheckpoint()
	log.Printf("Starting backfill from checkpoint: lastPostID=%q, alreadyProcessed=%d",
		checkpoint.LastProcessedPostID, checkpoint.ProcessedCount)

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	session := driver.NewSession(context.Background(), neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(context.Background())

	query := `
		MATCH (a:User)-[:POSTED]->(p:Post)
		WHERE p.deletedAt IS NULL
		  AND p.files IS NOT NULL
		  AND size(p.files) > 0
		  AND p.privacy IN ['PUBLIC', 'FRIEND']
		RETURN p.id AS postID, a.id AS authorID, p.privacy AS privacy, p.files AS files
		ORDER BY p.createdAt ASC
	`

	res, err := session.Run(context.Background(), query, nil)
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	ticker := time.NewTicker(defaultThrottle)
	defer ticker.Stop()

	skipped := checkpoint.LastProcessedPostID != ""
	processedInThisRun := 0

	for res.Next(context.Background()) {
		select {
		case <-sigChan:
			log.Println("Received termination signal. Saving checkpoint and exiting...")
			saveCheckpoint(checkpoint)
			return
		case <-ticker.C:
		}

		rec := res.Record()
		postID, _ := rec.Get("postID")
		authorID, _ := rec.Get("authorID")
		privacy, _ := rec.Get("privacy")
		filesRaw, _ := rec.Get("files")

		pid := fmt.Sprintf("%v", postID)
		if skipped {
			if pid == checkpoint.LastProcessedPostID {
				skipped = false
			}
			continue
		}

		var files []string
		if fl, ok := filesRaw.([]interface{}); ok {
			for _, f := range fl {
				files = append(files, fmt.Sprintf("%v", f))
			}
		}

		event := BackfillPostEvent{
			Event:     "post_created",
			PostID:    pid,
			AuthorID:  fmt.Sprintf("%v", authorID),
			Privacy:   fmt.Sprintf("%v", privacy),
			Files:     files,
			Timestamp: time.Now(),
		}

		body, err := json.Marshal(event)
		if err != nil {
			log.Printf("Failed to marshal event for post %s: %v", pid, err)
			continue
		}

		writeCtx, writeCancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = writer.WriteMessages(writeCtx, kafka.Message{
			Key:   []byte(pid),
			Value: body,
		})
		writeCancel()

		if err != nil {
			log.Printf("Failed to publish message for post %s: %v", pid, err)
			continue
		}

		checkpoint.LastProcessedPostID = pid
		checkpoint.ProcessedCount++
		processedInThisRun++

		if processedInThisRun%10 == 0 {
			saveCheckpoint(checkpoint)
			log.Printf("Progress: processed %d posts in this run (total %d). Last post: %s",
				processedInThisRun, checkpoint.ProcessedCount, pid)
		}
	}

	saveCheckpoint(checkpoint)
	log.Printf("Backfill complete! Processed %d posts in this run. Total: %d",
		processedInThisRun, checkpoint.ProcessedCount)
}

func loadCheckpoint() Checkpoint {
	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		return Checkpoint{}
	}
	var cp Checkpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return Checkpoint{}
	}
	return cp
}

func saveCheckpoint(cp Checkpoint) {
	cp.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(checkpointFile, data, 0644)
}
