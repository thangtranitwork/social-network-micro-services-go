package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"social-network-go/search-service/db"
	"social-network-go/search-service/model"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

type ESRepository struct {
	client *elasticsearch.Client
}

func NewESRepository(client *elasticsearch.Client) *ESRepository {
	return &ESRepository{client: client}
}

func (r *ESRepository) SearchUsers(ctx context.Context, query string, blockedUserIDs []string, limit int) ([]*model.User, error) {
	if r.client == nil {
		return nil, fmt.Errorf("elasticsearch client not available")
	}
	if limit <= 0 {
		limit = 15
	}

	boolQuery := map[string]interface{}{
		"should": []map[string]interface{}{
			{
				"term": map[string]interface{}{
					"username.keyword": map[string]interface{}{
						"value": query,
						"boost": 10.0,
					},
				},
			},
			{
				"match_phrase_prefix": map[string]interface{}{
					"username": map[string]interface{}{
						"query": query,
						"boost": 5.0,
					},
				},
			},
			{
				"match": map[string]interface{}{
					"full_name.exact": map[string]interface{}{
						"query": query,
						"boost": 6.0,
					},
				},
			},
			{
				"match": map[string]interface{}{
					"full_name": map[string]interface{}{
						"query":     query,
						"boost":     3.0,
						"fuzziness": "AUTO",
					},
				},
			},
			{
				"match": map[string]interface{}{
					"given_name": map[string]interface{}{
						"query": query,
						"boost": 2.0,
					},
				},
			},
			{
				"match": map[string]interface{}{
					"family_name": map[string]interface{}{
						"query": query,
						"boost": 2.0,
					},
				},
			},
			{
				"match": map[string]interface{}{
					"bio": map[string]interface{}{
						"query": query,
						"boost": 1.0,
					},
				},
			},
		},
		"minimum_should_match": 1,
	}

	// Filter out blocked users (bidirectional block guard)
	if len(blockedUserIDs) > 0 {
		boolQuery["must_not"] = []map[string]interface{}{
			{
				"terms": map[string]interface{}{
					"id": blockedUserIDs,
				},
			},
		}
	}

	searchQuery := map[string]interface{}{
		"size":  limit,
		"query": map[string]interface{}{"bool": boolQuery},
	}

	body, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, err
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex("users"),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search users failed with status: %s", res.Status())
	}

	var esRes struct {
		Hits struct {
			Hits []struct {
				ID     string `json:"_id"`
				Source struct {
					ID                string    `json:"id"`
					Username          string    `json:"username"`
					GivenName         string    `json:"given_name"`
					FamilyName        string    `json:"family_name"`
					Bio               string    `json:"bio"`
					ProfilePictureURL string    `json:"profile_picture_url"`
					CreatedAt         time.Time `json:"created_at"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, err
	}

	users := make([]*model.User, 0, len(esRes.Hits.Hits))
	for _, hit := range esRes.Hits.Hits {
		u := hit.Source
		users = append(users, &model.User{
			ID:                u.ID,
			Username:          u.Username,
			GivenName:         u.GivenName,
			FamilyName:        u.FamilyName,
			Email:             "", // Security/Privacy: Do NOT leak emails in public search results
			Bio:               u.Bio,
			ProfilePictureUrl: u.ProfilePictureURL,
		})
	}

	return users, nil
}

func (r *ESRepository) SearchPosts(ctx context.Context, query string, currentUserID string, limit int) ([]*model.Post, error) {
	posts, _, err := r.SearchPostsWithFacets(ctx, query, currentUserID, limit)
	return posts, err
}

// SearchPostsWithFacets runs full-text search with hit highlighting and term aggregations
func (r *ESRepository) SearchPostsWithFacets(ctx context.Context, query string, currentUserID string, limit int) ([]*model.Post, *model.PostSearchFacets, error) {
	if r.client == nil {
		return nil, nil, fmt.Errorf("elasticsearch client not available")
	}
	if limit <= 0 {
		limit = 15
	}

	searchQuery := map[string]interface{}{
		"size": limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"match": map[string]interface{}{
							"content": map[string]interface{}{
								"query":     query,
								"fuzziness": "AUTO",
							},
						},
					},
				},
				"filter": []map[string]interface{}{
					{
						"term": map[string]interface{}{
							"privacy": "PUBLIC",
						},
					},
				},
				"must_not": []map[string]interface{}{
					{
						"exists": map[string]interface{}{
							"field": "deleted_at",
						},
					},
				},
			},
		},
		"highlight": map[string]interface{}{
			"pre_tags":  []string{"<mark class=\"search-highlight\">"},
			"post_tags": []string{"</mark>"},
			"fields": map[string]interface{}{
				"content": map[string]interface{}{
					"number_of_fragments": 3,
					"fragment_size":       150,
				},
			},
		},
		"aggs": map[string]interface{}{
			"privacy_counts": map[string]interface{}{
				"terms": map[string]interface{}{"field": "privacy"},
			},
			"top_authors": map[string]interface{}{
				"terms": map[string]interface{}{"field": "author_id", "size": 10},
			},
		},
		"sort": []map[string]interface{}{
			{"_score": map[string]interface{}{"order": "desc"}},
			{"created_at": map[string]interface{}{"order": "desc"}},
		},
	}

	body, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, nil, err
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex("posts"),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, nil, fmt.Errorf("search posts failed with status: %s", res.Status())
	}

	var esRes struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string `json:"_id"`
				Source struct {
					ID           string     `json:"id"`
					AuthorID     string     `json:"author_id"`
					Content      string     `json:"content"`
					Privacy      string     `json:"privacy"`
					Files        []string   `json:"files"`
					LikeCount    int        `json:"like_count"`
					CommentCount int        `json:"comment_count"`
					ShareCount   int        `json:"share_count"`
					CreatedAt    time.Time  `json:"created_at"`
					UpdatedAt    *time.Time `json:"updated_at,omitempty"`
					DeletedAt    *time.Time `json:"deleted_at,omitempty"`
				} `json:"_source"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
		Aggregations struct {
			PrivacyCounts struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"privacy_counts"`
			TopAuthors struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"top_authors"`
		} `json:"aggregations"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, nil, err
	}

	posts := make([]*model.Post, 0, len(esRes.Hits.Hits))
	for _, hit := range esRes.Hits.Hits {
		p := hit.Source
		postModel := &model.Post{
			ID:           p.ID,
			AuthorID:     p.AuthorID,
			Content:      p.Content,
			Privacy:      p.Privacy,
			Files:        p.Files,
			Images:       p.Files,
			LikeCount:    p.LikeCount,
			CommentCount: p.CommentCount,
			ShareCount:   p.ShareCount,
			CreatedAt:    p.CreatedAt,
			UpdatedAt:    p.UpdatedAt,
		}
		if len(hit.Highlight["content"]) > 0 {
			postModel.Highlights = hit.Highlight["content"]
		}
		posts = append(posts, postModel)
	}

	facets := &model.PostSearchFacets{
		TotalHits: esRes.Hits.Total.Value,
		Privacy:   make(map[string]int64),
		Authors:   make(map[string]int64),
	}
	for _, b := range esRes.Aggregations.PrivacyCounts.Buckets {
		facets.Privacy[b.Key] = b.DocCount
	}
	for _, b := range esRes.Aggregations.TopAuthors.Buckets {
		facets.Authors[b.Key] = b.DocCount
	}

	return posts, facets, nil
}

func (r *ESRepository) SearchMessages(ctx context.Context, query string, currentUserID string, userConversationIDs []string, limit int) ([]*model.Message, error) {
	// Security check: User must be a member of at least one conversation to search messages
	if currentUserID == "" || len(userConversationIDs) == 0 {
		return []*model.Message{}, nil
	}
	if r.client == nil {
		return nil, fmt.Errorf("elasticsearch client not available")
	}
	if limit <= 0 {
		limit = 15
	}

	searchQuery := map[string]interface{}{
		"size": limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must": []map[string]interface{}{
					{
						"match": map[string]interface{}{
							"content": map[string]interface{}{
								"query":     query,
								"fuzziness": "AUTO",
							},
						},
					},
				},
				"filter": []map[string]interface{}{
					{
						"terms": map[string]interface{}{
							"conversation_id": userConversationIDs,
						},
					},
				},
				"must_not": []map[string]interface{}{
					{
						"exists": map[string]interface{}{
							"field": "deleted_at",
						},
					},
				},
			},
		},
		"sort": []map[string]interface{}{
			{"created_at": map[string]interface{}{"order": "desc"}},
		},
	}

	body, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, err
	}

	// Shard Routing Optimization: limit query execution only to shards containing these conversation IDs
	routingKey := strings.Join(userConversationIDs, ",")
	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex("messages"),
		r.client.Search.WithRouting(routingKey),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search messages failed with status: %s", res.Status())
	}

	var esRes struct {
		Hits struct {
			Hits []struct {
				Source model.Message `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, err
	}

	messages := make([]*model.Message, 0, len(esRes.Hits.Hits))
	for _, hit := range esRes.Hits.Hits {
		msg := hit.Source
		messages = append(messages, &msg)
	}

	return messages, nil
}

func (r *ESRepository) SearchGroups(ctx context.Context, query string, currentUserID string, joinedGroupIDs []string, limit int) ([]*model.Group, error) {
	if r.client == nil {
		return nil, fmt.Errorf("elasticsearch client not available")
	}
	if limit <= 0 {
		limit = 15
	}

	shouldFilters := []map[string]interface{}{
		{"term": map[string]interface{}{"privacy": "PUBLIC"}},
	}
	if len(joinedGroupIDs) > 0 {
		shouldFilters = append(shouldFilters, map[string]interface{}{
			"terms": map[string]interface{}{"id": joinedGroupIDs},
		})
	}

	searchQuery := map[string]interface{}{
		"size": limit,
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"should": []map[string]interface{}{
					{
						"match_phrase_prefix": map[string]interface{}{
							"name": map[string]interface{}{
								"query": query,
								"boost": 3.0,
							},
						},
					},
					{
						"match": map[string]interface{}{
							"description": map[string]interface{}{
								"query": query,
								"boost": 1.0,
							},
						},
					},
				},
				"minimum_should_match": 1,
				"filter": []map[string]interface{}{
					{
						"bool": map[string]interface{}{
							"should":               shouldFilters,
							"minimum_should_match": 1,
						},
					},
				},
			},
		},
	}

	body, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, err
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex("groups"),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("search groups failed with status: %s", res.Status())
	}

	var esRes struct {
		Hits struct {
			Hits []struct {
				Source model.Group `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, err
	}

	groups := make([]*model.Group, 0, len(esRes.Hits.Hits))
	for _, hit := range esRes.Hits.Hits {
		g := hit.Source
		groups = append(groups, &g)
	}

	return groups, nil
}

// Index / Upsert document helpers
func (r *ESRepository) IndexUser(ctx context.Context, userDoc map[string]interface{}) error {
	id, ok := userDoc["id"].(string)
	if !ok || id == "" {
		return fmt.Errorf("user document missing id")
	}

	body, err := json.Marshal(userDoc)
	if err != nil {
		return err
	}

	res, err := r.client.Index(
		"users",
		bytes.NewReader(body),
		r.client.Index.WithDocumentID(id),
		r.client.Index.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index user error: %s", res.Status())
	}
	return nil
}

func (r *ESRepository) IndexPost(ctx context.Context, postDoc map[string]interface{}) error {
	id, ok := postDoc["id"].(string)
	if !ok || id == "" {
		return fmt.Errorf("post document missing id")
	}

	body, err := json.Marshal(postDoc)
	if err != nil {
		return err
	}

	res, err := r.client.Index(
		"posts",
		bytes.NewReader(body),
		r.client.Index.WithDocumentID(id),
		r.client.Index.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index post error: %s", res.Status())
	}
	return nil
}

func (r *ESRepository) UpdatePost(ctx context.Context, postID string, updates map[string]interface{}) error {
	body, err := json.Marshal(map[string]interface{}{"doc": updates})
	if err != nil {
		return err
	}

	res, err := r.client.Update(
		"posts",
		postID,
		bytes.NewReader(body),
		r.client.Update.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("update post error: %s", res.Status())
	}
	return nil
}

func (r *ESRepository) DeletePost(ctx context.Context, postID string) error {
	// Mark deleted_at or delete doc
	now := time.Now().UTC()
	return r.UpdatePost(ctx, postID, map[string]interface{}{
		"deleted_at": now,
	})
}

// IndexMessage indexes a message document into Elasticsearch with Shard Routing based on conversation_id
func (r *ESRepository) IndexMessage(ctx context.Context, msgDoc map[string]interface{}) error {
	id, ok := msgDoc["id"].(string)
	if !ok || id == "" {
		return fmt.Errorf("message document missing id")
	}

	convID, _ := msgDoc["conversation_id"].(string)

	body, err := json.Marshal(msgDoc)
	if err != nil {
		return err
	}

	var res *esapi.Response
	if convID != "" {
		res, err = r.client.Index(
			"messages",
			bytes.NewReader(body),
			r.client.Index.WithDocumentID(id),
			r.client.Index.WithContext(ctx),
			r.client.Index.WithRouting(convID),
		)
	} else {
		res, err = r.client.Index(
			"messages",
			bytes.NewReader(body),
			r.client.Index.WithDocumentID(id),
			r.client.Index.WithContext(ctx),
		)
	}
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index message error: %s", res.Status())
	}
	return nil
}

// ReindexIndex provides repository-level zero-downtime reindexing using atomic alias switching
func (r *ESRepository) ReindexIndex(ctx context.Context, aliasName, targetIndex, targetMappingJSON string) error {
	return db.ReindexAndSwitchAlias(ctx, r.client, aliasName, targetIndex, targetMappingJSON)
}

// -------------------------------------------------------------
// Visual Search (Vector DB) Operations
// -------------------------------------------------------------

// ToRawCosine converts Elasticsearch cosine _score ((1 + cos) / 2) to raw cosine in [-1.0, 1.0].
func ToRawCosine(esScore float64) float64 {
	cos := 2.0*esScore - 1.0
	if cos > 1.0 {
		return 1.0
	}
	if cos < -1.0 {
		return -1.0
	}
	return cos
}

// AssignRelevanceLabel assigns qualitative labels based on raw cosine similarity.
func AssignRelevanceLabel(rawCosine float64, isImageToImage bool) string {
	if isImageToImage {
		switch {
		case rawCosine >= 0.75:
			return "Rất giống"
		case rawCosine >= 0.60:
			return "Tương đồng cao"
		case rawCosine >= 0.50:
			return "Có nét tương đồng"
		default:
			return "Ít liên quan"
		}
	}
	// Text-to-Image has lower cross-modal cosine distribution
	switch {
	case rawCosine >= 0.35:
		return "Rất phù hợp mô tả"
	case rawCosine >= 0.20:
		return "Khá liên quan"
	default:
		return "Ít liên quan"
	}
}

// IndexPostImage indexes an image embedding vector into post_images index idempotently.
func (r *ESRepository) IndexPostImage(ctx context.Context, doc *model.PostImageDoc) error {
	if r.client == nil {
		return fmt.Errorf("elasticsearch client not available")
	}
	if doc == nil || doc.PostID == "" || doc.FileID == "" {
		return fmt.Errorf("invalid post image document: missing post_id or file_id")
	}
	if doc.ID == "" {
		doc.ID = fmt.Sprintf("%s_%s", doc.PostID, doc.FileID)
	}

	body, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal post image doc: %w", err)
	}

	res, err := r.client.Index(
		"post_images",
		bytes.NewReader(body),
		r.client.Index.WithDocumentID(doc.ID),
		r.client.Index.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("index post image error: %s", res.Status())
	}
	return nil
}

// DeletePostImagesByPost deletes all image vectors of a given post (e.g. post deleted or made PRIVATE).
func (r *ESRepository) DeletePostImagesByPost(ctx context.Context, postID string) error {
	if r.client == nil {
		return fmt.Errorf("elasticsearch client not available")
	}
	if postID == "" {
		return nil
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"post_id": postID,
			},
		},
	}

	body, err := json.Marshal(query)
	if err != nil {
		return err
	}

	res, err := r.client.DeleteByQuery(
		[]string{"post_images"},
		bytes.NewReader(body),
		r.client.DeleteByQuery.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("delete post images by post error: %s", res.Status())
	}
	return nil
}

// DeletePostImageByFile deletes a single image vector (e.g. file removed from post during edit).
func (r *ESRepository) DeletePostImageByFile(ctx context.Context, postID, fileID string) error {
	if r.client == nil {
		return fmt.Errorf("elasticsearch client not available")
	}
	docID := fmt.Sprintf("%s_%s", postID, fileID)

	res, err := r.client.Delete(
		"post_images",
		docID,
		r.client.Delete.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() && res.StatusCode != 404 {
		return fmt.Errorf("delete post image by file error: %s", res.Status())
	}
	return nil
}

// UpdatePostImagesPrivacy updates the privacy attribute of all image vectors belonging to a post.
func (r *ESRepository) UpdatePostImagesPrivacy(ctx context.Context, postID, privacy string) error {
	if r.client == nil {
		return fmt.Errorf("elasticsearch client not available")
	}
	if postID == "" {
		return nil
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"post_id": postID,
			},
		},
		"script": map[string]interface{}{
			"source": "ctx._source.privacy = params.privacy",
			"lang":   "painless",
			"params": map[string]interface{}{
				"privacy": privacy,
			},
		},
	}

	body, err := json.Marshal(query)
	if err != nil {
		return err
	}

	res, err := r.client.UpdateByQuery(
		[]string{"post_images"},
		r.client.UpdateByQuery.WithBody(bytes.NewReader(body)),
		r.client.UpdateByQuery.WithContext(ctx),
	)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("update post images privacy error: %s", res.Status())
	}
	return nil
}

// GetImageVectorByFileID retrieves the stored image vector and metadata for a specific fileID.
// Used for 1-click similar search without calling Gemini API again.
func (r *ESRepository) GetImageVectorByFileID(ctx context.Context, fileID string) ([]float32, string, string, error) {
	if r.client == nil {
		return nil, "", "", fmt.Errorf("elasticsearch client not available")
	}
	if fileID == "" {
		return nil, "", "", fmt.Errorf("empty fileID")
	}

	query := map[string]interface{}{
		"size": 1,
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"file_id": fileID,
			},
		},
	}

	body, err := json.Marshal(query)
	if err != nil {
		return nil, "", "", err
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex("post_images"),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, "", "", err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, "", "", fmt.Errorf("search image vector by fileID error: %s", res.Status())
	}

	var esRes struct {
		Hits struct {
			Hits []struct {
				Source model.PostImageDoc `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, "", "", err
	}

	if len(esRes.Hits.Hits) == 0 {
		return nil, "", "", fmt.Errorf("image vector not found for file %s", fileID)
	}

	doc := esRes.Hits.Hits[0].Source
	return doc.ImageVector, doc.PostID, doc.AuthorID, nil
}

// BuildKNNQuery constructs an Elasticsearch 8.13 kNN query with pre-filter.
func BuildKNNQuery(queryVector []float32, allowedAuthorIDs []string, excludePostID string, limit int) map[string]interface{} {
	k := limit * 4
	if k < 100 {
		k = 100
	}
	if k > 300 {
		k = 300
	}
	numCandidates := k * 3
	if numCandidates < 300 {
		numCandidates = 300
	}
	if numCandidates > 1000 {
		numCandidates = 1000
	}

	// Filter conditions:
	// 1. Must not match excludePostID (if specified)
	// 2. Should match:
	//    - privacy: "PUBLIC"
	//    - author_id in allowedAuthorIDs (includes viewerID and viewer's friends)
	shouldClauses := []map[string]interface{}{
		{
			"term": map[string]interface{}{
				"privacy": "PUBLIC",
			},
		},
	}

	if len(allowedAuthorIDs) > 0 {
		shouldClauses = append(shouldClauses, map[string]interface{}{
			"terms": map[string]interface{}{
				"author_id": allowedAuthorIDs,
			},
		})
	}

	filterBool := map[string]interface{}{
		"should":               shouldClauses,
		"minimum_should_match": 1,
	}

	if excludePostID != "" {
		filterBool["must_not"] = []map[string]interface{}{
			{
				"term": map[string]interface{}{
					"post_id": excludePostID,
				},
			},
		}
	}

	return map[string]interface{}{
		"size": limit,
		"knn": map[string]interface{}{
			"field":          "image_vector",
			"query_vector":   queryVector,
			"k":              k,
			"num_candidates": numCandidates,
			"filter": map[string]interface{}{
				"bool": filterBool,
			},
		},
	}
}

// SearchSimilarImagesWithFilter runs kNN search on post_images with pre-filtering and score conversion.
func (r *ESRepository) SearchSimilarImagesWithFilter(ctx context.Context, queryVector []float32, allowedAuthorIDs []string, excludePostID string, minRawCosine float64, limit int, isImageToImage bool) ([]*model.ImageSearchResult, error) {
	if r.client == nil {
		return nil, fmt.Errorf("elasticsearch client not available")
	}
	if len(queryVector) == 0 {
		return nil, fmt.Errorf("empty query vector")
	}
	if limit <= 0 {
		limit = 20
	}

	searchQuery := BuildKNNQuery(queryVector, allowedAuthorIDs, excludePostID, limit)

	body, err := json.Marshal(searchQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal knn query: %w", err)
	}

	res, err := r.client.Search(
		r.client.Search.WithContext(ctx),
		r.client.Search.WithIndex("post_images"),
		r.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("knn search error: %s", res.Status())
	}

	var esRes struct {
		Hits struct {
			Hits []struct {
				Score  float64            `json:"_score"`
				Source model.PostImageDoc `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(res.Body).Decode(&esRes); err != nil {
		return nil, fmt.Errorf("failed to decode knn search response: %w", err)
	}

	results := make([]*model.ImageSearchResult, 0, len(esRes.Hits.Hits))
	seenPostIDs := make(map[string]bool)

	for _, hit := range esRes.Hits.Hits {
		rawCosine := ToRawCosine(hit.Score)
		if rawCosine < minRawCosine {
			continue
		}

		// Deduplicate: Keep highest cosine score hit per post
		if seenPostIDs[hit.Source.PostID] {
			continue
		}
		seenPostIDs[hit.Source.PostID] = true

		results = append(results, &model.ImageSearchResult{
			PostID:         hit.Source.PostID,
			FileID:         hit.Source.FileID,
			AuthorID:       hit.Source.AuthorID,
			Privacy:        hit.Source.Privacy,
			Score:          hit.Score,
			RawCosine:      rawCosine,
			RelevanceLabel: AssignRelevanceLabel(rawCosine, isImageToImage),
			CreatedAt:      hit.Source.CreatedAt,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}
