package db

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/search-service/config"

	"github.com/elastic/go-elasticsearch/v8"
)

var ESClient *elasticsearch.Client

// InitElasticsearch initializes and validates connectivity to Elasticsearch
func InitElasticsearch(cfg *config.Config) *elasticsearch.Client {
	esCfg := elasticsearch.Config{
		Addresses: []string{cfg.ElasticsearchURL},
		// Retry on 502, 503, 504
		RetryOnStatus: []int{502, 503, 504},
		MaxRetries:    3,
	}

	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		logger.Warn("Warning: Failed to create Elasticsearch client at %s: %v", cfg.ElasticsearchURL, err)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := client.Info(client.Info.WithContext(ctx))
	if err != nil {
		logger.Warn("Warning: Elasticsearch is unreachable at %s: %v", cfg.ElasticsearchURL, err)
		return nil
	}
	defer res.Body.Close()

	if res.IsError() {
		logger.Warn("Warning: Elasticsearch Info returned error status: %s", res.Status())
		return nil
	}

	ESClient = client
	logger.Info("Successfully connected to Elasticsearch at %s", cfg.ElasticsearchURL)

	// Ensure predefined indices and mappings
	if err := EnsureIndices(ESClient); err != nil {
		logger.Err(err).Warn("Failed to ensure Elasticsearch indices")
	}

	return ESClient
}

// IndexMappings exports default schema definitions for each index
var IndexMappings = map[string]string{
	"users":       userIndexMapping,
	"posts":       postIndexMapping,
	"messages":    messageIndexMapping,
	"groups":      groupIndexMapping,
	"post_images": postImagesIndexMapping,
}

// EnsureIndices creates required indices with versioned names and aliases if they do not exist
func EnsureIndices(client *elasticsearch.Client) error {
	for aliasName, mappingJSON := range IndexMappings {
		// 1. Check if alias already exists
		aliasRes, err := client.Indices.ExistsAlias([]string{aliasName})
		if err != nil {
			logger.Err(err).Warn("Failed to check alias existence: %s", aliasName)
			continue
		}
		aliasExists := aliasRes.StatusCode == http.StatusOK
		aliasRes.Body.Close()

		if aliasExists {
			logger.Info("Elasticsearch alias '%s' is active", aliasName)
			continue
		}

		// 2. Check if a concrete index with aliasName already exists (legacy unversioned mode)
		res, err := client.Indices.Exists([]string{aliasName})
		if err != nil {
			logger.Err(err).Warn("Failed to check concrete index existence: %s", aliasName)
			continue
		}
		concreteExists := res.StatusCode == http.StatusOK
		res.Body.Close()

		if concreteExists {
			logger.Info("Elasticsearch index '%s' exists as a legacy concrete index", aliasName)
			continue
		}

		// 3. Create versioned index (e.g., users_v1) and point alias to it
		versionedIndex := aliasName + "_v1"
		createRes, err := client.Indices.Create(
			versionedIndex,
			client.Indices.Create.WithBody(strings.NewReader(mappingJSON)),
		)
		if err != nil {
			logger.Err(err).Warn("Failed to create versioned index %s", versionedIndex)
			continue
		}
		if createRes.IsError() {
			logger.Warn("Failed creating versioned index %s: status %s", versionedIndex, createRes.Status())
			createRes.Body.Close()
			continue
		}
		createRes.Body.Close()
		logger.Info("Created versioned Elasticsearch index: %s", versionedIndex)

		// 4. Create alias pointing to the versioned index
		putAliasRes, err := client.Indices.PutAlias([]string{versionedIndex}, aliasName)
		if err != nil {
			logger.Err(err).Warn("Failed to create alias %s -> %s", aliasName, versionedIndex)
			continue
		}
		if putAliasRes.IsError() {
			logger.Warn("Failed creating alias %s -> %s: status %s", aliasName, versionedIndex, putAliasRes.Status())
		} else {
			logger.Info("Successfully created alias: %s -> %s", aliasName, versionedIndex)
		}
		putAliasRes.Body.Close()
	}

	return nil
}

