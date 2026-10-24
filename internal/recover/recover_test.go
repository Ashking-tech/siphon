package recover

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	makeJournal(t, dir, "film", src, filepath.Join(dir, "film.mp4"), "relocate", "prepared")

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

	// Replace the file after the journal was written.
	if err := os.Remove(src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("impostor"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := Run(context.Background(), src, progress.Noop{})
	if !siphonerr.IsCode(err, siphonerr.SourceChanged) {
		t.Fatalf("got %v, want E_SOURCE_CHANGED", err)
	}
}
