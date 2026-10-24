package stitch

import (
	"encoding/binary"
	"fmt"
	"math"
)

// mp4Box is one ISO-BMFF box in a memory buffer.
type mp4Box struct {
	typ  string
	off  int
	size int // total size including the header
	hdr  int // header length (8, or 16 for 64-bit sizes)
}

// walkBoxes iterates boxes in buf[start:end].
func walkBoxes(buf []byte, start, end int) ([]mp4Box, error) {
	var out []mp4Box
	off := start
	for off+8 <= end {
		size := int64(binary.BigEndian.Uint32(buf[off:]))
		typ := string(buf[off+4 : off+8])
		hdr := int64(8)
		switch {
		case size == 1:
			if off+16 > end {
				return nil, fmt.Errorf("box %s at %d: truncated 64-bit size", typ, off)
			}
			size = int64(binary.BigEndian.Uint64(buf[off+8:]))
			hdr = 16
		case size == 0:
			size = int64(end - off)
		}
		if size < hdr || int64(off)+size > int64(end) {
			return nil, fmt.Errorf("box %s at %d: size %d overruns %d", typ, off, size, end)
		}
		out = append(out, mp4Box{typ: typ, off: off, size: int(size), hdr: int(hdr)})
		off += int(size)
	}
	return out, nil
}

// FragmentInfo is what the spike records about one extracted fragment.
type FragmentInfo struct {
	Index        int
	Bytes        int64
	Moofs        int
	Tfdts        map[uint32]uint64 // baseMediaDecodeTime per track (first moof)
	FirstPTS     map[uint32]int64  // first sample PTS per track, raw ticks
	FirstCTS     map[uint32]int64  // that sample's composition offset, raw ticks
	AnchorNS     map[uint32]int64  // source first-block PTS per track the fragment was re-anchored to
	SampleCounts map[uint32]int
	InitLen      int64 // leading ftyp+moov bytes (present on every extraction)
	Tracks       []uint32
}

// analyzeFragment parses a standalone fragmented-MP4 fragment produced by one
// segment extraction: box layout, per-track trun sample counts, and the raw
// tfdt/first-sample times of the first moof that carries each track.
func analyzeFragment(buf []byte) (FragmentInfo, error) {
	info := FragmentInfo{
		SampleCounts: map[uint32]int{},
		Tfdts:        map[uint32]uint64{},
		FirstPTS:     map[uint32]int64{},
		FirstCTS:     map[uint32]int64{},
	}
	top, err := walkBoxes(buf, 0, len(buf))
	if err != nil {
		return info, err
	}
	sawMoof := false
	for _, b := range top {
		switch b.typ {
		case "ftyp", "moov":
			info.InitLen += int64(b.size)
		case "moof":
			sawMoof = true
			info.Moofs++
			children, err := walkBoxes(buf, b.off+b.hdr, b.off+b.size)
			if err != nil {
				return info, err
			}
			for _, c := range children {
				if c.typ != "traf" {
					continue
				}
				ti, err := parseTraf(buf, c)
				if err != nil {
					return info, err
				}
				if ti.track == 0 {
					continue
				}
				if _, ok := info.SampleCounts[ti.track]; !ok {
					info.Tracks = append(info.Tracks, ti.track)
				}
				info.SampleCounts[ti.track] += ti.count
				if _, ok := info.FirstPTS[ti.track]; !ok {
					info.Tfdts[ti.track] = ti.tfdt
					info.FirstPTS[ti.track] = int64(ti.tfdt) + ti.ctsFirst + ctsShift(ti)
					info.FirstCTS[ti.track] = ti.ctsFirst
				}
			}
		}
	}
	if !sawMoof {
		return info, fmt.Errorf("fragment has no moof box")
	}
	return info, nil
}

// trafInfo is one parsed traf: track ID, sample count, tfdt, and the
// composition-time offsets of its samples (first sample and minimum).
type trafInfo struct {
	track      uint32
	count      int
	tfdt       uint64
	tfdtVer    byte
	ctsPresent bool
	ctsFirst   int64
	ctsMin     int64
}

// trun trun-box flag bits (ISO/IEC 14496-12 §8.8.8).
const (
	trunDataOffset       = 0x000001
	trunFirstSampleFlags = 0x000004
	trunSampleDuration   = 0x000100
	trunSampleSize       = 0x000200
	trunSampleFlags      = 0x000400
	trunSampleCTS        = 0x000800
)

