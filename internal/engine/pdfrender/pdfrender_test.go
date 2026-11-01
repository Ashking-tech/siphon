package pdfrender

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"siphon/internal/core"
	"siphon/internal/engine/pdfwrite"
	"siphon/internal/formats"
	"siphon/internal/progress"
)

func writeTestPNG(t *testing.T, path string, alpha bool) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 16; x++ {
			a := uint8(255)
			if alpha && x == 0 && y == 0 {
				a = 128
			}
			img.SetNRGBA(x, y, color.NRGBA{R: 200, G: 40, B: 20, A: a})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestCheckPNGNoAlphaAcceptsOpaquePNG(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opaque.png")
	writeTestPNG(t, path, false)
	if err := checkPNGNoAlpha(path); err != nil {
		t.Fatalf("opaque PNG rejected: %v", err)
	}
}

func TestCheckPNGNoAlphaRejectsTransparency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alpha.png")
	writeTestPNG(t, path, true)
	if err := checkPNGNoAlpha(path); err == nil {
		t.Fatal("transparent PNG accepted")
	}
}

func TestPDFToPNGRoundTrip(t *testing.T) {
	for _, tool := range []string{"pdfinfo", "pdftoppm"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}

	dir := t.TempDir()
	pngPath := filepath.Join(dir, "source.png")
	writeTestPNG(t, pngPath, false)

	toPDF, ok := formats.Lookup(formats.PNG, formats.PDF)
	if !ok {
		t.Fatal("PNG → PDF pair missing")
	}
	pdfPath := filepath.Join(dir, "source.pdf")
	if _, err := pdfwrite.New().Convert(context.Background(), &core.Job{
		SourcePath: pngPath,
		TargetPath: pdfPath,
		Source:     formats.PNG,
		Target:     formats.PDF,
		Pair:       toPDF,
		Mode:       formats.ModeSafe,
		Quality:    90,
		Timeout:    time.Minute,
	}, nil, progress.Noop{}); err != nil {
		t.Fatalf("png → pdf: %v", err)
	}

	toPNG, ok := formats.Lookup(formats.PDF, formats.PNG)
	if !ok {
		t.Fatal("PDF → PNG pair missing")
	}
	outPath := filepath.Join(dir, "out.png")
	if _, err := New().Convert(context.Background(), &core.Job{
		SourcePath: pdfPath,
		TargetPath: outPath,
		Source:     formats.PDF,
		Target:     formats.PNG,
		Pair:       toPNG,
		Mode:       formats.ModeSafe,
		Quality:    90,
		DPI:        150,
		Timeout:    time.Minute,
	}, nil, progress.Noop{}); err != nil {
		t.Fatalf("pdf → png: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if img.Bounds().Dx() == 0 || img.Bounds().Dy() == 0 {
		t.Fatalf("empty output: %v", img.Bounds())
	}
}
