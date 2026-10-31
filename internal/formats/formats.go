// Package formats owns the format table, mode resolution, and the mapping
// from a (source, target, mode) triple to an engine.
package formats

import (
	"path/filepath"
	"strings"

	"siphon/internal/siphonerr"
)

// Format is a media container, document, or image encoding.
type Format string

const (
	WAV  Format = "wav"
	MP3  Format = "mp3"
	MKV  Format = "mkv"
	MP4  Format = "mp4"
	MOV  Format = "mov"
	WEBM Format = "webm"
	GIF  Format = "gif"
	PNG  Format = "png"
	JPG  Format = "jpg"
	WEBP Format = "webp"
	PDF  Format = "pdf"
)

// Mode is a space/crash-safety trade-off.
type Mode string

const (
	ModeAuto     Mode = "auto"
	ModeSafe     Mode = "safe"
	ModeRelocate Mode = "relocate"
	ModeReclaim  Mode = "reclaim"
)

// EngineKind names an engine implementation.
type EngineKind string

const (
	EngineAtomicCopy EngineKind = "atomiccopy"
	EngineSliding    EngineKind = "sliding"
	EngineRelocate   EngineKind = "relocate"
	EngineRemux      EngineKind = "remux"
	EnginePDFRender  EngineKind = "pdfrender"
	EnginePDFWrite   EngineKind = "pdfwrite"
	EngineMedia      EngineKind = "media"
)

// Pair describes one supported conversion.
type Pair struct {
	Source     Format
	Target     Format
	Default    Mode
	Modes      []Mode
	Engine     EngineKind
	Lossy      bool
	OutputKind string
}

