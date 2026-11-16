package service

import "testing"

func TestFormatRoundedDistance(t *testing.T) {
	tests := []struct {
		in      float64
		wantKm  float64
		wantLbl string
	}{
		{0.05, 0.1, "< 100m"},
		{0.35, 0.5, "< 500m"},
		{0.82, 1.0, "< 1 km"},
		{2.34, 2.3, "~2.3 km"},
	}
	for _, tc := range tests {
		km, lbl := formatRoundedDistance(tc.in)
		if km != tc.wantKm || lbl != tc.wantLbl {
			t.Fatalf("formatRoundedDistance(%v) = (%v, %q), want (%v, %q)", tc.in, km, lbl, tc.wantKm, tc.wantLbl)
		}
	}
}
