package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/segmentio/kafka-go"
	"social-network-go/internal/moderation"
	"social-network-go/logger"
	"social-network-go/profiler"
)

type PostCreatedEvent struct {
	Event     string    `json:"event"`
	PostID    string    `json:"postId"`
	Content   string    `json:"content"`
	IsUpdate  bool      `json:"isUpdate"`
	AuthorID  string    `json:"authorId"`
	TraceID   string    `json:"traceId,omitempty"`
	RequestID string    `json:"requestId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type AIService struct {
	GeminiKey        string
	GeminiModel      string
	KafkaAddr        string
	Neo4jDriver      neo4j.DriverWithContext
	moderationWriter *kafka.Writer
}

func NewAIService(geminiKey, geminiModel, kafkaAddr string, neo4jDriver neo4j.DriverWithContext) *AIService {
	return &AIService{
		GeminiKey:   geminiKey,
		GeminiModel: geminiModel,
		KafkaAddr:   kafkaAddr,
		Neo4jDriver: neo4jDriver,
		moderationWriter: &kafka.Writer{
			Addr:         kafka.TCP(kafkaAddr),
			Topic:        moderation.TopicCompleted,
			Balancer:     &kafka.LeastBytes{},
			BatchTimeout: 50 * time.Millisecond,
			WriteTimeout: 1 * time.Second,
		},
	}
}

// ExtractKeywords calls Gemini API or falls back to rules
func (s *AIService) ExtractKeywords(ctx context.Context, content string) []string {
	if s.GeminiKey != "" {
		keywords, err := s.callGeminiAPI(ctx, content)
		if err == nil && len(keywords) > 0 {
			return keywords
		}
		logGeminiFallback(ctx, err, "keyword_extraction", "", "")
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

func (s *AIService) callGeminiAPI(ctx context.Context, content string) ([]string, error) {
	// Format body for Gemini REST Endpoint
	requestURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", s.GeminiModel)
	prompt := fmt.Sprintf("Analyze the following social network post and extract 3-5 keywords that represent its core topic. Respond ONLY with a comma-separated list of lowercase single-word keywords. Post content: %q", content)

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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", s.GeminiKey)

	client := &http.Client{
		Timeout: 25 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, geminiStatusError(resp)
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

func initNeo4j(ctx context.Context, uri, user, pass string) neo4j.DriverWithContext {
	driver, err := neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))
	if err != nil {
		logger.Warn("AI service failed to create Neo4j driver at %s: %v", uri, err)
		return nil
	}
	if err := driver.VerifyConnectivity(ctx); err != nil {
		logger.Warn("AI service Neo4j connectivity check failed at %s: %v", uri, err)
		return driver
	}

	if err := ensureKeywordConstraint(ctx, driver); err != nil {
		logger.Warn("AI service failed to ensure Keyword constraint: %v", err)
	}
	logger.Info("AI service connected to Neo4j for Keyword graph writes")
	return driver
}

func ensureKeywordConstraint(ctx context.Context, driver neo4j.DriverWithContext) error {
	if driver == nil {
		return nil
	}
	session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		_, err := tx.Run(ctx, "CREATE CONSTRAINT keyword_text_unique IF NOT EXISTS FOR (k:Keyword) REQUIRE k.text IS UNIQUE", nil)
		return nil, err
	})
	return err
}

func (s *AIService) SavePostKeywords(ctx context.Context, postID string, keywords []string, isUpdate bool) error {
	if s.Neo4jDriver == nil {
		return fmt.Errorf("neo4j driver is not initialized")
	}
	keywords = normalizeKeywords(keywords)
	if postID == "" || len(keywords) == 0 {
		return nil
	}

	session := s.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		params := map[string]interface{}{
			"postID":   postID,
			"keywords": keywords,
		}
		query := `
			MATCH (post:Post {id: $postID})
			WITH post
			UNWIND $keywords AS keyword
			MERGE (k:Keyword {text: keyword})
			ON CREATE SET k.score = 0
			MERGE (post)-[:HAS_KEYWORDS]->(k)
		`
		if isUpdate {
			query = `
				MATCH (post:Post {id: $postID})
				OPTIONAL MATCH (post)-[rel:HAS_KEYWORDS]->(:Keyword)
				DELETE rel

				WITH post
				UNWIND $keywords AS keyword
				MERGE (k:Keyword {text: keyword})
				ON CREATE SET k.score = 0
				MERGE (post)-[:HAS_KEYWORDS]->(k)
			`
		}
		_, err := tx.Run(ctx, query, params)
		return nil, err
	})
	return err
}

func normalizeKeywords(keywords []string) []string {
	seen := make(map[string]bool, len(keywords))
	out := make([]string, 0, len(keywords))
	for _, keyword := range keywords {
		cleaned := strings.ToLower(strings.TrimSpace(keyword))
		cleaned = strings.Trim(cleaned, "[]\"'`.,!?;:()#")
		if cleaned == "" || seen[cleaned] {
			continue
		}
		seen[cleaned] = true
		out = append(out, cleaned)
	}
	return out
}

