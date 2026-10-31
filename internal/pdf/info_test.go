package pdf

import (
	"math"
	"strings"
	"testing"

	"siphon/internal/siphonerr"
)

const singlePageInfo = `Title:          Report
Author:         A. Person
Pages:          1
Page size:      612 x 792
Page rot:       90
MediaBox:       0 0 612 792
CropBox:        10 20 600 780
PDF version:    1.7
Encrypted:      no
`

func TestParseInfoOutputSinglePage(t *testing.T) {
	info, err := ParseInfoOutput(singlePageInfo)
	if err != nil {
		t.Fatalf("ParseInfoOutput: %v", err)
	}
	if info.Pages != 1 {
		t.Errorf("Pages = %d, want 1", info.Pages)
	}
	if info.Rotation != 90 {
		t.Errorf("Rotation = %d, want 90", info.Rotation)
	}
	if info.PageSize != "612 x 792 pts" {
		t.Errorf("PageSize = %q", info.PageSize)
	}
	if info.PDFVersion != "1.7" {
		t.Errorf("PDFVersion = %q", info.PDFVersion)
	}
	if info.Encrypted {
		t.Error("Encrypted = true")
	}
	if !info.HasCropBox {
		t.Fatal("CropBox was not recognised")
	}
	if got, want := info.PageBox(), (Box{X1: 10, Y1: 20, X2: 600, Y2: 780}); got != want {
		t.Errorf("PageBox = %+v, want %+v", got, want)
	}
	if !info.MediaBox.Valid() || !info.CropBox.Valid() {
		t.Error("boxes should be valid")
	}
}

func TestPageBoxPrefersUsableCropBox(t *testing.T) {
	info := &Info{
		MediaBox:   Box{X1: 0, Y1: 0, X2: 612, Y2: 792},
		CropBox:    Box{X1: 0, Y1: 0, X2: 0, Y2: 100},
		HasCropBox: true,
	}
	if got := info.PageBox(); got != info.MediaBox {
		t.Fatalf("invalid CropBox used as PageBox: %+v", got)
	}
	info.CropBox = Box{X1: 10, Y1: 10, X2: 410, Y2: 410}
	if got := info.PageBox(); got != info.CropBox {
		t.Fatalf("PageBox = %+v, want CropBox", got)
	}
}

func TestParseInfoOutputCropBoxFallback(t *testing.T) {
	out := strings.Replace(singlePageInfo, "CropBox:        10 20 600 780\n", "", 1)
	info, err := ParseInfoOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasCropBox {
		t.Fatal("HasCropBox = true without a CropBox line")
	}
	if info.PageBox() != info.MediaBox {
		t.Fatalf("PageBox = %+v, want MediaBox", info.PageBox())
	}

	broken := strings.Replace(singlePageInfo, "CropBox:        10 20 600 780",
		"CropBox:        10 20 10 30", 1)
	info, err = ParseInfoOutput(broken)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasCropBox {
		t.Fatal("a zero-width CropBox must fall back to MediaBox")
	}
}

func TestParseInfoOutputRefusals(t *testing.T) {
	tests := []struct {
		name string
		out  string
		code siphonerr.Code
	}{
		{"missing pages", "MediaBox: 0 0 612 792\n", siphonerr.PDFInvalid},
		{"zero pages", "Pages: 0\nMediaBox: 0 0 612 792\n", siphonerr.PDFInvalid},
		{"bad pages", "Pages: many\nMediaBox: 0 0 612 792\n", siphonerr.PDFInvalid},
		{"missing mediabox", "Pages: 1\n", siphonerr.PDFInvalid},
		{"bad mediabox", "Pages: 1\nMediaBox: 0 0 x 792\n", siphonerr.PDFInvalid},
		{"three numbers", "Pages: 1\nMediaBox: 0 0 612\n", siphonerr.PDFInvalid},
		{"nan mediabox", "Pages: 1\nMediaBox: 0 0 NaN 792\n", siphonerr.PDFInvalid},
		{"zero size", "Pages: 1\nMediaBox: 0 0 0 792\n", siphonerr.PDFInvalid},
		{"bad rotation", "Pages: 1\nPage rot: 45\nMediaBox: 0 0 612 792\n", siphonerr.PDFInvalid},
		{"unparseable rotation", "Pages: 1\nPage rot: sideways\nMediaBox: 0 0 612 792\n", siphonerr.PDFInvalid},
		{"encrypted", "Pages: 1\nEncrypted: yes\nMediaBox: 0 0 612 792\n", siphonerr.PDFEncrypted},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseInfoOutput(tc.out)
			if !siphonerr.IsCode(err, tc.code) {
				t.Fatalf("got %v, want %s", err, tc.code)
			}
		})
	}
}