var pairs = []Pair{
	// v1.7 pairs. MKV → MP4 keeps its relocate default and classic-MP4 safe
	// mode; everything else in this table is safe only.
	{WAV, MP3, ModeSafe, []Mode{ModeSafe, ModeReclaim}, EngineSliding, true, "MP3"},
	{MKV, MP4, ModeRelocate, []Mode{ModeRelocate, ModeSafe}, EngineRelocate, false, "fragmented MP4"},
	{PNG, JPG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "JPEG"},
	{JPG, PNG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "PNG"},
	{PNG, WEBP, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "WebP"},
	{WEBP, PNG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "PNG"},
	{JPG, WEBP, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "WebP"},
	{WEBP, JPG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "JPEG"},

	// v0.2 document/image pairs (spec §3.1).
	{PDF, PNG, ModeSafe, []Mode{ModeSafe}, EnginePDFRender, false, "8-bit sRGB PNG"},
	{PDF, JPG, ModeSafe, []Mode{ModeSafe}, EnginePDFRender, true, "8-bit sRGB JPEG"},
	{PNG, PDF, ModeSafe, []Mode{ModeSafe}, EnginePDFWrite, false, "PDF 1.7, one page"},
	{JPG, PDF, ModeSafe, []Mode{ModeSafe}, EnginePDFWrite, false, "PDF 1.7, one page"},

	// v0.2 video container pairs (spec §3.2). MKV → MP4 is above and is not
	// reimplemented; these complete the reverse directions.
	{MP4, MKV, ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MKV"},
	{MOV, MKV, ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MKV"},
	{MKV, MOV, ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MOV"},
	{WEBM, MP4, ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MP4"},
	{MP4, WEBM, ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "WebM"},
	{GIF, MP4, ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "classic MP4, no audio"},
	{MP4, GIF, ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "animated GIF, no audio"},

	// v0.2 audio extraction pairs (spec §3.3). MP3 → video is deliberately
	// absent: adding a still image is authoring, not a conversion inverse.
	{MP4, MP3, ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
	{MKV, MP3, ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
	{MOV, MP3, ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
	{WEBM, MP3, ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
}

// OutputKindFor returns the concrete output kind for a mode. Video is the only
// pair whose container shape depends on the mode: relocate writes fragmented
// MP4, safe writes classic MP4 (spec §3.1).
func (p Pair) OutputKindFor(m Mode) string {
	if p.Source == MKV && p.Target == MP4 && m == ModeSafe {
		return "MP4"
	}
	return p.OutputKind
}

// Parse normalizes a user-supplied format name (".jpeg", "JPEG", "jpg").
func Parse(s string) (Format, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, ".")
	switch s {
	case "wav", "wave":
		return WAV, true
	case "mp3":
		return MP3, true
	case "mkv", "matroska":
		return MKV, true
	case "mp4", "m4v":
		return MP4, true
	case "mov", "qt", "quicktime":
		return MOV, true
	case "webm":
		return WEBM, true
	case "gif":
		return GIF, true
	case "png":
		return PNG, true
	case "jpg", "jpeg", "jpe":
		return JPG, true
	case "webp":
		return WEBP, true
	case "pdf":
		return PDF, true
	}
	return "", false
}

// FromPath infers the format from a filename extension.
func FromPath(path string) (Format, error) {
	ext := filepath.Ext(path)
	if ext == "" {
		return "", siphonerr.New(siphonerr.FormatUnsupported, "%s has no file extension", path)
	}
	f, ok := Parse(ext)
	if !ok {
		return "", siphonerr.New(siphonerr.FormatUnsupported, "unsupported extension %q", ext)
	}
	return f, nil
}

// Ext returns the canonical extension without a dot.
func (f Format) Ext() string { return string(f) }

// String implements fmt.Stringer.
func (f Format) String() string { return string(f) }

// IsImage reports whether f is a still-image format in the image engine.
func IsImage(f Format) bool { return f == PNG || f == JPG || f == WEBP }

// IsVideo reports whether f is a video container. GIF is included because the
// v0.2 pair table treats it as a video source/target.
func IsVideo(f Format) bool {
	switch f {
	case MP4, MOV, MKV, WEBM, GIF:
		return true
	}
	return false
}

// IsPDFImagePair reports whether the pair is a single-page PDF → image render.
func IsPDFImagePair(p Pair) bool { return p.Source == PDF && (p.Target == PNG || p.Target == JPG) }

// IsImagePDFPair reports whether the pair is image → one-page PDF.
func IsImagePDFPair(p Pair) bool {
	return (p.Source == PNG || p.Source == JPG) && p.Target == PDF
}

// IsContainerRemux reports whether the pair is one of the safe container
// remuxes. MKV → MOV and MP4/MOV → MKV are new; the WebM/MP4 pair is handled
// separately because it may need an explicit transcode.
func IsContainerRemux(p Pair) bool {
	switch {
	case p.Source == MP4 && p.Target == MKV:
		return true
	case p.Source == MOV && p.Target == MKV:
		return true
	case p.Source == MKV && p.Target == MOV:
		return true
	}
	return false
}

// IsWebMMP4 reports whether the pair is WebM ↔ MP4.
func IsWebMMP4(p Pair) bool {
	return (p.Source == WEBM && p.Target == MP4) || (p.Source == MP4 && p.Target == WEBM)
}

// IsAudioExtract reports whether the pair is video → MP3.
func IsAudioExtract(p Pair) bool { return p.Target == MP3 && IsVideo(p.Source) }

// IsGIFPair reports whether the pair is GIF ↔ MP4.
func IsGIFPair(p Pair) bool {
	return (p.Source == GIF && p.Target == MP4) || (p.Source == MP4 && p.Target == GIF)
}

// IsV02Pair reports whether the pair was introduced by the v0.2 media format
// addendum. The v1.7 pairs keep their original pre-flight and engine paths.
func IsV02Pair(p Pair) bool {
	switch {
	case p.Source == WAV && p.Target == MP3:
		return false
	case p.Source == MKV && p.Target == MP4:
		return false
	case IsImage(p.Source) && IsImage(p.Target):
		return false
	}
	return true
}

// ParseMode normalizes a mode name.
func ParseMode(s string) (Mode, bool) {
	switch Mode(strings.ToLower(strings.TrimSpace(s))) {
	case "", ModeAuto:
		return ModeAuto, true
	case ModeSafe:
		return ModeSafe, true
	case ModeRelocate:
		return ModeRelocate, true
	case ModeReclaim:
		return ModeReclaim, true
	}
	return "", false
}

// Lookup finds the pair for source→target.
func Lookup(source, target Format) (Pair, bool) {
	for _, p := range pairs {
		if p.Source == source && p.Target == target {
			return p, true
		}
	}
	return Pair{}, false
}

// ResolveMode turns auto into the pair's default and validates explicit modes.
func ResolveMode(p Pair, requested Mode) (Mode, error) {
	if requested == "" || requested == ModeAuto {
		return p.Default, nil
	}
	for _, m := range p.Modes {
		if m == requested {
			return m, nil
		}
	}
	return "", siphonerr.New(siphonerr.ModeUnsupported,
		"mode %q is not available for %s → %s (supported: %s)",
		requested, p.Source, p.Target, modeList(p.Modes))
}

// EngineFor selects the engine for a resolved mode.
func EngineFor(p Pair, mode Mode) EngineKind {
	if p.Source == MKV && mode == ModeSafe {
		if p.Target == MP4 {
			return EngineRemux
		}
	}
	return p.Engine
}

// SupportedTargets lists targets for a source format, in table order.
func SupportedTargets(source Format) []Format {
	var out []Format
	for _, p := range pairs {
		if p.Source == source {
			out = append(out, p.Target)
		}
	}
	return out
}

// AllPairs returns a copy of the table.
func AllPairs() []Pair {
	out := make([]Pair, len(pairs))
	copy(out, pairs)
	return out
}

// PairForPath resolves a source path and target format.
func PairForPath(sourcePath string, target Format) (Format, Pair, error) {
	src, err := FromPath(sourcePath)
	if err != nil {
		return "", Pair{}, err
	}
	if src == target {
		return "", Pair{}, siphonerr.New(siphonerr.FormatUnsupported,
			"source and target formats are both %s", src)
	}
	p, ok := Lookup(src, target)
	if !ok {
		targets := SupportedTargets(src)
		names := make([]string, len(targets))
		for i, t := range targets {
			names[i] = t.String()
		}
		return "", Pair{}, siphonerr.New(siphonerr.FormatUnsupported,
			"%s → %s is not supported (supported targets from %s: %s)",
			src, target, src, strings.Join(names, ", "))
	}
	return src, p, nil
}

func modeList(modes []Mode) string {
	names := make([]string, len(modes))
	for i, m := range modes {
		names[i] = string(m)
	}
	return strings.Join(names, ", ")
}
