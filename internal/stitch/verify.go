package stitch

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
)

// assembledMoof is one moof read back from the assembled file.
type assembledMoof struct {
	off    int64
	seq    uint32
	tfdts  map[uint32]uint64
	cts    map[uint32]bool
	ctsMin map[uint32]int64
}

// scanOutputBoxes walks the assembled file's top-level box chain, reading
// headers only except for moof bodies (which are small). It returns every
// moof, the first box type, and the file size.
func scanOutputBoxes(path string) (moofs []assembledMoof, firstBox string, size int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", 0, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, "", 0, err
	}
	off := int64(0)
	first := true
	for off+8 <= st.Size() {
		var hdr [16]byte
		if _, err := f.ReadAt(hdr[:8], off); err != nil {
			return nil, "", 0, err
		}
		sz := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:8])
		hdrLen := int64(8)
		switch {
		case sz == 1:
			if _, err := f.ReadAt(hdr[8:16], off+8); err != nil {
				return nil, "", 0, err
			}
			sz = int64(binary.BigEndian.Uint64(hdr[8:16]))
			hdrLen = 16
		case sz == 0:
			sz = st.Size() - off
		}
		if first {
			firstBox = typ
			first = false
		}
		if sz < hdrLen || off+sz > st.Size() {
			return nil, "", 0, fmt.Errorf("box %s at %d overruns the file", typ, off)
		}
		if typ == "moof" {
			buf := make([]byte, sz)
			if _, err := f.ReadAt(buf, off); err != nil {
				return nil, "", 0, err
			}
			m := assembledMoof{off: off, tfdts: map[uint32]uint64{}, cts: map[uint32]bool{}, ctsMin: map[uint32]int64{}}
			children, err := walkBoxes(buf, int(hdrLen), len(buf))
			if err != nil {
				return nil, "", 0, err
			}
			for _, c := range children {
				switch c.typ {
				case "mfhd":
					p := c.off + c.hdr
					if p+8 <= c.off+c.size {
						m.seq = binary.BigEndian.Uint32(buf[p+4:])
					}
				case "traf":
					ti, err := parseTraf(buf, c)
					if err == nil && ti.track != 0 {
						m.tfdts[ti.track] = ti.tfdt
						m.cts[ti.track] = ti.ctsPresent
						m.ctsMin[ti.track] = ti.ctsMin
					}
				}
			}
			moofs = append(moofs, m)
		}
		off += sz
	}
	return moofs, firstBox, st.Size(), nil
}

// streamPacketCounts runs ffprobe -count_packets and returns per-stream packet
// counts keyed by codec type and ordinal ("video/0", "audio/0", …).
func streamPacketCounts(ctx context.Context, path string) (map[string]int, error) {
	cmd := makeFFprobe(ctx, "-v", "error", "-count_packets",
		"-show_entries", "stream=codec_type,nb_read_packets",
		"-of", "csv=p=0", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe -count_packets %s: %w", path, err)
	}
	counts := map[string]int{}
	ord := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			continue
		}
		typ := strings.TrimSpace(parts[0])
		key := fmt.Sprintf("%s/%d", typ, ord[typ])
		ord[typ]++
		counts[key] = n
	}
	return counts, nil
}

// streamStartTimes returns the first packet PTS (seconds) of each codec type,
// reading only the first packets of the file.
func streamStartTimes(ctx context.Context, path string) (map[string]float64, error) {
	type stream struct {
		Index     int    `json:"index"`
		CodecType string `json:"codec_type"`
	}
	type packet struct {
		StreamIndex int    `json:"stream_index"`
		PTSTime     string `json:"pts_time"`
	}
	var raw struct {
		Streams []stream `json:"streams"`
		Packets []packet `json:"packets"`
	}
	cmd := makeFFprobe(ctx, "-v", "error",
		"-read_intervals", "%+#64",
		"-show_entries", "stream=index,codec_type:packet=stream_index,pts_time",
		"-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe start times %s: %w", path, err)
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ffprobe start times %s: %w", path, err)
	}
	types := map[int]string{}
	for _, s := range raw.Streams {
		types[s.Index] = s.CodecType
	}
	starts := map[string]float64{}
	for _, p := range raw.Packets {
		typ := types[p.StreamIndex]
		if typ == "" || p.PTSTime == "" || p.PTSTime == "N/A" {
			continue
		}
		if _, seen := starts[typ]; seen {
			continue
		}
		v, err := strconv.ParseFloat(p.PTSTime, 64)
		if err != nil {
			continue
		}
		starts[typ] = v
	}
	return starts, nil
}

