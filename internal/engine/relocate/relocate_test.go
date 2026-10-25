package relocate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/media"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
	"siphon/internal/stitch"
)

func needTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

// relocateFixture builds a source MKV with 1 s GOPs. extra is appended before
// the output path (metadata, codec overrides).
func relocateFixture(t *testing.T, seconds int, extra ...string) (dir, src, target string) {
	t.Helper()
	needTools(t)
	dir = t.TempDir()
	src = filepath.Join(dir, "source.mkv")
	target = filepath.Join(dir, "source.mp4")
	args := []string{
		"-y", "-v", "error",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc2=size=320x240:rate=25:duration=%d", seconds),
		"-f", "lavfi", "-i", fmt.Sprintf("sine=frequency=440:sample_rate=44100:duration=%d", seconds),
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "25",
		"-c:a", "aac", "-b:a", "96k",
		"-metadata", "title=Siphon relocate test",
		"-metadata:s:a:0", "language=eng",
	}
	args = append(args, extra...)
	args = append(args, src)
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, stderr.String())
	}
	return dir, src, target
}

func relocateJob(src, target string) *core.Job {
	return &core.Job{
		SourcePath:       src,
		TargetPath:       target,
		Source:           formats.MKV,
		Target:           formats.MP4,
		Mode:             formats.ModeRelocate,
		Engine:           formats.EngineRelocate,
		SegmentDuration:  2 * time.Second,
		SegmentTolerance: 500 * time.Millisecond,
		Verify:           "sampled",
		VerifyFrames:     3,
		ProbeSegments:    4,
		Timeout:          3 * time.Minute,
	}
}

