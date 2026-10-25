package stitch

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"

	"siphon/internal/siphonerr"
)

// Session owns the extraction state for one journaled relocate run. The engine
// owns the source fd, the temp fd, and the journal; the session owns the cached
// header, the running reverse cursor, and the per-segment timings needed by the
// final timestamp patch pass.
//
// A fresh run starts with OutCursor == plan.ReserveSize. A resume starts with
// the journal's OutCursor and calls SetDone for every segment whose fragment is
// already durable. Prepare is only called for segments that still need work.
type Session struct {
	Plan            *Plan
	OutCursor       int64
	Stats           IOStats
	Fragments       []FragmentInfo
	Init            []byte
	ExpectedExtract int64 // payload bytes this session will read (one-pass check)
	SourceStarts    map[string]float64

	src        io.ReaderAt
	anchors    countingReaderAt
	anchorRead atomic.Int64
	want       map[uint64]bool
	delayNS    map[uint64]int64
	timings    []segmentTiming
}

// SegmentResult is one extracted, analyzed, and sampled-verified fragment.
type SegmentResult struct {
	Index   int
	Body    []byte
	Init    []byte
	Info    FragmentInfo
	Anchors map[uint32]int64
}

// NewSession prepares extraction state for plan. outCursor is the journal's
// durable reverse cursor (plan.ReserveSize for a fresh run).
func NewSession(plan *Plan, src io.ReaderAt, outCursor int64) *Session {
	s := &Session{
		Plan:      plan,
		OutCursor: outCursor,
		src:       src,
		Fragments: make([]FragmentInfo, len(plan.Segments)),
		timings:   make([]segmentTiming, len(plan.Segments)),
		want:      make(map[uint64]bool, len(plan.TrackMap)),
		delayNS:   make(map[uint64]int64, len(plan.TrackMap)),
	}
	s.anchors = countingReaderAt{r: src, n: &s.anchorRead}
	for _, t := range plan.Tracks {
		if _, ok := plan.TrackMap[t.number]; ok {
			s.want[t.number] = true
			s.delayNS[t.number] = t.codecDelayNS
		}
	}
	return s
}

// Prepare extracts one segment, analyzes the fragment, computes the source
// anchors, runs the sampled decode gate, and strips the leading init. The
// caller writes Body at the returned offset with Place, then checkpoints.
func (s *Session) Prepare(ctx context.Context, i, verifyFrames int) (*SegmentResult, error) {
	if i < 0 || i >= len(s.Plan.Segments) {
		return nil, siphonerr.New(siphonerr.Internal, "segment %d is outside the plan", i)
	}
	seg := s.Plan.Segments[i]
	frag, err := extractSegment(ctx, s.src, s.Plan.Header, seg, &s.Stats)
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
	s.Stats.Segments++

	// First-block anchors: the absolute PTS (ns) of each track's first block
	// in the segment's opening clusters, minus the track's codec delay
	// (Matroska block timecodes are pre-delay; demuxers subtract CodecDelay).
	anchorsNS, err := firstBlockTimes(s.anchors, seg.SrcOff, seg.AnchorFrom, seg.SrcEnd,
		s.Plan.TimecodeScaleNS, s.want)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.StitchFailed, err,
			"segment %d: cannot read the first-block anchors", i)
	}
	if err := s.checkFirstPTS(seg, anchorsNS); err != nil {
		return nil, err
	}
	anchorNS := make(map[uint32]int64, len(s.Plan.TrackMap))
	for mkv, mp4 := range s.Plan.TrackMap {
		if ns, ok := anchorsNS[mkv]; ok {
			anchorNS[mp4] = ns - s.delayNS[mkv]
		} else {
			anchorNS[mp4] = seg.FirstPTSNS
		}
	}
	s.timings[i] = segmentTiming{moofs: info.Moofs, anchorNS: anchorNS}
	info.AnchorNS = anchorNS
	s.Fragments[i] = info

	if verifyFrames > 0 {
		if err := verifyFragmentBytes(ctx, frag, verifyFrames, &s.Stats); err != nil {
			return nil, siphonerr.Wrap(siphonerr.StitchFailed, err,
				"segment %d (%d..%d) failed the sampled decode", i, seg.SrcOff, seg.SrcEnd)
		}
	}

	ini, body, err := stripInit(frag)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.StitchFailed, err, "segment %d: bad fragment layout", i)
	}
	if i == len(s.Plan.Segments)-1 {
		if len(ini) == 0 {
			return nil, siphonerr.New(siphonerr.StitchFailed,
				"the first processed segment produced no init segment")
		}
		s.Init = ini
	}
	s.ExpectedExtract += segmentBytes(seg)
	if seg.TailCached {
		s.ExpectedExtract -= int64(len(seg.TailData))
	}
	return &SegmentResult{Index: i, Body: body, Init: ini, Info: info, Anchors: anchorNS}, nil
}

