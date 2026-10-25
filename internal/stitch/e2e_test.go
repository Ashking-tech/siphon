package stitch

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// The end-to-end tests build one small real Matroska fixture with ffmpeg and
// drive the spike through it. They skip when ffmpeg/ffprobe are unavailable.

var (
	fixtureOnce sync.Once
	fixtureDir  string
	fixturePath string
	fixtureErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if fixtureDir != "" {
		os.RemoveAll(fixtureDir)
	}
	os.Exit(code)
}

func needTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
}

func runFFmpeg(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("ffmpeg: %v\n%s", err, stderr.String())
	}
}

// fixture returns a 12 s h264+AAC Matroska file: 1 s GOPs, 1 s clusters, B
// frames (so fragments carry negative composition offsets), 44.1 kHz audio.
func fixture(t *testing.T) string {
	t.Helper()
	needTools(t)
	fixtureOnce.Do(func() {
		dir, err := os.MkdirTemp("", "siphon-stitch-")
		if err != nil {
			fixtureErr = err
			return
		}
		fixtureDir = dir
		fixturePath = filepath.Join(dir, "base.mkv")
		cmd := exec.Command("ffmpeg",
			"-nostdin", "-v", "error", "-y",
			"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
			"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
			"-t", "12",
			"-map", "0:v", "-map", "1:a",
			"-c:v", "libx264", "-preset", "veryfast", "-bf", "2",
			"-g", "30", "-keyint_min", "30", "-sc_threshold", "0",
			"-pix_fmt", "yuv420p",
			"-c:a", "aac", "-b:a", "96k",
			"-cluster_time_limit", "1000",
			"-f", "matroska", fixturePath,
		)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			fixtureErr = fmt.Errorf("ffmpeg fixture: %v\n%s", err, stderr.String())
		}
	})
	if fixtureErr != nil {
		t.Fatalf("fixture: %v", fixtureErr)
	}
	return fixturePath
}

func planOpts() Options {
	return Options{
		SegmentDuration:  4 * time.Second,
		SegmentTolerance: 500 * time.Millisecond,
		VerifyFrames:     3,
	}
}

func TestPlanCuesAndScanAgree(t *testing.T) {
	path := fixture(t)
	ctx := context.Background()

	cues, err := BuildPlan(ctx, path, planOpts(), nil)
	if err != nil {
		t.Fatalf("BuildPlan(cues): %v", err)
	}
	if cues.PlanSource != "cues" {
		t.Fatalf("plan source = %q, want cues (Cues coverage must reach 90%% of the duration)", cues.PlanSource)
	}
	opt := planOpts()
	opt.ForceScan = true
	scan, err := BuildPlan(ctx, path, opt, nil)
	if err != nil {
		t.Fatalf("BuildPlan(scan): %v", err)
	}
	if scan.PlanSource != "scan" {
		t.Fatalf("forced plan source = %q, want scan", scan.PlanSource)
	}

	if len(cues.Keyframes) != len(scan.Keyframes) {
		t.Fatalf("keyframes: cues %d, scan %d", len(cues.Keyframes), len(scan.Keyframes))
	}
	for i := range cues.Keyframes {
		if cues.Keyframes[i] != scan.Keyframes[i] {
			t.Errorf("keyframe %d: cues %+v, scan %+v", i, cues.Keyframes[i], scan.Keyframes[i])
		}
	}
	if len(cues.Segments) != len(scan.Segments) {
		t.Fatalf("segments: cues %d, scan %d", len(cues.Segments), len(scan.Segments))
	}
	for i := range cues.Segments {
		if !reflect.DeepEqual(cues.Segments[i], scan.Segments[i]) {
			t.Errorf("segment %d: cues %+v, scan %+v", i, cues.Segments[i], scan.Segments[i])
		}
	}
	if !bytes.Equal(cues.Header, scan.Header) {
		t.Error("cached headers differ between the cues and scan plans")
	}
	if len(cues.Segments) != 3 {
		t.Errorf("12 s / 4 s plan has %d segments, want 3", len(cues.Segments))
	}
	t.Logf("cues = scan: %d keyframes, %d segments, header %d bytes",
		len(cues.Keyframes), len(cues.Segments), len(cues.Header))
}

