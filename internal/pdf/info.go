// Package pdf implements the document half of the v0.2 format addendum:
// pdfinfo parsing, render/page geometry, the internal image → PDF writer, and
// the structural verifier. It deliberately does not link poppler.
package pdf

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"siphon/internal/siphonerr"
)

const (
	// DefaultDPI is the PDF → image render resolution (spec §5.1).
	DefaultDPI = 150
	// MinDPI and MaxDPI bound --dpi.
	MinDPI = 36
	MaxDPI = 600
	// DefaultMaxPixels bounds a rendered page (spec §5.1).
	DefaultMaxPixels int64 = 100_000_000
	// MaxImagePixels is the fixed decoded-image cap for image → PDF (§12.2).
	MaxImagePixels int64 = 100_000_000
)

// Box is a PDF page box in points.
type Box struct {
	X1, Y1, X2, Y2 float64
}

// Width returns the box width.
func (b Box) Width() float64 { return b.X2 - b.X1 }

// Height returns the box height.
func (b Box) Height() float64 { return b.Y2 - b.Y1 }

// Valid reports whether the box has positive dimensions.
func (b Box) Valid() bool {
	return isFinite(b.X1) && isFinite(b.Y1) && isFinite(b.X2) && isFinite(b.Y2) &&
		b.Width() > 0 && b.Height() > 0
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// Info is the parsed, single-page view of a PDF source.
type Info struct {
	Pages      int
	MediaBox   Box
	CropBox    Box
	HasCropBox bool
	Rotation   int
	Encrypted  bool
	PageSize   string
	PDFVersion string
	Metadata   []string
}

// PageBox returns the usable page box: CropBox when it is present and usable,
// otherwise MediaBox (spec §6.1 P5).
func (i *Info) PageBox() Box {
	if i.HasCropBox && i.CropBox.Valid() {
		return i.CropBox
	}
	return i.MediaBox
}

// RotatedSize returns the page box dimensions after applying page rotation.
// Rotations of 90 and 270 swap width and height.
func (i *Info) RotatedSize() (float64, float64) {
	b := i.PageBox()
	w, h := b.Width(), b.Height()
	if i.Rotation == 90 || i.Rotation == 270 {
		return h, w
	}
	return w, h
}

// ExpectedDimensions computes the rendered pixel dimensions for dpi:
//
//	width_px  = ceil(width_pt  / 72 × dpi)
//	height_px = ceil(height_pt / 72 × dpi)
//
// The page rotation has already been applied to the box.
func ExpectedDimensions(i *Info, dpi int) (int, int) {
	w, h := i.RotatedSize()
	return int(math.Ceil(w / 72 * float64(dpi))), int(math.Ceil(h / 72 * float64(dpi)))
}

// PixelCount returns the expected rendered pixel count, saturating at
// math.MaxInt64 instead of overflowing.
func PixelCount(i *Info, dpi int) int64 {
	w, h := i.RotatedSize()
	pw := math.Ceil(w / 72 * float64(dpi))
	ph := math.Ceil(h / 72 * float64(dpi))
	if pw <= 0 || ph <= 0 {
		return 0
	}
	if pw > float64(math.MaxInt64) || ph > float64(math.MaxInt64) || pw*ph > float64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(pw * ph)
}

// CheckPixelCap enforces --max-pixels before rendering (spec §6.1 P7).
func CheckPixelCap(i *Info, dpi int, maxPixels int64) error {
	if maxPixels <= 0 {
		return siphonerr.New(siphonerr.Usage, "--max-pixels must be positive")
	}
	pixels := PixelCount(i, dpi)
	if pixels > maxPixels {
		w, h := ExpectedDimensions(i, dpi)
		return siphonerr.New(siphonerr.PDFTooLarge,
			"page renders to %d × %d px at %d DPI (%d pixels), above the %d-pixel cap; lower --dpi or raise --max-pixels",
			w, h, dpi, pixels, maxPixels)
	}
	return nil
}

// Probe runs pdfinfo -box with the contract's safety settings: LC_ALL=C,
// stdin from /dev/null, -upw ” so an empty-password PDF is recognised, and a
// deadline. It classifies encryption and malformed files into the stable PDF
// error codes. A multi-page PDF is returned as Info with Pages > 1; the caller
// applies E_PDF_MULTIPAGE.
func Probe(ctx context.Context, path string) (*Info, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot resolve %s", path)
	}
	args := []string{"-box", "-upw", "", abs}
	cmd := exec.CommandContext(ctx, "pdfinfo", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	devnull, err := os.Open(os.DevNull)
	if err == nil {
		defer devnull.Close()
		cmd.Stdin = devnull
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if runErr != nil {
		return nil, classifyProbeFailure(path, stderr.String(), runErr)
	}
	info, err := ParseInfoOutput(stdout.String())
	if err != nil {
		return nil, err
	}
	info.Metadata = parseMetadataClasses(stdout.String())
	return info, nil
}

func classifyProbeFailure(path, stderr string, cause error) error {
	lower := strings.ToLower(stderr)
	switch {
	case strings.Contains(lower, "password"), strings.Contains(lower, "encrypted"):
		return siphonerr.Wrap(siphonerr.PDFEncrypted, cause,
			"%s is encrypted or requires a password; PDF → image does not support encrypted PDFs", path)
	default:
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = cause.Error()
		}
		return siphonerr.Wrap(siphonerr.PDFInvalid, cause,
			"pdfinfo cannot parse %s: %s", path, firstLine(msg))
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}

// ParseInfoOutput parses `pdfinfo -box` text. It returns an Info even for a
// multi-page document; callers decide the refusal. It rejects a document with
// no parseable page count or no usable MediaBox.
func ParseInfoOutput(out string) (*Info, error) {
	info := &Info{}
	var pagesSet bool

	for _, raw := range strings.Split(out, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch strings.ToLower(key) {
		case "pages":
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return nil, siphonerr.New(siphonerr.PDFInvalid,
					"pdfinfo reported an unusable page count %q", value)
			}
			info.Pages = n
			pagesSet = true
		case "encrypted":
			info.Encrypted = strings.EqualFold(value, "yes")
		case "page size":
			if w, h, ok := parsePageSize(value); ok {
				info.PageSize = fmt.Sprintf("%.0f x %.0f pts", w, h)
			}
		case "page rot":
			n, err := strconv.Atoi(strings.TrimSuffix(value, "°"))
			if err != nil {
				return nil, siphonerr.New(siphonerr.PDFInvalid,
					"pdfinfo reported an unusable page rotation %q", value)
			}
			info.Rotation = n
		case "mediabox":
			b, err := parseBox(value)
			if err != nil {
				return nil, siphonerr.New(siphonerr.PDFInvalid,
					"pdfinfo reported an invalid MediaBox %q", value)
			}
			info.MediaBox = b
		case "cropbox":
			b, err := parseBox(value)
			if err != nil {
				return nil, siphonerr.New(siphonerr.PDFInvalid,
					"pdfinfo reported an invalid CropBox %q", value)
			}
			info.CropBox = b
			info.HasCropBox = true
		case "pdf version":
			info.PDFVersion = value
		}
	}

	if !pagesSet {
		return nil, siphonerr.New(siphonerr.PDFInvalid,
			"pdfinfo did not report a page count")
	}
	if info.Encrypted {
		return info, siphonerr.New(siphonerr.PDFEncrypted,
			"PDF is encrypted; PDF → image does not support encrypted PDFs")
	}
	// A zero-page document is malformed and cannot be rendered. A page count
	// of one or more is valid here; multipage is handled separately so the
	// error can name the actual count.
	if info.Pages < 1 {
		return info, siphonerr.New(siphonerr.PDFInvalid,
			"PDF has %d pages", info.Pages)
	}
	if !info.MediaBox.Valid() {
		return info, siphonerr.New(siphonerr.PDFInvalid,
			"PDF has no usable MediaBox")
	}
	if info.HasCropBox && !info.CropBox.Valid() {
		// A damaged CropBox is not usable; fall back to MediaBox, which is
		// what poppler's -cropbox does for a page without a valid crop box.
		info.HasCropBox = false
	}
	switch info.Rotation {
	case 0, 90, 180, 270:
	default:
		return info, siphonerr.New(siphonerr.PDFInvalid,
			"PDF page rotation %d is not one of 0, 90, 180, 270", info.Rotation)
	}
	if !info.PageBox().Valid() {
		return info, siphonerr.New(siphonerr.PDFInvalid,
			"PDF page box has no positive width and height")
	}
	return info, nil
}

func parsePageSize(v string) (float64, float64, bool) {
	v = strings.TrimSuffix(strings.TrimSpace(v), "pts")
	v = strings.TrimSpace(v)
	parts := strings.SplitN(v, " x ", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	w, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	h, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return w, h, err1 == nil && err2 == nil
}

func parseBox(v string) (Box, error) {
	fields := strings.Fields(v)
	if len(fields) != 4 {
		return Box{}, fmt.Errorf("want four numbers")
	}
	var nums [4]float64
	for i, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil || !isFinite(n) {
			return Box{}, fmt.Errorf("bad number %q", f)
		}
		nums[i] = n
	}
	return Box{X1: nums[0], Y1: nums[1], X2: nums[2], Y2: nums[3]}, nil
}

func parseMetadataClasses(out string) []string {
	// A raster image cannot represent a PDF's interactive or document-level
	// structures. They are always reported as lost (spec §11.1), even when
	// pdfinfo's summary output does not enumerate them.
	classes := []string{"outlines", "links", "annotations", "signatures"}
	seen := map[string]bool{}
	for _, c := range classes {
		seen[c] = true
	}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			classes = append(classes, s)
		}
	}
	for _, raw := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "Title", "Author", "Subject", "Keywords", "Creator", "Producer":
			if value != "" {
				add("PDF Info")
			}
		case "Metadata Stream":
			if strings.EqualFold(value, "yes") {
				add("XMP")
			}
		case "Tagged":
			if strings.EqualFold(value, "yes") {
				add("tagged structure")
			}
		case "JavaScript":
			if strings.EqualFold(value, "yes") {
				add("JavaScript")
			}
		case "Form":
			if !strings.EqualFold(value, "none") && value != "" {
				add("forms")
			}
		}
	}
	return classes
}
