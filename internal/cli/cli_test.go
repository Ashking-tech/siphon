package cli

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"siphon/internal/siphonerr"
)

func writeTestPNG(t *testing.T, path string, alpha bool) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 12, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 12; x++ {
			a := uint8(255)
			if alpha && x == 0 && y == 0 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 20), G: uint8(y * 20), B: 40, A: a})
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

// captureStdout runs fn with os.Stdout redirected to a pipe.
func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	runErr := fn()
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), runErr
}

func TestConvertCommandJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "photo.png")
	writeTestPNG(t, src, false)

	out, err := captureStdout(t, func() error {
		root := NewRoot()
		root.SetArgs([]string{"convert", src, "--to", "jpg", "--json"})
		return root.Execute()
	})
	if err != nil {
		t.Fatalf("convert failed: %v\noutput: %s", err, out)
	}

	var res struct {
		OK              bool `json:"ok"`
		SourceUntouched bool `json:"source_untouched"`
		Plan            struct {
			Engine      string `json:"engine"`
			Mode        string `json:"mode"`
			ImageWidth  int    `json:"image_width"`
			ImageHeight int    `json:"image_height"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if !res.OK || res.SourceUntouched {
		t.Fatalf("result = %+v", res)
	}
	if res.Plan.Engine != "atomiccopy" || res.Plan.Mode != "safe" {
		t.Fatalf("plan = %+v", res.Plan)
	}
	if res.Plan.ImageWidth != 12 || res.Plan.ImageHeight != 8 {
		t.Fatalf("plan dimensions = %+v", res.Plan)
	}
	if _, err := os.Stat(filepath.Join(dir, "photo.jpg")); err != nil {
		t.Fatalf("target missing: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatal("source still exists")
	}
}

func TestConvertCommandAlphaRefusalExit2(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "alpha.png")
	writeTestPNG(t, src, true)

	out, err := captureStdout(t, func() error {
		root := NewRoot()
		root.SetArgs([]string{"convert", src, "--to", "jpg", "--json"})
		return root.Execute()
	})
	if err == nil {
		t.Fatal("expected a refusal")
	}
	if !siphonerr.IsCode(err, siphonerr.AlphaUnsupported) {
		t.Fatalf("got %v, want E_ALPHA_UNSUPPORTED", err)
	}
	if code := siphonerr.ExitCode(err); code != 2 {
		t.Fatalf("exit code = %d, want 2", code)
	}
	var res struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if res.Code != "E_ALPHA_UNSUPPORTED" {
		t.Fatalf("code = %q", res.Code)
	}
	if strings.Contains(res.Message, res.Code) {
		t.Fatalf("message duplicates code: %q", res.Message)
	}
	if !strings.Contains(res.Message, "transparency") {
		t.Fatalf("message = %q", res.Message)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatal("source disappeared on refusal")
	}
	if _, err := os.Stat(filepath.Join(dir, "alpha.jpg")); !os.IsNotExist(err) {
		t.Fatal("target created on refusal")
	}
}
