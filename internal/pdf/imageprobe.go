package pdf

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"strconv"
	"strings"

	"siphon/internal/formats"
	"siphon/internal/imaging"
	"siphon/internal/siphonerr"
)

var pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

// ImageInfo is everything the image → PDF writer needs from a PNG or JPEG,
// plus the facts the pre-flight gate reports in the plan.
type ImageInfo struct {
	Format      formats.Format
	Width       int
	Height      int
	Gray        bool
	HasAlpha    bool
	BPC         int
	DPI         float64
	Orientation int

	Pixels      image.Image // decoded PNG pixels; nil for JPEG
	JPEG        []byte      // original JPEG bitstream; nil for PNG
	Progressive bool

	ICC  []byte
	EXIF []byte
	XMP  []byte

	EXIFInfo EXIFInfo
	Warnings []string
}

// ProbeImage decodes an image for image → PDF. It refuses 16-bit images,
// CMYK JPEGs, animated images, and images above the fixed pixel cap.
func ProbeImage(ctx context.Context, path string, format formats.Format) (*ImageInfo, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	switch format {
	case formats.PNG:
		return probePNG(path)
	case formats.JPG:
		return probeJPEG(path)
	default:
		return nil, siphonerr.New(siphonerr.FormatUnsupported,
			"image → PDF does not support %s sources", format)
	}
}

func probePNG(path string) (*ImageInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot read %s", path)
	}
	w, h, depth, colorType, err := parsePNGHeader(data)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "%s is not a usable PNG", path)
	}
	if err := checkPNGDepth(depth, colorType); err != nil {
		return nil, siphonerr.Wrap(siphonerr.FormatUnsupported, err, "%s cannot be embedded", path)
	}
	if int64(w)*int64(h) > MaxImagePixels {
		return nil, siphonerr.New(siphonerr.SourceInvalid,
			"%s is %d × %d px, above the %d-pixel limit", path, w, h, MaxImagePixels)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot decode PNG %s", path)
	}
	b := img.Bounds()
	if b.Dx() != w || b.Dy() != h {
		return nil, siphonerr.New(siphonerr.SourceCorrupt,
			"%s header says %dx%d but the pixels are %dx%d", path, w, h, b.Dx(), b.Dy())
	}

	meta, warnings, err := imaging.Extract(path, formats.PNG)
	if err != nil {
		return nil, err
	}
	if meta.Animated {
		return nil, siphonerr.New(siphonerr.FormatUnsupported,
			"%s is an animated PNG; image → PDF converts still images only", path)
	}

	info := &ImageInfo{
		Format:      formats.PNG,
		Width:       w,
		Height:      h,
		Gray:        colorType == 0 || colorType == 4,
		BPC:         8,
		DPI:         pngDPI(data),
		Pixels:      img,
		ICC:         meta.ICC,
		EXIF:        meta.EXIF,
		XMP:         meta.XMP,
		Warnings:    warnings,
		Orientation: imaging.Orientation(meta.EXIF),
	}
	info.HasAlpha = hasNonOpaqueAlpha(img)
	info.EXIFInfo = ParseEXIF(meta.EXIF)
	return info, nil
}

func probeJPEG(path string) (*ImageInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot read %s", path)
	}
	j, err := parseJPEGHeader(data)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "%s is not a usable JPEG", path)
	}
	if int64(j.width)*int64(j.height) > MaxImagePixels {
		return nil, siphonerr.New(siphonerr.SourceInvalid,
			"%s is %d × %d px, above the %d-pixel limit", path, j.width, j.height, MaxImagePixels)
	}
	if j.precision != 8 {
		return nil, siphonerr.New(siphonerr.FormatUnsupported,
			"%s is a %d-bit JPEG; image → PDF supports 8-bit JPEGs only", path, j.precision)
	}
	switch j.components {
	case 1, 3:
	case 4:
		return nil, siphonerr.New(siphonerr.CodecIncompatible,
			"%s is a CMYK JPEG; image → PDF supports RGB and grayscale JPEGs only", path)
	default:
		return nil, siphonerr.New(siphonerr.CodecIncompatible,
			"%s has %d JPEG components; image → PDF supports RGB and grayscale only", path, j.components)
	}

	// I2: the header alone is not enough. Decode the pixels once so a
	// truncated or corrupt scan is refused before any write.
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot decode JPEG %s", path)
	}
	if b := img.Bounds(); b.Dx() != j.width || b.Dy() != j.height {
		return nil, siphonerr.New(siphonerr.SourceCorrupt,
			"%s header says %dx%d but the pixels are %dx%d", path, j.width, j.height, b.Dx(), b.Dy())
	}

	meta, warnings, err := imaging.Extract(path, formats.JPG)
	if err != nil {
		return nil, err
	}
	info := &ImageInfo{
		Format:      formats.JPG,
		Width:       j.width,
		Height:      j.height,
		Gray:        j.components == 1,
		BPC:         8,
		DPI:         j.dpi,
		JPEG:        append([]byte(nil), data...),
		Progressive: j.progressive,
		ICC:         meta.ICC,
		EXIF:        meta.EXIF,
		XMP:         meta.XMP,
		Warnings:    warnings,
		Orientation: imaging.Orientation(meta.EXIF),
	}
	info.EXIFInfo = ParseEXIF(meta.EXIF)
	return info, nil
}

