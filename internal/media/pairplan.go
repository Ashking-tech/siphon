package media

import (
	"fmt"
	"strings"
	"time"

	"siphon/internal/formats"
	"siphon/internal/siphonerr"
)

// PairOptions is the policy input for one v0.2 media pair.
type PairOptions struct {
	DropIncompatible bool
	AllowNonstandard bool
	Transcode        bool
	DropMetadata     bool
	VideoQuality     int
	VideoQualitySet  bool
	AudioBitrate     int
	AudioTrack       int
	FPS              float64
	Width            int
}

// PairPlan is the explicit stream map and encoder settings for a v0.2 media
// pair. It is built once at pre-flight and rebuilt identically by the engine,
// so a plan accepted by the gate is the plan executed by ffmpeg.
type PairPlan struct {
	SourceContainer string
	TargetContainer string
	Remux           bool
	Transcode       bool

	Streams  []PairStream
	Dropped  []string
	Warnings []string

	VideoIndex int
	AudioIndex int
	AudioTrack int // zero-based audio ordinal (video → MP3)

	VideoCodec       string // selected source codec
	AudioCodec       string // selected source codec
	TargetVideoCodec string
	TargetAudioCodec string

	VideoQuality int
	AudioBitrate int
	FPS          float64
	Width        int
	Height       int
	SourceWidth  int
	SourceHeight int

	Chapters    bool
	Attachments bool
	Timecode    bool
	Strict      bool

	Duration time.Duration
	Hints    map[string]bool
}

// PairStream is one input stream and its fate.
type PairStream struct {
	Index        int
	Type         string
	Codec        string
	Action       TrackAction
	AudioOrdinal int
}

// BuildPairPlan applies the v0.2 codec/transcode policy. It returns a partial
// plan even on refusal so `inspect` can show the stream map and proposed
// settings (spec §5.3).
func BuildPairPlan(p *Probe, source, target formats.Format, opt PairOptions) (*PairPlan, error) {
	if p == nil {
		return nil, siphonerr.New(siphonerr.SourceCorrupt, "no ffprobe data")
	}
	pp := &PairPlan{
		SourceContainer: string(source),
		TargetContainer: string(target),
		VideoIndex:      -1,
		AudioIndex:      -1,
		AudioTrack:      opt.AudioTrack,
		Duration:        p.Duration(),
	}
	switch {
	case formats.IsContainerRemux(formats.Pair{Source: source, Target: target}):
		return containerRemuxPlan(pp, p, target, opt)
	case formats.IsWebMMP4(formats.Pair{Source: source, Target: target}):
		return webmMP4Plan(pp, p, source, target, opt)
	case formats.IsAudioExtract(formats.Pair{Source: source, Target: target}):
		return audioExtractPlan(pp, p, opt)
	case formats.IsGIFPair(formats.Pair{Source: source, Target: target}):
		return gifPlan(pp, p, source, target, opt)
	}
	return nil, siphonerr.New(siphonerr.FormatUnsupported,
		"no media policy for %s → %s", source, target)
}

// --- container remux -------------------------------------------------------

