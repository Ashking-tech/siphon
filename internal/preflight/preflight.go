// Package preflight is the ordered refusal gate from spec §4.9. It either
// produces a Plan or refuses with a structured error before any write.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"siphon/internal/core"
	"siphon/internal/formats"
	"siphon/internal/imaging"
	"siphon/internal/journal"
	"siphon/internal/media"
	"siphon/internal/platform"
	"siphon/internal/siphonerr"
	"siphon/internal/stitch"
	"siphon/internal/wav"
)

const (
	mib = 1 << 20
	gib = 1 << 30
)

// Run executes the gate in spec order and returns a plan on success.
func Run(ctx context.Context, job *core.Job) (*core.Plan, error) {
	// 1. Source exists, regular, not a symlink.
	info, err := platform.IsRegularNoSymlink(job.SourcePath)
	if err != nil {
		return nil, refuse(err)
	}

	// 2. Format pair and distinct paths.
	if job.Source == job.Target {
		return nil, refuse(siphonerr.New(siphonerr.FormatUnsupported,
			"source and target formats are both %s", job.Source))
	}
	if filepath.Clean(job.SourcePath) == filepath.Clean(job.TargetPath) {
		return nil, refuse(siphonerr.New(siphonerr.FormatUnsupported,
			"target path is the source path"))
	}

	// 3. Mode is available for this pair.
	mode, err := formats.ResolveMode(job.Pair, job.Mode)
	if err != nil {
		return nil, refuse(err)
	}
	job.Mode = mode
	job.Engine = formats.EngineFor(job.Pair, mode)

	// 4. Hardlinks.
	var st unix.Stat_t
	if err := unix.Stat(job.SourcePath, &st); err != nil {
		return nil, refuse(siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot stat source"))
	}
	if st.Nlink > 1 && !job.AllowHardlinks {
		return nil, refuse(siphonerr.New(siphonerr.Multilink,
			"%s has %d hard links; replacing it would split the link set (use --allow-hardlinks)",
			job.SourcePath, st.Nlink))
	}

	// 5. Write permission on the directory and the source.
	dir := filepath.Dir(job.SourcePath)
	if err := platform.IsDirWritable(dir); err != nil {
		return nil, refuse(err)
	}
	if err := platform.IsWritable(job.SourcePath); err != nil {
		return nil, refuse(err)
	}

	// 6. Target must not exist unless --force.
	if _, err := os.Lstat(job.TargetPath); err == nil {
		if !job.Force {
			return nil, refuse(siphonerr.New(siphonerr.TargetExists,
				"%s already exists (use --force to replace it)", job.TargetPath))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, refuse(siphonerr.Wrap(siphonerr.TargetExists, err, "cannot check target"))
	}

	// 7. Same filesystem. The target is a sibling of the source by
	// construction, so compare the source with its directory.
	same, err := platform.SameDevice(job.SourcePath, dir)
	if err != nil {
		return nil, refuse(siphonerr.Wrap(siphonerr.CrossDevice, err, "cannot stat target directory"))
	}
	if !same {
		return nil, refuse(siphonerr.New(siphonerr.CrossDevice,
			"%s and %s are on different filesystems", job.SourcePath, job.TargetPath))
	}

	// 17 (early, because probing needs the tools). Dependencies.
	toolVersions, err := media.Check(ctx, media.RequiredFor(job.Pair.Source, job.Pair.Target))
	if err != nil {
		return nil, refuse(err)
	}
	job.Tools = toolVersions

	// 8/13/15. Probe and validate the container. MKV video needs an ffprobe
	// pass for its space model; WAV is read from its own header, which is
	// also what the reclaim cursor and the R ≤ 0.90 gate need.
	var probe *media.Probe
	var trackPlan *media.TrackPlan
	var mkvWarnings []string
	if job.Pair.Source == formats.MKV {
		probe, err = media.ProbeFile(ctx, job.SourcePath)
		if err != nil {
			return nil, refuse(err)
		}
		// 8. Codec/track compatibility (spec §4.5.3).
		trackPlan, err = media.BuildTrackPlan(probe, media.TrackOptions{
			DropIncompatible: job.DropIncompatible,
			TranscodeAudio:   job.TranscodeAudio,
			AllowNonstandard: job.AllowNonstandard,
			Relocate:         job.Mode == formats.ModeRelocate,
		})
		if err != nil {
			return nil, refuse(err)
		}
		// Relocate's fixed -map cannot drop A/V streams, so a policy that
		// would drop one is not an in-place option; safe mode can.
		if job.Mode == formats.ModeRelocate {
			for _, d := range trackPlan.Dropped {
				if strings.HasPrefix(d, "video:") || strings.HasPrefix(d, "audio:") {
					return nil, refuse(siphonerr.New(siphonerr.InplaceInfeasible,
						"relocate cannot drop %s during segmented extraction; use --mode safe", d))
				}
			}
		}
		// 9. Metadata classes that cannot be carried.
		if !job.DropMetadata && (trackPlan.Attachments || trackPlan.Chapters) {
			var lost []string
			if trackPlan.Attachments {
				lost = append(lost, "attachments")
			}
			if trackPlan.Chapters {
				lost = append(lost, "chapters")
			}
			return nil, refuse(siphonerr.New(siphonerr.MetadataDrop,
				"%s carries %s, which cannot be carried into MP4; pass --drop-metadata to accept this",
				job.SourcePath, strings.Join(lost, " and ")))
		}
		mkvWarnings = append(mkvWarnings, trackPlan.Warnings...)
		mkvWarnings = append(mkvWarnings, trackPlan.MetadataWarnings(job.DropMetadata)...)

		// 13 (part). Relocate is remux-only and must not grow the file.
		if job.Mode == formats.ModeRelocate {
			if est := sumStreamBits(probe) / 8; est > 0 && est > info.Size()*103/100 {
				return nil, refuse(siphonerr.New(siphonerr.InplaceInfeasible,
					"relocate needs estimated output ≤ 103%% of the source; this file estimates %.0f%% (use --mode safe)",
					100*float64(est)/float64(info.Size())))
			}
		}
	}
	var wavInfo *wav.Info
	if job.Pair.Source == formats.WAV {
		wavInfo, err = wav.Parse(job.SourcePath)
		if err != nil {
			return nil, refuse(err)
		}
		if wavInfo.Channels > 2 {
			return nil, refuse(siphonerr.New(siphonerr.FormatUnsupported,
				"%s has %d channels; Siphon converts mono and stereo WAV only",
				job.SourcePath, wavInfo.Channels))
		}
		if !job.DropMetadata && wavInfo.HasMetadata() {
			return nil, refuse(siphonerr.New(siphonerr.MetadataDrop,
				"%s carries WAV metadata (%s) that cannot be put into an MP3; pass --drop-metadata to accept this",
				job.SourcePath, strings.Join(wavInfo.MetadataNames(), ", ")))
		}
	}

	// 14. No stale journal.
	stem := strings.TrimSuffix(filepath.Base(job.SourcePath), filepath.Ext(job.SourcePath))
	if paths, err := journal.Find(dir, stem); err == nil && len(paths) > 0 {
		return nil, refuse(siphonerr.New(siphonerr.StaleJournal,
			"a Siphon journal exists for %s; run `siphon recover %s`", job.SourcePath, job.SourcePath))
	}

	// 15. Magic bytes.
	if err := platform.ValidateMagic(job.SourcePath, job.Pair.Source); err != nil {
		return nil, refuse(err)
	}

	// 8/9. Image probe: decode once so the alpha policy, the free-space
	// estimate, and the metadata warnings are all decided before any write.
	var imageInfo *imaging.Info
	if isImage(job.Pair.Source) {
		probeCtx := ctx
		if job.Timeout > 0 {
			var cancel context.CancelFunc
			probeCtx, cancel = context.WithTimeout(ctx, job.Timeout)
			defer cancel()
		}
		imageInfo, err = imaging.Probe(probeCtx, job.SourcePath, job.Pair.Source)
		if err != nil {
			return nil, refuse(err)
		}
		if job.Target == formats.JPG && imageInfo.HasAlpha && job.Background == "" {
			return nil, refuse(siphonerr.New(siphonerr.AlphaUnsupported,
				"%s has transparency and JPEG cannot carry it; pass --bg <colour>", job.SourcePath))
		}
		if job.Background != "" {
			if _, err := imaging.ParseBackground(job.Background); err != nil {
				return nil, refuse(err)
			}
		}
	}

	// 16. Immutable attribute.
	if immutable, err := platform.HasImmutable(job.SourcePath); err != nil {
		return nil, refuse(siphonerr.Wrap(siphonerr.Permission, err, "cannot read file flags"))
	} else if immutable {
		return nil, refuse(siphonerr.New(siphonerr.Permission, "%s is immutable", job.SourcePath))
	}

	// 18. Root.
	if os.Geteuid() == 0 && !job.AllowRoot {
		return nil, refuse(siphonerr.New(siphonerr.RootDenied,
			"refusing to run as root; pass --allow-root only if you are certain"))
	}

	// 20. Reclaim confirmation when stdin is not a TTY.
	if job.Mode == formats.ModeReclaim && !job.Yes && !stdinIsTTY() {
		return nil, refuse(siphonerr.New(siphonerr.ConfirmationNeeded,
			"reclaim can splice the audio on a crash; re-run with --yes to accept"))
	}

	// 10. Free space (and 11/12 where applicable).
	needed, warnings, err := estimateFree(ctx, job, info.Size(), probe, wavInfo, imageInfo)
	if err != nil {
		return nil, refuse(err)
	}
	if imageInfo != nil {
		warnings = append(warnings, imageInfo.Warnings...)
	}
	warnings = append(warnings, mkvWarnings...)
	free, err := platform.FreeSpace(dir)
	if err != nil {
		return nil, refuse(siphonerr.Wrap(siphonerr.NoSpace, err, "cannot read free space"))
	}
	if free < needed {
		msg := fmt.Sprintf("%s needs %s free; %s has %s available",
			job.SourcePath, core.HumanBytes(needed), dir, core.HumanBytes(free))
		if job.Mode == formats.ModeRelocate && probe != nil {
			if max := maxSegmentDuration(free, probe.TotalBitRate()); max > 0 {
				msg += fmt.Sprintf(" (largest viable --segment-duration: %s)", max.Round(time.Second))
			}
		}
		return nil, refuse(siphonerr.New(siphonerr.NoSpace, "%s", msg))
	}

	// 12. Sparse support for relocate.
	if job.Mode == formats.ModeRelocate {
		reserve := reserveSize(info.Size())
		if err := platform.SparseProbe(dir, reserve); err != nil {
			return nil, refuse(err)
		}
	}

	// 13. Constant track config and enough keyframe-aligned boundaries.
	var stitchPlan *stitch.Plan
	if job.Mode == formats.ModeRelocate {
		if err := checkRelocateConfig(ctx, job.SourcePath, trackPlan); err != nil {
			return nil, refuse(err)
		}
		stitchPlan, err = stitch.BuildPlan(ctx, job.SourcePath, stitch.Options{
			SegmentDuration:  job.SegmentDuration,
			SegmentTolerance: job.SegmentTolerance,
			VerifyFrames:     job.VerifyFrames,
		}, nil)
		if err != nil {
			return nil, refuse(err)
		}
		if len(stitchPlan.Segments) < 3 {
			return nil, refuse(siphonerr.New(siphonerr.InplaceInfeasible,
				"relocate needs at least 3 keyframe-aligned segments; %s yields %d (use --mode safe)",
				job.SourcePath, len(stitchPlan.Segments)))
		}
	}

	plan := &core.Plan{
		Source:             job.SourcePath,
		Target:             job.TargetPath,
		SourceFormat:       job.Source.String(),
		TargetFormat:       job.Target.String(),
		Mode:               string(job.Mode),
		Engine:             string(job.Engine),
		OutputKind:         job.Pair.OutputKindFor(job.Mode),
		Lossy:              job.Pair.Lossy,
		SourceSize:         info.Size(),
		FreeNeeded:         needed,
		FreeNeededHuman:    core.HumanBytes(needed),
		FreeAvailable:      free,
		FreeAvailableHuman: core.HumanBytes(free),
		CrashCost:          crashCost(job),
		Warnings:           warnings,
	}
	if job.Mode == formats.ModeRelocate {
		plan.SegmentDuration = job.SegmentDuration.String()
		plan.Verify = job.Verify
		if stitchPlan != nil {
			plan.Segments = len(stitchPlan.Segments)
			plan.PlanSource = stitchPlan.PlanSource
		}
	}
	if trackPlan != nil {
		plan.Streams = trackPlan.Describe()
		plan.Dropped = trackPlan.Dropped
	}
	if imageInfo != nil {
		plan.ImageWidth = imageInfo.Width
		plan.ImageHeight = imageInfo.Height
		plan.HasAlpha = imageInfo.HasAlpha
		if !job.DropMetadata && !imageInfo.Metadata.Empty() {
			plan.Metadata = imageInfo.Metadata.Classes()
		}
	}
	if wavInfo != nil {
		plan.AudioChannels = wavInfo.Channels
		plan.AudioSampleRate = wavInfo.SampleRate
		plan.AudioDurationMS = wavInfo.Duration.Milliseconds()
		if wavInfo.HasMetadata() {
			plan.AudioTags = wavInfo.MetadataNames()
		}
	}
	return plan, nil
}

// checkRelocateConfig probes the head, middle, and tail of the source and
// requires every retained stream's decoder configuration to be byte-identical.
// A mid-file SPS/PPS or audio parameter change would make a segment copied
// behind the head's cached header undecodable, so relocate must refuse it.
func checkRelocateConfig(ctx context.Context, path string, tp *media.TrackPlan) error {
	return checkRelocateConfigWith(ctx, path, tp, media.ProbeAt)
}

// checkRelocateConfigWith is the testable form of the three-point track
// configuration probe. probeAt is injected so the refusal logic can be tested
// without manufacturing a file whose SPS changes mid-stream.
func checkRelocateConfigWith(ctx context.Context, path string, tp *media.TrackPlan,
	probeAt func(context.Context, string, string) (*media.Probe, error)) error {
	if tp == nil {
		return nil
	}
	intervals := []string{"0%+1", "50%+1", "90%+1"}
	probes := make([]*media.Probe, 0, len(intervals))
	for _, iv := range intervals {
		p, err := probeAt(ctx, path, iv)
		if err != nil {
			return err
		}
		probes = append(probes, p)
	}
	for _, t := range tp.Tracks {
		var key string
		for i, p := range probes {
			var s *media.Stream
			for j := range p.Streams {
				if p.Streams[j].Index == t.Index {
					s = &p.Streams[j]
					break
				}
			}
			if s == nil {
				return siphonerr.New(siphonerr.InplaceInfeasible,
					"stream %d is missing at the %s probe; relocate needs a constant track config (use --mode safe)",
					t.Index, intervals[i])
			}
			k := s.ConfigKey()
			if i == 0 {
				key = k
				continue
			}
			if k != key {
				return siphonerr.New(siphonerr.InplaceInfeasible,
					"stream %d changes configuration between %s and %s; relocate needs a constant track config (use --mode safe)",
					t.Index, intervals[0], intervals[i])
			}
		}
	}
	return nil
}

func refuse(err error) error {
	var se *siphonerr.Error
	if errors.As(err, &se) {
		se.Untouched(true)
		if se.Code == siphonerr.Dependency || se.Code == siphonerr.DependencyOutdated {
			se.Exit = 2
		}
		return se
	}
	return siphonerr.Wrap(siphonerr.Internal, err, "pre-flight failed").Untouched(true)
}

// TargetPath derives the in-place sibling target.
func TargetPath(source string, target formats.Format) string {
	ext := filepath.Ext(source)
	stem := strings.TrimSuffix(source, ext)
	return stem + "." + target.Ext()
}

// Stem returns the basename without its extension.
func Stem(path string) string {
	base := filepath.Base(path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func estimateFree(ctx context.Context, job *core.Job, sourceSize int64, probe *media.Probe, wavInfo *wav.Info, imageInfo *imaging.Info) (int64, []string, error) {
	var warnings []string
	var needed int64

	switch {
	case job.Pair.Source == formats.MKV:
		if probe == nil {
			needed = sourceSize*2 + 64*mib
			break
		}
		if job.Mode == formats.ModeRelocate {
			needed = segmentBytes(job.SegmentDuration, probe.TotalBitRate()) + 16*mib
			warnings = append(warnings,
				"output will be a fragmented MP4; use --mode safe if a player rejects it")
		} else {
			needed = sourceSize*103/100 + sumStreamBits(probe)*11/10/8 + 4*mib
			warnings = append(warnings,
				"safe mode builds a classic MP4 and needs about 2× the source space")
		}
	case job.Pair.Source == formats.WAV:
		if wavInfo == nil {
			needed = sourceSize*2 + 2*mib
			break
		}
		estOut := int64(wavInfo.Duration.Seconds() * float64(wavCeilingBitrate(job)) / 8)
		switch job.Mode {
		case formats.ModeReclaim:
			// 11. The forward cursor only holds if the output stays behind
			// the read position with margin: R ≤ 0.90 (spec §3.4).
			if sourceSize > 0 && float64(estOut) > 0.90*float64(sourceSize) {
				return 0, nil, siphonerr.New(siphonerr.InplaceInfeasible,
					"reclaim needs estimated MP3 ≤ 90%% of the WAV; this file estimates %.0f%% (use --mode safe)",
					100*float64(estOut)/float64(sourceSize))
			}
			needed = sourceSize*101/100 + mib
		default:
			// spec §7.1: source + duration × ceiling_bitrate / 8 × 1.05
			needed = sourceSize + estOut + estOut/20 + mib
		}
		if job.DropMetadata && wavInfo.HasMetadata() {
			warnings = append(warnings,
				fmt.Sprintf("--drop-metadata set: WAV metadata (%s) will not be carried",
					strings.Join(wavInfo.MetadataNames(), ", ")))
		}
		warnings = append(warnings, wavInfo.Warnings...)
	case isImage(job.Pair.Source):
		if imageInfo != nil {
			// spec §7.1: source + max(2 × source, decoded_raw × 1.5) + 2 MiB
			decoded := decodedRaw(imageInfo)
			needed = sourceSize + maxInt64(2*sourceSize, decoded+decoded/2) + 2*mib
		} else {
			needed = sourceSize*3 + 2*mib
		}
		if job.DropMetadata && imageInfo != nil && !imageInfo.Metadata.Empty() {
			warnings = append(warnings, "--drop-metadata set: ICC/EXIF/XMP metadata will not be carried")
		}
	default:
		needed = sourceSize*2 + 16*mib
	}
	if needed < 0 {
		needed = math.MaxInt64
	}
	return needed, warnings, nil
}

// wavCeilingBitrate is the pessimistic output bitrate for the space estimate
// and the reclaim ratio: the LAME -V 2 ceiling of 190 kbps, or the requested
// CBR bitrate.
func wavCeilingBitrate(job *core.Job) int64 {
	if job.Bitrate > 0 {
		return int64(job.Bitrate) * 1000
	}
	return 190_000
}

func segmentBytes(d time.Duration, bitrate int64) int64 {
	if bitrate <= 0 {
		return 64 * mib
	}
	return int64(d.Seconds()*float64(bitrate)/8) + 8*mib
}

// isImage reports whether the source goes through the AtomicCopy engine.
func isImage(f formats.Format) bool {
	return f == formats.PNG || f == formats.JPG || f == formats.WEBP
}

// decodedRaw estimates the decoded bitmap size, guarding against absurd
// headers: four bytes per pixel.
func decodedRaw(info *imaging.Info) int64 {
	if info == nil || info.Width <= 0 || info.Height <= 0 ||
		info.Width > 1<<20 || info.Height > 1<<20 {
		return 0
	}
	return int64(info.Width) * int64(info.Height) * 4
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func sumStreamBits(p *media.Probe) int64 {
	d := p.DurationSeconds()
	var total int64
	for _, s := range p.Streams {
		var br int64
		fmt.Sscanf(s.BitRate, "%d", &br)
		if br == 0 {
			continue
		}
		total += int64(float64(br) * d)
	}
	return total
}

func reserveSize(sourceSize int64) int64 {
	out := (sourceSize*105+99)/100 + 16*mib
	if out < sourceSize {
		out = sourceSize
	}
	return out
}

func maxSegmentDuration(free int64, bitrate int64) time.Duration {
	if bitrate <= 0 || free <= 16*mib {
		return 0
	}
	seconds := float64(free-16*mib) * 8 / float64(bitrate)
	return time.Duration(seconds * float64(time.Second))
}

func stdinIsTTY() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func crashCost(job *core.Job) string {
	switch job.Mode {
	case formats.ModeRelocate:
		return "source stays a playable MKV; recover resumes exactly where it stopped"
	case formats.ModeReclaim:
		return "source head becomes a valid MP3 prefix; recover splices at the last frame (~26 ms seam)"
	default:
		return "source untouched; a crash leaves only a temp file, which the next run reaps"
	}
}

// FormatPlan renders the human `inspect` output.
func FormatPlan(p *core.Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s → %s\n", p.Source, p.Target)
	fmt.Fprintf(&b, "  mode        %s\n", p.Mode)
	fmt.Fprintf(&b, "  output      %s\n", p.OutputKind)
	if p.PlanSource != "" {
		fmt.Fprintf(&b, "  plan        %s\n", p.PlanSource)
	}
	if len(p.Streams) > 0 {
		fmt.Fprintf(&b, "  streams     %s\n", strings.Join(p.Streams, ", "))
	}
	if len(p.Dropped) > 0 {
		fmt.Fprintf(&b, "  dropping    %s\n", strings.Join(p.Dropped, ", "))
	}
	if p.Segments > 0 {
		fmt.Fprintf(&b, "  segments    %d × %s\n", p.Segments, p.SegmentDuration)
	}
	if p.Verify != "" {
		fmt.Fprintf(&b, "  verify      %s\n", p.Verify)
	}
	if p.ImageWidth > 0 {
		alpha := ""
		if p.HasAlpha {
			alpha = " (has alpha)"
		}
		fmt.Fprintf(&b, "  image       %dx%d%s\n", p.ImageWidth, p.ImageHeight, alpha)
	}
	if p.AudioChannels > 0 {
		fmt.Fprintf(&b, "  audio       %d Hz, %d ch, %s\n",
			p.AudioSampleRate, p.AudioChannels, humanDuration(time.Duration(p.AudioDurationMS)*time.Millisecond))
	}
	if len(p.Metadata) > 0 {
		fmt.Fprintf(&b, "  metadata    %s\n", strings.Join(p.Metadata, ", "))
	}
	if len(p.AudioTags) > 0 {
		fmt.Fprintf(&b, "  tags        %s (dropped)\n", strings.Join(p.AudioTags, ", "))
	}
	fmt.Fprintf(&b, "  free needed %s     you have %s\n", p.FreeNeededHuman, p.FreeAvailableHuman)
	fmt.Fprintf(&b, "  crash cost  %s\n", p.CrashCost)
	for _, w := range p.Warnings {
		fmt.Fprintf(&b, "  warning     %s\n", w)
	}
	return b.String()
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	return fmt.Sprintf("%dm%02ds", m, s)
}
