package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"siphon/internal/siphonerr"
)

// PacketCount is one stream's packet count as ffprobe sees it.
type PacketCount struct {
	Index     int    `json:"index"`
	CodecType string `json:"codec_type"`
	Packets   int64  `json:"-"`
}

// CountPackets runs ffprobe -count_packets and returns per-stream counts.
func CountPackets(ctx context.Context, path string) ([]PacketCount, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-count_packets",
		"-show_entries", "stream=index,codec_type,nb_read_packets",
		"-of", "json",
		path,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err,
			"ffprobe -count_packets failed on %s: %s", path, tail(stderr.Bytes()))
	}
	var raw struct {
		Streams []struct {
			Index     int    `json:"index"`
			CodecType string `json:"codec_type"`
			Packets   string `json:"nb_read_packets"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot parse packet counts")
	}
	out := make([]PacketCount, 0, len(raw.Streams))
	for _, s := range raw.Streams {
		n, err := strconv.ParseInt(s.Packets, 10, 64)
		if err != nil {
			return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err,
				"stream %d has an invalid packet count %q", s.Index, s.Packets)
		}
		out = append(out, PacketCount{Index: s.Index, CodecType: s.CodecType, Packets: n})
	}
	return out, nil
}

// DecodeCheck decodes the whole file with -xerror. This is the authoritative
// full gate for remux and relocate output.
func DecodeCheck(ctx context.Context, path string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-nostdin", "-v", "error", "-xerror",
		"-i", path,
		"-f", "null", "-",
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return siphonerr.Wrap(siphonerr.VerifyFailed, err,
			"full decode failed: %s", tail(stderr.Bytes()))
	}
	return nil
}

// ProbeAt probes one interval (ffprobe -read_intervals syntax, e.g. "50%+1")
// with extradata, for the constant-track-config check.
func ProbeAt(ctx context.Context, path, interval string) (*Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	args := []string{"-v", "error"}
	if interval != "" {
		args = append(args, "-read_intervals", interval)
	}
	args = append(args,
		"-show_format", "-show_streams", "-show_data",
		"-of", "json",
		path,
	)
	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err,
			"ffprobe interval %q failed on %s: %s", interval, path, tail(stderr.Bytes()))
	}
	var p Probe
	if err := json.Unmarshal(stdout.Bytes(), &p); err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot parse ffprobe interval output")
	}
	return &p, nil
}

// MissingTags returns source tag keys that the output did not carry. Tags are
// compared globally and per stream; outToSrc maps each output stream index to
// its source stream index (the -map order).
func MissingTags(src, out *Probe, outToSrc []int) []string {
	var missing []string
	for k, v := range src.Format.Tags {
		if ignorableTag(k) {
			continue
		}
		if out.Format.Tags[k] != v {
			missing = append(missing, "format:"+k)
		}
	}
	srcByIndex := map[int]Stream{}
	for _, s := range src.Streams {
		srcByIndex[s.Index] = s
	}
	for i, o := range out.Streams {
		if i >= len(outToSrc) {
			break
		}
		s, ok := srcByIndex[outToSrc[i]]
		if !ok {
			continue
		}
		for k, v := range s.Tags {
			if ignorableTag(k) {
				continue
			}
			if o.Tags[k] != v {
				missing = append(missing, fmt.Sprintf("stream:%d:%s", i, k))
			}
		}
	}
	return missing
}

// ignorableTag filters muxer bookkeeping, not user metadata. These keys are
// regenerated (or intentionally differ) in every container and would otherwise
// make every remux look like a metadata loss.
func ignorableTag(k string) bool {
	u := strings.ToUpper(k)
	switch u {
	case "ENCODER", "DURATION", "BPS", "NUMBER_OF_FRAMES", "NUMBER_OF_BYTES":
		return true
	}
	return strings.HasPrefix(u, "_STATISTICS_")
}

func tail(b []byte) string {
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
