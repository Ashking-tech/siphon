package media

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"siphon/internal/siphonerr"
)

// Feature is one encoder or filter that a pair's ffmpeg invocation needs.
type Feature struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "encoder" or "filter"
}

// FeaturesFor is the ffmpeg build feature set for a media pair. Empty for
// remux-only pairs. The caller decides whether a transcode path is selected
// before enforcing these (spec §4.2).
func FeaturesFor(sourceTarget string, transcode bool) []Feature {
	switch sourceTarget {
	case "webm-mp4":
		if transcode {
			return []Feature{{"libx264", "encoder"}, {"aac", "encoder"}}
		}
	case "mp4-webm":
		if transcode {
			return []Feature{{"libvpx-vp9", "encoder"}, {"libopus", "encoder"}}
		}
	case "video-mp3":
		return []Feature{{"libmp3lame", "encoder"}}
	case "gif-mp4":
		return []Feature{{"libx264", "encoder"}}
	case "mp4-gif":
		return []Feature{{"gif", "encoder"}, {"palettegen", "filter"}, {"paletteuse", "filter"}}
	}
	return nil
}

// CheckFeatures verifies encoder/filter availability with the distro ffmpeg.
// A missing feature is E_DEPENDENCY with a build-specific hint.
func CheckFeatures(ctx context.Context, features []Feature) error {
	if len(features) == 0 {
		return nil
	}
	var missing []string
	var encoders, filters string
	for _, f := range features {
		var list string
		var err error
		switch f.Kind {
		case "encoder":
			if encoders == "" {
				encoders, err = ffmpegList(ctx, "-encoders")
				if err != nil {
					return err
				}
			}
			list = encoders
		case "filter":
			if filters == "" {
				filters, err = ffmpegList(ctx, "-filters")
				if err != nil {
					return err
				}
			}
			list = filters
		default:
			continue
		}
		if !listContainsFeature(list, f.Name) {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return siphonerr.New(siphonerr.Dependency,
		"ffmpeg is missing %s; install the full ffmpeg package (Fedora: sudo dnf install ffmpeg; Debian/Ubuntu: sudo apt install ffmpeg)",
		strings.Join(missing, ", ")).WithExit(2)
}

// InspectFeatures returns the missing features for doctor, without failing.
func InspectFeatures(ctx context.Context) []Feature {
	want := []Feature{
		{"libx264", "encoder"}, {"aac", "encoder"},
		{"libvpx-vp9", "encoder"}, {"libopus", "encoder"},
		{"libmp3lame", "encoder"}, {"gif", "encoder"},
		{"palettegen", "filter"}, {"paletteuse", "filter"},
	}
	var missing []Feature
	encoders, err := ffmpegList(ctx, "-encoders")
	if err != nil {
		return want
	}
	filters, err := ffmpegList(ctx, "-filters")
	if err != nil {
		return want
	}
	for _, f := range want {
		list := encoders
		if f.Kind == "filter" {
			list = filters
		}
		if !listContainsFeature(list, f.Name) {
			missing = append(missing, f)
		}
	}
	return missing
}

func ffmpegList(ctx context.Context, flag string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", flag)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", siphonerr.Wrap(siphonerr.Dependency, err, "cannot query ffmpeg %s", flag)
	}
	return out.String(), nil
}

// listContainsFeature matches a whole token. FFmpeg's listing is whitespace
// separated; the token may carry punctuation in some builds, so trim it.
func listContainsFeature(list, name string) bool {
	for _, line := range strings.Split(list, "\n") {
		for _, field := range strings.Fields(line) {
			field = strings.Trim(field, ".,;:()[]")
			if field == name {
				return true
			}
		}
	}
	return false
}

// FeatureSummary renders a missing-feature list for doctor.
func FeatureSummary(features []Feature) string {
	if len(features) == 0 {
		return ""
	}
	parts := make([]string, len(features))
	for i, f := range features {
		parts[i] = fmt.Sprintf("%s (%s)", f.Name, f.Kind)
	}
	return strings.Join(parts, ", ")
}
