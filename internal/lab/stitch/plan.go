package stitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync/atomic"
	"time"

	"siphon/internal/siphonerr"
)

// Keyframe is one video keyframe together with the byte offsets needed to
// split its cluster around it.
type Keyframe struct {
	TimeNS     int64
	ClusterPos int64 // absolute file offset of the containing Cluster element
	KeyPos     int64 // absolute file offset of the keyframe's Block/SimpleBlock
	PayloadPos int64 // absolute file offset of the cluster payload (Timecode)
	ClusterEnd int64 // absolute file offset one past the Cluster element
	TCOff      int64 // Timecode element range [TCOff, TCEnd)
	TCEnd      int64
}

// Piece is one contiguous piece of a segment's synthetic Matroska stream.
// Ranges are source byte ranges copied in order; Wrap puts a freshly encoded
// Cluster header (with the concatenated payload length) in front of them.
type Piece struct {
	Ranges [][2]int64
	Wrap   bool
}

// Segment is one planned extraction: the source media range it covers plus the
// synthetic pieces fed to ffmpeg. A cluster whose first block is not the
// keyframe is split: its prefix rides at the tail of the previous segment (so
// the leading B-frames keep their decode order) and the next segment opens
// with a synthetic cluster that starts at the keyframe.
type Segment struct {
	Index      int
	SrcOff     int64 // first Cluster element offset (for reporting)
	SrcEnd     int64 // next keyframe block offset, or the end of the Segment
	AnchorFrom int64 // first byte the anchor walk considers in the opening cluster
	FirstPTSNS int64
	Pieces     []Piece
}

// Plan is everything the extraction loop needs.
type Plan struct {
	Path            string
	FileSize        int64
	TimecodeScaleNS int64
	DurationNS      int64
	PlanSource      string // "cues" or "scan"
	VideoTrack      uint64
	VideoCodec      string
	AudioTrack      uint64 // 0 when the file has no audio
	AudioCodec      string
	TrackMap        map[uint64]uint32 // MKV track number → MP4 track ID (video=1, audio=2)
	Tracks          []trackInfo
	Keyframes       []Keyframe
	Segments        []Segment
	ExtractBytes    int64 // exact source bytes the pieces copy (one pass)
	PlanRead        int64 // header/index/planning bytes read at plan time
	Header          []byte
	HeaderRegion    int64 // [0, HeaderRegion) is the cached header
	ReserveSize     int64
}

// Options are the spike's knobs. They mirror the relocate flags.
type Options struct {
	SegmentDuration  time.Duration
	SegmentTolerance time.Duration
	ForceScan        bool // use the ffprobe packet scan even when Cues exist
	VerifyFrames     int  // video frames decoded per fragment (spec default 3; 0 disables)
	FullDecode       bool // decode the whole assembled file as a final check
	Log              func(format string, args ...any)
}

// IOStats counts the bytes the spike moves.
type IOStats struct {
	PlanRead    int64 // header + index reads at plan time
	ExtractRead int64 // byte-range reads (source payload)
	HeaderSends int64 // bytes of cached header re-sent per segment
	AnchorRead  int64 // source bytes read to find the first-block anchors
	Write       int64 // fragment bytes written
	PatchRead   int64 // moof bytes read back by the timestamp patch pass
	PatchWrite  int64 // moof bytes rewritten by the patch pass
	Segments    int
	DecodeRuns  int
}

// countingReaderAt wraps a ReaderAt and counts bytes read.
type countingReaderAt struct {
	r io.ReaderAt
	n *atomic.Int64
}

func (c countingReaderAt) ReadAt(p []byte, off int64) (int, error) {
	n, err := c.r.ReadAt(p, off)
	c.n.Add(int64(n))
	return n, err
}

