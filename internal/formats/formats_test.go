package formats

import (
	"strings"
	"testing"

	"siphon/internal/siphonerr"
)

type pairExpectation struct {
	defaultMode Mode
	modes       []Mode
	engine      EngineKind
	lossy       bool
	outputKind  string
}

func TestTable(t *testing.T) {
	pairs := AllPairs()
	if len(pairs) != 23 {
		t.Fatalf("expected 23 pairs, got %d", len(pairs))
	}

	want := map[[2]Format]pairExpectation{
		{WAV, MP3}:  {ModeSafe, []Mode{ModeSafe, ModeReclaim}, EngineSliding, true, "MP3"},
		{MKV, MP4}:  {ModeRelocate, []Mode{ModeRelocate, ModeSafe}, EngineRelocate, false, "fragmented MP4"},
		{PNG, JPG}:  {ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "JPEG"},
		{JPG, PNG}:  {ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "PNG"},
		{PNG, WEBP}: {ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "WebP"},
		{WEBP, PNG}: {ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, false, "PNG"},
		{JPG, WEBP}: {ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "WebP"},
		{WEBP, JPG}: {ModeSafe, []Mode{ModeSafe}, EngineAtomicCopy, true, "JPEG"},

		{PDF, PNG}: {ModeSafe, []Mode{ModeSafe}, EnginePDFRender, false, "8-bit sRGB PNG"},
		{PDF, JPG}: {ModeSafe, []Mode{ModeSafe}, EnginePDFRender, true, "8-bit sRGB JPEG"},
		{PNG, PDF}: {ModeSafe, []Mode{ModeSafe}, EnginePDFWrite, false, "PDF 1.7, one page"},
		{JPG, PDF}: {ModeSafe, []Mode{ModeSafe}, EnginePDFWrite, false, "PDF 1.7, one page"},

		{MP4, MKV}: {ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MKV"},
		{MOV, MKV}: {ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MKV"},
		{MKV, MOV}: {ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MOV"},
		{WEBM, MP4}: {ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "classic MP4"},
		{MP4, WEBM}: {ModeSafe, []Mode{ModeSafe}, EngineMedia, false, "WebM"},
		{GIF, MP4}:  {ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "classic MP4, no audio"},
		{MP4, GIF}:  {ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "animated GIF, no audio"},

		{MP4, MP3}:  {ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
		{MKV, MP3}:  {ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
		{MOV, MP3}:  {ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
		{WEBM, MP3}: {ModeSafe, []Mode{ModeSafe}, EngineMedia, true, "MP3"},
	}
	if len(want) != len(pairs) {
		t.Fatalf("expectation table has %d entries for %d pairs", len(want), len(pairs))
	}

	seen := map[[2]Format]bool{}
	for _, p := range pairs {
		key := [2]Format{p.Source, p.Target}
		if seen[key] {
			t.Errorf("duplicate pair %s → %s", p.Source, p.Target)
		}
		seen[key] = true

		got, ok := Lookup(p.Source, p.Target)
		if !ok || got.Source != p.Source || got.Target != p.Target {
			t.Errorf("%s→%s missing from Lookup", p.Source, p.Target)
		}
		exp, ok := want[key]
		if !ok {
			t.Errorf("unexpected pair %s → %s", p.Source, p.Target)
			continue
		}
		if p.Default != exp.defaultMode {
			t.Errorf("%s→%s default = %s, want %s", p.Source, p.Target, p.Default, exp.defaultMode)
		}
		if strings.Join(modeNames(p.Modes), ",") != strings.Join(modeNames(exp.modes), ",") {
			t.Errorf("%s→%s modes = %v, want %v", p.Source, p.Target, p.Modes, exp.modes)
		}
		if p.Engine != exp.engine {
			t.Errorf("%s→%s engine = %s, want %s", p.Source, p.Target, p.Engine, exp.engine)
		}
		if p.Lossy != exp.lossy {
			t.Errorf("%s→%s lossy = %v, want %v", p.Source, p.Target, p.Lossy, exp.lossy)
		}
		if p.OutputKind != exp.outputKind {
			t.Errorf("%s→%s output kind = %q, want %q", p.Source, p.Target, p.OutputKind, exp.outputKind)
		}
	}
}

func modeNames(modes []Mode) []string {
	out := make([]string, len(modes))
	for i, m := range modes {
		out[i] = string(m)
	}
	return out
}

func TestNewPairsAreSafeOnly(t *testing.T) {
	for _, p := range AllPairs() {
		if !IsV02Pair(p) {
			continue
		}
		if len(p.Modes) != 1 || p.Modes[0] != ModeSafe {
			t.Errorf("%s → %s must be safe only, got %v", p.Source, p.Target, p.Modes)
		}
		m, err := ResolveMode(p, ModeAuto)
		if err != nil || m != ModeSafe {
			t.Errorf("%s → %s auto = %s, %v", p.Source, p.Target, m, err)
		}
		for _, bad := range []Mode{ModeRelocate, ModeReclaim} {
			_, err := ResolveMode(p, bad)
			if !siphonerr.IsCode(err, siphonerr.ModeUnsupported) {
				t.Errorf("%s → %s %s: got %v, want E_MODE_UNSUPPORTED", p.Source, p.Target, bad, err)
			}
			if err != nil && !strings.Contains(err.Error(), "safe") {
				t.Errorf("%s → %s mode refusal does not name safe: %v", p.Source, p.Target, err)
			}
		}
	}
}

func TestResolveMode(t *testing.T) {
	mkv, _ := Lookup(MKV, MP4)
	m, err := ResolveMode(mkv, ModeAuto)
	if err != nil || m != ModeRelocate {
		t.Fatalf("default video mode = %s, %v", m, err)
	}
	if _, err := ResolveMode(mkv, ModeReclaim); err == nil {
		t.Fatal("video reclaim must be refused")
	}
	wav, _ := Lookup(WAV, MP3)
	if _, err := ResolveMode(wav, ModeRelocate); err == nil {
		t.Fatal("audio relocate must be refused")
	}
	if _, err := ResolveMode(mkv, ModeSafe); err != nil {
		t.Fatal("video safe must be allowed")
	}

	pdf, _ := Lookup(PDF, PNG)
	if _, err := ResolveMode(pdf, ModeRelocate); !siphonerr.IsCode(err, siphonerr.ModeUnsupported) {
		t.Fatalf("PDF relocate: got %v, want E_MODE_UNSUPPORTED", err)
	}
	gif, _ := Lookup(GIF, MP4)
	if _, err := ResolveMode(gif, ModeReclaim); !siphonerr.IsCode(err, siphonerr.ModeUnsupported) {
		t.Fatalf("GIF reclaim: got %v, want E_MODE_UNSUPPORTED", err)
	}
}

func TestEngineFor(t *testing.T) {
	mkv, _ := Lookup(MKV, MP4)
	if EngineFor(mkv, ModeRelocate) != EngineRelocate {
		t.Fatal("relocate engine")
	}
	if EngineFor(mkv, ModeSafe) != EngineRemux {
		t.Fatal("safe video uses remux")
	}
	img, _ := Lookup(PNG, JPG)
	if EngineFor(img, ModeSafe) != EngineAtomicCopy {
		t.Fatal("image engine")
	}
	pdf, _ := Lookup(PDF, PNG)
	if EngineFor(pdf, ModeSafe) != EnginePDFRender {
		t.Fatal("PDF render engine")
	}
	write, _ := Lookup(JPG, PDF)
	if EngineFor(write, ModeSafe) != EnginePDFWrite {
		t.Fatal("PDF write engine")
	}
	mediaPair, _ := Lookup(MP4, MKV)
	if EngineFor(mediaPair, ModeSafe) != EngineMedia {
		t.Fatal("media engine")
	}
	mkvSafe, _ := Lookup(MKV, MP4)
	if EngineFor(mkvSafe, ModeSafe) != EngineRemux {
		t.Fatal("MKV safe must keep the v1.7 remux engine")
	}
}

func TestOutputKindFor(t *testing.T) {
	mkv, _ := Lookup(MKV, MP4)
	if got := mkv.OutputKindFor(ModeRelocate); got != "fragmented MP4" {
		t.Errorf("relocate output kind = %q", got)
	}
	if got := mkv.OutputKindFor(ModeSafe); got != "MP4" {
		t.Errorf("safe output kind = %q", got)
	}
	for _, p := range AllPairs() {
		if p.Source == MKV && p.Target == MP4 {
			continue
		}
		if got := p.OutputKindFor(ModeSafe); got != p.OutputKind {
			t.Errorf("%s → %s safe output kind = %q, want %q", p.Source, p.Target, got, p.OutputKind)
		}
	}
}

func TestPairPredicates(t *testing.T) {
	tests := []struct {
		source, target Format
		pdfImage       bool
		imagePDF       bool
		containerRemux bool
		webmMP4        bool
		audioExtract   bool
		gifPair        bool
		v02            bool
	}{
		{PDF, PNG, true, false, false, false, false, false, true},
		{PDF, JPG, true, false, false, false, false, false, true},
		{PNG, PDF, false, true, false, false, false, false, true},
		{JPG, PDF, false, true, false, false, false, false, true},
		{MP4, MKV, false, false, true, false, false, false, true},
		{MOV, MKV, false, false, true, false, false, false, true},
		{MKV, MOV, false, false, true, false, false, false, true},
		{MKV, MP4, false, false, false, false, false, false, false},
		{WEBM, MP4, false, false, false, true, false, false, true},
		{MP4, WEBM, false, false, false, true, false, false, true},
		{MP4, MP3, false, false, false, false, true, false, true},
		{MKV, MP3, false, false, false, false, true, false, true},
		{MOV, MP3, false, false, false, false, true, false, true},
		{WEBM, MP3, false, false, false, false, true, false, true},
		{GIF, MP4, false, false, false, false, false, true, true},
		{MP4, GIF, false, false, false, false, false, true, true},
		{PNG, JPG, false, false, false, false, false, false, false},
		{WAV, MP3, false, false, false, false, false, false, false},
	}
	for _, tc := range tests {
		p := Pair{Source: tc.source, Target: tc.target}
		if got := IsPDFImagePair(p); got != tc.pdfImage {
			t.Errorf("IsPDFImagePair(%s→%s) = %v", tc.source, tc.target, got)
		}
		if got := IsImagePDFPair(p); got != tc.imagePDF {
			t.Errorf("IsImagePDFPair(%s→%s) = %v", tc.source, tc.target, got)
		}
		if got := IsContainerRemux(p); got != tc.containerRemux {
			t.Errorf("IsContainerRemux(%s→%s) = %v", tc.source, tc.target, got)
		}
		if got := IsWebMMP4(p); got != tc.webmMP4 {
			t.Errorf("IsWebMMP4(%s→%s) = %v", tc.source, tc.target, got)
		}
		if got := IsAudioExtract(p); got != tc.audioExtract {
			t.Errorf("IsAudioExtract(%s→%s) = %v", tc.source, tc.target, got)
		}
		if got := IsGIFPair(p); got != tc.gifPair {
			t.Errorf("IsGIFPair(%s→%s) = %v", tc.source, tc.target, got)
		}
		if got := IsV02Pair(p); got != tc.v02 {
			t.Errorf("IsV02Pair(%s→%s) = %v", tc.source, tc.target, got)
		}
	}
	if !IsVideo(GIF) || !IsVideo(WEBM) || IsVideo(PDF) {
		t.Fatal("IsVideo classification is wrong")
	}
	if !IsImage(PNG) || !IsImage(JPG) || IsImage(PDF) {
		t.Fatal("IsImage classification is wrong")
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Format{
		".jpeg": JPG,
		"JPEG":  JPG,
		".jpe":  JPG,
		"mkv":   MKV,
		"matroska": MKV,
		"m4v":   MP4,
		"qt":    MOV,
		"quicktime": MOV,
		"webm":  WEBM,
		"gif":   GIF,
		"pdf":   PDF,
		"webp":  WEBP,
	} {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %s, %v; want %s", in, got, ok, want)
		}
	}
	for _, in := range []string{"flac", "avi", "docx", "mkv.mkv"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) accepted an unsupported format", in)
		}
	}
}

func TestPairForPath(t *testing.T) {
	src, p, err := PairForPath("/films/a.mkv", MP4)
	if err != nil || src != MKV || p.Default != ModeRelocate {
		t.Fatalf("got %s %v %v", src, p, err)
	}
	src, p, err = PairForPath("/scans/page.jpeg", PDF)
	if err != nil || src != JPG || p.Engine != EnginePDFWrite {
		t.Fatalf("jpeg alias: got %s %v %v", src, p, err)
	}
	src, p, err = PairForPath("clip.mp4", WEBM)
	if err != nil || src != MP4 || p.Engine != EngineMedia || p.Default != ModeSafe {
		t.Fatalf("mp4→webm: got %s %v %v", src, p, err)
	}
	if _, _, err := PairForPath("a.png", PNG); err == nil {
		t.Fatal("same format must fail")
	}
	if _, _, err := PairForPath("a.png", MP3); err == nil {
		t.Fatal("unsupported pair must fail")
	}
	if _, _, err := PairForPath("a.flac", MP3); err == nil {
		t.Fatal("unsupported extension must fail")
	}
	if _, _, err := PairForPath("noextension", PNG); err == nil {
		t.Fatal("missing extension must fail")
	}
}

func TestSupportedTargets(t *testing.T) {
	got := SupportedTargets(PDF)
	want := []Format{PNG, JPG}
	if len(got) != len(want) {
		t.Fatalf("PDF targets = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("PDF targets = %v, want %v", got, want)
		}
	}
	if got := SupportedTargets(MP3); len(got) != 0 {
		t.Fatalf("MP3 must have no targets, got %v", got)
	}
}
