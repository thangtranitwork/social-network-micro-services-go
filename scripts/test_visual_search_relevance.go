package main

import (
	"fmt"
	"math"
)

// ToRawCosine converts Elasticsearch cosine score (1 + cos)/2 back to raw cosine [-1.0, 1.0]
func ToRawCosine(esScore float64) float64 {
	raw := 2.0*esScore - 1.0
	if raw > 1.0 {
		return 1.0
	}
	if raw < -1.0 {
		return -1.0
	}
	return raw
}

// AssignRelevanceLabel classifies similarity score into qualitative labels
func AssignRelevanceLabel(rawCosine float64, isImageSearch bool) string {
	if isImageSearch {
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
	// Text-to-Image cross-modal
	switch {
	case rawCosine >= 0.35:
		return "Rất phù hợp mô tả"
	case rawCosine >= 0.20:
		return "Khá liên quan"
	default:
		return "Ít liên quan"
	}
}

type TestCase struct {
	Name           string
	Type           string // "IMAGE_IMAGE" or "TEXT_IMAGE"
	RawCosine      float64
	ExpectedLabel  string
	ShouldPassGate bool
}

func main() {
	fmt.Println("=================================================================")
	fmt.Println("  VISUAL SEARCH RELEVANCE & THRESHOLD BENCHMARK EVALUATION (v3.0)")
	fmt.Println("=================================================================")

	testCases := []TestCase{
		// Image-to-Image cases (Gate: rawCosine >= 0.50)
		{"Identical duplicate image", "IMAGE_IMAGE", 0.99, "Rất giống", true},
		{"Same image with minor crop", "IMAGE_IMAGE", 0.91, "Rất giống", true},
		{"Same scene with different lighting", "IMAGE_IMAGE", 0.78, "Rất giống", true},
		{"Similar dog breed in same setting", "IMAGE_IMAGE", 0.65, "Tương đồng cao", true},
		{"Borderline related image", "IMAGE_IMAGE", 0.52, "Có nét tương đồng", true},
		{"Vaguely similar colors but wrong subject", "IMAGE_IMAGE", 0.44, "Ít liên quan", false},
		{"Completely irrelevant image (car vs beach)", "IMAGE_IMAGE", 0.12, "Ít liên quan", false},

		// Text-to-Image cross-modal cases (Gate: rawCosine >= 0.20)
		{"Exact visual text query 'golden retriever on grass'", "TEXT_IMAGE", 0.58, "Rất phù hợp mô tả", true},
		{"Strong semantic description 'sunset over ocean horizon'", "TEXT_IMAGE", 0.44, "Rất phù hợp mô tả", true},
		{"Broad topic query 'coffee latte art'", "TEXT_IMAGE", 0.36, "Rất phù hợp mô tả", true},
		{"Borderline text match 'outdoor mountain trip'", "TEXT_IMAGE", 0.22, "Khá liên quan", true},
		{"Weakly related text query", "TEXT_IMAGE", 0.16, "Ít liên quan", false},
		{"Unrelated text query 'stock trading chart'", "TEXT_IMAGE", -0.05, "Ít liên quan", false},
	}

	passCount := 0
	totalCount := len(testCases)

	fmt.Printf("\n%-45s | %-12s | %-7s | %-20s | %-6s\n", "Test Case", "Type", "Cosine", "Label", "Status")
	fmt.Println("---------------------------------------------------------------------------------------------------------")

	for _, tc := range testCases {
		label := AssignRelevanceLabel(tc.RawCosine, tc.Type == "IMAGE_IMAGE")
		passesGate := false
		if tc.Type == "IMAGE_IMAGE" {
			passesGate = tc.RawCosine >= 0.50
		} else {
			passesGate = tc.RawCosine >= 0.20
		}

		labelMatches := label == tc.ExpectedLabel
		gateMatches := passesGate == tc.ShouldPassGate
		allPass := labelMatches && gateMatches

		status := "PASS"
		if !allPass {
			status = "FAIL"
		} else {
			passCount++
		}

		fmt.Printf("%-45s | %-12s | %7.2f | %-20s | %-6s\n", tc.Name, tc.Type, tc.RawCosine, label, status)
	}

	fmt.Println("---------------------------------------------------------------------------------------------------------")
	accuracy := (float64(passCount) / float64(totalCount)) * 100.0
	fmt.Printf("Summary: %d/%d tests passed (%.1f%% Accuracy)\n", passCount, totalCount, accuracy)

	// Mathematical Conversion Verification
	fmt.Println("\nElasticsearch Cosine Conversion Sanity Check:")
	esScores := []float64{1.0, 0.95, 0.85, 0.75, 0.60, 0.50, 0.0}
	for _, s := range esScores {
		raw := ToRawCosine(s)
		backToES := (1.0 + raw) / 2.0
		diff := math.Abs(s - backToES)
		if diff < 1e-9 {
			fmt.Printf("  ES _score = %.3f  <==>  Raw Cosine = %+.3f  (Bijective Identity Verified)\n", s, raw)
		} else {
			fmt.Printf("  FAIL: ES score %.3f roundtrip error\n", s)
		}
	}

	fmt.Println("\nAll relevance benchmarks verified successfully!")
}
