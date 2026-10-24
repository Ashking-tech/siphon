package sliding

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
	"siphon/internal/wav"
)

// --- helpers ---------------------------------------------------------------

func requireTools(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			t.Skipf("skipping: %s is not installed", n)
		}
	}
}

// wavTone builds a canonical 16-bit PCM WAV with a 440 Hz sine.
func wavTone(channels, sampleRate int, seconds float64) []byte {
	frames := int(float64(sampleRate) * seconds)
	data := make([]byte, frames*channels*2)
	for i := 0; i < frames; i++ {
		v := uint16(int16(math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)) * 12000))
		for c := 0; c < channels; c++ {
			binary.LittleEndian.PutUint16(data[(i*channels+c)*2:], v)
		}
	}
	var b bytes.Buffer
	b.WriteString("RIFF")
	_ = binary.Write(&b, binary.LittleEndian, uint32(36+len(data)))
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	_ = binary.Write(&b, binary.LittleEndian, uint32(16))
	_ = binary.Write(&b, binary.LittleEndian, uint16(1))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&b, binary.LittleEndian, uint32(sampleRate*channels*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	_ = binary.Write(&b, binary.LittleEndian, uint32(len(data)))
	b.Write(data)
	return b.Bytes()
}

func writeWAV(t *testing.T, dir, name string, channels, sampleRate int, seconds float64) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, wavTone(channels, sampleRate, seconds), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func audioJob(t *testing.T, src string, mode formats.Mode) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.WAV, formats.MP3)
	if !ok {
		t.Fatal("pair wav → mp3 missing")
	}
	ext := filepath.Ext(src)
	return &core.Job{
		SourcePath: src,
		TargetPath: strings.TrimSuffix(src, ext) + ".mp3",
		Source:     formats.WAV,
		Target:     formats.MP3,
		Pair:       pair,
		Mode:       mode,
		Yes:        true,
		Timeout:    time.Minute,
		AllowRoot:  os.Geteuid() == 0,
	}
}

// lameToBuffer encodes a file through the same pipe invocation reclaim uses.
func lameToBuffer(t *testing.T, src string) []byte {
	t.Helper()
	f, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cmd := exec.Command("lame", "--silent", "-V", "2", "-", "-")
	cmd.Stdin = f
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("lame: %v (%s)", err, stderr.String())
	}
	return out.Bytes()
}

// frameEnds lists the byte offset after each complete frame of data.
func frameEnds(data []byte) []int {
	var ends []int
	off := 0
	for off+4 <= len(data) {
		var hdr [4]byte
		copy(hdr[:], data[off:off+4])
		length, _, ok := frameHeader(hdr)
		if !ok || off+int(length) > len(data) {
			break
		}
		off += int(length)
		ends = append(ends, off)
	}
	return ends
}

func decodedSeconds(t *testing.T, path string, channels, rate int) float64 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	n, err := decodeMP3(context.Background(), f, true)
	if err != nil {
		t.Fatal(err)
	}
	if channels <= 0 || rate <= 0 {
		t.Fatal("decodedSeconds needs the channel count and rate")
	}
	return float64(n) / float64(2*channels*rate)
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".siphon-") {
			t.Fatalf("leftover temp %s", e.Name())
		}
	}
}

// --- frame scanner ---------------------------------------------------------

func TestScanMP3Frames(t *testing.T) {
	mpeg1 := func(bitrateIndex, rateIndex, padding byte) []byte {
		return []byte{0xFF, 0xFB, bitrateIndex<<4 | rateIndex<<2 | padding<<1, 0xC4}
	}
	mpeg2 := func(bitrateIndex, rateIndex, padding byte) []byte {
		return []byte{0xFF, 0xF3, bitrateIndex<<4 | rateIndex<<2 | padding<<1, 0xC4}
	}
	v1 := mpeg1(9, 0, 0)  // 128 kbps, 44.1 kHz, no padding → 417 bytes
	v1p := mpeg1(9, 0, 1) // padded → 418 bytes
	v2 := mpeg2(8, 0, 0)  // 64 kbps, 22.05 kHz → 208 bytes

	data := bytes.Join([][]byte{
		v1, bytes.Repeat([]byte{0}, 417-4),
		v1p, bytes.Repeat([]byte{0}, 418-4),
		v2, bytes.Repeat([]byte{0}, 208-4),
	}, nil)

	scan := scanMP3Frames(bytes.NewReader(data), int64(len(data)))
	if scan.Complete != 3 {
		t.Fatalf("complete = %d, want 3", scan.Complete)
	}
	if want := int64(417 + 418 + 208); scan.End != want {
		t.Fatalf("end = %d, want %d", scan.End, want)
	}
	if want := int64(1152 + 1152 + 576); scan.Samples != want {
		t.Fatalf("samples = %d, want %d", scan.Samples, want)
	}
	if scan.LastSamples != 576 {
		t.Fatalf("lastSamples = %d, want 576", scan.LastSamples)
	}

	// A truncated trailing frame is not counted.
	truncated := append(append([]byte{}, data...), v1...)
	scan = scanMP3Frames(bytes.NewReader(truncated), int64(len(truncated)))
	if scan.Complete != 3 {
		t.Fatalf("complete = %d after truncation, want 3", scan.Complete)
	}

	// Garbage is not a frame.
	scan = scanMP3Frames(bytes.NewReader([]byte("not mp3 at all")), 16)
	if scan.Complete != 0 || scan.End != 0 {
		t.Fatalf("garbage parsed as frames: %+v", scan)
	}
}