// packetPTS returns per-stream packet PTS values in storage order.
func packetPTS(ctx context.Context, path string) (map[int][]float64, error) {
	out, err := makeFFprobe(ctx, "-v", "error",
		"-show_entries", "packet=stream_index,pts_time",
		"-of", "csv=p=0", path).Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe packets %s: %w", path, err)
	}
	res := map[int][]float64{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		idx, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil {
			continue
		}
		s := strings.TrimSpace(parts[1])
		if s == "" || s == "N/A" {
			continue
		}
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			continue
		}
		res[idx] = append(res[idx], v)
	}
	return res, nil
}

// checkTimeline proves every output packet carries its source timestamp plus
// the global pad: the strongest statement the spike can make about A/V sync.
func checkTimeline(t *testing.T, ctx context.Context, src, out string, padNS int64) {
	t.Helper()
	srcPTS, err := packetPTS(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	outPTS, err := packetPTS(ctx, out)
	if err != nil {
		t.Fatal(err)
	}
	pad := float64(padNS) / 1e9
	if len(srcPTS) != len(outPTS) {
		t.Fatalf("output has %d streams, source %d", len(outPTS), len(srcPTS))
	}
	for idx, want := range srcPTS {
		got, ok := outPTS[idx]
		if !ok {
			t.Errorf("stream %d missing from the output", idx)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("stream %d: %d packets, want %d", idx, len(got), len(want))
			continue
		}
		sort.Float64s(want)
		sort.Float64s(got)
		worst, worstAt := 0.0, -1
		for i := range want {
			d := math.Abs(got[i] - (want[i] + pad))
			if d > worst {
				worst, worstAt = d, i
			}
			if d > 0.002 {
				t.Errorf("stream %d packet %d: out %.6f, source+pad %.6f (delta %.3f ms)",
					idx, i, got[i], want[i]+pad, d*1000)
				break
			}
		}
		t.Logf("stream %d: %d packets shifted by %.3f ms, worst deviation %.3f ms (packet %d)",
			idx, len(want), pad*1000, worst*1000, worstAt)
	}
}

func TestRunEndToEnd(t *testing.T) {
	path := fixture(t)
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "out.mp4")
	opt := planOpts()
	opt.FullDecode = true

	var stats IOStats
	plan, err := BuildPlan(ctx, path, opt, &stats)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	rep, err := Run(ctx, plan, out, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Pass {
		for _, c := range rep.Checks {
			t.Logf("check %-20s %v  %s", c.Name, c.OK, c.Detail)
		}
		t.Fatal("Run reported FAIL")
	}
	for _, c := range rep.Checks {
		if !c.OK {
			t.Errorf("check %s failed: %s", c.Name, c.Detail)
		}
	}
	if rep.PadNS <= 0 {
		t.Errorf("pad = %d ns, want a positive video reorder pad", rep.PadNS)
	}

	var wantExtract int64
	for _, s := range plan.Segments {
		for _, p := range s.Pieces {
			for _, rg := range p.Ranges {
				wantExtract += rg[1] - rg[0]
			}
		}
	}
	if rep.Stats.ExtractRead != wantExtract {
		t.Errorf("source payload read = %d, want %d (one pass)",
			rep.Stats.ExtractRead, wantExtract)
	}
	if want := int64(len(plan.Header)) * int64(len(plan.Segments)); rep.Stats.HeaderSends != want {
		t.Errorf("header bytes sent = %d, want %d", rep.Stats.HeaderSends, want)
	}
	if rep.Stats.Segments != len(plan.Segments) {
		t.Errorf("segments extracted = %d, want %d", rep.Stats.Segments, len(plan.Segments))
	}
	t.Logf("plan %s, %d segments, pad %.3f ms, source read %.2f MB, header sends %.2f MB, patch %.2f MB, %d decode runs",
		plan.PlanSource, len(plan.Segments), float64(rep.PadNS)/1e6,
		float64(rep.Stats.ExtractRead)/1e6, float64(rep.Stats.HeaderSends)/1e6,
		float64(rep.Stats.PatchRead)/1e6, rep.Stats.DecodeRuns)

	checkTimeline(t, ctx, path, rep.OutPath, rep.PadNS)
}

func TestRunSingleSegment(t *testing.T) {
	path := fixture(t)
	ctx := context.Background()
	out := filepath.Join(t.TempDir(), "single.mp4")
	opt := Options{
		SegmentDuration:  time.Minute,
		SegmentTolerance: 500 * time.Millisecond,
		VerifyFrames:     1,
	}
	plan, err := BuildPlan(ctx, path, opt, nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if len(plan.Segments) != 1 {
		t.Fatalf("got %d segments, want 1", len(plan.Segments))
	}
	rep, err := Run(ctx, plan, out, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Pass {
		for _, c := range rep.Checks {
			t.Logf("check %-20s %v  %s", c.Name, c.OK, c.Detail)
		}
		t.Fatal("single-segment Run reported FAIL")
	}
	if rep.Stats.Segments != 1 {
		t.Errorf("segments = %d, want 1", rep.Stats.Segments)
	}
	checkTimeline(t, ctx, path, rep.OutPath, rep.PadNS)
}

// TestDelayedAudioSync keeps a 0.5 s A/V offset through planning, per-track
// re-anchoring and assembly.
func TestDelayedAudioSync(t *testing.T) {
	needTools(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "delayed.mkv")
	runFFmpeg(t,
		"-nostdin", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=30",
		"-itsoffset", "0.5",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=44100",
		"-t", "6",
		"-map", "0:v", "-map", "1:a",
		"-c:v", "libx264", "-preset", "veryfast", "-bf", "2",
		"-g", "30", "-keyint_min", "30", "-sc_threshold", "0",
		"-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "96k",
		"-cluster_time_limit", "1000",
		"-f", "matroska", src,
	)

	ctx := context.Background()
	out := filepath.Join(dir, "delayed-out.mp4")
	opt := Options{
		SegmentDuration:  3 * time.Second,
		SegmentTolerance: 500 * time.Millisecond,
		VerifyFrames:     1,
	}
	plan, err := BuildPlan(ctx, src, opt, nil)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	rep, err := Run(ctx, plan, out, opt)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !rep.Pass {
		for _, c := range rep.Checks {
			t.Logf("check %-20s %v  %s", c.Name, c.OK, c.Detail)
		}
		t.Fatal("delayed-audio Run reported FAIL")
	}

	srcStarts, err := streamStartTimes(ctx, src)
	if err != nil {
		t.Fatal(err)
	}
	outStarts, err := streamStartTimes(ctx, rep.OutPath)
	if err != nil {
		t.Fatal(err)
	}
	srcAudio, ok1 := srcStarts["audio"]
	srcVideo, ok2 := srcStarts["video"]
	outAudio, ok3 := outStarts["audio"]
	outVideo, ok4 := outStarts["video"]
	if !(ok1 && ok2 && ok3 && ok4) {
		t.Fatalf("missing stream start times: source %v, output %v", srcStarts, outStarts)
	}
	relSrc := srcAudio - srcVideo
	relOut := outAudio - outVideo
	if math.Abs(relOut-relSrc) > 0.010 {
		t.Errorf("A/V offset changed: source %.3f ms, output %.3f ms", relSrc*1000, relOut*1000)
	}
	t.Logf("A/V offset: source %.3f ms, output %.3f ms (pad %.3f ms)",
		relSrc*1000, relOut*1000, float64(rep.PadNS)/1e6)
	if math.Abs(relSrc-0.5) > 0.05 {
		t.Errorf("fixture offset = %.3f ms, want 500 ms", relSrc*1000)
	}
}
