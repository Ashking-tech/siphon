// Package mediaengine implements the v0.2 media pairs that share ffmpeg:
// container remuxes, WebM ↔ MP4, video → MP3, and GIF ↔ MP4. Every path uses
// an explicit stream map, verifies the candidate before installation, and
// leaves the source untouched on any failure.
package mediaengine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"siphon/internal/core"
	"siphon/internal/engine/safeio"
	"siphon/internal/formats"
	"siphon/internal/media"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Engine converts one planned media job in safe mode.
type Engine struct{}

// New returns the engine (registered by internal/engines).
func New() *Engine { return &Engine{} }

// Name identifies the engine.
func (*Engine) Name() string { return string(formats.EngineMedia) }

// Convert runs the ffmpeg-based pipeline.
func (e *Engine) Convert(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	if sink == nil {
		sink = progress.Noop{}
	}
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}
	if err := validateJob(job); err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}

	dir := filepath.Dir(job.SourcePath)
	stem := safeio.Stem(job.SourcePath)
	src, err := safeio.OpenSource(job.SourcePath)
	if err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	defer src.Close()
	safeio.Reap(dir, stem, sink)

	sink.Emit(progress.Event{Phase: "probe", Percent: 1, Message: filepath.Base(job.SourcePath)})
	probe, err := media.ProbeFile(ctx, job.SourcePath)
	if err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "media conversion failed")
	}

	var gifInfo *media.GIFInfo
	if job.Source == formats.GIF {
		gifInfo, err = media.ProbeGIF(job.SourcePath)
		if err != nil {
			return nil, safeio.Untouched(err, "media conversion failed")
		}
	}

	pp, err := media.BuildPairPlan(probe, job.Source, job.Target, pairOptions(job))
	if err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	if job.Source == formats.GIF && job.Target == formats.MP4 &&
		gifInfo.Transparent && job.Background == "" {
		return nil, safeio.Untouched(siphonerr.New(siphonerr.AlphaUnsupported,
			"%s has GIF transparency; pass --bg <colour> to flatten it",
			job.SourcePath).WithHints(map[string]bool{"bg_required": true}),
			"media conversion failed")
	}

	// The temp carries the target extension so the muxer and the reaper both
	// see a conventional suffix; the explicit -f argument still decides the
	// container.
	suffix := ".tmp." + job.Target.Ext()
	tmp, err := safeio.CreateLockedTemp(dir, stem, suffix)
	if err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	tmpPath := tmp.Name()
	defer tmp.Close()
	defer platform.RemoveIfExists(tmpPath)

	args, err := media.BuildFFmpegArgs(pp, job.SourcePath, tmpPath, gifBackground(job, gifInfo))
	if err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}

	duration := probe.Duration()
	if gifInfo != nil && gifInfo.Total > 0 {
		duration = gifInfo.Total
	}
	sink.Emit(progress.Event{Phase: "encode", Percent: 5, Message: filepath.Base(job.SourcePath)})
	if err := runFFmpeg(ctx, args, duration, sink); err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	if err := tmp.Close(); err != nil {
		return nil, safeio.Untouched(siphonerr.Wrap(siphonerr.Internal, err,
			"cannot close the temp file"), "media conversion failed")
	}

	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	sink.Emit(progress.Event{Phase: "verify", Percent: 82})
	if err := verify(ctx, job, pp, probe, gifInfo, tmpPath, sink); err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	if err := src.Snapshot.Check(job.SourcePath); err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}

	metaWarnings, err := platform.CopyMetadata(job.SourcePath, tmpPath)
	if err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	for _, w := range metaWarnings {
		sink.Warn(w)
	}
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}

	res, err := safeio.Finalize(ctx, job, plan, src.Snapshot, tmpPath, sink)
	if err != nil {
		return nil, safeio.Untouched(err, "media conversion failed")
	}
	tmpPath = ""
	return res, nil
}

func pairOptions(job *core.Job) media.PairOptions {
	return media.PairOptions{
		DropIncompatible: job.DropIncompatible,
		AllowNonstandard: job.AllowNonstandard,
		Transcode:        job.Transcode,
		DropMetadata:     job.DropMetadata,
		VideoQuality:     job.VideoQuality,
		VideoQualitySet:  job.VideoQualitySet,
		AudioBitrate:     job.AudioBitrate,
		AudioTrack:       job.AudioTrack,
		FPS:              job.FPS,
		Width:            job.Width,
	}
}

