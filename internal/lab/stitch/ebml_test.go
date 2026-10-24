package stitch

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestParseInfo(t *testing.T) {
	info := ebmlElem(idInfo,
		ebmlUint(idTimecodeScale, 1_000_000),
		ebmlFloat(idDuration, 24_000),
	)
	scale, dur, err := parseInfo(bytes.NewReader(info), 0)
	if err != nil {
		t.Fatalf("parseInfo: %v", err)
	}
	if scale != 1_000_000 {
		t.Errorf("scale = %d, want 1000000", scale)
	}
	if want := int64(24_000 * 1_000_000); dur != want {
		t.Errorf("duration = %d, want %d", dur, want)
	}
}

func TestParseTracksCodecDelay(t *testing.T) {
	tracks := ebmlElem(idTracks,
		ebmlElem(idTrackEntry,
			ebmlUint(idTrackNumber, 1),
			ebmlUint(idTrackType, 1),
			ebmlStr(idCodecID, "V_MPEG4/ISO/AVC"),
			ebmlUint(idDefaultDuration, 33_333_333),
		),
		ebmlElem(idTrackEntry,
			ebmlUint(idTrackNumber, 2),
			ebmlUint(idTrackType, 2),
			ebmlStr(idCodecID, "A_AAC"),
			ebmlUint(idCodecDelay, 23_219_954),
		),
	)
	got, err := parseTracks(bytes.NewReader(tracks), 0)
	if err != nil {
		t.Fatalf("parseTracks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d tracks, want 2", len(got))
	}
	if got[0].number != 1 || got[0].typ != 1 || got[0].codec != "V_MPEG4/ISO/AVC" {
		t.Errorf("video track = %+v", got[0])
	}
	if got[0].defaultDurationNS != 33_333_333 {
		t.Errorf("default duration = %d", got[0].defaultDurationNS)
	}
	if got[1].number != 2 || got[1].typ != 2 || got[1].codec != "A_AAC" {
		t.Errorf("audio track = %+v", got[1])
	}
	if got[1].codecDelayNS != 23_219_954 {
		t.Errorf("codec delay = %d, want 23219954", got[1].codecDelayNS)
	}
}

func TestFirstBlockTimes(t *testing.T) {
	c1 := ebmlElem(idCluster,
		ebmlUint(idTimecode, 2000),
		ebmlElem(idSimpleBlock, []byte{0x81}), // truncated header: tolerated
		ebmlElem(idSimpleBlock, mkvBlock(1, -23)),
		ebmlElem(idSimpleBlock, mkvBlock(2, 0)),
		ebmlElem(idBlockGroup, ebmlElem(idBlock, mkvBlock(1, 100))),
	)
	c2 := ebmlElem(idCluster,
		ebmlUint(idTimecode, 4000),
		ebmlElem(idSimpleBlock, mkvBlock(3, 0)),
	)
	buf := concat(c1, c2)
	got, err := firstBlockTimes(bytes.NewReader(buf), 0, 0, int64(len(buf)), 1e6,
		map[uint64]bool{1: true, 2: true, 3: true})
	if err != nil {
		t.Fatalf("firstBlockTimes: %v", err)
	}
	want := map[uint64]int64{
		1: (2000 - 23) * 1_000_000,
		2: 2000 * 1_000_000,
		3: 4000 * 1_000_000,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("anchors = %v, want %v", got, want)
	}
}

func TestFirstBlockTimesMidClusterIsRejected(t *testing.T) {
	// A zero payload bytes inside a cluster header: parsing at this offset must
	// fail instead of inventing anchors (this is the guard against a bad plan).
	buf := concat(ebmlID(idCluster), ebmlSize(8), make([]byte, 8))
	payload := int64(len(ebmlID(idCluster)) + len(ebmlSize(8)))
	if _, err := firstBlockTimes(bytes.NewReader(buf), payload, payload, int64(len(buf)), 1e6,
		map[uint64]bool{1: true}); err == nil {
		t.Fatal("firstBlockTimes accepted a mid-cluster start")
	}
}

func kfsEvery(step time.Duration, n int) []Keyframe {
	out := make([]Keyframe, n)
	for i := range out {
		out[i] = Keyframe{TimeNS: int64(step) * int64(i), ClusterPos: int64(1000 + i*100)}
	}
	return out
}

