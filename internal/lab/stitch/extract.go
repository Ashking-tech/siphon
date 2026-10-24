package stitch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"siphon/internal/platform"
	"siphon/internal/siphonerr"
)

// Check is one named verification result in the report.
type Check struct {
	Name   string
	OK     bool
	Detail string
}

// Report is the spike's full outcome.
type Report struct {
	Plan        *Plan
	Stats       IOStats
	Fragments   []FragmentInfo
	Init        []byte
	InitLen     int64
	OutPath     string
	OutSize     int64
	PadNS       int64 // global timeline pad applied by the patch pass
	Checks      []Check
	Pass        bool
	Wall        time.Duration
	PlanWall    time.Duration
	ExtractWall time.Duration
}

// Run performs the whole spike: plan-sized extraction tail-first, per-fragment
// verification, sparse reverse assembly, then assembled-file verification.
func Run(ctx context.Context, plan *Plan, outPath string, opt Options) (*Report, error) {
	logf := opt.Log
	if logf == nil {
		logf = func(string, ...any) {}
	}
	rep := &Report{Plan: plan}

	src, err := os.Open(plan.Path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot open %s", plan.Path)
	}
	defer src.Close()

	var anchorRead atomic.Int64
	anchors := countingReaderAt{r: src, n: &anchorRead}
	want := make(map[uint64]bool, len(plan.TrackMap))
	delayNS := make(map[uint64]int64, len(plan.TrackMap))
	for _, t := range plan.Tracks {
		if _, ok := plan.TrackMap[t.number]; ok {
			want[t.number] = true
			delayNS[t.number] = t.codecDelayNS
		}
	}

	dir := filepath.Dir(outPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(dir, ".siphon-stitch-*.mp4")
	if err != nil {
		return nil, err
	}
	defer func() {
		tmp.Close()
		os.Remove(tmp.Name())
	}()
	if err := tmp.Truncate(plan.ReserveSize); err != nil {
		return nil, siphonerr.Wrap(siphonerr.NoSpace, err, "cannot reserve the output")
	}

	segments := plan.Segments
	rep.ExtractWall = 0
	fragInfos := make([]FragmentInfo, len(segments))
	timings := make([]segmentTiming, len(segments))
	outCursor := plan.ReserveSize
	var initBytes []byte
	extractStart := time.Now()

	for i := len(segments) - 1; i >= 0; i-- {
		seg := segments[i]
		frag, err := extractSegment(ctx, src, plan.Header, seg, &rep.Stats)
		if err != nil {
			return nil, err
		}
		info, err := analyzeFragment(frag)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.StitchFailed, err,
				"segment %d: cannot parse the extracted fragment", i)
		}
		info.Index = i
		info.Bytes = int64(len(frag))
		rep.Stats.Segments++

		// First-block anchors: the absolute PTS (ns) of each track's first
		// block in the segment's opening clusters, minus the track's codec
		// delay (Matroska block timecodes are pre-delay; demuxers subtract
		// CodecDelay, e.g. AAC's 1024-sample priming). These are what the
		// patch pass re-anchors the fragment's rebased timestamps to.
		anchorsNS, err := firstBlockTimes(anchors, seg.SrcOff, seg.AnchorFrom, seg.SrcEnd,
			plan.TimecodeScaleNS, want)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.StitchFailed, err,
				"segment %d: cannot read the first-block anchors", i)
		}
		anchorNS := make(map[uint32]int64, len(plan.TrackMap))
		for mkv, mp4 := range plan.TrackMap {
			if ns, ok := anchorsNS[mkv]; ok {
				anchorNS[mp4] = ns - delayNS[mkv]
			} else {
				// The track has no block in the segment's opening clusters
				// (very unusual): fall back to the cue time.
				anchorNS[mp4] = seg.FirstPTSNS
			}
		}
		timings[i] = segmentTiming{moofs: info.Moofs, anchorNS: anchorNS}
		info.AnchorNS = anchorNS

		// Per-segment gate (spec §4.7 step 5, sampled: N frames per fragment).
		if opt.VerifyFrames > 0 {
			if err := verifyFragmentBytes(ctx, frag, opt.VerifyFrames, &rep.Stats); err != nil {
				return nil, siphonerr.Wrap(siphonerr.StitchFailed, err,
					"segment %d (%d..%d) failed the sampled decode", i, seg.SrcOff, seg.SrcEnd)
			}
		}

		ini, body, err := stripInit(frag)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.StitchFailed, err, "segment %d: bad fragment layout", i)
		}
		if i == len(segments)-1 {
			if len(ini) == 0 {
				return nil, siphonerr.New(siphonerr.StitchFailed, "the first processed segment produced no init segment")
			}
			initBytes = ini
			rep.Init = ini
			rep.InitLen = int64(len(ini))
		}

		if outCursor-int64(len(body)) < 0 {
			return nil, siphonerr.New(siphonerr.NoSpaceRuntime,
				"reverse cursor underflow at segment %d (reserve %d)", i, plan.ReserveSize)
		}
		outCursor -= int64(len(body))
		if _, err := tmp.WriteAt(body, outCursor); err != nil {
			return nil, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot write segment %d", i)
		}
		rep.Stats.Write += int64(len(body))
		fragInfos[i] = info
		logf("segment %2d/%d  %8d..%-8d  %7d B  %d moofs  first %v  anchors %v  samples %v",
			i+1, len(segments), seg.SrcOff, seg.SrcEnd, len(body), info.Moofs,
			info.FirstPTS, anchorNS, info.SampleCounts)
	}
	rep.ExtractWall = time.Since(extractStart)
	rep.Stats.AnchorRead = anchorRead.Load()

	// Step 10: the init segment sits directly in front of the fragments.
	if outCursor-int64(len(initBytes)) < 0 {
		return nil, siphonerr.New(siphonerr.NoSpaceRuntime, "reverse cursor underflow at the init segment")
	}
	outCursor -= int64(len(initBytes))
	if _, err := tmp.WriteAt(initBytes, outCursor); err != nil {
		return nil, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot write the init segment")
	}
	rep.Stats.Write += int64(len(initBytes))

	// Step 11: whatever is left at the front is a legal free box over the
	// sparse hole.
	if outCursor > 0 {
		hdr, err := freeBoxHeader(outCursor)
		if err != nil {
			return nil, siphonerr.New(siphonerr.NoSpaceRuntime, "%v", err)
		}
		if _, err := tmp.WriteAt(hdr, 0); err != nil {
			return nil, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot write the free box")
		}
		rep.Stats.Write += int64(len(hdr))
	}

	// Step 12: re-anchor every fragment's timestamps to the source timeline
	// (the muxer rebases each track per extraction run; see RESULTS.md).
	padNS, err := patchTimestamps(tmp, initBytes, timings, &rep.Stats)
	if err != nil {
		return nil, err
	}
	rep.PadNS = padNS

	if err := platform.Fdatasync(tmp); err != nil {
		return nil, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot flush the assembled output")
	}
	rep.Fragments = fragInfos

	if err := tmp.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp.Name(), outPath); err != nil {
		return nil, err
	}
	rep.OutPath = outPath
	if st, err := os.Stat(outPath); err == nil {
		rep.OutSize = st.Size()
	}

	rep.Checks = verifyAssembly(ctx, plan, rep, opt)
	rep.Pass = true
	for _, c := range rep.Checks {
		if !c.OK {
			rep.Pass = false
		}
	}
	return rep, nil
}

