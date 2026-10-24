// Package sliding implements the audio engine from spec §4.5.2: WAV → MP3 via
// LAME. safe mode builds the MP3 in a same-directory temp and leaves the
// source untouched until the output is written, fsynced, and verified.
// reclaim mode implements the forward-cursor invariant of spec §4.4.1: the
// output is written into the source's own head behind the read cursor, with a
// journal for splice-resume.
package sliding

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
	"siphon/internal/wav"
)

// Engine converts one planned audio job.
type Engine struct{}

// New returns the engine (registered by internal/engines).
func New() *Engine { return &Engine{} }

// Name identifies the engine.
func (*Engine) Name() string { return string(formats.EngineSliding) }

// Convert dispatches on the resolved mode.
func (e *Engine) Convert(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	if sink == nil {
		sink = progress.Noop{}
	}
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}
	switch job.Mode {
	case formats.ModeReclaim:
		return convertReclaim(ctx, job, plan, sink)
	default:
		return convertSafe(ctx, job, plan, sink)
	}
}

// lameArgs builds the encoder invocation for a job: VBR -V 2 by default,
// CBR when --bitrate was given (spec §4.5.2, §5.1).
func lameArgs(job *core.Job) []string {
	args := []string{"--silent"}
	if job.Bitrate > 0 {
		return append(args, "-b", strconv.Itoa(job.Bitrate))
	}
	return append(args, "-V", "2")
}

// convertSafe is the immutable-source path: LAME reads the WAV from a pipe
// (fed from the source fd) and writes the MP3 to a same-directory temp. The
// source is only unlinked after the temp is verified and renamed.
func convertSafe(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	dir := filepath.Dir(job.SourcePath)
	stem := stemOf(job.SourcePath)
	snap, err := platform.TakeSnapshot(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}

	wavInfo, err := wav.Parse(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}

	if removed, err := platform.ReapTemps(dir, stem, time.Hour); err == nil && len(removed) > 0 {
		sink.Warn(fmt.Sprintf("removed %d stale temp file(s) from an earlier run", len(removed)))
	}

	source, err := platform.OpenReadOnly(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}
	defer source.Close()
	if err := snap.CheckOpen(source); err != nil {
		return nil, untouched(err)
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
	counter := &countingReader{r: source, size: snap.Size, sink: sink}
	args := append(lameArgs(job), "-", tmpPath)
	cmd := exec.CommandContext(ctx, "lame", args...)
	cmd.Stdin = counter
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if err := ctx.Err(); err != nil {
		return nil, interrupted(err, job.SourcePath)
	}
	if runErr != nil {
		return nil, untouched(lameFailure(runErr, &stderr, false))
	}

	sink.Emit(progress.Event{Phase: "verify", Percent: 85})
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, untouched(err)
	}
	expect := expectForInfo(wavInfo, false)
	if _, err := verifyMP3File(ctx, tmpPath, expect); err != nil {
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

// countingReader tracks how much of the source LAME has pulled, for progress.
// It is read from the exec copying goroutine, which is the only goroutine
// emitting while the main goroutine waits in cmd.Run.
type countingReader struct {
	r     io.Reader
	size  int64
	sink  progress.Sink
	total int64
	last  int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.total += int64(n)
	if c.size > 0 {
		pct := 5 + int(float64(c.total)*85/float64(c.size))
		if pct != c.last {
			c.last = pct
			c.sink.Emit(progress.Event{Phase: "encode", Percent: float64(pct)})
		}
	}
	return n, err
}

// lameFailure maps a nonzero LAME exit to a structured error. midStream is
// true once the source has been partially overwritten (reclaim); the error
// then points at `siphon recover` instead of claiming an untouched source.
func lameFailure(runErr error, stderr *bytes.Buffer, midStream bool) error {
	msg := strings.TrimSpace(stderr.String())
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	if midStream {
		return siphonerr.Wrap(siphonerr.RecoveryAvailable, runErr,
			"lame failed (%v); the source holds a valid MP3 prefix and the journal allows a splice-resume — run `siphon recover` on it%s",
			runErr, stderrSuffix(msg))
	}
	return siphonerr.Wrap(siphonerr.Internal, runErr, "lame failed: %v%s", runErr, stderrSuffix(msg))
}

func stderrSuffix(msg string) string {
	if msg == "" {
		return ""
	}
	return " (" + msg + ")"
}

func stemOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// untouched marks an error as leaving the source byte-identical.
func untouched(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return interrupted(err, "")
	}
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(true)
		return err
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "audio conversion failed").Untouched(true)
}

// interrupted renders the spec §6.9 cancel message for a safe-mode job.
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
