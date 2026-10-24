package stitch

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The fault tests share one completed Run (a real fixture through the whole
// spike) and inject one fault at a time into a copy of the assembled file.
// Each fault must trip the specific detector that guards that property.

var (
	faultOnce sync.Once
	faultPlan *Plan
	faultRep  *Report
	faultErr  error
)

func faultFixture(t *testing.T) (*Plan, *Report) {
	t.Helper()
	path := fixture(t)
	faultOnce.Do(func() {
		opt := planOpts()
		faultPlan, faultErr = BuildPlan(context.Background(), path, opt, nil)
		if faultErr != nil {
			return
		}
		out := filepath.Join(fixtureDir, "fault-base.mp4")
		faultRep, faultErr = Run(context.Background(), faultPlan, out, opt)
	})
	if faultErr != nil {
		t.Fatalf("shared run: %v", faultErr)
	}
	return faultPlan, faultRep
}

// copyOutput returns a mutable copy of the assembled file.
func copyOutput(t *testing.T, src string) string {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "mutated.mp4")
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

func findCheck(checks []Check, name string) (Check, bool) {
	for _, c := range checks {
		if c.Name == name {
			return c, true
		}
	}
	return Check{}, false
}

func requireCheck(t *testing.T, checks []Check, name string, wantOK bool) {
	t.Helper()
	c, ok := findCheck(checks, name)
	if !ok {
		t.Fatalf("no %q check in the report", name)
	}
	if c.OK != wantOK {
		t.Errorf("check %q OK = %v, want %v (%s)", name, c.OK, wantOK, c.Detail)
		return
	}
	t.Logf("check %q: OK=%v (%s)", name, c.OK, c.Detail)
}

func verifyMutated(t *testing.T, plan *Plan, base *Report, path string) []Check {
	t.Helper()
	rep := &Report{
		Plan:      plan,
		OutPath:   path,
		Fragments: base.Fragments,
		PadNS:     base.PadNS,
		Stats:     base.Stats,
	}
	return verifyAssembly(context.Background(), plan, rep, Options{})
}

// moofAt returns the offset, size, and header length of the index-th moof.
func moofAt(t *testing.T, f *os.File, fileSize int64, index int) (off, size, hdr int64) {
	t.Helper()
	seen := 0
	for at := int64(0); at+8 <= fileSize; {
		typ, sz, h, err := readBoxHeaderAt(f, at)
		if err != nil {
			t.Fatalf("box header at %d: %v", at, err)
		}
		if sz < 0 {
			sz = fileSize - at
		}
		if sz < h || at+sz > fileSize {
			t.Fatalf("box %s at %d overruns the file", typ, at)
		}
		if typ == "moof" {
			if seen == index {
				return at, sz, h
			}
			seen++
		}
		at += sz
	}
	t.Fatalf("the file has no moof %d", index)
	return 0, 0, 0
}

func readMoof(t *testing.T, path string, index int) (*os.File, []byte, int64) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	off, size, _ := moofAt(t, f, st.Size(), index)
	buf := make([]byte, size)
	if err := readFull(f, buf, off); err != nil {
		f.Close()
		t.Fatal(err)
	}
	return f, buf, off
}

// moofChildren returns the boxes directly below a one-moof buffer.
func moofChildren(t *testing.T, buf []byte) []mp4Box {
	t.Helper()
	top, err := walkBoxes(buf, 0, len(buf))
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 1 || top[0].typ != "moof" {
		t.Fatalf("expected one moof box, got %+v", top)
	}
	children, err := walkBoxes(buf, top[0].off+top[0].hdr, top[0].off+top[0].size)
	if err != nil {
		t.Fatal(err)
	}
	return children
}