// extractSegment runs one ffmpeg extraction: cached header plus one byte range
// on stdin, a fragmented MP4 fragment on stdout. No seeking anywhere.
func extractSegment(ctx context.Context, src io.ReaderAt, header []byte, seg Segment, stats *IOStats) ([]byte, error) {
	args := []string{
		"-nostdin", "-v", "error",
		"-f", "matroska", "-i", "pipe:0",
		"-map", "0:v", "-map", "0:a?",
		"-c", "copy",
		"-copyts", "-muxdelay", "0", "-avoid_negative_ts", "disabled",
		"-f", "mp4",
		"-movflags", "+empty_moov+default_base_moof+negative_cts_offsets",
		"-frag_duration", "1000000",
		"pipe:1",
	}
	cmd := makeFFmpeg(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Dependency, err, "cannot start ffmpeg")
	}
	feedErr := make(chan error, 1)
	go func() {
		err := feedSegment(stdin, src, header, seg, stats)
		// EOF: ffmpeg must see the end of the byte range, or it waits forever.
		_ = stdin.Close()
		feedErr <- err
	}()
	waitErr := cmd.Wait()
	if err := <-feedErr; err != nil && waitErr == nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if waitErr != nil {
		return nil, siphonerr.Wrap(siphonerr.StitchFailed, waitErr,
			"segment %d: ffmpeg failed: %s", seg.Index, stderrTail(stderr.Bytes()))
	}
	return stdout.Bytes(), nil
}

