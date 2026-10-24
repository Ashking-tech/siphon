package media

import (
	"fmt"
	"strings"

	"siphon/internal/siphonerr"
)

// TrackAction is what the remux does with one input stream.
type TrackAction string

const (
	ActionCopy      TrackAction = "copy"
	ActionTranscode TrackAction = "transcode"
	ActionDrop      TrackAction = "drop"
)

// Track is one input stream and its fate.
type Track struct {
	Index        int         `json:"index"`
	Type         string      `json:"type"`
	Codec        string      `json:"codec"`
	Action       TrackAction `json:"action"`
	AudioOrdinal int         `json:"-"` // output audio ordinal, for -c:a:N
}

// TrackOptions are the user flags that affect the MP4 track policy.
type TrackOptions struct {
	DropIncompatible bool
	TranscodeAudio   bool
	AllowNonstandard bool
	Relocate         bool
}

// TrackPlan is the explicit -map policy for one MKV → MP4 remux. Tracks are
// ordered video first, then audio, which is also the MP4 output order.
type TrackPlan struct {
	Tracks      []Track
	Dropped     []string // human descriptions of dropped tracks/metadata
	Warnings    []string
	Attachments bool
	Chapters    bool
	Tags        bool
	Strict      bool // needs -strict -2 (AC-3/E-AC-3/FLAC/Opus in MP4)
}

// BuildTrackPlan applies the v0.1 compatibility matrix (spec §4.5.3). It is
// shared by pre-flight (refusal) and the remux engine (the actual -map list),
// so a plan accepted by one is the plan executed by the other.
func BuildTrackPlan(p *Probe, opt TrackOptions) (*TrackPlan, error) {
	if p == nil {
		return nil, siphonerr.New(siphonerr.SourceCorrupt, "no ffprobe data")
	}
	tp := &TrackPlan{}
	var video, audio []Track

	for _, s := range p.Streams {
		typ := s.CodecType
		codec := strings.ToLower(s.CodecName)
		switch typ {
		case "video":
			switch codec {
			case "h264", "hevc", "av1", "mpeg4":
				video = append(video, Track{Index: s.Index, Type: typ, Codec: codec, Action: ActionCopy})
			case "vp8", "vp9":
				if opt.AllowNonstandard {
					video = append(video, Track{Index: s.Index, Type: typ, Codec: codec, Action: ActionCopy})
					tp.Strict = true
					tp.Warnings = append(tp.Warnings,
						fmt.Sprintf("video stream %d is %s; MP4 support is nonstandard (--allow-nonstandard)", s.Index, codec))
				} else if opt.DropIncompatible {
					tp.Dropped = append(tp.Dropped, fmt.Sprintf("video:%d %s", s.Index, codec))
				} else {
					return nil, siphonerr.New(siphonerr.CodecIncompatible,
						"video stream %d is %s, which MP4 players do not reliably support; pass --allow-nonstandard or --drop-incompatible",
						s.Index, codec)
				}
			default:
				if opt.DropIncompatible {
					tp.Dropped = append(tp.Dropped, fmt.Sprintf("video:%d %s", s.Index, codec))
				} else {
					return nil, siphonerr.New(siphonerr.CodecIncompatible,
						"video stream %d uses %s, which Siphon cannot copy into MP4; pass --drop-incompatible to skip it",
						s.Index, codec)
				}
			}
		case "audio":
			t := Track{Index: s.Index, Type: typ, Codec: codec, Action: ActionCopy}
			switch codec {
			case "aac", "alac":
				// copy
			case "ac3", "eac3":
				tp.Strict = true
			case "flac", "opus":
				if opt.AllowNonstandard {
					tp.Strict = true
					tp.Warnings = append(tp.Warnings,
						fmt.Sprintf("audio stream %d is %s; MP4 support is nonstandard (--allow-nonstandard)", s.Index, codec))
				} else if opt.DropIncompatible {
					tp.Dropped = append(tp.Dropped, fmt.Sprintf("audio:%d %s", s.Index, codec))
					continue
				} else {
					return nil, siphonerr.New(siphonerr.CodecIncompatible,
						"audio stream %d is %s, which MP4 players do not reliably support; pass --allow-nonstandard or --drop-incompatible",
						s.Index, codec)
				}
			default:
				if opt.TranscodeAudio {
					if opt.Relocate {
						return nil, siphonerr.New(siphonerr.InplaceInfeasible,
							"relocate is remux-only and cannot transcode audio stream %d (%s); use --mode safe",
							s.Index, codec)
					}
					t.Action = ActionTranscode
					tp.Warnings = append(tp.Warnings,
						fmt.Sprintf("transcoding audio stream %d (%s) to AAC", s.Index, codec))
				} else if opt.DropIncompatible {
					tp.Dropped = append(tp.Dropped, fmt.Sprintf("audio:%d %s", s.Index, codec))
					continue
				} else {
					return nil, siphonerr.New(siphonerr.CodecIncompatible,
						"audio stream %d uses %s, which MP4 cannot carry; pass --transcode-audio (safe mode) or --drop-incompatible",
						s.Index, codec)
				}
			}
			audio = append(audio, t)
		case "subtitle":
			if opt.DropIncompatible {
				tp.Dropped = append(tp.Dropped, fmt.Sprintf("subtitle:%d %s", s.Index, codec))
				continue
			}
			return nil, siphonerr.New(siphonerr.CodecIncompatible,
				"subtitle stream %d (%s) cannot be copied into MP4; pass --drop-incompatible to skip it",
				s.Index, codec)
		case "attachment":
			tp.Attachments = true
			tp.Dropped = append(tp.Dropped, fmt.Sprintf("attachment:%d %s", s.Index, codec))
		case "data":
			if opt.DropIncompatible {
				tp.Dropped = append(tp.Dropped, fmt.Sprintf("data:%d %s", s.Index, codec))
			} else {
				return nil, siphonerr.New(siphonerr.CodecIncompatible,
					"data stream %d (%s) cannot be copied into MP4; pass --drop-incompatible to skip it",
					s.Index, codec)
			}
		default:
			if opt.DropIncompatible {
				tp.Dropped = append(tp.Dropped, fmt.Sprintf("%s:%d %s", typ, s.Index, codec))
			} else {
				return nil, siphonerr.New(siphonerr.CodecIncompatible,
					"stream %d has type %q, which Siphon cannot copy into MP4; pass --drop-incompatible to skip it",
					s.Index, typ)
			}
		}
	}

	if len(video) == 0 {
		return nil, siphonerr.New(siphonerr.CodecIncompatible,
			"no MP4-copyable video stream remains")
	}

	tp.Tracks = append(tp.Tracks, video...)
	ordinal := 0
	for _, t := range audio {
		t.AudioOrdinal = ordinal
		ordinal++
		tp.Tracks = append(tp.Tracks, t)
	}
	tp.Chapters = len(p.Chapters) > 0
	if tp.Chapters {
		tp.Dropped = append(tp.Dropped, "chapters")
	}
	tp.Tags = hasTags(p)
	if tp.Tags {
		tp.Warnings = append(tp.Warnings,
			"container/track tags are copied on a best-effort basis; unmatched tags are dropped and reported")
	}
	return tp, nil
}

