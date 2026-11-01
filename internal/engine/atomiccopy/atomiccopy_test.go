package atomiccopy

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/imaging"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// --- helpers ---------------------------------------------------------------

func makePNG(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x < w/2 && y < h/2 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(x * 255 / max(1, w-1)),
				G: uint8(y * 255 / max(1, h-1)),
				B: 40,
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

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 20, G: 20, B: 20, A: 255}
			if x < w/2 {
				c = color.NRGBA{R: 230, G: 230, B: 230, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func writeFile(t *testing.T, dir, name string, data []byte, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	return path
}

func targetFor(src string, dst formats.Format) string {
	ext := filepath.Ext(src)
	return strings.TrimSuffix(src, ext) + "." + dst.Ext()
}

func jobFor(t *testing.T, src string, srcFormat, dstFormat formats.Format) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(srcFormat, dstFormat)
	if !ok {
		t.Fatalf("pair %s → %s missing", srcFormat, dstFormat)
	}
	return &core.Job{
		SourcePath: src,
		TargetPath: targetFor(src, dstFormat),
		Source:     srcFormat,
		Target:     dstFormat,
		Pair:       pair,
		Mode:       formats.ModeSafe,
		Quality:    90,
		Timeout:    time.Minute,
		AllowRoot:  os.Geteuid() == 0,
	}
}

func run(t *testing.T, job *core.Job) (*core.Result, error) {
	t.Helper()
	return New().Convert(context.Background(), job, nil, progress.Noop{})
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".siphon-") {
			t.Fatalf("leftover temp %s", e.Name())
		}
	}
}

func decodeFile(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func hasTool(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// --- tests -----------------------------------------------------------------

func TestConvertPNGToJPEG(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 24, 16, false), 0o640)

	job := jobFor(t, src, formats.PNG, formats.JPG)
	res, err := run(t, job)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists: %v", err)
	}
	info, err := os.Stat(job.TargetPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("target mode = %o, want 640", info.Mode().Perm())
	}
	img := decodeFile(t, job.TargetPath)
	if img.Bounds().Dx() != 24 || img.Bounds().Dy() != 16 {
		t.Fatalf("decoded target = %v", img.Bounds())
	}
	assertNoTemps(t, dir)
}

func TestAlphaRefusedWithoutBackground(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "alpha.png", makePNG(t, 8, 8, true), 0o600)
	before, _ := os.ReadFile(src)

	job := jobFor(t, src, formats.PNG, formats.JPG)
	_, err := run(t, job)
	if !siphonerr.IsCode(err, siphonerr.AlphaUnsupported) {
		t.Fatalf("got %v, want E_ALPHA_UNSUPPORTED", err)
	}
	if !siphonerr.SourceUntouched(err) {
		t.Fatal("alpha refusal must leave the source untouched")
	}
	after, _ := os.ReadFile(src)
	if !bytes.Equal(before, after) {
		t.Fatal("source bytes changed")
	}
	if _, err := os.Stat(job.TargetPath); !os.IsNotExist(err) {
		t.Fatal("target was created on refusal")
	}
	assertNoTemps(t, dir)
}

func TestAlphaCompositeWithBackground(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "alpha.png", makePNG(t, 32, 32, true), 0o600)

	job := jobFor(t, src, formats.PNG, formats.JPG)
	job.Background = "red"
	if _, err := run(t, job); err != nil {
		t.Fatal(err)
	}
	img := decodeFile(t, job.TargetPath)
	r, g, b, _ := img.At(8, 8).RGBA()
	if uint8(r>>8) < 200 || uint8(g>>8) > 80 || uint8(b>>8) > 80 {
		t.Fatalf("transparent pixel did not composite onto red: %d %d %d", r>>8, g>>8, b>>8)
	}
}

func TestJPEGToPNGOrientation(t *testing.T) {
	dir := t.TempDir()
	meta := &imaging.Metadata{EXIF: tinyEXIF(6)}
	data, err := imaging.AddMetadata(makeJPEG(t, 4, 2), formats.JPG, meta)
	if err != nil {
		t.Fatal(err)
	}
	src := writeFile(t, dir, "rotated.jpg", data, 0o600)

	job := jobFor(t, src, formats.JPG, formats.PNG)
	if _, err := run(t, job); err != nil {
		t.Fatal(err)
	}
	img := decodeFile(t, job.TargetPath)
	if img.Bounds().Dx() != 2 || img.Bounds().Dy() != 4 {
		t.Fatalf("orientation not applied: %v", img.Bounds())
	}
	outMeta, _, err := imaging.Extract(job.TargetPath, formats.PNG)
	if err != nil {
		t.Fatal(err)
	}
	if imaging.Orientation(outMeta.EXIF) != 1 {
		t.Fatalf("output EXIF orientation = %d, want 1", imaging.Orientation(outMeta.EXIF))
	}
}

func TestMetadataPreserved(t *testing.T) {
	dir := t.TempDir()
	profile := bytes.Repeat([]byte{0x5A}, 2048)
	xmp := []byte("<x:xmpmeta>keep me</x:xmpmeta>")
	meta := &imaging.Metadata{ICC: profile, EXIF: tinyEXIF(1), XMP: xmp}
	data, err := imaging.AddMetadata(makePNG(t, 12, 12, false), formats.PNG, meta)
	if err != nil {
		t.Fatal(err)
	}
	src := writeFile(t, dir, "tagged.png", data, 0o600)

	job := jobFor(t, src, formats.PNG, formats.JPG)
	if _, err := run(t, job); err != nil {
		t.Fatal(err)
	}
	got, _, err := imaging.Extract(job.TargetPath, formats.JPG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.ICC, profile) {
		t.Fatalf("ICC not preserved: %d bytes", len(got.ICC))
	}
	if len(got.EXIF) == 0 {
		t.Fatal("EXIF not preserved")
	}
	if !bytes.Equal(got.XMP, xmp) {
		t.Fatalf("XMP not preserved: %q", got.XMP)
	}
}

