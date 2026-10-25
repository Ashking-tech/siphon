// Package relocate is the journaled, resumable MKV → fragmented-MP4 engine
// (spec §4.5.4). It reuses the proven stitch core: a cached EBML header plus
// keyframe-aligned byte ranges, tail-first extraction, sparse reverse assembly,
// and timestamp re-anchoring.
package relocate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/media"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
	"siphon/internal/stitch"
)

// Engine is the relocate engine.
type Engine struct{}

// New returns a relocate engine.
func New() *Engine { return &Engine{} }

func (*Engine) Name() string { return string(formats.EngineRelocate) }

// Convert plans, probes, and runs a fresh relocate job.
func (e *Engine) Convert(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	if job.TranscodeAudio {
		return nil, untouched(siphonerr.New(siphonerr.InplaceInfeasible,
			"relocate is remux-only and cannot transcode audio; use --mode safe"), job.SourcePath)
	}
	if job.SegmentDuration <= 0 || job.SegmentTolerance < 0 {
		return nil, untouched(siphonerr.New(siphonerr.Internal, "invalid segment planning parameters"), job.SourcePath)
	}
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}

	sp, err := stitch.BuildPlan(ctx, job.SourcePath, stitch.Options{
		SegmentDuration:  job.SegmentDuration,
		SegmentTolerance: job.SegmentTolerance,
		VerifyFrames:     job.VerifyFrames,
	}, nil)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	if len(sp.Segments) < 3 {
		return nil, untouched(siphonerr.New(siphonerr.InplaceInfeasible,
			"relocate needs at least 3 keyframe-aligned segments; %s yields %d (use --mode safe)",
			job.SourcePath, len(sp.Segments)), job.SourcePath)
	}

	if err := checkRelocateTrackPlan(ctx, job); err != nil {
		return nil, untouched(err, job.SourcePath)
	}

	snap, err := platform.TakeSnapshot(job.SourcePath)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	src, err := platform.OpenSource(job.SourcePath)
	if err != nil {
		return nil, untouched(siphonerr.Wrap(siphonerr.SourceInvalid, err,
			"cannot open %s for relocation", job.SourcePath), job.SourcePath)
	}
	defer src.Close()
	if err := snap.CheckOpen(src); err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	if err := platform.LockFile(src); err != nil {
		return nil, untouched(siphonerr.Wrap(siphonerr.SourceChanged, err,
			"source is locked by another process"), job.SourcePath)
	}

	// The pre-flight stitch probe extracts the first, last, and (by default)
	// two middle segments. It is the last chance to refuse with the source
	// byte-identical.
	if job.SkipProbe {
		sink.Warn("--skip-probe set: the pre-flight stitch probe was skipped")
	} else if err := probePlan(ctx, job, sp, src, sink); err != nil {
		if ctx.Err() != nil {
			return nil, interrupted(ctx.Err(), job.SourcePath, false)
		}
		return nil, untouched(err, job.SourcePath)
	}

	// Capture the source timeline before any truncation. The full gate uses it
	// after the source media has been consumed.
	starts, err := stitch.SourceStarts(ctx, job.SourcePath)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	sourceDuration, err := stitch.SourceDuration(ctx, job.SourcePath)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	sp.SourceDurationNS = sourceDuration
	// Capture the exact per-stream packet counts while the source still has
	// its tail. The full gate compares the output against them after assembly.
	counts, err := stitch.CountPackets(ctx, job.SourcePath)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	counts = stitch.RetainedCounts(counts)

	dir := filepath.Dir(job.SourcePath)
	stem := stemOf(job.SourcePath)
	if removed, err := platform.ReapTemps(dir, stem, time.Hour); err == nil && len(removed) > 0 {
		sink.Warn(fmt.Sprintf("removed %d stale temp file(s) from an earlier run", len(removed)))
	}

	tmp, err := platform.CreateTemp(dir, stem, ".tmp")
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		tmp.Close()
		if cleanup {
			platform.RemoveIfExists(tmpPath)
		}
	}()
	if err := platform.LockFile(tmp); err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	if err := tmp.Truncate(sp.ReserveSize); err != nil {
		return nil, untouched(siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
			"cannot reserve %s for the output", core.HumanBytes(sp.ReserveSize)), job.SourcePath)
	}
	var tmpSt unix.Stat_t
	if err := unix.Fstat(int(tmp.Fd()), &tmpSt); err != nil {
		return nil, untouched(siphonerr.Wrap(siphonerr.Internal, err,
			"cannot stat the output temp"), job.SourcePath)
	}

	store, j, err := journal.Create(dir, stem)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	defer store.Close()
	// From here on the temp and journal are the recovery state. A handled
	// failure leaves both in place and tells the user to run recover.
	cleanup = false

	source, err := journal.NewSource(job.SourcePath)
	if err != nil {
		return nil, untouched(err, job.SourcePath)
	}
	j.Job = journal.Job{
		Engine: string(formats.EngineRelocate),
		Mode:   string(formats.ModeRelocate),
		Source: source,
		Target: journal.Target{Path: job.TargetPath, Format: job.Target.String()},
		Params: journal.Params{
			SegmentDurationNS:  int64(job.SegmentDuration),
			SegmentToleranceNS: int64(job.SegmentTolerance),
			PlanSource:         sp.PlanSource,
			Verify:             job.Verify,
			VerifyFrames:       job.VerifyFrames,
			DropIncompatible:   job.DropIncompatible,
			AllowNonstandard:   job.AllowNonstandard,
			DropMetadata:       job.DropMetadata,
			Force:              job.Force,
		},
		Tools:       job.Tools,
		ReserveSize: sp.ReserveSize,
		TempPath:    tmpPath,
		TempDev:     uint64(tmpSt.Dev),
		TempIno:     tmpSt.Ino,
	}
	j.Segments = make([]journal.Segment, len(sp.Segments))
	for i, s := range sp.Segments {
		j.Segments[i] = journal.Segment{
			I: i, SrcOff: s.SrcOff, SrcEnd: s.SrcEnd,
			FirstPTSNS: s.FirstPTSNS, State: "pending",
		}
	}
	j.Cursors = journal.Cursors{
		TailCursor:    sp.FileSize,
		OutCursor:     sp.ReserveSize,
		SourceMtimeNS: snap.MtimeNS,
	}
	j.ExpectedCounts = counts
	j.CountsExact = true
	j.SourceStarts = starts
	j.SourceDurationNS = sourceDuration
	j.Phase = "prepared"
	if err := store.Save(j); err != nil {
		return nil, fail(err, false, job.SourcePath)
	}

	sess := stitch.NewSession(sp, src, sp.ReserveSize)
	sess.SourceStarts = starts
	if err := runSegments(ctx, job, j, store, sess, src, tmp, sink); err != nil {
		return nil, err
	}
	return finish(ctx, job, plan, j, store, sess, tmp, tmpPath, src, sink)
}