// matroskaCodecName maps the CodecID stored in trackInfo to ffprobe's
// codec_name, which is what the output probe reports.
func matroskaCodecName(id string) string {
	switch {
	case strings.HasPrefix(id, "V_MPEG4/ISO/AVC"):
		return "h264"
	case strings.HasPrefix(id, "V_MPEGH/ISO/HEVC"):
		return "hevc"
	case strings.HasPrefix(id, "V_AV1"):
		return "av1"
	case strings.HasPrefix(id, "V_MPEG4/ISO/ASP"):
		return "mpeg4"
	case strings.HasPrefix(id, "V_VP8"):
		return "vp8"
	case strings.HasPrefix(id, "V_VP9"):
		return "vp9"
	case strings.HasPrefix(id, "A_AAC"):
		return "aac"
	case strings.HasPrefix(id, "A_ALAC"):
		return "alac"
	case strings.HasPrefix(id, "A_AC3"):
		return "ac3"
	case strings.HasPrefix(id, "A_EAC3"):
		return "eac3"
	case strings.HasPrefix(id, "A_FLAC"):
		return "flac"
	case strings.HasPrefix(id, "A_OPUS"):
		return "opus"
	case strings.HasPrefix(id, "A_DTS"):
		return "dts"
	case strings.HasPrefix(id, "A_TRUEHD"):
		return "truehd"
	default:
		return strings.ToLower(id)
	}
}

// outputStreams returns the output's video/audio stream types and codecs in
// container order. The full gate compares this against the retained plan.
func outputStreams(ctx context.Context, path string) ([]struct{ Typ, Codec string }, error) {
	var raw struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			CodecName string `json:"codec_name"`
		} `json:"streams"`
	}
	cmd := makeFFprobe(ctx, "-v", "error",
		"-show_entries", "stream=codec_type,codec_name",
		"-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe streams %s: %w", path, err)
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("ffprobe streams %s: %w", path, err)
	}
	var streams []struct{ Typ, Codec string }
	for _, s := range raw.Streams {
		if s.CodecType == "video" || s.CodecType == "audio" {
			streams = append(streams, struct{ Typ, Codec string }{s.CodecType, strings.ToLower(s.CodecName)})
		}
	}
	return streams, nil
}

// SourceDuration reads the retained A/V duration before relocation truncates
// the source. Per-stream durations are used because a delayed audio track can
// legitimately extend past Matroska's Info duration; the engine journals the
// value for the full gate.
func SourceDuration(ctx context.Context, path string) (int64, error) {
	d, err := maxAVDuration(ctx, path)
	if err != nil {
		return 0, err
	}
	return int64(d * 1e9), nil
}

