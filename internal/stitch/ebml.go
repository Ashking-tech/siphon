// Package stitch is the video stitching core proved by the phase-4 spike
// (spec §8): it plans keyframe-aligned segments from Cues (with an ffprobe
// scan fallback), extracts them tail-first from a cached EBML header plus one
// byte range each, assembles a fragmented MP4 into a sparse temp written
// backwards, and verifies packet counts, tfdt monotonicity, and one-pass I/O.
//
// The standalone Run harness and RESULTS.md record what the spike proved; the
// RelocateEngine uses the Session API for the journaled, resumable production
// path.
package stitch

import (
	"encoding/binary"
	"io"
	"math"

	"siphon/internal/siphonerr"
)

// EBML element IDs, raw (marker bits included), as they appear on disk.
const (
	idEBML                uint64 = 0x1A45DFA3
	idSegment             uint64 = 0x18538067
	idSeekHead            uint64 = 0x114D9B74
	idSeek                uint64 = 0x4DBB
	idSeekID              uint64 = 0x53AB
	idSeekPosition        uint64 = 0x53AC
	idInfo                uint64 = 0x1549A966
	idTimecodeScale       uint64 = 0x2AD7B1
	idDuration            uint64 = 0x4489
	idTracks              uint64 = 0x1654AE6B
	idTrackEntry          uint64 = 0xAE
	idTrackNumber         uint64 = 0xD7
	idTrackType           uint64 = 0x83
	idCodecID             uint64 = 0x86
	idDefaultDuration     uint64 = 0x23E383
	idCodecDelay          uint64 = 0x56AA
	idCues                uint64 = 0x1C53BB6B
	idCuePoint            uint64 = 0xBB
	idCueTime             uint64 = 0xB3
	idCueTrackPositions   uint64 = 0xB7
	idCueTrack            uint64 = 0xF7
	idCueClusterPosition  uint64 = 0xF1
	idCueRelativePosition uint64 = 0xF0
	idCluster             uint64 = 0x1F43B675
	idVoid                uint64 = 0xEC
	idCRC32               uint64 = 0xBF
)

// elem is one parsed EBML element header plus its byte offsets.
type elem struct {
	id      uint64
	off     int64 // offset of the element's first byte
	sizeOff int64 // offset of the size vint
	payload int64 // offset of the payload (one past the size vint)
	size    uint64
	hdrLen  int
	idLen   int
	sizeLen int
	unknown bool
}

// end returns the offset one past the element (only valid when !unknown).
func (e elem) end() int64 { return e.payload + int64(e.size) }

// idLen returns the length in bytes of the element ID starting at b.
func idLen(b byte) int {
	switch {
	case b&0x80 != 0:
		return 1
	case b&0x40 != 0:
		return 2
	case b&0x20 != 0:
		return 3
	case b&0x10 != 0:
		return 4
	}
	return 0
}

// sizeLen returns the length in bytes of a size vint starting at b.
func sizeLen(b byte) int {
	if b == 0 {
		return 0
	}
	n := 1
	for m := byte(0x80); b&m == 0; m >>= 1 {
		n++
	}
	return n
}

// parseElemHeader parses an element header from buf (which must hold it).
func parseElemHeader(buf []byte) (elem, error) {
	var e elem
	if len(buf) < 2 {
		return e, corrupt("truncated element header")
	}
	il := idLen(buf[0])
	if il == 0 || len(buf) < il+1 {
		return e, corrupt("invalid element ID at % X", buf[:minInt(4, len(buf))])
	}
	for i := 0; i < il; i++ {
		e.id = e.id<<8 | uint64(buf[i])
	}
	sl := sizeLen(buf[il])
	if sl == 0 || len(buf) < il+sl {
		return e, corrupt("invalid size vint after element 0x%X", e.id)
	}
	e.size = uint64(buf[il]) & (0xFF >> sl)
	for i := 1; i < sl; i++ {
		e.size = e.size<<8 | uint64(buf[il+i])
	}
	e.unknown = e.size == uint64(1)<<(7*sl)-1
	e.idLen, e.sizeLen, e.hdrLen = il, sl, il+sl
	return e, nil
}

