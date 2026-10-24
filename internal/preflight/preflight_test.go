package preflight

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/siphonerr"
)

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

func imageJob(t *testing.T, srcPath string) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.PNG, formats.JPG)
	if !ok {
		t.Fatal("pair missing")
	}
	return &core.Job{
		SourcePath:      srcPath,
		TargetPath:      TargetPath(srcPath, formats.JPG),
		Source:          formats.PNG,
		Target:          formats.JPG,
		Pair:            pair,
		Mode:            formats.ModeAuto,
		Quality:         90,
		SegmentDuration: 60 * time.Second,
		Verify:          "sampled",
		Timeout:         time.Hour,
		AllowRoot:       os.Geteuid() == 0,
	}
}

func TestPlanImage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	if err := os.WriteFile(src, pngHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := Run(context.Background(), imageJob(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Mode != "safe" || plan.Engine != "atomiccopy" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.FreeNeeded <= 0 || plan.FreeAvailable <= 0 {
		t.Fatalf("space not reported: %+v", plan)
	}
}

func TestRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.png")
	if err := os.WriteFile(real, pngHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, link))
	if !siphonerr.IsCode(err, siphonerr.SourceInvalid) {
		t.Fatalf("got %v, want E_SOURCE_INVALID", err)
	}
}

func TestRefuseExistingTarget(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	dst := filepath.Join(dir, "img.jpg")
	if err := os.WriteFile(src, pngHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("exists"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.TargetExists) {
		t.Fatalf("got %v, want E_TARGET_EXISTS", err)
	}
}

func TestRefuseBadMagic(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	if err := os.WriteFile(src, []byte("not a png at all"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("got %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestRefuseStaleJournal(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	if err := os.WriteFile(src, pngHeader, 0o600); err != nil {
		t.Fatal(err)
	}
	store, _, err := journal.Create(dir, "img")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Remove()

	_, err = Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.StaleJournal) {
		t.Fatalf("got %v, want E_STALE_JOURNAL", err)
	}
}

func TestTargetPath(t *testing.T) {
	if got := TargetPath("/a/film.mkv", formats.MP4); got != "/a/film.mp4" {
		t.Fatalf("got %s", got)
	}
	if got := TargetPath("a.b.png", formats.JPG); got != "a.b.jpg" {
		t.Fatalf("got %s", got)
	}
}
