package preflight

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/media"
	"siphon/internal/siphonerr"
)

// testPNG builds a real PNG so the image probe can decode it. With alpha the
// top-left pixel is transparent.
func testPNG(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x == 0 && y == 0 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / maxInt(1, w-1)),
				G: uint8(y * 255 / maxInt(1, h-1)),
				B: 128,
				A: a,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writePNG(t *testing.T, dir, name string, w, h int, alpha bool) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, testPNG(t, w, h, alpha), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func imageJob(t *testing.T, srcPath string) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.PNG, formats.JPG)
	if !ok {
		t.Fatal("pair missing")
	}
	return &core.Job{
		SourcePath:      srcPath,
		TargetPath:      TargetPath(srcPath, formats.JPG),
		Source:          formats.PNG,
		Target:          formats.JPG,
		Pair:            pair,
		Mode:            formats.ModeAuto,
		Quality:         90,
		SegmentDuration: 60 * time.Second,
		Verify:          "sampled",
		Timeout:         time.Hour,
		AllowRoot:       os.Geteuid() == 0,
	}
}

func TestPlanImage(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "img.png", 20, 10, false)
	plan, err := Run(context.Background(), imageJob(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "safe" || plan.Engine != "atomiccopy" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.FreeNeeded <= 0 || plan.FreeAvailable <= 0 {
		t.Fatalf("space not reported: %+v", plan)
	}
	if plan.ImageWidth != 20 || plan.ImageHeight != 10 {
		t.Fatalf("image dimensions not reported: %+v", plan)
	}
}

func TestImageEstimateIncludesDecodedSize(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "big.png", 100, 100, false)
	plan, err := Run(context.Background(), imageJob(t, src))
	if err != nil {
		t.Fatal(err)
	}
	// spec §7.1: source + max(2×source, decoded_raw×1.5) + 2 MiB
	want := int64(100*100*4)*3/2 + 2<<20
	if plan.FreeNeeded < want {
		t.Fatalf("free estimate %d is below the decoded-size floor %d", plan.FreeNeeded, want)
	}
}

func TestRefuseAlphaForJPEG(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "alpha.png", 8, 8, true)
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.AlphaUnsupported) {
		t.Fatalf("got %v, want E_ALPHA_UNSUPPORTED", err)
	}
}

func TestAlphaAllowedWithBackground(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "alpha.png", 8, 8, true)
	job := imageJob(t, src)
	job.Background = "white"
	plan, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasAlpha {
		t.Fatalf("plan should note the source alpha: %+v", plan)
	}
}

func TestRefuseBadBackground(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "alpha.png", 8, 8, true)
	job := imageJob(t, src)
	job.Background = "chartreuse-ish"
	_, err := Run(context.Background(), job)
	if !siphonerr.IsCode(err, siphonerr.Usage) {
		t.Fatalf("got %v, want E_USAGE", err)
	}
}

func TestRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	real := writePNG(t, dir, "real.png", 4, 4, false)
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, link))
	if !siphonerr.IsCode(err, siphonerr.SourceInvalid) {
		t.Fatalf("got %v, want E_SOURCE_INVALID", err)
	}
}

func TestRefuseExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "img.png", 4, 4, false)
	dst := filepath.Join(dir, "img.jpg")
	if err := os.WriteFile(dst, []byte("exists"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.TargetExists) {
		t.Fatalf("got %v, want E_TARGET_EXISTS", err)
	}
}

