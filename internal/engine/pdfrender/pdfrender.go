// Package pdfrender implements the safe-only PDF → PNG/JPEG pair from the v0.2
// addendum. It rasterizes exactly one page with poppler, verifies the single
// output file, and installs it atomically before consuming the PDF.
package pdfrender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"siphon/internal/core"
	"siphon/internal/engine/safeio"
	"siphon/internal/formats"
	"siphon/internal/pdf"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
)

// Engine renders one planned PDF job in safe mode.
type Engine struct{}

// New returns the engine (registered by internal/engines).
func New() *Engine { return &Engine{} }

// Name identifies the engine.
func (*Engine) Name() string { return string(formats.EnginePDFRender) }

// Convert runs the PDF → image pipeline.
func (e *Engine) Convert(ctx context.Context, job *core.Job, plan *core.Plan, sink progress.Sink) (*core.Result, error) {
	if sink == nil {
		sink = progress.Noop{}
	}
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}
	if err := validateJob(job); err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}

	dir := filepath.Dir(job.SourcePath)
	stem := safeio.Stem(job.SourcePath)
	src, err := safeio.OpenSource(job.SourcePath)
	if err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	defer src.Close()
	safeio.Reap(dir, stem, sink)

	sink.Emit(progress.Event{Phase: "probe", Percent: 0, Message: filepath.Base(job.SourcePath)})
	info, err := pdf.Probe(ctx, job.SourcePath)
	if err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	if info.Pages != 1 {
		return nil, safeio.Untouched(siphonerr.New(siphonerr.PDFMultipage,
			"%s has %d pages; PDF → image supports exactly one page",
			job.SourcePath, info.Pages), "PDF render failed")
	}

	dpi := job.DPI
	if dpi == 0 {
		dpi = pdf.DefaultDPI
	}
	maxPixels := job.MaxPixels
	if maxPixels == 0 {
		maxPixels = pdf.DefaultMaxPixels
	}
	if err := pdf.CheckPixelCap(info, dpi, maxPixels); err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	width, height := pdf.ExpectedDimensions(info, dpi)

	prefix, err := safeio.CreateLockedTemp(dir, stem, ".tmp")
	if err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	prefixPath := prefix.Name()
	defer prefix.Close()
	defer removeRenderFiles(filepath.Dir(prefixPath), filepath.Base(prefixPath))

	if err := runPDFToPPM(ctx, job.SourcePath, prefixPath, dpi, job.Target, job.Quality); err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "PDF render failed")
	}

	outPath, err := exactlyOneOutput(prefixPath, job.Target)
	if err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}

	if err := platform.FsyncPath(outPath); err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	if err := verifyOutput(outPath, job.Target, width, height); err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	if err := src.Snapshot.Check(job.SourcePath); err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}

	metaWarnings, err := platform.CopyMetadata(job.SourcePath, outPath)
	if err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	for _, w := range metaWarnings {
		sink.Warn(w)
	}
	if err := platform.FsyncPath(outPath); err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}

	res, err := safeio.FinalizeQuiet(ctx, job, plan, src.Snapshot, outPath)
	if err != nil {
		return nil, safeio.Untouched(err, "PDF render failed")
	}
	sink.Emit(progress.Event{Phase: "complete", Percent: 100})
	return res, nil
}

func validateJob(job *core.Job) error {
	if job == nil {
		return siphonerr.New(siphonerr.Internal, "nil PDF job")
	}
	if !formats.IsPDFImagePair(job.Pair) || job.Source != formats.PDF {
		return siphonerr.New(siphonerr.FormatUnsupported,
			"pdfrender cannot convert %s → %s", job.Source, job.Target)
	}
	if job.Target != formats.PNG && job.Target != formats.JPG {
		return siphonerr.New(siphonerr.FormatUnsupported,
			"pdfrender cannot write %s", job.Target)
	}
	if job.DPI != 0 && (job.DPI < pdf.MinDPI || job.DPI > pdf.MaxDPI) {
		return siphonerr.New(siphonerr.Usage, "--dpi must be between %d and %d", pdf.MinDPI, pdf.MaxDPI)
	}
	if job.Quality < 1 || job.Quality > 100 {
		return siphonerr.New(siphonerr.Usage, "--quality must be between 1 and 100")
	}
	return nil
}

func runPDFToPPM(ctx context.Context, src, prefix string, dpi int, target formats.Format, quality int) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot resolve %s", src)
	}
	args := []string{
		"-f", "1", "-l", "1", "-singlefile", "-cropbox",
		"-r", fmt.Sprintf("%d", dpi),
	}
	switch target {
	case formats.PNG:
		args = append(args, "-png")
	case formats.JPG:
		if quality <= 0 {
			quality = 90
		}
		args = append(args, "-jpeg", "-jpegopt", fmt.Sprintf("quality=%d,optimize=y", quality))
	default:
		return siphonerr.New(siphonerr.Internal, "unexpected PDF render target %s", target)
	}
	args = append(args, absSrc, prefix)

	cmd := exec.CommandContext(ctx, "pdftoppm", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	if devnull, err := os.Open(os.DevNull); err == nil {
		defer devnull.Close()
		cmd.Stdin = devnull
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		detail := lastLine(stderr.Bytes())
		if strings.Contains(strings.ToLower(detail), "no space left on device") {
			return siphonerr.Wrap(siphonerr.NoSpaceRuntime, err, "pdftoppm ran out of disk space")
		}
		return siphonerr.Wrap(siphonerr.PDFInvalid, err,
			"pdftoppm could not render %s: %s", src, detail)
	}
	return nil
}

