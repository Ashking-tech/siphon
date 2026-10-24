// Package core holds the types shared by the CLI, the pre-flight gate, and
// every engine. It deliberately has no dependencies beyond formats.
package core

import (
	"fmt"
	"time"

	"siphon/internal/formats"
)

// Job is a fully parsed user request. Validation happens in preflight.
type Job struct {
	SourcePath string
	TargetPath string
	Source     formats.Format
	Target     formats.Format
	Pair       formats.Pair
	Mode       formats.Mode
	Engine     formats.EngineKind

	Force            bool
	Yes              bool
	DropIncompatible bool
	TranscodeAudio   bool
	AllowNonstandard bool
	DropMetadata     bool
	AllowHardlinks   bool
	AllowRoot        bool
	Background       string
	Quality          int
	Bitrate          int
	SegmentDuration  time.Duration
	SegmentTolerance time.Duration
	Verify           string
	VerifyFrames     int
	ProbeSegments    int
	SkipProbe        bool
	Timeout          time.Duration
	JSON             bool

	// Tools records the versions verified at pre-flight, for the journal.
	Tools map[string]string
}

// Plan is the result of a successful pre-flight. It is also the payload of
// `siphon inspect`, so every field is JSON-stable.
type Plan struct {
	Source          string `json:"source"`
	Target          string `json:"target"`
	SourceFormat    string `json:"source_format"`
	TargetFormat    string `json:"target_format"`
	Mode            string `json:"mode"`
	Engine          string `json:"engine"`
	OutputKind      string `json:"output_kind"`
	Lossy           bool   `json:"lossy"`
	PlanSource      string `json:"plan_source,omitempty"`
	Segments        int    `json:"segments,omitempty"`
	SegmentDuration string `json:"segment_duration,omitempty"`
	Verify          string `json:"verify,omitempty"`

	SourceSize         int64  `json:"source_size_bytes"`
	FreeNeeded         int64  `json:"free_needed_bytes"`
	FreeNeededHuman    string `json:"free_needed"`
	FreeAvailable      int64  `json:"free_available_bytes"`
	FreeAvailableHuman string `json:"free_available"`

	CrashCost string   `json:"crash_cost"`
	Warnings  []string `json:"warnings,omitempty"`

	// MKV track policy details (remux/relocate).
	Streams []string `json:"streams,omitempty"`
	Dropped []string `json:"dropped,omitempty"`

	// Image-specific plan details.
	ImageWidth  int      `json:"image_width,omitempty"`
	ImageHeight int      `json:"image_height,omitempty"`
	HasAlpha    bool     `json:"has_alpha,omitempty"`
	Metadata    []string `json:"metadata,omitempty"`

	// Audio-specific plan details.
	AudioChannels   int      `json:"audio_channels,omitempty"`
	AudioSampleRate int      `json:"audio_sample_rate,omitempty"`
	AudioDurationMS int64    `json:"audio_duration_ms,omitempty"`
	AudioTags       []string `json:"audio_tags,omitempty"`
}

// Segment is one keyframe-aligned byte range in a relocate plan.
type Segment struct {
	Index    int
	SrcOff   int64
	SrcEnd   int64
	OutOff   int64
	OutLen   int64
	Packets  int64
	Duration time.Duration
}

// Result is the machine-readable outcome of a command.
type Result struct {
	OK              bool   `json:"ok"`
	Code            string `json:"code,omitempty"`
	Message         string `json:"message,omitempty"`
	Plan            *Plan  `json:"plan,omitempty"`
	SourceUntouched bool   `json:"source_untouched"`
}

// HumanBytes renders a byte count for inspect output. It always states
// absolute units, never just a multiplier (spec P3).
func HumanBytes(n int64) string {
	const (
		kib = 1024
		mib = 1024 * kib
		gib = 1024 * mib
	)
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GiB", float64(n)/gib)
	case n >= mib:
		return fmt.Sprintf("%.0f MiB", float64(n)/mib)
	case n >= kib:
		return fmt.Sprintf("%.0f KiB", float64(n)/kib)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
