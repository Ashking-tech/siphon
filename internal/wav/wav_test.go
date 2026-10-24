package wav

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"siphon/internal/siphonerr"
)

type chunk struct {
	id      string
	payload []byte
}

func buildWAV(t *testing.T, fmtPayload []byte, chunks []chunk) []byte {
	t.Helper()
	var body bytes.Buffer
	body.WriteString("WAVE")
	writeChunk := func(id string, payload []byte) {
		var h [8]byte
		copy(h[0:4], id)
		binary.LittleEndian.PutUint32(h[4:8], uint32(len(payload)))
		body.Write(h[:])
		body.Write(payload)
		if len(payload)%2 == 1 {
			body.WriteByte(0)
		}
	}
	writeChunk("fmt ", fmtPayload)
	for _, c := range chunks {
		writeChunk(c.id, c.payload)
	}
	out := make([]byte, 0, body.Len()+8)
	var h [8]byte
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], uint32(body.Len()))
	out = append(out, h[:]...)
	out = append(out, body.Bytes()...)
	return out
}

func pcmFmt(tag, channels, rate, bits int) []byte {
	f := make([]byte, 16)
	binary.LittleEndian.PutUint16(f[0:2], uint16(tag))
	binary.LittleEndian.PutUint16(f[2:4], uint16(channels))
	binary.LittleEndian.PutUint32(f[4:8], uint32(rate))
	block := channels * bits / 8
	binary.LittleEndian.PutUint32(f[8:12], uint32(rate*block))
	binary.LittleEndian.PutUint16(f[12:14], uint16(block))
	binary.LittleEndian.PutUint16(f[14:16], uint16(bits))
	return f
}

func extensibleFmt(channels, rate, bits int) []byte {
	f := pcmFmt(fmtTagExtensible, channels, rate, bits)
	f = append(f, make([]byte, 24)...)                    // cbSize + extension
	binary.LittleEndian.PutUint16(f[16:18], 22)           // cbSize
	binary.LittleEndian.PutUint16(f[18:20], uint16(bits)) // valid bits
	binary.LittleEndian.PutUint32(f[20:24], 3)            // FL|FR mask
	binary.LittleEndian.PutUint16(f[24:26], fmtTagPCM)    // PCM subformat GUID
	return f
}

