package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"siphon/internal/formats"
	"siphon/internal/siphonerr"
)

// --- helpers ---------------------------------------------------------------

func makePNG(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if alpha && x == 0 && y == 0 {
				a = 0
			}
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 37), G: uint8(y * 53), B: 99, A: a})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 11), G: uint8(y * 17), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// tinyEXIF builds a minimal little-endian TIFF with one IFD entry: the
// orientation tag.
func tinyEXIF(orientation uint16) []byte {
	b := make([]byte, 26)
	copy(b[0:2], "II")
	binary.LittleEndian.PutUint16(b[2:4], 42)
	binary.LittleEndian.PutUint32(b[4:8], 8)  // IFD0 offset
	binary.LittleEndian.PutUint16(b[8:10], 1) // entry count
	e := b[10:22]
	binary.LittleEndian.PutUint16(e[0:2], 0x0112) // Orientation
	binary.LittleEndian.PutUint16(e[2:4], 3)      // SHORT
	binary.LittleEndian.PutUint32(e[4:8], 1)      // count
	binary.LittleEndian.PutUint16(e[8:10], orientation)
	binary.LittleEndian.PutUint32(b[22:26], 0) // next IFD
	return b
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// --- orientation -----------------------------------------------------------

func TestOrientMappings(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 2, 3))
	values := []uint8{1, 2, 3, 4, 5, 6}
	i := 0
	for y := 0; y < 3; y++ {
		for x := 0; x < 2; x++ {
			src.SetGray(x, y, color.Gray{Y: values[i]})
			i++
		}
	}
	cases := []struct {
		orientation int
		want        [][]uint8
	}{
		{2, [][]uint8{{2, 1}, {4, 3}, {6, 5}}},
		{3, [][]uint8{{6, 5}, {4, 3}, {2, 1}}},
		{4, [][]uint8{{5, 6}, {3, 4}, {1, 2}}},
		{5, [][]uint8{{1, 3, 5}, {2, 4, 6}}},
		{6, [][]uint8{{5, 3, 1}, {6, 4, 2}}},
		{7, [][]uint8{{6, 4, 2}, {5, 3, 1}}},
		{8, [][]uint8{{2, 4, 6}, {1, 3, 5}}},
	}
	for _, c := range cases {
		got := Orient(src, c.orientation)
		if got.Bounds().Dy() != len(c.want) || got.Bounds().Dx() != len(c.want[0]) {
			t.Fatalf("orientation %d: got %v, want %dx%d",
				c.orientation, got.Bounds(), len(c.want[0]), len(c.want))
		}
		for y, row := range c.want {
			for x, want := range row {
				r, _, _, _ := got.At(x, y).RGBA()
				if uint8(r>>8) != want {
					t.Fatalf("orientation %d at (%d,%d): got %d, want %d",
						c.orientation, x, y, uint8(r>>8), want)
				}
			}
		}
	}
}

func TestEXIFOrientationParseAndPatch(t *testing.T) {
	exif := tinyEXIF(6)
	if got := Orientation(exif); got != 6 {
		t.Fatalf("Orientation = %d, want 6", got)
	}
	patched := SetOrientation(exif, 1)
	if got := Orientation(patched); got != 1 {
		t.Fatalf("patched Orientation = %d, want 1", got)
	}
	if Orientation(exif) != 6 {
		t.Fatal("SetOrientation modified the input")
	}
	if Orientation(nil) != 1 || Orientation([]byte("garbage")) != 1 {
		t.Fatal("absent/invalid EXIF should read as orientation 1")
	}
	if Orientation(tinyEXIF(99)) != 1 {
		t.Fatal("out-of-range orientation should read as 1")
	}
}

// --- metadata round trips ---------------------------------------------------

