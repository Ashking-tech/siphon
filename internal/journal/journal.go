// Package journal implements the schema-2 crash journal described in spec
// §4.6. A journal is the only record that recovery trusts; it is written
// atomically and carries a CRC over its own payload.
package journal

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/platform"
	"siphon/internal/siphonerr"
)

const Schema = 2

// Source identifies the inode a journal belongs to.
type Source struct {
	Path    string `json:"path"`
	Dev     uint64 `json:"dev"`
	Ino     uint64 `json:"ino"`
	Size    int64  `json:"size"`
	MtimeNS int64  `json:"mtime_ns"`
	Mode    uint32 `json:"mode"`
	UID     uint32 `json:"uid"`
	GID     uint32 `json:"gid"`
}

// Target is the planned output.
type Target struct {
	Path   string `json:"path"`
	Format string `json:"format"`
}

// Params records the decisions that affect recovery.
type Params struct {
	SegmentDurationNS  int64  `json:"segment_duration_ns"`
	SegmentToleranceNS int64  `json:"segment_tolerance_ns"`
	PlanSource         string `json:"plan_source,omitempty"`
	Verify             string `json:"verify,omitempty"`
	VerifyFrames       int    `json:"verify_frames,omitempty"`
	DropIncompatible   bool   `json:"drop_incompatible,omitempty"`
	TranscodeAudio     bool   `json:"transcode_audio,omitempty"`
	AllowNonstandard   bool   `json:"allow_nonstandard,omitempty"`
	DropMetadata       bool   `json:"drop_metadata,omitempty"`

	// Audio reclaim: 0 bitrate means LAME VBR -V 2; otherwise CBR kbps.
	// The WAV shape is journaled because reclaim destroys the source's own
	// header, so a splice-resume cannot re-read it. Force records whether the
	// final rename may replace an existing target.
	Bitrate int          `json:"bitrate,omitempty"`
	Force   bool         `json:"force,omitempty"`
	Audio   *AudioParams `json:"audio,omitempty"`
}

// AudioParams is the WAV shape needed to rebuild a synthetic header for a
// splice-resume after reclaim has overwritten the original one.
type AudioParams struct {
	DataOffset    int64 `json:"data_offset"`
	DataEnd       int64 `json:"data_end"`
	SampleRate    int   `json:"sample_rate"`
	Channels      int   `json:"channels"`
	BitsPerSample int   `json:"bits_per_sample"`
	ByteRate      int64 `json:"byte_rate"`
	BlockAlign    int   `json:"block_align"`
}

// Job is the immutable header of a job.
type Job struct {
	Engine      string            `json:"engine"`
	Mode        string            `json:"mode"`
	Source      Source            `json:"source"`
	Target      Target            `json:"target"`
	Params      Params            `json:"params"`
	Tools       map[string]string `json:"tools,omitempty"`
	ReserveSize int64             `json:"reserve_size,omitempty"`
	TempPath    string            `json:"temp_path,omitempty"`
	// TempDev/TempIno identify the inode the engine created as its output
	// temp. rename(2) keeps the inode, so a finalising journal can prove that
	// a target at the destination is its own output and not a pre-existing
	// file that happens to have the reserve size.
	TempDev uint64 `json:"temp_dev,omitempty"`
	TempIno uint64 `json:"temp_ino,omitempty"`
}

// Anchor is one MP4 track's source first-block PTS (ns), used by relocate's
// final timestamp patch pass after a resume.
type Anchor struct {
	Track uint32 `json:"track"`
	NS    int64  `json:"ns"`
}

// Segment is one independently recoverable unit.
type Segment struct {
	I          int      `json:"i"`
	SrcOff     int64    `json:"src_off"`
	SrcEnd     int64    `json:"src_end"`
	FirstPTSNS int64    `json:"first_pts_ns,omitempty"`
	OutOff     *int64   `json:"out_off"`
	OutLen     *int64   `json:"out_len"`
	Packets    int64    `json:"packets,omitempty"`
	Moofs      int      `json:"moofs,omitempty"`
	Anchors    []Anchor `json:"anchors,omitempty"`
	// TailFrom/TailTo/TailPrefix cache the next cluster's prefix before its
	// segment truncation removes it from the source.
	TailFrom   int64  `json:"tail_from,omitempty"`
	TailTo     int64  `json:"tail_to,omitempty"`
	TailPrefix []byte `json:"tail_prefix,omitempty"`
	State      string `json:"state"`
}

