package imaging

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"strconv"
	"strings"

	"siphon/internal/formats"
	"siphon/internal/platform"
	"siphon/internal/siphonerr"
)

// Info is the pre-flight view of an image source.
type Info struct {
	Width    int
	Height   int
	HasAlpha bool
	Metadata *Metadata
	Warnings []string
}

// Options controls encoding.
type Options struct {
	JPEGQuality  int
	LosslessWebP bool
	DropMetadata bool
}

// Probe fully decodes the source (images are small), applies orientation, and
// reports the dimensions, transparency, and carriable metadata. It is the
// pre-flight probe, so it refuses animated containers and corrupt images.
func Probe(ctx context.Context, path string, format formats.Format) (*Info, error) {
	img, meta, warnings, err := Decode(ctx, path, format, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	b := img.Bounds()
	return &Info{
		Width:    b.Dx(),
		Height:   b.Dy(),
		HasAlpha: HasTransparency(img),
		Metadata: meta,
		Warnings: warnings,
	}, nil
}

// Decode returns the oriented image plus the metadata Siphon will carry.
// Animated containers are refused: converting a single frame silently would
// be a data loss the user did not ask for.
func Decode(ctx context.Context, path string, format formats.Format, scratchDir string) (image.Image, *Metadata, []string, error) {
	meta, warnings, err := Extract(path, format)
	if err != nil {
		return nil, nil, nil, err
	}
	if meta.Animated {
		return nil, nil, nil, siphonerr.New(siphonerr.FormatUnsupported,
			"%s is an animated image (APNG or animated WebP); Siphon converts still images only", path)
	}

	img, err := decodePixels(ctx, path, format, scratchDir)
	if err != nil {
		return nil, nil, nil, err
	}

	if o := Orientation(meta.EXIF); o != 1 {
		img = Orient(img, o)
		meta.EXIF = SetOrientation(meta.EXIF, 1)
	}
	return img, meta, warnings, nil
}

func decodePixels(ctx context.Context, path string, format formats.Format, scratchDir string) (image.Image, error) {
	switch format {
	case formats.PNG:
		f, err := platform.OpenReadOnly(path)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot decode PNG %s", path)
		}
		return img, nil
	case formats.JPG:
		f, err := platform.OpenReadOnly(path)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
		}
		defer f.Close()
		img, err := jpeg.Decode(f)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot decode JPEG %s", path)
		}
		return img, nil
	case formats.WEBP:
		intermediate, err := decodeWebP(ctx, path, scratchDir)
		if err != nil {
			return nil, err
		}
		defer platform.RemoveIfExists(intermediate)

		f, err := platform.OpenReadOnly(intermediate)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open decoded WebP")
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot decode WebP pixels")
		}
		return img, nil
	default:
		return nil, siphonerr.New(siphonerr.FormatUnsupported,
			"no image decoder for %s", format)
	}
}

// Encode writes img to destPath in the target format, carrying metadata when
// the format allows it. destPath must already exist as a same-directory temp.
func Encode(ctx context.Context, img image.Image, target formats.Format, opts Options, meta *Metadata, destPath, scratchDir string) error {
	if opts.DropMetadata {
		meta = nil
	}
	switch target {
	case formats.JPG:
		quality := opts.JPEGQuality
		if quality < 1 || quality > 100 {
			quality = 90
		}
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return siphonerr.Wrap(siphonerr.Internal, err, "JPEG encoding failed")
		}
		data, err := AddMetadata(buf.Bytes(), formats.JPG, meta)
		if err != nil {
			return err
		}
		return platform.WriteFile(destPath, data)

	case formats.PNG:
		data, err := encodePNG(img, meta)
		if err != nil {
			return err
		}
		return platform.WriteFile(destPath, data)

	case formats.WEBP:
		// cwebp copies metadata from the intermediate PNG, which is why the
		// intermediate carries the ICC/EXIF/XMP chunks we extracted.
		data, err := encodePNG(img, meta)
		if err != nil {
			return err
		}
		intermediate, err := platform.CreateTemp(scratchDir, "webp-encode", ".png")
		if err != nil {
			return err
		}
		intermediatePath := intermediate.Name()
		intermediate.Close()
		defer platform.RemoveIfExists(intermediatePath)

		if err := platform.WriteFile(intermediatePath, data); err != nil {
			return err
		}
		return runCWebP(ctx, intermediatePath, destPath, opts.LosslessWebP, opts.JPEGQuality)

	default:
		return siphonerr.New(siphonerr.FormatUnsupported, "no image encoder for %s", target)
	}
}

func encodePNG(img image.Image, meta *Metadata) ([]byte, error) {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err, "PNG encoding failed")
	}
	return AddMetadata(buf.Bytes(), formats.PNG, meta)
}

