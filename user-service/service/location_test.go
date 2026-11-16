package service

import (
	"math"
	"testing"
)

func TestFuzzCoordinatesBounds(t *testing.T) {
	lat, lng := 10.7769, 106.7009
	for i := 0; i < 50; i++ {
		fLat, fLng := fuzzCoordinates(lat, lng, 150)
		if math.Abs(fLat-lat) > 0.003 || math.Abs(fLng-lng) > 0.003 {
			t.Fatalf("fuzzed coordinate (%v, %v) drifted too far from (%v, %v)", fLat, fLng, lat, lng)
		}
	}
	sameLat, sameLng := fuzzCoordinates(lat, lng, 0)
	if sameLat != lat || sameLng != lng {
		t.Fatalf("expected unchanged coordinates when maxMeters=0")
	}
}
