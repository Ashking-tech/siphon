// Package safeio holds the small set of crash-safety primitives shared by the
// v0.2 safe-only engines: lock the source, create a same-directory temp, verify
// the source is unchanged, install atomically, and remove the source last.
package safeio

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Source is an open, advisory-locked source with its identity snapshot.
type Source struct {
	File     *os.File
	Snapshot platform.Snapshot
}

// OpenSource opens a source read-only without following symlinks, takes an
// exclusive advisory lock, and snapshots it. The caller must Close it.
func OpenSource(path string) (*Source, error) {
	f, err := platform.OpenReadOnly(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot open %s", path)
	}
	if err := platform.LockFile(f); err != nil {
		f.Close()
		return nil, siphonerr.Wrap(siphonerr.Permission, err, "cannot lock %s", path)
	}
	snap, err := platform.TakeSnapshot(path)
	if err != nil {
		f.Close()
		return nil, err
	}
	if err := snap.CheckOpen(f); err != nil {
		f.Close()
		return nil, err
	}
	return &Source{File: f, Snapshot: snap}, nil
}

// Close releases the source descriptor and its advisory lock.
func (s *Source) Close() error {
	if s == nil || s.File == nil {
		return nil
	}
	return s.File.Close()
}

// SourcePath returns the path the source was opened from.
func (s *Source) SourcePath() string {
	if s == nil || s.File == nil {
		return ""
	}
	return s.File.Name()
}

// Stem returns the source basename without its final extension.
func Stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Reap removes stale temps from an earlier run and reports the count.
func Reap(dir, stem string, sink progress.Sink) {
	if sink == nil {
		return
	}
	removed, err := platform.ReapTemps(dir, stem, time.Hour)
	if err == nil && len(removed) > 0 {
		sink.Warn("removed " + itoa(len(removed)) + " stale temp file(s) from an earlier run")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// CreateLockedTemp creates a same-directory temp and locks it so the reaper
// leaves it alone while the engine is running.
func CreateLockedTemp(dir, stem, suffix string) (*os.File, error) {
	f, err := platform.CreateTemp(dir, stem, suffix)
	if err != nil {
		return nil, err
	}
	if err := platform.LockFile(f); err != nil {
		path := f.Name()
		f.Close()
		platform.RemoveIfExists(path)
		return nil, err
	}
	return f, nil
}

// Finalize checks the source snapshot one last time, installs tmpPath at
// job.TargetPath, removes the source, and fsyncs the directory. It does not
// delete tmpPath on failure; the engine's deferred cleanup owns that.
func Finalize(ctx context.Context, job *core.Job, plan *core.Plan, snap platform.Snapshot,
	tmpPath string, sink progress.Sink) (*core.Result, error) {
	return finalize(ctx, job, plan, snap, tmpPath, sink, true)
}

// FinalizeQuiet is Finalize without progress events. The PDF/image pairs emit
// only 0 and 100 (spec §14), so the shared finalise/complete events would
// violate that contract.
func FinalizeQuiet(ctx context.Context, job *core.Job, plan *core.Plan, snap platform.Snapshot,
	tmpPath string) (*core.Result, error) {
	return finalize(ctx, job, plan, snap, tmpPath, nil, false)
}

func finalize(ctx context.Context, job *core.Job, plan *core.Plan, snap platform.Snapshot,
	tmpPath string, sink progress.Sink, emit bool) (*core.Result, error) {
	if err := snap.Check(job.SourcePath); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, Interrupted(err, job.SourcePath)
	}

	if emit && sink != nil {
		sink.Emit(progress.Event{Phase: "finalise", Percent: 95})
	}
	var err error
	if job.Force {
		err = platform.RenameReplace(tmpPath, job.TargetPath)
	} else {
		err = platform.RenameNoReplace(tmpPath, job.TargetPath)
	}
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, siphonerr.New(siphonerr.TargetExists,
				"%s appeared while converting (use --force to replace it)", job.TargetPath)
		}
		return nil, siphonerr.Wrap(siphonerr.Internal, err, "cannot finalise the output")
	}

	dir := filepath.Dir(job.SourcePath)
	if err := os.Remove(job.SourcePath); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err,
			"output is at %s but the source could not be removed", job.TargetPath)
	}
	if err := platform.FsyncDir(dir); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err, "cannot fsync %s", dir)
	}
	if emit && sink != nil {
		sink.Emit(progress.Event{Phase: "complete", Percent: 100})
	}
	return &core.Result{OK: true, Plan: plan, SourceUntouched: false}, nil
}

// Untouched marks err as leaving the source byte-identical.
func Untouched(err error, fallback string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Interrupted(err, "")
	}
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(true)
		return err
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "%s", fallback).Untouched(true)
}

// Interrupted maps cancellation and deadline expiry onto the stable CLI exit
// contract, with the source untouched. Safe-mode v0.2 pairs have no journal, so
// there is no recovery claim: only a reapable temp file can remain.
func Interrupted(err error, source string) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return siphonerr.Wrap(siphonerr.Internal, err, "timed out").Untouched(true)
	}
	where := source
	if where == "" {
		where = "<file>"
	}
	return siphonerr.New(siphonerr.Interrupted,
		"interrupted while converting %s; source untouched, temp files are reaped on the next run", where).Untouched(true)
}

// RuntimeFailure classifies a subprocess failure. ENOSPC is its own stable
// runtime code so the CLI and KDE wrapper can report it precisely.
func RuntimeFailure(err error, detail, what string) error {
	if strings.Contains(strings.ToLower(detail), "no space left on device") {
		return siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "%s ran out of disk space", what)
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "%s failed: %s", what, detail)
}
