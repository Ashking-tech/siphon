package stitch

import (
	"encoding/binary"
	"io"
)

// Cluster-child element IDs used for the first-block anchor parse.
const (
	idTimecode       uint64 = 0xE7
	idSimpleBlock    uint64 = 0xA3
	idBlockGroup     uint64 = 0xA0
	idBlock          uint64 = 0xA1
	idReferenceBlock uint64 = 0xFB
)

// readBlockHeader parses just the header of a (Simple)Block payload: the
// track-number vint, the signed int16 relative timecode, and the flags byte.
// For a SimpleBlock bit 0x80 marks a keyframe; a Block inside a BlockGroup is
// a keyframe when the group has no ReferenceBlock.
func readBlockHeader(r io.ReaderAt, payload, end int64) (track uint64, tc int64, flags byte, err error) {
	var b [10]byte
	if err := readFull(r, b[:1], payload); err != nil {
		return 0, 0, 0, err
	}
	n := sizeLen(b[0])
	if n == 0 || n > 8 {
		return 0, 0, 0, corrupt("invalid track-number vint at %d", payload)
	}
	if end-payload < int64(n+3) {
		return 0, 0, 0, corrupt("block header at %d runs past its element", payload)
	}
	if err := readFull(r, b[:n+3], payload); err != nil {
		return 0, 0, 0, err
	}
	track = uint64(b[0]) & (0xFF >> n)
	for i := 1; i < n; i++ {
		track = track<<8 | uint64(b[i])
	}
	tc = int64(int16(binary.BigEndian.Uint16(b[n:])))
	flags = b[n+2]
	return track, tc, flags, nil
}

// errAnchorsDone stops the cluster walk once every wanted track is found.
var errAnchorsDone = corrupt("anchors done")

// firstBlockTimes returns, per wanted track, the PTS (in ns) of the track's
// first block in the segment's input, starting at the Cluster element start.
// from is the first byte the opening cluster contributes (its keyframe block,
// or the payload start for the first segment): children before it belong to
// the previous segment's tail and are skipped. This is the absolute anchor the
// fragment's timestamps are rebuilt from: ffmpeg's fragment muxer rebases each
// track independently, so neither tfdt nor pts can be trusted to carry
// absolute time (see RESULTS.md).
func firstBlockTimes(r io.ReaderAt, start, from, end, scaleNS int64, want map[uint64]bool) (map[uint64]int64, error) {
	out := make(map[uint64]int64, len(want))
	if len(want) == 0 {
		return out, nil
	}
	firstCluster := true
	off := start
	for off < end && len(out) < len(want) {
		e, err := readElem(r, off)
		if err != nil {
			return out, err
		}
		if off == start && e.id != idCluster {
			return out, corrupt("segment start %d is not a Cluster (element 0x%X)", start, e.id)
		}
		if e.id != idCluster {
			if e.unknown {
				break
			}
			off = e.end()
			continue
		}
		if e.unknown {
			break
		}
		clEnd := e.end()
		if clEnd > end {
			clEnd = end
		}
		var clusterTime int64 = -1
		err = forChildren(r, e.payload, clEnd, func(c elem) error {
			switch c.id {
			case idTimecode:
				v, err := uintPayload(r, c)
				if err != nil {
					return err
				}
				clusterTime = int64(v)
			case idSimpleBlock:
				if firstCluster && c.off < from {
					return nil
				}
				if clusterTime < 0 {
					return nil
				}
				track, tc, _, err := readBlockHeader(r, c.payload, c.end())
				if err != nil {
					return nil // tolerate unreadable blocks
				}
				if want[track] {
					if _, ok := out[track]; !ok {
						out[track] = (clusterTime + tc) * scaleNS
					}
				}
			case idBlockGroup:
				if firstCluster && c.off < from {
					return nil
				}
				return forChildren(r, c.payload, c.end(), func(g elem) error {
					if g.id != idBlock || clusterTime < 0 {
						return nil
					}
					track, tc, _, err := readBlockHeader(r, g.payload, g.end())
					if err != nil {
						return nil
					}
					if want[track] {
						if _, ok := out[track]; !ok {
							out[track] = (clusterTime + tc) * scaleNS
						}
					}
					return nil
				})
			}
			if len(out) == len(want) {
				return errAnchorsDone
			}
			return nil
		})
		if err != nil && err != errAnchorsDone {
			return out, err
		}
		if len(out) == len(want) {
			return out, nil
		}
		firstCluster = false
		off = clEnd
	}
	return out, nil
}

// trackTimescales maps track_ID → media timescale (mdhd) from an init segment.
func trackTimescales(init []byte) map[uint32]uint64 {
	out := map[uint32]uint64{}
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
			var scale uint64
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
						if s.typ != "mdhd" {
							continue
						}
						p := s.off + s.hdr
						if p+4 > s.off+s.size {
							continue
						}
						switch init[p] {
						case 0:
							if p+16 <= s.off+s.size {
								scale = uint64(binary.BigEndian.Uint32(init[p+12:]))
							}
						case 1:
							if p+24 <= s.off+s.size {
								scale = uint64(binary.BigEndian.Uint32(init[p+20:]))
							}
						}
					}
				}
			}
			if track != 0 && scale != 0 {
				out[track] = scale
			}
		}
	}
	return out
}
