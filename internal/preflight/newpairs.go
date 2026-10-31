package preflight

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/imaging"
	"siphon/internal/journal"
	"siphon/internal/media"
	"siphon/internal/pdf"
	"siphon/internal/platform"
	"siphon/internal/siphonerr"
)

// runNewPair handles the v0.2 document/image and media pairs. It returns
// (nil, nil) when the pair is an existing v1.7 pair so Run can continue with
// the frozen legacy gate.
func runNewPair(ctx context.Context, job *core.Job, info os.FileInfo) (*core.Plan, error) {
	switch {
	case formats.IsPDFImagePair(job.Pair):
		return runPDFImage(ctx, job, info)
	case formats.IsImagePDFPair(job.Pair):
		return runImagePDF(ctx, job, info)
	case formats.IsContainerRemux(job.Pair), formats.IsWebMMP4(job.Pair),
		formats.IsAudioExtract(job.Pair), formats.IsGIFPair(job.Pair):
		return runMediaPair(ctx, job, info)
	}
	return nil, nil
}

func runPDFImage(ctx context.Context, job *core.Job, info os.FileInfo) (*core.Plan, error) {
	if err := commonNewPairChecks(job); err != nil {
		return nil, refuse(err)
	}

	dpi := job.DPI
	if dpi == 0 {
		dpi = pdf.DefaultDPI
	}
	maxPixels := job.MaxPixels
	if maxPixels == 0 {
		maxPixels = pdf.DefaultMaxPixels
	}
	if dpi < pdf.MinDPI || dpi > pdf.MaxDPI {
		return nil, refuse(siphonerr.New(siphonerr.Usage,
			"--dpi must be between %d and %d", pdf.MinDPI, pdf.MaxDPI))
	}
	if maxPixels <= 0 {
		return nil, refuse(siphonerr.New(siphonerr.Usage, "--max-pixels must be positive"))
	}

	pinfo, err := pdf.Probe(ctx, job.SourcePath)
	if err != nil {
		return nil, refuse(err)
	}
	if pinfo.Pages > 1 {
		return nil, refuse(siphonerr.New(siphonerr.PDFMultipage,
			"%s has %d pages; PDF → image supports exactly one page",
			job.SourcePath, pinfo.Pages))
	}
	if pinfo.Pages < 1 {
		return nil, refuse(siphonerr.New(siphonerr.PDFInvalid,
			"PDF has %d pages", pinfo.Pages))
	}
	if err := pdf.CheckPixelCap(pinfo, dpi, maxPixels); err != nil {
		return nil, refuse(err)
	}
	widthPX, heightPX := pdf.ExpectedDimensions(pinfo, dpi)
	pixels := int64(widthPX) * int64(heightPX)

	estimate := pixels*4 + mib
	if job.Target == formats.JPG {
		estimate = pixels*3 + mib
	}
	needed := info.Size() + estimate + 2*mib

	box := pinfo.PageBox()
	plan := &core.Plan{
		Source:          job.SourcePath,
		Target:          job.TargetPath,
		SourceFormat:    job.Source.String(),
		TargetFormat:    job.Target.String(),
		Mode:            string(job.Mode),
		Engine:          string(job.Engine),
		OutputKind:      job.Pair.OutputKindFor(job.Mode),
		Lossy:           job.Pair.Lossy,
		SourceSize:      info.Size(),
		FreeNeeded:      needed,
		FreeNeededHuman: core.HumanBytes(needed),
		CrashCost:       crashCost(job),
		PDF: &core.PDFPlan{
			Pages:        pinfo.Pages,
			PageWidthPT:  box.Width(),
			PageHeightPT: box.Height(),
			Rotation:     pinfo.Rotation,
			Encrypted:    pinfo.Encrypted,
		},
		Render: &core.RenderPlan{
			DPI:       dpi,
			WidthPX:   widthPX,
			HeightPX:  heightPX,
			MaxPixels: maxPixels,
		},
	}
	plan.Metadata = pinfo.Metadata
	plan.Warnings = append(plan.Warnings,
		fmt.Sprintf("PDF is rasterized at %d DPI; this is not a lossless conversion", dpi))
	if job.Target == formats.PNG {
		plan.Warnings = append(plan.Warnings, "output is 8-bit sRGB PNG on a white background")
	} else {
		plan.Warnings = append(plan.Warnings, "output is 8-bit sRGB JPEG on a white background")
	}
	if len(pinfo.Metadata) > 0 {
		plan.Warnings = append(plan.Warnings,
			"PDF metadata not carried: "+strings.Join(pinfo.Metadata, ", "))
	}
	return finishSpace(job, plan)
}

