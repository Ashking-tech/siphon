package convert

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
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 8), G: uint8(y * 8), B: 60, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func imageJob(t *testing.T, src string) *core.Job {
	t.Helper()
	pair, ok := formats.Lookup(formats.PNG, formats.JPG)
	if !ok {
		t.Fatal("pair missing")
	}
	stem := src[:len(src)-len(filepath.Ext(src))]
	return &core.Job{
		SourcePath:      src,
		TargetPath:      stem + ".jpg",
		Source:          formats.PNG,
		Target:          formats.JPG,
		Pair:            pair,
		Mode:            formats.ModeAuto,
		Quality:         90,
		SegmentDuration: 60 * time.Second,
		Verify:          "sampled",
		Timeout:         time.Minute,
		AllowRoot:       os.Geteuid() == 0,
	}
}

func TestInspectImage(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png")
	writePNG(t, src, 12, 8)

	plan, err := Inspect(context.Background(), imageJob(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Engine != "atomiccopy" || plan.Mode != "safe" {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.ImageWidth != 12 || plan.ImageHeight != 8 {
		t.Fatalf("plan dimensions = %dx%d", plan.ImageWidth, plan.ImageHeight)
	}
}

func TestRunImageConversion(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png")
	writePNG(t, src, 12, 8)
	job := imageJob(t, src)

	res, err := Run(context.Background(), job, progress.Noop{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source still exists")
	}
	if _, err := os.Stat(job.TargetPath); err != nil {
		t.Fatalf("target missing: %v", err)
	}

	// A second run must refuse: the target now exists.
	writePNG(t, src, 12, 8)
	if _, err := Run(context.Background(), job, progress.Noop{}); !siphonerr.IsCode(err, siphonerr.TargetExists) {
		t.Fatalf("got %v, want E_TARGET_EXISTS", err)
	}
}
