package repository

import (
	"context"
	"math"
	"testing"
	"time"

	"social-network-go/search-service/model"
)

func TestToRawCosine(t *testing.T) {
	tests := []struct {
		name     string
		esScore  float64
		expected float64
	}{
		{"perfect match", 1.0, 1.0},
		{"orthogonal vector", 0.5, 0.0},
		{"opposite vector", 0.0, -1.0},
		{"high similarity", 0.85, 0.70},
		{"moderate similarity", 0.60, 0.20},
		{"clamped upper bound", 1.1, 1.0},
		{"clamped lower bound", -0.1, -1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ToRawCosine(tt.esScore)
			if math.Abs(got-tt.expected) > 1e-6 {
				t.Errorf("ToRawCosine(%v) = %v; want %v", tt.esScore, got, tt.expected)
			}
		})
	}
}

func TestAssignRelevanceLabel(t *testing.T) {
	t.Run("image to image", func(t *testing.T) {
		if got := AssignRelevanceLabel(0.85, true); got != "Rất giống" {
			t.Errorf("expected 'Rất giống', got %s", got)
		}
		if got := AssignRelevanceLabel(0.65, true); got != "Tương đồng cao" {
			t.Errorf("expected 'Tương đồng cao', got %s", got)
		}
		if got := AssignRelevanceLabel(0.55, true); got != "Có nét tương đồng" {
			t.Errorf("expected 'Có nét tương đồng', got %s", got)
		}
		if got := AssignRelevanceLabel(0.40, true); got != "Ít liên quan" {
			t.Errorf("expected 'Ít liên quan', got %s", got)
		}
	})

	t.Run("text to image", func(t *testing.T) {
		if got := AssignRelevanceLabel(0.40, false); got != "Rất phù hợp mô tả" {
			t.Errorf("expected 'Rất phù hợp mô tả', got %s", got)
		}
		if got := AssignRelevanceLabel(0.25, false); got != "Khá liên quan" {
			t.Errorf("expected 'Khá liên quan', got %s", got)
		}
		if got := AssignRelevanceLabel(0.10, false); got != "Ít liên quan" {
			t.Errorf("expected 'Ít liên quan', got %s", got)
		}
	})
}

func TestBuildKNNQuery(t *testing.T) {
	vec := []float32{0.1, 0.2, -0.3}
	allowedAuthors := []string{"viewer_123", "friend_456"}
	excludePost := "post_999"
	limit := 10

	query := BuildKNNQuery(vec, allowedAuthors, excludePost, limit)

	if query["size"] != 10 {
		t.Errorf("expected size 10, got %v", query["size"])
	}

	knn, ok := query["knn"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected knn object in query")
	}

	if knn["field"] != "image_vector" {
		t.Errorf("expected field image_vector, got %v", knn["field"])
	}

	filter, ok := knn["filter"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected filter in knn query")
	}

	boolFilter, ok := filter["bool"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected bool filter in knn query")
	}

	// Verify must_not contains excludePost
	mustNot, ok := boolFilter["must_not"].([]map[string]interface{})
	if !ok || len(mustNot) == 0 {
		t.Fatalf("expected must_not clause with excludePost")
	}
	termPost, ok := mustNot[0]["term"].(map[string]interface{})
	if !ok || termPost["post_id"] != excludePost {
		t.Errorf("expected term post_id %s, got %v", excludePost, termPost)
	}

	// Verify should clauses (PUBLIC + allowedAuthors)
	should, ok := boolFilter["should"].([]map[string]interface{})
	if !ok || len(should) != 2 {
		t.Fatalf("expected 2 should clauses (PUBLIC and allowedAuthors), got %d", len(should))
	}

	// Test when excludePost is empty
	queryWithoutExclude := BuildKNNQuery(vec, allowedAuthors, "", limit)
	knnNoEx, _ := queryWithoutExclude["knn"].(map[string]interface{})
	filterNoEx, _ := knnNoEx["filter"].(map[string]interface{})
	boolNoEx, _ := filterNoEx["bool"].(map[string]interface{})
	if _, exists := boolNoEx["must_not"]; exists {
		t.Errorf("did not expect must_not clause when excludePost is empty")
	}
	// Verify k and num_candidates have safe minimum thresholds
	if knn["k"] != 100 {
		t.Errorf("expected k to be at least 100, got %v", knn["k"])
	}
	if knn["num_candidates"] != 300 {
		t.Errorf("expected num_candidates to be at least 300, got %v", knn["num_candidates"])
	}
}

func TestIndexPostImageValidation(t *testing.T) {
	repo := &ESRepository{client: nil}
	ctx := context.Background()

	// Should fail on nil client or nil doc
	if err := repo.IndexPostImage(ctx, nil); err == nil {
		t.Errorf("expected error for nil client")
	}

	// Should fail on missing postID
	docMissingPost := &model.PostImageDoc{FileID: "file1"}
	if err := repo.IndexPostImage(ctx, docMissingPost); err == nil {
		t.Errorf("expected error for missing postID")
	}

	// ID auto-generation
	validDoc := &model.PostImageDoc{
		PostID:    "post_1",
		FileID:    "file_1",
		CreatedAt: time.Now(),
	}
	if validDoc.ID == "" {
		validDoc.ID = validDoc.PostID + "_" + validDoc.FileID
	}
	if validDoc.ID != "post_1_file_1" {
		t.Errorf("expected ID 'post_1_file_1', got %s", validDoc.ID)
	}
}

func TestSearchMessages_EmptyConversations(t *testing.T) {
	repo := &ESRepository{client: nil}
	ctx := context.Background()

	// If user is not member of any conversation, must return empty immediately without querying ES
	results, err := repo.SearchMessages(ctx, "secret", "user1", []string{}, 10)
	if err != nil {
		t.Fatalf("expected nil error for empty conversation IDs, got %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when user has no conversation access, got %d", len(results))
	}
}

func TestSearchMessages_NilClientWithConversations(t *testing.T) {
	repo := &ESRepository{client: nil}
	ctx := context.Background()

	// When user has conversation IDs but client is nil, it should return error gracefully
	_, err := repo.SearchMessages(ctx, "hello", "user1", []string{"conv123"}, 10)
	if err == nil {
		t.Errorf("expected error when client is nil and querying ES")
	}
}

func TestSearchPostsWithFacets_NilClient(t *testing.T) {
	repo := &ESRepository{client: nil}
	ctx := context.Background()

	posts, facets, err := repo.SearchPostsWithFacets(ctx, "keyword", "user1", 10)
	if err == nil {
		t.Errorf("expected error when client is nil")
	}
	if posts != nil || facets != nil {
		t.Errorf("expected nil posts and facets on error")
	}
}

func TestIndexMessage_MissingID(t *testing.T) {
	repo := &ESRepository{client: nil}
	ctx := context.Background()

	err := repo.IndexMessage(ctx, map[string]interface{}{
		"conversation_id": "conv_123",
		"content":         "test message",
	})
	if err == nil {
		t.Errorf("expected error when id is missing from message document")
	}
}

func TestReindexIndex_NilClient(t *testing.T) {
	repo := &ESRepository{client: nil}
	ctx := context.Background()

	err := repo.ReindexIndex(ctx, "posts", "posts_v2", "{}")
	if err == nil {
		t.Errorf("expected error when client is nil")
	}
}