// BuildPlan parses path and returns a segment plan. Nothing is written.
func BuildPlan(ctx context.Context, path string, opt Options, stats *IOStats) (*Plan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot open %s", path)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot stat %s", path)
	}

	var planRead atomic.Int64
	cr := countingReaderAt{r: f, n: &planRead}
	defer func() {
		if stats != nil {
			stats.PlanRead = planRead.Load()
		}
	}()

	h, err := parseFileHeader(cr, st.Size())
	if err != nil {
		return nil, err
	}
	ix, err := scanTopLevel(cr, h, false)
	if err != nil {
		return nil, err
	}
	if ix.firstCluster == 0 {
		return nil, siphonerr.New(siphonerr.SourceInvalid, "%s has no Cluster elements", path)
	}

	scaleNS, durationNS := int64(1_000_000), int64(0)
	if ix.infoOff != 0 {
		scaleNS, durationNS, err = parseInfo(cr, ix.infoOff)
		if err != nil {
			return nil, err
		}
	}

	if ix.tracksOff == 0 {
		return nil, siphonerr.New(siphonerr.SourceInvalid, "%s has no Tracks element", path)
	}
	tracks, err := parseTracks(cr, ix.tracksOff)
	if err != nil {
		return nil, err
	}
	videoTrack, err := pickVideoTrack(tracks)
	if err != nil {
		return nil, err
	}
	audioTrack, err := pickAudioTrack(tracks)
	if err != nil {
		return nil, err
	}

	p := &Plan{
		Path:            path,
		FileSize:        st.Size(),
		TimecodeScaleNS: scaleNS,
		DurationNS:      durationNS,
		VideoTrack:      videoTrack.number,
		VideoCodec:      videoTrack.codec,
		AudioTrack:      audioTrack.number,
		AudioCodec:      audioTrack.codec,
		Tracks:          tracks,
		HeaderRegion:    ix.firstCluster,
	}
	p.TrackMap = map[uint64]uint32{videoTrack.number: 1}
	if audioTrack.number != 0 {
		p.TrackMap[audioTrack.number] = 2
	}

	// Boundary source: Cues first (spec §4.5.4.1 step 3).
	var kfs []Keyframe
	if !opt.ForceScan && ix.cuesOff != 0 {
		kfs, err = keyframesFromCues(cr, h, ix, videoTrack.number, scaleNS)
		if err != nil {
			return nil, err
		}
		if planCoverage(kfs, p.DurationNS) {
			p.PlanSource = "cues"
		} else {
			kfs = nil
		}
	}
	if kfs == nil {
		// The scan fallback needs every cluster offset.
		ix, err = scanTopLevel(cr, h, true)
		if err != nil {
			return nil, err
		}
		kfs, err = keyframesFromScan(ctx, cr, path, ix, videoTrack.number, h, scaleNS)
		if err != nil {
			return nil, err
		}
		p.PlanSource = "scan"
	}
	if len(kfs) == 0 {
		return nil, siphonerr.New(siphonerr.SegmentPlan, "no keyframes found in %s", path)
	}
	if kfs[0].ClusterPos != ix.firstCluster {
		return nil, siphonerr.New(siphonerr.SegmentPlan,
			"first keyframe cluster %d is not the first cluster %d", kfs[0].ClusterPos, ix.firstCluster)
	}
	p.Keyframes = kfs

	picks, err := selectBoundaries(kfs, p.DurationNS, opt.SegmentDuration, opt.SegmentTolerance)
	if err != nil {
		return nil, err
	}
	p.Segments = segmentsFromPicks(kfs, picks, h.segEnd)
	p.ExtractBytes = extractBytes(p.Segments)

	p.Header, err = cacheHeader(cr, h, ix, ix.firstCluster)
	if err != nil {
		return nil, err
	}
	p.ReserveSize = reserveSize(st.Size())
	p.PlanRead = planRead.Load()
	return p, nil
}

// pickVideoTrack returns the single video track; multi-video is out of scope
// for the spike.
func pickVideoTrack(tracks []trackInfo) (trackInfo, error) {
	var vids []trackInfo
	for _, t := range tracks {
		if t.typ == 1 {
			vids = append(vids, t)
		}
	}
	switch len(vids) {
	case 0:
		return trackInfo{}, siphonerr.New(siphonerr.FormatUnsupported, "no video track")
	case 1:
		return vids[0], nil
	}
	return trackInfo{}, siphonerr.New(siphonerr.FormatUnsupported,
		"%d video tracks; the spike plans single-video files only", len(vids))
}

// pickAudioTrack returns the single audio track, or the zero track when the
// file has none. Multi-audio is out of scope for the spike (the `-map 0:a?`
// would emit several tracks; the anchor patch maps exactly one).
func pickAudioTrack(tracks []trackInfo) (trackInfo, error) {
	var auds []trackInfo
	for _, t := range tracks {
		if t.typ == 2 {
			auds = append(auds, t)
		}
	}
	switch len(auds) {
	case 0:
		return trackInfo{}, nil
	case 1:
		return auds[0], nil
	}
	return trackInfo{}, siphonerr.New(siphonerr.FormatUnsupported,
		"%d audio tracks; the spike plans single-audio files only", len(auds))
}