// Cursors are the durable positions of a destructive engine. Relocate uses
// the tail/out pair; audio reclaim uses the read/write pair. Truncating marks
// the crash window between the durable fragment checkpoint and the source
// ftruncate: the source may be at either TailCursor or the truncating
// segment's SrcEnd.
type Cursors struct {
	TailCursor    int64 `json:"tail_cursor,omitempty"`
	OutCursor     int64 `json:"out_cursor,omitempty"`
	ReadCursor    int64 `json:"read_cursor,omitempty"`
	WriteCursor   int64 `json:"write_cursor,omitempty"`
	SourceMtimeNS int64 `json:"source_mtime_ns"`
	// TruncateFrom is the source size before the pending ftruncate. The
	// pre-truncate window is not seg.SrcEnd: the source already ended at the
	// next segment's SrcOff, which can be below this segment's SrcEnd.
	TruncateFrom int64 `json:"truncate_from,omitempty"`
	Truncating   bool  `json:"truncating,omitempty"`
}

// Init describes the MP4 initialisation segment, if any. Relocate stores the
// bytes in the journal so a crash after the last source truncation can still
// assemble and verify the output.
type Init struct {
	OutOff int64  `json:"out_off"`
	Len    int64  `json:"len"`
	SHA256 string `json:"sha256"`
	Data   []byte `json:"data,omitempty"`
}

// Journal is the on-disk document.
type Journal struct {
	Schema   int       `json:"schema"`
	Seq      uint64    `json:"seq"`
	CRC32C   string    `json:"crc32c"`
	Job      Job       `json:"job"`
	Segments []Segment `json:"segments,omitempty"`
	Cursors  Cursors   `json:"cursors"`
	Init     *Init     `json:"init,omitempty"`
	// ExpectedCounts is the per-stream packet count captured before the first
	// truncation (exact) or the planner's estimate after a crash that lost the
	// tail. CountsExact says which.
	ExpectedCounts map[string]int `json:"expected_counts,omitempty"`
	CountsExact    bool           `json:"counts_exact,omitempty"`
	// SourceStarts is the first packet PTS per codec type, captured before the
	// first truncation. The full gate compares the output against it after the
	// source's media has been consumed.
	SourceStarts map[string]float64 `json:"source_starts,omitempty"`
	// SourceDurationNS is the source container duration captured before the
	// first truncation, so the full gate can compare the assembled duration
	// after the source's own tail is gone.
	SourceDurationNS int64 `json:"source_duration_ns,omitempty"`
	// PadNS is the global timeline pad applied by the relocate patch pass.
	// It lets a resume after assembly skip the (non-idempotent on disk) init
	// write while still running the same full gate.
	PadNS     int64  `json:"pad_ns,omitempty"`
	Phase     string `json:"phase"`
	CreatedNS int64  `json:"created_ns"`
	UpdatedNS int64  `json:"updated_ns"`
}

var crcTable = crc32.MakeTable(crc32.Castagnoli)

