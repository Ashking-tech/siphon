package preflight

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
	"siphon/internal/siphonerr"
)

// mkvFixture builds a small real Matroska file, optionally with one subtitle
// stream or one chapter list. The tests skip when ffmpeg is absent.
func mkvFixture(t *testing.T, subtitle, chapters bool) string {
	t.Helper()
	skipWithoutTool(t, "ffmpeg")
	skipWithoutTool(t, "ffprobe")
	dir := t.TempDir()
	src := filepath.Join(dir, "source.mkv")
	args := []string{
		"-y", "-v", "error",
		"-f", "lavfi", "-i", "testsrc2=size=160x120:rate=25:duration=3",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100:duration=3",
	}
	var srt, chaptersFile string
	if subtitle {
		srt = filepath.Join(dir, "sub.srt")
		if err := os.WriteFile(srt, []byte("1\n00:00:00,000 --> 00:00:02,000\nhello\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", srt)
	}
	if chapters {
		chaptersFile = filepath.Join(dir, "chapters.txt")
		body := ";FFMETADATA1\n[CHAPTER]\nTIMEBASE=1/1000\nSTART=0\nEND=1500\ntitle=One\n" +
			"[CHAPTER]\nTIMEBASE=1/1000\nSTART=1500\nEND=3000\ntitle=Two\n"
		if err := os.WriteFile(chaptersFile, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-i", chaptersFile)
	}
	args = append(args, "-map", "0:v", "-map", "1:a")
	if subtitle {
		args = append(args, "-map", "2:s")
	}
	args = append(args,
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25",
		"-c:a", "aac", "-b:a", "64k",
	)
	if subtitle {
		args = append(args, "-c:s", "srt")
	}
	if chapters {
		args = append(args, "-map_chapters", "2")
	}
	args = append(args, src)
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, stderr.String())
	}
	return src
}

func mkvJob(t *testing.T, src string) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.MKV, formats.MP4)
	if !ok {
		t.Fatal("MKV→MP4 pair missing")
	}
	return &core.Job{
		SourcePath:       src,
		TargetPath:       TargetPath(src, formats.MP4),
		Source:           formats.MKV,
		Target:           formats.MP4,
		Pair:             pair,
		Mode:             formats.ModeSafe,
		SegmentDuration:  4 * time.Second,
		SegmentTolerance: 500 * time.Millisecond,
		Verify:           "sampled",
		VerifyFrames:     3,
		Timeout:          time.Hour,
		AllowRoot:        os.Geteuid() == 0,
	}
}

func TestPlanMKVSafe(t *testing.T) {
	src := mkvFixture(t, false, false)
	plan, err := Run(context.Background(), mkvJob(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Engine != "remux" || plan.OutputKind != "MP4" {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Streams) != 2 || !strings.Contains(plan.Streams[0], "h264 copy") {
		t.Fatalf("streams = %v", plan.Streams)
	}
	if plan.FreeNeeded <= 0 {
		t.Fatalf("free needed = %d", plan.FreeNeeded)
	}
	if !strings.Contains(strings.Join(plan.Warnings, "\n"), "classic MP4") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}

func TestRefuseMKVSubtitle(t *testing.T) {
	src := mkvFixture(t, true, false)
	_, err := Run(context.Background(), mkvJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.CodecIncompatible) {
		t.Fatalf("got %v, want E_CODEC_INCOMPATIBLE", err)
	}

	job := mkvJob(t, src)
	job.DropIncompatible = true
	plan, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Dropped, ","), "subtitle:2") {
		t.Fatalf("dropped = %v", plan.Dropped)
	}
}

func TestRefuseMKVChapters(t *testing.T) {
	src := mkvFixture(t, false, true)
	_, err := Run(context.Background(), mkvJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.MetadataDrop) {
		t.Fatalf("got %v, want E_METADATA_DROP", err)
	}

	job := mkvJob(t, src)
	job.DropMetadata = true
	plan, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Dropped, ","), "chapters") {
		t.Fatalf("dropped = %v", plan.Dropped)
	}
	if !strings.Contains(strings.Join(plan.Warnings, "\n"), "will not be carried") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}
}