func hasTags(p *Probe) bool {
	if len(p.Format.Tags) > 0 {
		return true
	}
	for _, s := range p.Streams {
		if len(s.Tags) > 0 {
			return true
		}
	}
	return false
}

// MapArgs returns the -map arguments in output order.
func (tp *TrackPlan) MapArgs() []string {
	args := make([]string, 0, len(tp.Tracks)*2)
	for _, t := range tp.Tracks {
		args = append(args, "-map", fmt.Sprintf("0:%d", t.Index))
	}
	return args
}

// CodecArgs returns the codec selection: copy by default, with per-audio
// overrides for transcoded streams.
func (tp *TrackPlan) CodecArgs() []string {
	args := []string{"-c", "copy"}
	for _, t := range tp.Tracks {
		if t.Action == ActionTranscode {
			args = append(args, fmt.Sprintf("-c:a:%d", t.AudioOrdinal), "aac", "-b:a", "192k")
		}
	}
	if tp.Strict {
		args = append(args, "-strict", "-2")
	}
	return args
}

// Describe renders the retained tracks for inspect.
func (tp *TrackPlan) Describe() []string {
	out := make([]string, 0, len(tp.Tracks))
	for _, t := range tp.Tracks {
		out = append(out, fmt.Sprintf("%s:%d %s %s", t.Type, t.Index, t.Codec, t.Action))
	}
	return out
}

// MetadataWarnings returns the warnings that depend on --drop-metadata.
func (tp *TrackPlan) MetadataWarnings(drop bool) []string {
	if !drop || !(tp.Attachments || tp.Chapters) {
		return nil
	}
	var what []string
	if tp.Attachments {
		what = append(what, "attachments")
	}
	if tp.Chapters {
		what = append(what, "chapters")
	}
	return []string{fmt.Sprintf("--drop-metadata set: %s will not be carried", strings.Join(what, " and "))}
}
