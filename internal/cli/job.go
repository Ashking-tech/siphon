package cli

import (
	"time"

	"github.com/spf13/cobra"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/preflight"
	"siphon/internal/siphonerr"
)

type conversionFlags struct {
	cmd              *cobra.Command
	to               string
	mode             string
	force            bool
	yes              bool
	dropIncompatible bool
	transcodeAudio   bool
	allowNonstandard bool
	dropMetadata     bool
	transcode        bool
	bg               string
	quality          int
	bitrate          int
	dpi              int
	maxPixels        int64
	videoQuality     int
	audioBitrate     int
	fps              float64
	width            int
	audioTrack       int
	segmentDuration  time.Duration
	segmentTolerance time.Duration
	verify           string
	verifyFrames     int
	probeSegments    int
	skipProbe        bool
	dryRun           bool
}

var conversionFlagNames = []string{
	"to", "mode", "force", "yes", "drop-incompatible", "transcode-audio",
	"allow-nonstandard", "drop-metadata", "transcode", "bg", "quality",
	"bitrate", "dpi", "max-pixels", "video-quality", "audio-bitrate", "fps",
	"width", "audio-track", "segment-duration", "segment-tolerance", "verify",
	"verify-frames", "probe-segments", "skip-probe", "dry-run",
}

func (f *conversionFlags) bind(cmd *cobra.Command) {
	f.cmd = cmd
	fl := cmd.Flags()
	fl.StringVar(&f.to, "to", "", "target format (required for convert)")
	fl.StringVar(&f.mode, "mode", "auto", "auto|safe|relocate|reclaim")
	fl.BoolVar(&f.force, "force", false, "overwrite an existing target")
	fl.BoolVar(&f.yes, "yes", false, "skip the reclaim cost confirmation")
	fl.BoolVar(&f.dropIncompatible, "drop-incompatible", false, "skip tracks the target container cannot carry")
	fl.BoolVar(&f.transcodeAudio, "transcode-audio", false, "encode FLAC/DTS/TrueHD/PCM to AAC (safe only)")
	fl.BoolVar(&f.allowNonstandard, "allow-nonstandard", false, "permit VP9/Opus/FLAC in MP4")
	fl.BoolVar(&f.dropMetadata, "drop-metadata", false, "permit metadata loss")
	fl.BoolVar(&f.transcode, "transcode", false, "permit re-encoding for WebM ↔ MP4")
	fl.StringVar(&f.bg, "bg", "", "background colour for alpha stripping")
	fl.IntVar(&f.quality, "quality", 90, "image encoder quality")
	fl.IntVar(&f.bitrate, "bitrate", 0, "WAV CBR kbps instead of VBR")
	fl.IntVar(&f.dpi, "dpi", 0, "PDF render resolution (default 150)")
	fl.Int64Var(&f.maxPixels, "max-pixels", 0, "maximum rendered PDF pixels (default 100000000)")
	fl.IntVar(&f.videoQuality, "video-quality", 23, "video CRF (H.264 default 23, VP9 default 32)")
	fl.IntVar(&f.audioBitrate, "audio-bitrate", 0, "audio bitrate in kbps (0 uses the pair default)")
	fl.Float64Var(&f.fps, "fps", 0, "GIF output frame rate (0 uses the pair default)")
	fl.IntVar(&f.width, "width", 0, "maximum GIF width (0 uses 480)")
	fl.IntVar(&f.audioTrack, "audio-track", 0, "zero-based audio-track ordinal for video → MP3")
	fl.DurationVar(&f.segmentDuration, "segment-duration", 60*time.Second, "relocate segment window")
	fl.DurationVar(&f.segmentTolerance, "segment-tolerance", 500*time.Millisecond, "keyframe snap tolerance")
	fl.StringVar(&f.verify, "verify", "sampled", "sampled|structural|full")
	fl.IntVar(&f.verifyFrames, "verify-frames", 3, "frames decoded per fragment")
	fl.IntVar(&f.probeSegments, "probe-segments", 4, "segments sampled by the pre-flight stitch probe")
	fl.BoolVar(&f.skipProbe, "skip-probe", false, "skip the pre-flight stitch probe (unsafe)")
	fl.BoolVar(&f.dryRun, "dry-run", false, "alias for inspect")
}