type ModerationResult struct {
	Verdict    string   `json:"verdict"`
	Categories []string `json:"categories"`
	Confidence float64  `json:"confidence"`
	Reason     string   `json:"reason"`
}

func (s *AIService) ModerateContent(ctx context.Context, event moderation.RequestEvent) ModerationResult {
	if s.GeminiKey != "" {
		result, err := s.callGeminiModeration(ctx, event.Content)
		if err == nil && moderation.IsValidVerdict(result.Verdict) {
			result.Categories = moderation.NormalizeCategories(result.Categories)
			return result
		}
		logGeminiFallback(ctx, err, "moderation", event.TargetType, event.TargetID)
	}
	return ruleBasedModeration(event.Content)
}

func (s *AIService) callGeminiModeration(ctx context.Context, content string) (ModerationResult, error) {
	requestURL := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", s.GeminiModel)
	prompt := fmt.Sprintf(`Classify this social-network content for moderation.
Return only JSON with fields: verdict ("safe", "needs_review", "violation"), categories (array using SPAM, TOXIC, HARASSMENT, SEXUAL, VIOLENCE, SCAM, HATE, SELF_HARM), confidence (0-1), reason (short).
Content: %q`, content)

	payload := map[string]interface{}{
		"contents": []map[string]interface{}{
			{"parts": []map[string]interface{}{{"text": prompt}}},
		},
	}

	payloadBytes, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return ModerationResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-goog-api-key", s.GeminiKey)

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ModerationResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ModerationResult{}, geminiStatusError(resp)
	}

	text, err := parseGeminiText(resp.Body)
	if err != nil {
		return ModerationResult{}, err
	}
	text = strings.TrimSpace(strings.Trim(text, "`"))
	text = strings.TrimPrefix(text, "json")
	text = strings.TrimSpace(text)

	var result ModerationResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return ModerationResult{}, err
	}
	return result, nil
}

type GeminiAPIError struct {
	Status  string                 `json:"status"`
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Reason  string                 `json:"reason,omitempty"`
	Domain  string                 `json:"domain,omitempty"`
	Service string                 `json:"service,omitempty"`
	Raw     map[string]interface{} `json:"raw,omitempty"`
}

func (e *GeminiAPIError) Error() string {
	if e == nil {
		return "gemini api error"
	}
	if e.Reason != "" {
		return fmt.Sprintf("gemini api returned status=%s reason=%s message=%s", e.Status, e.Reason, e.Message)
	}
	return fmt.Sprintf("gemini api returned status=%s message=%s", e.Status, e.Message)
}

func geminiStatusError(resp *http.Response) error {
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	body := strings.TrimSpace(string(bodyBytes))
	if body == "" {
		return &GeminiAPIError{Status: resp.Status, Code: resp.StatusCode, Message: "empty error response"}
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return &GeminiAPIError{
			Status:  resp.Status,
			Code:    resp.StatusCode,
			Message: compactLogText(redactGeminiErrorBody(body)),
		}
	}

	info := &GeminiAPIError{
		Status: resp.Status,
		Code:   resp.StatusCode,
		Raw:    parsed,
	}
	if errObj, ok := parsed["error"].(map[string]interface{}); ok {
		if code, ok := errObj["code"].(float64); ok {
			info.Code = int(code)
		}
		if message, ok := errObj["message"].(string); ok {
			info.Message = message
		}
		if details, ok := errObj["details"].([]interface{}); ok {
			for _, detail := range details {
				detailMap, ok := detail.(map[string]interface{})
				if !ok {
					continue
				}
				if reason, ok := detailMap["reason"].(string); ok && info.Reason == "" {
					info.Reason = reason
				}
				if domain, ok := detailMap["domain"].(string); ok && info.Domain == "" {
					info.Domain = domain
				}
				if metadata, ok := detailMap["metadata"].(map[string]interface{}); ok {
					if service, ok := metadata["service"].(string); ok && info.Service == "" {
						info.Service = service
					}
				}
			}
		}
	}
	if info.Message == "" {
		info.Message = compactLogText(redactGeminiErrorBody(body))
	}
	return info
}