func runImagePDF(ctx context.Context, job *core.Job, info os.FileInfo) (*core.Plan, error) {
	if err := commonNewPairChecks(job); err != nil {
		return nil, refuse(err)
	}

	probeCtx := ctx
	if job.Timeout > 0 {
		var cancel context.CancelFunc
		probeCtx, cancel = context.WithTimeout(ctx, job.Timeout)
		defer cancel()
	}
	iinfo, err := pdf.ProbeImage(probeCtx, job.SourcePath, job.Source)
	if err != nil {
		return nil, refuse(err)
	}

	channels := 3
	if iinfo.Gray {
		channels = 1
	}
	if len(iinfo.ICC) > 0 {
		if err := pdf.ValidateICC(iinfo.ICC, channels); err != nil {
			if !job.DropMetadata {
				return nil, refuse(siphonerr.New(siphonerr.MetadataDrop,
					"%s has an ICC profile that cannot be embedded in the PDF: %v; pass --drop-metadata to treat it as sRGB",
					job.SourcePath, err).WithHints(map[string]bool{"drop_metadata": true}))
			}
		}
	}
	untranslatable := pdf.UntranslatableEXIF(*iinfo)
	if len(untranslatable) > 0 && !job.DropMetadata {
		return nil, refuse(siphonerr.New(siphonerr.MetadataDrop,
			"%s carries EXIF fields with no PDF equivalent (%s); pass --drop-metadata to accept this",
			job.SourcePath, strings.Join(untranslatable, ", ")).
			WithHints(map[string]bool{"drop_metadata": true}))
	}

	pageW, pageH := pdf.PageSize(iinfo)
	usedChannels := channels
	if iinfo.HasAlpha {
		usedChannels = 4
	}
	raw := int64(iinfo.Width) * int64(iinfo.Height) * int64(usedChannels)
	recompressed := raw + raw/10
	estimate := recompressed
	if info.Size() > estimate {
		estimate = info.Size()
	}
	estimate += mib
	needed := info.Size() + estimate + 2*mib

	plan := &core.Plan{
		Source:          job.SourcePath,
		Target:          job.TargetPath,
		SourceFormat:    job.Source.String(),
		TargetFormat:    job.Target.String(),
		Mode:            string(job.Mode),
		Engine:          string(job.Engine),
		OutputKind:      job.Pair.OutputKindFor(job.Mode),
		Lossy:           job.Pair.Lossy,
		SourceSize:      info.Size(),
		FreeNeeded:      needed,
		FreeNeededHuman: core.HumanBytes(needed),
		CrashCost:       crashCost(job),
		ImageWidth:      iinfo.Width,
		ImageHeight:     iinfo.Height,
		HasAlpha:        iinfo.HasAlpha,
		Page: &core.PagePlan{
			DPI:      iinfo.DPI,
			WidthPT:  pageW,
			HeightPT: pageH,
		},
	}
	if len(iinfo.ICC) > 0 {
		plan.Metadata = append(plan.Metadata, "ICC profile")
	}
	if len(iinfo.EXIF) > 0 {
		plan.Metadata = append(plan.Metadata, "EXIF")
	}
	if len(iinfo.XMP) > 0 {
		plan.Metadata = append(plan.Metadata, "XMP")
	}
	if iinfo.HasAlpha {
		plan.Warnings = append(plan.Warnings, "alpha is preserved with a PDF SMask")
	}
	if len(iinfo.ICC) > 0 && job.DropMetadata {
		plan.Warnings = append(plan.Warnings, "--drop-metadata set: invalid ICC metadata is treated as sRGB")
	}
	if len(iinfo.XMP) > 0 {
		plan.Warnings = append(plan.Warnings, "XMP metadata is not carried into the PDF")
	}
	if job.DropMetadata && len(untranslatable) > 0 {
		plan.Warnings = append(plan.Warnings,
			"--drop-metadata set: EXIF fields not carried: "+strings.Join(untranslatable, ", "))
	}
	plan.Warnings = append(plan.Warnings, iinfo.Warnings...)
	return finishSpace(job, plan)
}