func TestRefuseBadMagic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	if err := os.WriteFile(src, []byte("not a png at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("got %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestRefuseTruncatedImage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	data := testPNG(t, 8, 8, false)
	if err := os.WriteFile(src, data[:20], 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("got %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestRefuseStaleJournal(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "img.png", 4, 4, false)
	store, _, err := journal.Create(dir, "img")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Remove()

	_, err = Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.StaleJournal) {
		t.Fatalf("got %v, want E_STALE_JOURNAL", err)
	}
}

func TestTargetPath(t *testing.T) {
	if got := TargetPath("/a/film.mkv", formats.MP4); got != "/a/film.mp4" {
		t.Fatalf("got %s", got)
	}
	if got := TargetPath("a.b.png", formats.JPG); got != "a.b.jpg" {
		t.Fatalf("got %s", got)
	}
}

// --- audio (WAV → MP3) -----------------------------------------------------

func skipWithoutTool(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("skipping: %s is not installed", name)
	}
}

// wavBytes builds a WAV with an optional LIST/INFO chunk and bext chunk.
func wavBytes(channels, sampleRate int, seconds float64, info map[string]string, bext bool) []byte {
	frames := int(float64(sampleRate) * seconds)
	data := make([]byte, frames*channels*2)
	for i := 0; i < frames; i++ {
		v := uint16(int16(math.Sin(2*math.Pi*440*float64(i)/float64(sampleRate)) * 9000))
		for c := 0; c < channels; c++ {
			binary.LittleEndian.PutUint16(data[(i*channels+c)*2:], v)
		}
	}

	var payload bytes.Buffer
	payload.WriteString("fmt ")
	_ = binary.Write(&payload, binary.LittleEndian, uint32(16))
	_ = binary.Write(&payload, binary.LittleEndian, uint16(1))
	_ = binary.Write(&payload, binary.LittleEndian, uint16(channels))
	_ = binary.Write(&payload, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(&payload, binary.LittleEndian, uint32(sampleRate*channels*2))
	_ = binary.Write(&payload, binary.LittleEndian, uint16(channels*2))
	_ = binary.Write(&payload, binary.LittleEndian, uint16(16))

	if len(info) > 0 {
		var list bytes.Buffer
		list.WriteString("INFO")
		keys := make([]string, 0, len(info))
		for k := range info {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := info[k]
			list.WriteString(k)
			_ = binary.Write(&list, binary.LittleEndian, uint32(len(v)))
			list.WriteString(v)
			if len(v)%2 == 1 {
				list.WriteByte(0)
			}
		}
		payload.WriteString("LIST")
		_ = binary.Write(&payload, binary.LittleEndian, uint32(list.Len()))
		payload.Write(list.Bytes())
	}

	if bext {
		b := make([]byte, 602)
		copy(b, "Siphon test broadcast wave")
		payload.WriteString("bext")
		_ = binary.Write(&payload, binary.LittleEndian, uint32(len(b)))
		payload.Write(b)
	}

	payload.WriteString("data")
	_ = binary.Write(&payload, binary.LittleEndian, uint32(len(data)))
	payload.Write(data)

	var out bytes.Buffer
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(payload.Len()+4))
	out.WriteString("WAVE")
	out.Write(payload.Bytes())
	return out.Bytes()
}

func writeWAVFixture(t *testing.T, dir, name string, channels, sampleRate int, seconds float64, info map[string]string, bext bool) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, wavBytes(channels, sampleRate, seconds, info, bext), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func wavJob(t *testing.T, srcPath string, mode formats.Mode) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.WAV, formats.MP3)
	if !ok {
		t.Fatal("pair wav → mp3 missing")
	}
	return &core.Job{
		SourcePath: srcPath,
		TargetPath: TargetPath(srcPath, formats.MP3),
		Source:     formats.WAV,
		Target:     formats.MP3,
		Pair:       pair,
		Mode:       mode,
		Yes:        true,
		Timeout:    time.Hour,
		AllowRoot:  os.Geteuid() == 0,
	}
}

func TestPlanWAVReclaim(t *testing.T) {
	skipWithoutTool(t, "lame")
	dir := t.TempDir()
	src := writeWAVFixture(t, dir, "tone.wav", 2, 44100, 2, nil, false)

	plan, err := Run(context.Background(), wavJob(t, src, formats.ModeReclaim))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "reclaim" || plan.Engine != "sliding" {
		t.Fatalf("plan = %+v", plan)
	}
	// spec §7.1: reclaim needs source × 1.01 plus a 1 MiB cushion.
	srcSize := plan.SourceSize
	if plan.FreeNeeded < srcSize*101/100 {
		t.Fatalf("free estimate %d is below source×1.01 (%d)", plan.FreeNeeded, srcSize)
	}
	if plan.FreeNeeded > srcSize*101/100+2*mib {
		t.Fatalf("free estimate %d is unreasonably above source×1.01", plan.FreeNeeded)
	}
	if plan.AudioChannels != 2 || plan.AudioSampleRate != 44100 {
		t.Fatalf("audio shape not reported: %+v", plan)
	}
	if plan.AudioDurationMS < 1900 || plan.AudioDurationMS > 2100 {
		t.Fatalf("duration %d ms", plan.AudioDurationMS)
	}
	if !strings.Contains(plan.CrashCost, "splice") {
		t.Fatalf("crash cost does not mention the splice: %q", plan.CrashCost)
	}
}

