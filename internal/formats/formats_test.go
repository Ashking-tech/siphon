package formats

import "testing"

func TestTable(t *testing.T) {
	pairs := AllPairs()
	if len(pairs) != 8 {
		t.Fatalf("expected 8 pairs, got %d", len(pairs))
	}
	for _, p := range pairs {
		if _, ok := Lookup(p.Source, p.Target); !ok {
			t.Errorf("%s→%s missing", p.Source, p.Target)
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
}

func TestOutputKindFor(t *testing.T) {
	mkv, _ := Lookup(MKV, MP4)
	if got := mkv.OutputKindFor(ModeRelocate); got != "fragmented MP4" {
		t.Errorf("relocate output kind = %q", got)
	}
	if got := mkv.OutputKindFor(ModeSafe); got != "MP4" {
		t.Errorf("safe output kind = %q", got)
	}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Format{
		".jpeg": JPG,
		"JPEG":  JPG,
		"mkv":   MKV,
		"webp":  WEBP,
	} {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %s, %v; want %s", in, got, ok, want)
		}
	}
	if _, ok := Parse("flac"); ok {
		t.Fatal("flac must be unsupported in v0.1")
	}
}

func TestPairForPath(t *testing.T) {
	src, p, err := PairForPath("/films/a.mkv", MP4)
	if err != nil || src != MKV || p.Default != ModeRelocate {
		t.Fatalf("got %s %v %v", src, p, err)
	}
	if _, _, err := PairForPath("a.png", PNG); err == nil {
		t.Fatal("same format must fail")
	}
	if _, _, err := PairForPath("a.png", MP3); err == nil {
		t.Fatal("unsupported pair must fail")
	}
}