// keyframesFromCues builds the keyframe table from the Cues element.
func keyframesFromCues(r io.ReaderAt, h fileHeader, ix topIndex, videoTrack uint64, scaleNS int64) ([]Keyframe, error) {
	e, err := readElem(r, ix.cuesOff)
	if err != nil {
		return nil, err
	}
	cues, err := parseCues(r, e)
	if err != nil {
		return nil, err
	}
	var out []Keyframe
	seen := map[int64]bool{}
	for _, c := range cues {
		if c.track != videoTrack {
			continue
		}
		pos := h.segStart + c.clusterPos
		if pos < ix.firstCluster || pos >= h.segEnd || pos >= h.fileSize {
			continue
		}
		if seen[pos] {
			continue
		}
		seen[pos] = true
		cl, err := readElem(r, pos)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SegmentPlan, err,
				"cue at %d ms points at an unreadable cluster (%d)", c.timeTicks, pos)
		}
		if cl.id != idCluster || cl.unknown || cl.end() > h.segEnd {
			return nil, siphonerr.New(siphonerr.SegmentPlan,
				"cue at %d ms points at element 0x%X (%d), not a sized Cluster", c.timeTicks, cl.id, pos)
		}
		kf, err := keyframeBlock(r, cl, videoTrack, scaleNS, c.timeTicks*scaleNS)
		if err != nil {
			return nil, err
		}
		out = append(out, kf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClusterPos < out[j].ClusterPos })
	return out, nil
}

// keyframeBlock locates the video keyframe block inside a cluster. Clusters
// can begin with the previous GOP's trailing B-frames (decode order), so the
// keyframe is not always the first video block; timeNS (from Cues or the
// packet scan) selects it. It also records the Timecode element range so a
// synthetic cluster can be rebuilt around just the keyframe.
func keyframeBlock(r io.ReaderAt, cl elem, videoTrack uint64, scaleNS, timeNS int64) (Keyframe, error) {
	base := Keyframe{ClusterPos: cl.off, PayloadPos: cl.payload, ClusterEnd: cl.end()}
	var clusterTime int64 = -1
	var first, best *Keyframe
	bestDist := int64(math.MaxInt64)
	record := func(k Keyframe) {
		k.ClusterPos, k.PayloadPos, k.ClusterEnd = base.ClusterPos, base.PayloadPos, base.ClusterEnd
		k.TCOff, k.TCEnd = base.TCOff, base.TCEnd
		if first == nil {
			f := k
			first = &f
		}
		d := k.TimeNS - timeNS
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			b := k
			best, bestDist = &b, d
		}
	}
	err := forChildren(r, cl.payload, cl.end(), func(c elem) error {
		switch c.id {
		case idTimecode:
			if base.TCOff != 0 {
				return nil
			}
			v, err := uintPayload(r, c)
			if err != nil {
				return err
			}
			clusterTime = int64(v)
			base.TCOff, base.TCEnd = c.off, c.end()
		case idSimpleBlock:
			if clusterTime < 0 {
				return siphonerr.New(siphonerr.SegmentPlan,
					"cluster at %d has a block before its Timecode", cl.off)
			}
			track, tc, flags, err := readBlockHeader(r, c.payload, c.end())
			if err != nil || track != videoTrack || flags&0x80 == 0 {
				return nil
			}
			record(Keyframe{TimeNS: (clusterTime + tc) * scaleNS, KeyPos: c.off})
		case idBlockGroup:
			if clusterTime < 0 {
				return siphonerr.New(siphonerr.SegmentPlan,
					"cluster at %d has a block before its Timecode", cl.off)
			}
			var blockOff int64
			var track uint64
			var tc int64
			var hasRef bool
			err := forChildren(r, c.payload, c.end(), func(g elem) error {
				switch g.id {
				case idBlock:
					blockOff = g.off
					track, tc, _, _ = readBlockHeader(r, g.payload, g.end())
				case idReferenceBlock:
					hasRef = true
				}
				return nil
			})
			if err != nil {
				return err
			}
			if blockOff != 0 && track == videoTrack && !hasRef {
				record(Keyframe{TimeNS: (clusterTime + tc) * scaleNS, KeyPos: blockOff})
			}
		}
		return nil
	})
	if err != nil {
		return Keyframe{}, err
	}
	if base.TCOff == 0 {
		return Keyframe{}, siphonerr.New(siphonerr.SegmentPlan,
			"cluster at %d has no Timecode element", cl.off)
	}
	if best == nil || bestDist > scaleNS {
		best = first // a cue a tick or two off still names this cluster's keyframe
	}
	if best == nil {
		return Keyframe{}, siphonerr.New(siphonerr.SegmentPlan,
			"no video keyframe block in the cluster at %d", cl.off)
	}
	return *best, nil
}

