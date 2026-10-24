package sliding

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
	"siphon/internal/wav"
)

// Spec §4.4.1 constants.
const (
	checkpointSpan  = 8 << 20 // CHECKPOINT_SPAN
	checkpointEvery = time.Second
	minMargin       = 1 << 20
	maxMP3Frame     = 1441
	pipeChunk       = 128 << 10
)

// convertReclaim is the destructive path: the MP3 is written into the
// source's own head, behind the read cursor, and the source inode is finally
// truncated to the output length and renamed over the target.
func convertReclaim(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	dir := filepath.Dir(job.SourcePath)
	stem := stemOf(job.SourcePath)
	snap, err := platform.TakeSnapshot(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}
	info, err := wav.Parse(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}
	if info.Channels > 2 {
		return nil, untouched(siphonerr.New(siphonerr.FormatUnsupported,
			"%s has %d channels; Siphon converts mono and stereo WAV only",
			job.SourcePath, info.Channels))
	}

	source, err := platform.OpenSource(job.SourcePath)
	if err != nil {
		return nil, untouched(err)
	}
	defer source.Close()
	if err := snap.CheckOpen(source); err != nil {
		return nil, untouched(err)
	}
	// Best-effort advisory lock; the journal lock is the real serialisation.
	_ = platform.LockFile(source)

	store, j, err := journal.Create(dir, stem)
	if err != nil {
		return nil, untouched(err)
	}
	sourceRecord, err := sourceJournalRecord(job.SourcePath, source)
	if err != nil {
		store.Remove()
		return nil, untouched(err)
	}
	j.Job = journal.Job{
		Engine: string(formats.EngineSliding),
		Mode:   string(formats.ModeReclaim),
		Source: sourceRecord,
		Target: journal.Target{Path: job.TargetPath, Format: job.Target.String()},
		Params: journal.Params{
			Bitrate: job.Bitrate,
			Force:   job.Force,
			Audio: &journal.AudioParams{
				DataOffset:    info.DataOffset,
				DataEnd:       info.DataOffset + info.DataSize,
				SampleRate:    info.SampleRate,
				Channels:      info.Channels,
				BitsPerSample: info.BitsPerSample,
				ByteRate:      info.ByteRate,
				BlockAlign:    info.BlockAlign,
			},
		},
		Tools: job.Tools,
	}
	j.Cursors = journal.Cursors{SourceMtimeNS: snap.MtimeNS}
	j.Phase = "prepared"
	if err := store.Save(j); err != nil {
		store.Remove()
		return nil, untouched(err)
	}
	if err := store.Checkpoint(j, func(j *journal.Journal) { j.Phase = "encoding" }); err != nil {
		store.Remove()
		return nil, untouched(err)
	}

	w, err := runReclaim(ctx, store, j, source, info, 0, 0, sink)
	if err != nil {
		return nil, err // journal intact: recovery is available
	}
	if err := finalizeReclaim(ctx, store, j, source, job.SourcePath, job.TargetPath, w, false, sink); err != nil {
		return nil, err
	}
	return &core.Result{OK: true, Plan: plan, SourceUntouched: false}, nil
}