// Resume continues a journaled relocate job. The caller has opened the journal
// and verified the source identity; Resume reconciles the truncate window,
// re-plans the remaining range, and finishes the assembly.
func Resume(ctx context.Context, store *journal.Store, j *journal.Journal, sink progress.Sink) (*core.Result, error) {
	if j.Job.Mode != string(formats.ModeRelocate) {
		return nil, siphonerr.New(siphonerr.StaleJournal, "journal mode %q is not relocate", j.Job.Mode)
	}
	job := &core.Job{
		SourcePath:       j.Job.Source.Path,
		TargetPath:       j.Job.Target.Path,
		Source:           formats.MKV,
		Target:           formats.MP4,
		Mode:             formats.ModeRelocate,
		Engine:           formats.EngineRelocate,
		Force:            j.Job.Params.Force,
		DropIncompatible: j.Job.Params.DropIncompatible,
		AllowNonstandard: j.Job.Params.AllowNonstandard,
		DropMetadata:     j.Job.Params.DropMetadata,
		SegmentDuration:  time.Duration(j.Job.Params.SegmentDurationNS),
		SegmentTolerance: time.Duration(j.Job.Params.SegmentToleranceNS),
		Verify:           j.Job.Params.Verify,
		VerifyFrames:     j.Job.Params.VerifyFrames,
		Tools:            j.Job.Tools,
	}

	if err := journal.VerifySource(j, job.SourcePath); err != nil {
		return nil, err
	}
	src, err := platform.OpenSource(job.SourcePath)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot open %s", job.SourcePath)
	}
	defer src.Close()
	if err := platform.LockFile(src); err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceChanged, err, "source is locked by another process")
	}
	if err := checkOpenSource(j, src); err != nil {
		return nil, err
	}
	if j.Cursors.Truncating {
		if err := reconcileTruncate(j, store, src); err != nil {
			return nil, err
		}
	}

	tmpPath := j.Job.TempPath
	if tmpPath == "" {
		return nil, siphonerr.New(siphonerr.StaleJournal, "journal has no output temp path")
	}

	// A crash after renameat2 but before the journal unlink leaves a
	// "finalising" journal with no temp and a complete target. Prove the
	// target is the inode this job created, finish the source cleanup, and
	// drop the journal; there is nothing left to assemble.
	if j.Phase == "finalising" {
		if _, err := os.Stat(tmpPath); errors.Is(err, os.ErrNotExist) {
			if !journal.OutputMatches(j, job.TargetPath) {
				return nil, siphonerr.New(siphonerr.StaleJournal,
					"the output temp is gone and %s is not this job's output", job.TargetPath)
			}
			if err := removeSource(job, src); err != nil {
				return nil, err
			}
			store.Remove()
			return &core.Result{OK: true, SourceUntouched: false}, nil
		}
	}
	if !journal.OutputMatches(j, tmpPath) {
		return nil, siphonerr.New(siphonerr.StaleJournal,
			"the output temp %s is not the inode this job created", tmpPath)
	}

	tmp, err := os.OpenFile(tmpPath, os.O_RDWR, 0)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.StaleJournal, err,
			"the journal's output temp %s is missing", tmpPath)
	}
	defer tmp.Close()
	if err := platform.LockFile(tmp); err != nil {
		return nil, siphonerr.Wrap(siphonerr.StaleJournal, err,
			"the journal's output temp is owned by another process")
	}
	if st, err := tmp.Stat(); err != nil {
		return nil, err
	} else if st.Size() != j.Job.ReserveSize {
		return nil, siphonerr.New(siphonerr.StaleJournal,
			"output temp is %d bytes, journal reserved %d", st.Size(), j.Job.ReserveSize)
	}

	// Segments are completed tail-first, so the done suffix is the run of
	// highest-index segments whose state is done.
	doneFrom := len(j.Segments)
	for i := len(j.Segments) - 1; i >= 0; i-- {
		if j.Segments[i].State != "done" {
			break
		}
		doneFrom = i
	}
	for i := 0; i < doneFrom; i++ {
		if j.Segments[i].State != "pending" {
			return nil, siphonerr.New(siphonerr.StaleJournal,
				"journal segment %d is not pending before the done suffix", i)
		}
	}
	doneCount := len(j.Segments) - doneFrom
	if doneCount == 0 && len(j.Segments) == 0 {
		return nil, siphonerr.New(siphonerr.StaleJournal, "journal has no segments")
	}

	var initBytes []byte
	if doneCount > 0 {
		initBytes, err = journalInit(j)
		if err != nil {
			return nil, err
		}
	}

	sp, pending, err := resumePlan(ctx, job, j, doneFrom, doneCount)
	if err != nil {
		return nil, err
	}

	// The re-plan sees a source that ends at tail_cursor, so its last segment
	// has no "next" keyframe and would not append the cached tail prefix. The
	// done suffix's cluster prefix (the leading B-frames before its keyframe)
	// was truncated, so reattach the journal's cached bytes to that segment.
	if pending > 0 && doneCount > 0 {
		old := j.Segments[doneFrom-1]
		if old.TailTo > old.TailFrom {
			if int64(len(old.TailPrefix)) != old.TailTo-old.TailFrom {
				return nil, siphonerr.New(siphonerr.StaleJournal,
					"journal tail prefix is %d bytes, expected %d", len(old.TailPrefix), old.TailTo-old.TailFrom)
			}
			cur := &sp.Segments[pending-1]
			cur.TailFrom, cur.TailTo = old.TailFrom, old.TailTo
			cur.TailData = old.TailPrefix
			cur.TailCached = true
			cur.Pieces = append(cur.Pieces, stitch.Piece{
				Ranges: [][2]int64{{old.TailFrom, old.TailTo}},
				Wrap:   true,
			})
		}
	}

	// Rewrite the journal to the combined plan so a second crash has the same
	// pending/done split.
	if pending > 0 {
		segs := make([]journal.Segment, len(sp.Segments))
		for i := 0; i < pending; i++ {
			s := sp.Segments[i]
			segs[i] = journal.Segment{
				I: i, SrcOff: s.SrcOff, SrcEnd: s.SrcEnd,
				FirstPTSNS: s.FirstPTSNS, State: "pending",
				TailFrom: s.TailFrom, TailTo: s.TailTo, TailPrefix: s.TailData,
			}
		}
		for x := 0; x < doneCount; x++ {
			old := j.Segments[doneFrom+x]
			old.I = pending + x
			segs[pending+x] = old
		}
		j.Segments = segs
		j.Phase = "encoding"
		if err := store.Save(j); err != nil {
			return nil, fail(err, true, job.SourcePath)
		}
	}

	// Journals from before CountsExact lack the original packet counts. Use
	// the planner estimate with the full gate's ±1% tolerance rather than
	// probing a source whose media tail is already gone.
	if !j.CountsExact && len(j.ExpectedCounts) == 0 {
		if est := stitch.EstimateCounts(sp); est != nil {
			j.ExpectedCounts = est
		}
	}

	sess := stitch.NewSession(sp, src, j.Cursors.OutCursor)
	sess.Init = initBytes
	sess.SourceStarts = j.SourceStarts
	for x := 0; x < doneCount; x++ {
		s := j.Segments[pending+x]
		if s.OutOff == nil || s.OutLen == nil {
			return nil, siphonerr.New(siphonerr.StaleJournal,
				"done segment %d has no output range", s.I)
		}
		sess.SetDone(s.I, *s.OutOff, *s.OutLen, s.Moofs, anchorsFromJournal(s.Anchors))
	}

	if pending > 0 {
		if err := runSegments(ctx, job, j, store, sess, src, tmp, sink); err != nil {
			return nil, err
		}
	}
	return finish(ctx, job, nil, j, store, sess, tmp, tmpPath, src, sink)
}