func containerRemuxPlan(pp *PairPlan, p *Probe, target formats.Format, opt PairOptions) (*PairPlan, error) {
	pp.Remux = true
	toMOV := target == formats.MOV

	for _, s := range p.Streams {
		codec := strings.ToLower(s.CodecName)
		switch s.CodecType {
		case "video":
			allowed := mp4ToMKVVideo(codec)
			if toMOV {
				allowed = movVideo(codec)
			}
			if !allowed {
				if opt.DropIncompatible {
					pp.Dropped = append(pp.Dropped, fmt.Sprintf("video:%d %s", s.Index, codec))
					continue
				}
				return pp, codecErr(opt, fmt.Sprintf(
					"video stream %d uses %s, which %s cannot carry; pass --drop-incompatible to skip a non-primary stream",
					s.Index, codec, target))
			}
			addPairStream(pp, PairStream{Index: s.Index, Type: "video", Codec: codec, Action: ActionCopy})
			if pp.VideoIndex < 0 {
				pp.VideoIndex = s.Index
				pp.VideoCodec = codec
				pp.SourceWidth, pp.SourceHeight = s.Width, s.Height
			}
		case "audio":
			allowed := mp4ToMKVAudio(codec)
			if toMOV {
				allowed = movAudio(codec)
			}
			if !allowed {
				if opt.DropIncompatible {
					pp.Dropped = append(pp.Dropped, fmt.Sprintf("audio:%d %s", s.Index, codec))
					continue
				}
				return pp, codecErr(opt, fmt.Sprintf(
					"audio stream %d uses %s, which %s cannot carry; pass --drop-incompatible to skip it",
					s.Index, codec, target))
			}
			addPairStream(pp, PairStream{Index: s.Index, Type: "audio", Codec: codec, Action: ActionCopy})
			if pp.AudioIndex < 0 {
				pp.AudioIndex = s.Index
				pp.AudioCodec = codec
			}
		case "subtitle":
			if opt.DropIncompatible {
				pp.Dropped = append(pp.Dropped, fmt.Sprintf("subtitle:%d %s", s.Index, codec))
				continue
			}
			return pp, codecErr(opt, fmt.Sprintf(
				"subtitle stream %d (%s) cannot be copied into %s; pass --drop-incompatible to skip it",
				s.Index, codec, target))
		case "attachment":
			if toMOV {
				pp.Attachments = true
				pp.Dropped = append(pp.Dropped, fmt.Sprintf("attachment:%d %s", s.Index, codec))
				continue
			}
			// MKV can represent attachments. They are copied via an explicit
			// map, never by a wildcard.
			addPairStream(pp, PairStream{Index: s.Index, Type: "attachment", Codec: codec, Action: ActionCopy})
			pp.Attachments = true
		case "data":
			// Timecode and other data tracks are metadata losses, not media
			// streams: they must be dropped and reported (spec §9.2).
			pp.Timecode = true
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("data:%d %s", s.Index, codec))
		default:
			if opt.DropIncompatible {
				pp.Dropped = append(pp.Dropped, fmt.Sprintf("%s:%d %s", s.CodecType, s.Index, codec))
				continue
			}
			return pp, codecErr(opt, fmt.Sprintf(
				"stream %d has type %q, which %s cannot carry; pass --drop-incompatible to skip it",
				s.Index, s.CodecType, target))
		}
	}

	if pp.VideoIndex < 0 {
		return pp, siphonerr.New(siphonerr.CodecIncompatible,
			"no %s-copyable video stream remains", target)
	}
	if toMOV {
		pp.Chapters = len(p.Chapters) > 0
		if pp.Chapters {
			pp.Dropped = append(pp.Dropped, "chapters")
		}
	} else {
		pp.Chapters = len(p.Chapters) > 0
	}
	if pp.Attachments || pp.Timecode {
		// MKV → MOV attachments and every timecode/data track need
		// --drop-metadata; MP4/MOV → MKV only has timecode losses (attachments are
		// copied into MKV).
	}
	if err := metadataGate(pp, toMOV, opt); err != nil {
		return pp, err
	}
	return pp, nil
}

func metadataGate(pp *PairPlan, toMOV bool, opt PairOptions) error {
	var lost []string
	if toMOV && pp.Attachments {
		lost = append(lost, "attachments")
	}
	if toMOV && pp.Chapters {
		lost = append(lost, "chapters")
	}
	if pp.Timecode {
		lost = append(lost, "timecode/data tracks")
	}
	if len(lost) == 0 {
		return nil
	}
	if !opt.DropMetadata {
		return siphonerr.New(siphonerr.MetadataDrop,
			"source carries %s, which %s cannot carry; pass --drop-metadata to accept this",
			strings.Join(lost, " and "), pp.TargetContainer).WithHints(map[string]bool{"drop_metadata": true})
	}
	pp.Warnings = append(pp.Warnings,
		fmt.Sprintf("--drop-metadata set: %s will not be carried", strings.Join(lost, " and ")))
	return nil
}

func mp4ToMKVVideo(codec string) bool {
	switch codec {
	case "h264", "hevc", "av1", "mpeg4", "prores", "vp8", "vp9":
		return true
	}
	return false
}

func mp4ToMKVAudio(codec string) bool {
	switch codec {
	case "aac", "alac", "ac3", "eac3", "flac", "opus", "vorbis":
		return true
	}
	return strings.HasPrefix(codec, "pcm_")
}

func movVideo(codec string) bool {
	switch codec {
	case "h264", "hevc", "mpeg4", "prores":
		return true
	}
	return false
}