func runMediaPair(ctx context.Context, job *core.Job, info os.FileInfo) (*core.Plan, error) {
	if err := commonNewPairChecks(job); err != nil {
		return nil, refuse(err)
	}
	if job.Background != "" {
		if _, err := imaging.ParseBackground(job.Background); err != nil {
			return nil, refuse(err)
		}
	}

	probe, err := media.ProbeFile(ctx, job.SourcePath)
	if err != nil {
		return nil, refuse(err)
	}
	var gifInfo *media.GIFInfo
	if job.Source == formats.GIF {
		gifInfo, err = media.ProbeGIF(job.SourcePath)
		if err != nil {
			return nil, refuse(err)
		}
		if job.Target == formats.MP4 && gifInfo.Transparent && job.Background == "" {
			return nil, refuse(siphonerr.New(siphonerr.AlphaUnsupported,
				"%s has GIF transparency; pass --bg <colour> to flatten it",
				job.SourcePath).WithHints(map[string]bool{"bg_required": true}))
		}
	}

	pp, planErr := media.BuildPairPlan(probe, job.Source, job.Target, media.PairOptions{
		DropIncompatible: job.DropIncompatible,
		AllowNonstandard: job.AllowNonstandard,
		Transcode:        job.Transcode,
		DropMetadata:     job.DropMetadata,
		VideoQuality:     job.VideoQuality,
		VideoQualitySet:  job.VideoQualitySet,
		AudioBitrate:     job.AudioBitrate,
		AudioTrack:       job.AudioTrack,
		FPS:              job.FPS,
		Width:            job.Width,
	})
	if pp == nil {
		if planErr != nil {
			return nil, refuse(planErr)
		}
		return nil, refuse(siphonerr.New(siphonerr.SourceCorrupt, "ffprobe returned no media plan"))
	}

	needed := mediaEstimate(job, probe, pp, gifInfo, info.Size())
	plan := &core.Plan{
		Source:          job.SourcePath,
		Target:          job.TargetPath,
		SourceFormat:    job.Source.String(),
		TargetFormat:    job.Target.String(),
		Mode:            string(job.Mode),
		Engine:          string(job.Engine),
		OutputKind:      job.Pair.OutputKindFor(job.Mode),
		Lossy:           job.Pair.Lossy || pp.Transcode,
		SourceSize:      info.Size(),
		FreeNeeded:      needed,
		FreeNeededHuman: core.HumanBytes(needed),
		CrashCost:       crashCost(job),
		Warnings:        append([]string(nil), pp.Warnings...),
		Dropped:         append([]string(nil), pp.Dropped...),
	}
	plan.Media = &core.MediaPlan{
		SourceContainer: pp.SourceContainer,
		TargetContainer: pp.TargetContainer,
		Remux:           pp.Remux,
		Transcode:       pp.Transcode,
	}
	for _, s := range pp.Streams {
		plan.Media.Streams = append(plan.Media.Streams, core.MediaStream{
			Type: s.Type, Codec: s.Codec, Action: string(s.Action),
		})
		plan.Streams = append(plan.Streams, fmt.Sprintf("%s:%d %s", s.Type, s.Index, s.Codec))
	}
	if pp.Transcode && job.Target != formats.GIF {
		plan.Transcode = &core.TranscodePlan{
			VideoCodec:   pp.TargetVideoCodec,
			VideoQuality: pp.VideoQuality,
			AudioCodec:   pp.TargetAudioCodec,
			AudioBitrate: pp.AudioBitrate,
		}
	}
	if formats.IsAudioExtract(job.Pair) {
		// Audio extraction has its own plan group; a video/audio transcode
		// group would be misleading.
		plan.Transcode = nil
		plan.Audio = &core.AudioPlan{
			Track:       pp.AudioTrack,
			SourceCodec: pp.AudioCodec,
			TargetCodec: "mp3",
			Bitrate:     pp.AudioBitrate,
			VBR:         pp.AudioBitrate == 0,
		}
		if selected := findStream(probe, pp.AudioIndex); selected != nil && selected.Channels > 2 {
			plan.Warnings = append(plan.Warnings,
				fmt.Sprintf("audio track %d has %d channels; MP3 is stereo and the track will be downmixed",
					pp.AudioTrack, selected.Channels))
		}
	}
	if job.Target == formats.GIF {
		plan.GIF = &core.GIFPlan{Width: pp.Width, FPS: pp.FPS, Loop: 0}
	}
	if pp.Remux && pp.Strict {
		plan.Warnings = append(plan.Warnings,
			"VP9/Opus in MP4 is nonstandard; --allow-nonstandard permits a poorly supported remux")
	}
	if pp.Transcode && formats.IsWebMMP4(job.Pair) {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("video and audio are re-encoded to %s / %s; this is lossy",
				pp.TargetVideoCodec, pp.TargetAudioCodec))
	}
	if pp.Transcode && formats.IsGIFPair(job.Pair) {
		if job.Source == formats.GIF {
			if len(probe.AudioStreams()) > 0 {
				plan.Warnings = append(plan.Warnings, "GIF audio is dropped")
			}
		} else {
			plan.Warnings = append(plan.Warnings,
				"audio, subtitles, chapters, and metadata are not carried into GIF")
		}
	}
	if gifInfo != nil && gifInfo.Transparent {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("GIF transparency is flattened onto %s", job.Background))
	}
	if (job.Source == formats.GIF || job.Target == formats.GIF) &&
		gifInfo != nil && gifInfo.FrameRate > 0 {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("GIF source averages %.2f fps", gifInfo.FrameRate))
	}
	if job.Source == formats.GIF && job.Target == formats.MP4 &&
		(pp.SourceWidth != pp.Width || pp.SourceHeight != pp.Height) {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("odd GIF dimensions are padded to %d×%d", pp.Width, pp.Height))
	}
	for _, w := range gifInfoWarnings(gifInfo) {
		plan.Warnings = append(plan.Warnings, w)
	}

	if planErr != nil {
		return refusalPlan(plan, planErr)
	}
	if pp.Transcode {
		features := media.FeaturesFor(featureKey(job.Source, job.Target), true)
		if err := media.CheckFeatures(ctx, features); err != nil {
			return refusalPlan(plan, err)
		}
	}
	return finishSpace(job, plan)
}

