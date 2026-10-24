package platform

import (
	"bytes"
	"fmt"
	"io"

	"siphon/internal/formats"
	"siphon/internal/siphonerr"
)

var (
	ebmlMagic = []byte{0x1a, 0x45, 0xdf, 0xa3}
	pngMagic  = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	jpegMagic = []byte{0xff, 0xd8, 0xff}
)

// ValidateMagic checks that the first bytes of path match the claimed format.
// The engine is chosen from the extension, so a renamed file must be refused
// before it is handed to the wrong encoder (spec §6.7).
func ValidateMagic(path string, f formats.Format) error {
	fh, err := OpenReadOnly(path)
	if err != nil {
		return siphonerr.Wrap(siphonerr.SourceInvalid, err, "cannot open %s", path)
	}
	defer fh.Close()

	buf := make([]byte, 16)
	n, err := io.ReadFull(fh, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot read header of %s", path)
	}
	buf = buf[:n]

	ok := false
	switch f {
	case formats.WAV:
		ok = n >= 12 && bytes.Equal(buf[0:4], []byte("RIFF")) && bytes.Equal(buf[8:12], []byte("WAVE"))
	case formats.MKV:
		ok = n >= 4 && bytes.Equal(buf[0:4], ebmlMagic)
	case formats.PNG:
		ok = n >= 8 && bytes.Equal(buf[0:8], pngMagic)
	case formats.JPG:
		ok = n >= 3 && bytes.Equal(buf[0:3], jpegMagic)
	case formats.WEBP:
		ok = n >= 12 && bytes.Equal(buf[0:4], []byte("RIFF")) && bytes.Equal(buf[8:12], []byte("WEBP"))
	default:
		return siphonerr.New(siphonerr.SourceCorrupt, "no magic-byte rule for %s", f)
	}
	if !ok {
		return siphonerr.New(siphonerr.SourceCorrupt,
			"%s does not look like %s (header %s)", path, f, hexPreview(buf))
	}
	return nil
}

func hexPreview(b []byte) string {
	if len(b) > 8 {
		b = b[:8]
	}
	return fmt.Sprintf("% x", b)
}
