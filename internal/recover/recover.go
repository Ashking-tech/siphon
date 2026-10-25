// Package recover drives crashed-job recovery (spec §4.6.5). It finds the
// journal for a source, proves the inode still matches (accepting that a
// reclaim job may already have been renamed to its target), then dispatches
// to the owning engine's resume path.
package recover

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/engine/relocate"
	"siphon/internal/engine/sliding"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/preflight"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Run recovers the job whose journal sits next to source. It returns the
// result, the journal path, and any error.
func Run(ctx context.Context, source string, sink progress.Sink) (*core.Result, string, error) {
	dir := filepath.Dir(source)
	stem := preflight.Stem(source)

	path, err := journal.FindOne(dir, stem)
	if err != nil {
		return nil, "", err
	}
	store, j, err := journal.Open(path)
	if err != nil {
		return nil, path, err
	}
	defer store.Close()

	// A crash between journal.Create and the first Save leaves an empty
	// journal. It carries no recovery information and no source was touched;
	// drop it and report the source safe.
	if j.Job.Engine == "" && j.Job.Mode == "" && len(j.Segments) == 0 {
		store.Remove()
		return &core.Result{OK: true, SourceUntouched: true,
			Message: "removed an empty journal; source untouched"}, path, nil
	}

	// A relocate crash after the output rename can leave a "finalising"
	// journal whose source has already been emptied or unlinked. The output is
	// complete; there is nothing left to resume, only the journal to drop.
	if formats.EngineKind(j.Job.Engine) == formats.EngineRelocate && j.Phase == "finalising" &&
		j.Job.Target.Path != "" {
		if _, err := os.Stat(j.Job.TempPath); errors.Is(err, os.ErrNotExist) {
			if journal.OutputMatches(j, j.Job.Target.Path) {
				srcSt, srcErr := os.Stat(source)
				if srcErr == nil && srcSt.Size() == 0 {
					var ust unix.Stat_t
					if err := unix.Stat(source, &ust); err != nil {
						return nil, path, siphonerr.Wrap(siphonerr.SourceChanged, err,
							"cannot stat the emptied source")
					}
					if uint64(ust.Dev) != j.Job.Source.Dev || ust.Ino != j.Job.Source.Ino {
						return nil, path, siphonerr.New(siphonerr.SourceChanged,
							"source inode changed after the output rename")
					}
				}
				if errors.Is(srcErr, os.ErrNotExist) || (srcErr == nil && srcSt.Size() == 0) {
					if srcErr == nil {
						if err := os.Remove(source); err != nil && !errors.Is(err, os.ErrNotExist) {
							return nil, path, siphonerr.Wrap(siphonerr.SourceChanged, err,
								"output is at %s but the empty source could not be removed", j.Job.Target.Path)
						}
					}
					store.Remove()
					return &core.Result{OK: true, SourceUntouched: false}, path, nil
				}
				if srcErr != nil {
					return nil, path, siphonerr.Wrap(siphonerr.SourceChanged, srcErr,
						"cannot stat the source after the output rename")
				}
			}
		}
	}

	// A reclaim job renames the source inode to the target as its last
	// destructive step, so the inode may live at either path by now.
	probe := source
	if _, err := os.Stat(source); err != nil && j.Job.Target.Path != "" {
		if _, terr := os.Stat(j.Job.Target.Path); terr == nil {
			probe = j.Job.Target.Path
		}
	}
	if err := journal.VerifySource(j, probe); err != nil {
		return nil, path, err
	}

	switch formats.EngineKind(j.Job.Engine) {
	case formats.EngineSliding:
		res, err := sliding.ResumeReclaim(ctx, store, j, sink)
		return res, path, err
	case formats.EngineRelocate:
		res, err := relocate.Resume(ctx, store, j, sink)
		return res, path, err
	default:
		return nil, path, siphonerr.New(siphonerr.RecoveryAvailable,
			"no recovery driver for engine %q in this build", j.Job.Engine)
	}
}
