package pdf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"
)

// EXIFInfo is the subset of EXIF that image → PDF translates, plus the fields
// that have no PDF equivalent and therefore need --drop-metadata.
type EXIFInfo struct {
	Orientation      int
	DateTimeOriginal string
	Make             string
	Model            string
	Untranslatable   []string
}

// ParseEXIF walks IFD0, the Exif sub-IFD, the GPS IFD, and the thumbnail IFD.
// It never panics on malformed input: an unreadable EXIF block is reported as
// an untranslatable field so the gate asks for --drop-metadata.
func ParseEXIF(exif []byte) EXIFInfo {
	info := EXIFInfo{Orientation: 1}
	if len(exif) == 0 {
		return info
	}
	if len(exif) >= 6 && bytes.Equal(exif[:4], []byte("Exif")) && exif[4] == 0 && exif[5] == 0 {
		exif = exif[6:]
	}
	order, ifd0, ok := tiffHeader(exif)
	if !ok {
		info.Untranslatable = appendUnique(info.Untranslatable, "EXIF (unparseable)")
		return info
	}

	seen := map[string]bool{}
	add := func(name string) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		info.Untranslatable = append(info.Untranslatable, name)
	}

	var exifIFD, gpsIFD uint32
	nextIFD := walkIFD(exif, order, ifd0, ifd0Visitor{
		onTag: func(tag uint16, typ uint16, count uint32, value []byte) {
			switch tag {
			case 0x0112:
				if o := tagUint16(order, typ, value); o >= 1 && o <= 8 {
					info.Orientation = int(o)
				}
			case 0x010F:
				info.Make = tagASCII(typ, count, value)
			case 0x0110:
				info.Model = tagASCII(typ, count, value)
			case 0x8769:
				exifIFD = tagUint32(order, typ, value)
			case 0x8825:
				gpsIFD = tagUint32(order, typ, value)
			case 0x0132:
				add("DateTime")
			default:
				if name, structural := ifd0TagName(tag); !structural {
					add(name)
				}
			}
		},
	})
	if nextIFD != 0 {
		add("thumbnail image")
	}

	if exifIFD != 0 {
		walkIFD(exif, order, int(exifIFD), ifd0Visitor{
			onTag: func(tag uint16, typ uint16, count uint32, value []byte) {
				switch tag {
				case 0x9003:
					info.DateTimeOriginal = tagASCII(typ, count, value)
				case 0xA005:
					// Interop IFD pointer: contents are not translatable.
					add("Interoperability")
				case 0x9004:
					add("DateTimeDigitized")
				default:
					add(exifTagName(tag))
				}
			},
		})
	}
	if gpsIFD != 0 {
		count, ok := ifdCount(exif, order, int(gpsIFD))
		if ok && count > 0 {
			add("GPS")
		}
	}
	return info
}

type ifd0Visitor struct {
	onTag func(tag uint16, typ uint16, count uint32, value []byte)
}