// checkFirstPTS validates the planned boundary against the source's actual
// first video block. The fragment's own PTS is rebased by the MP4 muxer, so the
// source anchor is the only reliable pre-truncation proof that src_off points
// at the keyframe the plan named (spec §4.5.4.3 step 2).
func (s *Session) checkFirstPTS(seg Segment, anchors map[uint64]int64) error {
	if s.Plan.VideoTrack == 0 {
		return nil
	}
	got, ok := anchors[s.Plan.VideoTrack]
	if !ok {
		return nil
	}
	frameNS := int64(0)
	for _, t := range s.Plan.Tracks {
		if t.number == s.Plan.VideoTrack {
			frameNS = t.defaultDurationNS
			break
		}
	}
	if frameNS <= 0 {
		return nil
	}
	if d := got - seg.FirstPTSNS; d < -frameNS || d > frameNS {
		return siphonerr.New(siphonerr.SegmentPlan,
			"segment %d: source keyframe PTS %d ns differs from the plan's %d ns by more than one frame",
			seg.Index, got, seg.FirstPTSNS)
	}
	return nil
}

// CacheTail reads the tail piece of segment next-1 before the next segment's
// source bytes are truncated. The tail is the next cluster's prefix (Timecode
// through the keyframe block); without it the previous segment's leading
// B-frames would reference frames that are no longer in the source.
func (s *Session) CacheTail(next int) error {
	if next <= 0 || next >= len(s.Plan.Segments) {
		return nil
	}
	prev := &s.Plan.Segments[next-1]
	if prev.TailTo <= prev.TailFrom {
		prev.TailData = nil
		prev.TailCached = false
		return nil
	}
	n := prev.TailTo - prev.TailFrom
	data := make([]byte, n)
	read, err := s.src.ReadAt(data, prev.TailFrom)
	if err != nil && !(errors.Is(err, io.EOF) && int64(read) == n) {
		return siphonerr.Wrap(siphonerr.StitchFailed, err,
			"cannot cache the tail prefix [%d,%d) for segment %d", prev.TailFrom, prev.TailTo, prev.Index)
	}
	if int64(read) != n {
		return siphonerr.New(siphonerr.StitchFailed,
			"tail prefix [%d,%d) for segment %d read %d bytes", prev.TailFrom, prev.TailTo, prev.Index, read)
	}
	s.Stats.ExtractRead += int64(read)
	prev.TailData = data
	prev.TailCached = false
	return nil
}

// Place writes one body at the reverse cursor and returns its output offset.
func (s *Session) Place(tmp io.WriterAt, body []byte) (int64, error) {
	if s.OutCursor < int64(len(body)) {
		return 0, siphonerr.New(siphonerr.NoSpaceRuntime,
			"reverse cursor underflow (reserve %d, body %d)", s.Plan.ReserveSize, len(body))
	}
	s.OutCursor -= int64(len(body))
	if _, err := tmp.WriteAt(body, s.OutCursor); err != nil {
		return 0, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot write the fragment")
	}
	s.Stats.Write += int64(len(body))
	return s.OutCursor, nil
}

// SetDone records a segment whose fragment is already durable in the temp.
func (s *Session) SetDone(i int, outOff, outLen int64, moofs int, anchors map[uint32]int64) {
	if i < 0 || i >= len(s.Fragments) {
		return
	}
	s.Fragments[i] = FragmentInfo{Index: i, Bytes: outLen, Moofs: moofs, AnchorNS: anchors}
	s.timings[i] = segmentTiming{moofs: moofs, anchorNS: anchors}
	s.Stats.Write += outLen
	if outOff < s.OutCursor {
		s.OutCursor = outOff
	}
}