func movAudio(codec string) bool {
	switch codec {
	case "aac", "alac", "ac3", "eac3":
		return true
	}
	return strings.HasPrefix(codec, "pcm_")
}

// --- WebM ↔ MP4 ------------------------------------------------------------

func webmMP4Plan(pp *PairPlan, p *Probe, source, target formats.Format, opt PairOptions) (*PairPlan, error) {
	video := p.VideoStreams()
	audio := p.AudioStreams()
	if len(video) == 0 {
		return pp, siphonerr.New(siphonerr.SourceCorrupt, "source has no video stream")
	}
	primary := video[0]
	pp.VideoIndex = primary.Index
	pp.VideoCodec = strings.ToLower(primary.CodecName)
	pp.SourceWidth, pp.SourceHeight = primary.Width, primary.Height
	if len(audio) > 0 {
		pp.AudioIndex = audio[0].Index
		pp.AudioCodec = strings.ToLower(audio[0].CodecName)
	}

	// Explicit --transcode always wins once the gate has accepted it.
	if opt.Transcode {
		return transcodePlan(pp, p, source, target, opt)
	}

	remux, needAllow, err := remuxDecision(pp, p, source, target, opt)
	if err != nil {
		return pp, err
	}
	if !remux {
		// Remux is impossible: present the proposed transcode settings and
		// refuse until the user explicitly accepts the lossy path.
		if _, err := transcodePlan(pp, p, source, target, opt); err != nil {
			return pp, err
		}
		pp.Hints = map[string]bool{"transcode_required": true}
		return pp, siphonerr.New(siphonerr.TranscodeRequired,
			"%s → %s needs re-encoding; rerun with --transcode to accept the lossy conversion",
			source, target).WithHints(pp.Hints)
	}
	if needAllow && !opt.AllowNonstandard {
		pp.Hints = map[string]bool{"allow_nonstandard": true}
		return pp, siphonerr.New(siphonerr.CodecIncompatible,
			"%s → %s can remux, but %s is nonstandard in MP4; pass --allow-nonstandard or --transcode",
			source, target, nonstandardCodecs(pp)).WithHints(pp.Hints)
	}
	return pp, nil
}

// remuxDecision fills the copy stream map when the primary streams are
// remux-compatible. It returns needAllow for VP9/Opus in MP4.
func remuxDecision(pp *PairPlan, p *Probe, source, target formats.Format, opt PairOptions) (bool, bool, error) {
	toMP4 := target == formats.MP4
	pp.Remux = true
	pp.Transcode = false
	needAllow := false

	video := p.VideoStreams()
	primary := video[0]
	primaryCodec := strings.ToLower(primary.CodecName)
	if toMP4 {
		if primaryCodec != "vp9" && primaryCodec != "av1" {
			return false, false, nil
		}
		if primaryCodec == "vp9" {
			needAllow = true
		}
	} else {
		if primaryCodec != "vp9" && primaryCodec != "av1" {
			return false, false, nil
		}
	}

	// All retained video must fit the target. Extra unsupported video can be
	// dropped only with --drop-incompatible; the primary must survive.
	for i, s := range video {
		codec := strings.ToLower(s.CodecName)
		allowed := (toMP4 && (codec == "vp9" || codec == "av1")) ||
			(!toMP4 && (codec == "vp9" || codec == "av1"))
		if !allowed {
			if i == 0 {
				return false, false, nil
			}
			if !opt.DropIncompatible {
				return false, false, siphonerr.New(siphonerr.CodecIncompatible,
					"video stream %d uses %s, which %s cannot carry; pass --drop-incompatible to skip this extra stream",
					s.Index, codec, target).WithHints(map[string]bool{"drop_incompatible": true})
			}
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("video:%d %s", s.Index, codec))
			continue
		}
		addPairStream(pp, PairStream{Index: s.Index, Type: "video", Codec: codec, Action: ActionCopy})
		if toMP4 && codec == "vp9" {
			pp.Strict = true
		}
	}

	audio := p.AudioStreams()
	for i, s := range audio {
		codec := strings.ToLower(s.CodecName)
		if codec != "opus" {
			if i == 0 {
				return false, false, nil
			}
			if !opt.DropIncompatible {
				return false, false, siphonerr.New(siphonerr.CodecIncompatible,
					"audio stream %d uses %s, which %s cannot carry; pass --drop-incompatible to skip this extra track",
					s.Index, codec, target).WithHints(map[string]bool{"drop_incompatible": true})
			}
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("audio:%d %s", s.Index, codec))
			continue
		}
		if toMP4 {
			needAllow = true
		}
		addPairStream(pp, PairStream{Index: s.Index, Type: "audio", Codec: codec, Action: ActionCopy})
	}

	for _, s := range p.Streams {
		switch s.CodecType {
		case "video", "audio":
			// handled above
		case "subtitle":
			if !opt.DropIncompatible {
				return false, false, siphonerr.New(siphonerr.CodecIncompatible,
					"subtitle stream %d (%s) cannot be copied into %s; pass --drop-incompatible to skip it",
					s.Index, s.CodecName, target).WithHints(map[string]bool{"drop_incompatible": true})
			}
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("subtitle:%d %s", s.Index, s.CodecName))
		case "attachment":
			pp.Attachments = true
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("attachment:%d %s", s.Index, s.CodecName))
		case "data":
			pp.Timecode = true
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("data:%d %s", s.Index, s.CodecName))
		}
	}
	pp.Chapters = len(p.Chapters) > 0
	pp.TargetVideoCodec = primaryCodec
	if pp.AudioIndex >= 0 {
		pp.TargetAudioCodec = "opus"
		if toMP4 {
			pp.Strict = true
		}
	}
	if pp.Attachments || pp.Timecode {
		if !opt.DropMetadata {
			return false, false, siphonerr.New(siphonerr.MetadataDrop,
				"source carries attachments or timecode data that %s cannot carry; pass --drop-metadata to accept this",
				target).WithHints(map[string]bool{"drop_metadata": true})
		}
		pp.Warnings = append(pp.Warnings,
			"--drop-metadata set: attachments/timecode data will not be carried")
	}
	return true, needAllow, nil
}

