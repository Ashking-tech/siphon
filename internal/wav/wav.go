// Package wav parses RIFF/WAVE files: the PCM properties the SlidingEngine
// needs, the exact byte range of the audio data (for the reclaim cursor), the
// sample values for the R ≤ 0.90 gate, and the metadata chunks whose loss the
// pre-flight gate must report.
package wav

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"siphon/internal/siphonerr"
)

const (
	// maxInfoChunk caps how much of a metadata chunk is read into memory.
	maxInfoChunk = 4 << 20

	fmtTagPCM        = 0x0001
	fmtTagExtensible = 0xFFFE
)

// Tag is one LIST/INFO subchunk (INAM, IART, ...).
type Tag struct {
	ID    string `json:"id"`
	Value string `json:"value,omitempty"`
}

// Info is the parsed view of a WAVE file.
type Info struct {
	Channels      int
	SampleRate    int
	BitsPerSample int
	ByteRate      int64
	BlockAlign    int
	DataOffset    int64
	DataSize      int64
	FileSize      int64
	Duration      time.Duration
	Tags          []Tag
	HasBext       bool
	Warnings      []string
}

// HasMetadata reports whether the file carries metadata Siphon cannot put
// into an MP3: LIST/INFO tags or a Broadcast Wave `bext` chunk.
func (i *Info) HasMetadata() bool {
	return len(i.Tags) > 0 || i.HasBext
}

// MetadataNames lists the metadata present, for inspect and refusals.
func (i *Info) MetadataNames() []string {
	var out []string
	for _, t := range i.Tags {
		out = append(out, t.ID)
	}
	if i.HasBext {
		out = append(out, "bext")
	}
	return out
}

// Parse opens path and reads its WAVE structure.
func Parse(path string) (*Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot stat %s", path)
	}
	return ParseReader(f, info.Size(), path)
}

// ParseReader reads a WAVE structure from r. name is used in error messages.
// PCM and WAVE_FORMAT_EXTENSIBLE-with-PCM-subformat are accepted; every other
// format tag is refused as unsupported.
func ParseReader(r io.ReaderAt, size int64, name string) (*Info, error) {
	if size < 12 {
		return nil, corrupt("%s is too small to be a WAVE file", name)
	}

	var head [12]byte
	if err := readAt(r, head[:], 0); err != nil {
		return nil, err
	}
	if string(head[0:4]) != "RIFF" || string(head[8:12]) != "WAVE" {
		return nil, corrupt("%s is not a RIFF/WAVE file", name)
	}

	info := &Info{FileSize: size}
	var (
		haveFmt  bool
		haveData bool
	)
	for off := int64(12); off+8 <= size; {
		var ch [8]byte
		if err := readAt(r, ch[:], off); err != nil {
			return nil, err
		}
		id := string(ch[0:4])
		declared := int64(binary.LittleEndian.Uint32(ch[4:8]))
		length := declared
		payload := off + 8
		if payload+length > size {
			if id == "data" && haveFmt && !haveData {
				// Streamed writers leave 0xFFFFFFFF, or the file was cut
				// short; either way the audio runs to the end of the file.
				info.DataOffset = payload
				info.DataSize = size - payload
				haveData = true
				if declared == 0xFFFFFFFF {
					info.Warnings = append(info.Warnings,
						"data chunk size is a streamed placeholder; using the bytes present")
				} else {
					info.Warnings = append(info.Warnings,
						fmt.Sprintf("data chunk declares %d bytes but the file has %d; using the bytes present",
							declared, size-payload))
				}
				break
			}
			// A trailing chunk with a bogus length ends the walk; the audio
			// data has already been located and bounded by the file size.
			info.Warnings = append(info.Warnings,
				fmt.Sprintf("trailing %q chunk declares %d bytes but only %d remain; ignoring it",
					id, declared, size-payload))
			break
		}

		switch id {
		case "fmt ":
			if haveData {
				return nil, corrupt("%s has its fmt chunk after the data chunk", name)
			}
			if err := parseFmt(r, payload, length, info, name); err != nil {
				return nil, err
			}
			haveFmt = true
		case "data":
			if !haveFmt {
				return nil, corrupt("%s has no fmt chunk before its data chunk", name)
			}
			if haveData {
				return nil, corrupt("%s has more than one data chunk", name)
			}
			info.DataOffset = payload
			info.DataSize = length
			haveData = true
		case "LIST":
			tags, err := parseList(r, payload, length)
			if err != nil {
				return nil, err
			}
			info.Tags = append(info.Tags, tags...)
		case "bext":
			info.HasBext = true
		}

		off = payload + length
		if length&1 == 1 {
			off++ // chunks are word-aligned
		}
	}

	if !haveFmt {
		return nil, corrupt("%s has no fmt chunk", name)
	}
	if !haveData {
		return nil, corrupt("%s has no data chunk", name)
	}

	available := size - info.DataOffset
	switch {
	case info.DataSize == 0:
		return nil, corrupt("%s has an empty data chunk", name)
	case info.DataSize > available:
		// Defensive: the walk already clamps, but keep the invariant local.
		info.DataSize = available
	}

	if info.ByteRate <= 0 || info.BlockAlign <= 0 {
		return nil, corrupt("%s has a zero byte rate or block alignment", name)
	}
	info.Duration = time.Duration(float64(info.DataSize) / float64(info.ByteRate) * float64(time.Second))
	return info, nil
}