// ResumeReclaim continues a crashed reclaim job from its journal (spec
// §4.6.5). It is called by the recovery driver with the journal lock held.
func ResumeReclaim(ctx context.Context, store *journal.Store, j *journal.Journal, sink progress.Sink) (*core.Result, error) {
	if sink == nil {
		sink = progress.Noop{}
	}
	p := j.Job.Params
	if p.Audio == nil && j.Phase != "complete" {
		return nil, siphonerr.New(siphonerr.StaleJournal,
			"the reclaim journal has no audio parameters; cannot resume")
	}
	source, target := j.Job.Source.Path, j.Job.Target.Path
	if source == "" || target == "" {
		return nil, siphonerr.New(siphonerr.StaleJournal, "the reclaim journal is missing its paths")
	}

	// The inode is at the source path, or already renamed to the target.
	path := source
	dev, ino, size, err := statDevIno(source)
	renamed := false
	if err != nil {
		dev, ino, size, err = statDevIno(target)
		if err != nil {
			return nil, siphonerr.New(siphonerr.SourceChanged,
				"neither %s nor %s exists", source, target)
		}
		path, renamed = target, true
	}
	if dev != j.Job.Source.Dev || ino != j.Job.Source.Ino {
		return nil, siphonerr.New(siphonerr.SourceChanged,
			"%s is no longer the inode this journal belongs to", path)
	}

	switch j.Phase {
	case "complete":
		store.Remove()
		return &core.Result{OK: true, SourceUntouched: false}, nil
	case "verifying", "finalising":
		f, err := platform.OpenSource(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if err := finalizeReclaim(ctx, store, j, f, path, target, j.Cursors.WriteCursor, true, sink); err != nil {
			return nil, err
		}
		return &core.Result{OK: true, SourceUntouched: false}, nil
	}

	// prepared / encoding: re-encode from the last complete frame.
	f, err := platform.OpenSource(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info := infoFromParams(p.Audio)
	writePos, pcmStart := resumePoint(f, info, j.Cursors.WriteCursor)
	if renamed && size != writePos {
		return nil, siphonerr.New(siphonerr.SourceChanged,
			"the renamed output is %d bytes, not the journal's %d", size, writePos)
	}
	sink.Warn("resuming a reclaimed conversion: the splice may leave a ≈26 ms discontinuity")

	w, err := runReclaim(ctx, store, j, f, info, pcmStart, writePos, sink)
	if err != nil {
		return nil, err
	}
	if err := finalizeReclaim(ctx, store, j, f, path, target, w, true, sink); err != nil {
		return nil, err
	}
	return &core.Result{OK: true, SourceUntouched: false}, nil
}

// runReclaim is the §4.4.1 per-chunk protocol: feed the encoder from pread
// positions, write its output behind the read cursor, checkpoint on a byte
// or time span. It returns the final write cursor and leaves the journal at
// phase "verifying" with exact cursors.
func runReclaim(ctx context.Context, store *journal.Store, j *journal.Journal, source *os.File, info *wav.Info, startRead, startWrite int64, sink progress.Sink) (int64, error) {
	feed := feedPlan(info, startRead, startWrite)

	cmd := exec.CommandContext(ctx, "lame", append(lameArgsFor(j.Job.Params.Bitrate), "-", "-")...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return startWrite, prepareFail(store, j, startWrite, err, "cannot open lame stdin")
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return startWrite, prepareFail(store, j, startWrite, err, "cannot open lame stdout")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return startWrite, prepareFail(store, j, startWrite, err, "cannot start lame")
	}

	var (
		readCursor  atomic.Int64
		producerEnd atomic.Bool
	)
	readCursor.Store(feed.start)
	writer := &reclaimWriter{
		f:           source,
		margin:      reclaimMargin(),
		readCursor:  &readCursor,
		producerEnd: &producerEnd,
		writeCursor: startWrite,
		sessionEnd:  feed.end,
	}
	sink.Emit(progress.Event{Phase: "encode", Current: feed.start, Total: feed.end})

	var (
		feedErr error
		outErr  error
		wg      sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stdin.Close()
		feedErr = feedLoop(stdin, source, feed, &readCursor)
		producerEnd.Store(true)
	}()

	chunks := make(chan []byte, 8)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(chunks)
		buf := make([]byte, pipeChunk)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				c := make([]byte, n)
				copy(c, buf[:n])
				chunks <- c
			}
			if err != nil {
				if err != io.EOF && !errors.Is(err, os.ErrClosed) {
					outErr = err
				}
				return
			}
		}
	}()

	w, collectErr := collect(ctx, writer, store, j, chunks, sink)
	if collectErr != nil {
		_ = cmd.Process.Kill()
		drain(chunks)
		wg.Wait()
		return writer.writeCursor, abortReclaim(store, j, writer, collectErr)
	}

	wg.Wait()
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return writer.writeCursor, abortReclaim(store, j, writer, ctx.Err())
		}
		return writer.writeCursor, abortReclaim(store, j, writer, lameFailure(err, &stderr, writer.writeCursor > 0))
	}
	if feedErr != nil {
		return writer.writeCursor, abortReclaim(store, j, writer, reclaimErr(feedErr, j.Job.Source.Path, "cannot read the source"))
	}
	if outErr != nil {
		return writer.writeCursor, abortReclaim(store, j, writer, reclaimErr(outErr, j.Job.Source.Path, "cannot read lame output"))
	}

	// Persist exact cursors before anything reads the prefix back. Even when
	// the context was cancelled during the final flush, the data is complete
	// and durable; recover only has to verify and rename.
	if err := platform.Fdatasync(source); err != nil {
		return w, siphonerr.Wrap(siphonerr.RecoveryAvailable, err,
			"cannot flush the reclaimed output; the journal allows a resume")
	}
	j.Cursors.ReadCursor = readCursor.Load()
	j.Cursors.WriteCursor = w
	j.Phase = "verifying"
	if err := store.Save(j); err != nil {
		return w, siphonerr.Wrap(siphonerr.RecoveryAvailable, err,
			"cannot save the journal checkpoint; the journal allows a resume")
	}
	return w, nil
}

