package recover

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"

	"siphon/internal/journal"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// makeJournal writes a reclaim journal for path and returns the journal path.
func makeJournal(t *testing.T, dir, stem, path, target, engine, phase string) string {
	t.Helper()
	rec, err := journal.NewSource(path)
	if err != nil {
		t.Fatal(err)
	}
	store, j, err := journal.Create(dir, stem)
	if err != nil {
		t.Fatal(err)
	}
	j.Job = journal.Job{
		Engine: engine,
		Mode:   "reclaim",
		Source: rec,
		Target: journal.Target{Path: target, Format: "mp3"},
	}
	j.Cursors = journal.Cursors{SourceMtimeNS: rec.MtimeNS, WriteCursor: rec.Size}
	j.Phase = phase
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	p := store.Path()
	store.Close()
	return p
}

func TestRunFinishesCompleteJob(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tone.wav")
	if err := os.WriteFile(src, []byte("already an mp3, really"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "tone.mp3")
	jp := makeJournal(t, dir, "tone", src, target, "sliding", "complete")

	res, jpath, err := Run(context.Background(), src, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || jpath != jp {
		t.Fatalf("res=%+v path=%s want %s", res, jpath, jp)
	}
	if _, err := os.Stat(jp); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
}

func TestRunUnknownEngine(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(src, []byte("not a real mkv"), 0o600); err != nil {
		t.Fatal(err)
	}
	makeJournal(t, dir, "film", src, filepath.Join(dir, "film.mp4"), "unknown", "prepared")

	_, _, err := Run(context.Background(), src, progress.Noop{})
	if !siphonerr.IsCode(err, siphonerr.RecoveryAvailable) {
		t.Fatalf("got %v, want E_RECOVERY_AVAILABLE", err)
	}
}

// A reclaim job renames the source inode to the target before it removes the
// journal; recovery must probe the target when the source is gone.
func TestRunProbesRenamedTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tone.wav")
	target := filepath.Join(dir, "tone.mp3")
	if err := os.WriteFile(src, []byte("finished mp3 bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	jp := makeJournal(t, dir, "tone", src, target, "sliding", "complete")
	if err := os.Rename(src, target); err != nil {
		t.Fatal(err)
	}

	res, _, err := Run(context.Background(), target, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(jp); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
}

func TestRunRefusesWrongInode(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "tone.wav")
	if err := os.WriteFile(src, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	makeJournal(t, dir, "tone", src, filepath.Join(dir, "tone.mp3"), "sliding", "complete")

	// Replace the file after the journal was written. Create the impostor
	// first so it cannot reuse the source inode on filesystems that recycle
	// inode numbers immediately.
	impostor := filepath.Join(dir, "impostor.wav")
	if err := os.WriteFile(impostor, []byte("impostor"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(impostor, src); err != nil {
		t.Fatal(err)
	}

	_, _, err := Run(context.Background(), src, progress.Noop{})
	if !siphonerr.IsCode(err, siphonerr.SourceChanged) {
		t.Fatalf("got %v, want E_SOURCE_CHANGED", err)
	}
}

// A relocate crash after rename can leave a finalising journal whose source is
// empty or gone. Recovery must accept only the exact output inode it created.
func TestRunRelocateFinalisingAfterRename(t *testing.T) {
	for _, sourceGone := range []bool{false, true} {
		name := "empty source"
		if sourceGone {
			name = "unlinked source"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "film.mkv")
			if err := os.WriteFile(src, make([]byte, 757), 0o600); err != nil {
				t.Fatal(err)
			}
			rec, err := journal.NewSource(src)
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(dir, "film.mp4")
			if err := os.WriteFile(target, make([]byte, 1000), 0o600); err != nil {
				t.Fatal(err)
			}
			var tst unix.Stat_t
			if err := unix.Stat(target, &tst); err != nil {
				t.Fatal(err)
			}
			store, j, err := journal.Create(dir, "film")
			if err != nil {
				t.Fatal(err)
			}
			j.Job = journal.Job{
				Engine:      "relocate",
				Mode:        "relocate",
				Source:      rec,
				Target:      journal.Target{Path: target, Format: "mp4"},
				ReserveSize: 1000,
				TempPath:    filepath.Join(dir, ".siphon-film-gone.tmp"),
				TempDev:     uint64(tst.Dev),
				TempIno:     tst.Ino,
			}
			j.Cursors = journal.Cursors{TailCursor: 757, SourceMtimeNS: rec.MtimeNS}
			j.Phase = "finalising"
			if err := store.Save(j); err != nil {
				t.Fatal(err)
			}
			jp := store.Path()
			store.Close()

			if sourceGone {
				if err := os.Remove(src); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Truncate(src, 0); err != nil {
				t.Fatal(err)
			}
			res, _, err := Run(context.Background(), src, progress.Noop{})
			if err != nil {
				t.Fatal(err)
			}
			if res == nil || !res.OK || res.SourceUntouched {
				t.Fatalf("res = %+v", res)
			}
			if _, err := os.Stat(src); !os.IsNotExist(err) {
				t.Fatalf("source still present: %v", err)
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("target missing: %v", err)
			}
			if _, err := os.Stat(jp); !os.IsNotExist(err) {
				t.Fatalf("journal still present: %v", err)
			}
		})
	}
}

func TestRunRelocateFinalisingRefusesForeignTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(src, make([]byte, 757), 0o600); err != nil {
		t.Fatal(err)
	}
	rec, err := journal.NewSource(src)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "film.mp4")
	if err := os.WriteFile(target, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.tmp")
	if err := os.WriteFile(other, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	var ost unix.Stat_t
	if err := unix.Stat(other, &ost); err != nil {
		t.Fatal(err)
	}
	store, j, err := journal.Create(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	j.Job = journal.Job{
		Engine:      "relocate",
		Mode:        "relocate",
		Source:      rec,
		Target:      journal.Target{Path: target, Format: "mp4"},
		ReserveSize: 1000,
		TempPath:    filepath.Join(dir, ".siphon-film-gone.tmp"),
		TempDev:     uint64(ost.Dev),
		TempIno:     ost.Ino,
	}
	j.Cursors = journal.Cursors{TailCursor: 757, SourceMtimeNS: rec.MtimeNS}
	j.Phase = "finalising"
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	store.Close()

	if err := os.Truncate(src, 0); err != nil {
		t.Fatal(err)
	}
	_, _, err = Run(context.Background(), src, progress.Noop{})
	if !siphonerr.IsCode(err, siphonerr.SourceChanged) {
		t.Fatalf("got %v, want E_SOURCE_CHANGED", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("foreign target removed: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source removed: %v", err)
	}
}

func TestRunDropsEmptyJournal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(src, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, _, err := journal.Create(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	jp := store.Path()
	store.Close()

	res, _, err := Run(context.Background(), src, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || !res.SourceUntouched {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source removed: %v", err)
	}
	if _, err := os.Stat(jp); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
}

func TestRunRelocateFinalisingRemovesTruncatedSource(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "film.mkv")
	if err := os.WriteFile(src, make([]byte, 757), 0o600); err != nil {
		t.Fatal(err)
	}
	rec, err := journal.NewSource(src)
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "film.mp4")
	if err := os.WriteFile(target, make([]byte, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	var tst unix.Stat_t
	if err := unix.Stat(target, &tst); err != nil {
		t.Fatal(err)
	}
	store, j, err := journal.Create(dir, "film")
	if err != nil {
		t.Fatal(err)
	}
	j.Job = journal.Job{
		Engine:      "relocate",
		Mode:        "relocate",
		Source:      rec,
		Target:      journal.Target{Path: target, Format: "mp4"},
		ReserveSize: 1000,
		TempPath:    filepath.Join(dir, ".siphon-film-gone.tmp"),
		TempDev:     uint64(tst.Dev),
		TempIno:     tst.Ino,
	}
	j.Cursors = journal.Cursors{TailCursor: 757, SourceMtimeNS: rec.MtimeNS}
	j.Phase = "finalising"
	if err := store.Save(j); err != nil {
		t.Fatal(err)
	}
	jp := store.Path()
	store.Close()

	res, _, err := Run(context.Background(), src, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if res == nil || !res.OK || res.SourceUntouched {
		t.Fatalf("res = %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still present: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("target missing: %v", err)
	}
	if _, err := os.Stat(jp); !os.IsNotExist(err) {
		t.Fatalf("journal still present: %v", err)
	}
}