func gifInfoWarnings(g *media.GIFInfo) []string {
	if g == nil {
		return nil
	}
	return g.Warnings
}

func findStream(p *media.Probe, index int) *media.Stream {
	if p == nil || index < 0 {
		return nil
	}
	for i := range p.Streams {
		if p.Streams[i].Index == index {
			return &p.Streams[i]
		}
	}
	return nil
}

func featureKey(source, target formats.Format) string {
	switch {
	case formats.IsAudioExtract(formats.Pair{Source: source, Target: target}):
		return "video-mp3"
	case source == formats.GIF && target == formats.MP4:
		return "gif-mp4"
	case source == formats.MP4 && target == formats.GIF:
		return "mp4-gif"
	case source == formats.WEBM && target == formats.MP4:
		return "webm-mp4"
	case source == formats.MP4 && target == formats.WEBM:
		return "mp4-webm"
	}
	return ""
}

func mediaEstimate(job *core.Job, probe *media.Probe, pp *media.PairPlan, gif *media.GIFInfo, sourceSize int64) int64 {
	duration := probe.Duration()
	if gif != nil && gif.Total > 0 {
		duration = gif.Total
	}
	switch {
	case job.Source == formats.GIF && job.Target == formats.MP4:
		return sourceSize + sourceSize*3/2 + 64*mib
	case job.Source == formats.MP4 && job.Target == formats.GIF:
		w := int64(pp.Width)
		h := int64(pp.Height)
		fps := pp.FPS
		if w <= 0 || h <= 0 || fps <= 0 || duration <= 0 {
			return sourceSize*3 + 34*mib
		}
		pixels := w * h * int64(math.Ceil(fps)) * int64(math.Ceil(duration.Seconds()))
		// spec §12.7: estimate = max(source_size, pixels × 1.5) + 32 MiB.
		gifBytes := pixels * 3 / 2
		if sourceSize > gifBytes {
			gifBytes = sourceSize
		}
		return sourceSize + gifBytes + 32*mib + 2*mib
	case formats.IsAudioExtract(job.Pair):
		seconds := duration.Seconds()
		est := int64(seconds*320000/8*105/100) + 2*mib
		return sourceSize + est
	case formats.IsWebMMP4(job.Pair):
		return sourceSize + sourceSize*3/2 + 64*mib
	default:
		// Container remux, §12.3.
		return sourceSize + sourceSize*103/100 + sumStreamBits(probe)*11/10/8 + 4*mib
	}
}