// resumePlan rebuilds the remaining [first cluster, tail_cursor) plan from the
// truncated source and appends the journal's done suffix. If every segment is
// done, it returns a minimal plan: the init and timings are already durable.
func resumePlan(ctx context.Context, job *core.Job, j *journal.Journal, doneFrom, doneCount int) (*stitch.Plan, int, error) {
	if doneCount == len(j.Segments) {
		// Every segment is durable; the plan only needs placeholder entries so
		// the session's fragment/timing arrays line up with the journal. The
		// finish path skips extraction and uses the stored init and anchors.
		p := &stitch.Plan{Path: job.SourcePath, ReserveSize: j.Job.ReserveSize,
			SourceDurationNS: j.SourceDurationNS}
		p.Segments = make([]stitch.Segment, len(j.Segments))
		for i, s := range j.Segments {
			p.Segments[i] = stitch.Segment{Index: i, SrcOff: s.SrcOff, SrcEnd: s.SrcEnd}
		}
		return p, 0, nil
	}
	endNS := tailPlanEnd(j, doneFrom, doneCount)
	if doneCount > 0 && endNS <= 0 {
		return nil, 0, siphonerr.New(siphonerr.StaleJournal,
			"done suffix has no first keyframe time; cannot bound the remaining plan")
	}
	np, err := stitch.BuildPlan(ctx, job.SourcePath, stitch.Options{
		SegmentDuration:  time.Duration(j.Job.Params.SegmentDurationNS),
		SegmentTolerance: time.Duration(j.Job.Params.SegmentToleranceNS),
		VerifyFrames:     j.Job.Params.VerifyFrames,
		EndNS:            endNS,
	}, nil)
	if err != nil {
		return nil, 0, err
	}
	np.SourceDurationNS = j.SourceDurationNS
	if len(np.Segments) == 0 {
		return nil, 0, siphonerr.New(siphonerr.StaleJournal, "the remaining source has no segments")
	}
	if got := np.Segments[len(np.Segments)-1].SrcEnd; got != j.Cursors.TailCursor {
		return nil, 0, siphonerr.New(siphonerr.StaleJournal,
			"re-plan ends at %d, journal tail cursor is %d", got, j.Cursors.TailCursor)
	}
	np.ReserveSize = j.Job.ReserveSize
	np.ExtractBytes = 0

	pending := len(np.Segments)
	combined := *np
	combined.Segments = append([]stitch.Segment(nil), np.Segments...)
	for x := 0; x < doneCount; x++ {
		old := j.Segments[doneFrom+x]
		combined.Segments = append(combined.Segments, stitch.Segment{
			Index:  len(combined.Segments),
			SrcOff: old.SrcOff,
			SrcEnd: old.SrcEnd,
		})
	}
	return &combined, pending, nil
}