func parsePNGHeader(data []byte) (w, h int, depth, colorType byte, err error) {
	if len(data) < 33 || !bytes.Equal(data[:8], pngSignature) {
		return 0, 0, 0, 0, fmt.Errorf("bad PNG signature")
	}
	if string(data[12:16]) != "IHDR" {
		return 0, 0, 0, 0, fmt.Errorf("first chunk is not IHDR")
	}
	w = int(binary.BigEndian.Uint32(data[16:20]))
	h = int(binary.BigEndian.Uint32(data[20:24]))
	depth = data[24]
	colorType = data[25]
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("invalid PNG dimensions %dx%d", w, h)
	}
	if data[26] != 0 || data[27] != 0 {
		return 0, 0, 0, 0, fmt.Errorf("unsupported PNG compression/filter method")
	}
	return w, h, depth, colorType, nil
}

func checkPNGDepth(depth, colorType byte) error {
	switch colorType {
	case 0:
		if depth == 16 {
			return fmt.Errorf("16-bit grayscale PNG is not supported")
		}
		if depth != 1 && depth != 2 && depth != 4 && depth != 8 {
			return fmt.Errorf("unsupported PNG bit depth %d", depth)
		}
	case 2:
		if depth == 16 {
			return fmt.Errorf("16-bit RGB PNG is not supported")
		}
		if depth != 8 {
			return fmt.Errorf("unsupported PNG bit depth %d", depth)
		}
	case 3:
		if depth != 1 && depth != 2 && depth != 4 && depth != 8 {
			return fmt.Errorf("unsupported palette bit depth %d", depth)
		}
	case 4, 6:
		if depth == 16 {
			return fmt.Errorf("16-bit PNG is not supported")
		}
		if depth != 8 {
			return fmt.Errorf("unsupported PNG bit depth %d", depth)
		}
	default:
		return fmt.Errorf("unsupported PNG color type %d", colorType)
	}
	return nil
}

func pngDPI(data []byte) float64 {
	pos := 8
	for pos+8 <= len(data) {
		length := int(binary.BigEndian.Uint32(data[pos : pos+4]))
		typ := string(data[pos+4 : pos+8])
		if length < 0 || pos+12+length > len(data) {
			break
		}
		if typ == "pHYs" && length >= 9 {
			p := data[pos+8 : pos+8+9]
			if p[8] == 1 { // pixels per metre
				x := binary.BigEndian.Uint32(p[0:4])
				y := binary.BigEndian.Uint32(p[4:8])
				dpi := float64(minU32(x, y)) * 0.0254
				if dpi >= 36 && dpi <= 1200 {
					return dpi
				}
			}
			return 96
		}
		if typ == "IDAT" || typ == "IEND" {
			break
		}
		pos += 12 + length
	}
	return 96
}

func minU32(a, b uint32) uint32 {
	if a < b {
		return a
	}
	return b
}

type jpegHeader struct {
	width       int
	height      int
	precision   int
	components  int
	progressive bool
	dpi         float64
}

func parseJPEGHeader(data []byte) (*jpegHeader, error) {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("bad JPEG signature")
	}
	j := &jpegHeader{dpi: 96}
	pos := 2
	sawSOF := false
	for pos+1 < len(data) {
		if data[pos] != 0xFF {
			return nil, fmt.Errorf("malformed JPEG marker at offset %d", pos)
		}
		for pos < len(data) && data[pos] == 0xFF {
			pos++
		}
		if pos >= len(data) {
			break
		}
		marker := data[pos]
		pos++
		switch marker {
		case 0xD9: // EOI
			if !sawSOF {
				return nil, fmt.Errorf("JPEG has no frame header")
			}
			return j, nil
		case 0x01, 0xD0, 0xD1, 0xD2, 0xD3, 0xD4, 0xD5, 0xD6, 0xD7:
			continue
		case 0xDA: // start of scan, frame header must precede it
			if !sawSOF {
				return nil, fmt.Errorf("JPEG scan appears before its frame header")
			}
			return j, nil
		}
		if pos+2 > len(data) {
			return nil, fmt.Errorf("truncated JPEG segment")
		}
		size := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if size < 2 || pos+size > len(data) {
			return nil, fmt.Errorf("invalid JPEG segment length %d", size)
		}
		payload := data[pos+2 : pos+size]
		switch {
		case marker == 0xE0 && bytes.HasPrefix(payload, []byte("JFIF\x00")):
			j.dpi = jfifDPI(payload)
		case isSOF(marker):
			if len(payload) < 6 {
				return nil, fmt.Errorf("truncated JPEG frame header")
			}
			j.precision = int(payload[0])
			j.height = int(binary.BigEndian.Uint16(payload[1:3]))
			j.width = int(binary.BigEndian.Uint16(payload[3:5]))
			j.components = int(payload[5])
			j.progressive = marker == 0xC2
			if j.width <= 0 || j.height <= 0 || j.components < 1 {
				return nil, fmt.Errorf("invalid JPEG frame dimensions")
			}
			sawSOF = true
		}
		pos += size
	}
	if !sawSOF {
		return nil, fmt.Errorf("JPEG has no frame header")
	}
	return j, nil
}