func finishSpace(job *core.Job, plan *core.Plan) (*core.Plan, error) {
	dir := filepath.Dir(job.SourcePath)
	free, err := platform.FreeSpace(dir)
	if err != nil {
		return refusalPlan(plan, siphonerr.Wrap(siphonerr.NoSpace, err, "cannot read free space"))
	}
	plan.FreeAvailable = free
	plan.FreeAvailableHuman = core.HumanBytes(free)
	if free < plan.FreeNeeded {
		msg := fmt.Sprintf("%s needs %s free; %s has %s available",
			job.SourcePath, core.HumanBytes(plan.FreeNeeded), dir, core.HumanBytes(free))
		return refusalPlan(plan, siphonerr.New(siphonerr.NoSpace, "%s", msg))
	}
	return plan, nil
}

func refusalPlan(plan *core.Plan, err error) (*core.Plan, error) {
	if plan != nil {
		plan.Refusal = string(siphonerr.CodeOf(err))
		plan.Hints = siphonerr.HintsOf(err)
		if plan.FreeAvailableHuman == "" {
			if free, statErr := platform.FreeSpace(filepath.Dir(plan.Source)); statErr == nil {
				plan.FreeAvailable = free
				plan.FreeAvailableHuman = core.HumanBytes(free)
			}
		}
	}
	return plan, refuse(err)
}

func commonNewPairChecks(job *core.Job) error {
	dir := filepath.Dir(job.SourcePath)
	stem := strings.TrimSuffix(filepath.Base(job.SourcePath), filepath.Ext(job.SourcePath))
	if paths, err := journal.Find(dir, stem); err == nil && len(paths) > 0 {
		return siphonerr.New(siphonerr.StaleJournal,
			"a Siphon journal exists for %s; run `siphon recover %s`", job.SourcePath, job.SourcePath)
	}
	if err := platform.ValidateMagic(job.SourcePath, job.Pair.Source); err != nil {
		return err
	}
	if immutable, err := platform.HasImmutable(job.SourcePath); err != nil {
		return siphonerr.Wrap(siphonerr.Permission, err, "cannot read file flags")
	} else if immutable {
		return siphonerr.New(siphonerr.Permission, "%s is immutable", job.SourcePath)
	}
	if os.Geteuid() == 0 && !job.AllowRoot {
		return siphonerr.New(siphonerr.RootDenied,
			"refusing to run as root; pass --allow-root only if you are certain")
	}
	return nil
}