func nonstandardCodecs(pp *PairPlan) string {
	var names []string
	for _, s := range pp.Streams {
		if s.Action == ActionCopy && (s.Codec == "vp9" || s.Codec == "opus") {
			names = append(names, fmt.Sprintf("%s (%s)", s.Codec, s.Type))
		}
	}
	if len(names) == 0 {
		return "a selected codec"
	}
	return strings.Join(names, ", ")
}

// transcodePlan fills the transcode decisions. It is also used to populate the
// proposed settings on an E_TRANSCODE_REQUIRED refusal.
func transcodePlan(pp *PairPlan, p *Probe, source, target formats.Format, opt PairOptions) (*PairPlan, error) {
	pp.Remux = false
	pp.Transcode = true
	pp.Streams = nil
	pp.Dropped = nil

	video := p.VideoStreams()
	audio := p.AudioStreams()
	if len(video) == 0 {
		return pp, siphonerr.New(siphonerr.SourceCorrupt, "source has no video stream")
	}
	primary := video[0]
	pp.VideoIndex = primary.Index
	pp.VideoCodec = strings.ToLower(primary.CodecName)
	pp.SourceWidth, pp.SourceHeight = primary.Width, primary.Height
	if len(video) > 1 {
		for _, extra := range video[1:] {
			pp.Warnings = append(pp.Warnings,
				fmt.Sprintf("video stream %d (%s) is dropped; only the primary video stream is retained",
					extra.Index, extra.CodecName))
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("video:%d %s", extra.Index, extra.CodecName))
		}
	}
	if isHighBitDepth(primary.PixFmt) {
		pp.Warnings = append(pp.Warnings,
			fmt.Sprintf("source pixel format %s is converted to 8-bit yuv420p", primary.PixFmt))
	}
	if strings.Contains(strings.ToLower(primary.PixFmt), "a") {
		pp.Warnings = append(pp.Warnings, "video alpha is not preserved; the output is opaque")
	}

	if target == formats.WEBM {
		pp.TargetVideoCodec = "vp9"
		pp.VideoQuality = qualityOr(opt, 32)
		pp.TargetAudioCodec = "opus"
		pp.AudioBitrate = opt.AudioBitrate
		if pp.AudioBitrate <= 0 {
			pp.AudioBitrate = 96
		}
	} else {
		pp.TargetVideoCodec = "h264"
		pp.VideoQuality = qualityOr(opt, 23)
		pp.TargetAudioCodec = "aac"
		pp.AudioBitrate = opt.AudioBitrate
		if pp.AudioBitrate <= 0 {
			pp.AudioBitrate = 128
		}
	}

	addPairStream(pp, PairStream{Index: primary.Index, Type: "video", Codec: pp.VideoCodec, Action: ActionTranscode})
	pp.Width, pp.Height = even(primary.Width), even(primary.Height)
	pp.SourceWidth, pp.SourceHeight = primary.Width, primary.Height
	if pp.Width != primary.Width || pp.Height != primary.Height {
		pp.Warnings = append(pp.Warnings,
			fmt.Sprintf("odd dimensions %d×%d are padded to %d×%d with black",
				primary.Width, primary.Height, pp.Width, pp.Height))
	}

	if len(audio) > 0 {
		a := audio[0]
		pp.AudioIndex = a.Index
		pp.AudioCodec = strings.ToLower(a.CodecName)
		addPairStream(pp, PairStream{Index: a.Index, Type: "audio", Codec: pp.AudioCodec, Action: ActionTranscode})
		if len(audio) > 1 {
			for _, extra := range audio[1:] {
				pp.Warnings = append(pp.Warnings,
					fmt.Sprintf("audio track %d (%s) is dropped; only the first audio stream is retained",
						extra.Index, extra.CodecName))
			}
		}
	}

	for _, s := range p.Streams {
		switch s.CodecType {
		case "video", "audio":
		case "subtitle":
			if !opt.DropIncompatible {
				return pp, siphonerr.New(siphonerr.CodecIncompatible,
					"subtitle stream %d (%s) is not converted in v0.2; pass --drop-incompatible to drop it",
					s.Index, s.CodecName).WithHints(map[string]bool{"drop_incompatible": true})
			}
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("subtitle:%d %s", s.Index, s.CodecName))
		case "attachment":
			pp.Attachments = true
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("attachment:%d %s", s.Index, s.CodecName))
		case "data":
			pp.Timecode = true
			pp.Dropped = append(pp.Dropped, fmt.Sprintf("data:%d %s", s.Index, s.CodecName))
		}
	}
	if pp.Attachments || pp.Timecode {
		if !opt.DropMetadata {
			return pp, siphonerr.New(siphonerr.MetadataDrop,
				"source carries attachments or timecode data that %s cannot carry; pass --drop-metadata to accept this",
				target).WithHints(map[string]bool{"drop_metadata": true})
		}
		pp.Warnings = append(pp.Warnings,
			"--drop-metadata set: attachments/timecode data will not be carried")
	}
	pp.Chapters = len(p.Chapters) > 0
	if pp.Chapters {
		pp.Warnings = append(pp.Warnings, "chapters are copied where the target muxer can represent them")
	}
	return pp, nil
}

