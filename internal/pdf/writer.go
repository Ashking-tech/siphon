package pdf

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"siphon/internal/formats"
	"siphon/internal/siphonerr"
	"siphon/internal/version"
)

// ImageInput is the pixel/bitstream payload of a one-page PDF.
type ImageInput struct {
	Width       int
	Height      int
	Gray        bool
	ICC         []byte
	JPEG        []byte
	RGB         []byte // Width × Height × 3
	GrayPixels  []byte // Width × Height
	Alpha       []byte // Width × Height; nil means no SMask
	Orientation int
	PageWidth   float64 // points; zero means derive from 96 DPI
	PageHeight  float64
}

// DocInfo is the PDF document information dictionary.
type DocInfo struct {
	Producer     string
	Creator      string
	CreationDate string
	ModDate      string
}

// PageSize returns the page dimensions in points for an image at its source
// DPI, applying the EXIF orientation swap for 5–8 (spec §8.1).
func PageSize(info *ImageInfo) (float64, float64) {
	dpi := info.DPI
	if dpi < 36 || dpi > 1200 {
		dpi = 96
	}
	w := float64(info.Width) / dpi * 72
	h := float64(info.Height) / dpi * 72
	switch info.Orientation {
	case 5, 6, 7, 8:
		return h, w
	default:
		return w, h
	}
}

// BuildInput converts a probed image into the writer's payload. JPEGs keep
// their original bitstream; PNGs are expanded to 8-bit samples and, when
// needed, an 8-bit alpha plane.
func BuildInput(info *ImageInfo) (*ImageInput, error) {
	pageW, pageH := PageSize(info)
	in := &ImageInput{
		Width:       info.Width,
		Height:      info.Height,
		Gray:        info.Gray,
		ICC:         append([]byte(nil), info.ICC...),
		Orientation: info.Orientation,
		PageWidth:   pageW,
		PageHeight:  pageH,
	}
	if info.JPEG != nil {
		in.JPEG = append([]byte(nil), info.JPEG...)
		return in, nil
	}
	if info.Pixels == nil {
		return nil, siphonerr.New(siphonerr.Internal, "no decoded pixels for %s", info.Format)
	}
	if in.Width <= 0 || in.Height <= 0 || int64(in.Width)*int64(in.Height) > MaxImagePixels {
		return nil, siphonerr.New(siphonerr.SourceInvalid,
			"image dimensions %dx%d are outside the supported range", in.Width, in.Height)
	}
	rgb, gray, alpha, err := imagePlanes(info.Pixels, in.Width, in.Height, in.Gray, info.HasAlpha)
	if err != nil {
		return nil, err
	}
	in.RGB = rgb
	in.GrayPixels = gray
	in.Alpha = alpha
	return in, nil
}

func imagePlanes(img image.Image, w, h int, gray, wantAlpha bool) (rgb, grayPix, alpha []byte, err error) {
	b := img.Bounds()
	if b.Dx() != w || b.Dy() != h {
		return nil, nil, nil, siphonerr.New(siphonerr.Internal,
			"decoded image is %dx%d, expected %dx%d", b.Dx(), b.Dy(), w, h)
	}
	n := w * h
	if gray {
		grayPix = make([]byte, n)
	} else {
		rgb = make([]byte, n*3)
	}
	if wantAlpha {
		alpha = make([]byte, n)
	}

	switch src := img.(type) {
	case *image.Gray:
		for y := 0; y < h; y++ {
			row := src.Pix[y*src.Stride : y*src.Stride+w]
			if gray {
				copy(grayPix[y*w:(y+1)*w], row)
				continue
			}
			for x := 0; x < w; x++ {
				i := y*w + x
				v := row[x]
				rgb[i*3], rgb[i*3+1], rgb[i*3+2] = v, v, v
			}
		}
		return rgb, grayPix, alpha, nil
	case *image.NRGBA:
		for y := 0; y < h; y++ {
			row := src.Pix[y*src.Stride : y*src.Stride+4*w]
			for x := 0; x < w; x++ {
				r, g, b2, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
				i := y*w + x
				if gray {
					grayPix[i] = r
				} else {
					rgb[i*3], rgb[i*3+1], rgb[i*3+2] = r, g, b2
				}
				if alpha != nil {
					alpha[i] = a
				}
			}
		}
		return rgb, grayPix, alpha, nil
	case *image.RGBA:
		for y := 0; y < h; y++ {
			row := src.Pix[y*src.Stride : y*src.Stride+4*w]
			for x := 0; x < w; x++ {
				r, g, b2, a := row[x*4], row[x*4+1], row[x*4+2], row[x*4+3]
				if a != 255 && a != 0 {
					r = uint8(uint32(r) * 255 / uint32(a))
					g = uint8(uint32(g) * 255 / uint32(a))
					b2 = uint8(uint32(b2) * 255 / uint32(a))
				}
				i := y*w + x
				if gray {
					grayPix[i] = r
				} else {
					rgb[i*3], rgb[i*3+1], rgb[i*3+2] = r, g, b2
				}
				if alpha != nil {
					alpha[i] = a
				}
			}
		}
		return rgb, grayPix, alpha, nil
	case *image.Paletted:
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				index := int(src.Pix[y*src.Stride+x])
				if index < 0 || index >= len(src.Palette) {
					return nil, nil, nil, siphonerr.New(siphonerr.SourceCorrupt,
						"palette index %d is outside the %d-entry palette", index, len(src.Palette))
				}
				c := src.Palette[index]
				nr, ng, nb, na := c.RGBA()
				i := y*w + x
				if gray {
					grayPix[i] = uint8(nr >> 8)
				} else {
					rgb[i*3], rgb[i*3+1], rgb[i*3+2] = uint8(nr>>8), uint8(ng>>8), uint8(nb>>8)
				}
				if alpha != nil {
					alpha[i] = uint8(na >> 8)
				}
			}
		}
		return rgb, grayPix, alpha, nil
	case *image.Gray16, *image.NRGBA64, *image.RGBA64:
		return nil, nil, nil, siphonerr.New(siphonerr.FormatUnsupported,
			"16-bit images are not supported by image → PDF")
	}

	// Fallback for uncommon image.Image implementations.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := y*w + x
			if gray {
				grayPix[i] = c.R
			} else {
				rgb[i*3], rgb[i*3+1], rgb[i*3+2] = c.R, c.G, c.B
			}
			if alpha != nil {
				alpha[i] = c.A
			}
		}
	}
	return rgb, grayPix, alpha, nil
}

