// Package recover drives crashed-job recovery (spec §4.6.5). It finds the
// journal for a source, proves the inode still matches (accepting that a
// reclaim job may already have been renamed to its target), then dispatches
// to the owning engine's resume path.
package recover

import (
	"context"
	"os"
	"path/filepath"

	"siphon/internal/core"
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
	default:
		return nil, path, siphonerr.New(siphonerr.RecoveryAvailable,
			"no recovery driver for engine %q in this build", j.Job.Engine)
	}
}
