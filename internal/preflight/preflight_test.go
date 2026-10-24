package preflight

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/journal"
	"siphon/internal/siphonerr"
)

// testPNG builds a real PNG so the image probe can decode it. With alpha the
// top-left pixel is transparent.
func testPNG(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x == 0 && y == 0 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / maxInt(1, w-1)),
				G: uint8(y * 255 / maxInt(1, h-1)),
				B: 128,
				A: a,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writePNG(t *testing.T, dir, name string, w, h int, alpha bool) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, testPNG(t, w, h, alpha), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

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
	src := writePNG(t, dir, "img.png", 20, 10, false)
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
	if plan.ImageWidth != 20 || plan.ImageHeight != 10 {
		t.Fatalf("image dimensions not reported: %+v", plan)
	}
}

func TestImageEstimateIncludesDecodedSize(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "big.png", 100, 100, false)
	plan, err := Run(context.Background(), imageJob(t, src))
	if err != nil {
		t.Fatal(err)
	}
	// spec §7.1: source + max(2×source, decoded_raw×1.5) + 2 MiB
	want := int64(100*100*4)*3/2 + 2<<20
	if plan.FreeNeeded < want {
		t.Fatalf("free estimate %d is below the decoded-size floor %d", plan.FreeNeeded, want)
	}
}

func TestRefuseAlphaForJPEG(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "alpha.png", 8, 8, true)
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.AlphaUnsupported) {
		t.Fatalf("got %v, want E_ALPHA_UNSUPPORTED", err)
	}
}

func TestAlphaAllowedWithBackground(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "alpha.png", 8, 8, true)
	job := imageJob(t, src)
	job.Background = "white"
	plan, err := Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.HasAlpha {
		t.Fatalf("plan should note the source alpha: %+v", plan)
	}
}

func TestRefuseBadBackground(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "alpha.png", 8, 8, true)
	job := imageJob(t, src)
	job.Background = "chartreuse-ish"
	_, err := Run(context.Background(), job)
	if !siphonerr.IsCode(err, siphonerr.Usage) {
		t.Fatalf("got %v, want E_USAGE", err)
	}
}

func TestRefuseSymlink(t *testing.T) {
	dir := t.TempDir()
	real := writePNG(t, dir, "real.png", 4, 4, false)
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
	src := writePNG(t, dir, "img.png", 4, 4, false)
	dst := filepath.Join(dir, "img.jpg")
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

func TestRefuseTruncatedImage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "img.png")
	data := testPNG(t, 8, 8, false)
	if err := os.WriteFile(src, data[:20], 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Run(context.Background(), imageJob(t, src))
	if !siphonerr.IsCode(err, siphonerr.SourceCorrupt) {
		t.Fatalf("got %v, want E_SOURCE_CORRUPT", err)
	}
}

func TestRefuseStaleJournal(t *testing.T) {
	dir := t.TempDir()
	src := writePNG(t, dir, "img.png", 4, 4, false)
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