// tailPlanEnd is the source time at the journal's tail cursor: the first
// keyframe of the done suffix. Boundary selection stops there so recovery does
// not ask for a segment whose cluster has already been truncated.
func tailPlanEnd(j *journal.Journal, doneFrom, doneCount int) int64 {
	if doneCount == 0 || doneFrom < 0 || doneFrom >= len(j.Segments) {
		return 0
	}
	return j.Segments[doneFrom].FirstPTSNS
}

// checkRelocateTrackPlan refuses sources whose retained video/audio set would
// require dropping a stream. Relocate extracts with a fixed `-map 0:v -map
// 0:a?`, so it cannot honor --drop-incompatible for A/V; safe mode can.
func checkRelocateTrackPlan(ctx context.Context, job *core.Job) error {
	p, err := media.ProbeFile(ctx, job.SourcePath)
	if err != nil {
		return err
	}
	tp, err := media.BuildTrackPlan(p, media.TrackOptions{
		DropIncompatible: job.DropIncompatible,
		AllowNonstandard: job.AllowNonstandard,
		Relocate:         true,
	})
	if err != nil {
		return err
	}
	for _, d := range tp.Dropped {
		if strings.HasPrefix(d, "video:") || strings.HasPrefix(d, "audio:") {
			return siphonerr.New(siphonerr.InplaceInfeasible,
				"relocate cannot drop %s during segmented extraction; use --mode safe", d)
		}
	}
	return nil
}

