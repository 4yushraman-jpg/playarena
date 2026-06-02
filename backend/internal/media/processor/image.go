// Package processor handles image validation, MIME detection, resizing,
// JPEG conversion, and SHA-256 content hashing for uploaded media files.
// All operations are synchronous and in-process (no background workers).
// Pure Go — no CGo dependencies.
package processor

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"net/http"

	_ "golang.org/x/image/webp" // register WebP decoder for image.Decode
)

// Allowed MIME types for Phase 11. SVG is explicitly excluded due to XSS risk.
// image/webp is accepted as input; all output is encoded as JPEG.
var allowedMIMETypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// Output MIME type and file suffix for all stored variants.
const (
	OutputMIME    = "image/jpeg"
	OutputSuffix  = ".jpg"
	ThumbSMSuffix = "_sm.jpg"
	ThumbMDSuffix = "_md.jpg"
)

// Variant represents a single processed image variant (full, sm, md).
type Variant struct {
	Data   []byte // JPEG-encoded bytes
	Suffix string // storage key suffix: ".jpg", "_sm.jpg", "_md.jpg"
	Width  int
	Height int
}

// Result holds everything produced by Process for one uploaded file.
type Result struct {
	OriginalFileName string
	DetectedMIME     string // detected MIME of the raw input
	ContentHash      string // SHA-256 hex of raw uploaded bytes (pre-processing)
	Width            int    // dimensions of the full-size output variant
	Height           int
	Full             Variant // full-size JPEG (original dimensions)
	ThumbSM          Variant // 150px wide, proportional height
	ThumbMD          Variant // 400px wide, proportional height
}

// Thumbnail target widths.
const (
	ThumbSMWidth = 150
	ThumbMDWidth = 400
)

// ErrUnsupportedMIME is returned when the detected MIME type is not permitted.
var ErrUnsupportedMIME = errors.New("processor: unsupported MIME type (allowed: jpeg, png, webp, gif)")

// ErrDecodeFailure is returned when the image bytes cannot be decoded.
var ErrDecodeFailure = errors.New("processor: image could not be decoded")

// Process reads the raw file bytes from src, validates MIME type, decodes,
// converts all variants to JPEG, and returns a Result.
//
// The caller must have already bounded src via http.MaxBytesReader so that
// this function never reads more than the configured limit.
func Process(src io.Reader, originalFileName string) (*Result, error) {
	// ── 1. Read full body into memory (bounded by caller's MaxBytesReader) ──
	raw, err := io.ReadAll(src)
	if err != nil {
		return nil, fmt.Errorf("processor: read: %w", err)
	}
	if len(raw) == 0 {
		return nil, errors.New("processor: empty file")
	}

	// ── 2. Server-side MIME detection (first 512 bytes) ──────────────────────
	// Never trust the client-supplied Content-Type header.
	sniff := raw
	if len(sniff) > 512 {
		sniff = sniff[:512]
	}
	detectedMIME := http.DetectContentType(sniff)
	// http.DetectContentType returns "application/octet-stream" for WebP because
	// the standard library doesn't have a built-in WebP sniffer. Supplement
	// with a manual magic-byte check.
	if detectedMIME == "application/octet-stream" && isWebP(raw) {
		detectedMIME = "image/webp"
	}
	baseMIME := baseMIMEType(detectedMIME)
	if !allowedMIMETypes[baseMIME] {
		return nil, ErrUnsupportedMIME
	}

	// ── 3. Compute content hash BEFORE any processing ─────────────────────────
	h := sha256.Sum256(raw)
	contentHash := hex.EncodeToString(h[:])

	// ── 4. Decode image ───────────────────────────────────────────────────────
	// golang.org/x/image/webp is blank-imported above to register the WebP
	// decoder, so image.Decode handles all four supported MIME types.
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		// Fallback: try GIF-specific decoder (image.Decode handles GIF but
		// returns only the first frame; gif.DecodeAll is more explicit).
		if baseMIME == "image/gif" {
			img, err = decodeGIF(raw)
		}
		if err != nil {
			return nil, ErrDecodeFailure
		}
	}

	bounds := img.Bounds()
	origW := bounds.Dx()
	origH := bounds.Dy()

	// ── 5. Convert full-size to JPEG ──────────────────────────────────────────
	fullData, err := encodeJPEG(img)
	if err != nil {
		return nil, fmt.Errorf("processor: encode full-size JPEG: %w", err)
	}

	// ── 6. Generate thumbnail variants ────────────────────────────────────────
	smImg := resizeFit(img, ThumbSMWidth)
	smData, err := encodeJPEG(smImg)
	if err != nil {
		return nil, fmt.Errorf("processor: encode sm thumbnail: %w", err)
	}
	smB := smImg.Bounds()

	mdImg := resizeFit(img, ThumbMDWidth)
	mdData, err := encodeJPEG(mdImg)
	if err != nil {
		return nil, fmt.Errorf("processor: encode md thumbnail: %w", err)
	}
	mdB := mdImg.Bounds()

	return &Result{
		OriginalFileName: originalFileName,
		DetectedMIME:     baseMIME,
		ContentHash:      contentHash,
		Width:            origW,
		Height:           origH,
		Full:             Variant{Data: fullData, Suffix: OutputSuffix, Width: origW, Height: origH},
		ThumbSM:          Variant{Data: smData, Suffix: ThumbSMSuffix, Width: smB.Dx(), Height: smB.Dy()},
		ThumbMD:          Variant{Data: mdData, Suffix: ThumbMDSuffix, Width: mdB.Dx(), Height: mdB.Dy()},
	}, nil
}

