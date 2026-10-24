// Command stitch is the phase-4 lab harness (spec §8): it runs the video
// stitch spike against one MKV — plan from Cues or the packet scan, extract
// segments tail-first from cached-header byte ranges, assemble a fragmented
// MP4 with reverse pwrites over a sparse reserve, verify, and print a report.
//
//	go run ./internal/lab/stitch/cmd/stitch [flags] input.mkv
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"siphon/internal/lab/stitch"
	"siphon/internal/siphonerr"
)

func main() {
	var (
		segDur    = flag.Duration("segment-duration", 60*time.Second, "segment window")
		tol       = flag.Duration("segment-tolerance", 500*time.Millisecond, "keyframe snap tolerance")
		vframes   = flag.Int("verify-frames", 3, "video frames decoded per fragment (0 disables the gate)")
		full      = flag.Bool("full-decode", false, "decode the whole assembled file as a final check")
		forceScan = flag.Bool("force-scan", false, "use the ffprobe packet scan even when Cues exist")
		out       = flag.String("out", "", "output path (default: temp dir)")
		keep      = flag.Bool("keep", true, "keep the assembled file")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: stitch [flags] <input.mkv>\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	input := flag.Arg(0)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	opt := stitch.Options{
		SegmentDuration:  *segDur,
		SegmentTolerance: *tol,
		ForceScan:        *forceScan,
		VerifyFrames:     *vframes,
		FullDecode:       *full,
		Log:              func(format string, args ...any) { fmt.Printf(format+"\n", args...) },
	}

	planStart := time.Now()
	var planStats stitch.IOStats
	plan, err := stitch.BuildPlan(ctx, input, opt, &planStats)
	if err != nil {
		fatal(err)
	}
	planWall := time.Since(planStart)

	outPath := *out
	if outPath == "" {
		stem := strings.TrimSuffix(filepath.Base(input), filepath.Ext(input))
		outPath = filepath.Join(os.TempDir(), "siphon-stitch-"+stem+".mp4")
	}

	rep, err := stitch.Run(ctx, plan, outPath, opt)
	if err != nil {
		fatal(err)
	}
	rep.PlanWall = planWall
	rep.Stats.PlanRead = planStats.PlanRead
	printReport(rep)

	if !*keep {
		os.Remove(rep.OutPath)
	}
	if !rep.Pass {
		os.Exit(3)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "stitch: %v\n", err)
	os.Exit(siphonerr.ExitCode(err))
}

func printReport(rep *stitch.Report) {
	p := rep.Plan
	fmt.Printf("input      %s (%s, %s)\n", p.Path, humanBytes(p.FileSize), humanNS(p.DurationNS))
	fmt.Printf("plan       %s, %d keyframes, %d segments, video track %d (%s), audio track %d (%s), scale %s\n",
		p.PlanSource, len(p.Keyframes), len(p.Segments), p.VideoTrack, p.VideoCodec,
		p.AudioTrack, p.AudioCodec, humanNS(p.TimecodeScaleNS))
	fmt.Printf("header     %s cached at [0,%d), resent %d× (%s total)\n",
		humanBytes(p.HeaderRegion), p.HeaderRegion, rep.Stats.Segments, humanBytes(rep.Stats.HeaderSends))
	fmt.Printf("plan time  %s\n", rep.PlanWall.Round(time.Millisecond))
	fmt.Printf("extraction %s, %d segments, %d sampled decodes (gate: %d frames each)\n",
		rep.ExtractWall.Round(time.Millisecond), rep.Stats.Segments, rep.Stats.DecodeRuns, len(rep.Fragments))
	for _, f := range rep.Fragments {
		fmt.Printf("  seg %3d  %8d B  %d moofs  first %v  anchors %v  samples %v\n",
			f.Index, f.Bytes, f.Moofs, f.FirstPTS, f.AnchorNS, f.SampleCounts)
	}
	passes := float64(rep.Stats.ExtractRead+rep.Stats.AnchorRead+rep.Stats.PlanRead+p.HeaderRegion) / float64(p.FileSize)
	fmt.Printf("timeline   global pad %s (per-segment tfdt re-anchoring)\n", humanNS(rep.PadNS))
	fmt.Printf("I/O        read %s payload + %s anchors + %s plan, %.4f passes incl. header; wrote %s; moof patch r/w %s/%s; reserve %s (sparse)\n",
		humanBytes(rep.Stats.ExtractRead), humanBytes(rep.Stats.AnchorRead), humanBytes(rep.Stats.PlanRead), passes,
		humanBytes(rep.Stats.Write), humanBytes(rep.Stats.PatchRead), humanBytes(rep.Stats.PatchWrite),
		humanBytes(p.ReserveSize))
	fmt.Printf("output     %s (%s)\n", rep.OutPath, humanBytes(rep.OutSize))
	fmt.Printf("checks\n")
	for _, c := range rep.Checks {
		status := "PASS"
		if !c.OK {
			status = "FAIL"
		}
		fmt.Printf("  [%s] %-16s %s\n", status, c.Name, c.Detail)
	}
	if rep.Pass {
		fmt.Printf("result     PASS\n")
	} else {
		fmt.Printf("result     FAIL\n")
	}
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	f := float64(n)
	for _, u := range units {
		f /= unit
		if f < unit {
			return fmt.Sprintf("%.1f %s", f, u)
		}
	}
	return fmt.Sprintf("%.1f PiB", f)
}

func humanNS(ns int64) string {
	return time.Duration(ns).Round(time.Millisecond).String()
}
