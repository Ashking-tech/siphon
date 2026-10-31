package media

import (
	"bytes"
	"image"
	"image/gif"
	"os"
	"time"

	"siphon/internal/siphonerr"
)

// GIFInfo is the decoded view of a GIF source used by the GIF ↔ MP4 pairs.
// Decoding with image/gif is authoritative for frame count, logical canvas,
// and transparency; ffprobe alone cannot tell whether the palette is actually
// used as transparent.
type GIFInfo struct {
	Width       int
	Height      int
	Frames      int
	LoopCount   int
	Delays      []time.Duration
	Total       time.Duration
	FrameRate   float64
	Transparent bool
	Warnings    []string
}

// ProbeGIF decodes the whole GIF. It refuses an empty or malformed animation
// as E_SOURCE_CORRUPT (spec §6.6 A4).
func ProbeGIF(path string) (*GIFInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
	}
	defer f.Close()

	g, err := gif.DecodeAll(f)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot decode GIF %s", path)
	}
	if len(g.Image) == 0 || g.Config.Width <= 0 || g.Config.Height <= 0 {
		return nil, siphonerr.New(siphonerr.SourceCorrupt, "%s has no GIF frames", path)
	}

	info := &GIFInfo{
		Width:     g.Config.Width,
		Height:    g.Config.Height,
		Frames:    len(g.Image),
		LoopCount: g.LoopCount,
		Delays:    make([]time.Duration, 0, len(g.Delay)),
	}
	for _, d := range g.Delay {
		// image/gif stores delay in hundredths of a second.
		info.Delays = append(info.Delays, time.Duration(d)*10*time.Millisecond)
	}
	for _, d := range info.Delays {
		info.Total += d
	}
	if info.Total <= 0 {
		// A missing delay is treated as 100 ms; the GIF still has at least one
		// frame, and the conversion remains well-defined.
		info.Total = time.Duration(len(info.Delays)) * 100 * time.Millisecond
		info.Warnings = append(info.Warnings, "GIF frame delays are zero or missing; assuming 100 ms per frame")
	}
	info.FrameRate = float64(info.Frames) / info.Total.Seconds()

	info.Transparent = gifHasTransparency(g.Image)
	if info.Frames == 1 {
		info.Warnings = append(info.Warnings, "GIF has one frame; the MP4 will be a still clip")
	}
	if info.Frames > 100000 {
		return nil, siphonerr.New(siphonerr.SourceCorrupt,
			"%s has an implausible %d GIF frames", path, info.Frames)
	}
	return info, nil
}

func gifHasTransparency(frames []*image.Paletted) bool {
	for _, frame := range frames {
		if frame == nil {
			continue
		}
		b := frame.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				if _, _, _, a := frame.At(x, y).RGBA(); a == 0 {
					return true
				}
			}
		}
	}
	return false
}

// GIFBytes magic is useful to unit tests without an external fixture.
var GIFBytes = []struct {
	Name string
	Data []byte
}{
	{"gif87a", []byte("GIF87a")},
	{"gif89a", []byte("GIF89a")},
}

// LooksLikeGIF is a cheap header check used before image/gif decoding.
func LooksLikeGIF(b []byte) bool {
	return len(b) >= 6 && (bytes.Equal(b[:6], GIFBytes[0].Data) || bytes.Equal(b[:6], GIFBytes[1].Data))
}