// FmtChunk rebuilds a canonical 16-byte PCM fmt chunk from the parsed values.
// The reclaim resume path feeds lame a synthetic WAV header built from this.
func (i *Info) FmtChunk() []byte {
	b := make([]byte, 16)
	binary.LittleEndian.PutUint16(b[0:2], fmtTagPCM)
	binary.LittleEndian.PutUint16(b[2:4], uint16(i.Channels))
	binary.LittleEndian.PutUint32(b[4:8], uint32(i.SampleRate))
	binary.LittleEndian.PutUint32(b[8:12], uint32(i.ByteRate))
	binary.LittleEndian.PutUint16(b[12:14], uint16(i.BlockAlign))
	binary.LittleEndian.PutUint16(b[14:16], uint16(i.BitsPerSample))
	return b
}

// Header builds a complete WAVE header for len(payload) bytes of PCM, in the
// same canonical form as FmtChunk.
func (i *Info) Header(payload int64) []byte {
	b := make([]byte, 44)
	copy(b[0:4], "RIFF")
	binary.LittleEndian.PutUint32(b[4:8], uint32(36+payload))
	copy(b[8:12], "WAVE")
	copy(b[12:16], "fmt ")
	binary.LittleEndian.PutUint32(b[16:20], 16)
	copy(b[20:36], i.FmtChunk())
	copy(b[36:40], "data")
	binary.LittleEndian.PutUint32(b[40:44], uint32(payload))
	return b
}

func parseFmt(r io.ReaderAt, off, length int64, info *Info, name string) error {
	if length < 16 {
		return corrupt("%s has a %d-byte fmt chunk (need at least 16)", name, length)
	}
	buf := make([]byte, length)
	if err := readAt(r, buf, off); err != nil {
		return err
	}
	tag := binary.LittleEndian.Uint16(buf[0:2])
	switch tag {
	case fmtTagPCM:
	case fmtTagExtensible:
		if length < 40 {
			return corrupt("%s has a WAVE_FORMAT_EXTENSIBLE fmt chunk shorter than 40 bytes", name)
		}
		if sub := binary.LittleEndian.Uint16(buf[24:26]); sub != fmtTagPCM {
			return siphonerr.New(siphonerr.FormatUnsupported,
				"%s uses a non-PCM WAVE_FORMAT_EXTENSIBLE subformat %#x", name, sub)
		}
	default:
		return siphonerr.New(siphonerr.FormatUnsupported,
			"%s uses WAV format tag %#x; only integer PCM is supported", name, tag)
	}

	info.Channels = int(binary.LittleEndian.Uint16(buf[2:4]))
	info.SampleRate = int(binary.LittleEndian.Uint32(buf[4:8]))
	info.ByteRate = int64(binary.LittleEndian.Uint32(buf[8:12]))
	info.BlockAlign = int(binary.LittleEndian.Uint16(buf[12:14]))
	info.BitsPerSample = int(binary.LittleEndian.Uint16(buf[14:16]))

	if info.Channels <= 0 {
		return corrupt("%s declares zero channels", name)
	}
	if info.SampleRate <= 0 {
		return corrupt("%s declares a zero sample rate", name)
	}
	if info.BitsPerSample <= 0 || info.BitsPerSample > 64 {
		return corrupt("%s declares %d bits per sample", name, info.BitsPerSample)
	}
	return nil
}

func parseList(r io.ReaderAt, off, length int64) ([]Tag, error) {
	if length < 4 {
		return nil, nil
	}
	if length > maxInfoChunk {
		return nil, nil // unreasonably large metadata block: ignore it
	}
	buf := make([]byte, length)
	if err := readAt(r, buf, off); err != nil {
		return nil, err
	}
	if string(buf[0:4]) != "INFO" {
		return nil, nil
	}
	var tags []Tag
	for i := int64(4); i+8 <= length; {
		id := string(buf[i : i+4])
		size := int64(binary.LittleEndian.Uint32(buf[i+4 : i+8]))
		i += 8
		if size < 0 || i+size > length {
			break
		}
		if id != "" && id[0] != 0 {
			value := strings.TrimRight(string(buf[i:i+size]), "\x00")
			value = strings.TrimSpace(value)
			tags = append(tags, Tag{ID: id, Value: value})
		}
		i += size
		if size&1 == 1 {
			i++
		}
	}
	return tags, nil
}

func readAt(r io.ReaderAt, buf []byte, off int64) error {
	if _, err := r.ReadAt(buf, off); err != nil {
		return siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot read WAVE structure")
	}
	return nil
}

func corrupt(format string, args ...any) error {
	return siphonerr.New(siphonerr.SourceCorrupt, format, args...)
}