// ReindexAndSwitchAlias performs zero-downtime reindexing from the existing index/alias
// to a new targetIndex, then atomically updates the alias to point to targetIndex.
func ReindexAndSwitchAlias(ctx context.Context, client *elasticsearch.Client, aliasName, targetIndex, targetMappingJSON string) error {
	if client == nil {
		return fmt.Errorf("elasticsearch client not initialized")
	}

	// 1. Create targetIndex with new mapping if it does not exist
	resExists, err := client.Indices.Exists([]string{targetIndex}, client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("failed checking target index %s: %w", targetIndex, err)
	}
	targetExists := resExists.StatusCode == http.StatusOK
	resExists.Body.Close()

	if !targetExists {
		createRes, err := client.Indices.Create(
			targetIndex,
			client.Indices.Create.WithContext(ctx),
			client.Indices.Create.WithBody(strings.NewReader(targetMappingJSON)),
		)
		if err != nil {
			return fmt.Errorf("failed creating target index %s: %w", targetIndex, err)
		}
		defer createRes.Body.Close()
		if createRes.IsError() {
			return fmt.Errorf("error creating target index %s: %s", targetIndex, createRes.Status())
		}
		logger.Info("Created target index %s for zero-downtime migration", targetIndex)
	}

	// 2. Execute Reindex from current alias/index to targetIndex
	reindexBody := map[string]interface{}{
		"source": map[string]interface{}{
			"index": aliasName,
		},
		"dest": map[string]interface{}{
			"index": targetIndex,
		},
	}
	reindexJSON, err := json.Marshal(reindexBody)
	if err != nil {
		return fmt.Errorf("failed to marshal reindex body: %w", err)
	}

	reindexRes, err := client.Reindex(
		bytes.NewReader(reindexJSON),
		client.Reindex.WithContext(ctx),
		client.Reindex.WithWaitForCompletion(true),
	)
	if err != nil {
		return fmt.Errorf("reindex failed from %s to %s: %w", aliasName, targetIndex, err)
	}
	defer reindexRes.Body.Close()
	if reindexRes.IsError() {
		return fmt.Errorf("reindex returned error status: %s", reindexRes.Status())
	}
	logger.Info("Reindex completed successfully from %s to %s", aliasName, targetIndex)

	// 3. Atomically switch alias: Remove old pointers to aliasName and point to targetIndex
	aliasExistsRes, err := client.Indices.ExistsAlias([]string{aliasName}, client.Indices.ExistsAlias.WithContext(ctx))
	isAlias := false
	if err == nil {
		isAlias = aliasExistsRes.StatusCode == http.StatusOK
		aliasExistsRes.Body.Close()
	}

	if isAlias {
		aliasPayload := map[string]interface{}{
			"actions": []map[string]interface{}{
				{
					"remove": map[string]interface{}{
						"index": "*",
						"alias": aliasName,
					},
				},
				{
					"add": map[string]interface{}{
						"index": targetIndex,
						"alias": aliasName,
					},
				},
			},
		}
		aliasBody, _ := json.Marshal(aliasPayload)
		updateRes, err := client.Indices.UpdateAliases(
			bytes.NewReader(aliasBody),
			client.Indices.UpdateAliases.WithContext(ctx),
		)
		if err != nil {
			return fmt.Errorf("failed updating alias %s: %w", aliasName, err)
		}
		defer updateRes.Body.Close()
		if updateRes.IsError() {
			return fmt.Errorf("failed updating alias: %s", updateRes.Status())
		}
	} else {
		// If aliasName was a legacy concrete index, delete old index then create alias
		delRes, err := client.Indices.Delete([]string{aliasName}, client.Indices.Delete.WithContext(ctx))
		if err == nil {
			delRes.Body.Close()
		}
		putRes, err := client.Indices.PutAlias([]string{targetIndex}, aliasName, client.Indices.PutAlias.WithContext(ctx))
		if err != nil {
			return fmt.Errorf("failed creating alias %s -> %s: %w", aliasName, targetIndex, err)
		}
		defer putRes.Body.Close()
	}

	logger.Info("Successfully completed zero-downtime reindex for %s -> %s", aliasName, targetIndex)
	return nil
}

// Index mappings with Vietnamese & Autocomplete Analyzers
const commonSettings = `
  "settings": {
    "index": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    },
    "analysis": {
      "filter": {
        "edge_ngram_filter": {
          "type": "edge_ngram",
          "min_gram": 2,
          "max_gram": 20
        }
      },
      "analyzer": {
        "vietnamese_analyzer": {
          "type": "custom",
          "tokenizer": "standard",
          "filter": ["lowercase", "asciifolding"]
        },
        "autocomplete_analyzer": {
          "type": "custom",
          "tokenizer": "standard",
          "filter": ["lowercase", "asciifolding", "edge_ngram_filter"]
        }
      }
    }
  }
`