func sourceCounts(t *testing.T, path string) map[string]int {
	t.Helper()
	counts, err := stitch.CountPackets(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return stitch.RetainedCounts(counts)
}

func outputCounts(t *testing.T, path string) map[string]int {
	t.Helper()
	return sourceCounts(t, path)
}

func firstBox(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var h [8]byte
	if _, err := f.Read(h[:]); err != nil {
		t.Fatal(err)
	}
	return string(h[4:])
}

func TestConvertE2E(t *testing.T) {
	dir, src, target := relocateFixture(t, 10)
	wantCounts := sourceCounts(t, src)

	res, err := New().Convert(context.Background(), relocateJob(src, target), nil, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still present: %v", err)
	}
	if got := outputCounts(t, target); !reflect.DeepEqual(got, wantCounts) {
		t.Fatalf("output counts = %v, want %v", got, wantCounts)
	}
	probe, err := media.ProbeFile(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Format.Tags["title"] != "Siphon relocate test" {
		t.Errorf("title tag = %q", probe.Format.Tags["title"])
	}
	var audioTags map[string]string
	for _, s := range probe.Streams {
		if s.CodecType == "audio" {
			audioTags = s.Tags
		}
	}
	if audioTags["language"] != "eng" {
		t.Errorf("audio language = %q", audioTags["language"])
	}
	d, err := strconv.ParseFloat(probe.Format.Duration, 64)
	if err != nil || d < 9.9 || d > 10.2 {
		t.Errorf("duration = %q", probe.Format.Duration)
	}
	if box := firstBox(t, target); box != "free" && box != "ftyp" {
		t.Errorf("first box = %q", box)
	}
	if err := media.DecodeCheck(context.Background(), target); err != nil {
		t.Fatalf("full decode: %v", err)
	}
	journals, _ := journal.Find(dir, "source")
	if len(journals) != 0 {
		t.Fatalf("journals left: %v", journals)
	}
	temps, _ := filepath.Glob(filepath.Join(dir, ".siphon-source-*.tmp"))
	if len(temps) != 0 {
		t.Fatalf("temps left: %v", temps)
	}
}

func TestConvertAudioFirstTracks(t *testing.T) {
	// Matroska may store audio before video. Extraction maps video first, so
	// the full-gate stream-identity order must follow the output, not the
	// source track table.
	dir, src, target := relocateFixture(t, 10, "-map", "1:a", "-map", "0:v")
	wantCounts := sourceCounts(t, src)

	res, err := New().Convert(context.Background(), relocateJob(src, target), nil, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if got := outputCounts(t, target); !reflect.DeepEqual(got, wantCounts) {
		t.Fatalf("output counts = %v, want %v", got, wantCounts)
	}
	if err := media.DecodeCheck(context.Background(), target); err != nil {
		t.Fatalf("full decode: %v", err)
	}
	if journals, _ := journal.Find(dir, "source"); len(journals) != 0 {
		t.Fatalf("journals left: %v", journals)
	}
}

func TestConvertRefusesMetadataDrop(t *testing.T) {
	_, src, target := relocateFixture(t, 8, "-metadata", "SIPHON_CUSTOM=keepme")
	before, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	job := relocateJob(src, target)
	if _, err := New().Convert(context.Background(), job, nil, progress.Noop{}); !siphonerr.IsCode(err, siphonerr.MetadataDrop) {
		t.Fatalf("got %v, want E_METADATA_DROP", err)
	}
	after, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatalf("source changed: %d/%v -> %d/%v", before.Size(), before.ModTime(), after.Size(), after.ModTime())
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target exists after refusal: %v", err)
	}
	if journals, _ := journal.Find(filepath.Dir(src), "source"); len(journals) != 0 {
		t.Fatalf("journals left after refusal: %v", journals)
	}

	job.DropMetadata = true
	if _, err := New().Convert(context.Background(), job, nil, progress.Noop{}); err != nil {
		t.Fatal(err)
	}
	probe, err := media.ProbeFile(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Format.Tags["SIPHON_CUSTOM"] != "" {
		t.Errorf("custom tag was carried unexpectedly: %v", probe.Format.Tags)
	}
	if probe.Format.Tags["title"] != "Siphon relocate test" {
		t.Errorf("title tag = %q", probe.Format.Tags["title"])
	}
}

func TestConvertResumesAfterInterrupt(t *testing.T) {
	dir, src, target := relocateFixture(t, 30)
	wantCounts := sourceCounts(t, src)
	before, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		res *core.Result
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := New().Convert(ctx, relocateJob(src, target), nil, progress.Noop{})
		done <- outcome{res, err}
	}()

	// Cancel after the first source truncation. The engine checks the context
	// at the top of the next segment, so the journal is left mid-run.
	deadline := time.Now().Add(2 * time.Minute)
	for {
		st, err := os.Stat(src)
		if err == nil && st.Size() < before.Size() {
			cancel()
			break
		}
		if errors.Is(err, os.ErrNotExist) {
			t.Fatal("conversion finished before the interrupt could be injected")
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the first truncation")
		}
		time.Sleep(2 * time.Millisecond)
	}
	got := <-done
	if got.err == nil || !siphonerr.IsCode(got.err, siphonerr.Interrupted) {
		t.Fatalf("interrupt result = %+v, err = %v", got.res, got.err)
	}
	if got.res != nil && got.res.SourceUntouched {
		t.Fatal("interrupt after truncation marked the source untouched")
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source after interrupt: %v", err)
	}
	path, err := journal.FindOne(dir, "source")
	if err != nil {
		t.Fatal(err)
	}
	store, j, err := journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Resume(context.Background(), store, j, progress.Noop{})
	store.Close()
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || res.SourceUntouched {
		t.Fatalf("resume result = %+v", res)
	}
	if _, err := os.Stat(src); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still present after resume: %v", err)
	}
	if got := outputCounts(t, target); !reflect.DeepEqual(got, wantCounts) {
		t.Fatalf("resumed counts = %v, want %v", got, wantCounts)
	}
	if err := media.DecodeCheck(context.Background(), target); err != nil {
		t.Fatalf("full decode: %v", err)
	}
}

func TestReconcileTruncateWindows(t *testing.T) {
	for _, tc := range []struct {
		name string
		size int64
		want int64
	}{
		{"before ftruncate", 200, 100},
		{"after ftruncate", 100, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			srcPath := filepath.Join(dir, "film.mkv")
			if err := os.WriteFile(srcPath, make([]byte, tc.size), 0o600); err != nil {
				t.Fatal(err)
			}
			src, err := os.OpenFile(srcPath, os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer src.Close()
			store, j, err := journal.Create(dir, "film")
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			j.Job = journal.Job{Engine: string(formats.EngineRelocate), Mode: string(formats.ModeRelocate)}
			j.Segments = []journal.Segment{{I: 0, SrcOff: 100, SrcEnd: 300, State: "truncating"}}
			j.Cursors = journal.Cursors{TailCursor: 100, TruncateFrom: 200, Truncating: true}
			if err := reconcileTruncate(j, store, src); err != nil {
				t.Fatal(err)
			}
			st, err := src.Stat()
			if err != nil {
				t.Fatal(err)
			}
			if st.Size() != tc.want {
				t.Fatalf("source size = %d, want %d", st.Size(), tc.want)
			}
			if j.Segments[0].State != "done" || j.Cursors.Truncating || j.Cursors.TruncateFrom != 0 {
				t.Fatalf("journal not reconciled: %+v", j.Cursors)
			}
			if j.Cursors.SourceMtimeNS == 0 {
				t.Fatal("source mtime was not checkpointed")
			}
		})
	}
}