// drain consumes the encoder's output until the pipe goroutine closes the
// channel, so an aborted run can never deadlock on a full channel buffer.
func drain(chunks <-chan []byte) {
	for range chunks {
	}
}

// collect drains the encoder's stdout, applying the forward invariant and
// checkpointing. It is the only goroutine that emits progress.
func collect(ctx context.Context, writer *reclaimWriter, store *journal.Store, j *journal.Journal, chunks <-chan []byte, sink progress.Sink) (int64, error) {
	var pending [][]byte
	ticker := time.NewTicker(checkpointEvery)
	defer ticker.Stop()

	lastCpRead := int64(0)
	lastCp := time.Now()
	dirty := false
	for {
		select {
		case chunk, ok := <-chunks:
			if !ok {
				wrote, err := writer.flush(&pending)
				dirty = dirty || wrote
				if err != nil {
					return 0, err
				}
				if len(pending) > 0 {
					return 0, siphonerr.New(siphonerr.Internal,
						"encoder output exceeds the forward cursor margin; the journal allows a resume")
				}
				return writer.writeCursor, nil
			}
			pending = append(pending, chunk)
			wrote, err := writer.flush(&pending)
			dirty = dirty || wrote
			if err != nil {
				return 0, err
			}
			if writer.readCursor.Load()-lastCpRead >= checkpointSpan {
				if err := writer.checkpoint(store, j); err != nil {
					return 0, err
				}
				lastCpRead, lastCp, dirty = writer.readCursor.Load(), time.Now(), false
			}
		case <-ticker.C:
			wrote, err := writer.flush(&pending)
			dirty = dirty || wrote
			if err != nil {
				return 0, err
			}
			if dirty && time.Since(lastCp) >= checkpointEvery {
				if err := writer.checkpoint(store, j); err != nil {
					return 0, err
				}
				lastCpRead, lastCp, dirty = writer.readCursor.Load(), time.Now(), false
			}
			writer.report(sink)
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// reclaimWriter owns write_cursor and applies the invariant before every
// pwrite. Both cursors are source-file offsets, which is what makes the
// comparison meaningful.
type reclaimWriter struct {
	f           *os.File
	margin      int64
	writeCursor int64
	readCursor  *atomic.Int64
	producerEnd *atomic.Bool
	sessionEnd  int64
}

// flush writes as many pending chunks as the invariant currently permits.
// While the encoder is still being fed, the margin applies; after EOF the
// plain forward invariant is used, so files smaller than the margin can
// still finish (their output simply waits for the end of the input).
func (s *reclaimWriter) flush(pending *[][]byte) (bool, error) {
	wrote := false
	for len(*pending) > 0 {
		chunk := (*pending)[0]
		limit := s.readCursor.Load()
		if !s.producerEnd.Load() {
			limit -= s.margin
		}
		if s.writeCursor+int64(len(chunk)) > limit {
			return wrote, nil
		}
		if _, err := s.f.WriteAt(chunk, s.writeCursor); err != nil {
			return wrote, siphonerr.Wrap(siphonerr.Internal, err,
				"cannot write the reclaimed MP3 into the source")
		}
		s.writeCursor += int64(len(chunk))
		*pending = (*pending)[1:]
		wrote = true
	}
	return wrote, nil
}

func (s *reclaimWriter) checkpoint(store *journal.Store, j *journal.Journal) error {
	if err := platform.Fdatasync(s.f); err != nil {
		return siphonerr.Wrap(siphonerr.Internal, err, "cannot flush the reclaimed output")
	}
	j.Cursors.ReadCursor = s.readCursor.Load()
	j.Cursors.WriteCursor = s.writeCursor
	if err := store.Save(j); err != nil {
		return siphonerr.Wrap(siphonerr.Internal, err, "cannot save the journal checkpoint")
	}
	return nil
}

// report emits coarse progress from the collector goroutine.
func (s *reclaimWriter) report(sink progress.Sink) {
	if s.sessionEnd <= 0 {
		return
	}
	sink.Emit(progress.Event{Phase: "encode", Current: s.readCursor.Load(), Total: s.sessionEnd})
}

// finalizeReclaim runs the gate, truncates, and renames. It is idempotent so
// a crash between any two steps can be recovered by running it again.
func finalizeReclaim(ctx context.Context, store *journal.Store, j *journal.Journal, f *os.File, path, target string, w int64, seam bool, sink progress.Sink) error {
	want := expectForParams(j.Job.Params, seam)
	sink.Emit(progress.Event{Phase: "verify", Percent: 85})
	if _, err := verifyMP3(ctx, f, 0, w, want); err != nil {
		if ctx.Err() != nil {
			return interruptedReclaim(ctx.Err(), j.Job.Source.Path)
		}
		return err
	}
	if err := ctx.Err(); err != nil {
		return interruptedReclaim(err, j.Job.Source.Path)
	}

	// Persist the intent before the destructive steps: from here on the
	// journal's cursors alone describe the output, and a crash before,
	// between, or after the truncate/rename is recoverable by running this
	// function again (spec §4.4.1 step 10).
	j.Phase = "finalising"
	if err := store.Save(j); err != nil {
		return reclaimErr(err, j.Job.Source.Path, "cannot save the finalising checkpoint")
	}

	st, err := f.Stat()
	if err != nil {
		return reclaimErr(err, j.Job.Source.Path, "cannot stat the reclaimed source")
	}
	if st.Size() > w {
		if err := unix.Ftruncate(int(f.Fd()), w); err != nil {
			return reclaimErr(err, j.Job.Source.Path, "cannot truncate the source to the output length")
		}
		if err := platform.Fdatasync(f); err != nil {
			return reclaimErr(err, j.Job.Source.Path, "cannot flush the truncated source")
		}
	}

	if path != target {
		// The path must still name the inode we have been writing through.
		if err := checkIdentity(path, j.Job.Source); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return interruptedReclaim(err, j.Job.Source.Path)
		}
		sink.Emit(progress.Event{Phase: "finalise", Percent: 95})
		if err := renameFinal(path, target, j.Job.Params.Force); err != nil {
			return err
		}
	}

	store.Remove()
	sink.Emit(progress.Event{Phase: "complete", Percent: 100})
	return nil
}

// resumePoint picks the safe splice: the byte after the last complete frame,
// and the PCM position one frame before what that output covers. Backing off
// one frame turns a resume into a ≤26 ms echo instead of a gap of unknown
// size, because the encoder's start delay is not observable from the stream.
func resumePoint(f *os.File, info *wav.Info, journalWrite int64) (writePos, pcmStart int64) {
	scan := scanMP3Frames(f, journalWrite)
	writePos = scan.End
	samples := scan.Samples
	if scan.Complete > 0 {
		samples -= scan.LastSamples
	}
	if samples < 0 {
		samples = 0
	}
	pcmStart = info.DataOffset + samples*int64(info.BlockAlign)
	if end := info.DataOffset + info.DataSize; pcmStart > end {
		pcmStart = end
	}
	if pcmStart < info.DataOffset {
		pcmStart = info.DataOffset
	}
	return writePos, pcmStart
}

// feedSpec is the producer's instructions: an optional synthetic header and
// the source byte range that follows it.
type feedSpec struct {
	header []byte
	start  int64
	end    int64
}

func feedPlan(info *wav.Info, startRead, startWrite int64) feedSpec {
	end := info.DataOffset + info.DataSize
	if startWrite == 0 && startRead < info.DataOffset {
		// Nothing has been overwritten: feed the original WAV bytes.
		return feedSpec{start: startRead, end: end}
	}
	pcmStart := startRead
	if pcmStart < info.DataOffset {
		pcmStart = info.DataOffset
	}
	if pcmStart > end {
		pcmStart = end
	}
	return feedSpec{header: info.Header(end - pcmStart), start: pcmStart, end: end}
}

// feedLoop preads the source and pushes it to the encoder. read_cursor is
// advanced before the bytes are handed over, by definition of "handed".
func feedLoop(stdin io.WriteCloser, source *os.File, feed feedSpec, readCursor *atomic.Int64) error {
	if len(feed.header) > 0 {
		if _, err := stdin.Write(feed.header); err != nil {
			return nil // the encoder died; cmd.Wait reports why
		}
	}
	buf := make([]byte, pipeChunk)
	off := feed.start
	for off < feed.end {
		n := int64(len(buf))
		if feed.end-off < n {
			n = feed.end - off
		}
		m, err := source.ReadAt(buf[:n], off)
		if m > 0 {
			off += int64(m)
			readCursor.Store(off)
			if _, werr := stdin.Write(buf[:m]); werr != nil {
				if err != nil {
					return err
				}
				return nil // the encoder died; cmd.Wait reports why
			}
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// reclaimMargin is spec §4.4.1: max(1 MiB, 4 × frame, 2 × checkpoint span).
// marginOverride is a test seam; production always uses the spec formula.
var marginOverride int64

func reclaimMargin() int64 {
	if marginOverride > 0 {
		return marginOverride
	}
	m := int64(2 * checkpointSpan)
	if four := int64(4 * maxMP3Frame); four > m {
		m = four
	}
	if m < minMargin {
		m = minMargin
	}
	return m
}

func lameArgsFor(bitrate int) []string {
	args := []string{"--silent"}
	if bitrate > 0 {
		return append(args, "-b", strconv.Itoa(bitrate))
	}
	return append(args, "-V", "2")
}

func expectForParams(p journal.Params, seam bool) verifyExpect {
	a := p.Audio
	want := verifyExpect{Seam: seam}
	if a == nil {
		return want
	}
	want.Channels = a.Channels
	want.SampleRate = a.SampleRate
	if a.ByteRate > 0 {
		want.Duration = time.Duration(float64(a.DataEnd-a.DataOffset) / float64(a.ByteRate) * float64(time.Second))
	}
	return want
}

func infoFromParams(a *journal.AudioParams) *wav.Info {
	return &wav.Info{
		Channels:      a.Channels,
		SampleRate:    a.SampleRate,
		BitsPerSample: a.BitsPerSample,
		ByteRate:      a.ByteRate,
		BlockAlign:    a.BlockAlign,
		DataOffset:    a.DataOffset,
		DataSize:      a.DataEnd - a.DataOffset,
	}
}

func sourceJournalRecord(path string, f *os.File) (journal.Source, error) {
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		return journal.Source{}, siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot stat the source")
	}
	return journal.Source{
		Path:    path,
		Dev:     uint64(st.Dev),
		Ino:     st.Ino,
		Size:    st.Size,
		MtimeNS: st.Mtim.Nano(),
		Mode:    st.Mode,
		UID:     st.Uid,
		GID:     st.Gid,
	}, nil
}

func statDevIno(path string) (dev, ino uint64, size int64, err error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return 0, 0, 0, err
	}
	return uint64(st.Dev), st.Ino, st.Size, nil
}

func checkIdentity(path string, want journal.Source) error {
	dev, ino, _, err := statDevIno(path)
	if err != nil {
		return siphonerr.Wrap(siphonerr.SourceChanged, err, "cannot stat %s", path)
	}
	if dev != want.Dev || ino != want.Ino {
		return siphonerr.New(siphonerr.SourceChanged,
			"%s was replaced while it was being converted", path)
	}
	return nil
}

func renameFinal(path, target string, force bool) error {
	var err error
	if force {
		err = platform.RenameReplace(path, target)
	} else {
		err = platform.RenameNoReplace(path, target)
	}
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return siphonerr.New(siphonerr.TargetExists,
				"%s appeared while converting (use --force to replace it)", target)
		}
		return siphonerr.Wrap(siphonerr.Internal, err, "cannot finalise the output")
	}
	return nil
}

// abortReclaim cleans up a failed run: if nothing was written the source is
// untouched and the journal is removed; otherwise the journal stays and the
// error points at recovery.
func abortReclaim(store *journal.Store, j *journal.Journal, writer *reclaimWriter, err error) error {
	if writer.writeCursor == 0 {
		store.Remove()
		switch {
		case errors.Is(err, context.Canceled):
			return interrupted(err, j.Job.Source.Path)
		case errors.Is(err, context.DeadlineExceeded):
			return siphonerr.New(siphonerr.Internal,
				"timed out; the source is untouched").Untouched(true)
		}
		return untouched(err)
	}
	_ = writer.checkpoint(store, j)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return interruptedReclaim(err, j.Job.Source.Path)
	}
	return err
}

// prepareFail handles failures before the pipeline starts (nothing written).
func prepareFail(store *journal.Store, j *journal.Journal, writeCursor int64, err error, what string) error {
	store.Remove()
	return untouched(siphonerr.Wrap(siphonerr.Internal, err, "%s", what))
}

// reclaimErr classifies a runtime failure: the journal holds a resumable
// prefix once writing has started.
func reclaimErr(err error, source, what string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return interruptedReclaim(err, source)
	}
	return siphonerr.Wrap(siphonerr.RecoveryAvailable, err,
		"%s; the source holds a valid MP3 prefix and the journal allows a splice-resume", what)
}

// interruptedReclaim renders the spec §6.9 message for a destructive job: the
// source is a valid shorter file and recovery finishes it. A deadline is not
// a user interrupt, so it keeps the recovery exit code instead of 130.
func interruptedReclaim(err error, source string) error {
	where := source
	if where == "" {
		where = "<file>"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return siphonerr.New(siphonerr.RecoveryAvailable,
			"timed out; the source is a valid shorter file; run 'siphon recover %s' to finish.", where)
	}
	return siphonerr.New(siphonerr.Interrupted,
		"Interrupted. Source is a valid shorter file; run 'siphon recover %s' to finish.", where)
}