func (f *conversionFlags) supplied() map[string]bool {
	out := make(map[string]bool)
	if f.cmd == nil {
		return out
	}
	for _, name := range conversionFlagNames {
		if f.cmd.Flags().Changed(name) {
			out[name] = true
		}
	}
	return out
}

func (f *conversionFlags) build(source string) (*core.Job, error) {
	if f.to == "" {
		return nil, siphonerr.New(siphonerr.Usage, "--to <format> is required")
	}
	target, ok := formats.Parse(f.to)
	if !ok {
		return nil, siphonerr.New(siphonerr.FormatUnsupported, "unknown target format %q", f.to)
	}
	src, pair, err := formats.PairForPath(source, target)
	if err != nil {
		return nil, err
	}
	mode, ok := formats.ParseMode(f.mode)
	if !ok {
		return nil, siphonerr.New(siphonerr.Usage, "unknown mode %q (auto|safe|relocate|reclaim)", f.mode)
	}
	if f.quality < 1 || f.quality > 100 {
		return nil, siphonerr.New(siphonerr.Usage, "--quality must be between 1 and 100")
	}
	if f.bitrate < 0 {
		return nil, siphonerr.New(siphonerr.Usage, "--bitrate cannot be negative")
	}
	if f.segmentDuration <= 0 {
		return nil, siphonerr.New(siphonerr.Usage, "--segment-duration must be positive")
	}
	if f.segmentTolerance < 0 {
		return nil, siphonerr.New(siphonerr.Usage, "--segment-tolerance cannot be negative")
	}
	if f.verifyFrames < 1 {
		return nil, siphonerr.New(siphonerr.Usage, "--verify-frames must be at least 1")
	}
	if f.probeSegments < 0 {
		return nil, siphonerr.New(siphonerr.Usage, "--probe-segments cannot be negative")
	}
	switch f.verify {
	case "structural", "sampled", "full":
	default:
		return nil, siphonerr.New(siphonerr.Usage, "--verify must be structural, sampled, or full")
	}

	timeout := globals.Timeout
	if timeout <= 0 {
		timeout = time.Hour
	}

	return &core.Job{
		SourcePath:       source,
		TargetPath:       preflight.TargetPath(source, target),
		Source:           src,
		Target:           target,
		Pair:             pair,
		Mode:             mode,
		Force:            f.force,
		Yes:              f.yes,
		DropIncompatible: f.dropIncompatible,
		TranscodeAudio:   f.transcodeAudio,
		AllowNonstandard: f.allowNonstandard,
		DropMetadata:     f.dropMetadata,
		Transcode:        f.transcode,
		AllowHardlinks:   globals.AllowHardlinks,
		AllowRoot:        globals.AllowRoot,
		Background:       f.bg,
		Quality:          f.quality,
		Bitrate:          f.bitrate,
		DPI:              f.dpi,
		MaxPixels:        f.maxPixels,
		VideoQuality:     f.videoQuality,
		VideoQualitySet:  f.cmd != nil && f.cmd.Flags().Changed("video-quality"),
		AudioBitrate:     f.audioBitrate,
		FPS:              f.fps,
		Width:            f.width,
		AudioTrack:       f.audioTrack,
		SegmentDuration:  f.segmentDuration,
		SegmentTolerance: f.segmentTolerance,
		Verify:           f.verify,
		VerifyFrames:     f.verifyFrames,
		ProbeSegments:    f.probeSegments,
		SkipProbe:        f.skipProbe,
		Timeout:          timeout,
		JSON:             globals.JSON,
		Flags:            f.supplied(),
	}, nil
}
