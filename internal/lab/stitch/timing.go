package stitch

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"

	"siphon/internal/siphonerr"
)

// segmentTiming is the per-segment data the timestamp patch pass needs: how
// many moofs the segment contributed to the assembled file, and the source
// first-block PTS (ns) each MP4 track must be re-anchored to.
type segmentTiming struct {
	moofs    int
	anchorNS map[uint32]int64
}

// patchTimestamps rewrites the assembled file's fragment timestamps so every
// segment replays the source's own timeline.
//
// Two ffmpeg behaviours make raw fragments unusable as-is (both proven
// empirically; see RESULTS.md):
//
//   - the fragmented-MP4 muxer rebases each track of each extraction run to
//     that track's own first dts, so every run starts near zero;
//   - the mov demuxer reconstructs a track's PTS as tfdt + cts + dts_shift,
//     where dts_shift is the most negative composition offset of the track
//     seen so far (ffmov.c: mov_update_dts_shift). That makes the absolute
//     timeline depend on how much of the file a demuxer has parsed.
//
// This pass walks the moof chain twice — a few hundred KB per hour of film,
// never the media payloads:
//
//	pass 1: read each (segment, track) first sample time from the raw moofs,
//	        i.e. tfdt + cts + dts_shift; per track, the delta that maps it
//	        onto the source's first-block anchor, plus one global pad G so no
//	        patched decode time is negative;
//	pass 2: normalize each traf's composition offsets so none is negative
//	        (dts_shift becomes 0 for any demuxer and parse order), add each
//	        delta to every traf's tfdt, and renumber mfhd 1..N.
//
// Returns the global pad in ns.
func patchTimestamps(f *os.File, init []byte, timings []segmentTiming, stats *IOStats) (int64, error) {
	scales := trackTimescales(init)
	if len(scales) == 0 {
		return 0, siphonerr.New(siphonerr.StitchFailed, "the init segment carries no track timescales")
	}
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	fileSize := st.Size()

	type moofRef struct {
		off  int64
		size int64
		seg  int
	}
	var moofs []moofRef
	rawFirst := make([]map[uint32]int64, len(timings))
	for i := range timings {
		rawFirst[i] = map[uint32]int64{}
	}

	seg := 0
	for seg < len(timings) && timings[seg].moofs == 0 {
		seg++
	}
	segLeft := 0
	if seg < len(timings) {
		segLeft = timings[seg].moofs
	}

	// Pass 1: locate every moof, and per segment and track note the raw first
	// sample PTS (tfdt + composition offset) of the first moof carrying it.
	for off := int64(0); off+8 <= fileSize; {
		typ, size, hdr, err := readBoxHeaderAt(f, off)
		if err != nil {
			return 0, err
		}
		if size < 0 {
			size = fileSize - off
		}
		if hdr > size || off+size > fileSize {
			return 0, siphonerr.New(siphonerr.StitchFailed,
				"box %s at %d overruns the assembled file", typ, off)
		}
		if typ == "moof" {
			if seg >= len(timings) {
				return 0, siphonerr.New(siphonerr.StitchFailed,
					"the assembled file carries more moofs than the plan's segments")
			}
			buf := make([]byte, size)
			if err := readFull(f, buf, off); err != nil {
				return 0, err
			}
			stats.PatchRead += size
			children, err := walkBoxes(buf, int(hdr), len(buf))
			if err != nil {
				return 0, err
			}
			for _, c := range children {
				if c.typ != "traf" {
					continue
				}
				ti, err := parseTraf(buf, c)
				if err != nil {
					return 0, err
				}
				if ti.track == 0 {
					return 0, siphonerr.New(siphonerr.StitchFailed,
						"a moof at %d has a traf with no tfhd", off)
				}
				if _, ok := rawFirst[seg][ti.track]; !ok {
					rawFirst[seg][ti.track] = int64(ti.tfdt) + ti.ctsFirst + ctsShift(ti)
				}
			}
			moofs = append(moofs, moofRef{off: off, size: size, seg: seg})
			segLeft--
			if segLeft <= 0 {
				seg++
				for seg < len(timings) && timings[seg].moofs == 0 {
					seg++
				}
				if seg < len(timings) {
					segLeft = timings[seg].moofs
				}
			}
		}
		off += size
	}

	// Desired first-sample tick per (segment, track) from the anchors, and one
	// global pad so no patched decode time is negative in any track.
	desired := make([]map[uint32]int64, len(timings))
	padNS := int64(0)
	for i, t := range timings {
		desired[i] = map[uint32]int64{}
		for track, ns := range t.anchorNS {
			scale := scales[track]
			if scale == 0 {
				return 0, siphonerr.New(siphonerr.StitchFailed,
					"no timescale for track %d in the init segment", track)
			}
			d := roundTicks(ns, scale)
			desired[i][track] = d
			if raw, ok := rawFirst[i][track]; ok {
				if need := raw - d; need > 0 {
					if needNS := ceilNS(need, scale); needNS > padNS {
						padNS = needNS
					}
				}
			}
		}
	}

	delta := make([]map[uint32]int64, len(timings))
	for i := range timings {
		delta[i] = map[uint32]int64{}
		for track, d := range desired[i] {
			raw, ok := rawFirst[i][track]
			if !ok {
				continue // this track has no samples in the segment
			}
			scale := scales[track]
			delta[i][track] = d + ceilTicks(padNS, scale) - raw
		}
	}

	// Pass 2: rewrite every traf's tfdt in place and renumber mfhd 1..N.
	for i, m := range moofs {
		buf := make([]byte, m.size)
		if err := readFull(f, buf, m.off); err != nil {
			return 0, err
		}
		stats.PatchRead += m.size
		if err := patchMoof(buf, delta[m.seg], uint32(i+1)); err != nil {
			return 0, siphonerr.Wrap(siphonerr.StitchFailed, err, "moof %d at %d", i, m.off)
		}
		if _, err := f.WriteAt(buf, m.off); err != nil {
			return 0, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot patch the moof at %d", m.off)
		}
		stats.PatchWrite += m.size
	}
	return padNS, nil
}

