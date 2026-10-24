package sliding

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"strconv"
	"time"

	"siphon/internal/siphonerr"
	"siphon/internal/wav"
)

// verifyExpect is what the source says the MP3 must decode to.
type verifyExpect struct {
	Channels   int
	SampleRate int // input rate; LAME may resample this down to MP3 rates
	Duration   time.Duration
	// Seam reports a splice-resume, whose join may carry one damaged frame.
	// The full gate then tolerates one extra frame of duration.
	Seam bool
}

func expectForInfo(info *wav.Info, seam bool) verifyExpect {
	return verifyExpect{
		Channels:   info.Channels,
		SampleRate: info.SampleRate,
		Duration:   info.Duration,
		Seam:       seam,
	}
}

// verifyMP3File runs the full gate (spec §4.7) on a path.
func verifyMP3File(ctx context.Context, path string, want verifyExpect) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot open the encoded MP3")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return 0, siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot stat the encoded MP3")
	}
	return verifyMP3(ctx, f, 0, info.Size(), want)
}

// verifyMP3 is the full gate for an MP3 stream: structure, decodability, and
// a decoded-duration check against the source. The stream is read through a
// section of r so reclaim can verify the prefix it has written without
// letting the verifier see the WAV bytes still behind it.
func verifyMP3(ctx context.Context, r io.ReaderAt, off, length int64, want verifyExpect) (int, error) {
	if length <= 0 {
		return 0, siphonerr.New(siphonerr.VerifyFailed, "the encoded MP3 is empty")
	}

	rate, channels, err := probeMP3(ctx, io.NewSectionReader(r, off, length))
	if err != nil {
		return 0, err
	}
	if channels != want.Channels {
		return 0, siphonerr.New(siphonerr.VerifyFailed,
			"the MP3 has %d channel(s); the WAV has %d", channels, want.Channels)
	}

	decoded, err := decodeMP3(ctx, io.NewSectionReader(r, off, length), !want.Seam)
	if err != nil {
		return 0, err
	}
	got := time.Duration(float64(decoded) / float64(rate) / float64(channels) / 2 * float64(time.Second))

	tolerance := want.Duration / 100 // 1%, per spec §4.7
	if frame := 2 * 1152 * time.Second / time.Duration(rate); frame > tolerance {
		tolerance = frame
	}
	if want.Seam {
		tolerance += 1152 * time.Second / time.Duration(rate)
	}
	if diff := got - want.Duration; diff > tolerance || diff < -tolerance {
		return 0, siphonerr.New(siphonerr.VerifyFailed,
			"the MP3 decodes to %s, but the WAV is %s (±%s allowed)",
			got.Round(time.Millisecond), want.Duration.Round(time.Millisecond), tolerance.Round(time.Millisecond))
	}

	if min := int64(want.Duration.Seconds() * 1024); length < min {
		return 0, siphonerr.New(siphonerr.VerifyFailed,
			"the MP3 is %d bytes for %s of audio; that is not credible",
			length, want.Duration.Round(time.Second))
	}
	return rate, nil
}

type probeStream struct {
	CodecName  string `json:"codec_name"`
	Channels   int    `json:"channels"`
	SampleRate string `json:"sample_rate"`
}

func probeMP3(ctx context.Context, src io.Reader) (rate, channels int, err error) {
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_name,channels,sample_rate",
		"-of", "json",
		"-i", "pipe:0",
	)
	cmd.Stdin = src
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, 0, siphonerr.Wrap(siphonerr.VerifyFailed, err,
			"ffprobe rejected the encoded MP3: %s", bytes.TrimSpace(stderr.Bytes()))
	}
	var out struct {
		Streams []probeStream `json:"streams"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return 0, 0, siphonerr.Wrap(siphonerr.VerifyFailed, err, "cannot parse ffprobe output")
	}
	if len(out.Streams) != 1 || out.Streams[0].CodecName != "mp3" {
		return 0, 0, siphonerr.New(siphonerr.VerifyFailed,
			"expected exactly one MP3 stream, found %d in the encoded file", len(out.Streams))
	}
	rate, err = strconv.Atoi(out.Streams[0].SampleRate)
	if err != nil || rate <= 0 {
		return 0, 0, siphonerr.New(siphonerr.VerifyFailed, "the MP3 reports no sample rate")
	}
	return rate, out.Streams[0].Channels, nil
}

// decodeMP3 fully decodes the stream and returns the PCM byte count: the
// strongest available duration evidence for headerless VBR output. strict
// enables -xerror; splice-resumed streams pass false, because the join can
// contain one damaged frame by design.
func decodeMP3(ctx context.Context, src io.Reader, strict bool) (int64, error) {
	args := []string{"-v", "error"}
	if strict {
		args = append(args, "-xerror")
	}
	args = append(args, "-i", "pipe:0", "-f", "s16le", "-")
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdin = src
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	counter := &countWriter{}
	cmd.Stdout = counter
	if err := cmd.Run(); err != nil {
		msg := bytes.TrimSpace(stderr.Bytes())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return 0, siphonerr.Wrap(siphonerr.VerifyFailed, err,
			"the encoded MP3 does not decode: %s", msg)
	}
	return counter.n, nil
}

type countWriter struct{ n int64 }

func (w *countWriter) Write(p []byte) (int, error) {
	w.n += int64(len(p))
	return len(p), nil
}