// trunCTSLayout locates the per-sample composition-time offsets of one trun:
// the byte offset of the first value and the stride between values.
func trunCTSLayout(buf []byte, c mp4Box) (off, stride, count int, version byte, present bool, err error) {
	p := c.off + c.hdr
	if p+8 > c.off+c.size {
		return 0, 0, 0, 0, false, fmt.Errorf("trun truncated")
	}
	version = buf[p]
	flags := uint32(buf[p+1])<<16 | uint32(buf[p+2])<<8 | uint32(buf[p+3])
	count = int(binary.BigEndian.Uint32(buf[p+4:]))
	present = flags&trunSampleCTS != 0
	if !present {
		return 0, 0, count, version, false, nil
	}
	off = p + 8
	if flags&trunDataOffset != 0 {
		off += 4
	}
	if flags&trunFirstSampleFlags != 0 {
		off += 4
	}
	stride = 4 // the composition offset itself
	if flags&trunSampleDuration != 0 {
		stride += 4
	}
	if flags&trunSampleSize != 0 {
		stride += 4
	}
	if flags&trunSampleFlags != 0 {
		stride += 4
	}
	// Duration/size/flags precede the composition offset within each sample.
	off += stride - 4
	if count > 0 && off+stride*(count-1)+4 > c.off+c.size {
		return 0, 0, 0, 0, false, fmt.Errorf("trun composition offsets overrun the box")
	}
	return off, stride, count, version, true, nil
}

// readCTS decodes one composition-time offset: unsigned in trun v0, signed in
// v1.
func readCTS(buf []byte, at int, version byte) int64 {
	raw := binary.BigEndian.Uint32(buf[at:])
	if version == 0 {
		return int64(raw)
	}
	return int64(int32(raw))
}

// trunCTS returns the first and minimum composition offsets of one trun.
func trunCTS(buf []byte, c mp4Box) (present bool, count int, first, min int64, err error) {
	off, stride, count, version, present, err := trunCTSLayout(buf, c)
	if err != nil || !present {
		return present, count, 0, 0, err
	}
	for i := 0; i < count; i++ {
		v := readCTS(buf, off+i*stride, version)
		if i == 0 {
			first, min = v, v
		} else if v < min {
			min = v
		}
	}
	return present, count, first, min, nil
}

// shiftTrunCTS adds delta to every composition offset of one trun.
func shiftTrunCTS(buf []byte, c mp4Box, delta int64) error {
	off, stride, count, version, present, err := trunCTSLayout(buf, c)
	if err != nil || !present {
		return err
	}
	for i := 0; i < count; i++ {
		at := off + i*stride
		nv := readCTS(buf, at, version) + delta
		if nv < math.MinInt32 || nv > math.MaxInt32 {
			return fmt.Errorf("composition offset %d overflows int32", nv)
		}
		binary.BigEndian.PutUint32(buf[at:], uint32(int32(nv)))
	}
	return nil
}

// parseTraf reads the track ID, total sample count, tfdt, and the first/min
// composition offset from one traf box.
func parseTraf(buf []byte, traf mp4Box) (trafInfo, error) {
	var ti trafInfo
	children, err := walkBoxes(buf, traf.off+traf.hdr, traf.off+traf.size)
	if err != nil {
		return ti, err
	}
	for _, c := range children {
		p := c.off + c.hdr // fullbox payload (version+flags first)
		switch c.typ {
		case "tfhd":
			if p+8 > c.off+c.size {
				return ti, fmt.Errorf("tfhd truncated")
			}
			ti.track = binary.BigEndian.Uint32(buf[p+4:])
		case "tfdt":
			if p+4 > c.off+c.size {
				return ti, fmt.Errorf("tfdt truncated")
			}
			ti.tfdtVer = buf[p]
			switch ti.tfdtVer {
			case 0:
				if p+8 > c.off+c.size {
					return ti, fmt.Errorf("tfdt v0 truncated")
				}
				ti.tfdt = uint64(binary.BigEndian.Uint32(buf[p+4:]))
			case 1:
				if p+12 > c.off+c.size {
					return ti, fmt.Errorf("tfdt v1 truncated")
				}
				ti.tfdt = binary.BigEndian.Uint64(buf[p+4:])
			default:
				return ti, fmt.Errorf("tfdt version %d", ti.tfdtVer)
			}
		case "trun":
			present, n, first, min, err := trunCTS(buf, c)
			if err != nil {
				return ti, err
			}
			ti.count += n
			if present {
				if !ti.ctsPresent {
					ti.ctsFirst = first
					ti.ctsMin = min
				} else if min < ti.ctsMin {
					ti.ctsMin = min
				}
				ti.ctsPresent = true
			}
		}
	}
	return ti, nil
}

