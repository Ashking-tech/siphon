// Package atomiccopy implements the image engine from spec §4.5.1. Images are
// always converted through a same-directory temp: the source is never modified
// until the output has been written, fsynced, verified, and given the source's
// metadata. There is no journal because a crash can only leave a stray temp.
package atomiccopy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/imaging"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Engine converts one planned image job.
type Engine struct{}

// New returns the engine (registered by internal/engines).
func New() *Engine { return &Engine{} }

// Name identifies the engine.
func (*Engine) Name() string { return string(formats.EngineAtomicCopy) }

// Convert runs the image pipeline: decode → alpha policy → encode →
// verify → metadata transfer → atomic rename → remove source.
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
	snap, err := takeSnapshot(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}

	// A crash in a previous image conversion can only have left a stray temp.
	// Reap old, unlocked ones; a concurrent conversion's temp is flocked and
	// is left alone.
	if removed, err := platform.ReapTemps(dir, stem, time.Hour); err == nil && len(removed) > 0 {
		sink.Warn(fmt.Sprintf("removed %d stale temp file(s) from an earlier run", len(removed)))
	}

	sink.Emit(progress.Event{Phase: "decode", Percent: 5, Message: filepath.Base(job.SourcePath)})
	img, meta, warnings, err := imaging.Decode(ctx, job.SourcePath, job.Source, dir)
	if err != nil {
		return nil, untouched(err)
	}
	for _, w := range warnings {
		sink.Warn(w)
	}

	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	hadAlpha := imaging.HasTransparency(img)

	if job.Target == formats.JPG && hadAlpha {
		if job.Background == "" {
			return nil, untouched(siphonerr.New(siphonerr.AlphaUnsupported,
				"%s has transparency and JPEG cannot carry it; pass --bg <colour> to choose the background",
				job.SourcePath))
		}
		bg, err := imaging.ParseBackground(job.Background)
		if err != nil {
			return nil, untouched(err)
		}
		img = imaging.Composite(img, bg)
	}
	if err := ctx.Err(); err != nil {
		return nil, interrupted(err)
	}
	if err := checkSnapshot(job.SourcePath, snap); err != nil {
		return nil, untouched(err)
	}

	sink.Emit(progress.Event{Phase: "encode", Percent: 25})
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

	opts := imaging.Options{
		JPEGQuality:  job.Quality,
		LosslessWebP: job.Source == formats.PNG,
		DropMetadata: job.DropMetadata,
	}
	if err := imaging.Encode(ctx, img, job.Target, opts, meta, tmpPath, dir); err != nil {
		return nil, untouched(err)
	}

	sink.Emit(progress.Event{Phase: "verify", Percent: 80})
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, untouched(err)
	}
	info := &imaging.Info{
		Width:    width,
		Height:   height,
		HasAlpha: hadAlpha,
		Metadata: meta,
	}
	if _, err := imaging.Verify(ctx, tmpPath, job.Target, info, meta, job.DropMetadata); err != nil {
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

	if err := checkSnapshot(job.SourcePath, snap); err != nil {
		return nil, untouched(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, interrupted(err)
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

func stemOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

type sourceSnapshot struct {
	dev     uint64
	ino     uint64
	size    int64
	mtimeNS int64
}

func takeSnapshot(path string) (sourceSnapshot, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return sourceSnapshot{}, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot stat %s", path)
	}
	return sourceSnapshot{
		dev:     uint64(st.Dev),
		ino:     st.Ino,
		size:    st.Size,
		mtimeNS: st.Mtim.Nano(),
	}, nil
}

func checkSnapshot(path string, want sourceSnapshot) error {
	got, err := takeSnapshot(path)
	if err != nil {
		return err
	}
	if got != want {
		return siphonerr.New(siphonerr.SourceChanged,
			"%s changed while it was being converted; nothing was replaced", path)
	}
	return nil
}

// untouched marks an error as leaving the source byte-identical. Context
// cancellation keeps its own exit status (130 interrupted, 1 timeout).
func untouched(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return interrupted(err)
	}
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(true)
		return err
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "image conversion failed").Untouched(true)
}

func interrupted(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return siphonerr.Wrap(siphonerr.Internal, err, "timed out").Untouched(true)
	}
	return siphonerr.Wrap(siphonerr.Interrupted, err, "interrupted; the source is untouched").Untouched(true)
}
