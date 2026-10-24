package remux

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/media"
	"siphon/internal/progress"
)

func needTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

// fixture builds a fresh 6 s MKV in the test's temp dir. Every test owns its
// source because the engine deletes it on success.
func fixture(t *testing.T, audioCodec string) string {
	t.Helper()
	needTools(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	args := []string{
		"-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=25:duration=6",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=6",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25",
		"-c:a", audioCodec,
	}
	if audioCodec == "aac" {
		args = append(args, "-b:a", "96k")
	}
	args = append(args,
		"-metadata", "title=Siphon remux test",
		"-metadata:s:a:0", "language=eng",
		src,
	)
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, stderr.String())
	}
	return src
}

func newJob(src, dst string) *core.Job {
	return &core.Job{
		SourcePath: src,
		TargetPath: dst,
		Source:     formats.MKV,
		Target:     formats.MP4,
		Pair:       formats.Pair{Source: formats.MKV, Target: formats.MP4},
		Mode:       formats.ModeSafe,
		Engine:     formats.EngineRemux,
		Timeout:    2 * time.Minute,
	}
}

func TestRemuxSafe(t *testing.T) {
	src := fixture(t, "aac")
	dir := filepath.Dir(src)
	dst := filepath.Join(dir, "out.mp4")
	if err := os.Chmod(src, 0o640); err != nil {
		t.Fatal(err)
	}

	before, err := media.ProbeFile(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	srcCounts, err := media.CountPackets(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}

	job := newJob(src, dst)
	plan := &core.Plan{Source: src, Target: dst}
	res, err := New().Convert(context.Background(), job, plan, progress.Noop{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res == nil || !res.OK {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("source still exists: %v", err)
	}
	st, err := os.Stat(dst)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o640 {
		t.Errorf("mode = %o, want 640", st.Mode().Perm())
	}

	out, err := media.ProbeFile(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Streams) != 2 || out.Streams[0].CodecName != "h264" || out.Streams[1].CodecName != "aac" {
		t.Fatalf("output streams = %+v", out.Streams)
	}
	outCounts, err := media.CountPackets(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(outCounts) != len(srcCounts) {
		t.Fatalf("packet streams: src %v out %v", srcCounts, outCounts)
	}
	for i := range srcCounts {
		if outCounts[i].Packets != srcCounts[i].Packets {
			t.Errorf("stream %d: out %d packets, src %d", i, outCounts[i].Packets, srcCounts[i].Packets)
		}
	}
	if got := out.Format.Tags["title"]; got != "Siphon remux test" {
		t.Errorf("title tag = %q", got)
	}
	if got := out.Streams[1].Tags["language"]; got != "eng" {
		t.Errorf("audio language tag = %q", got)
	}
	if d := abs(out.Duration() - before.Duration()); d > time.Second {
		t.Errorf("duration delta %s", d)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".siphon-") {
			t.Errorf("leftover temp: %s", e.Name())
		}
	}
}

func TestRemuxTranscodePCM(t *testing.T) {
	src := fixture(t, "pcm_s16le")
	dst := filepath.Join(filepath.Dir(src), "out-pcm.mp4")

	job := newJob(src, dst)
	job.TranscodeAudio = true
	plan := &core.Plan{Source: src, Target: dst}
	if _, err := New().Convert(context.Background(), job, plan, progress.Noop{}); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out, err := media.ProbeFile(context.Background(), dst)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Streams) != 2 || out.Streams[1].CodecName != "aac" {
		t.Fatalf("output streams = %+v", out.Streams)
	}
	if d := abs(out.Duration() - 6*time.Second); d > time.Second {
		t.Errorf("duration = %s", out.Duration())
	}
}