func compactLogText(body string) string {
	body = strings.ReplaceAll(body, "\n", " ")
	body = strings.ReplaceAll(body, "\t", " ")
	for strings.Contains(body, "  ") {
		body = strings.ReplaceAll(body, "  ", " ")
	}
	return body
}

func redactGeminiErrorBody(body string) string {
	if key := os.Getenv("GEMINI_KEY"); key != "" {
		body = strings.ReplaceAll(body, key, "[REDACTED]")
	}
	return body
}

func logGeminiFallback(ctx context.Context, err error, operation, targetType, targetID string) {
	entry := logger.WithContext(ctx).
		Err(err).
		Field("provider", "gemini").
		Field("operation", operation).
		Field("fallback", "rule_based")
	if targetType != "" {
		entry.Field("target_type", targetType)
	}
	if targetID != "" {
		entry.Field("target_id", targetID)
	}

	var geminiErr *GeminiAPIError
	if errors.As(err, &geminiErr) && geminiErr != nil {
		entry.
			Field("gemini_status", geminiErr.Status).
			Field("gemini_code", geminiErr.Code)
		if geminiErr.Reason != "" {
			entry.Field("gemini_reason", geminiErr.Reason)
		}
		if geminiErr.Service != "" {
			entry.Field("gemini_service", geminiErr.Service)
		}
		if geminiErr.Raw != nil {
			entry.JsonField("gemini_error", geminiErr.Raw)
		}
	}
	entry.Warn("Gemini request failed; using rule-based fallback")
}

func parseGeminiText(body io.Reader) (string, error) {
	bodyBytes, _ := io.ReadAll(body)
	var geminiResp map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return "", err
	}
	candidates, ok := geminiResp["candidates"].([]interface{})
	if !ok || len(candidates) == 0 {
		return "", fmt.Errorf("no candidates returned")
	}
	candidate, ok := candidates[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid candidate structure")
	}
	contentMap, ok := candidate["content"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("no content in candidate")
	}
	parts, ok := contentMap["parts"].([]interface{})
	if !ok || len(parts) == 0 {
		return "", fmt.Errorf("no parts in content")
	}
	part, ok := parts[0].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("invalid part structure")
	}
	text, ok := part["text"].(string)
	if !ok {
		return "", fmt.Errorf("no text in part")
	}
	return text, nil
}

func ruleBasedModeration(content string) ModerationResult {
	lower := strings.ToLower(content)
	matches := map[string]int{}
	rules := map[string][]string{
		moderation.CategorySpam:       {"free money", "click here", "buy now", "promo code", "kiếm tiền nhanh"},
		moderation.CategoryToxic:      {"idiot", "stupid", "ngu", "đồ ngu", "câm mồm"},
		moderation.CategoryHarassment: {"kill yourself", "go die", "biến đi", "quấy rối"},
		moderation.CategorySexual:     {"porn", "sex", "nude", "18+"},
		moderation.CategoryViolence:   {"kill", "murder", "đâm", "giết"},
		moderation.CategoryScam:       {"bank account", "otp", "password", "chuyển khoản ngay"},
		moderation.CategoryHate:       {"racist", "nazi", "terrorist"},
		moderation.CategorySelfHarm:   {"suicide", "self harm", "tự tử"},
	}
	for category, keywords := range rules {
		for _, keyword := range keywords {
			if strings.Contains(lower, keyword) {
				matches[category]++
			}
		}
	}
	if len(matches) == 0 {
		return ModerationResult{
			Verdict:    moderation.VerdictSafe,
			Categories: []string{},
			Confidence: 0.95,
			Reason:     "No moderation signals detected",
		}
	}
	categories := make([]string, 0, len(matches))
	total := 0
	for category, count := range matches {
		categories = append(categories, category)
		total += count
	}
	confidence := 0.55 + float64(total)*0.15
	if confidence > 0.95 {
		confidence = 0.95
	}
	verdict := moderation.VerdictNeedsReview
	if confidence >= 0.85 {
		verdict = moderation.VerdictViolation
	}
	return ModerationResult{
		Verdict:    verdict,
		Categories: moderation.NormalizeCategories(categories),
		Confidence: confidence,
		Reason:     "Rule-based moderation matched restricted terms",
	}
}