// validateNewPairFlags rejects flags outside their applicable pair instead of
// silently ignoring them (spec §5.1). Only flags recorded as explicitly
// supplied are checked, so direct core.Job construction in tests keeps its
// zero-value defaults.
func validateNewPairFlags(job *core.Job) error {
	if job == nil || len(job.Flags) == 0 {
		return nil
	}
	p := job.Pair
	allowed := map[string]bool{}
	add := func(names ...string) {
		for _, n := range names {
			allowed[n] = true
		}
	}
	add("mode", "force", "yes")

	switch {
	case formats.IsPDFImagePair(p):
		add("dpi", "max-pixels")
		if job.Target == formats.JPG {
			add("quality")
		}
	case formats.IsImagePDFPair(p):
		add("drop-metadata")
	case formats.IsContainerRemux(p):
		add("drop-incompatible", "drop-metadata")
	case formats.IsWebMMP4(p):
		add("transcode", "video-quality", "audio-bitrate",
			"drop-incompatible", "drop-metadata", "allow-nonstandard")
	case formats.IsAudioExtract(p):
		add("audio-bitrate", "audio-track", "drop-metadata")
	case formats.IsGIFPair(p):
		add("fps")
		if p.Source == formats.GIF {
			add("bg", "video-quality")
		} else {
			add("width")
		}
	case p.Source == formats.MKV && p.Target == formats.MP4:
		// The frozen v1.7 pair keeps its existing flags.
		add("drop-incompatible", "transcode-audio", "allow-nonstandard",
			"drop-metadata", "segment-duration", "segment-tolerance",
			"verify", "verify-frames", "probe-segments", "skip-probe")
	case p.Source == formats.WAV && p.Target == formats.MP3:
		add("bitrate", "drop-metadata")
	case formats.IsImage(p.Source) && formats.IsImage(p.Target):
		// Existing image pairs.
		if p.Target == formats.JPG || p.Target == formats.WEBP {
			add("quality", "bg")
		}
		add("drop-metadata")
	}

	for name := range job.Flags {
		switch name {
		case "to", "dry-run", "json", "timeout", "allow-root", "allow-hardlinks":
			continue
		}
		if !allowed[name] {
			return siphonerr.New(siphonerr.Usage,
				"--%s does not apply to %s → %s", name, job.Source, job.Target)
		}
	}
	return validateNewPairValues(job)
}

func validateNewPairValues(job *core.Job) error {
	if job.Flags["dpi"] && (job.DPI < pdf.MinDPI || job.DPI > pdf.MaxDPI) {
		return siphonerr.New(siphonerr.Usage,
			"--dpi must be between %d and %d", pdf.MinDPI, pdf.MaxDPI)
	}
	if job.Flags["max-pixels"] && job.MaxPixels <= 0 {
		return siphonerr.New(siphonerr.Usage, "--max-pixels must be positive")
	}
	if job.Flags["quality"] && (job.Quality < 1 || job.Quality > 100) {
		return siphonerr.New(siphonerr.Usage, "--quality must be between 1 and 100")
	}
	if job.Flags["video-quality"] && (job.VideoQuality < 0 || job.VideoQuality > 51) {
		return siphonerr.New(siphonerr.Usage, "--video-quality must be between 0 and 51")
	}
	if job.Flags["audio-bitrate"] && (job.AudioBitrate < 8 || job.AudioBitrate > 512) {
		return siphonerr.New(siphonerr.Usage, "--audio-bitrate must be between 8 and 512 kbps")
	}
	if job.Flags["fps"] && (job.FPS <= 0 || job.FPS > 240) {
		return siphonerr.New(siphonerr.Usage, "--fps must be greater than 0 and at most 240")
	}
	if job.Flags["width"] && (job.Width < 1 || job.Width > 16384) {
		return siphonerr.New(siphonerr.Usage, "--width must be between 1 and 16384")
	}
	if job.Flags["audio-track"] && job.AudioTrack < 0 {
		return siphonerr.New(siphonerr.Usage, "--audio-track cannot be negative")
	}
	return nil
}