// probePlan extracts and sampled-decodes the first, last, and up to two middle
// segments. It also checks that the init segment carries the source's user
// tags, so a metadata refusal happens before the first truncation.
func probePlan(ctx context.Context, job *core.Job, sp *stitch.Plan, src *os.File, sink progress.Sink) error {
	n := len(sp.Segments)
	k := job.ProbeSegments
	if k < 2 {
		k = 2
	}
	if k > n {
		k = n
	}
	idx := map[int]bool{0: true, n - 1: true}
	for j := 1; j < k-1; j++ {
		idx[j*(n-1)/(k-1)] = true
	}
	order := make([]int, 0, len(idx))
	for i := range idx {
		order = append(order, i)
	}
	sort.Ints(order)

	ps := stitch.NewSession(sp, src, sp.ReserveSize)
	var initBytes []byte
	for _, i := range order {
		sink.Emit(progress.Event{Phase: "probe", Percent: 2,
			Message: fmt.Sprintf("segment %d/%d", i+1, n)})
		res, err := ps.Prepare(ctx, i, job.VerifyFrames)
		if err != nil {
			code := siphonerr.StitchFailed
			if siphonerr.IsCode(err, siphonerr.SegmentPlan) {
				code = siphonerr.SegmentPlan
			}
			return siphonerr.Wrap(code, err,
				"pre-flight stitch probe failed at segment %d", i)
		}
		if i == n-1 {
			initBytes = res.Init
		}
	}
	if len(initBytes) == 0 {
		return siphonerr.New(siphonerr.StitchFailed, "the pre-flight probe produced no init segment")
	}
	if err := checkInitTags(ctx, job, initBytes, sink); err != nil {
		return err
	}
	return nil
}

// checkInitTags probes the MP4 init in a tiny temp and compares its user tags
// with the source. It is the relocate metadata gate; the source is still intact.
func checkInitTags(ctx context.Context, job *core.Job, initBytes []byte, sink progress.Sink) error {
	dir := filepath.Dir(job.SourcePath)
	f, err := platform.CreateTemp(dir, stemOf(job.SourcePath), ".tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() {
		f.Close()
		platform.RemoveIfExists(name)
	}()
	if _, err := f.Write(initBytes); err != nil {
		return siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot write the init probe")
	}
	if err := f.Sync(); err != nil {
		return siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot flush the init probe")
	}

	src, err := media.ProbeFile(ctx, job.SourcePath)
	if err != nil {
		return err
	}
	out, err := media.ProbeFile(ctx, name)
	if err != nil {
		return siphonerr.Wrap(siphonerr.MetadataDrop, err, "cannot probe the MP4 init segment")
	}
	tp, err := media.BuildTrackPlan(src, media.TrackOptions{
		DropIncompatible: job.DropIncompatible,
		AllowNonstandard: job.AllowNonstandard,
		Relocate:         true,
	})
	if err != nil {
		return err
	}
	outToSrc := make([]int, 0, len(tp.Tracks))
	for _, t := range tp.Tracks {
		outToSrc = append(outToSrc, t.Index)
	}
	missing := media.MissingTags(src, out, outToSrc)
	if len(missing) == 0 {
		return nil
	}
	if !job.DropMetadata {
		return siphonerr.New(siphonerr.MetadataDrop,
			"the MP4 output cannot carry these tags: %s; pass --drop-metadata to accept this",
			strings.Join(missing, ", "))
	}
	sink.Warn(fmt.Sprintf("--drop-metadata set: tags not carried: %s", strings.Join(missing, ", ")))
	return nil
}