func isSOF(marker byte) bool {
	switch marker {
	case 0xC0, 0xC1, 0xC2, 0xC3, 0xC5, 0xC6, 0xC7,
		0xC9, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF:
		return true
	}
	return false
}

func jfifDPI(payload []byte) float64 {
	// Identifier (5) + version (2) + units (1) + Xdensity (2) + Ydensity (2)
	if len(payload) < 12 {
		return 96
	}
	units := payload[7]
	var x, y uint16
	x = binary.BigEndian.Uint16(payload[8:10])
	y = binary.BigEndian.Uint16(payload[10:12])
	var dpi float64
	switch units {
	case 1:
		dpi = float64(minU16(x, y))
	case 2:
		dpi = float64(minU16(x, y)) * 2.54
	default:
		return 96
	}
	if dpi < 36 || dpi > 1200 {
		return 96
	}
	return dpi
}

func minU16(a, b uint16) uint16 {
	if a < b {
		return a
	}
	return b
}

// HasNonOpaqueAlpha reports whether any pixel has alpha < 255.
func hasNonOpaqueAlpha(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xFFFF {
				return true
			}
		}
	}
	return false
}

// ValidateICC checks that an ICC profile is structurally embeddable and its
// color space matches the image channels (1 gray, 3 RGB). CMYK profiles are
// rejected because CMYK JPEG input is refused.
func ValidateICC(profile []byte, channels int) error {
	if len(profile) < 128 {
		return fmt.Errorf("ICC profile is shorter than 128 bytes")
	}
	declared := int(binary.BigEndian.Uint32(profile[0:4]))
	if declared < 128 || declared > len(profile) {
		return fmt.Errorf("ICC profile size field %d does not match %d bytes", declared, len(profile))
	}
	if string(profile[36:40]) != "acsp" {
		return fmt.Errorf("ICC profile is missing the 'acsp' signature")
	}
	cs := string(profile[16:20])
	switch channels {
	case 1:
		if cs != "GRAY" {
			return fmt.Errorf("ICC profile color space %q is not GRAY", strings.TrimSpace(cs))
		}
	case 3:
		if cs != "RGB " && cs != "RGB" {
			return fmt.Errorf("ICC profile color space %q is not RGB", strings.TrimSpace(cs))
		}
	default:
		return fmt.Errorf("unsupported channel count %d", channels)
	}
	return nil
}

// UntranslatableEXIF lists EXIF fields that have no PDF equivalent. The
// orientation, DateTimeOriginal, Make, and Model are translated by the writer.
func UntranslatableEXIF(info ImageInfo) []string {
	return info.EXIFInfo.Untranslatable
}

// JpegInfo is exposed for tests that need to inspect the marker parser without
// inventing a full encoder.
type JPEGHeader struct {
	Width       int
	Height      int
	Precision   int
	Components  int
	Progressive bool
	DPI         float64
}

// ProbeJPEGHeader parses only the marker stream. It is used by tests and by
// the writer when validating a direct-embed JPEG.
func ProbeJPEGHeader(data []byte) (JPEGHeader, error) {
	j, err := parseJPEGHeader(data)
	if err != nil {
		return JPEGHeader{}, err
	}
	return JPEGHeader{
		Width:       j.width,
		Height:      j.height,
		Precision:   j.precision,
		Components:  j.components,
		Progressive: j.progressive,
		DPI:         j.dpi,
	}, nil
}

// DecodeImageHeader is a light PNG/JPEG probe used by tests.
func DecodeImageHeader(data []byte, format formats.Format) (int, int, error) {
	switch format {
	case formats.PNG:
		w, h, _, _, err := parsePNGHeader(data)
		return w, h, err
	case formats.JPG:
		j, err := parseJPEGHeader(data)
		if err != nil {
			return 0, 0, err
		}
		return j.width, j.height, nil
	}
	return 0, 0, fmt.Errorf("unsupported image format %s", format)
}

// ReadAllLimit is a small helper for tests that want a bounded read.
func ReadAllLimit(r io.Reader, n int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, n))
}

// FormatFloat renders a PDF number without trailing zeros.
func FormatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