// removeRenderFiles removes the renderer's prefix file and every page file it
// may have produced. A misbehaving or interrupted pdftoppm can leave a
// page-suffixed name that the expected-output check did not predict.
func removeRenderFiles(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		platform.RemoveIfExists(filepath.Join(dir, prefix))
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			platform.RemoveIfExists(filepath.Join(dir, e.Name()))
		}
	}
}

func exactlyOneOutput(prefix string, target formats.Format) (string, error) {
	want := prefix + "." + target.Ext()
	if target == formats.JPG {
		want = prefix + ".jpg"
	}
	if target == formats.PNG {
		want = prefix + ".png"
	}
	entries, err := filepath.Glob(prefix + "*")
	if err != nil {
		return "", siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot inspect the renderer output")
	}
	var extra []string
	for _, p := range entries {
		if p == prefix || p == want {
			continue
		}
		extra = append(extra, filepath.Base(p))
	}
	if len(extra) > 0 {
		return "", siphonerr.New(siphonerr.VerifyFailed,
			"pdftoppm produced unexpected file(s): %s", strings.Join(extra, ", "))
	}
	info, err := os.Lstat(want)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", siphonerr.New(siphonerr.VerifyFailed,
				"pdftoppm did not produce %s", filepath.Base(want))
		}
		return "", siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot stat the rendered page")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 {
		return "", siphonerr.New(siphonerr.VerifyFailed,
			"%s is not a non-empty regular file", filepath.Base(want))
	}
	return want, nil
}

func verifyOutput(path string, target formats.Format, wantW, wantH int) error {
	f, err := os.Open(path)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot open the rendered image")
	}
	defer f.Close()

	img, kind, err := image.Decode(f)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "rendered output does not decode")
	}
	switch target {
	case formats.PNG:
		if kind != "png" {
			return siphonerr.New(siphonerr.VerifyFailed, "rendered output is %s, not PNG", kind)
		}
		if err := checkPNGNoAlpha(path); err != nil {
			return err
		}
	case formats.JPG:
		if kind != "jpeg" {
			return siphonerr.New(siphonerr.VerifyFailed, "rendered output is %s, not JPEG", kind)
		}
	}

	b := img.Bounds()
	if absInt(b.Dx()-wantW) > 1 || absInt(b.Dy()-wantH) > 1 {
		return siphonerr.New(siphonerr.VerifyFailed,
			"rendered image is %d × %d px, expected %d × %d px (±1)",
			b.Dx(), b.Dy(), wantW, wantH)
	}
	return nil
}

// checkPNGNoAlpha rejects IHDR alpha color types and palette transparency. The
// rasterizer contract is an opaque PNG composited on white.
func checkPNGNoAlpha(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot inspect the PNG header")
	}
	defer f.Close()
	var hdr [33]byte
	if _, err := f.Read(hdr[:]); err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot read the PNG header")
	}
	if !bytes.Equal(hdr[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}) ||
		string(hdr[12:16]) != "IHDR" {
		return siphonerr.New(siphonerr.VerifyFailed, "rendered output has no valid PNG IHDR")
	}
	colorType := hdr[25]
	if colorType == 4 || colorType == 6 {
		return siphonerr.New(siphonerr.VerifyFailed,
			"rendered PNG has an alpha channel; PDF → image must be opaque")
	}
	if colorType == 3 && pngHasTRNS(path) {
		return siphonerr.New(siphonerr.VerifyFailed,
			"rendered PNG palette is transparent; PDF → image must be opaque")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot rewind the PNG header")
	}
	if _, err := png.DecodeConfig(f); err != nil {
		return siphonerr.Wrap(siphonerr.VerifyFailed, err, "rendered PNG header does not decode")
	}
	return nil
}

func pngHasTRNS(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	// The contract's rasterizer output is small-header; scanning the first
	// chunk stream for tRNS is sufficient and non-allocating.
	buf := make([]byte, 64<<10)
	n, _ := f.Read(buf)
	return bytes.Contains(buf[:n], []byte("tRNS"))
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func lastLine(b []byte) string {
	lines := bytes.Split(bytes.TrimSpace(b), []byte{'\n'})
	if len(lines) == 0 {
		return "(no stderr)"
	}
	s := strings.TrimSpace(string(lines[len(lines)-1]))
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	return s
}