// NewSource snapshots the source inode for a journal.
func NewSource(path string) (Source, error) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return Source{}, err
	}
	return Source{
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

// Store owns a journal file and its lock.
type Store struct {
	path     string
	lockPath string
	lock     *os.File
}

// Create makes a fresh journal in dir for stem, acquires the job lock, and
// writes the initial record atomically.
func Create(dir, stem string) (*Store, *Journal, error) {
	path, err := platform.TempPath(dir, stem, ".journal")
	if err != nil {
		return nil, nil, err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, nil, &os.PathError{Op: "create-journal", Path: path, Err: err}
	}
	unix.Close(fd)

	store, err := openLocked(path)
	if err != nil {
		os.Remove(path)
		return nil, nil, err
	}
	now := time.Now().UnixNano()
	j := &Journal{
		Schema:    Schema,
		Phase:     "prepared",
		CreatedNS: now,
		UpdatedNS: now,
	}
	if err := store.Save(j); err != nil {
		store.Remove()
		return nil, nil, err
	}
	return store, j, nil
}

// Open loads and locks an existing journal.
func Open(path string) (*Store, *Journal, error) {
	store, err := openLocked(path)
	if err != nil {
		return nil, nil, err
	}
	j, err := store.Load()
	if err != nil {
		store.Close()
		return nil, nil, err
	}
	return store, j, nil
}

func openLocked(path string) (*Store, error) {
	lockPath := path + ".lock"
	lf, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, &os.PathError{Op: "open-lock", Path: lockPath, Err: err}
	}
	if err := unix.Flock(int(lf.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lf.Close()
		return nil, siphonerr.Wrap(siphonerr.StaleJournal, err,
			"another Siphon instance holds the journal lock for %s", filepath.Base(path))
	}
	return &Store{path: path, lockPath: lockPath, lock: lf}, nil
}

// Path returns the journal path.
func (s *Store) Path() string { return s.path }

// Load reads, validates, and returns the journal.
func (s *Store) Load() (*Journal, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var j Journal
	if err := json.Unmarshal(b, &j); err != nil {
		return nil, siphonerr.Wrap(siphonerr.StaleJournal, err, "journal is not valid JSON")
	}
	if j.Schema != Schema {
		return nil, siphonerr.New(siphonerr.StaleJournal, "journal schema %d is not supported", j.Schema)
	}
	if j.Seq == 0 {
		return nil, siphonerr.New(siphonerr.StaleJournal, "journal has no sequence number")
	}
	if err := j.verifyCRC(); err != nil {
		return nil, err
	}
	return &j, nil
}

// Save increments the sequence, seals with CRC, and atomically replaces the
// journal: tmp → fsync → rename → dir fsync.
func (s *Store) Save(j *Journal) error {
	j.Schema = Schema
	j.Seq++
	j.UpdatedNS = time.Now().UnixNano()
	if j.CreatedNS == 0 {
		j.CreatedNS = j.UpdatedNS
	}
	if err := j.sealCRC(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return &os.PathError{Op: "write-journal", Path: tmp, Err: err}
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := platform.RenameReplace(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := platform.FsyncDir(filepath.Dir(s.path)); err != nil {
		return err
	}
	return nil
}

// Checkpoint mutates and saves in one call.
func (s *Store) Checkpoint(j *Journal, mutate func(*Journal)) error {
	mutate(j)
	return s.Save(j)
}

// Remove deletes the journal and lock and fsyncs the directory.
func (s *Store) Remove() {
	dir := filepath.Dir(s.path)
	if s.lock != nil {
		s.lock.Close()
		s.lock = nil
	}
	os.Remove(s.path)
	os.Remove(s.path + ".tmp")
	os.Remove(s.lockPath)
	_ = platform.FsyncDir(dir)
}

// Close releases the lock without deleting the journal.
func (s *Store) Close() error {
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

// Find lists journal paths for a source stem in dir.
func Find(dir, stem string) ([]string, error) {
	stem = filepath.Base(stem)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	prefix := ".siphon-" + stem + "-"
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".journal") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out, nil
}

// FindOne returns the single journal for a source, or an error if there are
// zero or multiple candidates.
func FindOne(dir, stem string) (string, error) {
	paths, err := Find(dir, stem)
	if err != nil {
		return "", err
	}
	switch len(paths) {
	case 0:
		return "", siphonerr.New(siphonerr.StaleJournal, "no journal found for %s", stem)
	case 1:
		return paths[0], nil
	default:
		return "", siphonerr.New(siphonerr.StaleJournal,
			"multiple journals found for %s; refusing to guess", stem)
	}
}

func (j *Journal) sealCRC() error {
	j.CRC32C = ""
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	j.CRC32C = fmt.Sprintf("%08x", crc32.Checksum(b, crcTable))
	return nil
}

func (j *Journal) verifyCRC() error {
	want := strings.ToLower(j.CRC32C)
	j.CRC32C = ""
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	got := fmt.Sprintf("%08x", crc32.Checksum(b, crcTable))
	if want == "" || want != got {
		return siphonerr.New(siphonerr.StaleJournal, "journal CRC mismatch (want %s, got %s)", want, got)
	}
	j.CRC32C = got
	return nil
}

// OutputMatches reports whether path is the inode the journal created as its
// output temp, at the reserved size. A successful rename keeps the inode, so
// this is how recovery proves a finalising target is its own output rather
// than a pre-existing file of the same size.
func OutputMatches(j *Journal, path string) bool {
	if j == nil || j.Job.TempDev == 0 || j.Job.TempIno == 0 {
		return false
	}
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return false
	}
	return uint64(st.Dev) == j.Job.TempDev && st.Ino == j.Job.TempIno &&
		st.Size == j.Job.ReserveSize
}

// VerifySource checks the fields that must match for this mode, per spec
// §4.6.3. Destructive modes compare against their durable cursors.
func VerifySource(j *Journal, path string) error {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return siphonerr.Wrap(siphonerr.SourceChanged, err, "source disappeared")
	}
	if uint64(st.Dev) != j.Job.Source.Dev || st.Ino != j.Job.Source.Ino {
		return siphonerr.New(siphonerr.SourceChanged, "source inode changed since planning")
	}

	switch j.Job.Mode {
	case "relocate":
		if j.Cursors.Truncating {
			var seg *Segment
			for i := range j.Segments {
				if j.Segments[i].State == "truncating" {
					seg = &j.Segments[i]
					break
				}
			}
			if seg == nil {
				return siphonerr.New(siphonerr.StaleJournal,
					"journal is marked truncating but has no truncating segment")
			}
			switch st.Size {
			case j.Cursors.TailCursor:
				// Our ftruncate completed but its checkpoint did not. The
				// mtime moved because of our own write, so it is not evidence
				// of an external change; recover reconciles it.
			default:
				pre := j.Cursors.TruncateFrom
				if pre <= 0 {
					// Journals written before TruncateFrom existed only got
					// this window right for the last segment.
					pre = seg.SrcEnd
				}
				if st.Size != pre {
					return siphonerr.New(siphonerr.SourceChanged,
						"source size %d is neither tail cursor %d nor the pending truncate start %d",
						st.Size, j.Cursors.TailCursor, pre)
				}
				// The checkpoint landed but the ftruncate did not. The source
				// must still be exactly what the checkpoint recorded.
				if j.Cursors.SourceMtimeNS != 0 && st.Mtim.Nano() != j.Cursors.SourceMtimeNS {
					return siphonerr.New(siphonerr.SourceChanged,
						"source mtime changed before the pending truncate")
				}
			}
			return nil
		}
		if st.Size != j.Cursors.TailCursor {
			return siphonerr.New(siphonerr.SourceChanged,
				"source size %d does not match tail cursor %d", st.Size, j.Cursors.TailCursor)
		}
		if j.Cursors.SourceMtimeNS != 0 && st.Mtim.Nano() != j.Cursors.SourceMtimeNS {
			return siphonerr.New(siphonerr.SourceChanged,
				"source mtime changed since the last checkpoint")
		}
	case "reclaim":
		// The engine writes into the source, so mtime moves with every
		// checkpoint and cannot be compared. Until the final ftruncate the
		// size must be the original; during finalising it may still be the
		// original (crash before the truncate) or the write cursor (after);
		// once complete it is always the write cursor.
		switch j.Phase {
		case "finalising":
			if st.Size != j.Job.Source.Size && st.Size != j.Cursors.WriteCursor {
				return siphonerr.New(siphonerr.SourceChanged,
					"source size %d is neither the original %d nor the write cursor %d",
					st.Size, j.Job.Source.Size, j.Cursors.WriteCursor)
			}
		case "complete":
			if st.Size != j.Cursors.WriteCursor {
				return siphonerr.New(siphonerr.SourceChanged,
					"source size %d does not match write cursor %d", st.Size, j.Cursors.WriteCursor)
			}
		default:
			if st.Size != j.Job.Source.Size {
				return siphonerr.New(siphonerr.SourceChanged,
					"source size %d does not match the original %d", st.Size, j.Job.Source.Size)
			}
		}
	default:
		if st.Size != j.Job.Source.Size || st.Mtim.Nano() != j.Job.Source.MtimeNS {
			return siphonerr.New(siphonerr.SourceChanged, "source changed since planning")
		}
	}
	return nil
}

// ErrNoJournal is returned by callers that expect a journal and find none.
var ErrNoJournal = errors.New("no journal")
