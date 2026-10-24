package media

import (
	"reflect"
	"strings"
	"testing"

	"siphon/internal/siphonerr"
)

func probe(streams ...Stream) *Probe {
	return &Probe{Streams: streams}
}

func TestBuildTrackPlanCopy(t *testing.T) {
	p := probe(
		Stream{Index: 0, CodecType: "video", CodecName: "h264"},
		Stream{Index: 1, CodecType: "audio", CodecName: "aac"},
	)
	tp, err := BuildTrackPlan(p, TrackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tp.Tracks) != 2 {
		t.Fatalf("tracks = %+v", tp.Tracks)
	}
	if got, want := tp.MapArgs(), []string{"-map", "0:0", "-map", "0:1"}; !reflect.DeepEqual(got, want) {
		t.Errorf("MapArgs = %v, want %v", got, want)
	}
	if got, want := tp.CodecArgs(), []string{"-c", "copy"}; !reflect.DeepEqual(got, want) {
		t.Errorf("CodecArgs = %v, want %v", got, want)
	}
}

func TestBuildTrackPlanNonstandard(t *testing.T) {
	p := probe(
		Stream{Index: 0, CodecType: "video", CodecName: "vp9"},
		Stream{Index: 1, CodecType: "audio", CodecName: "opus"},
	)
	if _, err := BuildTrackPlan(p, TrackOptions{}); !siphonerr.IsCode(err, siphonerr.CodecIncompatible) {
		t.Fatalf("VP9 without --allow-nonstandard: %v", err)
	}
	tp, err := BuildTrackPlan(p, TrackOptions{AllowNonstandard: true})
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Strict {
		t.Error("VP9/Opus plan must set -strict -2")
	}
}

func TestBuildTrackPlanTranscode(t *testing.T) {
	p := probe(
		Stream{Index: 0, CodecType: "video", CodecName: "h264"},
		Stream{Index: 1, CodecType: "audio", CodecName: "pcm_s16le"},
	)
	if _, err := BuildTrackPlan(p, TrackOptions{}); !siphonerr.IsCode(err, siphonerr.CodecIncompatible) {
		t.Fatalf("PCM without flags: %v", err)
	}
	tp, err := BuildTrackPlan(p, TrackOptions{TranscodeAudio: true})
	if err != nil {
		t.Fatal(err)
	}
	if tp.Tracks[1].Action != ActionTranscode {
		t.Fatalf("audio action = %s", tp.Tracks[1].Action)
	}
	if got, want := tp.CodecArgs(), []string{"-c", "copy", "-c:a:0", "aac", "-b:a", "192k"}; !reflect.DeepEqual(got, want) {
		t.Errorf("CodecArgs = %v, want %v", got, want)
	}
	if _, err := BuildTrackPlan(p, TrackOptions{TranscodeAudio: true, Relocate: true}); !siphonerr.IsCode(err, siphonerr.InplaceInfeasible) {
		t.Fatalf("relocate+transcode: %v", err)
	}
}

func TestBuildTrackPlanDropAndMetadata(t *testing.T) {
	p := probe(
		Stream{Index: 0, CodecType: "video", CodecName: "h264"},
		Stream{Index: 1, CodecType: "audio", CodecName: "aac"},
		Stream{Index: 2, CodecType: "subtitle", CodecName: "subrip"},
		Stream{Index: 3, CodecType: "attachment", CodecName: "ttf"},
	)
	p.Chapters = []Chapter{{ID: 1}}
	tp, err := BuildTrackPlan(p, TrackOptions{DropIncompatible: true})
	if err != nil {
		t.Fatal(err)
	}
	if !tp.Attachments || !tp.Chapters {
		t.Fatalf("metadata flags: attachments=%v chapters=%v", tp.Attachments, tp.Chapters)
	}
	if len(tp.Tracks) != 2 {
		t.Fatalf("retained tracks = %+v", tp.Tracks)
	}
	joined := strings.Join(tp.Dropped, ",")
	if !strings.Contains(joined, "subtitle:2") || !strings.Contains(joined, "attachment:3") || !strings.Contains(joined, "chapters") {
		t.Errorf("dropped = %v", tp.Dropped)
	}
	if len(tp.MetadataWarnings(true)) == 0 {
		t.Error("expected a --drop-metadata warning")
	}
}

func TestBuildTrackPlanMultipleAudio(t *testing.T) {
	p := probe(
		Stream{Index: 0, CodecType: "video", CodecName: "hevc"},
		Stream{Index: 1, CodecType: "audio", CodecName: "aac"},
		Stream{Index: 2, CodecType: "audio", CodecName: "ac3"},
	)
	tp, err := BuildTrackPlan(p, TrackOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if tp.Tracks[0].Type != "video" || tp.Tracks[1].AudioOrdinal != 0 || tp.Tracks[2].AudioOrdinal != 1 {
		t.Fatalf("order/ordinals = %+v", tp.Tracks)
	}
	if !tp.Strict {
		t.Error("AC-3 must set -strict -2")
	}
}

func TestBuildTrackPlanNoVideo(t *testing.T) {
	p := probe(Stream{Index: 0, CodecType: "audio", CodecName: "aac"})
	if _, err := BuildTrackPlan(p, TrackOptions{DropIncompatible: true}); !siphonerr.IsCode(err, siphonerr.CodecIncompatible) {
		t.Fatalf("audio-only MKV: %v", err)
	}
}
