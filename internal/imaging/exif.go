package imaging

import (
	"bytes"
	"encoding/binary"
)

// Orientation returns the EXIF orientation (1–8), or 1 when absent/invalid.
func Orientation(exif []byte) int {
	_, order, typ, pos, ok := orientationEntry(exif)
	if !ok {
		return 1
	}
	var value uint32
	switch typ {
	case 3:
		value = uint32(order.Uint16(exif[pos : pos+2]))
	case 4:
		value = order.Uint32(exif[pos : pos+4])
	default:
		return 1
	}
	if value < 1 || value > 8 {
		return 1
	}
	return int(value)
}

// SetOrientation returns a copy of exif with the orientation tag set. If the
// tag is absent, the bytes are returned unchanged (as a copy).
func SetOrientation(exif []byte, orientation uint16) []byte {
	out := append([]byte(nil), exif...)
	_, order, typ, pos, ok := orientationEntry(out)
	if !ok {
		return out
	}
	switch typ {
	case 3:
		order.PutUint16(out[pos:pos+2], orientation)
	case 4:
		order.PutUint32(out[pos:pos+4], uint32(orientation))
	}
	return out
}

// orientationEntry returns the location of tag 0x0112 in IFD0.
func orientationEntry(exif []byte) (start int, order binary.ByteOrder, typ uint16, valuePos int, ok bool) {
	if len(exif) >= 6 && bytes.Equal(exif[:4], []byte("Exif")) && exif[4] == 0 && exif[5] == 0 {
		start = 6
	}
	tiff := exif[start:]
	if len(tiff) < 8 {
		return 0, nil, 0, 0, false
	}
	switch string(tiff[0:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 0, nil, 0, 0, false
	}
	if order.Uint16(tiff[2:4]) != 42 {
		return 0, nil, 0, 0, false
	}
	ifd := int(order.Uint32(tiff[4:8]))
	if ifd < 8 || ifd+2 > len(tiff) {
		return 0, nil, 0, 0, false
	}
	count := int(order.Uint16(tiff[ifd : ifd+2]))
	if count <= 0 || count > 4096 {
		return 0, nil, 0, 0, false
	}
	entries := ifd + 2
	if entries+12*count > len(tiff) {
		return 0, nil, 0, 0, false
	}
	for i := 0; i < count; i++ {
		e := entries + 12*i
		if order.Uint16(tiff[e:e+2]) != 0x0112 {
			continue
		}
		typ = order.Uint16(tiff[e+2 : e+4])
		n := int(order.Uint32(tiff[e+4 : e+8]))
		if n < 1 {
			return 0, nil, 0, 0, false
		}
		var size int
		switch typ {
		case 1, 2, 6, 7:
			size = 1
		case 3, 8:
			size = 2
		case 4, 9:
			size = 4
		case 5, 10, 11, 12:
			size = 8
		default:
			return 0, nil, 0, 0, false
		}
		var off int
		if n*size <= 4 {
			off = e + 8
		} else {
			off = int(order.Uint32(tiff[e+8 : e+12]))
		}
		if off < 0 || off+size > len(tiff) {
			return 0, nil, 0, 0, false
		}
		return start, order, typ, start + off, true
	}
	return 0, nil, 0, 0, false
}