func tiffHeader(exif []byte) (binary.ByteOrder, int, bool) {
	if len(exif) < 8 {
		return nil, 0, false
	}
	var order binary.ByteOrder
	switch string(exif[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return nil, 0, false
	}
	if order.Uint16(exif[2:4]) != 42 {
		return nil, 0, false
	}
	ifd := int(order.Uint32(exif[4:8]))
	if ifd < 8 || ifd+2 > len(exif) {
		return nil, 0, false
	}
	return order, ifd, true
}

func walkIFD(exif []byte, order binary.ByteOrder, offset int, v ifd0Visitor) uint32 {
	count, ok := ifdCount(exif, order, offset)
	if !ok {
		return 0
	}
	pos := offset + 2
	for i := 0; i < count; i++ {
		if pos+12 > len(exif) {
			return 0
		}
		tag := order.Uint16(exif[pos : pos+2])
		typ := order.Uint16(exif[pos+2 : pos+4])
		n := order.Uint32(exif[pos+4 : pos+8])
		value, ok := tagValue(exif, order, typ, n, exif[pos+8:pos+12])
		if ok && v.onTag != nil {
			v.onTag(tag, typ, n, value)
		}
		pos += 12
	}
	if pos+4 <= len(exif) {
		return order.Uint32(exif[pos : pos+4])
	}
	return 0
}

func ifdCount(exif []byte, order binary.ByteOrder, offset int) (int, bool) {
	if offset < 0 || offset+2 > len(exif) {
		return 0, false
	}
	n := int(order.Uint16(exif[offset : offset+2]))
	if n < 0 || offset+2+12*n > len(exif) {
		return 0, false
	}
	return n, true
}

func tagValue(exif []byte, order binary.ByteOrder, typ uint16, count uint32, inline []byte) ([]byte, bool) {
	size := typeSize(typ)
	if size == 0 || count > 1<<20 {
		return nil, false
	}
	total := int(count) * size
	if total < 0 {
		return nil, false
	}
	if total <= 4 {
		return inline[:total], true
	}
	off := int(order.Uint32(inline))
	if off < 0 || off+total > len(exif) {
		return nil, false
	}
	return exif[off : off+total], true
}

func typeSize(typ uint16) int {
	switch typ {
	case 1, 2, 6, 7:
		return 1
	case 3, 8:
		return 2
	case 4, 9, 11:
		return 4
	case 5, 10, 12:
		return 8
	default:
		return 0
	}
}

func tagASCII(typ uint16, count uint32, value []byte) string {
	if typ != 2 {
		return ""
	}
	if count > 0 && int(count) <= len(value) {
		value = value[:count]
	}
	if i := bytes.IndexByte(value, 0); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(string(value))
}

func tagUint16(order binary.ByteOrder, typ uint16, value []byte) uint16 {
	if typ != 3 || len(value) < 2 {
		return 0
	}
	return order.Uint16(value)
}

func tagUint32(order binary.ByteOrder, typ uint16, value []byte) uint32 {
	if typ != 4 || len(value) < 4 {
		return 0
	}
	return order.Uint32(value)
}

func ifd0TagName(tag uint16) (string, bool) {
	switch tag {
	case 0x8769, 0x8825:
		return "", true // structural pointers
	case 0x0112, 0x010F, 0x0110:
		return "", true
	case 0x0132:
		return "DateTime", false
	case 0x010E:
		return "ImageDescription", false
	case 0x0131:
		return "Software", false
	case 0x013B:
		return "Artist", false
	case 0x8298:
		return "Copyright", false
	case 0x0100:
		return "ImageWidth", false
	case 0x0101:
		return "ImageLength", false
	case 0x011A:
		return "XResolution", false
	case 0x011B:
		return "YResolution", false
	case 0x0128:
		return "ResolutionUnit", false
	case 0x0213:
		return "YCbCrPositioning", false
	case 0x0212:
		return "YCbCrSubSampling", false
	case 0x011C:
		return "PlanarConfiguration", false
	}
	return fmt.Sprintf("EXIF tag 0x%04X", tag), false
}

func exifTagName(tag uint16) string {
	switch tag {
	case 0x829A:
		return "ExposureTime"
	case 0x829D:
		return "FNumber"
	case 0x8822:
		return "ExposureProgram"
	case 0x8827:
		return "ISOSpeedRatings"
	case 0x9003:
		return "DateTimeOriginal"
	case 0x9201:
		return "ShutterSpeedValue"
	case 0x9202:
		return "ApertureValue"
	case 0x9204:
		return "ExposureBiasValue"
	case 0x9207:
		return "MeteringMode"
	case 0x9209:
		return "Flash"
	case 0x920A:
		return "FocalLength"
	case 0x927C:
		return "MakerNote"
	case 0x9286:
		return "UserComment"
	case 0xA002:
		return "PixelXDimension"
	case 0xA003:
		return "PixelYDimension"
	case 0xA402:
		return "ExposureMode"
	case 0xA403:
		return "WhiteBalance"
	case 0xA405:
		return "FocalLengthIn35mmFilm"
	case 0xA406:
		return "SceneCaptureType"
	}
	return fmt.Sprintf("EXIF tag 0x%04X", tag)
}

func appendUnique(list []string, s string) []string {
	for _, existing := range list {
		if existing == s {
			return list
		}
	}
	return append(list, s)
}

// PDFDate converts an EXIF "YYYY:MM:DD HH:MM:SS" timestamp to a PDF date
// string. It returns "" when the timestamp is absent or invalid.
func (e EXIFInfo) PDFDate() string {
	v := strings.TrimSpace(e.DateTimeOriginal)
	if v == "" {
		return ""
	}
	for _, layout := range []string{"2006:01:02 15:04:05", "2006:01:02"} {
		if t, err := time.Parse(layout, v); err == nil {
			return "D:" + t.Format("20060102150405")
		}
	}
	return ""
}

// Creator returns the EXIF camera make/model when both are present.
func (e EXIFInfo) Creator() string {
	make := strings.TrimSpace(e.Make)
	model := strings.TrimSpace(e.Model)
	if make == "" || model == "" {
		return ""
	}
	if strings.Contains(strings.ToLower(model), strings.ToLower(make)) {
		return model
	}
	return make + " " + model
}