// planCoverage reports whether the Cues-derived keyframes reach ≥90% of the
// file's duration (spec §4.5.4.1: otherwise fall back to the packet scan).
func planCoverage(kfs []Keyframe, durationNS int64) bool {
	if len(kfs) == 0 {
		return false
	}
	if durationNS <= 0 {
		return true // duration unknown; accept what the index offers
	}
	return kfs[len(kfs)-1].TimeNS*10 >= durationNS*9
}

// keyframesFromScan is the fallback: ffprobe -show_packets (streamed JSON)
// for keyframe (pts, pos), plus the top-level cluster offsets, mapped to the
// cluster that contains each keyframe.
func keyframesFromScan(ctx context.Context, r io.ReaderAt, path string, ix topIndex, videoTrack uint64, h fileHeader, scaleNS int64) ([]Keyframe, error) {
	if len(ix.clusters) == 0 {
		return nil, siphonerr.New(siphonerr.FormatUnsupported,
			"the file's clusters have unknown sizes; the scan fallback needs sized clusters")
	}
	type pkt struct {
		PTSTime string `json:"pts_time"`
		Flags   string `json:"flags"`
		Pos     string `json:"pos"`
	}
	// ffprobe emits {"packets":[{...},{...}]}; decode packet by packet so a
	// 20 MB JSON never sits in memory at once.
	cmd := makeFFprobe(ctx,
		"-v", "error",
		"-select_streams", fmt.Sprintf("v:%d", 0),
		"-show_packets",
		"-show_entries", "packet=pts_time,flags,pos",
		"-of", "json",
		path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Dependency, err, "cannot start ffprobe")
	}

	type sample struct {
		timeNS int64
		pos    int64
	}
	var samples []sample
	dec := json.NewDecoder(stdout)
	// Skip into the packets array.
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = cmd.Wait()
			return nil, siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot parse ffprobe output")
		}
		if d, ok := tok.(json.Delim); ok && d == '[' {
			break
		}
	}
	for dec.More() {
		var p pkt
		if err := dec.Decode(&p); err != nil {
			_ = cmd.Wait()
			return nil, siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot parse ffprobe packet")
		}
		if !containsFlag(p.Flags, 'K') {
			continue
		}
		ns, ok := parsePTS(p.PTSTime)
		if !ok {
			continue
		}
		var pos int64 = -1
		if _, err := fmt.Sscanf(p.Pos, "%d", &pos); err != nil {
			pos = -1
		}
		if pos < 0 {
			continue
		}
		samples = append(samples, sample{timeNS: ns, pos: pos})
	}
	if err := cmd.Wait(); err != nil {
		return nil, siphonerr.Wrap(siphonerr.VerifyFailed, err, "ffprobe failed")
	}

	// Map each keyframe packet to its containing cluster (the greatest
	// cluster offset ≤ packet position), keeping the first per cluster.
	clusters := ix.clusters
	var out []Keyframe
	lastCluster := int64(-1)
	for _, s := range samples {
		i := sort.Search(len(clusters), func(i int) bool { return clusters[i] > s.pos }) - 1
		if i < 0 {
			continue
		}
		c := clusters[i]
		if c == lastCluster {
			continue
		}
		lastCluster = c
		cl, err := readElem(r, c)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SegmentPlan, err,
				"scan keyframe at %d ms points at an unreadable cluster (%d)", s.timeNS/1e6, c)
		}
		if cl.id != idCluster || cl.unknown || cl.end() > h.segEnd {
			return nil, siphonerr.New(siphonerr.SegmentPlan,
				"scan keyframe at %d ms points at element 0x%X (%d), not a sized Cluster", s.timeNS/1e6, cl.id, c)
		}
		kf, err := keyframeBlock(r, cl, videoTrack, scaleNS, s.timeNS)
		if err != nil {
			return nil, err
		}
		out = append(out, kf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClusterPos < out[j].ClusterPos })
	return out, nil
}

