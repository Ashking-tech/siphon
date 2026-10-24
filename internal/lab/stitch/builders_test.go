package stitch

import (
	"encoding/binary"
	"math"
)

// This file holds the byte-level builders shared by the spike's unit tests:
// just enough EBML and ISO-BMFF to exercise the parsers without a media file.

// concat joins byte slices.
func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

// --- EBML ---

// ebmlID encodes an element ID with its marker bits.
func ebmlID(id uint64) []byte {
	n := 1
	for n < 8 && id>>(8*uint(n)) != 0 {
		n++
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[n-1-i] = byte(id >> (8 * uint(i)))
	}
	return b
}

// ebmlSize encodes a size vint for a payload of n bytes.
func ebmlSize(n uint64) []byte {
	for sl := 1; sl <= 8; sl++ {
		if n <= uint64(1)<<(7*uint(sl))-1 {
			b := make([]byte, sl)
			v := n | uint64(1)<<(7*uint(sl))
			for i := 0; i < sl; i++ {
				b[sl-1-i] = byte(v >> (8 * uint(i)))
			}
			return b
		}
	}
	panic("EBML size too large")
}

// ebmlElem builds one element: ID, minimal size vint, payload.
func ebmlElem(id uint64, parts ...[]byte) []byte {
	payload := concat(parts...)
	out := append(ebmlID(id), ebmlSize(uint64(len(payload)))...)
	return append(out, payload...)
}

// ebmlUint builds an unsigned-integer element.
func ebmlUint(id uint64, v uint64) []byte {
	n := 1
	for n < 8 && v>>(8*uint(n)) != 0 {
		n++
	}
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		b[n-1-i] = byte(v >> (8 * uint(i)))
	}
	return ebmlElem(id, b)
}

// ebmlStr builds a string element.
func ebmlStr(id uint64, s string) []byte { return ebmlElem(id, []byte(s)) }

// ebmlFloat builds a float64 element.
func ebmlFloat(id uint64, v float64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
	return ebmlElem(id, b[:])
}

// mkvBlock builds a SimpleBlock/Block payload header for track 1..126 at
// timecode tc. No frame data is needed: only the header is parsed.
func mkvBlock(track byte, tc int16) []byte {
	b := []byte{0x80 | track, 0, 0, 0x80}
	binary.BigEndian.PutUint16(b[1:], uint16(tc))
	return b
}

// --- ISO-BMFF ---

// boxOf builds one box with the given payload parts.
func boxOf(typ string, parts ...[]byte) []byte {
	body := concat(parts...)
	out := make([]byte, 8+len(body))
	binary.BigEndian.PutUint32(out, uint32(len(out)))
	copy(out[4:8], typ)
	copy(out[8:], body)
	return out
}

// fullBox builds one full box (version + 24-bit flags + payload parts).
func fullBox(typ string, version byte, flags uint32, parts ...[]byte) []byte {
	hdr := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	return boxOf(typ, append([][]byte{hdr}, parts...)...)
}

func be32(v uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	return b[:]
}

func be64(v uint64) []byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], v)
	return b[:]
}

// testTrun builds a trun box. samples are {duration, size, composition offset}
// and each field is emitted according to flags.
func testTrun(version byte, flags uint32, dataOffset, firstFlags uint32, samples [][3]int32) []byte {
	payload := []byte{version, byte(flags >> 16), byte(flags >> 8), byte(flags)}
	payload = append(payload, be32(uint32(len(samples)))...)
	if flags&trunDataOffset != 0 {
		payload = append(payload, be32(dataOffset)...)
	}
	if flags&trunFirstSampleFlags != 0 {
		payload = append(payload, be32(firstFlags)...)
	}
	for _, s := range samples {
		if flags&trunSampleDuration != 0 {
			payload = append(payload, be32(uint32(s[0]))...)
		}
		if flags&trunSampleSize != 0 {
			payload = append(payload, be32(uint32(s[1]))...)
		}
		if flags&trunSampleFlags != 0 {
			payload = append(payload, be32(0)...)
		}
		if flags&trunSampleCTS != 0 {
			payload = append(payload, be32(uint32(s[2]))...)
		}
	}
	return boxOf("trun", payload)
}

// testTraf builds a traf with tfhd, tfdt v1 and one trun.
func testTraf(track uint32, tfdt uint64, trun []byte) []byte {
	tfhd := fullBox("tfhd", 0, 0x020038, be32(track), be32(0), be32(0), be32(0))
	base := fullBox("tfdt", 1, 0, be64(tfdt))
	return boxOf("traf", tfhd, base, trun)
}

// testMoof builds a moof with the given sequence number and trafs.
func testMoof(seq uint32, trafs ...[]byte) []byte {
	return boxOf("moof", fullBox("mfhd", 0, 0, be32(seq)), concat(trafs...))
}

// testTrak builds a trak with the tkhd/mdhd/hdlr fields the spike reads.
func testTrak(track uint32, scale uint32, handler string) []byte {
	tkhd := fullBox("tkhd", 0, 1,
		be32(0), be32(0), be32(track), be32(0), be32(0), make([]byte, 68))
	mdhd := fullBox("mdhd", 0, 0,
		be32(0), be32(0), be32(scale), be32(0), be32(0))
	hdlr := fullBox("hdlr", 0, 0, be32(0), []byte(handler), make([]byte, 12))
	return boxOf("trak", tkhd, boxOf("mdia", mdhd, hdlr))
}