func TestJPEGMetadataRoundTrip(t *testing.T) {
	profile := bytes.Repeat([]byte{0xAB}, 70_000) // forces two APP2 segments
	meta := &Metadata{
		ICC:  profile,
		EXIF: tinyEXIF(6),
		XMP:  []byte("<x:xmpmeta>hello</x:xmpmeta>"),
	}
	data, err := AddMetadata(makeJPEG(t, 16, 16), formats.JPG, meta)
	if err != nil {
		t.Fatal(err)
	}
	path := writeTemp(t, "meta.jpg", data)

	got, _, err := Extract(path, formats.JPG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.ICC, profile) {
		t.Fatalf("ICC mismatch: got %d bytes, want %d", len(got.ICC), len(profile))
	}
	if Orientation(got.EXIF) != 6 {
		t.Fatalf("EXIF orientation = %d, want 6", Orientation(got.EXIF))
	}
	if !bytes.Equal(got.XMP, meta.XMP) {
		t.Fatalf("XMP mismatch: %q", got.XMP)
	}
}

func TestPNGMetadataRoundTrip(t *testing.T) {
	profile := bytes.Repeat([]byte{0xCD}, 1024)
	meta := &Metadata{
		ICC:  profile,
		EXIF: tinyEXIF(3),
		XMP:  []byte("<x:xmpmeta>png</x:xmpmeta>"),
	}
	data, err := AddMetadata(makePNG(t, 8, 8, false), formats.PNG, meta)
	if err != nil {
		t.Fatal(err)
	}
	path := writeTemp(t, "meta.png", data)

	got, _, err := Extract(path, formats.PNG)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.ICC, profile) {
		t.Fatalf("ICC mismatch: got %d bytes, want %d", len(got.ICC), len(profile))
	}
	if Orientation(got.EXIF) != 3 {
		t.Fatalf("EXIF orientation = %d, want 3", Orientation(got.EXIF))
	}
	if !bytes.Equal(got.XMP, meta.XMP) {
		t.Fatalf("XMP mismatch: %q", got.XMP)
	}
}

func TestJPEGMetadataTooLarge(t *testing.T) {
	meta := &Metadata{EXIF: bytes.Repeat([]byte{0x00}, 70_000)}
	if _, err := AddMetadata(makeJPEG(t, 4, 4), formats.JPG, meta); !siphonerr.IsCode(err, siphonerr.MetadataDrop) {
		t.Fatalf("got %v, want E_METADATA_DROP", err)
	}
}

// --- WebP container ---------------------------------------------------------

func riffChunk(fourcc string, payload []byte) []byte {
	b := make([]byte, 8+len(payload))
	copy(b[0:4], fourcc)
	binary.LittleEndian.PutUint32(b[4:8], uint32(len(payload)))
	copy(b[8:], payload)
	if len(payload)%2 == 1 {
		b = append(b, 0)
	}
	return b
}

func riffWebP(chunks ...[]byte) []byte {
	body := []byte("WEBP")
	for _, c := range chunks {
		body = append(body, c...)
	}
	out := []byte("RIFF")
	var sz [4]byte
	binary.LittleEndian.PutUint32(sz[:], uint32(len(body)))
	out = append(out, sz[:]...)
	out = append(out, body...)
	return out
}