func TestPlanWAVSafeEstimate(t *testing.T) {
	skipWithoutTool(t, "lame")
	dir := t.TempDir()
	src := writeWAVFixture(t, dir, "tone.wav", 2, 44100, 2, nil, false)

	plan, err := Run(context.Background(), wavJob(t, src, formats.ModeSafe))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "safe" || plan.Engine != "sliding" {
		t.Fatalf("plan = %+v", plan)
	}
	// 2 s at the 190 kbps VBR ceiling, +5%.
	estOut := int64(2 * 190000 / 8 * 105 / 100)
	if plan.FreeNeeded < plan.SourceSize+estOut {
		t.Fatalf("free estimate %d is below source+estimate (%d)", plan.FreeNeeded, plan.SourceSize+estOut)
	}
	if !strings.Contains(plan.CrashCost, "untouched") {
		t.Fatalf("crash cost = %q", plan.CrashCost)
	}

	// An explicit --bitrate raises the estimate.
	job := wavJob(t, src, formats.ModeSafe)
	job.Bitrate = 320
	hi, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if hi.FreeNeeded <= plan.FreeNeeded {
		t.Fatalf("320 kbps estimate %d not above the VBR estimate %d", hi.FreeNeeded, plan.FreeNeeded)
	}
}

func TestRefuseWAVMultichannel(t *testing.T) {
	skipWithoutTool(t, "lame")
	dir := t.TempDir()
	src := writeWAVFixture(t, dir, "surround.wav", 4, 44100, 1, nil, false)

	_, err := Run(context.Background(), wavJob(t, src, formats.ModeSafe))
	if !siphonerr.IsCode(err, siphonerr.FormatUnsupported) {
		t.Fatalf("got %v, want E_FORMAT_UNSUPPORTED", err)
	}
}

func TestRefuseWAVMetadata(t *testing.T) {
	skipWithoutTool(t, "lame")
	dir := t.TempDir()
	src := writeWAVFixture(t, dir, "tagged.wav", 2, 44100, 1,
		map[string]string{"INAM": "Take One", "IART": "Someone"}, false)

	_, err := Run(context.Background(), wavJob(t, src, formats.ModeSafe))
	if !siphonerr.IsCode(err, siphonerr.MetadataDrop) {
		t.Fatalf("got %v, want E_METADATA_DROP", err)
	}
	if !siphonerr.SourceUntouched(err) {
		t.Fatalf("refusal must leave the source untouched: %v", err)
	}

	job := wavJob(t, src, formats.ModeSafe)
	job.DropMetadata = true
	plan, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.AudioTags) != 2 {
		t.Fatalf("plan should list the dropped tags: %+v", plan.AudioTags)
	}
	joined := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(joined, "will not be carried") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}

func TestRefuseWAVBext(t *testing.T) {
	skipWithoutTool(t, "lame")
	dir := t.TempDir()
	src := writeWAVFixture(t, dir, "bwf.wav", 1, 44100, 1, nil, true)

	_, err := Run(context.Background(), wavJob(t, src, formats.ModeSafe))
	if !siphonerr.IsCode(err, siphonerr.MetadataDrop) {
		t.Fatalf("got %v, want E_METADATA_DROP", err)
	}
}

func TestRefuseWAVCorrupt(t *testing.T) {
	skipWithoutTool(t, "lame")
	dir := t.TempDir()
	src := filepath.Join(dir, "broken.wav")
	if err := os.WriteFile(src, []byte("RIFF####WAVEjunk"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), wavJob(t, src, formats.ModeSafe))
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("got %v, want E_SOURCE_CORRUPT", err)
	}
}

// --- video (MKV → MP4 relocate) --------------------------------------------

func makeRelocateMKV(t *testing.T, dir, name string, seconds float64, extra ...string) string {
	t.Helper()
	skipWithoutTool(t, "ffmpeg")
	skipWithoutTool(t, "ffprobe")
	path := filepath.Join(dir, name)
	args := []string{
		"-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", fmt.Sprintf("%.3f", seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30",
		"-c:a", "aac", "-shortest",
	}
	args = append(args, extra...)
	args = append(args, path)
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ffmpeg fixture: %v\n%s", err, stderr.String())
	}
	return path
}