// WriteImage writes a valid PDF 1.7 document with exactly one page. The image
// is the entire page; there are no margins, annotations, or attachments.
func WriteImage(w io.Writer, in *ImageInput, info DocInfo) error {
	if in == nil || in.Width <= 0 || in.Height <= 0 {
		return siphonerr.New(siphonerr.Internal, "invalid image dimensions")
	}
	if in.Gray {
		if in.JPEG == nil && len(in.GrayPixels) != in.Width*in.Height {
			return siphonerr.New(siphonerr.Internal, "grayscale plane has the wrong size")
		}
	} else if in.JPEG == nil && len(in.RGB) != in.Width*in.Height*3 {
		return siphonerr.New(siphonerr.Internal, "RGB plane has the wrong size")
	}
	if in.Alpha != nil && len(in.Alpha) != in.Width*in.Height {
		return siphonerr.New(siphonerr.Internal, "alpha plane has the wrong size")
	}

	pageW, pageH := pdfPageSize(in)
	now := time.Now().UTC()
	if info.Producer == "" {
		info.Producer = "Siphon " + version.String()
	}
	if info.Creator == "" {
		info.Creator = "Siphon " + version.String()
	}
	if info.CreationDate == "" {
		info.CreationDate = "D:" + now.Format("20060102150405") + "Z"
	}
	if info.ModDate == "" {
		info.ModDate = info.CreationDate
	}

	imageData, imageFilter, err := imageStream(in)
	if err != nil {
		return err
	}
	var alphaData []byte
	if in.Alpha != nil {
		alphaData, err = deflate(in.Alpha)
		if err != nil {
			return err
		}
	}
	var iccData []byte
	if len(in.ICC) > 0 {
		iccData, err = deflate(in.ICC)
		if err != nil {
			return err
		}
	}

	const (
		catalogObj = 1
		pagesObj   = 2
		pageObj    = 3
		imageObj   = 4
		contentObj = 5
		infoObj    = 6
	)
	next := 7
	iccObj := 0
	if len(in.ICC) > 0 {
		iccObj = next
		next++
	}
	alphaObj := 0
	if in.Alpha != nil {
		alphaObj = next
		next++
	}

	colorSpace := "/DeviceGray"
	if !in.Gray {
		colorSpace = "/DeviceRGB"
	}
	if iccObj != 0 {
		colorSpace = fmt.Sprintf("[/ICCBased %d 0 R]", iccObj)
	}

	var buf bytes.Buffer
	buf.WriteString("%PDF-1.7\n")
	buf.WriteString("%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, next)

	writeObj := func(num int, body string) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", num, body)
	}

	writeObj(catalogObj, fmt.Sprintf("<< /Type /Catalog /Pages %d 0 R >>", pagesObj))
	writeObj(pagesObj, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /Count 1 >>", pageObj))
	writeObj(pageObj, fmt.Sprintf(
		"<< /Type /Page /Parent %d 0 R /MediaBox [0 0 %s %s] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>",
		pagesObj, num(pageW), num(pageH), imageObj, contentObj))

	imageDict := fmt.Sprintf(
		"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace %s /BitsPerComponent 8 /Filter %s /Length %d",
		in.Width, in.Height, colorSpace, imageFilter, len(imageData))
	if alphaObj != 0 {
		imageDict += fmt.Sprintf(" /SMask %d 0 R", alphaObj)
	}
	imageDict += " >>"
	writeStreamObj := func(num int, dict string, data []byte) {
		offsets[num] = buf.Len()
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nstream\n", num, dict)
		buf.Write(data)
		buf.WriteString("\nendstream\nendobj\n")
	}
	writeStreamObj(imageObj, imageDict, imageData)

	content := fmt.Sprintf("q\n%s\n/Im0 Do\nQ\n", contentMatrix(pageW, pageH, in.Orientation))
	writeStreamObj(contentObj, fmt.Sprintf("<< /Length %d >>", len(content)), []byte(content))

	writeObj(infoObj, fmt.Sprintf(
		"<< /Producer %s /Creator %s /CreationDate %s /ModDate %s >>",
		pdfString(info.Producer), pdfString(info.Creator),
		pdfString(info.CreationDate), pdfString(info.ModDate)))

	if iccObj != 0 {
		n := 3
		alt := "/DeviceRGB"
		if in.Gray {
			n = 1
			alt = "/DeviceGray"
		}
		writeStreamObj(iccObj, fmt.Sprintf(
			"<< /N %d /Alternate %s /Filter /FlateDecode /Length %d >>", n, alt, len(iccData)), iccData)
	}
	if alphaObj != 0 {
		writeStreamObj(alphaObj, fmt.Sprintf(
			"<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceGray /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>",
			in.Width, in.Height, len(alphaData)), alphaData)
	}

	xref := buf.Len()
	fmt.Fprintf(&buf, "xref\n0 %d\n", next)
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i < next; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size %d /Root %d 0 R /Info %d 0 R >>\nstartxref\n%d\n%%%%EOF\n",
		next, catalogObj, infoObj, xref)
	if _, err := io.Copy(w, &buf); err != nil {
		return siphonerr.Wrap(siphonerr.Internal, err, "cannot write PDF")
	}
	return nil
}