// roundTicks converts ns to track ticks, rounding to nearest (halves away
// from zero); split so the product never overflows int64.
func roundTicks(ns int64, scale uint64) int64 {
	q := ns / 1e9
	r := ns % 1e9
	v := q * int64(scale)
	num := r * int64(scale)
	if num >= 0 {
		return v + (num+5e8)/1e9
	}
	return v - (-num+5e8)/1e9
}

// ceilTicks converts ns to track ticks, rounding up.
func ceilTicks(ns int64, scale uint64) int64 {
	q := ns / 1e9
	r := ns % 1e9
	num := r * int64(scale)
	v := q*int64(scale) + num/1e9 // for negative num this truncation is the ceil
	if num > 0 && num%1e9 != 0 {
		v++
	}
	return v
}

// ceilNS converts a tick count to ns, rounding up.
func ceilNS(ticks int64, scale uint64) int64 {
	return (ticks*1e9 + int64(scale) - 1) / int64(scale)
}

// readBoxHeaderAt reads one top-level ISO box header at off. A size of zero
// (extends to EOF) is returned as -1.
func readBoxHeaderAt(r io.ReaderAt, off int64) (typ string, size, hdr int64, err error) {
	var h [16]byte
	if err := readFull(r, h[:8], off); err != nil {
		return "", 0, 0, err
	}
	size = int64(binary.BigEndian.Uint32(h[:4]))
	typ = string(h[4:8])
	hdr = 8
	switch size {
	case 1:
		if err := readFull(r, h[8:16], off+8); err != nil {
			return "", 0, 0, err
		}
		size = int64(binary.BigEndian.Uint64(h[8:16]))
		hdr = 16
	case 0:
		size = -1
	}
	return typ, size, hdr, nil
}

// ctsShift is ffmpeg's mov-demuxer dts_shift convention: a traf's composition
// offsets are interpreted relative to its most negative one. Normalizing them
// to non-negative makes the demuxed timeline independent of the parser.
func ctsShift(ti trafInfo) int64 {
	if ti.ctsPresent && ti.ctsMin < 0 {
		return -ti.ctsMin
	}
	return 0
}

// patchMoof rewrites one moof: every traf's composition offsets are normalized
// to non-negative, every tfdt gets its track's delta, and mfhd is renumbered.
func patchMoof(buf []byte, deltas map[uint32]int64, seq uint32) error {
	top, err := walkBoxes(buf, 0, len(buf))
	if err != nil {
		return err
	}
	if len(top) != 1 || top[0].typ != "moof" {
		return fmt.Errorf("expected a single moof box")
	}
	b := top[0]
	children, err := walkBoxes(buf, b.off+b.hdr, b.off+b.size)
	if err != nil {
		return err
	}
	sawMfhd := false
	for _, c := range children {
		switch c.typ {
		case "mfhd":
			p := c.off + c.hdr
			if p+8 > c.off+c.size {
				return fmt.Errorf("mfhd truncated")
			}
			binary.BigEndian.PutUint32(buf[p+4:], seq)
			sawMfhd = true
		case "traf":
			ti, err := parseTraf(buf, c)
			if err != nil {
				return err
			}
			if ti.track == 0 {
				return fmt.Errorf("traf has no tfhd")
			}
			d, ok := deltas[ti.track]
			if !ok {
				return fmt.Errorf("no timestamp delta for track %d", ti.track)
			}
			if shift := ctsShift(ti); shift > 0 {
				if err := shiftTrafCTS(buf, c, shift); err != nil {
					return err
				}
			}
			if err := patchTrafTFDT(buf, c, ti, d); err != nil {
				return err
			}
		}
	}
	if !sawMfhd {
		return fmt.Errorf("moof has no mfhd")
	}
	return nil
}

// shiftTrafCTS adds delta to every composition offset in the traf's truns.
func shiftTrafCTS(buf []byte, traf mp4Box, delta int64) error {
	children, err := walkBoxes(buf, traf.off+traf.hdr, traf.off+traf.size)
	if err != nil {
		return err
	}
	for _, c := range children {
		if c.typ != "trun" {
			continue
		}
		if err := shiftTrunCTS(buf, c, delta); err != nil {
			return err
		}
	}
	return nil
}

// patchTrafTFDT adds one track's delta to the traf's tfdt.
func patchTrafTFDT(buf []byte, traf mp4Box, ti trafInfo, d int64) error {
	children, err := walkBoxes(buf, traf.off+traf.hdr, traf.off+traf.size)
	if err != nil {
		return err
	}
	patched := false
	for _, c := range children {
		if c.typ != "tfdt" {
			continue
		}
		p := c.off + c.hdr
		base := int64(ti.tfdt) + d
		if base < 0 {
			return fmt.Errorf("track %d patched tfdt is negative (%d)", ti.track, base)
		}
		switch ti.tfdtVer {
		case 0:
			if base > math.MaxUint32 {
				return fmt.Errorf("track %d patched tfdt %d overflows v0", ti.track, base)
			}
			binary.BigEndian.PutUint32(buf[p+4:], uint32(base))
		default:
			binary.BigEndian.PutUint64(buf[p+4:], uint64(base))
		}
		patched = true
	}
	if !patched {
		return fmt.Errorf("track %d traf has no tfdt", ti.track)
	}
	return nil
}