func relocateMKVJob(t *testing.T, srcPath string) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.MKV, formats.MP4)
	if !ok {
		t.Fatal("pair mkv → mp4 missing")
	}
	return &core.Job{
		SourcePath:       srcPath,
		TargetPath:       TargetPath(srcPath, formats.MP4),
		Source:           formats.MKV,
		Target:           formats.MP4,
		Pair:             pair,
		Mode:             formats.ModeRelocate,
		SegmentDuration:  2 * time.Second,
		SegmentTolerance: 500 * time.Millisecond,
		Verify:           "sampled",
		VerifyFrames:     3,
		ProbeSegments:    4,
		Timeout:          time.Minute,
		AllowRoot:        os.Geteuid() == 0,
	}
}

func TestRefuseRelocateTooFewSegments(t *testing.T) {
	dir := t.TempDir()
	src := makeRelocateMKV(t, dir, "short.mkv", 3)
	_, err := Run(context.Background(), relocateMKVJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.InplaceInfeasible) {
		t.Fatalf("got %v, want E_INPLACE_INFEASIBLE", err)
	}
	if !strings.Contains(err.Error(), "at least 3") {
		t.Fatalf("error does not explain the segment minimum: %v", err)
	}
}

func makeMultiAudioMKV(t *testing.T, dir, name string, seconds float64) string {
	t.Helper()
	skipWithoutTool(t, "ffmpeg")
	skipWithoutTool(t, "ffprobe")
	path := filepath.Join(dir, name)
	args := []string{
		"-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=44100",
		"-t", fmt.Sprintf("%.3f", seconds),
		"-map", "0:v", "-map", "1:a", "-map", "2:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30",
		"-c:a", "aac", "-shortest", path,
	}
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ffmpeg fixture: %v\n%s", err, stderr.String())
	}
	return path
}

func TestRefuseRelocateMultiStream(t *testing.T) {
	dir := t.TempDir()
	src := makeMultiAudioMKV(t, dir, "multi.mkv", 8)
	_, err := Run(context.Background(), relocateMKVJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.InplaceInfeasible) {
		t.Fatalf("got %v, want E_INPLACE_INFEASIBLE", err)
	}
	if !strings.Contains(err.Error(), "single-audio") {
		t.Fatalf("error does not explain the single-stream limit: %v", err)
	}
}

func TestRelocateConfigProbeRefusesChange(t *testing.T) {
	tp := &media.TrackPlan{Tracks: []media.Track{{Index: 0, Type: "video", Codec: "h264"}}}
	base := func(w int) *media.Probe {
		return &media.Probe{Streams: []media.Stream{{
			Index: 0, CodecType: "video", CodecName: "h264",
			Width: w, Height: 240, Profile: "High", Level: 31,
		}}}
	}
	probe := func(ctx context.Context, path, iv string) (*media.Probe, error) {
		if iv == "0%+1" {
			return base(320), nil
		}
		return base(640), nil
	}
	err := checkRelocateConfigWith(context.Background(), "ignored", tp, probe)
	if !siphonerr.IsCode(err, siphonerr.InplaceInfeasible) {
		t.Fatalf("got %v, want E_INPLACE_INFEASIBLE", err)
	}

	// A stream missing from a later probe is also a config change.
	probeMissing := func(ctx context.Context, path, iv string) (*media.Probe, error) {
		if iv == "0%+1" {
			return base(320), nil
		}
		return &media.Probe{}, nil
	}
	err = checkRelocateConfigWith(context.Background(), "ignored", tp, probeMissing)
	if !siphonerr.IsCode(err, siphonerr.InplaceInfeasible) {
		t.Fatalf("missing stream: got %v, want E_INPLACE_INFEASIBLE", err)
	}

	// A constant config passes.
	probeStable := func(ctx context.Context, path, iv string) (*media.Probe, error) {
		return base(320), nil
	}
	if err := checkRelocateConfigWith(context.Background(), "ignored", tp, probeStable); err != nil {
		t.Fatalf("stable config: %v", err)
	}
}

func TestRefuseRelocateDroppedAudio(t *testing.T) {
	dir := t.TempDir()
	src := makeRelocateMKV(t, dir, "pcm.mkv", 6, "-c:a", "pcm_s16le")
	job := relocateMKVJob(t, src)
	job.DropIncompatible = true
	_, err := Run(context.Background(), job)
	if !siphonerr.IsCode(err, siphonerr.InplaceInfeasible) {
		t.Fatalf("got %v, want E_INPLACE_INFEASIBLE", err)
	}
	if !strings.Contains(err.Error(), "cannot drop audio") {
		t.Fatalf("error does not explain the dropped stream: %v", err)
	}
}