// feedSegment writes the cached header and then the segment's synthetic
// pieces into ffmpeg's stdin. Raw cluster ranges are copied verbatim; a
// wrapped piece gets a freshly encoded Cluster header because its payload is a
// subset of the source cluster (Timecode plus the blocks the plan kept).
func feedSegment(w io.Writer, src io.ReaderAt, header []byte, seg Segment, stats *IOStats) error {
	if len(header) > 0 {
		if _, err := w.Write(header); err != nil {
			return err
		}
		stats.HeaderSends += int64(len(header))
	}
	buf := make([]byte, 1<<20)
	for _, p := range seg.Pieces {
		var payload int64
		for _, rg := range p.Ranges {
			payload += rg[1] - rg[0]
		}
		if p.Wrap {
			hdr := append([]byte(nil), clusterIDBytes...)
			hdr, err := appendSizeVint(hdr, payload)
			if err != nil {
				return err
			}
			if _, err := w.Write(hdr); err != nil {
				return err
			}
		}
		for _, rg := range p.Ranges {
			if err := copySourceRange(w, src, buf, rg[0], rg[1], stats); err != nil {
				return err
			}
		}
	}
	return nil
}

// copySourceRange copies [off, end) from src to w, counting the source reads.
func copySourceRange(w io.Writer, src io.ReaderAt, buf []byte, off, end int64, stats *IOStats) error {
	start := off
	for off < end {
		n := int64(len(buf))
		if rem := end - off; rem < n {
			n = rem
		}
		read, err := src.ReadAt(buf[:n], off)
		if read > 0 {
			if _, werr := w.Write(buf[:read]); werr != nil {
				return werr
			}
			stats.ExtractRead += int64(read)
			off += int64(read)
		}
		if err != nil {
			if errors.Is(err, io.EOF) && off >= end {
				break
			}
			return siphonerr.Wrap(siphonerr.Internal, err, "cannot read [%d,%d)", start, end)
		}
	}
	return nil
}

// verifyFragmentBytes runs the sampled per-fragment decode: N video frames,
// -xerror, stdin-fed so nothing is written to disk.
func verifyFragmentBytes(ctx context.Context, frag []byte, frames int, stats *IOStats) error {
	args := []string{
		"-nostdin", "-v", "error", "-xerror",
		"-f", "mp4", "-i", "pipe:0",
		"-frames:v", strconv.Itoa(frames),
		"-f", "null", "-",
	}
	cmd := makeFFmpeg(ctx, args...)
	cmd.Stdin = bytes.NewReader(frag)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	stats.DecodeRuns++
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("sampled decode failed: %s", stderrTail(stderr.Bytes()))
	}
	return nil
}

// stderrTail returns the last line of a subprocess error log.
func stderrTail(b []byte) string {
	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	if len(lines) == 0 {
		return "(no stderr)"
	}
	out := lines[len(lines)-1]
	if len(out) > 300 {
		out = out[:300]
	}
	return string(out)
}