// runSegments processes the pending segments tail-first, checkpointing before
// and after each source truncation.
func runSegments(ctx context.Context, job *core.Job, j *journal.Journal, store *journal.Store,
	sess *stitch.Session, src *os.File, tmp *os.File, sink progress.Sink) error {
	// A resumed journal may already have truncated the source even though this
	// process has not truncated anything yet. Keep source_untouched accurate
	// for failures before the next ftruncate.
	truncated := false
	for _, s := range j.Segments {
		if s.State == "done" || s.State == "truncating" {
			truncated = true
			break
		}
	}
	for i := len(j.Segments) - 1; i >= 0; i-- {
		if j.Segments[i].State != "pending" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return interrupted(err, job.SourcePath, truncated)
		}
		seg := j.Segments[i]
		percent := float64(10 + 75*(len(j.Segments)-i)/len(j.Segments))
		sink.Emit(progress.Event{Phase: "encode", Percent: percent,
			Message: fmt.Sprintf("%s  segment %d/%d", filepath.Base(job.SourcePath), i+1, len(j.Segments))})

		res, err := sess.Prepare(ctx, i, job.VerifyFrames)
		if err != nil {
			if ctx.Err() != nil {
				return interrupted(ctx.Err(), job.SourcePath, truncated)
			}
			return fail(err, truncated, job.SourcePath)
		}
		if i == len(j.Segments)-1 {
			if err := setInit(j, res.Init); err != nil {
				return fail(err, truncated, job.SourcePath)
			}
		}
		// Read the previous segment's tail prefix before this segment's
		// truncation destroys it. The checkpoint below makes it durable.
		if err := sess.CacheTail(i); err != nil {
			return fail(err, truncated, job.SourcePath)
		}
		outOff, err := sess.Place(tmp, res.Body)
		if err != nil {
			return fail(err, truncated, job.SourcePath)
		}
		if err := platform.Fdatasync(tmp); err != nil {
			return fail(siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
				"cannot flush the output fragment"), truncated, job.SourcePath)
		}

		bodyLen := int64(len(res.Body))
		anchors := anchorsToJournal(res.Anchors)
		truncateFrom := j.Cursors.TailCursor
		if err := store.Checkpoint(j, func(j *journal.Journal) {
			s := &j.Segments[i]
			s.State = "truncating"
			s.OutOff = &outOff
			s.OutLen = &bodyLen
			s.Moofs = res.Info.Moofs
			s.Anchors = anchors
			if i > 0 {
				p := &sess.Plan.Segments[i-1]
				prev := &j.Segments[i-1]
				prev.TailFrom, prev.TailTo = p.TailFrom, p.TailTo
				prev.TailPrefix = p.TailData
			}
			j.Cursors.OutCursor = outOff
			j.Cursors.TruncateFrom = truncateFrom
			j.Cursors.TailCursor = seg.SrcOff
			j.Cursors.Truncating = true
		}); err != nil {
			return fail(err, truncated, job.SourcePath)
		}

		if err := src.Truncate(seg.SrcOff); err != nil {
			return fail(siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
				"cannot truncate the source to %d", seg.SrcOff), truncated, job.SourcePath)
		}
		truncated = true
		if err := platform.Fdatasync(src); err != nil {
			return fail(siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
				"cannot flush the truncated source"), true, job.SourcePath)
		}
		var st unix.Stat_t
		if err := unix.Fstat(int(src.Fd()), &st); err != nil {
			return fail(siphonerr.Wrap(siphonerr.SourceChanged, err,
				"cannot stat the truncated source"), true, job.SourcePath)
		}
		if err := store.Checkpoint(j, func(j *journal.Journal) {
			j.Segments[i].State = "done"
			j.Cursors.Truncating = false
			j.Cursors.TruncateFrom = 0
			j.Cursors.TailCursor = seg.SrcOff
			j.Cursors.SourceMtimeNS = st.Mtim.Nano()
			j.Cursors.OutCursor = outOff
		}); err != nil {
			return fail(err, true, job.SourcePath)
		}
	}
	return nil
}

