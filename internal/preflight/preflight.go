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
	"siphon/internal/journal"
	"siphon/internal/media"
	"siphon/internal/platform"
	"siphon/internal/siphonerr"
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
	if _, err := media.Check(ctx, media.RequiredFor(job.Pair.Source, job.Pair.Target)); err != nil {
		return nil, refuse(err)
	}

	// 8/13/15. Probe and validate the container. MKV video needs a probe for
	// its space model; audio reclaim needs duration for the R ≤ 0.90 gate.
	var probe *media.Probe
	if job.Pair.Source == formats.MKV || job.Pair.Source == formats.WAV {
		probe, err = media.ProbeFile(ctx, job.SourcePath)
		if err != nil {
			return nil, refuse(err)
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
	needed, warnings, err := estimateFree(ctx, job, info.Size(), probe)
	if err != nil {
		return nil, refuse(err)
	}
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

	plan := &core.Plan{
		Source:             job.SourcePath,
		Target:             job.TargetPath,
		SourceFormat:       job.Source.String(),
		TargetFormat:       job.Target.String(),
		Mode:               string(job.Mode),
		Engine:             string(job.Engine),
		OutputKind:         job.Pair.OutputKind,
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
		plan.PlanSource = "probe"
	}
	return plan, nil
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

func estimateFree(ctx context.Context, job *core.Job, sourceSize int64, probe *media.Probe) (int64, []string, error) {
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
		if probe != nil && job.Mode == formats.ModeReclaim {
			estOut := int64(probe.DurationSeconds() * 190_000 / 8 * 1.05)
			if sourceSize > 0 && float64(estOut)/float64(sourceSize) > 0.90 {
				return 0, nil, siphonerr.New(siphonerr.InplaceInfeasible,
					"reclaim needs estimated MP3 ≤ 90%% of the WAV; this file estimates %.0f%% (use --mode safe)",
					100*float64(estOut)/float64(sourceSize))
			}
		}
		if job.Mode == formats.ModeReclaim {
			needed = sourceSize*101/100 + mib
		} else {
			needed = sourceSize*115/100 + 2*mib
		}
	case job.Pair.Source == formats.PNG || job.Pair.Source == formats.JPG || job.Pair.Source == formats.WEBP:
		// Conservative until the image engine can read dimensions: a lossless
		// transcode can inflate, and the spec says over-refusal is acceptable.
		needed = sourceSize*3 + 2*mib
		warnings = append(warnings, "image space estimate is conservative (lossless outputs can inflate)")
	default:
		needed = sourceSize*2 + 16*mib
	}
	if needed < 0 {
		needed = math.MaxInt64
	}
	return needed, warnings, nil
}

func segmentBytes(d time.Duration, bitrate int64) int64 {
	if bitrate <= 0 {
		return 64 * mib
	}
	return int64(d.Seconds()*float64(bitrate)/8) + 8*mib
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
	out := sourceSize*108/100 + 16*mib
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
	if p.Segments > 0 {
		fmt.Fprintf(&b, "  segments    %d × %s\n", p.Segments, p.SegmentDuration)
	}
	if p.Verify != "" {
		fmt.Fprintf(&b, "  verify      %s\n", p.Verify)
	}
	fmt.Fprintf(&b, "  free needed %s     you have %s\n", p.FreeNeededHuman, p.FreeAvailableHuman)
	fmt.Fprintf(&b, "  crash cost  %s\n", p.CrashCost)
	for _, w := range p.Warnings {
		fmt.Fprintf(&b, "  warning     %s\n", w)
	}
	return b.String()
}
