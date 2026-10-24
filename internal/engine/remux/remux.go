// Package remux implements the safe MKV → MP4 path from spec §4.5.3: ffmpeg
// remuxes into a same-directory temp, the result is verified and given the
// source's metadata, and only then is the source replaced. The source is
// never modified until that final rename, so a crash leaves it byte-identical.
package remux

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/media"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Engine converts one planned MKV job in safe mode.
type Engine struct{}

// New returns the engine (registered by internal/engines).
func New() *Engine { return &Engine{} }

// Name identifies the engine.
func (*Engine) Name() string { return string(formats.EngineRemux) }

// Convert runs the remux pipeline.
func (e *Engine) Convert(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	if sink == nil {
		sink = progress.Noop{}
	}
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}

	dir := filepath.Dir(job.SourcePath)
	stem := stemOf(job.SourcePath)
	snap, err := platform.TakeSnapshot(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}
	if removed, err := platform.ReapTemps(dir, stem, time.Hour); err == nil && len(removed) > 0 {
		sink.Warn(fmt.Sprintf("removed %d stale temp file(s) from an earlier run", len(removed)))
	}

	probe, err := media.ProbeFile(ctx, job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}
	tp, err := media.BuildTrackPlan(probe, media.TrackOptions{
		DropIncompatible: job.DropIncompatible,
		TranscodeAudio:   job.TranscodeAudio,
		AllowNonstandard: job.AllowNonstandard,
	})
	if err != nil {
		return nil, untouched(err)
	}
	if !job.DropMetadata && (tp.Attachments || tp.Chapters) {
		var lost []string
		if tp.Attachments {
			lost = append(lost, "attachments")
		}
		if tp.Chapters {
			lost = append(lost, "chapters")
		}
		return nil, untouched(siphonerr.New(siphonerr.MetadataDrop,
			"source carries %s, which cannot be carried into MP4; pass --drop-metadata to accept this",
			strings.Join(lost, " and ")))
	}

	tmp, err := platform.CreateTemp(dir, stem, ".tmp")
	if err != nil {
		return nil, untouched(err)
	}
	tmpPath := tmp.Name()
	if err := platform.LockFile(tmp); err != nil {
		tmp.Close()
		platform.RemoveIfExists(tmpPath)
		return nil, untouched(err)
	}
	defer tmp.Close()
	cleanup := true
	defer func() {
		if cleanup {
			platform.RemoveIfExists(tmpPath)
		}
	}()

	sink.Emit(progress.Event{Phase: "encode", Percent: 5, Message: filepath.Base(job.SourcePath)})
	if err := runFFmpeg(ctx, job.SourcePath, tmpPath, tp, probe.Duration(), sink); err != nil {
		if ctx.Err() != nil {
			return nil, interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, untouched(err)
	}

	sink.Emit(progress.Event{Phase: "verify", Percent: 82})
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, untouched(err)
	}
	if err := verify(ctx, job, plan, probe, tp, tmpPath, sink); err != nil {
		if ctx.Err() != nil {
			return nil, interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, untouched(err)
	}

	metaWarnings, err := platform.CopyMetadata(job.SourcePath, tmpPath)
	if err != nil {
		return nil, untouched(err)
	}
	for _, w := range metaWarnings {
		sink.Warn(w)
	}
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, untouched(err)
	}

	if err := snap.Check(job.SourcePath); err != nil {
		return nil, untouched(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, interrupted(err, job.SourcePath)
	}

	sink.Emit(progress.Event{Phase: "finalise", Percent: 95})
	if job.Force {
		err = platform.RenameReplace(tmpPath, job.TargetPath)
	} else {
		err = platform.RenameNoReplace(tmpPath, job.TargetPath)
	}
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, untouched(siphonerr.New(siphonerr.TargetExists,
				"%s appeared while converting (use --force to replace it)", job.TargetPath))
		}
		return nil, untouched(siphonerr.Wrap(siphonerr.Internal, err, "cannot finalise the output"))
	}
	cleanup = false

	if err := os.Remove(job.SourcePath); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err,
			"output is at %s but the source could not be removed", job.TargetPath).Untouched(true)
	}
	if err := platform.FsyncDir(dir); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err, "cannot fsync %s", dir)
	}
	sink.Emit(progress.Event{Phase: "complete", Percent: 100})
	return &core.Result{OK: true, Plan: plan, SourceUntouched: false}, nil
}