// finish writes the init and free box, patches timestamps, verifies, transfers
// metadata, and atomically renames the output into place.
func finish(ctx context.Context, job *core.Job, plan *core.Plan, j *journal.Journal,
	store *journal.Store, sess *stitch.Session, tmp *os.File, tmpPath string,
	src *os.File, sink progress.Sink) (*core.Result, error) {
	sink.Emit(progress.Event{Phase: "verify", Percent: 90})
	var pad int64
	var err error
	alreadyAssembled := j.Phase == "verifying" || j.Phase == "finalising"
	if alreadyAssembled {
		// The init/free/patch pass was checkpointed before the crash. Re-running
		// it would write a second init (the journal cursor already points at the
		// init start), so reuse the durable result.
		pad = j.PadNS
	} else {
		pad, err = sess.Finish(tmp)
		if err != nil {
			return nil, fail(err, true, job.SourcePath)
		}
		if err := platform.Fdatasync(tmp); err != nil {
			return nil, fail(siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
				"cannot flush the assembled output"), true, job.SourcePath)
		}
		if err := store.Checkpoint(j, func(j *journal.Journal) {
			j.Phase = "verifying"
			if j.Init != nil {
				j.Init.OutOff = sess.OutCursor
			}
			j.Cursors.OutCursor = sess.OutCursor
			j.PadNS = pad
		}); err != nil {
			return nil, fail(err, true, job.SourcePath)
		}
	}
	if err := platform.Fdatasync(tmp); err != nil {
		return nil, fail(siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
			"cannot flush the assembled output"), true, job.SourcePath)
	}

	sink.Emit(progress.Event{Phase: "verify", Percent: 94})
	sourceCounts := j.ExpectedCounts
	tolerant := !j.CountsExact
	checks, err := sess.Verify(ctx, tmpPath, pad, sourceCounts, tolerant, job.Verify == "full")
	if err != nil {
		if ctx.Err() != nil {
			return nil, interrupted(ctx.Err(), job.SourcePath, true)
		}
		return nil, fail(err, true, job.SourcePath)
	}
	for _, c := range checks {
		if strings.HasPrefix(c.Name, "full decode") {
			sink.Emit(progress.Event{Phase: "verify", Percent: 95, Message: c.Detail})
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, interrupted(err, job.SourcePath, true)
	}

	if err := store.Checkpoint(j, func(j *journal.Journal) { j.Phase = "finalising" }); err != nil {
		return nil, fail(err, true, job.SourcePath)
	}
	metaWarnings, err := platform.CopyMetadata(job.SourcePath, tmpPath)
	if err != nil {
		return nil, fail(err, true, job.SourcePath)
	}
	for _, w := range metaWarnings {
		sink.Warn(w)
	}
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, fail(err, true, job.SourcePath)
	}

	sink.Emit(progress.Event{Phase: "finalise", Percent: 98})
	if job.Force {
		err = platform.RenameReplace(tmpPath, job.TargetPath)
	} else {
		err = platform.RenameNoReplace(tmpPath, job.TargetPath)
	}
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil, fail(siphonerr.New(siphonerr.TargetExists,
				"%s appeared while converting (use --force to replace it)", job.TargetPath), true, job.SourcePath)
		}
		return nil, fail(siphonerr.Wrap(siphonerr.Internal, err, "cannot finalise the output"), true, job.SourcePath)
	}
	// The output is final. Empty and unlink the source, flush the directory,
	// then drop the journal (spec §4.5.4.3 step 15).
	if err := removeSource(job, src); err != nil {
		return nil, err
	}
	store.Remove()
	return &core.Result{OK: true, Plan: plan, SourceUntouched: false}, nil
}

// removeSource releases the source's blocks and unlinks it after the output is
// final. The source fd is still open, so truncation works even if the path has
// already gone.
func removeSource(job *core.Job, src *os.File) error {
	if err := src.Truncate(0); err != nil {
		return finalErr(err, "output is at %s but the source could not be emptied", job.TargetPath)
	}
	if err := platform.Fdatasync(src); err != nil {
		return finalErr(err, "output is at %s but the source could not be flushed", job.TargetPath)
	}
	if err := os.Remove(job.SourcePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return finalErr(err, "output is at %s but the source could not be removed", job.TargetPath)
	}
	if err := platform.FsyncDir(filepath.Dir(job.SourcePath)); err != nil {
		return finalErr(err, "output is at %s but the directory could not be flushed", job.TargetPath)
	}
	return nil
}

// finalErr reports a post-rename failure: the output is valid and the source
// state is final, so no recovery is needed.
func finalErr(err error, format string, args ...any) error {
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(false)
		return se
	}
	return siphonerr.Wrap(siphonerr.Internal, err, format, args...).Untouched(false)
}

// reconcileTruncate closes the window between the durable fragment checkpoint
// and the source ftruncate. It is called after journal.VerifySource accepted
// the source at either end of that window.
func reconcileTruncate(j *journal.Journal, store *journal.Store, src *os.File) error {
	var seg *journal.Segment
	for i := range j.Segments {
		if j.Segments[i].State == "truncating" {
			seg = &j.Segments[i]
			break
		}
	}
	if seg == nil {
		return siphonerr.New(siphonerr.StaleJournal, "journal is truncating but has no truncating segment")
	}
	var st unix.Stat_t
	if err := unix.Fstat(int(src.Fd()), &st); err != nil {
		return siphonerr.Wrap(siphonerr.SourceChanged, err, "cannot stat the source")
	}
	switch st.Size {
	case j.Cursors.TailCursor:
		// ftruncate completed before the crash; only the mtime checkpoint is
		// missing. Flush the truncation again so it survives another crash.
		if err := platform.Fdatasync(src); err != nil {
			return siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot flush the recovered truncate")
		}
		if err := unix.Fstat(int(src.Fd()), &st); err != nil {
			return siphonerr.Wrap(siphonerr.SourceChanged, err, "cannot stat the truncated source")
		}
	default:
		pre := j.Cursors.TruncateFrom
		if pre <= 0 {
			pre = seg.SrcEnd
		}
		if st.Size != pre {
			return siphonerr.New(siphonerr.SourceChanged,
				"source size %d is neither tail cursor %d nor the pending truncate start %d",
				st.Size, j.Cursors.TailCursor, pre)
		}
		if err := src.Truncate(j.Cursors.TailCursor); err != nil {
			return siphonerr.Wrap(siphonerr.NoSpaceRuntime, err,
				"cannot finish the pending truncate to %d", j.Cursors.TailCursor)
		}
		if err := platform.Fdatasync(src); err != nil {
			return siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "cannot flush the pending truncate")
		}
		if err := unix.Fstat(int(src.Fd()), &st); err != nil {
			return siphonerr.Wrap(siphonerr.SourceChanged, err, "cannot stat the truncated source")
		}
	}
	return store.Checkpoint(j, func(j *journal.Journal) {
		j.Segments[seg.I].State = "done"
		j.Cursors.Truncating = false
		j.Cursors.TruncateFrom = 0
		j.Cursors.SourceMtimeNS = st.Mtim.Nano()
	})
}

