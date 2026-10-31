// Package pdfwrite implements the safe-only image → one-page PDF pair from
// the v0.2 addendum. It probes the image, applies the metadata policy, writes
// an internal PDF 1.7 document, verifies it structurally, and installs it
// atomically before consuming the source.
package pdfwrite

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"siphon/internal/core"
	"siphon/internal/engine/safeio"
	"siphon/internal/formats"
	"siphon/internal/pdf"
	"siphon/internal/platform"
	"siphon/internal/progress"
	"siphon/internal/siphonerr"
	"siphon/internal/version"
)

// Engine writes one image → PDF job in safe mode.
type Engine struct{}

// New returns the engine (registered by internal/engines).
func New() *Engine { return &Engine{} }

// Name identifies the engine.
func (*Engine) Name() string { return string(formats.EnginePDFWrite) }

// Convert runs the image → PDF pipeline.
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
		return nil, safeio.Untouched(err, "image → PDF failed")
	}

	dir := filepath.Dir(job.SourcePath)
	stem := safeio.Stem(job.SourcePath)
	src, err := safeio.OpenSource(job.SourcePath)
	if err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}
	defer src.Close()
	safeio.Reap(dir, stem, sink)

	sink.Emit(progress.Event{Phase: "probe", Percent: 0, Message: filepath.Base(job.SourcePath)})
	info, err := pdf.ProbeImage(ctx, job.SourcePath, job.Source)
	if err != nil {
		if ctx.Err() != nil {
			return nil, safeio.Interrupted(ctx.Err(), job.SourcePath)
		}
		return nil, safeio.Untouched(err, "image → PDF failed")
	}

	channels := 3
	if info.Gray {
		channels = 1
	}
	if len(info.ICC) > 0 {
		if err := pdf.ValidateICC(info.ICC, channels); err != nil {
			if !job.DropMetadata {
				return nil, safeio.Untouched(siphonerr.New(siphonerr.MetadataDrop,
					"%s has an ICC profile that cannot be embedded in the PDF: %v; pass --drop-metadata to treat it as sRGB",
					job.SourcePath, err).WithHints(map[string]bool{"drop_metadata": true}),
					"image → PDF failed")
			}
			sink.Warn("--drop-metadata set: the unusable ICC profile will not be embedded")
			info.ICC = nil
		}
	}
	untranslatable := pdf.UntranslatableEXIF(*info)
	if len(untranslatable) > 0 && !job.DropMetadata {
		return nil, safeio.Untouched(siphonerr.New(siphonerr.MetadataDrop,
			"%s carries EXIF fields with no PDF equivalent (%s); pass --drop-metadata to accept this",
			job.SourcePath, strings.Join(untranslatable, ", ")).
			WithHints(map[string]bool{"drop_metadata": true}), "image → PDF failed")
	}
	if job.DropMetadata && len(untranslatable) > 0 {
		sink.Warn("--drop-metadata set: EXIF fields not carried: " + strings.Join(untranslatable, ", "))
	}
	if len(info.XMP) > 0 {
		sink.Warn("XMP metadata is not carried into the PDF")
	}
	for _, w := range info.Warnings {
		sink.Warn(w)
	}

	input, err := pdf.BuildInput(info)
	if err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}

	tmp, err := safeio.CreateLockedTemp(dir, stem, ".tmp.pdf")
	if err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}
	tmpPath := tmp.Name()
	defer func() {
		tmp.Close()
		platform.RemoveIfExists(tmpPath)
	}()

	docInfo := pdf.DocInfo{
		Producer:     "Siphon " + version.String(),
		Creator:      "Siphon " + version.String(),
		CreationDate: info.EXIFInfo.PDFDate(),
	}
	if creator := info.EXIFInfo.Creator(); creator != "" {
		docInfo.Creator = creator
	}
	if docInfo.CreationDate != "" {
		docInfo.ModDate = docInfo.CreationDate
	}
	if err := pdf.WriteImage(tmp, input, docInfo); err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}
	if err := tmp.Sync(); err != nil {
		return nil, safeio.Untouched(siphonerr.Wrap(siphonerr.Internal, err,
			"cannot fsync the generated PDF"), "image → PDF failed")
	}
	if err := tmp.Close(); err != nil {
		return nil, safeio.Untouched(siphonerr.Wrap(siphonerr.Internal, err,
			"cannot close the generated PDF"), "image → PDF failed")
	}

	if err := pdf.VerifyFile(tmpPath, pdf.Expect{
		Width:    info.Width,
		Height:   info.Height,
		Gray:     info.Gray,
		HasAlpha: info.HasAlpha,
		HasICC:   len(info.ICC) > 0,
	}); err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}
	if err := src.Snapshot.Check(job.SourcePath); err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}

	metaWarnings, err := platform.CopyMetadata(job.SourcePath, tmpPath)
	if err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}
	for _, w := range metaWarnings {
		sink.Warn(w)
	}
	if err := platform.FsyncPath(tmpPath); err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}

	res, err := safeio.FinalizeQuiet(ctx, job, plan, src.Snapshot, tmpPath)
	if err != nil {
		return nil, safeio.Untouched(err, "image → PDF failed")
	}
	tmpPath = ""
	sink.Emit(progress.Event{Phase: "complete", Percent: 100})
	return res, nil
}

func validateJob(job *core.Job) error {
	if job == nil {
		return siphonerr.New(siphonerr.Internal, "nil PDF job")
	}
	if !formats.IsImagePDFPair(job.Pair) {
		return siphonerr.New(siphonerr.FormatUnsupported,
			"pdfwrite cannot convert %s → %s", job.Source, job.Target)
	}
	if job.Source != formats.PNG && job.Source != formats.JPG {
		return siphonerr.New(siphonerr.FormatUnsupported,
			"image → PDF supports PNG and JPEG only")
	}
	if job.Target != formats.PDF {
		return siphonerr.New(siphonerr.FormatUnsupported,
			"pdfwrite cannot write %s", job.Target)
	}
	if job.DPI != 0 {
		return siphonerr.New(siphonerr.Usage, "--dpi does not apply to image → PDF")
	}
	if job.MaxPixels != 0 {
		return siphonerr.New(siphonerr.Usage, "--max-pixels does not apply to image → PDF")
	}
	if job.Quality != 0 && job.Quality != 90 {
		return siphonerr.New(siphonerr.Usage, "--quality does not apply to image → PDF")
	}
	return nil
}

// DescribeOutput is a small helper used by plan rendering and tests.
func DescribeOutput(info *pdf.ImageInfo) string {
	return fmt.Sprintf("%s %d×%d px → one-page PDF 1.7", info.Format, info.Width, info.Height)
}