// gifBackground applies --bg only when the GIF actually has transparency.
// Without transparency the padding colour is black (spec §10.5).
func gifBackground(job *core.Job, info *media.GIFInfo) string {
	if job.Source != formats.GIF || job.Target != formats.MP4 ||
		info == nil || !info.Transparent {
		return ""
	}
	return job.Background
}

func validateJob(job *core.Job) error {
	if job == nil {
		return siphonerr.New(siphonerr.Internal, "nil media job")
	}
	p := job.Pair
	switch {
	case formats.IsContainerRemux(p), formats.IsWebMMP4(p),
		formats.IsAudioExtract(p), formats.IsGIFPair(p):
	default:
		return siphonerr.New(siphonerr.FormatUnsupported,
			"media engine cannot convert %s → %s", job.Source, job.Target)
	}
	return nil
}

// runFFmpeg executes the explicit argv and maps -progress pipe:1 to the sink.
func runFFmpeg(ctx context.Context, args []string, duration time.Duration, sink progress.Sink) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return siphonerr.Wrap(siphonerr.Internal, err, "cannot read ffmpeg progress")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return siphonerr.Wrap(siphonerr.Dependency, err, "cannot start ffmpeg")
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			line := sc.Text()
			v, ok := strings.CutPrefix(line, "out_time_us=")
			if !ok {
				continue
			}
			us, err := strconv.ParseInt(v, 10, 64)
			if err != nil || duration <= 0 {
				continue
			}
			pct := 5 + 70*float64(us)/float64(duration.Microseconds())
			if pct > 75 {
				pct = 75
			}
			if pct < 5 {
				pct = 5
			}
			sink.Emit(progress.Event{Phase: "encode", Percent: pct})
		}
	}()

	waitErr := cmd.Wait()
	<-done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return safeio.RuntimeFailure(waitErr, stderr.String(), "ffmpeg")
	}
	return nil
}

func verify(ctx context.Context, job *core.Job, pp *media.PairPlan, src *media.Probe,
	gifInfo *media.GIFInfo, outPath string, sink progress.Sink) error {
	out, err := media.ProbeFile(ctx, outPath)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot probe the generated output")
	}
	if err := verifyContainer(job, out); err != nil {
		return err
	}

	switch {
	case job.Target == formats.MP3:
		if err := verifyAudio(job, src, out); err != nil {
			return err
		}
	case job.Source == formats.GIF && job.Target == formats.MP4:
		if err := verifyGIFToMP4(job, pp, src, gifInfo, out); err != nil {
			return err
		}
	case job.Source == formats.MP4 && job.Target == formats.GIF:
		if err := verifyMP4ToGIF(ctx, job, pp, out); err != nil {
			return err
		}
	case pp.Remux:
		if err := verifyRemux(ctx, job, pp, src, out, sink); err != nil {
			return err
		}
	default:
		if err := verifyAVTranscode(job, pp, src, out, sink); err != nil {
			return err
		}
	}

	sink.Emit(progress.Event{Phase: "verify", Percent: 88, Message: "full decode"})
	if err := media.DecodeCheck(ctx, outPath); err != nil {
		return err
	}
	return outputSize(job, pp, src, outPath)
}

func verifyContainer(job *core.Job, out *media.Probe) error {
	name := strings.ToLower(out.Format.FormatName)
	tokens := strings.Split(name, ",")
	has := func(want string) bool {
		for _, t := range tokens {
			if strings.TrimSpace(t) == want {
				return true
			}
		}
		return false
	}
	ok := false
	switch job.Target {
	case formats.MP4, formats.MOV:
		// ffprobe reports the ISO BMFF family as mov,mp4,m4a,3gp,3g2,mj2.
		ok = has("mov") || has("mp4") || has("m4a") || has("3gp") || has("3g2") || has("mj2")
	case formats.MKV, formats.WEBM:
		ok = has("matroska") || has("webm")
	case formats.MP3:
		ok = has("mp3")
	case formats.GIF:
		ok = has("gif")
	}
	if !ok {
		return verifyErr("output container is %q, want %s", out.Format.FormatName, job.Target)
	}
	return nil
}

