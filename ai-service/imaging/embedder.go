package imaging

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"

	"social-network-go/logger"
)

const (
	DefaultEmbeddingModel = "gemini-embedding-2"
	DefaultEmbeddingDim   = 768
)

type GeminiEmbedder struct {
	APIKey     string
	Model      string
	OutputDim  int
	HTTPClient *http.Client
}

func NewGeminiEmbedder(apiKey, model string, outputDim int) *GeminiEmbedder {
	if model == "" {
		model = DefaultEmbeddingModel
	}
	if outputDim <= 0 {
		outputDim = DefaultEmbeddingDim
	}
	return &GeminiEmbedder{
		APIKey:    apiKey,
		Model:     model,
		OutputDim: outputDim,
		HTTPClient: &http.Client{
			Timeout: 25 * time.Second,
		},
	}
}

type GeminiStatusError struct {
	Code    int
	Message string
}

func (e *GeminiStatusError) Error() string {
	return fmt.Sprintf("gemini embedding API error: status %d, message: %s", e.Code, e.Message)
}

func isRetriable(err error) bool {
	if err == nil {
		return false
	}
	var gErr *GeminiStatusError
	if errors.As(err, &gErr) && gErr != nil {
		return gErr.Code == http.StatusTooManyRequests || gErr.Code == http.StatusServiceUnavailable
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return false
}

func (g *GeminiEmbedder) callWithRetry(ctx context.Context, operation string, runReq func() (*http.Response, error)) (*http.Response, error) {
	baseDelay := 500 * time.Millisecond
	maxDelay := 4 * time.Second
	maxRetries := 3

	var resp *http.Response
	var err error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err = runReq()
		if err == nil {
			if resp.StatusCode == http.StatusOK {
				return resp, nil
			}
			bodyBytes, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			err = &GeminiStatusError{
				Code:    resp.StatusCode,
				Message: strings.TrimSpace(string(bodyBytes)),
			}
		}

		if !isRetriable(err) || attempt == maxRetries {
			return nil, err
		}

		delay := baseDelay * time.Duration(1<<attempt)
		if delay > maxDelay {
			delay = maxDelay
		}
		jitter := time.Duration(rand.Intn(200)-100) * delay / 1000
		delay += jitter

		logger.WithContext(ctx).Warn("Gemini %s attempt %d failed: %v. Retrying in %v...", operation, attempt+1, err, delay)

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, err
}

// EmbedImage sends image bytes to Gemini API and returns an L2-normalized vector of 768 dimensions.
func (g *GeminiEmbedder) EmbedImage(ctx context.Context, jpegBytes []byte) ([]float32, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is not configured")
	}
	if len(jpegBytes) == 0 {
		return nil, fmt.Errorf("empty image bytes")
	}

	b64Image := base64.StdEncoding.EncodeToString(jpegBytes)

	payload := map[string]interface{}{
		"content": map[string]interface{}{
			"parts": []map[string]interface{}{
				{
					"inline_data": map[string]interface{}{
						"mime_type": "image/jpeg",
						"data":      b64Image,
					},
				},
			},
		},
		"output_dimensionality": g.OutputDim,
	}

	return g.executeEmbedRequest(ctx, "embed_image", payload)
}

// EmbedText sends query text to Gemini API and returns an L2-normalized vector in the same multimodal space.
func (g *GeminiEmbedder) EmbedText(ctx context.Context, text string) ([]float32, error) {
	if g.APIKey == "" {
		return nil, fmt.Errorf("gemini API key is not configured")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("empty query text")
	}

	payload := map[string]interface{}{
		"content": map[string]interface{}{
			"parts": []map[string]interface{}{
				{
					"text": text,
				},
			},
		},
		"output_dimensionality": g.OutputDim,
	}

	return g.executeEmbedRequest(ctx, "embed_text", payload)
}

func (g *GeminiEmbedder) executeEmbedRequest(ctx context.Context, op string, payload map[string]interface{}) ([]float32, error) {
	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:embedContent", g.Model)

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := g.callWithRetry(ctx, op, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(payloadBytes))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-goog-api-key", g.APIKey)
		return g.HTTPClient.Do(req)
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var geminiResp struct {
		Embedding struct {
			Values []float32 `json:"values"`
		} `json:"embedding"`
	}

	if err := json.Unmarshal(bodyBytes, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to parse gemini embedding response: %w", err)
	}

	vec := geminiResp.Embedding.Values
	if len(vec) == 0 {
		return nil, fmt.Errorf("gemini embedding returned empty vector values")
	}

	// Apply L2 Normalization
	normalized := L2Normalize(vec)
	return normalized, nil
}