func TestResumePoint(t *testing.T) {
	dir := t.TempDir()
	src := writeWAV(t, dir, "tone.wav", 2, 44100, 2)
	info, err := wav.Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	requireTools(t, "lame")
	ref := lameToBuffer(t, src)
	ends := frameEnds(ref)
	if len(ends) < 10 {
		t.Fatalf("reference has only %d frames", len(ends))
	}
	cut := ends[len(ends)/2]

	f, err := os.OpenFile(src, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteAt(ref[:cut], 0); err != nil {
		t.Fatal(err)
	}

	writePos, pcmStart := resumePoint(f, info, int64(cut))
	if writePos != int64(cut) {
		t.Fatalf("writePos = %d, want %d", writePos, cut)
	}
	// The resume backs off exactly one frame of PCM from what the prefix
	// already covers.
	framesInPrefix := len(frameEnds(ref[:cut]))
	want := info.DataOffset + int64(framesInPrefix-1)*1152*int64(info.BlockAlign)
	if pcmStart != want {
		t.Fatalf("pcmStart = %d, want %d", pcmStart, want)
	}
}

// --- the forward invariant -------------------------------------------------

func TestFlushRespectsMargin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var read atomic.Int64
	var done atomic.Bool
	w := &reclaimWriter{f: f, margin: 1000, readCursor: &read, producerEnd: &done}

	pending := [][]byte{bytes.Repeat([]byte{1}, 600), bytes.Repeat([]byte{2}, 600)}
	wrote, err := w.flush(&pending)
	if err != nil || wrote || len(pending) != 2 {
		t.Fatalf("wrote through the margin: wrote=%v err=%v pending=%d", wrote, err, len(pending))
	}

	read.Store(1600) // 1600-1000 = 600 bytes of headroom
	wrote, err = w.flush(&pending)
	if err != nil || !wrote || len(pending) != 1 || w.writeCursor != 600 {
		t.Fatalf("first chunk should fit: wrote=%v err=%v pending=%d cursor=%d",
			wrote, err, len(pending), w.writeCursor)
	}

	// Endgame: the margin is dropped once the producer has finished.
	done.Store(true)
	wrote, err = w.flush(&pending)
	if err != nil || !wrote || len(pending) != 0 || w.writeCursor != 1200 {
		t.Fatalf("endgame flush failed: wrote=%v err=%v pending=%d cursor=%d",
			wrote, err, len(pending), w.writeCursor)
	}
}

// --- safe mode -------------------------------------------------------------