func TestDropMetadata(t *testing.T) {
	dir := t.TempDir()
	meta := &imaging.Metadata{ICC: []byte("icc"), XMP: []byte("<xmp/>")}
	data, err := imaging.AddMetadata(makePNG(t, 8, 8, false), formats.PNG, meta)
	if err != nil {
		t.Fatal(err)
	}
	src := writeFile(t, dir, "tagged.png", data, 0o600)

	job := jobFor(t, src, formats.PNG, formats.JPG)
	job.DropMetadata = true
	if _, err := run(t, job); err != nil {
		t.Fatal(err)
	}
	got, _, err := imaging.Extract(job.TargetPath, formats.JPG)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ICC) != 0 || len(got.XMP) != 0 {
		t.Fatalf("metadata survived --drop-metadata: %+v", got.Classes())
	}
}

func TestTargetExistsWithoutForce(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 8, 8, false), 0o600)
	target := targetFor(src, formats.JPG)
	if err := os.WriteFile(target, []byte("do not touch"), 0o600); err != nil {
		t.Fatal(err)
	}

	job := jobFor(t, src, formats.PNG, formats.JPG)
	_, err := run(t, job)
	if !siphonerr.IsCode(err, siphonerr.TargetExists) {
		t.Fatalf("got %v, want E_TARGET_EXISTS", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source disappeared")
	}
	body, _ := os.ReadFile(target)
	if string(body) != "do not touch" {
		t.Fatal("existing target was modified")
	}
	assertNoTemps(t, dir)
}

func TestForceReplacesTarget(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 8, 8, false), 0o600)
	target := targetFor(src, formats.JPG)
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	job := jobFor(t, src, formats.PNG, formats.JPG)
	job.Force = true
	if _, err := run(t, job); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source still exists")
	}
	if img := decodeFile(t, target); img.Bounds().Dx() != 8 {
		t.Fatalf("target was not replaced: %v", img.Bounds())
	}
}

func TestCheckSnapshotDetectsChange(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 4, 4, false), 0o600)
	snap, err := platform.TakeSnapshot(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := snap.Check(src); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := snap.Check(src); !siphonerr.IsCode(err, siphonerr.SourceChanged) {
		t.Fatalf("got %v, want E_SOURCE_CHANGED", err)
	}
}

func TestReapsStaleTemps(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 8, 8, false), 0o600)

	stale := filepath.Join(dir, ".siphon-photo-deadbeef.tmp")
	if err := os.WriteFile(stale, []byte("abandoned"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, jobFor(t, src, formats.PNG, formats.JPG)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatal("stale temp was not reaped")
	}
}

func TestReaperLeavesLockedTemps(t *testing.T) {
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 8, 8, false), 0o600)

	locked, err := platform.CreateTemp(dir, "photo", ".tmp")
	if err != nil {
		t.Fatal(err)
	}
	defer locked.Close()
	if err := platform.LockFile(locked); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(locked.Name(), old, old); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, jobFor(t, src, formats.PNG, formats.JPG)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(locked.Name()); err != nil {
		t.Fatal("a locked temp was reaped")
	}
}

func TestWebPRoundTrip(t *testing.T) {
	if !hasTool("cwebp") || !hasTool("dwebp") {
		t.Skip("cwebp/dwebp not installed")
	}
	dir := t.TempDir()
	src := writeFile(t, dir, "photo.png", makePNG(t, 16, 16, false), 0o600)

	job := jobFor(t, src, formats.PNG, formats.WEBP)
	if _, err := run(t, job); err != nil {
		t.Fatal(err)
	}
	img, _, _, err := imaging.Decode(context.Background(), job.TargetPath, formats.WEBP, dir)
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 16 {
		t.Fatalf("webp target = %v", img.Bounds())
	}
	assertNoTemps(t, dir)

	// And back: WebP → PNG.
	webp := writeFile(t, dir, "back.webp", mustRead(t, job.TargetPath), 0o600)
	job2 := jobFor(t, webp, formats.WEBP, formats.PNG)
	if _, err := run(t, job2); err != nil {
		t.Fatal(err)
	}
	if img := decodeFile(t, job2.TargetPath); img.Bounds().Dx() != 16 {
		t.Fatalf("png target = %v", img.Bounds())
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// tinyEXIF builds a minimal little-endian TIFF with one orientation entry.
func tinyEXIF(orientation uint16) []byte {
	b := make([]byte, 26)
	copy(b[0:2], "II")
	binaryLE(b[2:4], 42)
	binaryLE(b[4:8], 8)
	binaryLE(b[8:10], 1)
	e := b[10:22]
	binaryLE(e[0:2], 0x0112)
	binaryLE(e[2:4], 3)
	binaryLE(e[4:8], 1)
	binaryLE(e[8:10], uint32(orientation))
	binaryLE(b[22:26], 0)
	return b
}

func binaryLE(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	if len(b) >= 4 {
		b[2] = byte(v >> 16)
		b[3] = byte(v >> 24)
	}
}