// runFFmpeg remuxes into tmpPath. -progress pipe:1 drives the sink without
// mixing with the error log on stderr.
func runFFmpeg(ctx context.Context, srcPath, tmpPath string, tp *media.TrackPlan, duration time.Duration, sink progress.Sink) error {
	args := []string{
		"-nostdin", "-v", "error", "-y", "-nostats",
		"-progress", "pipe:1",
		"-i", srcPath,
	}
	args = append(args, tp.MapArgs()...)
	args = append(args, tp.CodecArgs()...)
	args = append(args,
		"-map_metadata", "0",
		"-map_chapters", "-1",
		"-sn", "-dn",
		"-movflags", "+faststart",
		"-f", "mp4",
		tmpPath,
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
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
			if v, ok := strings.CutPrefix(line, "out_time_us="); ok {
				if us, err := strconv.ParseInt(v, 10, 64); err == nil && duration > 0 {
					pct := 5 + 70*float64(us)/float64(duration.Microseconds())
					if pct > 75 {
						pct = 75
					}
					sink.Emit(progress.Event{Phase: "encode", Percent: pct})
				}
			}
		}
	}()

	waitErr := cmd.Wait()
	<-done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if waitErr != nil {
		return siphonerr.Wrap(siphonerr.Internal, waitErr,
			"ffmpeg failed: %s", mediaTail(stderr.Bytes()))
	}
	return nil
}

// verify is the full gate for the safe path: streams/codecs, duration, exact
// packet counts for copied streams, tags, a complete decode, and a size sanity
// check.
func verify(ctx context.Context, job *core.Job, plan *core.Plan, src *media.Probe, tp *media.TrackPlan, outPath string, sink progress.Sink) error {
	out, err := media.ProbeFile(ctx, outPath)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot probe the remuxed output")
	}
	if len(out.Streams) != len(tp.Tracks) {
		return siphonerr.New(siphonerr.VerifyFailed,
			"output has %d streams, plan retained %d", len(out.Streams), len(tp.Tracks))
	}
	outToSrc := make([]int, len(out.Streams))
	for i, want := range tp.Tracks {
		got := out.Streams[i]
		outToSrc[i] = want.Index
		if got.CodecType != want.Type {
			return siphonerr.New(siphonerr.VerifyFailed,
				"output stream %d is %s, want %s", i, got.CodecType, want.Type)
		}
		wantCodec := want.Codec
		if want.Action == media.ActionTranscode {
			wantCodec = "aac"
		}
		if !strings.EqualFold(got.CodecName, wantCodec) {
			return siphonerr.New(siphonerr.VerifyFailed,
				"output stream %d codec is %s, want %s", i, got.CodecName, wantCodec)
		}
	}

	if d := abs(out.Duration() - src.Duration()); d > tolerance(src.Duration()) {
		return siphonerr.New(siphonerr.VerifyFailed,
			"output duration %s differs from source %s by %s", out.Duration(), src.Duration(), d)
	}

	srcCounts, err := media.CountPackets(ctx, job.SourcePath)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot count source packets")
	}
	outCounts, err := media.CountPackets(ctx, outPath)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot count output packets")
	}
	srcByIndex := make(map[int]int64, len(srcCounts))
	for _, c := range srcCounts {
		srcByIndex[c.Index] = c.Packets
	}
	if len(outCounts) != len(tp.Tracks) {
		return siphonerr.New(siphonerr.VerifyFailed,
			"packet probe saw %d output streams, plan retained %d", len(outCounts), len(tp.Tracks))
	}
	for i, want := range tp.Tracks {
		if want.Action != media.ActionCopy {
			continue
		}
		if outCounts[i].Packets != srcByIndex[want.Index] {
			return siphonerr.New(siphonerr.VerifyFailed,
				"stream %d (%s): output has %d packets, source %d",
				i, want.Codec, outCounts[i].Packets, srcByIndex[want.Index])
		}
	}

	if missing := media.MissingTags(src, out, outToSrc); len(missing) > 0 {
		if !job.DropMetadata {
			return siphonerr.New(siphonerr.MetadataDrop,
				"output cannot carry these tags: %s; pass --drop-metadata to accept this",
				strings.Join(missing, ", "))
		}
		sink.Warn(fmt.Sprintf("--drop-metadata set: tags not carried: %s", strings.Join(missing, ", ")))
	}

	sink.Emit(progress.Event{Phase: "verify", Percent: 88, Message: "full decode"})
	if err := media.DecodeCheck(ctx, outPath); err != nil {
		return err
	}

	st, err := os.Stat(outPath)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot stat the output")
	}
	if st.Size() <= 0 || st.Size() < int64(src.Duration().Seconds()*1024) {
		return siphonerr.New(siphonerr.VerifyFailed,
			"output is implausibly small (%d bytes for %s)", st.Size(), src.Duration())
	}
	return nil
}

func tolerance(d time.Duration) time.Duration {
	t := d / 100
	if t < 100*time.Millisecond {
		t = 100 * time.Millisecond
	}
	return t
}

func abs(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func stemOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func untouched(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return interrupted(err, "")
	}
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(true)
		return err
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "remux failed").Untouched(true)
}

func interrupted(err error, source string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return siphonerr.New(siphonerr.Internal, "timed out").Untouched(true)
	}
	where := source
	if where == "" {
		where = "<file>"
	}
	return siphonerr.New(siphonerr.Interrupted,
		"Interrupted. Source safe. Run 'siphon recover %s'.", where).Untouched(true)
}

func mediaTail(b []byte) string {
	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	if len(lines) == 0 {
		return "(no stderr)"
	}
	s := strings.TrimSpace(string(lines[len(lines)-1]))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