func verifyRemux(ctx context.Context, job *core.Job, pp *media.PairPlan, src, out *media.Probe, sink progress.Sink) error {
	if len(out.Streams) != len(pp.Streams) {
		return verifyErr("output has %d streams, plan retained %d", len(out.Streams), len(pp.Streams))
	}
	outToSrc := make([]int, len(out.Streams))
	for i, want := range pp.Streams {
		got := out.Streams[i]
		outToSrc[i] = want.Index
		if got.CodecType != want.Type {
			return verifyErr("output stream %d is %s, want %s", i, got.CodecType, want.Type)
		}
		if !strings.EqualFold(got.CodecName, want.Codec) {
			return verifyErr("output stream %d codec is %s, want %s", i, got.CodecName, want.Codec)
		}
	}
	if err := checkDuration(src.Duration(), out.Duration(), videoTolerance(src, 2)); err != nil {
		return err
	}

	srcCounts, err := media.CountPackets(ctx, job.SourcePath)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot count source packets")
	}
	if err := verifyPackets(ctx, pp, srcCounts, out); err != nil {
		return err
	}

	wantChapters := 0
	if pp.Chapters && pp.TargetContainer != "mov" {
		wantChapters = len(src.Chapters)
	}
	if len(out.Chapters) != wantChapters {
		return verifyErr("output has %d chapters, plan expects %d", len(out.Chapters), wantChapters)
	}
	wantAttachments := countType(pp.Streams, "attachment")
	if got := len(streamsOf(out, "attachment")); got != wantAttachments {
		return verifyErr("output has %d attachment streams, plan expects %d", got, wantAttachments)
	}
	if got := len(streamsOf(out, "subtitle")); got != 0 {
		return verifyErr("output has %d subtitle streams, plan retained none", got)
	}
	return checkTags(job, src, out, outToSrc, sink)
}

// verifyPackets is separated to keep the remux verification readable and to
// make the source/output alignment explicit.
func verifyPackets(ctx context.Context, pp *media.PairPlan, srcCounts []media.PacketCount, out *media.Probe) error {
	outCounts, err := media.CountPackets(ctx, out.Format.Filename)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot count output packets")
	}
	srcByIndex := make(map[int]int64, len(srcCounts))
	for _, c := range srcCounts {
		srcByIndex[c.Index] = c.Packets
	}
	outByIndex := make(map[int]media.PacketCount, len(outCounts))
	for _, c := range outCounts {
		outByIndex[c.Index] = c
	}
	for i, want := range pp.Streams {
		if want.Action != media.ActionCopy {
			continue
		}
		if want.Type != "video" && want.Type != "audio" {
			// Attachments and data streams are verified by count elsewhere;
			// packet-count probes may omit them or report N/A.
			continue
		}
		if i >= len(out.Streams) {
			return verifyErr("output is missing stream %d (%s)", i, want.Codec)
		}
		got, ok := outByIndex[out.Streams[i].Index]
		if !ok || got.CodecType != want.Type {
			return verifyErr("packet probe has no %s stream for output stream %d", want.Type, i)
		}
		srcPackets, ok := srcByIndex[want.Index]
		if !ok {
			return verifyErr("source has no packet count for stream %d", want.Index)
		}
		if got.Packets != srcPackets {
			return verifyErr("stream %d (%s): output has %d packets, source %d",
				i, want.Codec, got.Packets, srcPackets)
		}
	}
	return nil
}

func verifyAVTranscode(job *core.Job, pp *media.PairPlan, src, out *media.Probe, sink progress.Sink) error {
	video := streamsOf(out, "video")
	audio := streamsOf(out, "audio")
	if len(video) != 1 {
		return verifyErr("output has %d video streams, want 1", len(video))
	}
	wantAudio := 0
	if pp.AudioIndex >= 0 {
		wantAudio = 1
	}
	if len(audio) != wantAudio {
		return verifyErr("output has %d audio streams, want %d", len(audio), wantAudio)
	}
	if got := strings.ToLower(video[0].CodecName); got != pp.TargetVideoCodec {
		return verifyErr("output video codec is %s, want %s", got, pp.TargetVideoCodec)
	}
	if wantAudio == 1 {
		if got := strings.ToLower(audio[0].CodecName); got != pp.TargetAudioCodec {
			return verifyErr("output audio codec is %s, want %s", got, pp.TargetAudioCodec)
		}
	}
	if pp.Width > 0 && pp.Height > 0 {
		if video[0].Width != pp.Width || video[0].Height != pp.Height {
			return verifyErr("output is %dx%d, want %dx%d",
				video[0].Width, video[0].Height, pp.Width, pp.Height)
		}
	}
	if video[0].Width%2 != 0 || video[0].Height%2 != 0 {
		return verifyErr("output dimensions %dx%d are not even", video[0].Width, video[0].Height)
	}
	if err := checkDuration(src.Duration(), out.Duration(), videoTolerance(src, 2)); err != nil {
		return err
	}
	if err := checkExtraStreams(out); err != nil {
		return err
	}
	return checkTags(job, src, out, pairOutToSrc(pp), sink)
}