// checkOpenSource verifies the open fd's identity against the journal.
func checkOpenSource(j *journal.Journal, src *os.File) error {
	var st unix.Stat_t
	if err := unix.Fstat(int(src.Fd()), &st); err != nil {
		return siphonerr.Wrap(siphonerr.SourceChanged, err, "cannot stat the open source")
	}
	if uint64(st.Dev) != j.Job.Source.Dev || st.Ino != j.Job.Source.Ino {
		return siphonerr.New(siphonerr.SourceChanged, "source inode changed since planning")
	}
	return nil
}

// journalInit returns the durable MP4 init, checking its hash.
func journalInit(j *journal.Journal) ([]byte, error) {
	if j.Init == nil || len(j.Init.Data) == 0 {
		return nil, siphonerr.New(siphonerr.StaleJournal, "journal has no init segment")
	}
	sum := sha256.Sum256(j.Init.Data)
	if j.Init.SHA256 != "" && hex.EncodeToString(sum[:]) != j.Init.SHA256 {
		return nil, siphonerr.New(siphonerr.SourceCorrupt, "journal init segment hash mismatch")
	}
	return j.Init.Data, nil
}

func setInit(j *journal.Journal, data []byte) error {
	if len(data) == 0 {
		return siphonerr.New(siphonerr.StitchFailed, "the last segment produced no init segment")
	}
	sum := sha256.Sum256(data)
	j.Init = &journal.Init{
		Data:   data,
		Len:    int64(len(data)),
		SHA256: hex.EncodeToString(sum[:]),
	}
	return nil
}

func anchorsToJournal(m map[uint32]int64) []journal.Anchor {
	out := make([]journal.Anchor, 0, len(m))
	for track, ns := range m {
		out = append(out, journal.Anchor{Track: track, NS: ns})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Track < out[j].Track })
	return out
}

func anchorsFromJournal(a []journal.Anchor) map[uint32]int64 {
	out := make(map[uint32]int64, len(a))
	for _, x := range a {
		out[x.Track] = x.NS
	}
	return out
}

func stemOf(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// untouched marks an error as leaving the source byte-identical.
func untouched(err error, source string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return interrupted(err, source, false)
	}
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(true)
		return err
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "relocate failed").Untouched(true)
}

// fail marks the source state and tells the user how to resume. The journal
// and temp are left in place by the caller.
func fail(err error, truncated bool, source string) error {
	var se *siphonerr.Error
	if !errors.As(err, &se) {
		se = siphonerr.Wrap(siphonerr.Internal, err, "relocate failed")
	}
	se.Untouched(!truncated)
	if truncated {
		se.Message += fmt.Sprintf("; source is a valid shorter file; run 'siphon recover %s' to finish", source)
	} else {
		se.Message += fmt.Sprintf("; source is safe; run 'siphon recover %s' to finish", source)
	}
	return se
}

// interrupted renders the spec §6.9 relocate cancel message.
func interrupted(err error, source string, truncated bool) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return siphonerr.New(siphonerr.Internal, "timed out").Untouched(!truncated)
	}
	where := source
	if where == "" {
		where = "<file>"
	}
	if truncated {
		return siphonerr.New(siphonerr.Interrupted,
			"Interrupted. Source is a valid shorter file; run 'siphon recover %s' to finish.", where).
			Untouched(false)
	}
	return siphonerr.New(siphonerr.Interrupted,
		"Interrupted. Source safe. Run 'siphon recover %s'.", where).Untouched(true)
}