func moderationAction(result ModerationResult) string {
	if result.Verdict == moderation.VerdictViolation && result.Confidence >= 0.85 {
		return moderation.ActionAutoHide
	}
	if result.Verdict == moderation.VerdictNeedsReview || result.Verdict == moderation.VerdictViolation {
		return moderation.ActionQueue
	}
	return moderation.ActionNone
}

func (s *AIService) publishModerationCompleted(ctx context.Context, request moderation.RequestEvent, result ModerationResult) error {
	event := moderation.CompletedEvent{
		TargetType: request.TargetType,
		TargetID:   request.TargetID,
		AuthorID:   request.AuthorID,
		Verdict:    result.Verdict,
		Categories: result.Categories,
		Confidence: result.Confidence,
		Reason:     result.Reason,
		Action:     moderationAction(result),
		TraceID:    request.TraceID,
		RequestID:  request.RequestID,
		OccurredAt: time.Now(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	return s.moderationWriter.WriteMessages(publishCtx, kafka.Message{
		Key:     []byte(request.TargetID),
		Value:   payload,
		Headers: kafkaTraceHeaders(request.TraceID, request.RequestID),
	})
}

func contextWithTraceIDs(parent context.Context, traceID, requestID string) context.Context {
	if parent == nil {
		parent = context.Background()
	}
	if traceID != "" {
		parent = context.WithValue(parent, "trace_id", traceID)
	}
	if requestID != "" {
		parent = context.WithValue(parent, "request_id", requestID)
	}
	return parent
}

func traceIDsFromKafkaMessage(m kafka.Message, payloadTraceID, payloadRequestID string) (string, string) {
	traceID := payloadTraceID
	requestID := payloadRequestID
	for _, header := range m.Headers {
		switch strings.ToLower(header.Key) {
		case "x-trace-id":
			if len(header.Value) > 0 {
				traceID = string(header.Value)
			}
		case "x-request-id":
			if len(header.Value) > 0 {
				requestID = string(header.Value)
			}
		}
	}
	return traceID, requestID
}

func kafkaTraceHeaders(traceID, requestID string) []kafka.Header {
	headers := make([]kafka.Header, 0, 2)
	if traceID != "" {
		headers = append(headers, kafka.Header{Key: "X-Trace-ID", Value: []byte(traceID)})
	}
	if requestID != "" {
		headers = append(headers, kafka.Header{Key: "X-Request-ID", Value: []byte(requestID)})
	}
	return headers
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
			event.TraceID, event.RequestID = traceIDsFromKafkaMessage(m, event.TraceID, event.RequestID)
			ctx := contextWithTraceIDs(context.Background(), event.TraceID, event.RequestID)

			logger.WithContext(ctx).Info("AI Worker processing post: %s from Author: %s", event.PostID, event.AuthorID)

			// Call Gemini API to extract keywords
			tags, _ := profiler.TrackResult("ai-service:worker keyword.extract", func() ([]string, error) {
				return s.ExtractKeywords(ctx, event.Content), nil
			})
			logger.WithContext(ctx).Info("[GEMINI INSIGHTS] Extracted keywords for post %s: %v", event.PostID, tags)

			_, err = profiler.TrackExecutionWithReturn("ai-service:worker keyword.neo4j.write", func() (any, error) {
				return nil, s.SavePostKeywords(ctx, event.PostID, tags, event.IsUpdate || event.Event == "post_updated")
			})
			if err != nil {
				logger.WithContext(ctx).Err(err).Error("AI worker failed to write Keyword relationships for post %s", event.PostID)
				continue
			}
			logger.WithContext(ctx).Info("[DB WRITE] Neo4j graph updated with relationships: Post(%s) -[:HAS_KEYWORDS]-> Keyword%v", event.PostID, tags)
		}
	}()
}