func verifyAudio(job *core.Job, src, out *media.Probe) error {
	video := streamsOf(out, "video")
	audio := streamsOf(out, "audio")
	if len(video) != 0 {
		return verifyErr("audio output has %d video streams", len(video))
	}
	if len(audio) != 1 {
		return verifyErr("audio output has %d audio streams, want 1", len(audio))
	}
	if got := strings.ToLower(audio[0].CodecName); got != "mp3" {
		return verifyErr("audio codec is %s, want mp3", got)
	}
	if audio[0].SampleRate != "" {
		rate, err := strconv.Atoi(audio[0].SampleRate)
		if err != nil || rate < 8000 || rate > 48000 {
			return verifyErr("MP3 sample rate %q is not valid", audio[0].SampleRate)
		}
	}
	if audio[0].Channels < 1 || audio[0].Channels > 2 {
		return verifyErr("MP3 has %d channels, want 1 or 2", audio[0].Channels)
	}
	if src.Duration() > 0 {
		if err := checkDuration(src.Duration(), out.Duration(), durationPercent(src.Duration(), 5)); err != nil {
			return err
		}
	}
	if job.AudioBitrate > 0 {
		got, err := strconv.ParseInt(audio[0].BitRate, 10, 64)
		if err != nil || got <= 0 {
			return verifyErr("CBR MP3 has no usable bitrate")
		}
		want := int64(job.AudioBitrate) * 1000
		if diff := abs64(got - want); diff > want/20 {
			return verifyErr("MP3 bitrate is %d kbps, want %d kbps (±5%%)",
				got/1000, job.AudioBitrate)
		}
	}
	return nil
}

func verifyGIFToMP4(job *core.Job, pp *media.PairPlan, src *media.Probe, gifInfo *media.GIFInfo, out *media.Probe) error {
	video := streamsOf(out, "video")
	if len(streamsOf(out, "audio")) != 0 {
		return verifyErr("GIF → MP4 output must not contain audio")
	}
	if len(video) != 1 {
		return verifyErr("output has %d video streams, want 1", len(video))
	}
	if got := strings.ToLower(video[0].CodecName); got != "h264" {
		return verifyErr("output video codec is %s, want h264", got)
	}
	if video[0].Width != pp.Width || video[0].Height != pp.Height {
		return verifyErr("output is %dx%d, want %dx%d",
			video[0].Width, video[0].Height, pp.Width, pp.Height)
	}
	if video[0].Width%2 != 0 || video[0].Height%2 != 0 {
		return verifyErr("output dimensions %dx%d are not even", video[0].Width, video[0].Height)
	}
	wantDuration := src.Duration()
	if gifInfo != nil && gifInfo.Total > 0 {
		wantDuration = gifInfo.Total
	}
	if err := checkDuration(wantDuration, out.Duration(), durationPercent(wantDuration, 5)); err != nil {
		return err
	}
	return checkExtraStreams(out)
}