func pdfPageSize(in *ImageInput) (float64, float64) {
	if in.PageWidth > 0 && in.PageHeight > 0 {
		return in.PageWidth, in.PageHeight
	}
	dpi := 96.0
	w := float64(in.Width) / dpi * 72
	h := float64(in.Height) / dpi * 72
	switch in.Orientation {
	case 5, 6, 7, 8:
		return h, w
	default:
		return w, h
	}
}

func contentMatrix(w, h float64, orientation int) string {
	W, H := num(w), num(h)
	var m [6]string
	switch orientation {
	case 2:
		m = [6]string{"-" + W, "0", "0", H, W, "0"}
	case 3:
		m = [6]string{"-" + W, "0", "0", "-" + H, W, H}
	case 4:
		m = [6]string{W, "0", "0", "-" + H, "0", H}
	case 5:
		m = [6]string{"0", "-" + H, "-" + W, "0", W, H}
	case 6:
		m = [6]string{"0", "-" + H, W, "0", "0", H}
	case 7:
		m = [6]string{"0", H, W, "0", "0", "0"}
	case 8:
		m = [6]string{"0", H, "-" + W, "0", W, "0"}
	default:
		m = [6]string{W, "0", "0", H, "0", "0"}
	}
	return fmt.Sprintf("%s %s %s %s %s %s cm", m[0], m[1], m[2], m[3], m[4], m[5])
}

func imageStream(in *ImageInput) ([]byte, string, error) {
	if in.JPEG != nil {
		return in.JPEG, "/DCTDecode", nil
	}
	var raw []byte
	if in.Gray {
		raw = in.GrayPixels
	} else {
		raw = in.RGB
	}
	data, err := deflate(raw)
	if err != nil {
		return nil, "", err
	}
	return data, "/FlateDecode", nil
}

func deflate(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err, "cannot compress PDF stream")
	}
	if err := zw.Close(); err != nil {
		return nil, siphonerr.Wrap(siphonerr.Internal, err, "cannot compress PDF stream")
	}
	return buf.Bytes(), nil
}

func num(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func pdfString(s string) string {
	ascii := true
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			ascii = false
			break
		}
	}
	if ascii {
		var b strings.Builder
		b.WriteByte('(')
		for _, r := range s {
			switch r {
			case '\\', '(', ')':
				b.WriteByte('\\')
			}
			b.WriteRune(r)
		}
		b.WriteByte(')')
		return b.String()
	}
	units := utf16.Encode([]rune(s))
	var b strings.Builder
	b.WriteString("<FEFF")
	b.WriteString(strings.ToUpper(hex.EncodeToString(utf16Bytes(units))))
	b.WriteByte('>')
	return b.String()
}

func utf16Bytes(units []uint16) []byte {
	out := make([]byte, len(units)*2)
	for i, u := range units {
		out[i*2] = byte(u >> 8)
		out[i*2+1] = byte(u)
	}
	return out
}

// FormatForTarget is a small guard used by the engine: image → PDF supports
// exactly PNG and JPEG sources.
func FormatForTarget(f formats.Format) bool {
	return f == formats.PNG || f == formats.JPG
}