func (s *AIService) StartModerationWorker() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{s.KafkaAddr},
		GroupID:  "ai-moderation-worker-group",
		Topic:    moderation.TopicRequested,
		MinBytes: 1,
		MaxBytes: 1e6,
	})

	logger.Info("AI Service: Moderation Worker listening on Kafka broker %s, topic: %s", s.KafkaAddr, moderation.TopicRequested)

	go func() {
		defer reader.Close()
		for {
			m, err := reader.ReadMessage(context.Background())
			if err != nil {
				logger.Error("AI moderation worker read error: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}

			var event moderation.RequestEvent
			if err := json.Unmarshal(m.Value, &event); err != nil {
				logger.Error("AI moderation worker: Error unmarshalling request: %v", err)
				continue
			}
			event.TraceID, event.RequestID = traceIDsFromKafkaMessage(m, event.TraceID, event.RequestID)
			baseCtx := contextWithTraceIDs(context.Background(), event.TraceID, event.RequestID)
			if !moderation.IsValidTargetType(event.TargetType) || event.TargetID == "" {
				logger.WithContext(baseCtx).Warn("AI moderation worker: invalid target in request: %+v", event)
				continue
			}

			ctx, cancel := context.WithTimeout(baseCtx, 25*time.Second)
			result, _ := profiler.TrackResult("ai-service:worker moderation.review", func() (ModerationResult, error) {
				return s.ModerateContent(ctx, event), nil
			})
			_, err = profiler.TrackExecutionWithReturn("ai-service:worker moderation.publishCompleted", func() (any, error) {
				return nil, s.publishModerationCompleted(ctx, event, result)
			})
			cancel()
			if err != nil {
				logger.WithContext(baseCtx).Error("AI moderation worker: failed to publish completed event for %s/%s: %v", event.TargetType, event.TargetID, err)
				continue
			}
			logger.WithContext(baseCtx).Info("AI moderation completed for %s/%s verdict=%s confidence=%.2f categories=%v", event.TargetType, event.TargetID, result.Verdict, result.Confidence, result.Categories)
		}
	}()
}

func main() {
	logger.Info("Starting AI & Media Service (Worker)...")
	_ = godotenv.Load("ai-service/.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	geminiKey := getEnv("GEMINI_KEY", "")
	geminiModel := getEnv("GEMINI_MODEL", "gemini-flash-latest")
	kafkaAddr := getEnv("KAFKA_ADDR", "localhost:9092") // Correct Kafka broker fallback port
	neo4jURI := getEnv("NEO4J_URI", "neo4j://localhost:7687")
	neo4jUser := getEnv("NEO4J_USER", "neo4j")
	neo4jPass := getEnv("NEO4J_PASS", "password")

	neo4jCtx, cancelNeo4j := context.WithTimeout(context.Background(), 3*time.Second)
	neo4jDriver := initNeo4j(neo4jCtx, neo4jURI, neo4jUser, neo4jPass)
	cancelNeo4j()
	if neo4jDriver != nil {
		defer neo4jDriver.Close(context.Background())
	}

	worker := NewAIService(geminiKey, geminiModel, kafkaAddr, neo4jDriver)
	worker.StartWorker()
	worker.StartModerationWorker()

	go func() {
		r := gin.New()
		r.Use(gin.Recovery())
		r.Use(logger.TraceMiddleware())
		r.Use(profiler.Middleware("ai-service"))
		r.Use(logger.GinMiddleware())

		r.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "ai-service"})
		})

		debugGroup := r.Group("/debug/profiler")
		debugGroup.Use(profiler.EndpointGuard())
		{
			debugGroup.GET("", profiler.Handler)
			debugGroup.POST("/reset", func(c *gin.Context) {
				profiler.Reset()
				c.JSON(http.StatusOK, gin.H{"status": "success"})
			})
		}

		if err := r.Run(":10091"); err != nil {
			logger.Error("AI health server failed: %v", err)
		}
	}()

	// Keep service alive
	select {}
}