func TestParseInfoOutputMultipageIsReturned(t *testing.T) {
	out := "Pages: 7\nMediaBox: 0 0 612 792\n"
	info, err := ParseInfoOutput(out)
	if err != nil {
		t.Fatalf("multi-page must be classified by the caller: %v", err)
	}
	if info.Pages != 7 {
		t.Fatalf("Pages = %d, want 7", info.Pages)
	}
}

func TestRotatedSizeAndExpectedDimensions(t *testing.T) {
	info := &Info{MediaBox: Box{X1: 0, Y1: 0, X2: 100, Y2: 200}}
	if w, h := info.RotatedSize(); w != 100 || h != 200 {
		t.Fatalf("rotated size without rotation = %v × %v", w, h)
	}
	info.Rotation = 90
	if w, h := info.RotatedSize(); w != 200 || h != 100 {
		t.Fatalf("rotated size at 90 = %v × %v", w, h)
	}
	w, h := ExpectedDimensions(info, 72)
	if w != 200 || h != 100 {
		t.Fatalf("ExpectedDimensions(72) = %d × %d", w, h)
	}
	w, h = ExpectedDimensions(info, 36)
	if w != 100 || h != 50 {
		t.Fatalf("ExpectedDimensions(36) = %d × %d", w, h)
	}
	// Ceil, not round: 612 pt at 150 DPI is 1275 px; 1 pt extra is 3 px.
	letter := &Info{MediaBox: Box{X1: 0, Y1: 0, X2: 612, Y2: 792}}
	w, h = ExpectedDimensions(letter, 150)
	if w != 1275 || h != 1650 {
		t.Fatalf("letter at 150 DPI = %d × %d, want 1275 × 1650", w, h)
	}
	odd := &Info{MediaBox: Box{X1: 0, Y1: 0, X2: 1, Y2: 1}}
	if w, h := ExpectedDimensions(odd, 150); w != 3 || h != 3 {
		t.Fatalf("1 pt at 150 DPI = %d × %d, want ceil 2.083 = 3", w, h)
	}
}

func TestPixelCountAndCap(t *testing.T) {
	info := &Info{MediaBox: Box{X1: 0, Y1: 0, X2: 100, Y2: 200}}
	if got, want := PixelCount(info, 72), int64(20000); got != want {
		t.Fatalf("PixelCount = %d, want %d", got, want)
	}
	if err := CheckPixelCap(info, 72, 20000); err != nil {
		t.Fatalf("exact cap must pass: %v", err)
	}
	err := CheckPixelCap(info, 72, 19999)
	if !siphonerr.IsCode(err, siphonerr.PDFTooLarge) {
		t.Fatalf("got %v, want E_PDF_TOO_LARGE", err)
	}
	if !strings.Contains(err.Error(), "20000") || !strings.Contains(err.Error(), "--max-pixels") {
		t.Fatalf("cap message is not explanatory: %v", err)
	}
	if err := CheckPixelCap(info, 72, 0); !siphonerr.IsCode(err, siphonerr.Usage) {
		t.Fatalf("zero cap: got %v, want E_USAGE", err)
	}

	huge := &Info{MediaBox: Box{X1: 0, Y1: 0, X2: 1e300, Y2: 1e300}}
	if got := PixelCount(huge, math.MaxInt32); got != math.MaxInt64 {
		t.Fatalf("overflowing PixelCount = %d, want MaxInt64", got)
	}
}

func TestParseMetadataClasses(t *testing.T) {
	got := parseMetadataClasses(`Title: A
Metadata Stream: yes
Tagged: yes
JavaScript: yes
Form: AcroForm
`)
	want := []string{
		"outlines", "links", "annotations", "signatures",
		"PDF Info", "XMP", "tagged structure", "JavaScript", "forms",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("metadata classes = %v, want %v", got, want)
	}
	base := parseMetadataClasses("")
	if len(base) != 4 || base[0] != "outlines" || base[3] != "signatures" {
		t.Fatalf("baseline classes = %v", base)
	}
}