func writeTemp(t *testing.T, b []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.wav")
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParseStereoPCM(t *testing.T) {
	data := make([]byte, 44100*2*2) // 1 s stereo 16-bit
	path := writeTemp(t, buildWAV(t, pcmFmt(fmtTagPCM, 2, 44100, 16), []chunk{{"data", data}}))

	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.Channels != 2 || info.SampleRate != 44100 || info.BitsPerSample != 16 {
		t.Fatalf("unexpected format: %+v", info)
	}
	if info.DataOffset != 44 || info.DataSize != int64(len(data)) {
		t.Fatalf("data range = [%d, %d), want [44, %d)", info.DataOffset, info.DataOffset+info.DataSize, 44+len(data))
	}
	if info.Duration != time.Second {
		t.Fatalf("duration = %v, want 1s", info.Duration)
	}
	if info.HasMetadata() {
		t.Fatal("plain PCM reported metadata")
	}
}

func TestParseExtensiblePCM(t *testing.T) {
	data := make([]byte, 3*44100*2) // ~0.5 s stereo 24-bit
	path := writeTemp(t, buildWAV(t, extensibleFmt(2, 44100, 24), []chunk{{"data", data}}))
	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.BitsPerSample != 24 || info.BlockAlign != 6 {
		t.Fatalf("unexpected format: %+v", info)
	}
}

func TestParseRefusesFloat(t *testing.T) {
	path := writeTemp(t, buildWAV(t, pcmFmt(3, 2, 44100, 32), []chunk{{"data", make([]byte, 4096)}}))
	_, err := Parse(path)
	if !siphonerr.IsCode(err, siphonerr.FormatUnsupported) {
		t.Fatalf("err = %v, want E_FORMAT_UNSUPPORTED", err)
	}
}

func TestParseMultichannelIsParsed(t *testing.T) {
	// Parsing succeeds; the pre-flight gate is what refuses >2 channels.
	data := make([]byte, 6*44100*2)
	path := writeTemp(t, buildWAV(t, pcmFmt(fmtTagPCM, 6, 44100, 16), []chunk{{"data", data}}))
	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.Channels != 6 {
		t.Fatalf("channels = %d, want 6", info.Channels)
	}
}

func TestParseInfoTagsAndBext(t *testing.T) {
	var list bytes.Buffer
	list.WriteString("INFO")
	addTag := func(id, value string) {
		var h [8]byte
		copy(h[0:4], id)
		binary.LittleEndian.PutUint32(h[4:8], uint32(len(value)+1))
		list.Write(h[:])
		list.WriteString(value)
		list.WriteByte(0)
		if (len(value)+1)%2 == 1 {
			list.WriteByte(0)
		}
	}
	addTag("INAM", "A tone")
	addTag("IART", "Siphon")

	path := writeTemp(t, buildWAV(t, pcmFmt(fmtTagPCM, 1, 8000, 16), []chunk{
		{"LIST", list.Bytes()},
		{"bext", make([]byte, 602)},
		{"data", make([]byte, 1600)},
	}))
	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(info.Tags) != 2 || info.Tags[0].ID != "INAM" || info.Tags[0].Value != "A tone" {
		t.Fatalf("tags = %+v", info.Tags)
	}
	if !info.HasBext {
		t.Fatal("bext not detected")
	}
	if names := info.MetadataNames(); strings.Join(names, ",") != "INAM,IART,bext" {
		t.Fatalf("names = %v", names)
	}
}

func TestParseOddChunkPadding(t *testing.T) {
	path := writeTemp(t, buildWAV(t, pcmFmt(fmtTagPCM, 1, 8000, 16), []chunk{
		{"JUNK", []byte{1, 2, 3}},
		{"data", make([]byte, 800)},
	}))
	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.DataSize != 800 {
		t.Fatalf("data size = %d, want 800", info.DataSize)
	}
}

func TestParseRefusesMissingData(t *testing.T) {
	path := writeTemp(t, buildWAV(t, pcmFmt(fmtTagPCM, 1, 8000, 16), nil))
	_, err := Parse(path)
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("err = %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestParseRefusesFmtAfterData(t *testing.T) {
	// A fmt chunk after the data chunk is malformed.
	var body bytes.Buffer
	body.WriteString("WAVE")
	for _, c := range []chunk{{"data", make([]byte, 800)}, {"fmt ", pcmFmt(fmtTagPCM, 1, 8000, 16)}} {
		var h [8]byte
		copy(h[0:4], c.id)
		binary.LittleEndian.PutUint32(h[4:8], uint32(len(c.payload)))
		body.Write(h[:])
		body.Write(c.payload)
	}
	var h [8]byte
	copy(h[0:4], "RIFF")
	binary.LittleEndian.PutUint32(h[4:8], uint32(body.Len()))
	out := append(h[:], body.Bytes()...)
	path := writeTemp(t, out)
	_, err := Parse(path)
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("err = %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestParseStreamedDataPlaceholder(t *testing.T) {
	raw := buildWAV(t, pcmFmt(fmtTagPCM, 1, 8000, 16), []chunk{{"data", make([]byte, 800)}})
	// Overwrite the data chunk length with 0xFFFFFFFF.
	binary.LittleEndian.PutUint32(raw[40:44], 0xFFFFFFFF)
	path := writeTemp(t, raw)
	info, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if info.DataSize != 800 {
		t.Fatalf("data size = %d, want 800", info.DataSize)
	}
	if len(info.Warnings) == 0 {
		t.Fatal("expected a streamed-placeholder warning")
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	info := &Info{Channels: 2, SampleRate: 44100, BitsPerSample: 16, ByteRate: 176400, BlockAlign: 4}
	payload := make([]byte, 4000)
	raw := append(info.Header(int64(len(payload))), payload...)
	path := writeTemp(t, raw)
	parsed, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse(synthetic header): %v", err)
	}
	if parsed.Channels != 2 || parsed.SampleRate != 44100 || parsed.BitsPerSample != 16 {
		t.Fatalf("round trip lost fields: %+v", parsed)
	}
	if parsed.DataOffset != 44 || parsed.DataSize != int64(len(payload)) {
		t.Fatalf("round trip data range wrong: %+v", parsed)
	}
	if parsed.Duration != time.Duration(float64(len(payload))/176400*float64(time.Second)) {
		t.Fatalf("duration = %v", parsed.Duration)
	}
}

func TestParseTruncatedHeader(t *testing.T) {
	path := writeTemp(t, []byte("RIFF\x00\x00\x00\x00WA"))
	_, err := Parse(path)
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("err = %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestParseEmptyDataRefused(t *testing.T) {
	path := writeTemp(t, buildWAV(t, pcmFmt(fmtTagPCM, 1, 8000, 16), []chunk{{"data", nil}}))
	_, err := Parse(path)
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("err = %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestParseNonWAV(t *testing.T) {
	path := writeTemp(t, []byte("this is not a wav file at all"))
	_, err := Parse(path)
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("err = %v, want E_SOURCE_CORRUPT", err)
	}
}
