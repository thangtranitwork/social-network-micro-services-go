package imaging

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"testing"
)

func TestL2Normalize(t *testing.T) {
	vec := []float32{3.0, 4.0}
	normVec := L2Normalize(vec)

	var sum float64
	for _, v := range normVec {
		sum += float64(v * v)
	}
	norm := math.Sqrt(sum)

	if math.Abs(norm-1.0) > 1e-5 {
		t.Errorf("expected unit norm 1.0, got %f", norm)
	}

	if math.Abs(float64(normVec[0])-0.6) > 1e-5 || math.Abs(float64(normVec[1])-0.8) > 1e-5 {
		t.Errorf("expected [0.6, 0.8], got %v", normVec)
	}

	// Zero vector test
	zeroVec := []float32{0, 0, 0}
	normZero := L2Normalize(zeroVec)
	if len(normZero) != 3 || normZero[0] != 0 {
		t.Errorf("zero vector should remain zero")
	}
}

func TestPreprocessImage(t *testing.T) {
	// Create a large 1000x800 test image in memory
	testImg := image.NewRGBA(image.Rect(0, 0, 1000, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1000; x++ {
			testImg.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}

	var rawBuf bytes.Buffer
	if err := png.Encode(&rawBuf, testImg); err != nil {
		t.Fatalf("failed to encode raw test image: %v", err)
	}

	// Preprocess with max dimension 512
	outBytes, mime, err := PreprocessImage(rawBuf.Bytes(), 512)
	if err != nil {
		t.Fatalf("PreprocessImage failed: %v", err)
	}

	if mime != "image/jpeg" {
		t.Errorf("expected mime image/jpeg, got %s", mime)
	}

	// Decode result and verify dimensions
	outImg, err := jpeg.Decode(bytes.NewReader(outBytes))
	if err != nil {
		t.Fatalf("failed to decode preprocessed JPEG: %v", err)
	}

	bounds := outImg.Bounds()
	if bounds.Dx() > 512 || bounds.Dy() > 512 {
		t.Errorf("expected dimensions <= 512, got %dx%d", bounds.Dx(), bounds.Dy())
	}
	if bounds.Dx() != 512 {
		t.Errorf("expected width 512 for 1000x800 source, got %d", bounds.Dx())
	}
	expectedH := int(math.Round(800.0 * 512.0 / 1000.0))
	if math.Abs(float64(bounds.Dy()-expectedH)) > 2 {
		t.Errorf("expected height ~%d, got %d", expectedH, bounds.Dy())
	}
}

func TestApplyOrientation(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 50))
	src.Set(0, 0, color.RGBA{R: 255, A: 255}) // top-left is red

	// Orientation 6: Rotate 90 CW (dimensions should swap to 50x100)
	rotated := ApplyOrientation(src, 6)
	b := rotated.Bounds()
	if b.Dx() != 50 || b.Dy() != 100 {
		t.Errorf("expected 50x100 for 90 CW rotation, got %dx%d", b.Dx(), b.Dy())
	}

	// Top-left in original should be top-right in 90 CW
	topRight := rotated.At(49, 0)
	r, _, _, _ := topRight.RGBA()
	if r == 0 {
		t.Errorf("expected top-right pixel to be red after 90 CW rotation")
	}
}
