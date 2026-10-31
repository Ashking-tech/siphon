package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"siphon/internal/siphonerr"
)

// Stream is the subset of ffprobe stream data Siphon uses.
type Stream struct {
	Index         int               `json:"index"`
	CodecName     string            `json:"codec_name"`
	CodecLongName string            `json:"codec_long_name"`
	CodecType     string            `json:"codec_type"`
	CodecTag      string            `json:"codec_tag_string"`
	Profile       string            `json:"profile"`
	Level         int               `json:"level"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	SampleRate    string            `json:"sample_rate"`
	BitRate       string            `json:"bit_rate"`
	TimeBase      string            `json:"time_base"`
	Duration      string            `json:"duration"`
	AvgFrameRate  string            `json:"avg_frame_rate"`
	RFrameRate    string            `json:"r_frame_rate"`
	NbFrames      string            `json:"nb_frames"`
	PixFmt        string            `json:"pix_fmt"`
	Extradata     string            `json:"extradata,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// ConfigKey is the decoder configuration that must stay constant for a
// segmented stream copy. It deliberately includes extradata: a mid-file
// SPS/PPS change is exactly the hazard the 3-point probe exists to catch.
func (s Stream) ConfigKey() string {
	return strings.Join([]string{
		s.CodecName, s.Profile, strconv.Itoa(s.Level),
		strconv.Itoa(s.Width), strconv.Itoa(s.Height),
		s.SampleRate, strconv.Itoa(s.Channels), s.ChannelLayout,
		s.TimeBase, s.Extradata,
	}, "|")
}

// FormatInfo is the subset of ffprobe container data Siphon uses.
type FormatInfo struct {
	Filename   string            `json:"filename"`
	FormatName string            `json:"format_name"`
	Duration   string            `json:"duration"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
	Tags       map[string]string `json:"tags,omitempty"`
}

// Chapter is one container chapter.
type Chapter struct {
	ID       int64             `json:"id"`
	TimeBase string            `json:"time_base"`
	Start    int64             `json:"start"`
	End      int64             `json:"end"`
	Tags     map[string]string `json:"tags,omitempty"`
}

// Probe is a single ffprobe pass.
type Probe struct {
	Streams  []Stream   `json:"streams"`
	Format   FormatInfo `json:"format"`
	Chapters []Chapter  `json:"chapters,omitempty"`
}

// ProbeFile runs ffprobe once and parses the JSON.
func ProbeFile(ctx context.Context, path string) (*Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot resolve %s", path)
	}
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_format", "-show_streams", "-show_chapters",
		"-of", "json",
		"-i", abs,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "ffprobe failed on %s: %s", path, msg)
	}
	var p Probe
	if err := json.Unmarshal(stdout.Bytes(), &p); err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot parse ffprobe output")
	}
	return &p, nil
}

// DurationSeconds returns the container duration in seconds.
func (p *Probe) DurationSeconds() float64 {
	return parseFloat(p.Format.Duration)
}

// Duration returns the container duration.
func (p *Probe) Duration() time.Duration {
	return time.Duration(p.DurationSeconds() * float64(time.Second))
}

// Size returns the container size in bytes.
func (p *Probe) Size() int64 {
	n, _ := strconv.ParseInt(p.Format.Size, 10, 64)
	return n
}

// TotalBitRate returns the container bitrate in bits per second, falling back
// to size/duration when the container omits it.
func (p *Probe) TotalBitRate() int64 {
	if n, err := strconv.ParseInt(p.Format.BitRate, 10, 64); err == nil && n > 0 {
		return n
	}
	d := p.DurationSeconds()
	if d > 0 {
		return int64(float64(p.Size()) * 8 / d)
	}
	return 0
}

// VideoStreams returns retained video streams.
func (p *Probe) VideoStreams() []Stream {
	var out []Stream
	for _, s := range p.Streams {
		if s.CodecType == "video" {
			out = append(out, s)
		}
	}
	return out
}

// AudioStreams returns audio streams.
func (p *Probe) AudioStreams() []Stream {
	var out []Stream
	for _, s := range p.Streams {
		if s.CodecType == "audio" {
			out = append(out, s)
		}
	}
	return out
}

// FrameRate parses avg_frame_rate (falling back to r_frame_rate) for a video
// stream. It returns zero when ffprobe did not report a usable rate.
func (s Stream) FrameRate() float64 {
	for _, v := range []string{s.AvgFrameRate, s.RFrameRate} {
		if f := parseRatio(v); f > 0 {
			return f
		}
	}
	return 0
}

// FrameCount parses nb_frames when the demuxer exposes it.
func (s Stream) FrameCount() int64 {
	if s.NbFrames == "" || s.NbFrames == "N/A" {
		return 0
	}
	n, err := strconv.ParseInt(s.NbFrames, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseRatio(v string) float64 {
	if v == "" || v == "0/0" {
		return 0
	}
	if i := strings.IndexByte(v, '/'); i >= 0 {
		num, err1 := strconv.ParseFloat(v[:i], 64)
		den, err2 := strconv.ParseFloat(v[i+1:], 64)
		if err1 != nil || err2 != nil || den == 0 {
			return 0
		}
		return num / den
	}
	f, _ := strconv.ParseFloat(v, 64)
	return f
}

// Summary is a short human description for inspect/doctor.
func (p *Probe) Summary() string {
	var v, a int
	for _, s := range p.Streams {
		switch s.CodecType {
		case "video":
			v++
		case "audio":
			a++
		}
	}
	return fmt.Sprintf("%s, %.0fs, %d video / %d audio stream(s)",
		p.Format.FormatName, p.DurationSeconds(), v, a)
}

func parseFloat(s string) float64 {
	n, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return n
}
