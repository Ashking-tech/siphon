// Package formats owns the v0.1 format table, mode resolution, and the
// mapping from a (source, target, mode) triple to an engine.
package formats

import (
	"path/filepath"
	"strings"

	"siphon/internal/siphonerr"
)

// Format is a media container or image encoding.
type Format string

const (
	WAV  Format = "wav"
	MP3  Format = "mp3"
	MKV  Format = "mkv"
	MP4  Format = "mp4"
	PNG  Format = "png"
	JPG  Format = "jpg"
	WEBP Format = "webp"
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
	{WAV, MP3, ModeSafe, []Mode{ModeSafe, ModeReclaim}, EngineSliding, true, "MP3"},
	{MKV, MP4, ModeRelocate, []Mode{ModeRelocate, ModeSafe}, EngineRelocate, false, "fragmented MP4"},
	{PNG, JPG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "JPEG"},
	{JPG, PNG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "PNG"},
	{PNG, WEBP, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "WebP"},
	{WEBP, PNG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "PNG"},
	{JPG, WEBP, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "WebP"},
	{WEBP, JPG, ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "JPEG"},
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
	case "png":
		return PNG, true
	case "jpg", "jpeg", "jpe":
		return JPG, true
	case "webp":
		return WEBP, true
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
		return EngineRemux
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

// AllPairs returns a copy of the v0.1 table.
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