// readElem reads and parses one element header at off.
func readElem(r io.ReaderAt, off int64) (elem, error) {
	var buf [12]byte
	n, err := r.ReadAt(buf[:], off)
	if n < 2 {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return elem{}, err
	}
	e, perr := parseElemHeader(buf[:n])
	if perr != nil {
		return elem{}, perr
	}
	if e.hdrLen > n {
		return elem{}, io.ErrUnexpectedEOF
	}
	e.off = off
	e.sizeOff = off + int64(e.idLen)
	e.payload = off + int64(e.hdrLen)
	return e, nil
}

// forChildren walks the children of a master element in [payload, end).
func forChildren(r io.ReaderAt, payload, end int64, fn func(e elem) error) error {
	for off := payload; off < end; {
		e, err := readElem(r, off)
		if err != nil {
			return corrupt("malformed EBML at %d: %v", off, err)
		}
		if e.unknown {
			return corrupt("unknown-size element 0x%X at %d", e.id, off)
		}
		if e.end() > end {
			return corrupt("element 0x%X at %d overruns its parent", e.id, off)
		}
		if err := fn(e); err != nil {
			return err
		}
		off = e.end()
	}
	return nil
}

// readFull fills buf from off, mapping a short read to a clear error.
func readFull(r io.ReaderAt, buf []byte, off int64) error {
	if n, err := r.ReadAt(buf, off); n != len(buf) {
		if err == nil {
			err = io.ErrUnexpectedEOF
		}
		return corrupt("short read at %d: %v", off, err)
	}
	return nil
}

// uintPayload reads an unsigned integer payload (1–8 bytes, big endian).
func uintPayload(r io.ReaderAt, e elem) (uint64, error) {
	if e.size > 8 {
		return 0, corrupt("integer element 0x%X is %d bytes", e.id, e.size)
	}
	var buf [8]byte
	if err := readFull(r, buf[:e.size], e.payload); err != nil {
		return 0, err
	}
	var v uint64
	for i := 0; i < int(e.size); i++ {
		v = v<<8 | uint64(buf[i])
	}
	return v, nil
}

// floatPayload reads a float payload (4 or 8 bytes, big endian).
func floatPayload(r io.ReaderAt, e elem) (float64, error) {
	var buf [8]byte
	switch e.size {
	case 4:
		if err := readFull(r, buf[:4], e.payload); err != nil {
			return 0, err
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(buf[:4]))), nil
	case 8:
		if err := readFull(r, buf[:8], e.payload); err != nil {
			return 0, err
		}
		return math.Float64frombits(binary.BigEndian.Uint64(buf[:8])), nil
	}
	return 0, corrupt("float element 0x%X is %d bytes", e.id, e.size)
}

// bytesPayload reads a binary payload, refusing unreasonably large elements.
func bytesPayload(r io.ReaderAt, e elem, limit int64) ([]byte, error) {
	if int64(e.size) > limit {
		return nil, corrupt("element 0x%X is %d bytes (limit %d)", e.id, e.size, limit)
	}
	buf := make([]byte, e.size)
	if err := readFull(r, buf, e.payload); err != nil {
		return nil, err
	}
	return buf, nil
}