// Finish writes the init segment and the leading free box, then re-anchors all
// fragment timestamps. It returns the global pad in ns.
func (s *Session) Finish(tmp *os.File) (int64, error) {
	if len(s.Init) == 0 {
		return 0, siphonerr.New(siphonerr.StitchFailed, "no init segment is available")
	}
	if s.OutCursor < int64(len(s.Init)) {
		return 0, siphonerr.New(siphonerr.NoSpaceRuntime, "reverse cursor underflow at the init segment")
	}
	s.OutCursor -= int64(len(s.Init))
	if _, err := tmp.WriteAt(s.Init, s.OutCursor); err != nil {
		return 0, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot write the init segment")
	}
	s.Stats.Write += int64(len(s.Init))
	if s.OutCursor > 0 {
		hdr, err := freeBoxHeader(s.OutCursor)
		if err != nil {
			return 0, siphonerr.New(siphonerr.NoSpaceRuntime, "%v", err)
		}
		if _, err := tmp.WriteAt(hdr, 0); err != nil {
			return 0, siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot write the free box")
		}
		s.Stats.Write += int64(len(hdr))
	}
	pad, err := patchTimestamps(tmp, s.Init, s.timings, &s.Stats)
	if err != nil {
		return 0, err
	}
	s.Stats.AnchorRead = s.anchorRead.Load()
	return pad, nil
}

// Verify runs the assembled-file gate. sourceCounts overrides the source probe
// after a resume; tolerant relaxes the comparison to ±1% (the planner's Cues
// estimate is all that survives once the tail is truncated).
func (s *Session) Verify(ctx context.Context, outPath string, padNS int64,
	sourceCounts map[string]int, tolerant, fullDecode bool) ([]Check, error) {
	s.Plan.ExtractBytes = s.ExpectedExtract
	rep := &Report{
		Plan:           s.Plan,
		Stats:          s.Stats,
		Fragments:      s.Fragments,
		OutPath:        outPath,
		PadNS:          padNS,
		SourceCounts:   sourceCounts,
		TolerantCounts: tolerant,
		SourceStarts:   s.SourceStarts,
	}
	checks := verifyAssembly(ctx, s.Plan, rep, Options{FullDecode: fullDecode})
	for _, c := range checks {
		if !c.OK {
			return checks, siphonerr.New(siphonerr.VerifyFailed, "%s: %s", c.Name, c.Detail)
		}
	}
	return checks, nil
}

// CountPackets returns per-stream packet counts keyed by codec type and ordinal
// ("video/0", "audio/0", …). Relocate captures the exact source counts before
// the first truncation.
func CountPackets(ctx context.Context, path string) (map[string]int, error) {
	return streamPacketCounts(ctx, path)
}

// RetainedCounts filters a count map to the streams relocate keeps (video and
// audio). Subtitles and attachments are dropped from the output.
func RetainedCounts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		if strings.HasPrefix(k, "video/") || strings.HasPrefix(k, "audio/") {
			out[k] = v
		}
	}
	return out
}

// SourceStarts returns the first packet PTS per codec type ("video", "audio")
// in seconds. Relocate captures it before the first truncation so the full gate
// can still compare the assembled timeline after the source media is gone.
func SourceStarts(ctx context.Context, path string) (map[string]float64, error) {
	return streamStartTimes(ctx, path)
}

// EstimateCounts is the journal's fallback packet estimate: duration divided
// by each retained track's Matroska DefaultDuration. It is used only after a
// resume, when the original tail is gone and the exact source count cannot be
// re-read.
func EstimateCounts(p *Plan) map[string]int {
	if p == nil || p.DurationNS <= 0 {
		return nil
	}
	out := map[string]int{}
	for _, t := range p.Tracks {
		if t.defaultDurationNS <= 0 {
			continue
		}
		n := int(p.DurationNS / t.defaultDurationNS)
		switch {
		case t.typ == 1 && t.number == p.VideoTrack:
			out["video/0"] = n
		case t.typ == 2 && t.number == p.AudioTrack:
			out["audio/0"] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func segmentBytes(seg Segment) int64 {
	var n int64
	for _, p := range seg.Pieces {
		for _, rg := range p.Ranges {
			n += rg[1] - rg[0]
		}
	}
	return n
}
