package imageproc

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

func solid(w, h int, c color.Color) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

// jpegWithExif encodes img and inserts an APP1 Exif segment carrying the
// orientation tag and a fake GPS marker string.
func jpegWithExif(t *testing.T, img image.Image, orientation uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0}
	entry := make([]byte, 12)
	binary.LittleEndian.PutUint16(entry[0:], 0x0112)
	binary.LittleEndian.PutUint16(entry[2:], 3)
	binary.LittleEndian.PutUint32(entry[4:], 1)
	binary.LittleEndian.PutUint16(entry[8:], orientation)
	tiff = append(tiff, entry...)
	tiff = append(tiff, 0, 0, 0, 0)
	tiff = append(tiff, []byte("GPSLatitude=24.4539;GPSLongitude=54.3773")...)
	payload := append([]byte("Exif\x00\x00"), tiff...)
	seg := []byte{0xFF, 0xE1, 0, 0}
	binary.BigEndian.PutUint16(seg[2:], uint16(len(payload)+2))
	seg = append(seg, payload...)
	b := buf.Bytes()
	return append(append(append([]byte{}, b[:2]...), seg...), b[2:]...)
}

func TestJPEGStripsMetadataAndAppliesOrientation(t *testing.T) {
	src := jpegWithExif(t, solid(1200, 600, color.RGBA{200, 50, 50, 255}), 6) // 6 = rotate 90° clockwise
	if jpegOrientation(src) != 6 {
		t.Fatal("fixture orientation not detected")
	}
	if !bytes.Contains(src, []byte("GPSLatitude")) {
		t.Fatal("fixture lacks GPS marker")
	}
	res, err := Process(src)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(res.JPEG, []byte("Exif")) || bytes.Contains(res.JPEG, []byte("GPSLatitude")) {
		t.Fatal("metadata survived processing")
	}
	if res.Width != 256 || res.Height != 512 {
		t.Fatalf("orientation/resize: got %dx%d, want 256x512", res.Width, res.Height)
	}
	if Format(res.JPEG) != "jpeg" {
		t.Fatal("output must be JPEG")
	}
}

func TestPNGWithAlphaAndSmallImagesNotUpscaled(t *testing.T) {
	var buf bytes.Buffer
	_ = png.Encode(&buf, solid(100, 80, color.RGBA{0, 0, 0, 0}))
	res, err := Process(buf.Bytes())
	if err != nil || res.Width != 100 || res.Height != 80 {
		t.Fatalf("png: %v %dx%d", err, res.Width, res.Height)
	}
	out, _ := jpeg.Decode(bytes.NewReader(res.JPEG))
	r, g, b, _ := out.At(50, 40).RGBA()
	if r>>8 < 0xE0 || g>>8 < 0xE0 || b>>8 < 0xD0 {
		t.Fatalf("transparent pixels should be flattened onto cream, got %d,%d,%d", r>>8, g>>8, b>>8)
	}
}

func TestWebP(t *testing.T) {
	// 1×1 lossless WebP.
	data, _ := base64.StdEncoding.DecodeString("UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA==")
	if Format(data) != "webp" {
		t.Fatal("webp signature")
	}
	if _, err := Process(data); err != nil {
		t.Fatalf("webp: %v", err)
	}
}

func pngHeader(w, h uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], w)
	binary.BigEndian.PutUint32(ihdr[4:], h)
	ihdr[8], ihdr[9] = 8, 2
	chunk := append([]byte("IHDR"), ihdr...)
	out := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n', 0, 0, 0, 13}
	out = append(out, chunk...)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint32(crc, crc32.ChecksumIEEE(chunk))
	return append(out, crc...)
}

func TestRejections(t *testing.T) {
	var g bytes.Buffer
	_ = gif.Encode(&g, solid(4, 4, color.White), nil)
	var good bytes.Buffer
	_ = jpeg.Encode(&good, solid(50, 50, color.White), nil)
	cases := map[string]struct {
		data []byte
		want error
	}{
		"gif":                {g.Bytes(), ErrUnsupported},
		"svg":                {[]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), ErrUnsupported},
		"html named .jpg":    {[]byte("<html><script>alert(1)</script></html>"), ErrUnsupported},
		"empty":              {nil, ErrUnsupported},
		"truncated jpeg":     {good.Bytes()[:40], ErrCorrupt},
		"too wide":           {pngHeader(9000, 10), ErrDimensions},
		"decompression bomb": {pngHeader(7000, 7000), ErrDimensions},
		"too many bytes":     {append([]byte{0xFF, 0xD8, 0xFF}, make([]byte, MaxBytes)...), ErrTooLarge},
	}
	for name, c := range cases {
		if _, err := Process(c.data); !errors.Is(err, c.want) {
			t.Errorf("%s: got %v, want %v", name, err, c.want)
		}
	}
}

func TestOrientationMalformedExifIsIgnored(t *testing.T) {
	bad := []byte{0xFF, 0xD8, 0xFF, 0xE1, 0x00, 0x08, 'E', 'x', 'i', 'f', 0, 0}
	if jpegOrientation(bad) != 1 {
		t.Fatal("malformed EXIF must fall back to orientation 1")
	}
	for o := 1; o <= 8; o++ {
		img := orient(solid(3, 2, color.White), o)
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		if (o >= 5 && (w != 2 || h != 3)) || (o < 5 && (w != 3 || h != 2)) {
			t.Errorf("orientation %d: %dx%d", o, w, h)
		}
	}
}
