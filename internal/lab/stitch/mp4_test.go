package stitch

import (
	"bytes"
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

func TestWalkBoxes(t *testing.T) {
	ftyp := boxOf("ftyp", []byte("isom\x00\x00\x00\x00"))
	large := make([]byte, 24)
	binary.BigEndian.PutUint32(large[0:], 1)
	copy(large[4:], "free")
	binary.BigEndian.PutUint64(large[8:], 24)
	toEOF := []byte{0, 0, 0, 0, 'm', 'd', 'a', 't', 1, 2, 3}
	buf := concat(ftyp, large, toEOF)

	boxes, err := walkBoxes(buf, 0, len(buf))
	if err != nil {
		t.Fatalf("walkBoxes: %v", err)
	}
	if len(boxes) != 3 {
		t.Fatalf("got %d boxes, want 3", len(boxes))
	}
	if boxes[0].typ != "ftyp" || boxes[2].typ != "mdat" || boxes[2].size != len(toEOF) {
		t.Errorf("boxes = %+v", boxes)
	}
	if boxes[1].hdr != 16 || boxes[1].size != 24 {
		t.Errorf("largesize box = %+v", boxes[1])
	}
}

func TestStripInit(t *testing.T) {
	ftyp := boxOf("ftyp", []byte("isom\x00\x00\x00\x00"))
	moov := boxOf("moov", boxOf("mvex"))
	moof := testMoof(1)
	mdat := boxOf("mdat", []byte{1, 2, 3})
	mfra := boxOf("mfra", []byte{9})
	buf := concat(ftyp, moov, moof, mdat, mfra)
	init, frag, err := stripInit(buf)
	if err != nil {
		t.Fatalf("stripInit: %v", err)
	}
	if !bytes.Equal(init, concat(ftyp, moov)) {
		t.Error("init does not hold ftyp+moov")
	}
	if !bytes.Equal(frag, concat(moof, mdat)) {
		t.Error("fragment does not hold moof+mdat without mfra")
	}

	// A fragment that is already bare (no ftyp/moov) yields no init.
	init, frag, err = stripInit(concat(moof, mdat))
	if err != nil {
		t.Fatalf("stripInit bare: %v", err)
	}
	if len(init) != 0 || !bytes.Equal(frag, concat(moof, mdat)) {
		t.Error("bare fragment layout changed")
	}
}

func TestTrunCTSAndShift(t *testing.T) {
	samples := [][3]int32{
		{528, 4958, 0},
		{544, 1960, 1072},
		{528, 1004, -1072},
		{544, 852, -528},
	}
	trun := testTrun(1, 0xB05, 0, 0, samples)
	box := mp4Box{typ: "trun", off: 0, size: len(trun), hdr: 8}

	present, count, first, min, err := trunCTS(trun, box)
	if err != nil {
		t.Fatalf("trunCTS: %v", err)
	}
	if !present || count != 4 || first != 0 || min != -1072 {
		t.Fatalf("trunCTS = present %v count %d first %d min %d", present, count, first, min)
	}
	if err := shiftTrunCTS(trun, box, 1072); err != nil {
		t.Fatalf("shiftTrunCTS: %v", err)
	}
	present, count, first, min, err = trunCTS(trun, box)
	if err != nil || !present || count != 4 || first != 1072 || min != 0 {
		t.Fatalf("after shift: present %v count %d first %d min %d err %v", present, count, first, min, err)
	}
	off, stride, _, version, present, err := trunCTSLayout(trun, box)
	if err != nil || !present {
		t.Fatalf("layout: %v", err)
	}
	var got []int64
	for i := 0; i < count; i++ {
		got = append(got, readCTS(trun, off+i*stride, version))
	}
	if want := []int64{1072, 2144, 0, 544}; !reflect.DeepEqual(got, want) {
		t.Errorf("cts = %v, want %v", got, want)
	}

	// No composition offsets: nothing to report or shift.
	audio := testTrun(0, 0x301, 0, 0, [][3]int32{{1024, 280, 0}})
	abox := mp4Box{typ: "trun", off: 0, size: len(audio), hdr: 8}
	present, count, _, _, err = trunCTS(audio, abox)
	if err != nil || present || count != 1 {
		t.Errorf("audio trun: present %v count %d err %v", present, count, err)
	}
	if err := shiftTrunCTS(audio, abox, 5); err != nil {
		t.Errorf("shift of a cts-less trun: %v", err)
	}

	// v0 offsets are unsigned.
	v0 := testTrun(0, 0x800, 0, 0, [][3]int32{{0, 0, 1}, {0, 0, 2}})
	v0box := mp4Box{typ: "trun", off: 0, size: len(v0), hdr: 8}
	if _, _, first, min, err := trunCTS(v0, v0box); err != nil || first != 1 || min != 1 {
		t.Fatalf("v0 cts: first %d min %d err %v", first, min, err)
	}
	if err := shiftTrunCTS(v0, v0box, math.MaxInt32); err == nil {
		t.Error("shiftTrunCTS accepted an int32 overflow")
	}
}

func TestParseTraf(t *testing.T) {
	samples := [][3]int32{{528, 10, 0}, {544, 20, 1072}, {528, 30, -1072}}
	trun := testTrun(1, 0xB05, 0, 0, samples)
	traf := testTraf(1, 8000, trun)
	ti, err := parseTraf(traf, mp4Box{typ: "traf", off: 0, size: len(traf), hdr: 8})
	if err != nil {
		t.Fatalf("parseTraf: %v", err)
	}
	if ti.track != 1 || ti.count != 3 || ti.tfdt != 8000 || ti.tfdtVer != 1 {
		t.Errorf("trafInfo = %+v", ti)
	}
	if !ti.ctsPresent || ti.ctsFirst != 0 || ti.ctsMin != -1072 {
		t.Errorf("cts = present %v first %d min %d", ti.ctsPresent, ti.ctsFirst, ti.ctsMin)
	}

	audio := testTraf(2, 9000, testTrun(0, 0x301, 0, 0, [][3]int32{{1024, 5, 0}}))
	ti, err = parseTraf(audio, mp4Box{typ: "traf", off: 0, size: len(audio), hdr: 8})
	if err != nil || ti.track != 2 || ti.ctsPresent || ti.tfdt != 9000 {
		t.Errorf("audio trafInfo = %+v err %v", ti, err)
	}
}

func TestPatchMoof(t *testing.T) {
	video := testTraf(1, 8000, testTrun(1, 0xB05, 0, 0, [][3]int32{
		{528, 10, 0}, {544, 20, 1072}, {528, 30, -1072}, {544, 40, -528},
	}))
	audio := testTraf(2, 8000, testTrun(0, 0x301, 0, 0, [][3]int32{
		{1024, 5, 0}, {1024, 6, 0},
	}))
	moof := testMoof(7, video, audio)
	if err := patchMoof(moof, map[uint32]int64{1: 1000, 2: -500}, 42); err != nil {
		t.Fatalf("patchMoof: %v", err)
	}
	boxes, err := walkBoxes(moof, 0, len(moof))
	if err != nil || len(boxes) != 1 {
		t.Fatalf("moof walk: %v", err)
	}
	children, err := walkBoxes(moof, boxes[0].off+boxes[0].hdr, boxes[0].off+boxes[0].size)
	if err != nil {
		t.Fatalf("children: %v", err)
	}
	got := map[uint32]trafInfo{}
	for _, c := range children {
		if c.typ == "traf" {
			ti, err := parseTraf(moof, c)
			if err != nil {
				t.Fatalf("parseTraf: %v", err)
			}
			got[ti.track] = ti
		}
		if c.typ == "mfhd" {
			p := c.off + c.hdr
			if seq := binary.BigEndian.Uint32(moof[p+4:]); seq != 42 {
				t.Errorf("mfhd sequence = %d, want 42", seq)
			}
		}
	}
	if ti := got[1]; ti.tfdt != 9000 || ti.ctsMin != 0 || ti.ctsFirst != 1072 {
		t.Errorf("video traf = %+v", ti)
	}
	if ti := got[2]; ti.tfdt != 7500 || ti.ctsPresent {
		t.Errorf("audio traf = %+v", ti)
	}

	if err := patchMoof(moof, map[uint32]int64{1: -9001, 2: 0}, 1); err == nil {
		t.Error("patchMoof accepted a negative tfdt")
	}
	if err := patchMoof(moof, map[uint32]int64{1: 0}, 1); err == nil {
		t.Error("patchMoof accepted a missing track delta")
	}
}

func TestPatchMoofV0Overflow(t *testing.T) {
	tfhd := fullBox("tfhd", 0, 0x020038, be32(1), be32(0), be32(0), be32(0))
	tfdt := fullBox("tfdt", 0, 0, be32(0xFFFFFF00))
	trun := testTrun(1, 0x800, 0, 0, [][3]int32{{0, 0, 0}})
	moof := testMoof(1, boxOf("traf", tfhd, tfdt, trun))
	if err := patchMoof(moof, map[uint32]int64{1: 1000}, 1); err == nil {
		t.Fatal("patchMoof accepted a tfdt v0 overflow")
	}
}

func TestFreeBoxHeader(t *testing.T) {
	hdr, err := freeBoxHeader(100)
	if err != nil {
		t.Fatalf("freeBoxHeader: %v", err)
	}
	if len(hdr) != 8 || binary.BigEndian.Uint32(hdr) != 100 || string(hdr[4:]) != "free" {
		t.Errorf("free header = % X", hdr)
	}
	hdr, err = freeBoxHeader(0x100000000)
	if err != nil {
		t.Fatalf("freeBoxHeader large: %v", err)
	}
	if len(hdr) != 16 || binary.BigEndian.Uint32(hdr) != 1 ||
		binary.BigEndian.Uint64(hdr[8:]) != 0x100000000 {
		t.Errorf("large free header = % X", hdr)
	}
	if _, err := freeBoxHeader(7); err == nil {
		t.Error("freeBoxHeader accepted a 7-byte hole")
	}
}

func TestTrackTimescalesAndHandlers(t *testing.T) {
	moov := boxOf("moov",
		testTrak(1, 16000, "vide"),
		testTrak(2, 44100, "soun"),
	)
	scales := trackTimescales(moov)
	if want := map[uint32]uint64{1: 16000, 2: 44100}; !reflect.DeepEqual(scales, want) {
		t.Errorf("timescales = %v, want %v", scales, want)
	}
	handlers := initTrackHandlers(moov)
	if want := map[uint32]string{1: "vide", 2: "soun"}; !reflect.DeepEqual(handlers, want) {
		t.Errorf("handlers = %v, want %v", handlers, want)
	}
}

func TestRounding(t *testing.T) {
	cases := []struct {
		ns    int64
		scale uint64
		round int64
		ceil  int64
	}{
		{0, 16000, 0, 0},
		{1_000_000_000, 16000, 16000, 16000},
		{33_250_000, 16000, 532, 532},
		{33_333_333, 16000, 533, 534},
		{-23_219_954, 44100, -1024, -1023},
		{-1, 16000, 0, 0},
		{-100_000_000, 16000, -1600, -1600},
	}
	for _, tc := range cases {
		if got := roundTicks(tc.ns, tc.scale); got != tc.round {
			t.Errorf("roundTicks(%d, %d) = %d, want %d", tc.ns, tc.scale, got, tc.round)
		}
		if got := ceilTicks(tc.ns, tc.scale); got != tc.ceil {
			t.Errorf("ceilTicks(%d, %d) = %d, want %d", tc.ns, tc.scale, got, tc.ceil)
		}
	}
	if got := ceilNS(1072, 16000); got != 67_000_000 {
		t.Errorf("ceilNS(1072, 16000) = %d", got)
	}
	if got := ceilNS(1024, 44100); got != 23_219_955 {
		t.Errorf("ceilNS(1024, 44100) = %d", got)
	}
}

func TestReadBoxHeaderAt(t *testing.T) {
	buf := concat(boxOf("ftyp", []byte("isom")), boxOf("moof"))
	typ, size, hdr, err := readBoxHeaderAt(bytes.NewReader(buf), 0)
	if err != nil || typ != "ftyp" || size != 12 || hdr != 8 {
		t.Fatalf("header = %q %d %d %v", typ, size, hdr, err)
	}
	if _, _, _, err := readBoxHeaderAt(bytes.NewReader(buf), int64(len(buf))-3); err == nil {
		t.Error("readBoxHeaderAt accepted a truncated header")
	}
}