// Verify re-decodes the candidate, checks dimensions, transparency, and the
// metadata classes that were supposed to survive.
func Verify(ctx context.Context, path string, target formats.Format, info *Info, srcMeta *Metadata, dropMetadata bool) ([]string, error) {
	img, meta, warnings, err := Decode(ctx, path, target, filepath.Dir(path))
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot decode the output")
	}
	b := img.Bounds()
	if b.Dx() != info.Width || b.Dy() != info.Height {
		return nil, siphonerr.New(siphonerr.VerifyFailed,
			"output is %dx%d, expected %dx%d", b.Dx(), b.Dy(), info.Width, info.Height)
	}
	if info.HasAlpha && target != formats.JPG && !HasTransparency(img) {
		return nil, siphonerr.New(siphonerr.VerifyFailed,
			"output lost the source transparency")
	}
	if !dropMetadata && srcMeta != nil {
		if len(srcMeta.ICC) > 0 && !bytes.Equal(srcMeta.ICC, meta.ICC) {
			return nil, siphonerr.New(siphonerr.MetadataDrop,
				"the ICC profile did not survive the re-encode")
		}
		if len(srcMeta.EXIF) > 0 {
			if len(meta.EXIF) == 0 {
				return nil, siphonerr.New(siphonerr.MetadataDrop,
					"EXIF did not survive the re-encode")
			}
			if Orientation(meta.EXIF) != 1 {
				return nil, siphonerr.New(siphonerr.VerifyFailed,
					"output EXIF orientation is not normalised")
			}
		}
		if len(srcMeta.XMP) > 0 && len(meta.XMP) == 0 {
			return nil, siphonerr.New(siphonerr.MetadataDrop,
				"XMP did not survive the re-encode")
		}
	}
	return warnings, nil
}

// HasTransparency reports whether any pixel is not fully opaque.
func HasTransparency(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return !o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xFFFF {
				return true
			}
		}
	}
	return false
}

// Composite draws img over an opaque background. Used when a JPEG target
// cannot carry alpha; the composite is always explicit (spec §4.5.1).
func Composite(img image.Image, bg color.Color) *image.RGBA {
	b := img.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), &image.Uniform{C: bg}, image.Point{}, draw.Src)
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Over)
	return dst
}

var backgroundNames = map[string]color.NRGBA{
	"black":   {R: 0, G: 0, B: 0, A: 255},
	"white":   {R: 255, G: 255, B: 255, A: 255},
	"gray":    {R: 128, G: 128, B: 128, A: 255},
	"grey":    {R: 128, G: 128, B: 128, A: 255},
	"silver":  {R: 192, G: 192, B: 192, A: 255},
	"red":     {R: 255, G: 0, B: 0, A: 255},
	"green":   {R: 0, G: 128, B: 0, A: 255},
	"lime":    {R: 0, G: 255, B: 0, A: 255},
	"blue":    {R: 0, G: 0, B: 255, A: 255},
	"yellow":  {R: 255, G: 255, B: 0, A: 255},
	"cyan":    {R: 0, G: 255, B: 255, A: 255},
	"magenta": {R: 255, G: 0, B: 255, A: 255},
	"orange":  {R: 255, G: 165, B: 0, A: 255},
	"purple":  {R: 128, G: 0, B: 128, A: 255},
	"maroon":  {R: 128, G: 0, B: 0, A: 255},
	"olive":   {R: 128, G: 128, B: 0, A: 255},
	"navy":    {R: 0, G: 0, B: 128, A: 255},
	"teal":    {R: 0, G: 128, B: 128, A: 255},
}

// ParseBackground accepts a small CSS-like colour set: names, #rgb, #rrggbb.
func ParseBackground(s string) (color.Color, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	if v == "" {
		return nil, siphonerr.New(siphonerr.Usage, "--bg needs a colour, for example --bg white")
	}
	if c, ok := backgroundNames[v]; ok {
		return c, nil
	}
	if strings.HasPrefix(v, "#") {
		hex := v[1:]
		switch len(hex) {
		case 3:
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		case 6:
		case 8:
			if hex[6:] != "ff" {
				return nil, siphonerr.New(siphonerr.Usage,
					"--bg must be opaque; %q has an alpha component", s)
			}
			hex = hex[:6]
		default:
			return nil, siphonerr.New(siphonerr.Usage,
				"--bg %q is not a colour (use a name or #rrggbb)", s)
		}
		n, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return nil, siphonerr.New(siphonerr.Usage,
				"--bg %q is not a colour (use a name or #rrggbb)", s)
		}
		return color.NRGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, nil
	}
	return nil, siphonerr.New(siphonerr.Usage,
		"--bg %q is not a colour (use a name or #rrggbb)", s)
}