// maxAVDuration returns the longest video/audio stream duration, falling back
// to the container duration when streams carry none. It deliberately ignores
// subtitle durations: relocation drops subtitles.
func maxAVDuration(ctx context.Context, path string) (float64, error) {
	var raw struct {
		Streams []struct {
			CodecType string `json:"codec_type"`
			Duration  string `json:"duration"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	cmd := makeFFprobe(ctx, "-v", "error",
		"-show_entries", "stream=codec_type,duration:format=duration",
		"-of", "json", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe duration %s: %w", path, err)
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return 0, fmt.Errorf("ffprobe duration %s: %w", path, err)
	}
	max := 0.0
	for _, s := range raw.Streams {
		if s.CodecType != "video" && s.CodecType != "audio" {
			continue
		}
		v, err := strconv.ParseFloat(s.Duration, 64)
		if err == nil && v > max {
			max = v
		}
	}
	if max == 0 {
		if v, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil {
			max = v
		}
	}
	if max == 0 {
		return 0, fmt.Errorf("ffprobe duration %s: no video/audio duration", path)
	}
	return max, nil
}

// videoFrameNS returns the selected video track's Matroska DefaultDuration,
// used for the full gate's "two frames" duration tolerance.
func videoFrameNS(p *Plan) int64 {
	if p == nil {
		return 0
	}
	for _, t := range p.Tracks {
		if t.number == p.VideoTrack {
			return t.defaultDurationNS
		}
	}
	return 0
}

// fullDecode decodes the whole file with -xerror.
func fullDecode(ctx context.Context, path string) error {
	cmd := makeFFmpeg(ctx, "-nostdin", "-v", "error", "-xerror",
		"-i", path, "-f", "null", "-")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("decode failed: %s", stderrTail(stderr.Bytes()))
	}
	return nil
}

// verifyAssembly runs the structural checks on the assembled file plus the
// packet-count, timeline, and (optionally) full-decode checks.
func verifyAssembly(ctx context.Context, plan *Plan, rep *Report, opt Options) []Check {
	var checks []Check
	add := func(name string, ok bool, format string, args ...any) {
		checks = append(checks, Check{Name: name, OK: ok, Detail: fmt.Sprintf(format, args...)})
	}

	moofs, firstBox, size, err := scanOutputBoxes(rep.OutPath)
	if err != nil {
		add("box chain", false, "%v", err)
		return checks
	}
	expectedMoofs := 0
	for _, f := range rep.Fragments {
		expectedMoofs += f.Moofs
	}
	add("box chain", len(moofs) == expectedMoofs && (firstBox == "free" || firstBox == "ftyp"),
		"first box %s, %d moofs (expected %d), %d bytes", firstBox, len(moofs), expectedMoofs, size)

	// Full-gate identity: the output must retain exactly the plan's video/audio
	// tracks and codecs. A resume with every segment done has no source track
	// table left; packet counts and decode still cover that case.
	if len(plan.Tracks) > 0 {
		// Extraction maps video first, then audio, regardless of the order
		// of the source's Matroska tracks. Build the expected list in that
		// output order.
		want := make([]struct{ Typ, Codec string }, 0, len(plan.TrackMap))
		for _, t := range plan.Tracks {
			if t.number != plan.VideoTrack {
				continue
			}
			if _, ok := plan.TrackMap[t.number]; ok {
				want = append(want, struct{ Typ, Codec string }{"video", matroskaCodecName(t.codec)})
			}
		}
		for _, t := range plan.Tracks {
			if t.number != plan.AudioTrack {
				continue
			}
			if _, ok := plan.TrackMap[t.number]; ok {
				want = append(want, struct{ Typ, Codec string }{"audio", matroskaCodecName(t.codec)})
			}
		}
		got, err := outputStreams(ctx, rep.OutPath)
		if err != nil {
			add("stream identity", false, "%v", err)
		} else {
			ok := len(got) == len(want)
			var b strings.Builder
			for i := range want {
				if i >= len(got) {
					fmt.Fprintf(&b, "missing %s/%s; ", want[i].Typ, want[i].Codec)
					continue
				}
				fmt.Fprintf(&b, "%s/%s vs %s/%s; ", got[i].Typ, got[i].Codec, want[i].Typ, want[i].Codec)
				if got[i].Typ != want[i].Typ || got[i].Codec != want[i].Codec {
					ok = false
				}
			}
			for i := len(want); i < len(got); i++ {
				fmt.Fprintf(&b, "extra %s/%s; ", got[i].Typ, got[i].Codec)
			}
			add("stream identity", ok, "%s", strings.TrimSpace(b.String()))
		}
	}

	// Full-gate duration and sanity. Prefer the journaled source container
	// duration; the lab still has its source, so it can probe it live.
	srcDurNS := plan.SourceDurationNS
	if srcDurNS == 0 && plan.Path != "" {
		if d, derr := maxAVDuration(ctx, plan.Path); derr == nil && d > 0 {
			srcDurNS = int64(d * 1e9)
		}
	}
	if srcDurNS == 0 {
		srcDurNS = plan.DurationNS
	}
	if srcDurNS > 0 {
		dur, err := maxAVDuration(ctx, rep.OutPath)
		if err != nil {
			add("duration", false, "%v", err)
		} else {
			src := float64(srcDurNS) / 1e9
			tol := src / 100
			frameTol := 2.0 / 30.0
			if ns := videoFrameNS(plan); ns > 0 {
				frameTol = 2 * float64(ns) / 1e9
			}
			if tol < frameTol {
				tol = frameTol
			}
			add("duration", math.Abs(dur-src) <= tol,
				"output %.6fs, source %.6fs (Δ %.6fs, tolerance %.6fs)", dur, src, math.Abs(dur-src), tol)
		}
		min := int64(math.Ceil(float64(srcDurNS) / 1e9 * 1024))
		add("size sanity", size >= min, "output %d B, minimum %d B for %.3fs",
			size, min, float64(srcDurNS)/1e9)
	}

	seqOK := len(moofs) > 0
	seqDetail := fmt.Sprintf("%d fragments", len(moofs))
	for i, m := range moofs {
		if int(m.seq) != i+1 {
			seqOK = false
			seqDetail = fmt.Sprintf("moof %d carries sequence %d", i, m.seq)
			break
		}
	}
	if seqOK && len(moofs) > 0 {
		seqDetail = fmt.Sprintf("%d fragments numbered 1..%d", len(moofs), len(moofs))
	}
	add("mfhd order", seqOK, "%s", seqDetail)

	last := map[uint32]uint64{}
	mono := len(moofs) > 0
	monoDetail := fmt.Sprintf("non-decreasing per track over %d moofs", len(moofs))
	for i, m := range moofs {
		tracks := make([]uint32, 0, len(m.tfdts))
		for track := range m.tfdts {
			tracks = append(tracks, track)
		}
		sort.Slice(tracks, func(a, b int) bool { return tracks[a] < tracks[b] })
		for _, track := range tracks {
			t := m.tfdts[track]
			if prev, ok := last[track]; ok && t < prev {
				mono = false
				monoDetail = fmt.Sprintf("moof %d track %d tfdt %d < previous %d", i, track, t, prev)
				break
			}
			last[track] = t
		}
		if !mono {
			break
		}
	}
	add("tfdt monotonic", mono, "%s", monoDetail)

	// Composition offsets must be non-negative: ffmpeg's mov demuxer derives a
	// per-track dts_shift from the most negative offset, which would make the
	// demuxed timeline depend on how much of the file was parsed.
	ctsOK := len(moofs) > 0
	ctsDetail := fmt.Sprintf("no negative composition offsets over %d moofs", len(moofs))
	for i, m := range moofs {
		tracks := make([]uint32, 0, len(m.cts))
		for track := range m.cts {
			tracks = append(tracks, track)
		}
		sort.Slice(tracks, func(a, b int) bool { return tracks[a] < tracks[b] })
		for _, track := range tracks {
			if !m.cts[track] {
				continue
			}
			if m.ctsMin[track] < 0 {
				ctsOK = false
				ctsDetail = fmt.Sprintf("moof %d track %d min cts %d", i, track, m.ctsMin[track])
				break
			}
		}
		if !ctsOK {
			break
		}
	}
	add("cts non-negative", ctsOK, "%s", ctsDetail)

	srcCounts := rep.SourceCounts
	var err1 error
	if srcCounts == nil {
		srcCounts, err1 = streamPacketCounts(ctx, plan.Path)
	}
	outCounts, err2 := streamPacketCounts(ctx, rep.OutPath)
	if err1 == nil && err2 == nil {
		keys := map[string]bool{}
		for k := range srcCounts {
			keys[k] = true
		}
		for k := range outCounts {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		ok := true
		var b strings.Builder
		for _, k := range sorted {
			fmt.Fprintf(&b, "%s %d/%d  ", k, outCounts[k], srcCounts[k])
			tol := 0
			if rep.TolerantCounts {
				tol = srcCounts[k] / 100
				if tol < 2 {
					tol = 2
				}
			}
			if d := outCounts[k] - srcCounts[k]; d > tol || d < -tol {
				ok = false
			}
		}
		label := "output/source"
		if rep.TolerantCounts {
			label = "output/estimate (±1%)"
		}
		add("packet counts", ok, "%s: %s", label, strings.TrimSpace(b.String()))
	} else {
		add("packet counts", false, "ffprobe failed: %v %v", err1, err2)
	}

	// Timeline: the assembled file may carry one global pad, but every stream's
	// offset relative to the video must match the source.
	srcStarts := rep.SourceStarts
	var startErr error
	if srcStarts == nil {
		srcStarts, startErr = streamStartTimes(ctx, plan.Path)
	}
	outStarts, outStartErr := streamStartTimes(ctx, rep.OutPath)
	if startErr == nil && outStartErr == nil {
		pad := float64(rep.PadNS) / 1e9
		sv, okSrc := srcStarts["video"]
		ov, okOut := outStarts["video"]
		if okSrc && okOut {
			diff := ov - sv - pad
			add("start anchor", diff < 0.010 && diff > -0.010,
				"video %.6fs, source %.6fs, pad %.6fs (Δ %.6fs)", ov, sv, pad, diff)
		} else {
			add("start anchor", false, "no video first-packet PTS (source %v, output %v)", okSrc, okOut)
		}
		if okSrc && okOut {
			var b strings.Builder
			ok := true
			for _, typ := range []string{"audio"} {
				sa, ok1 := srcStarts[typ]
				oa, ok2 := outStarts[typ]
				if !ok1 || !ok2 {
					continue
				}
				relSrc, relOut := sa-sv, oa-ov
				d := relOut - relSrc
				fmt.Fprintf(&b, "%s offset %.6fs vs source %.6fs (Δ %.6fs)", typ, relOut, relSrc, d)
				if d > 0.010 || d < -0.010 {
					ok = false
				}
			}
			if b.Len() == 0 {
				add("a/v sync", true, "single stream, nothing to compare")
			} else {
				add("a/v sync", ok, "%s", b.String())
			}
		}
	} else {
		add("start anchor", false, "ffprobe failed: %v %v", startErr, outStartErr)
	}

	// One-pass read: each payload byte the pieces copy was read exactly once
	// (cluster boundaries are rebuilt, so the synthetic stream is not a single
	// contiguous source range).
	payload := plan.ExtractBytes
	if payload == 0 {
		payload = extractBytes(plan.Segments)
	}
	planRead := plan.PlanRead
	if planRead == 0 {
		planRead = rep.Stats.PlanRead
	}
	add("one-pass read", rep.Stats.ExtractRead == payload,
		"payload %d B read as %d B; header %d B resent %d× (%d B), anchors %d B, plan %d B, moof patch r/w %d/%d B",
		payload, rep.Stats.ExtractRead, plan.HeaderRegion, rep.Stats.Segments,
		rep.Stats.HeaderSends, rep.Stats.AnchorRead, planRead,
		rep.Stats.PatchRead, rep.Stats.PatchWrite)

	if opt.FullDecode {
		if err := fullDecode(ctx, rep.OutPath); err != nil {
			add("full decode", false, "%v", err)
		} else {
			add("full decode", true, "-xerror clean")
		}
	}
	return checks
}