func corrupt(format string, args ...any) error {
	return siphonerr.New(siphonerr.SourceCorrupt, "EBML: "+format, args...)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fileHeader is the parsed EBML header plus the Segment box bounds.
type fileHeader struct {
	fileSize     int64
	segStart     int64 // Segment payload start; SeekHead positions are relative to this
	segEnd       int64 // Segment payload end, clamped to the file size
	segSize      elem  // the Segment element header
	segSizeKnown bool
}

// parseFileHeader reads the EBML header and the Segment box.
func parseFileHeader(r io.ReaderAt, fileSize int64) (fileHeader, error) {
	h := fileHeader{fileSize: fileSize}
	e, err := readElem(r, 0)
	if err != nil {
		return h, corrupt("cannot read the file header: %v", err)
	}
	if e.id != idEBML {
		return h, corrupt("not an EBML file (first element 0x%X)", e.id)
	}
	seg, err := readElem(r, e.end())
	if err != nil {
		return h, corrupt("cannot read the Segment header: %v", err)
	}
	if seg.id != idSegment {
		return h, corrupt("no Segment element after the EBML header (got 0x%X)", seg.id)
	}
	h.segSize = seg
	h.segStart = seg.payload
	if seg.unknown {
		h.segEnd = fileSize
		return h, nil
	}
	end := seg.payload + int64(seg.size)
	if end > fileSize || end <= seg.payload {
		end = fileSize
	} else {
		h.segSizeKnown = true
	}
	h.segEnd = end
	return h, nil
}

// unknownSizeVint is the 8-byte "unknown size" marker (0x01 followed by 0xFF×7).
var unknownSizeVint = []byte{0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}

// clusterIDBytes is a Cluster element's on-disk ID.
var clusterIDBytes = []byte{0x1F, 0x43, 0xB6, 0x75}

// appendSizeVint appends the shortest EBML size vint for n. The all-ones
// pattern is reserved for "unknown size", so n must stay below it.
func appendSizeVint(dst []byte, n int64) ([]byte, error) {
	if n < 0 {
		return nil, corrupt("negative element size %d", n)
	}
	for sl := 1; sl <= 8; sl++ {
		limit := int64(1)<<(7*uint(sl)) - 1
		if n < limit {
			v := uint64(n) | uint64(1)<<(7*uint(sl))
			for i := sl - 1; i >= 0; i-- {
				dst = append(dst, byte(v>>(8*uint(i))))
			}
			return dst, nil
		}
	}
	return nil, corrupt("element size %d cannot be encoded", n)
}

// seekEntry is one SeekHead Seek element.
type seekEntry struct {
	id  uint64 // the element ID the entry points at
	pos int64  // offset relative to the Segment payload start
}

// parseSeekHead reads a SeekHead element's entries.
func parseSeekHead(r io.ReaderAt, e elem) ([]seekEntry, error) {
	var out []seekEntry
	err := forChildren(r, e.payload, e.end(), func(c elem) error {
		if c.id != idSeek {
			return nil
		}
		var ent seekEntry
		err := forChildren(r, c.payload, c.end(), func(f elem) error {
			switch f.id {
			case idSeekID:
				b, err := bytesPayload(r, f, 8)
				if err != nil {
					return err
				}
				var id uint64
				for _, bb := range b {
					id = id<<8 | uint64(bb)
				}
				ent.id = id
			case idSeekPosition:
				v, err := uintPayload(r, f)
				if err != nil {
					return err
				}
				ent.pos = int64(v)
			}
			return nil
		})
		if err != nil {
			return err
		}
		if ent.id != 0 {
			out = append(out, ent)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// topIndex is the result of one top-level walk of the Segment.
type topIndex struct {
	infoOff      int64
	tracksOff    int64
	cuesOff      int64
	seekHeadOff  int64
	clusters     []int64 // every Cluster element offset, in file order
	firstCluster int64
	seek         []seekEntry
}

// scanTopLevel walks the Segment's top-level elements. It skips element
// payloads by their declared size (cluster skipping reads headers only), and
// jumps straight to Cues when a SeekHead points at it, unless collectClusters
// asks for every cluster offset (needed by the packet-scan fallback). Files
// with unknown-size clusters stop the walk there.
func scanTopLevel(r io.ReaderAt, h fileHeader, collectClusters bool) (topIndex, error) {
	var ix topIndex
	jumped := false
	for off := h.segStart; off < h.segEnd; {
		if ix.cuesOff != 0 && ix.firstCluster != 0 && !collectClusters {
			break
		}
		e, err := readElem(r, off)
		if err != nil {
			if off == h.segStart {
				return ix, corrupt("cannot read the first top-level element: %v", err)
			}
			break // trailing junk after the last element
		}
		if e.unknown {
			if e.id == idCluster {
				ix.clusters = append(ix.clusters, off)
				if ix.firstCluster == 0 {
					ix.firstCluster = off
				}
			}
			break
		}
		if e.end() > h.segEnd {
			break
		}
		switch e.id {
		case idSeekHead:
			if ix.seekHeadOff == 0 {
				ix.seekHeadOff = off
				entries, err := parseSeekHead(r, e)
				if err != nil {
					return ix, err
				}
				ix.seek = entries
			}
		case idInfo:
			if ix.infoOff == 0 {
				ix.infoOff = off
			}
		case idTracks:
			if ix.tracksOff == 0 {
				ix.tracksOff = off
			}
		case idCues:
			ix.cuesOff = off
		case idCluster:
			ix.clusters = append(ix.clusters, off)
			if ix.firstCluster == 0 {
				ix.firstCluster = off
			}
		}
		off = e.end()
		// A SeekHead found before the clusters names Cues. Once the first
		// cluster is known (that is the cached-header boundary), jump
		// straight over the remaining clusters to Cues.
		if !jumped && !collectClusters && ix.cuesOff == 0 && ix.firstCluster != 0 {
			for _, s := range ix.seek {
				if s.id == idCues {
					target := h.segStart + s.pos
					if target >= off && target < h.segEnd {
						off = target
						jumped = true
					}
					break
				}
			}
		}
	}
	return ix, nil
}

// parseInfo extracts the timecode scale (ns per tick) and the duration.
func parseInfo(r io.ReaderAt, off int64) (scaleNS int64, durationNS int64, err error) {
	scaleNS = 1_000_000 // 1 ms default
	e, err := readElem(r, off)
	if err != nil {
		return 0, 0, err
	}
	err = forChildren(r, e.payload, e.end(), func(c elem) error {
		switch c.id {
		case idTimecodeScale:
			v, err := uintPayload(r, c)
			if err != nil {
				return err
			}
			scaleNS = int64(v)
		case idDuration:
			v, err := floatPayload(r, c)
			if err != nil {
				return err
			}
			durationNS = int64(v * float64(scaleNS))
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}
	return scaleNS, durationNS, nil
}

// trackInfo is the slice of TrackEntry the spike needs.
type trackInfo struct {
	number            uint64
	typ               uint64 // 1 video, 2 audio
	codec             string
	defaultDurationNS int64
	codecDelayNS      int64 // CodecDelay: subtracted from block timecodes by demuxers
}

// parseTracks reads all TrackEntry elements.
func parseTracks(r io.ReaderAt, off int64) ([]trackInfo, error) {
	e, err := readElem(r, off)
	if err != nil {
		return nil, err
	}
	var out []trackInfo
	err = forChildren(r, e.payload, e.end(), func(c elem) error {
		if c.id != idTrackEntry {
			return nil
		}
		var t trackInfo
		err := forChildren(r, c.payload, c.end(), func(f elem) error {
			var err error
			switch f.id {
			case idTrackNumber:
				t.number, err = uintPayload(r, f)
			case idTrackType:
				t.typ, err = uintPayload(r, f)
			case idCodecID:
				b, berr := bytesPayload(r, f, 256)
				if berr != nil {
					return berr
				}
				t.codec = string(b)
			case idDefaultDuration:
				var v uint64
				v, err = uintPayload(r, f)
				t.defaultDurationNS = int64(v)
			case idCodecDelay:
				var v uint64
				v, err = uintPayload(r, f)
				t.codecDelayNS = int64(v)
			}
			return err
		})
		if err != nil {
			return err
		}
		out = append(out, t)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// cuePoint is one cluster-aligned keyframe from Cues.
type cuePoint struct {
	timeTicks  int64 // CueTime, in TimecodeScale ticks
	track      uint64
	clusterPos int64 // offset relative to the Segment payload start
	relPos     uint64
}

// parseCues reads every CuePoint/CueTrackPositions below a Cues element.
func parseCues(r io.ReaderAt, e elem) ([]cuePoint, error) {
	var out []cuePoint
	err := forChildren(r, e.payload, e.end(), func(cp elem) error {
		if cp.id != idCuePoint {
			return nil
		}
		var timeVal int64
		var track uint64
		var clusterRel int64
		var relPos uint64
		err := forChildren(r, cp.payload, cp.end(), func(f elem) error {
			switch f.id {
			case idCueTime:
				v, err := uintPayload(r, f)
				if err != nil {
					return err
				}
				timeVal = int64(v)
			case idCueTrackPositions:
				return forChildren(r, f.payload, f.end(), func(g elem) error {
					var err error
					switch g.id {
					case idCueTrack:
						track, err = uintPayload(r, g)
					case idCueClusterPosition:
						v, err2 := uintPayload(r, g)
						if err2 != nil {
							return err2
						}
						clusterRel = int64(v)
					case idCueRelativePosition:
						relPos, err = uintPayload(r, g)
					}
					return err
				})
			}
			return nil
		})
		if err != nil {
			return err
		}
		out = append(out, cuePoint{timeTicks: timeVal, track: track, clusterPos: clusterRel, relPos: relPos})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
