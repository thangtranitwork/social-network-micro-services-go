package imaging

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"
)

func init() {
	// Register decoders
	image.RegisterFormat("jpeg", "\xff\xd8", jpeg.Decode, jpeg.DecodeConfig)
	image.RegisterFormat("png", "\x89PNG\r\n\x1a\n", png.Decode, png.DecodeConfig)
}

// L2Normalize normalizes a vector to unit length (Euclidean norm = 1.0).
func L2Normalize(vec []float32) []float32 {
	if len(vec) == 0 {
		return vec
	}
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	if sum <= 0 {
		return vec
	}
	norm := float32(math.Sqrt(sum))
	normalized := make([]float32, len(vec))
	for i, v := range vec {
		normalized[i] = v / norm
	}
	return normalized
}

// PreprocessImage validates, auto-rotates (EXIF), resizes (max 512px) and converts to JPEG.
func PreprocessImage(data []byte, maxDim int) ([]byte, string, error) {
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty image data")
	}

	mimeType := http.DetectContentType(data)
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp":
		// Supported formats
	default:
		// Attempt decode regardless of sniff if header says image
		if len(data) < 4 {
			return nil, "", fmt.Errorf("unsupported image format: %s", mimeType)
		}
	}

	// 1. Check EXIF orientation if JPEG
	orientation := ReadExifOrientation(bytes.NewReader(data))

	// 2. Decode image
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	// 3. Apply EXIF orientation
	if orientation > 1 && orientation <= 8 {
		img = ApplyOrientation(img, orientation)
	}

	// 4. Resize if larger than maxDim
	if maxDim <= 0 {
		maxDim = 512
	}
	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w > maxDim || h > maxDim {
		var newW, newH int
		if w >= h {
			newW = maxDim
			newH = int(float64(h) * float64(maxDim) / float64(w))
		} else {
			newH = maxDim
			newW = int(float64(w) * float64(maxDim) / float64(h))
		}
		if newW < 1 {
			newW = 1
		}
		if newH < 1 {
			newH = 1
		}
		img = ResizeBilinear(img, newW, newH)
	}

	// 5. Encode to JPEG (85% quality)
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	if err != nil {
		return nil, "", fmt.Errorf("failed to encode processed JPEG: %w", err)
	}

	return buf.Bytes(), "image/jpeg", nil
}

// ReadExifOrientation extracts EXIF orientation from JPEG stream.
func ReadExifOrientation(r io.Reader) int {
	var header [2]byte
	if _, err := io.ReadFull(r, header[:]); err != nil || header[0] != 0xFF || header[1] != 0xD8 {
		return 1 // Not JPEG or read error
	}

	for {
		var marker [2]byte
		if _, err := io.ReadFull(r, marker[:]); err != nil {
			break
		}
		if marker[0] != 0xFF {
			break
		}

		// Skip padding bytes
		for marker[1] == 0xFF {
			if _, err := io.ReadFull(r, marker[1:]); err != nil {
				return 1
			}
		}

		if marker[1] == 0xD9 || marker[1] == 0xDA { // EOI or SOS
			break
		}

		var lengthBytes [2]byte
		if _, err := io.ReadFull(r, lengthBytes[:]); err != nil {
			break
		}
		length := int(binary.BigEndian.Uint16(lengthBytes[:])) - 2
		if length < 0 {
			break
		}

		// APP1 Marker containing Exif
		if marker[1] == 0xE1 {
			data := make([]byte, length)
			if _, err := io.ReadFull(r, data); err != nil {
				return 1
			}
			if len(data) > 6 && string(data[:4]) == "Exif" && data[4] == 0 && data[5] == 0 {
				return parseTiffOrientation(data[6:])
			}
			continue
		}

		// Discard other segments
		if _, err := io.CopyN(io.Discard, r, int64(length)); err != nil {
			break
		}
	}
	return 1
}

func parseTiffOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}

	var byteOrder binary.ByteOrder
	if tiff[0] == 'I' && tiff[1] == 'I' {
		byteOrder = binary.LittleEndian
	} else if tiff[0] == 'M' && tiff[1] == 'M' {
		byteOrder = binary.BigEndian
	} else {
		return 1
	}

	if byteOrder.Uint16(tiff[2:4]) != 42 {
		return 1
	}

	offset := int(byteOrder.Uint32(tiff[4:8]))
	if offset < 8 || offset+2 > len(tiff) {
		return 1
	}

	entries := int(byteOrder.Uint16(tiff[offset : offset+2]))
	curr := offset + 2

	for i := 0; i < entries; i++ {
		if curr+12 > len(tiff) {
			break
		}
		tag := byteOrder.Uint16(tiff[curr : curr+2])
		if tag == 0x0112 { // Orientation Tag
			return int(byteOrder.Uint16(tiff[curr+8 : curr+10]))
		}
		curr += 12
	}
	return 1
}

// ApplyOrientation transforms an image based on EXIF tag (1 to 8).
func ApplyOrientation(src image.Image, orientation int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	switch orientation {
	case 2: // Flip Horizontal
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(w-1-x, y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 3: // Rotate 180
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(w-1-x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 4: // Flip Vertical
		dst := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(x, h-1-y, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 5: // Transpose (Rotate 90 CW + Flip Horizontal)
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(y, x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 6: // Rotate 90 CW
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(h-1-y, x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 7: // Transverse
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(h-1-y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	case 8: // Rotate 270 CW (90 CCW)
		dst := image.NewRGBA(image.Rect(0, 0, h, w))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				dst.Set(y, w-1-x, src.At(b.Min.X+x, b.Min.Y+y))
			}
		}
		return dst
	default:
		return src
	}
}

// ResizeBilinear resizes src to dstW x dstH using bilinear interpolation.
func ResizeBilinear(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	if srcW == 0 || srcH == 0 || dstW == 0 || dstH == 0 {
		return dst
	}

	scaleX := float64(srcW) / float64(dstW)
	scaleY := float64(srcH) / float64(dstH)

	for y := 0; y < dstH; y++ {
		srcY := (float64(y)+0.5)*scaleY - 0.5
		y0 := int(math.Floor(srcY))
		y1 := y0 + 1
		yWeight := srcY - float64(y0)

		if y0 < 0 {
			y0 = 0
		}
		if y1 >= srcH {
			y1 = srcH - 1
		}

		for x := 0; x < dstW; x++ {
			srcX := (float64(x)+0.5)*scaleX - 0.5
			x0 := int(math.Floor(srcX))
			x1 := x0 + 1
			xWeight := srcX - float64(x0)

			if x0 < 0 {
				x0 = 0
			}
			if x1 >= srcW {
				x1 = srcW - 1
			}

			c00 := colorRGBA(src.At(bounds.Min.X+x0, bounds.Min.Y+y0))
			c10 := colorRGBA(src.At(bounds.Min.X+x1, bounds.Min.Y+y0))
			c01 := colorRGBA(src.At(bounds.Min.X+x0, bounds.Min.Y+y1))
			c11 := colorRGBA(src.At(bounds.Min.X+x1, bounds.Min.Y+y1))

			topR := (1-xWeight)*float64(c00.R) + xWeight*float64(c10.R)
			topG := (1-xWeight)*float64(c00.G) + xWeight*float64(c10.G)
			topB := (1-xWeight)*float64(c00.B) + xWeight*float64(c10.B)
			topA := (1-xWeight)*float64(c00.A) + xWeight*float64(c10.A)

			bottomR := (1-xWeight)*float64(c01.R) + xWeight*float64(c11.R)
			bottomG := (1-xWeight)*float64(c01.G) + xWeight*float64(c11.G)
			bottomB := (1-xWeight)*float64(c01.B) + xWeight*float64(c11.B)
			bottomA := (1-xWeight)*float64(c01.A) + xWeight*float64(c11.A)

			finalR := uint8(math.Round((1-yWeight)*topR + yWeight*bottomR))
			finalG := uint8(math.Round((1-yWeight)*topG + yWeight*bottomG))
			finalB := uint8(math.Round((1-yWeight)*topB + yWeight*bottomB))
			finalA := uint8(math.Round((1-yWeight)*topA + yWeight*bottomA))

			dst.SetRGBA(x, y, color.RGBA{R: finalR, G: finalG, B: finalB, A: finalA})
		}
	}
	return dst
}

func colorRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

// ConvertImageToRGBA converts any image.Image to *image.RGBA
func ConvertImageToRGBA(src image.Image) *image.RGBA {
	if rgba, ok := src.(*image.RGBA); ok {
		return rgba
	}
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), src, b.Min, draw.Src)
	return dst
}
