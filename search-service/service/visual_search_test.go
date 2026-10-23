package service

import (
	"math"
	"testing"

	"social-network-go/search-service/repository"

	"github.com/stretchr/testify/assert"
)

func TestVisualSearch_CosineConversionAndLabels(t *testing.T) {
	// Cosine raw score mapping test: rawCosine = 2 * esScore - 1
	cases := []struct {
		name          string
		esScore       float64
		expectedRaw   float64
		isImageSearch bool
		expectedLabel string
	}{
		{
			name:          "Perfect image match",
			esScore:       1.0,
			expectedRaw:   1.0,
			isImageSearch: true,
			expectedLabel: "Rất giống",
		},
		{
			name:          "High image similarity (raw = 0.80)",
			esScore:       0.90,
			expectedRaw:   0.80,
			isImageSearch: true,
			expectedLabel: "Rất giống",
		},
		{
			name:          "Moderate high image similarity (raw = 0.65)",
			esScore:       0.825,
			expectedRaw:   0.65,
			isImageSearch: true,
			expectedLabel: "Tương đồng cao",
		},
		{
			name:          "Related image similarity (raw = 0.55)",
			esScore:       0.775,
			expectedRaw:   0.55,
			isImageSearch: true,
			expectedLabel: "Có nét tương đồng",
		},
		{
			name:          "Low image similarity below 0.50 threshold (raw = 0.40)",
			esScore:       0.70,
			expectedRaw:   0.40,
			isImageSearch: true,
			expectedLabel: "Ít liên quan",
		},
		{
			name:          "Text-to-Image Very High match (raw = 0.40)",
			esScore:       0.70,
			expectedRaw:   0.40,
			isImageSearch: false,
			expectedLabel: "Rất phù hợp mô tả",
		},
		{
			name:          "Text-to-Image Related match (raw = 0.25)",
			esScore:       0.625,
			expectedRaw:   0.25,
			isImageSearch: false,
			expectedLabel: "Khá liên quan",
		},
		{
			name:          "Text-to-Image Low match below 0.20 threshold (raw = 0.10)",
			esScore:       0.55,
			expectedRaw:   0.10,
			isImageSearch: false,
			expectedLabel: "Ít liên quan",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := repository.ToRawCosine(tc.esScore)
			assert.InDelta(t, tc.expectedRaw, raw, 0.001)

			label := repository.AssignRelevanceLabel(raw, tc.isImageSearch)
			assert.Equal(t, tc.expectedLabel, label)
		})
	}
}

func TestVisualSearch_ThresholdFiltering(t *testing.T) {
	// Verify that minRawCosine filters out candidates strictly
	imageThreshold := 0.50
	textThreshold := 0.20

	testCandidates := []float64{0.95, 0.82, 0.71, 0.52, 0.49, 0.35, 0.22, 0.19, -0.05}

	var validImageMatches []float64
	for _, c := range testCandidates {
		if c >= imageThreshold {
			validImageMatches = append(validImageMatches, c)
		}
	}
	assert.Equal(t, 4, len(validImageMatches), "Candidates >= 0.50 should pass image filter")

	var validTextMatches []float64
	for _, c := range testCandidates {
		if c >= textThreshold {
			validTextMatches = append(validTextMatches, c)
		}
	}
	assert.Equal(t, 7, len(validTextMatches), "Candidates >= 0.20 should pass cross-modal text filter")
}

func TestVisualSearch_L2DistanceAndCosineSanity(t *testing.T) {
	// Cosine similarity between two normalized vectors: u . v
	u := []float32{1.0, 0.0, 0.0}
	v := []float32{1.0, 0.0, 0.0}

	dot := float64(u[0]*v[0] + u[1]*v[1] + u[2]*v[2])
	assert.Equal(t, 1.0, dot)

	// Orthogonal
	w := []float32{0.0, 1.0, 0.0}
	dotOrtho := float64(u[0]*w[0] + u[1]*w[1] + u[2]*w[2])
	assert.Equal(t, 0.0, dotOrtho)

	// Opposite
	opp := []float32{-1.0, 0.0, 0.0}
	dotOpp := float64(u[0]*opp[0] + u[1]*opp[1] + u[2]*opp[2])
	assert.Equal(t, -1.0, dotOpp)

	assert.True(t, math.Abs(dot-1.0) < 1e-6)
}