const userIndexMapping = `{` + commonSettings + `,
  "mappings": {
    "properties": {
      "id": { "type": "keyword" },
      "username": {
        "type": "text",
        "analyzer": "autocomplete_analyzer",
        "search_analyzer": "vietnamese_analyzer",
        "fields": {
          "keyword": { "type": "keyword" }
        }
      },
      "given_name": { "type": "text", "analyzer": "vietnamese_analyzer" },
      "family_name": { "type": "text", "analyzer": "vietnamese_analyzer" },
      "full_name": {
        "type": "text",
        "analyzer": "autocomplete_analyzer",
        "search_analyzer": "vietnamese_analyzer",
        "fields": {
          "exact": { "type": "text", "analyzer": "standard" }
        }
      },
      "email": { "type": "keyword" },
      "bio": { "type": "text", "analyzer": "vietnamese_analyzer" },
      "profile_picture_url": { "type": "keyword", "index": false },
      "created_at": { "type": "date" }
    }
  }
}`

const postIndexMapping = `{` + commonSettings + `,
  "mappings": {
    "properties": {
      "id": { "type": "keyword" },
      "author_id": { "type": "keyword" },
      "content": {
        "type": "text",
        "analyzer": "vietnamese_analyzer",
        "fields": {
          "exact": { "type": "text", "analyzer": "standard" }
        }
      },
      "privacy": { "type": "keyword" },
      "files": { "type": "keyword", "index": false },
      "like_count": { "type": "integer" },
      "comment_count": { "type": "integer" },
      "share_count": { "type": "integer" },
      "created_at": { "type": "date" },
      "updated_at": { "type": "date" },
      "deleted_at": { "type": "date" }
    }
  }
}`

const messageIndexMapping = `{` + commonSettings + `,
  "mappings": {
    "_routing": { "required": false },
    "properties": {
      "id": { "type": "keyword" },
      "conversation_id": { "type": "keyword" },
      "sender_id": { "type": "keyword" },
      "content": {
        "type": "text",
        "analyzer": "vietnamese_analyzer"
      },
      "message_type": { "type": "keyword" },
      "created_at": { "type": "date" },
      "deleted_at": { "type": "date" }
    }
  }
}`

const groupIndexMapping = `{` + commonSettings + `,
  "mappings": {
    "properties": {
      "id": { "type": "keyword" },
      "name": {
        "type": "text",
        "analyzer": "autocomplete_analyzer",
        "search_analyzer": "vietnamese_analyzer"
      },
      "description": {
        "type": "text",
        "analyzer": "vietnamese_analyzer"
      },
      "privacy": { "type": "keyword" },
      "member_count": { "type": "integer" },
      "created_at": { "type": "date" }
    }
  }
}`

const PostImagesIndex = "post_images"

const postImagesIndexMapping = `{
  "settings": {
    "index": {
      "number_of_shards": 1,
      "number_of_replicas": 0
    }
  },
  "mappings": {
    "properties": {
      "id": { "type": "keyword" },
      "post_id": { "type": "keyword" },
      "file_id": { "type": "keyword" },
      "author_id": { "type": "keyword" },
      "privacy": { "type": "keyword" },
      "embedding_model": { "type": "keyword" },
      "model_version": { "type": "keyword" },
      "image_vector": {
        "type": "dense_vector",
        "dims": 768,
        "index": true,
        "similarity": "cosine",
        "index_options": {
          "type": "int8_hnsw",
          "m": 16,
          "ef_construction": 100
        }
      },
      "created_at": { "type": "date" }
    }
  }
}`

// PingElasticsearch checks if Elasticsearch is currently responsive
func PingElasticsearch(ctx context.Context) error {
	if ESClient == nil {
		return fmt.Errorf("elasticsearch client not initialized")
	}
	res, err := ESClient.Ping(ESClient.Ping.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch ping error: %s", res.Status())
	}
	return nil
}

// BulkIndex helper to insert/upsert documents in bulk
func BulkIndex(ctx context.Context, indexName string, docs []map[string]interface{}) error {
	if ESClient == nil || len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		id, _ := doc["id"].(string)
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    id,
			},
		}
		if err := json.NewEncoder(&buf).Encode(meta); err != nil {
			return err
		}
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			return err
		}
	}

	res, err := ESClient.Bulk(bytes.NewReader(buf.Bytes()), ESClient.Bulk.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("bulk index error: %s", res.Status())
	}
	return nil
}
