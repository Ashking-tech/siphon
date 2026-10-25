package stitch

import (
	"context"
	"os/exec"
)

// makeFFmpeg builds the ffmpeg invocation the spike uses everywhere.
func makeFFmpeg(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "ffmpeg", args...)
}

// makeFFprobe builds the ffprobe invocation.
func makeFFprobe(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "ffprobe", args...)
}