func TestSafeConvert(t *testing.T) {
	requireTools(t, "lame", "ffmpeg", "ffprobe")
	dir := t.TempDir()
	src := writeWAV(t, dir, "tone.wav", 2, 44100, 2)
	job := audioJob(t, src, formats.ModeSafe)

	res, err := New().Convert(context.Background(), job, nil, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(job.TargetPath); err != nil {
		t.Fatalf("target missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still present: %v", err)
	}
	seconds := decodedSeconds(t, job.TargetPath, 2, 44100)
	if math.Abs(seconds-2) > 0.05 {
		t.Fatalf("decoded %.3fs, want 2s", seconds)
	}
	assertNoTemps(t, dir)
}

func TestSafeRefusesToOverwrite(t *testing.T) {
	requireTools(t, "lame", "ffmpeg", "ffprobe")
	dir := t.TempDir()
	src := writeWAV(t, dir, "tone.wav", 1, 44100, 1)
	bystander := strings.TrimSuffix(src, ".wav") + ".mp3"
	if err := os.WriteFile(bystander, []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	job := audioJob(t, src, formats.ModeSafe)
	_, err := New().Convert(context.Background(), job, nil, progress.Noop{})
	if !siphonerr.IsCode(err, siphonerr.TargetExists) {
		t.Fatalf("got %v, want E_TARGET_EXISTS", err)
	}
	if !siphonerr.SourceUntouched(err) {
		t.Fatalf("source must be untouched: %v", err)
	}
	got, err := os.ReadFile(bystander)
	if err != nil || string(got) != "mine" {
		t.Fatalf("bystander replaced: %q %v", got, err)
	}
}

// --- reclaim mode ----------------------------------------------------------

func TestReclaimConvert(t *testing.T) {
	requireTools(t, "lame", "ffmpeg", "ffprobe")
	old := marginOverride
	marginOverride = 32 << 10 // force mid-stream writes with the invariant
	defer func() { marginOverride = old }()

	dir := t.TempDir()
	src := writeWAV(t, dir, "tone.wav", 2, 44100, 4)
	job := audioJob(t, src, formats.ModeReclaim)

	res, err := New().Convert(context.Background(), job, nil, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still present: %v", err)
	}
	fi, err := os.Stat(job.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Size() >= 1<<20 {
		t.Fatalf("target is %d bytes; the WAV data was not reclaimed", fi.Size())
	}
	seconds := decodedSeconds(t, job.TargetPath, 2, 44100)
	if math.Abs(seconds-4) > 0.08 {
		t.Fatalf("decoded %.3fs, want 4s", seconds)
	}
	if paths, _ := journal.Find(dir, "tone"); len(paths) != 0 {
		t.Fatalf("journal left behind: %v", paths)
	}
	assertNoTemps(t, dir)
}

// --- crash resume ----------------------------------------------------------

func TestResumeReclaimSplice(t *testing.T) {
	requireTools(t, "lame", "ffmpeg", "ffprobe")
	dir := t.TempDir()
	src := writeWAV(t, dir, "tone.wav", 2, 44100, 6)
	info, err := wav.Parse(src)
	if err != nil {
		t.Fatal(err)
	}

	// A previous run's output, cut at a frame boundary roughly halfway.
	ref := lameToBuffer(t, src)
	ends := frameEnds(ref)
	if len(ends) < 20 {
		t.Fatalf("reference has only %d frames", len(ends))
	}
	cut := ends[len(ends)/2]

	// Simulate the crash: the prefix sits in the source's head, the source
	// kept its original length, and the journal records the cursors.
	f, err := os.OpenFile(src, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt(ref[:cut], 0); err != nil {
		t.Fatal(err)
	}
	rec, err := sourceJournalRecord(src, f)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	target := strings.TrimSuffix(src, ".wav") + ".mp3"
	store, j, err := journal.Create(dir, "tone")
	if err != nil {
		t.Fatal(err)
	}
	j.Job = journal.Job{
		Engine: "sliding",
		Mode:   "reclaim",
		Source: rec,
		Target: journal.Target{Path: target, Format: "mp3"},
		Params: journal.Params{Audio: &journal.AudioParams{
			DataOffset:    info.DataOffset,
			DataEnd:       info.DataOffset + info.DataSize,
			SampleRate:    info.SampleRate,
			Channels:      info.Channels,
			BitsPerSample: info.BitsPerSample,
			ByteRate:      info.ByteRate,
			BlockAlign:    info.BlockAlign,
		}},
	}
	j.Cursors = journal.Cursors{SourceMtimeNS: rec.MtimeNS, WriteCursor: int64(cut)}
	j.Phase = "encoding"
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	journalPath := store.Path()
	store.Close() // the crash: the process dies, the lock goes with it

	// Recovery opens the journal fresh, like the driver does.
	store2, j2, err := journal.Open(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()

	res, err := ResumeReclaim(context.Background(), store2, j2, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still present: %v", err)
	}
	seconds := decodedSeconds(t, target, 2, 44100)
	if math.Abs(seconds-6) > 0.12 {
		t.Fatalf("spliced output decodes to %.3fs, want 6s", seconds)
	}
	if paths, _ := journal.Find(dir, "tone"); len(paths) != 0 {
		t.Fatalf("journal left behind: %v", paths)
	}
	assertNoTemps(t, dir)
}

// Reclaim must produce exactly the pipe stream LAME writes to stdout; safe
// mode differs only by the Xing/LAME header it can write when the output is
// a real file. The safe file still decodes to the same audio.
func TestReclaimMatchesSafeLength(t *testing.T) {
	requireTools(t, "lame", "ffmpeg", "ffprobe")
	dirSafe := t.TempDir()
	dirRecl := t.TempDir()
	srcSafe := writeWAV(t, dirSafe, "tone.wav", 1, 44100, 3)
	srcRecl := writeWAV(t, dirRecl, "tone.wav", 1, 44100, 3)

	pipe := lameToBuffer(t, srcRecl)

	jobSafe := audioJob(t, srcSafe, formats.ModeSafe)
	if _, err := New().Convert(context.Background(), jobSafe, nil, progress.Noop{}); err != nil {
		t.Fatal(err)
	}
	jobRecl := audioJob(t, srcRecl, formats.ModeReclaim)
	if _, err := New().Convert(context.Background(), jobRecl, nil, progress.Noop{}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(jobRecl.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, pipe) {
		t.Fatalf("reclaim output (%d bytes) differs from the pipe encode (%d bytes)",
			len(got), len(pipe))
	}

	// Safe output: the Xing frame is metadata, so decoding trims it via the
	// gapless tag; both decode within two frames of each other.
	a := decodedSeconds(t, jobSafe.TargetPath, 1, 44100)
	b := decodedSeconds(t, jobRecl.TargetPath, 1, 44100)
	if math.Abs(a-b) > 2*1152.0/44100+0.005 {
		t.Fatalf("safe %.4fs vs reclaim %.4fs", a, b)
	}
}

// --- finalising windows ----------------------------------------------------

// reclaimJournal writes the journal a crashed reclaim job would leave behind.
func reclaimJournal(t *testing.T, dir, stem string, rec journal.Source, info *wav.Info, target string, writeCursor int64, phase string) string {
	t.Helper()
	store, j, err := journal.Create(dir, stem)
	if err != nil {
		t.Fatal(err)
	}
	j.Job = journal.Job{
		Engine: "sliding",
		Mode:   "reclaim",
		Source: rec,
		Target: journal.Target{Path: target, Format: "mp3"},
		Params: journal.Params{Audio: &journal.AudioParams{
			DataOffset:    info.DataOffset,
			DataEnd:       info.DataOffset + info.DataSize,
			SampleRate:    info.SampleRate,
			Channels:      info.Channels,
			BitsPerSample: info.BitsPerSample,
			ByteRate:      info.ByteRate,
			BlockAlign:    info.BlockAlign,
		}},
	}
	j.Cursors = journal.Cursors{SourceMtimeNS: rec.MtimeNS, WriteCursor: writeCursor}
	j.Phase = phase
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	path := store.Path()
	store.Close()
	return path
}

// TestResumeFinalisingWindows covers a crash after the encode is verified and
// the journal says finalising: before the truncate, after it, and after the
// rename. All three must finish to the same file.
func TestResumeFinalisingWindows(t *testing.T) {
	requireTools(t, "lame", "ffmpeg", "ffprobe")
	for _, tc := range []struct {
		name     string
		truncate bool
		renamed  bool
	}{
		{name: "before truncate"},
		{name: "after truncate", truncate: true},
		{name: "after rename", truncate: true, renamed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			src := writeWAV(t, dir, "tone.wav", 2, 44100, 2)
			info, err := wav.Parse(src)
			if err != nil {
				t.Fatal(err)
			}
			mp3 := lameToBuffer(t, src)
			target := strings.TrimSuffix(src, ".wav") + ".mp3"

			// The encode wrote the whole MP3 into the head; the question is
			// only how far the finalisation got.
			f, err := os.OpenFile(src, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteAt(mp3, 0); err != nil {
				t.Fatal(err)
			}
			if tc.truncate {
				if err := f.Truncate(int64(len(mp3))); err != nil {
					t.Fatal(err)
				}
			}
			rec, err := sourceJournalRecord(src, f)
			if err != nil {
				t.Fatal(err)
			}
			f.Close()

			start := src
			if tc.renamed {
				if err := os.Rename(src, target); err != nil {
					t.Fatal(err)
				}
				start = target
			}
			journalPath := reclaimJournal(t, dir, "tone", rec, info, target, int64(len(mp3)), "finalising")

			store, j, err := journal.Open(journalPath)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			// The driver's inode gate must accept both windows.
			if err := journal.VerifySource(j, start); err != nil {
				t.Fatal(err)
			}
			if _, err := ResumeReclaim(context.Background(), store, j, progress.Noop{}); err != nil {
				t.Fatal(err)
			}

			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, mp3) {
				t.Fatalf("output differs from the verified encode (%d vs %d bytes)", len(got), len(mp3))
			}
			if _, err := os.Stat(src); !os.IsNotExist(err) {
				t.Fatalf("source still present: %v", err)
			}
			if paths, _ := journal.Find(dir, "tone"); len(paths) != 0 {
				t.Fatalf("journal left behind: %v", paths)
			}
		})
	}
}