// TestFaultMidClusterStart: a segment whose SrcOff points inside its cluster
// (a Timecode child, not the Cluster) must be rejected by the anchor walk
// instead of silently producing no anchors.
func TestFaultMidClusterStart(t *testing.T) {
	plan, _ := faultFixture(t)
	f, err := os.Open(plan.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	kf := plan.Keyframes[0]
	seg := plan.Segments[0]
	if kf.PayloadPos <= kf.ClusterPos || kf.PayloadPos >= seg.SrcEnd {
		t.Fatalf("fixture has no usable mid-cluster offset (cluster %d, payload %d)", kf.ClusterPos, kf.PayloadPos)
	}
	_, err = firstBlockTimes(f, kf.PayloadPos, kf.PayloadPos, seg.SrcEnd,
		plan.TimecodeScaleNS, map[uint64]bool{plan.VideoTrack: true})
	if err == nil {
		t.Fatal("firstBlockTimes accepted a mid-cluster segment start")
	}
	t.Logf("rejected: %v", err)
}

// TestFaultCorruptAvcC: zeroing the codec configuration must make the final
// full decode fail.
func TestFaultCorruptAvcC(t *testing.T) {
	_, base := faultFixture(t)
	path := copyOutput(t, base.OutPath)

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	i := bytes.Index(b, []byte("avcC"))
	if i < 4 {
		t.Fatal("the assembled file has no avcC box")
	}
	size := int(binary.BigEndian.Uint32(b[i-4:]))
	if size < 8 || i-4+size > len(b) {
		t.Fatalf("avcC box size %d is invalid", size)
	}
	for k := i + 4; k < i-4+size; k++ {
		b[k] = 0
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := fullDecode(context.Background(), path); err == nil {
		t.Fatal("fullDecode accepted a corrupted avcC")
	} else {
		t.Logf("rejected: %v", err)
	}
}

// TestFaultTrunSampleCount: one fewer sample in the first trun must fail the
// exact packet-count check.
func TestFaultTrunSampleCount(t *testing.T) {
	plan, base := faultFixture(t)
	path := copyOutput(t, base.OutPath)

	f, buf, off := readMoof(t, path, 0)
	defer f.Close()
	children := moofChildren(t, buf)
	var traf mp4Box
	for _, c := range children {
		if c.typ == "traf" {
			traf = c
			break
		}
	}
	if traf.typ == "" {
		t.Fatal("the first moof has no traf")
	}
	sub, err := walkBoxes(buf, traf.off+traf.hdr, traf.off+traf.size)
	if err != nil {
		t.Fatal(err)
	}
	var trun mp4Box
	for _, c := range sub {
		if c.typ == "trun" {
			trun = c
			break
		}
	}
	if trun.typ == "" {
		t.Fatal("the first traf has no trun")
	}
	p := trun.off + trun.hdr
	if p+8 > trun.off+trun.size {
		t.Fatal("trun header is truncated")
	}
	count := binary.BigEndian.Uint32(buf[p+4:])
	if count == 0 {
		t.Fatal("the first trun has no samples")
	}
	binary.BigEndian.PutUint32(buf[p+4:], count-1)
	if _, err := f.WriteAt(buf, off); err != nil {
		t.Fatal(err)
	}

	checks := verifyMutated(t, plan, base, path)
	requireCheck(t, checks, "packet counts", false)
}

// TestFaultShiftedTFDT: moving a middle video fragment's tfdt below its
// predecessor must fail the monotonicity check.
func TestFaultShiftedTFDT(t *testing.T) {
	plan, base := faultFixture(t)
	path := copyOutput(t, base.OutPath)

	f, buf, off := readMoof(t, path, 5)
	defer f.Close()
	children := moofChildren(t, buf)
	patched := false
	for _, c := range children {
		if c.typ != "traf" {
			continue
		}
		ti, err := parseTraf(buf, c)
		if err != nil {
			t.Fatal(err)
		}
		if ti.track != 1 {
			continue
		}
		// The fixture's moof 5 video tfdt is 5 s (80000 ticks); -2 s puts it
		// below moof 4's 4 s without going negative.
		if err := patchTrafTFDT(buf, c, ti, -32000); err != nil {
			t.Fatal(err)
		}
		patched = true
	}
	if !patched {
		t.Fatal("the sixth moof has no video traf")
	}
	if _, err := f.WriteAt(buf, off); err != nil {
		t.Fatal(err)
	}

	checks := verifyMutated(t, plan, base, path)
	requireCheck(t, checks, "tfdt monotonic", false)
}

// TestFaultNegativeCTS: a negative composition offset must fail the cts check
// (the mov demuxer would otherwise derive a parse-order-dependent dts_shift).
func TestFaultNegativeCTS(t *testing.T) {
	plan, base := faultFixture(t)
	path := copyOutput(t, base.OutPath)

	f, buf, off := readMoof(t, path, 2)
	defer f.Close()
	children := moofChildren(t, buf)
	patched := false
	for _, c := range children {
		if c.typ != "traf" {
			continue
		}
		ti, err := parseTraf(buf, c)
		if err != nil {
			t.Fatal(err)
		}
		if ti.track != 1 {
			continue
		}
		if err := shiftTrafCTS(buf, c, -1000); err != nil {
			t.Fatal(err)
		}
		patched = true
	}
	if !patched {
		t.Fatal("the third moof has no video traf")
	}
	if _, err := f.WriteAt(buf, off); err != nil {
		t.Fatal(err)
	}

	checks := verifyMutated(t, plan, base, path)
	requireCheck(t, checks, "cts non-negative", false)
}
