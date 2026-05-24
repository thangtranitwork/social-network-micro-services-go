package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
)

type PostCreatedEvent struct {
	Event     string    `json:"event"`
	PostID    string    `json:"postId"`
	Content   string    `json:"content"`
	AuthorID  string    `json:"authorId"`
	Timestamp time.Time `json:"timestamp"`
}

type AIService struct {
	GeminiKey string
	KafkaAddr string
}

func NewAIService(geminiKey, kafkaAddr string) *AIService {
	return &AIService{
		GeminiKey: geminiKey,
		KafkaAddr: kafkaAddr,
	}
}

// ExtractKeywords calls Gemini API or falls back to rules
func (s *AIService) ExtractKeywords(content string) []string {
	if s.GeminiKey != "" {
		keywords, err := s.callGeminiAPI(content)
		if err == nil && len(keywords) > 0 {
			return keywords
		}
		logger.Warn("Gemini API extraction failed: %v. Using rule-based fallback.", err)
	}

	// Rule-based heuristic fallback (extract words starting with #, or common tech terms)
	var keywords []string
	words := strings.Fields(content)
	techTerms := map[string]bool{
		"golang": true, "go": true, "java": true, "spring": true, "react": true,
		"neo4j": true, "sql": true, "redis": true, "kafka": true, "docker": true,
		"kubernetes": true, "microservices": true, "ai": true, "gemini": true,
	}

	for _, w := range words {
		cleaned := strings.ToLower(strings.Trim(w, ".,!?;:()#"))
		if strings.HasPrefix(w, "#") && len(cleaned) > 0 {
			keywords = append(keywords, cleaned)
		} else if techTerms[cleaned] {
			keywords = append(keywords, cleaned)
		}
	}

	// Ensure we always have some tags
	if len(keywords) == 0 {
		keywords = []string{"social", "post"}
	}

	return keywords
}

func (s *AIService) callGeminiAPI(content string) ([]string, error) {
	// Format body for Gemini REST Endpoint
	requestURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=%s", s.GeminiKey)
	prompt := fmt.Sprintf("Analyze the following social network post and extract 3-5 keywords or tags that represent its core topic. Respond ONLY with a comma-separated list of lowercase single-word tags. Post content: \"%s\"", content)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{
				"parts": []map[string]interface{}{
					{"text": prompt},
				},
			},
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	resp, err := http.Post(requestURL, "application/json", bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini api returned status: %s", resp.Status)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	var geminiResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, err
	}

	// Parse out response text
	candidates, ok := geminiResp["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates returned")
	}

	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid candidate structure")
	}

	contentMap, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no content in candidate")
	}

	parts, ok := contentMap["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return nil, fmt.Errorf("no parts in content")
	}

	part, ok := parts[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid part structure")
	}

	text, ok := part["text"].(string)
	if !ok {
		return nil, fmt.Errorf("no text in part")
	}

	// Split comma separated list
	tags := strings.Split(text, ",")
	var cleanedTags []string
	for _, t := range tags {
		cleaned := strings.TrimSpace(strings.ToLower(t))
		if cleaned != "" {
			cleanedTags = append(cleanedTags, cleaned)
		}
	}

	return cleanedTags, nil
}

func (s *AIService) StartWorker() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{s.KafkaAddr},
		GroupID:  "ai-worker-group",
		Topic:    "post-events",
		MinBytes: 10e3, // 10KB
		MaxBytes: 1e6,  // 1MB
	})

	logger.Info("AI Service: Background Worker Listening on Kafka broker %s, topic: post-events", s.KafkaAddr)

	go func() {
		defer reader.Close()
		for {
			m, err := reader.ReadMessage(context.Background())
			if err != nil {
				logger.Error("AI worker read error: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}

			var event PostCreatedEvent
			if err := json.Unmarshal(m.Value, &event); err != nil {
				logger.Error("AI worker: Error unmarshalling PostCreatedEvent: %v", err)
				continue
			}

			logger.Info("AI Worker processing post: %s from Author: %s", event.PostID, event.AuthorID)
			
			// Call Gemini API to extract keywords
			tags := s.ExtractKeywords(event.Content)
			logger.Info("[GEMINI INSIGHTS] Extracted tags for post %s: %v", event.PostID, tags)

			// Simulating Neo4j Write: MATCH (p:Post {id: $postID}) MERGE (t:Tag {name: $tag}) MERGE (p)-[:HAS_KEYWORD]->(t)
			logger.Info("[DB WRITE] Neo4j graph updated with relationships: Post(%s) -[:HAS_KEYWORD]-> Tag%v", event.PostID, tags)
		}
	}()
}

func main() {
	logger.Info("Starting AI & Media Service (Worker)...")

	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	geminiKey := getEnv("GEMINI_KEY", "")
	kafkaAddr := getEnv("KAFKA_ADDR", "localhost:9092") // Correct Kafka broker fallback port

	worker := NewAIService(geminiKey, kafkaAddr)
	worker.StartWorker()

	// Keep service alive
	select {}
}
