// Package imageproc validates and normalises uploaded profile photos.
//
// Only JPEG, PNG and WebP are accepted, identified by their content signature
// (never by file name or declared type). Dimensions are checked before the
// full decode to stop decompression bombs. The output is a freshly encoded
// JPEG, so EXIF (including GPS location), ICC and any other metadata in the
// original is never stored; EXIF orientation is applied first.
package imageproc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const (
	MaxBytes   = 5 << 20    // 5 MiB upload limit
	MaxSide    = 8000       // reject absurd dimensions before decoding
	MaxPixels  = 24_000_000 // ~96 MB decoded; enough for 6000×4000 phone photos
	OutputSide = 512        // stored photos are at most 512×512
	SiteSide   = 2400       // website (CMS) images are at most 2400px on the long side
)

var (
	ErrTooLarge     = errors.New("photo is larger than 5 MB")
	ErrUnsupported  = errors.New("photo must be a JPEG, PNG or WebP image")
	ErrDimensions   = errors.New("photo dimensions are too large")
	ErrCorrupt      = errors.New("photo could not be read")
	errShortSegment = errors.New("short segment")
)

// Format identifies the image type from its leading bytes.
func Format(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "jpeg"
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		return "png"
	case len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP":
		return "webp"
	}
	return ""
}

// Result is a processed photo ready to store.
type Result struct {
	JPEG          []byte
	Width, Height int
}

// Process validates an upload and returns a metadata-free JPEG.
func Process(data []byte) (Result, error) { return process(data, OutputSide) }

// ProcessSite is Process for public website images, which keep more detail.
func ProcessSite(data []byte) (Result, error) { return process(data, SiteSide) }

func process(data []byte, side int) (Result, error) {
	if len(data) > MaxBytes {
		return Result{}, ErrTooLarge
	}
	format := Format(data)
	var cfg image.Config
	var err error
	switch format {
	case "jpeg":
		cfg, err = jpeg.DecodeConfig(bytes.NewReader(data))
	case "png":
		cfg, err = png.DecodeConfig(bytes.NewReader(data))
	case "webp":
		cfg, err = webp.DecodeConfig(bytes.NewReader(data))
	default:
		return Result{}, ErrUnsupported
	}
	if err != nil {
		return Result{}, ErrCorrupt
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > MaxSide || cfg.Height > MaxSide || cfg.Width*cfg.Height > MaxPixels {
		return Result{}, ErrDimensions
	}
	var img image.Image
	switch format {
	case "jpeg":
		img, err = jpeg.Decode(bytes.NewReader(data))
	case "png":
		img, err = png.Decode(bytes.NewReader(data))
	case "webp":
		img, err = webp.Decode(bytes.NewReader(data))
	}
	if err != nil {
		return Result{}, ErrCorrupt
	}
	if format == "jpeg" {
		img = orient(img, jpegOrientation(data))
	}
	img = fit(img, side)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, flatten(img), &jpeg.Options{Quality: 85}); err != nil {
		return Result{}, err
	}
	b := img.Bounds()
	return Result{JPEG: out.Bytes(), Width: b.Dx(), Height: b.Dy()}, nil
}

// fit scales img down so neither side exceeds max (never scales up).
func fit(img image.Image, max int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= max && h <= max {
		return img
	}
	if w >= h {
		h = h * max / w
		w = max
	} else {
		w = w * max / h
		h = max
	}
	dst := image.NewRGBA(image.Rect(0, 0, max1(w), max1(h)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

func max1(v int) int {
	if v < 1 {
		return 1
	}
	return v
}

// flatten composites transparency onto the site's cream background, since JPEG has no alpha.
func flatten(img image.Image) image.Image {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: color.RGBA{0xF7, 0xF2, 0xEA, 0xFF}}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
	return dst
}

// jpegOrientation reads the EXIF orientation tag (1–8) from a JPEG, or 1.
func jpegOrientation(b []byte) int {
	i := 2
	for i+4 <= len(b) {
		if b[i] != 0xFF {
			return 1
		}
		marker := b[i+1]
		if marker == 0xD9 || marker == 0xDA { // end of image / start of scan: no more metadata
			return 1
		}
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		if size < 2 || i+2+size > len(b) {
			return 1
		}
		seg := b[i+4 : i+2+size]
		if marker == 0xE1 && len(seg) > 6 && string(seg[:6]) == "Exif\x00\x00" {
			if o, err := tiffOrientation(seg[6:]); err == nil {
				return o
			}
			return 1
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) (int, error) {
	if len(t) < 8 {
		return 0, errShortSegment
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 0, errShortSegment
	}
	off := int(bo.Uint32(t[4:8]))
	if off < 8 || off+2 > len(t) {
		return 0, errShortSegment
	}
	n := int(bo.Uint16(t[off : off+2]))
	for e := 0; e < n; e++ {
		p := off + 2 + e*12
		if p+12 > len(t) {
			return 0, errShortSegment
		}
		if bo.Uint16(t[p:p+2]) == 0x0112 {
			v := int(bo.Uint16(t[p+8 : p+10]))
			if v >= 1 && v <= 8 {
				return v, nil
			}
			return 1, nil
		}
	}
	return 1, nil
}

// orient applies an EXIF orientation so the stored photo displays upright.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	swap := o >= 5
	dw, dh := w, h
	if swap {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var dx, dy int
			switch o {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, img.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return dst
}
