package media

import (
	"fmt"
	"image/color"
	"path/filepath"
	"strconv"

	"siphon/internal/imaging"
	"siphon/internal/siphonerr"
)

// FFmpegColor converts a Siphon --bg value into an ffmpeg filter colour. An
// empty value means black; GIF→MP4 only uses the flag when the source is
// transparent.
func FFmpegColor(name string) (string, error) {
	if name == "" {
		return "black", nil
	}
	c, err := imaging.ParseBackground(name)
	if err != nil {
		return "", err
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("0x%02X%02X%02X", r>>8, g>>8, b>>8), nil
}

// BuildFFmpegArgs constructs the complete argument vector for one media pair.
// It never uses a shell and every file argument is absolute. The stream map is
// always explicit; there is no wildcard copy (spec §9.1, §10.1).
func BuildFFmpegArgs(pp *PairPlan, sourcePath, tmpPath, background string) ([]string, error) {
	if pp == nil {
		return nil, siphonerr.New(siphonerr.Internal, "nil media plan")
	}
	src, err := absPath(sourcePath)
	if err != nil {
		return nil, err
	}
	out, err := absPath(tmpPath)
	if err != nil {
		return nil, err
	}
	args := []string{
		"-nostdin", "-hide_banner", "-loglevel", "error", "-xerror", "-y",
		"-nostats", "-progress", "pipe:1",
		"-i", src,
	}

	switch {
	case !pp.Transcode && pp.TargetContainer != "mp3":
		args = append(args, remuxMapArgs(pp)...)
		args = append(args, "-c", "copy", "-map_metadata", "0")
		if pp.Chapters && pp.TargetContainer != "mov" {
			args = append(args, "-map_chapters", "0")
		} else {
			args = append(args, "-map_chapters", "-1")
		}
		if pp.Strict {
			args = append(args, "-strict", "-2")
		}
		args = append(args, "-dn", "-sn")
		switch pp.TargetContainer {
		case "mkv":
			args = append(args, "-f", "matroska")
		case "mov":
			args = append(args, "-movflags", "+faststart", "-f", "mov")
		case "mp4":
			args = append(args, "-movflags", "+faststart", "-f", "mp4")
		case "webm":
			args = append(args, "-f", "webm")
		default:
			return nil, siphonerr.New(siphonerr.Internal,
				"no remux format for %s", pp.TargetContainer)
		}
		args = append(args, out)
		return args, nil

	case pp.TargetContainer == "mp3":
		args = append(args, "-map", fmt.Sprintf("0:a:%d", pp.AudioTrack), "-vn")
		args = append(args, "-c:a", "libmp3lame")
		if pp.AudioBitrate > 0 {
			args = append(args, "-b:a", fmt.Sprintf("%dk", pp.AudioBitrate))
		} else {
			args = append(args, "-q:a", "2")
		}
		args = append(args, "-map_metadata", "0", "-id3v2_version", "3", "-f", "mp3", out)
		return args, nil

	case pp.SourceContainer == "gif" && pp.TargetContainer == "mp4":
		bg, err := FFmpegColor(background)
		if err != nil {
			return nil, err
		}
		fps := pp.FPS
		if fps <= 0 {
			fps = 30
		}
		vf := fmt.Sprintf(
			"fps=%s,pad=ceil(iw/2)*2:ceil(ih/2)*2:0:0:color=%s,format=yuv420p",
			formatFloat(fps), bg)
		args = append(args,
			"-map", "0:v:0", "-an",
			"-vf", vf,
			"-c:v", "libx264", "-preset", "medium",
			"-crf", strconv.Itoa(effectiveCRF(pp.VideoQuality, 23)),
			"-movflags", "+faststart", "-map_metadata", "-1",
			"-f", "mp4", out)
		return args, nil

	case pp.SourceContainer == "mp4" && pp.TargetContainer == "gif":
		fps := pp.FPS
		if fps <= 0 {
			fps = 12
		}
		width := pp.Width
		if width <= 0 {
			width = 480
		}
		vf := fmt.Sprintf(
			"fps=%s,scale='min(%d,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse",
			formatFloat(fps), width)
		args = append(args,
			"-map", "0:v:0", "-an",
			"-vf", vf,
			"-map_metadata", "-1",
			"-loop", "0", "-f", "gif", out)
		return args, nil

	case pp.Transcode && pp.TargetContainer == "mp4":
		args = append(args, "-map", "0:v:0")
		if pp.AudioIndex >= 0 {
			args = append(args, "-map", "0:a:0")
		}
		args = append(args,
			"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2:0:0:color=black",
			"-c:v", "libx264", "-preset", "medium",
			"-crf", strconv.Itoa(effectiveCRF(pp.VideoQuality, 23)),
			"-pix_fmt", "yuv420p")
		if pp.AudioIndex >= 0 {
			args = append(args, "-c:a", "aac", "-b:a",
				fmt.Sprintf("%dk", effectiveAudioBitrate(pp.AudioBitrate, 128)))
		} else {
			args = append(args, "-an")
		}
		args = append(args, "-map_metadata", "0")
		if pp.Chapters {
			args = append(args, "-map_chapters", "0")
		} else {
			args = append(args, "-map_chapters", "-1")
		}
		args = append(args, "-movflags", "+faststart", "-sn", "-dn",
			"-f", "mp4", out)
		return args, nil

	case pp.Transcode && pp.TargetContainer == "webm":
		args = append(args, "-map", "0:v:0")
		if pp.AudioIndex >= 0 {
			args = append(args, "-map", "0:a:0")
		}
		args = append(args,
			"-vf", "pad=ceil(iw/2)*2:ceil(ih/2)*2:0:0:color=black",
			"-c:v", "libvpx-vp9",
			"-crf", strconv.Itoa(effectiveCRF(pp.VideoQuality, 32)),
			"-b:v", "0", "-row-mt", "1", "-pix_fmt", "yuv420p")
		if pp.AudioIndex >= 0 {
			args = append(args, "-c:a", "libopus", "-b:a",
				fmt.Sprintf("%dk", effectiveAudioBitrate(pp.AudioBitrate, 96)))
		} else {
			args = append(args, "-an")
		}
		args = append(args, "-map_metadata", "0")
		if pp.Chapters {
			args = append(args, "-map_chapters", "0")
		} else {
			args = append(args, "-map_chapters", "-1")
		}
		args = append(args, "-sn", "-dn", "-f", "webm", out)
		return args, nil
	}
	return nil, siphonerr.New(siphonerr.Internal,
		"no ffmpeg argument builder for %s → %s (transcode=%v)",
		pp.SourceContainer, pp.TargetContainer, pp.Transcode)
}

func remuxMapArgs(pp *PairPlan) []string {
	args := make([]string, 0, len(pp.Streams)*2)
	for _, s := range pp.Streams {
		if s.Action != ActionCopy {
			continue
		}
		args = append(args, "-map", fmt.Sprintf("0:%d", s.Index))
	}
	return args
}

func effectiveCRF(v, def int) int {
	if v < 0 {
		return def
	}
	if v > 51 {
		return 51
	}
	return v
}

func effectiveAudioBitrate(v, def int) int {
	if v <= 0 {
		return def
	}
	return v
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func absPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot resolve %s", path)
	}
	return abs, nil
}

// ColorFromRGBA is exposed for tests that assert the converted filter colour.
func ColorFromRGBA(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("0x%02X%02X%02X", r>>8, g>>8, b>>8)
}
