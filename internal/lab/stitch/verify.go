package stitch

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
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

	srcCounts, err1 := streamPacketCounts(ctx, plan.Path)
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
			if srcCounts[k] != outCounts[k] {
				ok = false
			}
		}
		add("packet counts", ok, "output/source: %s", strings.TrimSpace(b.String()))
	} else {
		add("packet counts", false, "ffprobe failed: %v %v", err1, err2)
	}

	// Timeline: the assembled file may carry one global pad, but every stream's
	// offset relative to the video must match the source.
	srcStarts, err1 := streamStartTimes(ctx, plan.Path)
	outStarts, err2 := streamStartTimes(ctx, rep.OutPath)
	if err1 == nil && err2 == nil {
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
		add("start anchor", false, "ffprobe failed: %v %v", err1, err2)
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