// --- video → MP3 -----------------------------------------------------------

func audioExtractPlan(pp *PairPlan, p *Probe, opt PairOptions) (*PairPlan, error) {
	audio := p.AudioStreams()
	if len(audio) == 0 {
		pp.Hints = map[string]bool{"no_audio": true}
		return pp, siphonerr.New(siphonerr.NoAudio,
			"source has no audio stream to extract").WithHints(pp.Hints)
	}
	if opt.AudioTrack < 0 || opt.AudioTrack >= len(audio) {
		pp.Hints = map[string]bool{"audio_track": true}
		return pp, siphonerr.New(siphonerr.AudioTrack,
			"audio track %d does not exist; the source has %d audio stream(s) (ordinals 0..%d)",
			opt.AudioTrack, len(audio), len(audio)-1).WithHints(pp.Hints)
	}
	sel := audio[opt.AudioTrack]
	pp.Transcode = true
	pp.TargetContainer = "mp3"
	pp.AudioTrack = opt.AudioTrack
	pp.AudioIndex = sel.Index
	pp.AudioCodec = strings.ToLower(sel.CodecName)
	pp.TargetAudioCodec = "mp3"
	pp.AudioBitrate = opt.AudioBitrate
	if len(audio) > 1 {
		for i, a := range audio {
			if i == opt.AudioTrack {
				continue
			}
			pp.Warnings = append(pp.Warnings,
				fmt.Sprintf("audio track %d (%s) is not carried; track %d is selected",
					i, a.CodecName, opt.AudioTrack))
		}
	}
	if len(p.VideoStreams()) > 0 {
		pp.Warnings = append(pp.Warnings, "video is dropped")
	}
	if len(p.Chapters) > 0 {
		pp.Chapters = true
		pp.Dropped = append(pp.Dropped, "chapters")
	}
	addPairStream(pp, PairStream{Index: sel.Index, Type: "audio", Codec: pp.AudioCodec, Action: ActionTranscode})
	return pp, nil
}