// stripInit removes leading ftyp/moov boxes and the trailing mfra index box,
// returning the rest plus the captured init bytes (ftyp+moov in file order).
// Every extraction run ends with its own mfra whose mfro/tfra offsets point
// into that standalone run; once fragments are concatenated those offsets are
// invalid, so the index must go.
func stripInit(buf []byte) (init, fragment []byte, err error) {
	top, err := walkBoxes(buf, 0, len(buf))
	if err != nil {
		return nil, nil, err
	}
	cut := 0
	initEnd := 0
	for _, b := range top {
		if b.typ == "ftyp" || b.typ == "moov" {
			if b.off != cut {
				break // interleaved; keep it simple and stop stripping
			}
			cut = b.off + b.size
			initEnd = cut
			continue
		}
		break
	}
	bodyEnd := len(buf)
	if n := len(top); n > 0 && top[n-1].typ == "mfra" {
		bodyEnd = top[n-1].off
	}
	return buf[:initEnd], buf[cut:bodyEnd], nil
}

// renumberMfhd rewrites the moof's mfhd sequence_number in place.
func renumberMfhd(buf []byte, seq uint32) error {
	top, err := walkBoxes(buf, 0, len(buf))
	if err != nil {
		return err
	}
	for _, b := range top {
		if b.typ != "moof" {
			continue
		}
		children, err := walkBoxes(buf, b.off+b.hdr, b.off+b.size)
		if err != nil {
			return err
		}
		for _, c := range children {
			if c.typ != "mfhd" {
				continue
			}
			p := c.off + c.hdr
			if p+8 > c.off+c.size {
				return fmt.Errorf("mfhd truncated")
			}
			binary.BigEndian.PutUint32(buf[p+4:], seq)
			return nil
		}
		return fmt.Errorf("moof has no mfhd")
	}
	return fmt.Errorf("fragment has no moof")
}

// freeBoxHeader builds the free box header for a hole of size n. The rest of
// the hole is already zero because the temp is sparse; only the header is
// written.
func freeBoxHeader(n int64) ([]byte, error) {
	if n < 8 {
		return nil, fmt.Errorf("hole of %d bytes cannot hold a free box", n)
	}
	if n <= 0xFFFFFFFF {
		hdr := make([]byte, 8)
		binary.BigEndian.PutUint32(hdr[0:], uint32(n))
		copy(hdr[4:], "free")
		return hdr, nil
	}
	hdr := make([]byte, 16)
	binary.BigEndian.PutUint32(hdr[0:], 1) // 64-bit largesize follows
	copy(hdr[4:], "free")
	binary.BigEndian.PutUint64(hdr[8:], uint64(n))
	return hdr, nil
}

// initTrackHandlers maps track_ID → handler_type ("vide"/"soun") from the
// assembled init segment (moov), so verification can tell which traf's tfdt
// belongs to the video track.
func initTrackHandlers(init []byte) map[uint32]string {
	out := map[uint32]string{}
	top, err := walkBoxes(init, 0, len(init))
	if err != nil {
		return out
	}
	for _, b := range top {
		if b.typ != "moov" {
			continue
		}
		children, err := walkBoxes(init, b.off+b.hdr, b.off+b.size)
		if err != nil {
			return out
		}
		for _, c := range children {
			if c.typ != "trak" {
				continue
			}
			var track uint32
			var handler string
			grand, err := walkBoxes(init, c.off+c.hdr, c.off+c.size)
			if err != nil {
				return out
			}
			for _, g := range grand {
				switch g.typ {
				case "tkhd":
					p := g.off + g.hdr
					if p+4 > g.off+g.size {
						continue
					}
					switch init[p] {
					case 0:
						if p+16 <= g.off+g.size {
							track = binary.BigEndian.Uint32(init[p+12:])
						}
					case 1:
						if p+24 <= g.off+g.size {
							track = binary.BigEndian.Uint32(init[p+20:])
						}
					}
				case "mdia":
					sub, err := walkBoxes(init, g.off+g.hdr, g.off+g.size)
					if err != nil {
						continue
					}
					for _, s := range sub {
						if s.typ == "hdlr" {
							p := s.off + s.hdr
							if p+12 <= s.off+s.size {
								handler = string(init[p+8 : p+12])
							}
						}
					}
				}
			}
			if track != 0 {
				out[track] = handler
			}
		}
	}
	return out
}