func containsFlag(flags string, f byte) bool {
	for i := 0; i < len(flags); i++ {
		if flags[i] == f {
			return true
		}
	}
	return false
}

func parsePTS(s string) (int64, bool) {
	if s == "" || s == "N/A" {
		return 0, false
	}
	sec, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return int64(sec * 1e9), true
}

// selectBoundaries picks keyframe indexes at least segmentDuration apart,
// snapped to the nearest keyframe within tolerance (spec §4.5.4.1 step 4).
func selectBoundaries(kfs []Keyframe, durationNS int64, segDur, tol time.Duration) ([]int, error) {
	if segDur <= 0 || tol < 0 {
		return nil, siphonerr.New(siphonerr.Usage, "segment duration must be positive")
	}
	end := durationNS
	if end <= 0 {
		end = kfs[len(kfs)-1].TimeNS
	}
	segNS := int64(segDur)
	tolNS := int64(tol)
	picks := []int{0}
	for {
		target := kfs[picks[len(picks)-1]].TimeNS + segNS
		// The remaining tail is shorter than the window (within tolerance):
		// let the last segment run to EOF.
		if target >= end-tolNS {
			break
		}
		best, bestDist := -1, int64(math.MaxInt64)
		for i := picks[len(picks)-1] + 1; i < len(kfs); i++ {
			t := kfs[i].TimeNS
			if t < target-tolNS {
				continue
			}
			if t > target+tolNS {
				break
			}
			d := t - target
			if d < 0 {
				d = -d
			}
			if d < bestDist {
				best, bestDist = i, d
			}
		}
		if best == -1 {
			return nil, siphonerr.New(siphonerr.SegmentPlan,
				"no keyframe within %s of %s after keyframe %d",
				tol, time.Duration(target), picks[len(picks)-1])
		}
		picks = append(picks, best)
	}
	return picks, nil
}

// segmentsFromPicks converts boundary keyframe indexes into synthetic pieces.
// The opening cluster is rebuilt from its Timecode plus the keyframe block
// onward; when the next boundary cluster starts with the previous GOP's
// trailing B-frames, those blocks ride at the tail of this segment (their
// decode order is after the previous GOP and before the next keyframe). The
// last segment runs to the end of the Segment box.
func segmentsFromPicks(kfs []Keyframe, picks []int, segEnd int64) []Segment {
	out := make([]Segment, 0, len(picks))
	for j, idx := range picks {
		kf := kfs[idx]
		seg := Segment{
			Index:      j,
			SrcOff:     kf.ClusterPos,
			SrcEnd:     segEnd,
			AnchorFrom: kf.KeyPos,
			FirstPTSNS: kf.TimeNS,
		}
		if j == 0 {
			// The first segment keeps everything before its keyframe
			// (e.g. AAC priming blocks that precede it in the cluster).
			seg.AnchorFrom = kf.PayloadPos
		}
		startEnd := kf.ClusterEnd
		var next *Keyframe
		if j+1 < len(picks) {
			nk := kfs[picks[j+1]]
			next = &nk
			seg.SrcEnd = nk.KeyPos
			if nk.ClusterPos == kf.ClusterPos && nk.KeyPos < startEnd {
				startEnd = nk.KeyPos
			}
		}
		start := Piece{Wrap: true}
		if j == 0 {
			start.Ranges = [][2]int64{{kf.PayloadPos, startEnd}}
		} else {
			start.Ranges = [][2]int64{{kf.TCOff, kf.TCEnd}, {kf.KeyPos, startEnd}}
		}
		seg.Pieces = append(seg.Pieces, start)
		switch {
		case next != nil && next.ClusterPos != kf.ClusterPos:
			if kf.ClusterEnd < next.ClusterPos {
				seg.Pieces = append(seg.Pieces, Piece{
					Ranges: [][2]int64{{kf.ClusterEnd, next.ClusterPos}},
				})
			}
			// Start at the Timecode, not the cluster payload: a leading
			// CRC-32 covers the whole source cluster and would no longer
			// match this truncated prefix.
			if next.TCOff < next.KeyPos {
				seg.Pieces = append(seg.Pieces, Piece{
					Ranges: [][2]int64{{next.TCOff, next.KeyPos}},
					Wrap:   true,
				})
			}
		case next == nil:
			if kf.ClusterEnd < segEnd {
				seg.Pieces = append(seg.Pieces, Piece{
					Ranges: [][2]int64{{kf.ClusterEnd, segEnd}},
				})
			}
		}
		out = append(out, seg)
	}
	return out
}

