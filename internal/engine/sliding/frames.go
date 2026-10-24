package sliding

import (
	"io"
)

// frameScan describes the complete MPEG audio frames in a written prefix.
type frameScan struct {
	Complete    int   // number of complete frames
	End         int64 // byte offset just past the last complete frame
	Samples     int64 // per-channel PCM samples covered by those frames
	LastSamples int64 // samples contributed by the final complete frame
}

// scanMP3Frames walks [0, limit) of r and measures the stream's frame
// structure. LAME output to a pipe carries no Xing header, so this scan is
// what tells a splice-resume exactly where the last whole frame ends.
func scanMP3Frames(r io.ReaderAt, limit int64) frameScan {
	var scan frameScan
	off := id3v2Skip(r, limit)
	var hdr [4]byte
	for off+4 <= limit {
		if _, err := r.ReadAt(hdr[:], off); err != nil {
			break
		}
		length, samples, ok := frameHeader(hdr)
		if !ok || off+length > limit {
			break
		}
		scan.Complete++
		scan.Samples += samples
		scan.LastSamples = samples
		scan.End = off + length
		off += length
	}
	return scan
}

// id3v2Skip returns the first byte after an ID3v2 tag, or 0 when absent.
func id3v2Skip(r io.ReaderAt, limit int64) int64 {
	var h [10]byte
	if limit < 10 {
		return 0
	}
	if _, err := r.ReadAt(h[:], 0); err != nil || string(h[0:3]) != "ID3" {
		return 0
	}
	size := int64(h[6]&0x7F)<<21 | int64(h[7]&0x7F)<<14 | int64(h[8]&0x7F)<<7 | int64(h[9]&0x7F)
	total := 10 + size
	if total < 0 || total > limit {
		return 0
	}
	return total
}

var (
	bitratesV1L3  = [16]int{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}
	bitratesV2L3  = [16]int{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0}
	sampleRateV1  = [3]int{44100, 48000, 32000}
	sampleRateV2  = [3]int{22050, 24000, 16000}
	sampleRateV25 = [3]int{11025, 12000, 8000}
)

// frameHeader parses a 4-byte MPEG Layer III frame header and returns the
// frame length in bytes and the per-channel sample count.
func frameHeader(h [4]byte) (length int64, samples int64, ok bool) {
	if h[0] != 0xFF || h[1]&0xE0 != 0xE0 {
		return 0, 0, false
	}
	version := (h[1] >> 3) & 0x03
	layer := (h[1] >> 1) & 0x03
	if version == 1 || layer != 1 { // reserved version, or not Layer III
		return 0, 0, false
	}
	bitrateIndex := int(h[2] >> 4)
	sampleRateIndex := int(h[2]>>2) & 0x03
	if bitrateIndex == 0 || bitrateIndex == 15 || sampleRateIndex == 3 {
		return 0, 0, false
	}
	padding := int64(h[2]>>1) & 0x01

	var (
		bitrate int
		rate    int
	)
	switch version {
	case 3: // MPEG 1
		bitrate = bitratesV1L3[bitrateIndex]
		rate = sampleRateV1[sampleRateIndex]
		samples = 1152
	case 2: // MPEG 2
		bitrate = bitratesV2L3[bitrateIndex]
		rate = sampleRateV2[sampleRateIndex]
		samples = 576
	default: // MPEG 2.5
		bitrate = bitratesV2L3[bitrateIndex]
		rate = sampleRateV25[sampleRateIndex]
		samples = 576
	}
	if bitrate == 0 || rate == 0 {
		return 0, 0, false
	}
	coeff := int64(144)
	if version != 3 {
		coeff = 72
	}
	length = coeff*int64(bitrate)*1000/int64(rate) + padding
	if length < 4 || length > 1441+1 {
		return 0, 0, false
	}
	return length, samples, true
}
