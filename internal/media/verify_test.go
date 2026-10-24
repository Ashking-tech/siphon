package media

import (
	"reflect"
	"testing"
)

func TestMissingTags(t *testing.T) {
	src := &Probe{
		Format: FormatInfo{Tags: map[string]string{
			"title":   "A film",
			"ENCODER": "Lavf-src",
		}},
		Streams: []Stream{
			{Index: 0, Tags: map[string]string{"language": "eng", "DURATION": "00:00:06"}},
			{Index: 1, Tags: map[string]string{"title": "Stereo"}},
		},
	}
	out := &Probe{
		Format: FormatInfo{Tags: map[string]string{"ENCODER": "Lavf-out"}},
		Streams: []Stream{
			{Tags: map[string]string{"DURATION": "00:00:06.02"}},
			{Tags: map[string]string{"title": "Stereo"}},
		},
	}
	got := MissingTags(src, out, []int{0, 1})
	want := []string{"format:title", "stream:0:language"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("MissingTags = %v, want %v", got, want)
	}

	out.Format.Tags["title"] = "A film"
	out.Streams[0].Tags["language"] = "eng"
	if got := MissingTags(src, out, []int{0, 1}); len(got) != 0 {
		t.Fatalf("MissingTags after restoring user tags = %v", got)
	}
}