func TestSelectBoundaries(t *testing.T) {
	kfs := kfsEvery(2*time.Second, 12) // 0..22 s
	tol := 500 * time.Millisecond
	cases := []struct {
		name   string
		segDur time.Duration
		dur    int64
		want   []int
	}{
		{"8s", 8 * time.Second, int64(24 * time.Second), []int{0, 4, 8}},
		{"4s", 4 * time.Second, int64(24 * time.Second), []int{0, 2, 4, 6, 8, 10}},
		{"6s", 6 * time.Second, int64(24 * time.Second), []int{0, 3, 6, 9}},
		{"12s", 12 * time.Second, int64(24 * time.Second), []int{0, 6}},
		{"single", time.Minute, int64(24 * time.Second), []int{0}},
		{"unknown duration", 8 * time.Second, 0, []int{0, 4, 8}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectBoundaries(kfs, tc.dur, tc.segDur, tol)
			if err != nil {
				t.Fatalf("selectBoundaries: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("picks = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSelectBoundariesTolerance(t *testing.T) {
	kfs := []Keyframe{
		{TimeNS: 0}, {TimeNS: int64(2200 * time.Millisecond)}, // 2.2 s
		{TimeNS: int64(4100 * time.Millisecond)}, // 4.1 s
		{TimeNS: int64(6300 * time.Millisecond)}, // 6.3 s
		{TimeNS: int64(8200 * time.Millisecond)}, // 8.2 s
	}
	got, err := selectBoundaries(kfs, int64(10*time.Second), 4*time.Second, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("selectBoundaries: %v", err)
	}
	if want := []int{0, 2, 4}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picks = %v, want %v", got, want)
	}
}

func TestSelectBoundariesRefusesGaps(t *testing.T) {
	kfs := kfsEvery(2*time.Second, 12)
	_, err := selectBoundaries(kfs, int64(24*time.Second), 3*time.Second, 500*time.Millisecond)
	if err == nil {
		t.Fatal("selectBoundaries accepted a 3 s window with no keyframe within 500 ms")
	}
	if !strings.Contains(err.Error(), "no keyframe within") {
		t.Errorf("error = %v", err)
	}
}

func TestSegmentsFromPicks(t *testing.T) {
	// Each cluster is [Timecode at PayloadPos][prefix...][KeyPos...].
	kfs := []Keyframe{
		{TimeNS: 0, ClusterPos: 100, KeyPos: 120, PayloadPos: 108, ClusterEnd: 180, TCOff: 108, TCEnd: 112},
		{TimeNS: int64(4 * time.Second), ClusterPos: 200, KeyPos: 220, PayloadPos: 208, ClusterEnd: 280, TCOff: 208, TCEnd: 212},
		{TimeNS: int64(8 * time.Second), ClusterPos: 300, KeyPos: 320, PayloadPos: 308, ClusterEnd: 380, TCOff: 308, TCEnd: 312},
		{TimeNS: int64(12 * time.Second), ClusterPos: 400, KeyPos: 420, PayloadPos: 408, ClusterEnd: 480, TCOff: 408, TCEnd: 412},
	}
	segs := segmentsFromPicks(kfs, []int{0, 2, 3}, 999)
	want := []Segment{
		{
			Index: 0, SrcOff: 100, SrcEnd: 320, AnchorFrom: 108, FirstPTSNS: 0,
			Pieces: []Piece{
				{Ranges: [][2]int64{{108, 180}}, Wrap: true},
				{Ranges: [][2]int64{{180, 300}}},
				{Ranges: [][2]int64{{308, 320}}, Wrap: true},
			},
		},
		{
			Index: 1, SrcOff: 300, SrcEnd: 420, AnchorFrom: 320, FirstPTSNS: int64(8 * time.Second),
			Pieces: []Piece{
				{Ranges: [][2]int64{{308, 312}, {320, 380}}, Wrap: true},
				{Ranges: [][2]int64{{380, 400}}},
				{Ranges: [][2]int64{{408, 420}}, Wrap: true},
			},
		},
		{
			Index: 2, SrcOff: 400, SrcEnd: 999, AnchorFrom: 420, FirstPTSNS: int64(12 * time.Second),
			Pieces: []Piece{
				{Ranges: [][2]int64{{408, 412}, {420, 480}}, Wrap: true},
				{Ranges: [][2]int64{{480, 999}}},
			},
		},
	}
	if !reflect.DeepEqual(segs, want) {
		t.Errorf("segments = %+v, want %+v", segs, want)
	}
	if got, want := extractBytes(segs), int64((180-108)+(300-180)+(320-308)+(312-308)+(380-320)+(400-380)+(420-408)+(412-408)+(480-420)+(999-480)); got != want {
		t.Errorf("extractBytes = %d, want %d", got, want)
	}
}

func TestPlanCoverage(t *testing.T) {
	kfs := []Keyframe{{TimeNS: int64(22 * time.Second)}}
	if !planCoverage(kfs, int64(24*time.Second)) {
		t.Error("22 s of 24 s should cover the file")
	}
	kfs[0].TimeNS = int64(20 * time.Second)
	if planCoverage(kfs, int64(24*time.Second)) {
		t.Error("20 s of 24 s should fall back to the scan")
	}
	if planCoverage(nil, int64(24*time.Second)) {
		t.Error("no keyframes should not cover the file")
	}
	if !planCoverage(kfs, 0) {
		t.Error("an unknown duration should accept the index")
	}
}

// testFileWithSegment builds EBML header + Segment around payload, with either
// a minimal known-size vint or the 8-byte unknown-size vint.
func testFileWithSegment(t *testing.T, payload []byte, unknown bool) []byte {
	t.Helper()
	var seg []byte
	seg = append(seg, ebmlID(idSegment)...)
	if unknown {
		seg = append(seg, unknownSizeVint...)
	} else {
		seg = append(seg, ebmlSize(uint64(len(payload)))...)
	}
	seg = append(seg, payload...)
	return concat(ebmlElem(idEBML), seg)
}

func testHeaderPayload() (payload []byte, seekTotal int64) {
	seekHead := ebmlElem(idSeekHead)
	info := ebmlElem(idInfo, ebmlUint(idTimecodeScale, 1_000_000), ebmlFloat(idDuration, 12_000))
	tracks := ebmlElem(idTracks, ebmlElem(idTrackEntry,
		ebmlUint(idTrackNumber, 1),
		ebmlUint(idTrackType, 1),
		ebmlStr(idCodecID, "V_MPEG4/ISO/AVC"),
	))
	cluster := ebmlElem(idCluster, ebmlUint(idTimecode, 0), ebmlElem(idSimpleBlock, mkvBlock(1, 0)))
	cluster2 := ebmlElem(idCluster, ebmlUint(idTimecode, 2000), ebmlElem(idSimpleBlock, mkvBlock(1, 0)))
	return concat(seekHead, info, tracks, cluster, cluster2), int64(len(seekHead))
}

func TestCacheHeaderRebuild(t *testing.T) {
	payload, seekTotal := testHeaderPayload()
	file := testFileWithSegment(t, payload, false)
	r := bytes.NewReader(file)
	h, err := parseFileHeader(r, int64(len(file)))
	if err != nil {
		t.Fatalf("parseFileHeader: %v", err)
	}
	if h.segSize.sizeLen >= 8 {
		t.Fatalf("test file has an 8-byte Segment size; want the rebuild path")
	}
	ix, err := scanTopLevel(r, h, false)
	if err != nil {
		t.Fatalf("scanTopLevel: %v", err)
	}
	if ix.firstCluster == 0 || ix.seekHeadOff == 0 {
		t.Fatalf("index incomplete: %+v", ix)
	}
	buf, err := cacheHeader(r, h, ix, ix.firstCluster)
	if err != nil {
		t.Fatalf("cacheHeader: %v", err)
	}
	delta := int64(8 - h.segSize.sizeLen)
	if want := ix.firstCluster + delta; int64(len(buf)) != want {
		t.Fatalf("header length = %d, want %d", len(buf), want)
	}
	if !bytes.Equal(buf[h.segSize.sizeOff:h.segSize.sizeOff+8], unknownSizeVint) {
		t.Errorf("Segment size was not rewritten to unknown")
	}
	voidOff := ix.seekHeadOff + delta
	if want := file[h.segStart:ix.seekHeadOff]; !bytes.Equal(buf[h.segSize.sizeOff+8:voidOff], want) {
		t.Errorf("header payload before the SeekHead differs after the Segment size rewrite")
	}
	if want := file[ix.seekHeadOff+seekTotal : ix.firstCluster]; !bytes.Equal(buf[voidOff+seekTotal:], want) {
		t.Errorf("header payload after the SeekHead differs after the Segment size rewrite")
	}
	e, err := readElem(bytes.NewReader(buf), voidOff)
	if err != nil {
		t.Fatalf("voided SeekHead: %v", err)
	}
	if e.id != idVoid || e.end()-voidOff != seekTotal {
		t.Errorf("SeekHead void = ID 0x%X size %d, want Void of %d", e.id, e.end()-voidOff, seekTotal)
	}
	h2, err := parseFileHeader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		t.Fatalf("rewritten header no longer parses: %v", err)
	}
	if !h2.segSize.unknown {
		t.Error("rewritten Segment size is not unknown")
	}
}

func TestCacheHeaderInPlace(t *testing.T) {
	payload, _ := testHeaderPayload()
	file := testFileWithSegment(t, payload, true)
	r := bytes.NewReader(file)
	h, err := parseFileHeader(r, int64(len(file)))
	if err != nil {
		t.Fatalf("parseFileHeader: %v", err)
	}
	if h.segSize.sizeLen != 8 {
		t.Fatalf("Segment size vint = %d bytes, want 8", h.segSize.sizeLen)
	}
	ix, err := scanTopLevel(r, h, false)
	if err != nil {
		t.Fatalf("scanTopLevel: %v", err)
	}
	buf, err := cacheHeader(r, h, ix, ix.firstCluster)
	if err != nil {
		t.Fatalf("cacheHeader: %v", err)
	}
	if !bytes.Equal(buf, file[:ix.firstCluster]) {
		t.Error("in-place header rewrite changed bytes outside the size vint")
	}
}

func TestVoidElement(t *testing.T) {
	for _, total := range []int64{2, 8, 10, 127, 128, 300, 16384} {
		buf := make([]byte, total)
		if err := voidElement(buf, 0, total); err != nil {
			t.Fatalf("voidElement(%d): %v", total, err)
		}
		e, err := readElem(bytes.NewReader(buf), 0)
		if err != nil {
			t.Fatalf("readElem(void %d): %v", total, err)
		}
		if e.id != idVoid || e.unknown || e.end() != total {
			t.Errorf("void of %d: id 0x%X unknown %v end %d", total, e.id, e.unknown, e.end())
		}
	}
	if err := voidElement(make([]byte, 1), 0, 1); err == nil {
		t.Error("voidElement accepted a 1-byte element")
	}
}
