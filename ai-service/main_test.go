package main

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"social-network-go/internal/moderation"
)

func TestRuleBasedModerationSafe(t *testing.T) {
	result := ruleBasedModeration("A normal post about golang and microservices")

	if result.Verdict != moderation.VerdictSafe {
		t.Fatalf("expected safe verdict, got %s", result.Verdict)
	}
	if len(result.Categories) != 0 {
		t.Fatalf("expected no categories, got %v", result.Categories)
	}
}

func TestRuleBasedModerationViolation(t *testing.T) {
	result := ruleBasedModeration("click here for free money and buy now with promo code")

	if result.Verdict != moderation.VerdictViolation {
		t.Fatalf("expected violation verdict, got %s", result.Verdict)
	}
	if result.Confidence < 0.85 {
		t.Fatalf("expected high confidence, got %f", result.Confidence)
	}
	if len(result.Categories) != 1 || result.Categories[0] != moderation.CategorySpam {
		t.Fatalf("expected spam category, got %v", result.Categories)
	}
}

func TestNormalizeKeywordsDeduplicatesAndCleansText(t *testing.T) {
	keywords := normalizeKeywords([]string{"#Neo4j", " neo4j ", "[Kafka]", "", "Go!"})

	expected := []string{"neo4j", "kafka", "go"}
	if len(keywords) != len(expected) {
		t.Fatalf("expected %d keywords, got %d: %v", len(expected), len(keywords), keywords)
	}
	for i := range expected {
		if keywords[i] != expected[i] {
			t.Fatalf("expected keyword %q at index %d, got %q", expected[i], i, keywords[i])
		}
	}
}

func TestGeminiStatusErrorExtractsStructuredFields(t *testing.T) {
	resp := &http.Response{
		Status:     "400 Bad Request",
		StatusCode: http.StatusBadRequest,
		Body: io.NopCloser(strings.NewReader(`{
			"error": {
				"code": 400,
				"message": "API key not valid. Please pass a valid API key.",
				"details": [
					{
						"@type": "type.googleapis.com/google.rpc.ErrorInfo",
						"reason": "API_KEY_INVALID",
						"domain": "googleapis.com",
						"metadata": {
							"service": "generativelanguage.googleapis.com"
						}
					}
				]
			}
		}`)),
	}

	err := geminiStatusError(resp)
	var geminiErr *GeminiAPIError
	if !errors.As(err, &geminiErr) {
		t.Fatalf("expected GeminiAPIError, got %T", err)
	}
	if geminiErr.Reason != "API_KEY_INVALID" {
		t.Fatalf("expected API_KEY_INVALID reason, got %q", geminiErr.Reason)
	}
	if geminiErr.Service != "generativelanguage.googleapis.com" {
		t.Fatalf("expected generativelanguage service, got %q", geminiErr.Service)
	}
}