func TestWebPHeaderAndMetadata(t *testing.T) {
	vp8x := make([]byte, 10)
	vp8x[0] = 0x30 // alpha + ICC flags
	put24 := func(b []byte, v int) {
		b[0] = byte(v)
		b[1] = byte(v >> 8)
		b[2] = byte(v >> 16)
	}
	put24(vp8x[4:7], 320-1)
	put24(vp8x[7:10], 240-1)

	exif := tinyEXIF(6)
	data := riffWebP(
		riffChunk("VP8X", vp8x),
		riffChunk("ICCP", []byte("icc-profile-bytes")),
		riffChunk("EXIF", exif),
		riffChunk("XMP ", []byte("<xmp/>")),
	)
	path := writeTemp(t, "meta.webp", data)

	info, err := readWebP(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != 320 || info.Height != 240 {
		t.Fatalf("dimensions = %dx%d, want 320x240", info.Width, info.Height)
	}
	if !info.HasAlpha {
		t.Fatal("alpha flag not detected")
	}
	if string(info.Metadata.ICC) != "icc-profile-bytes" {
		t.Fatalf("ICC = %q", info.Metadata.ICC)
	}
	if Orientation(info.Metadata.EXIF) != 6 {
		t.Fatalf("EXIF orientation = %d", Orientation(info.Metadata.EXIF))
	}
	if string(info.Metadata.XMP) != "<xmp/>" {
		t.Fatalf("XMP = %q", info.Metadata.XMP)
	}
}

func TestWebPLosslessHeader(t *testing.T) {
	w, h := 5, 7
	bits := uint32(w-1) | uint32(h-1)<<14 | 1<<28 // alpha bit
	var payload [5]byte
	payload[0] = 0x2F
	binary.LittleEndian.PutUint32(payload[1:5], bits)

	path := writeTemp(t, "lossless.webp", riffWebP(riffChunk("VP8L", payload[:])))
	info, err := readWebP(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Width != w || info.Height != h || !info.HasAlpha {
		t.Fatalf("info = %+v, want %dx%d alpha", info, w, h)
	}
}

func TestAnimatedSourcesRefused(t *testing.T) {
	// APNG: acTL inserted after IHDR.
	pngData := makePNG(t, 4, 4, false)
	ihdrLen := int(binary.BigEndian.Uint32(pngData[8:12]))
	insert := 8 + 12 + ihdrLen
	actl := pngChunk("acTL", []byte{0, 0, 0, 1, 0, 0, 0, 0})
	animatedPNG := append(append(append([]byte{}, pngData[:insert]...), actl...), pngData[insert:]...)
	pngPath := writeTemp(t, "anim.png", animatedPNG)

	if _, _, _, err := Decode(context.Background(), pngPath, formats.PNG, filepath.Dir(pngPath)); !siphonerr.IsCode(err, siphonerr.FormatUnsupported) {
		t.Fatalf("APNG: got %v, want E_FORMAT_UNSUPPORTED", err)
	}

	animWebP := riffWebP(riffChunk("VP8X", []byte{0x02, 0, 0, 0, 3, 0, 0, 3, 0, 0}),
		riffChunk("ANIM", make([]byte, 6)))
	webpPath := writeTemp(t, "anim.webp", animWebP)
	if _, _, _, err := Decode(context.Background(), webpPath, formats.WEBP, filepath.Dir(webpPath)); !siphonerr.IsCode(err, siphonerr.FormatUnsupported) {
		t.Fatalf("animated WebP: got %v, want E_FORMAT_UNSUPPORTED", err)
	}
}

// --- misc -------------------------------------------------------------------

func TestHasTransparency(t *testing.T) {
	opaque := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			opaque.SetNRGBA(x, y, color.NRGBA{R: 1, G: 2, B: 3, A: 255})
		}
	}
	if HasTransparency(opaque) {
		t.Fatal("opaque image reported as transparent")
	}
	opaque.SetNRGBA(1, 1, color.NRGBA{R: 1, G: 2, B: 3, A: 0})
	if !HasTransparency(opaque) {
		t.Fatal("transparent pixel not detected")
	}
}

func TestParseBackground(t *testing.T) {
	for _, s := range []string{"white", "BLACK", "#fff", "#102030", "#102030ff"} {
		if _, err := ParseBackground(s); err != nil {
			t.Errorf("ParseBackground(%q): %v", s, err)
		}
	}
	for _, s := range []string{"", "not-a-colour", "#12345", "#10203080"} {
		if _, err := ParseBackground(s); err == nil {
			t.Errorf("ParseBackground(%q) unexpectedly succeeded", s)
		}
	}
}

func TestCompositeOverBackground(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	src.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 0})
	src.SetNRGBA(1, 0, color.NRGBA{R: 0, G: 0, B: 255, A: 255})
	dst := Composite(src, color.NRGBA{R: 255, G: 255, B: 255, A: 255})
	r, g, b, _ := dst.At(0, 0).RGBA()
	if uint8(r>>8) != 255 || uint8(g>>8) != 255 || uint8(b>>8) != 255 {
		t.Fatalf("transparent pixel did not take the background: %d %d %d", r>>8, g>>8, b>>8)
	}
}