func verifyMP4ToGIF(ctx context.Context, job *core.Job, pp *media.PairPlan, out *media.Probe) error {
	video := streamsOf(out, "video")
	if len(streamsOf(out, "audio")) != 0 {
		return verifyErr("GIF output must not contain audio")
	}
	if len(video) != 1 {
		return verifyErr("output has %d video streams, want 1", len(video))
	}
	if got := strings.ToLower(video[0].CodecName); got != "gif" {
		return verifyErr("output video codec is %s, want gif", got)
	}
	if video[0].Width != pp.Width || video[0].Height != pp.Height {
		return verifyErr("output is %dx%d, want %dx%d",
			video[0].Width, video[0].Height, pp.Width, pp.Height)
	}
	frames, err := media.CountFrames(ctx, out.Format.Filename)
	if err != nil {
		return err
	}
	var total int64
	for _, f := range frames {
		if f.CodecType == "video" {
			total += f.Frames
		}
	}
	if total < 1 && video[0].FrameCount() < 1 {
		return verifyErr("GIF has no decodable frames")
	}
	if pp.FPS > 0 && total > 0 && out.Duration() > 0 {
		actual := float64(total) / out.Duration().Seconds()
		if absFloat(actual-pp.FPS) > 1.0 {
			return verifyErr("GIF frame rate is %.3f (%d frames / %s), want %.3f (±1 frame)",
				actual, total, out.Duration(), pp.FPS)
		}
	}
	return checkExtraStreams(out)
}

func outputSize(job *core.Job, pp *media.PairPlan, src *media.Probe, path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot stat the output")
	}
	if st.Size() <= 0 {
		return verifyErr("output is empty")
	}
	switch {
	case job.Target == formats.GIF:
		// GIF is intentionally resampled; no duration floor is applied.
		return nil
	case src.Duration() > 0:
		if st.Size() < int64(src.Duration().Seconds()*512) {
			return verifyErr("output is implausibly small (%d bytes for %s)", st.Size(), src.Duration())
		}
	}
	return nil
}

func checkExtraStreams(out *media.Probe) error {
	for _, s := range out.Streams {
		switch s.CodecType {
		case "video", "audio":
		default:
			return verifyErr("output contains an unexpected %s stream", s.CodecType)
		}
	}
	return nil
}

func checkTags(job *core.Job, src, out *media.Probe, outToSrc []int, sink progress.Sink) error {
	if outToSrc == nil {
		outToSrc = make([]int, len(out.Streams))
		for i := range out.Streams {
			outToSrc[i] = i
		}
	}
	missing := media.MissingTags(src, out, outToSrc)
	if len(missing) == 0 {
		return nil
	}
	if !job.DropMetadata {
		return siphonerr.New(siphonerr.MetadataDrop,
			"output cannot carry these tags: %s; pass --drop-metadata to accept this",
			strings.Join(missing, ", ")).WithHints(map[string]bool{"drop_metadata": true})
	}
	sink.Warn("--drop-metadata set: tags not carried: " + strings.Join(missing, ", "))
	return nil
}

func countType(streams []media.PairStream, typ string) int {
	n := 0
	for _, s := range streams {
		if s.Type == typ {
			n++
		}
	}
	return n
}

func pairOutToSrc(pp *media.PairPlan) []int {
	out := make([]int, 0, len(pp.Streams))
	for _, s := range pp.Streams {
		out = append(out, s.Index)
	}
	return out
}

func streamsOf(p *media.Probe, typ string) []media.Stream {
	var out []media.Stream
	for _, s := range p.Streams {
		if s.CodecType == typ {
			out = append(out, s)
		}
	}
	return out
}

func videoTolerance(src *media.Probe, frames int) time.Duration {
	video := src.VideoStreams()
	fps := 0.0
	if len(video) > 0 {
		fps = video[0].FrameRate()
	}
	t := durationPercent(src.Duration(), 1)
	if fps > 0 {
		frame := time.Duration(float64(time.Second)/fps) * time.Duration(frames)
		if frame > t {
			t = frame
		}
	}
	if t < 50*time.Millisecond {
		t = 50 * time.Millisecond
	}
	return t
}

func durationPercent(d time.Duration, pct int) time.Duration {
	if d <= 0 {
		return 100 * time.Millisecond
	}
	return d * time.Duration(pct) / 100
}

func checkDuration(src, out, tolerance time.Duration) error {
	if src <= 0 || out <= 0 {
		return nil
	}
	if abs(src-out) > tolerance {
		return verifyErr("output duration %s differs from source %s by %s (tolerance %s)",
			out, src, abs(src-out), tolerance)
	}
	return nil
}

func verifyErr(format string, args ...any) error {
	return siphonerr.New(siphonerr.VerifyFailed, "output verification failed: "+fmt.Sprintf(format, args...))
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func absFloat(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