// --- GIF ↔ MP4 -------------------------------------------------------------

func gifPlan(pp *PairPlan, p *Probe, source, target formats.Format, opt PairOptions) (*PairPlan, error) {
	video := p.VideoStreams()
	if len(video) == 0 {
		return pp, siphonerr.New(siphonerr.SourceCorrupt, "source has no video stream")
	}
	primary := video[0]
	pp.Transcode = true
	pp.VideoIndex = primary.Index
	pp.VideoCodec = strings.ToLower(primary.CodecName)
	pp.SourceWidth, pp.SourceHeight = primary.Width, primary.Height
	pp.FPS = opt.FPS
	pp.Width = opt.Width

	if source == formats.GIF {
		pp.TargetContainer = "mp4"
		pp.TargetVideoCodec = "h264"
		pp.VideoQuality = qualityOr(opt, 23)
		if pp.FPS <= 0 {
			pp.FPS = 30
		}
		pp.Width, pp.Height = even(primary.Width), even(primary.Height)
		addPairStream(pp, PairStream{Index: primary.Index, Type: "video", Codec: pp.VideoCodec, Action: ActionTranscode})
		return pp, nil
	}

	pp.TargetContainer = "gif"
	pp.TargetVideoCodec = "gif"
	if pp.FPS <= 0 {
		pp.FPS = 12
	}
	maxW := opt.Width
	if maxW <= 0 {
		maxW = 480
	}
	pp.Width, pp.Height = gifDimensions(primary.Width, primary.Height, maxW)
	addPairStream(pp, PairStream{Index: primary.Index, Type: "video", Codec: pp.VideoCodec, Action: ActionTranscode})
	if len(p.AudioStreams()) > 0 {
		pp.Warnings = append(pp.Warnings, "audio is dropped")
	}
	if len(p.Streams) > 1 {
		pp.Warnings = append(pp.Warnings, "subtitles, chapters, and metadata are not carried")
	}
	return pp, nil
}

// gifDimensions applies --width as a maximum and never upscales.
func gifDimensions(srcW, srcH, maxW int) (int, int) {
	if srcW <= 0 || srcH <= 0 {
		return 0, 0
	}
	if maxW <= 0 || maxW >= srcW {
		return srcW, srcH
	}
	h := int(float64(srcH)*float64(maxW)/float64(srcW) + 0.5)
	if h < 1 {
		h = 1
	}
	return maxW, h
}

func even(n int) int {
	if n <= 0 {
		return n
	}
	if n%2 != 0 {
		return n + 1
	}
	return n
}

// qualityOr returns the explicit CRF when one was supplied. CRF 0 is a valid
// request, so the caller records whether the flag was set separately from the
// integer value.
func qualityOr(opt PairOptions, def int) int {
	if !opt.VideoQualitySet {
		return def
	}
	if opt.VideoQuality < 0 {
		return 0
	}
	if opt.VideoQuality > 51 {
		return 51
	}
	return opt.VideoQuality
}

func addPairStream(pp *PairPlan, s PairStream) {
	if s.Type == "audio" {
		s.AudioOrdinal = 0
		for _, existing := range pp.Streams {
			if existing.Type == "audio" {
				s.AudioOrdinal++
			}
		}
	}
	pp.Streams = append(pp.Streams, s)
}

func codecErr(opt PairOptions, msg string) error {
	err := siphonerr.New(siphonerr.CodecIncompatible, "%s", msg)
	if opt.DropIncompatible {
		return err
	}
	return err.WithHints(map[string]bool{"drop_incompatible": true})
}

func isHighBitDepth(pixFmt string) bool {
	p := strings.ToLower(pixFmt)
	switch {
	case p == "":
		return false
	case strings.Contains(p, "10"), strings.Contains(p, "12"), strings.Contains(p, "16"):
		return true
	}
	return false
}
