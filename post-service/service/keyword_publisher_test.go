package service

import "testing"

func TestKeywordScoreWeightMatchesLegacyKeywordConstants(t *testing.T) {
	tests := map[string]int{
		"GET_SCORE":     1,
		"LIKE_SCORE":    1,
		"COMMENT_SCORE": 3,
		"SHARE_SCORE":   5,
		"UNKNOWN":       0,
	}

	for score, expected := range tests {
		if got := keywordScoreWeight(score); got != expected {
			t.Fatalf("expected %s to map to %d, got %d", score, expected, got)
		}
	}
}