// ── encoders ──────────────────────────────────────────────────────────────────

func encodeJPEG(img image.Image) ([]byte, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, toRGBA(img), &jpeg.Options{Quality: 85}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ── decoders ──────────────────────────────────────────────────────────────────

func decodeGIF(raw []byte) (image.Image, error) {
	g, err := gif.DecodeAll(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if len(g.Image) == 0 {
		return nil, errors.New("processor: GIF has no frames")
	}
	return frameToRGBA(g.Image[0]), nil
}

// ── image resize (bilinear, aspect-ratio preserving) ─────────────────────────

// resizeFit scales img so its width equals targetW while preserving aspect
// ratio. Returns src unchanged if the image is already narrower than targetW
// (no upscaling).
func resizeFit(src image.Image, targetW int) image.Image {
	b := src.Bounds()
	srcW := b.Dx()
	srcH := b.Dy()
	if srcW <= targetW {
		return src
	}
	targetH := int(math.Round(float64(srcH) * float64(targetW) / float64(srcW)))
	if targetH < 1 {
		targetH = 1
	}
	return bilinearResize(src, targetW, targetH)
}

// bilinearResize scales src to (dstW, dstH) using bilinear interpolation.
// Result is always *image.RGBA.
func bilinearResize(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	b := src.Bounds()
	srcW := float64(b.Dx())
	srcH := float64(b.Dy())

	for y := 0; y < dstH; y++ {
		gy := (float64(y)+0.5)*srcH/float64(dstH) - 0.5
		y0 := clampInt(int(math.Floor(gy)), 0, b.Dy()-1)
		y1 := clampInt(y0+1, 0, b.Dy()-1)
		dy := gy - float64(y0)

		for x := 0; x < dstW; x++ {
			gx := (float64(x)+0.5)*srcW/float64(dstW) - 0.5
			x0 := clampInt(int(math.Floor(gx)), 0, b.Dx()-1)
			x1 := clampInt(x0+1, 0, b.Dx()-1)
			dx := gx - float64(x0)

			c00 := toRGBAColor(src.At(b.Min.X+x0, b.Min.Y+y0))
			c10 := toRGBAColor(src.At(b.Min.X+x1, b.Min.Y+y0))
			c01 := toRGBAColor(src.At(b.Min.X+x0, b.Min.Y+y1))
			c11 := toRGBAColor(src.At(b.Min.X+x1, b.Min.Y+y1))

			dst.Set(x, y, color.RGBA{
				R: lerp8(lerp8(c00.R, c10.R, dx), lerp8(c01.R, c11.R, dx), dy),
				G: lerp8(lerp8(c00.G, c10.G, dx), lerp8(c01.G, c11.G, dx), dy),
				B: lerp8(lerp8(c00.B, c10.B, dx), lerp8(c01.B, c11.B, dx), dy),
				A: lerp8(lerp8(c00.A, c10.A, dx), lerp8(c01.A, c11.A, dx), dy),
			})
		}
	}
	return dst
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	return dst
}

func frameToRGBA(p *image.Paletted) *image.RGBA {
	b := p.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, p, b.Min, draw.Src)
	return dst
}

func toRGBAColor(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

func lerp8(a, b uint8, t float64) uint8 {
	return uint8(math.Round(float64(a)*(1-t) + float64(b)*t))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// isWebP reports whether data starts with the RIFF/WEBP magic bytes.
func isWebP(data []byte) bool {
	return len(data) >= 12 &&
		data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P'
}

// baseMIMEType strips any parameters (e.g. "; charset=utf-8") from a MIME type.
func baseMIMEType(mime string) string {
	for i, c := range mime {
		if c == ';' || c == ' ' {
			return mime[:i]
		}
	}
	return mime
}

// ── suppress unused import warnings ──────────────────────────────────────────
// The png and gif packages must remain imported for image.Decode to handle
// those formats; they register their decoders via init().
var _ = png.Decode
var _ = gif.DecodeAll