// extractBytes is the exact number of source bytes the pieces copy.
func extractBytes(segs []Segment) int64 {
	var n int64
	for _, s := range segs {
		for _, p := range s.Pieces {
			for _, rg := range p.Ranges {
				n += rg[1] - rg[0]
			}
		}
	}
	return n
}

// cacheHeader reads [0, firstCluster) and rewrites the Segment size to
// "unknown" so a byte range fed behind it demuxes to EOF naturally. When the
// existing size vint is 8 bytes this is an in-place patch; a narrower vint
// forces a rebuild, and any SeekHead in the cache is voided because its
// relative positions no longer hold.
func cacheHeader(r io.ReaderAt, h fileHeader, ix topIndex, firstCluster int64) ([]byte, error) {
	if firstCluster <= 0 || firstCluster > 64<<20 {
		return nil, siphonerr.New(siphonerr.SegmentPlan,
			"header region %d bytes is outside the supported range", firstCluster)
	}
	buf := make([]byte, firstCluster)
	if err := readFull(r, buf, 0); err != nil {
		return nil, err
	}
	off := int(h.segSize.sizeOff)
	if off+h.segSize.sizeLen > len(buf) {
		return nil, siphonerr.New(siphonerr.SegmentPlan, "Segment size vint outside the cached header")
	}
	if h.segSize.sizeLen == 8 {
		copy(buf[off:off+8], unknownSizeVint)
		return buf, nil
	}
	// Rebuild with an 8-byte unknown size and void the SeekHead.
	delta := 8 - h.segSize.sizeLen
	rebuilt := make([]byte, 0, len(buf)+delta)
	rebuilt = append(rebuilt, buf[:off]...)
	rebuilt = append(rebuilt, unknownSizeVint...)
	rebuilt = append(rebuilt, buf[off+h.segSize.sizeLen:]...)
	if ix.seekHeadOff != 0 {
		se, err := readElem(r, ix.seekHeadOff)
		if err == nil && !se.unknown {
			total := int64(se.hdrLen) + int64(se.size)
			if err := voidElement(rebuilt, ix.seekHeadOff+int64(delta), total); err != nil {
				return nil, err
			}
		}
	}
	return rebuilt, nil
}

// voidElement overwrites one whole element at off with a legal Void of the
// same total length (ID + size vint + zero padding).
func voidElement(buf []byte, off, total int64) error {
	if off < 0 || total < 2 || off+total > int64(len(buf)) {
		return siphonerr.New(siphonerr.SegmentPlan, "cannot void element at %d (%d bytes)", off, total)
	}
	base := int(off)
	n := int(total)
	for sl := 1; sl <= 8; sl++ {
		val := n - 1 - sl
		if val < 0 || int64(val) > int64(1)<<(7*sl)-1 {
			continue
		}
		buf[base] = 0xEC
		for i := 0; i < sl; i++ {
			buf[base+1+i] = byte(val >> (8 * (sl - 1 - i)))
		}
		buf[base+1] |= byte(0x80 >> (sl - 1))
		for i := 1 + sl; i < n; i++ {
			buf[base+i] = 0
		}
		return nil
	}
	return siphonerr.New(siphonerr.SegmentPlan, "cannot encode a Void of %d bytes", total)
}

// reserveSize is spec §4.5.4.1 step 6: ceil(max(output, source) × 1.05) + 16 MiB.
func reserveSize(sourceSize int64) int64 {
	return (sourceSize*105+99)/100 + 16<<20
}

// humanDuration renders a duration for the report.
func humanDuration(d time.Duration) string {
	return d.Round(time.Millisecond).String()
}

// outputName is where a spike run writes its assembled file.
func outputName(in string) string {
	ext := filepath.Ext(in)
	return filepath.Join(os.TempDir(), "siphon-stitch-"+filepath.Base(in[:len(in)-len(ext)])+".mp4")
}
