// Package imaging implements the still-image half of Siphon: decoding,
// orientation, metadata transplant, encoding through the external tools, and
// verification. It is deliberately independent of the CLI and the engine so
// pre-flight and the AtomicCopy engine share one implementation.
package imaging

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"

	"siphon/internal/formats"
	"siphon/internal/platform"
	"siphon/internal/siphonerr"
)

const (
	maxMetadataBytes = 64 << 20
	maxPNGChunkBytes = 256 << 20
)

// Metadata is the set of metadata classes Siphon carries across a re-encode.
// EXIF is stored as raw TIFF bytes (without the JPEG "Exif\0\0" prefix).
type Metadata struct {
	ICC      []byte
	EXIF     []byte
	XMP      []byte
	Animated bool
}

// Empty reports whether there is no metadata to carry.
func (m *Metadata) Empty() bool {
	return m == nil || (len(m.ICC) == 0 && len(m.EXIF) == 0 && len(m.XMP) == 0)
}

// Classes names the metadata classes present, for inspect output.
func (m *Metadata) Classes() []string {
	if m == nil {
		return nil
	}
	var out []string
	if len(m.ICC) > 0 {
		out = append(out, "ICC")
	}
	if len(m.EXIF) > 0 {
		out = append(out, "EXIF")
	}
	if len(m.XMP) > 0 {
		out = append(out, "XMP")
	}
	return out
}

// Extract reads the metadata classes Siphon understands from path. Structural
// corruption is an error; recoverable metadata problems are warnings.
func Extract(path string, format formats.Format) (*Metadata, []string, error) {
	switch format {
	case formats.JPG:
		return extractJPEG(path)
	case formats.PNG:
		return extractPNG(path)
	case formats.WEBP:
		info, err := readWebP(path)
		if err != nil {
			return nil, nil, err
		}
		return info.Metadata, info.Warnings, nil
	default:
		return nil, nil, siphonerr.New(siphonerr.FormatUnsupported,
			"no image metadata reader for %s", format)
	}
}

// AddMetadata injects metadata into an encoded JPEG or PNG. WebP metadata is
// carried by cwebp from an intermediate PNG instead.
func AddMetadata(data []byte, format formats.Format, m *Metadata) ([]byte, error) {
	if m == nil || m.Empty() {
		return data, nil
	}
	switch format {
	case formats.JPG:
		return addJPEGMetadata(data, m)
	case formats.PNG:
		return addPNGMetadata(data, m)
	default:
		return nil, siphonerr.New(siphonerr.Internal,
			"metadata injection into %s is not implemented", format)
	}
}

// --- JPEG -----------------------------------------------------------------

func extractJPEG(path string) (*Metadata, []string, error) {
	f, err := platform.OpenReadOnly(path)
	if err != nil {
		return nil, nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
	}
	defer f.Close()

	br := bufio.NewReaderSize(f, 64<<10)
	var soi [2]byte
	if _, err := io.ReadFull(br, soi[:]); err != nil {
		return nil, nil, corrupt(path, err, "cannot read JPEG header")
	}
	if soi[0] != 0xFF || soi[1] != 0xD8 {
		return nil, nil, siphonerr.New(siphonerr.SourceCorrupt, "%s is not a JPEG", path)
	}

	meta := &Metadata{}
	var warnings []string
	iccParts := make(map[int][]byte)
	iccCount := 0
	sawEXIF := false
	sawXMP := false

	for {
		marker, err := nextJPEGMarker(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, nil, corrupt(path, err, "malformed JPEG marker stream")
		}
		if marker == 0xD9 || marker == 0xDA { // EOI or start of scan
			break
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}

		size, err := readJPEGSegmentSize(br)
		if err != nil {
			return nil, nil, corrupt(path, err, "malformed JPEG segment length")
		}
		payloadLen := size - 2

		switch marker {
		case 0xE1: // APP1: EXIF or XMP
			payload, err := readN(br, payloadLen)
			if err != nil {
				return nil, nil, corrupt(path, err, "cannot read JPEG APP1")
			}
			switch {
			case bytes.HasPrefix(payload, []byte("Exif\x00\x00")):
				if !sawEXIF {
					meta.EXIF = normalizeEXIF(payload)
					sawEXIF = true
				}
			case bytes.HasPrefix(payload, xmpJPEGPrefix):
				if !sawXMP {
					meta.XMP = append([]byte(nil), payload[len(xmpJPEGPrefix):]...)
					sawXMP = true
				}
			case bytes.HasPrefix(payload, []byte("http://ns.adobe.com/xmp/extension/\x00")):
				warnings = append(warnings,
					"extended XMP is not carried; only the main XMP packet is preserved")
			}
		case 0xE2: // APP2: ICC profile (possibly split), or MPF
			payload, err := readN(br, payloadLen)
			if err != nil {
				return nil, nil, corrupt(path, err, "cannot read JPEG APP2")
			}
			if bytes.HasPrefix(payload, []byte("ICC_PROFILE\x00")) {
				if len(payload) < 14 {
					warnings = append(warnings, "ignoring a malformed ICC profile segment")
					continue
				}
				seq := int(payload[12])
				count := int(payload[13])
				if seq < 1 || count < 1 || seq > count {
					warnings = append(warnings, "ignoring an out-of-range ICC profile segment")
					continue
				}
				if iccCount != 0 && count != iccCount {
					warnings = append(warnings, "ignoring inconsistent ICC profile segments")
					continue
				}
				iccCount = count
				iccParts[seq] = append([]byte(nil), payload[14:]...)
			} else if bytes.HasPrefix(payload, []byte("MPF\x00")) {
				warnings = append(warnings,
					"multi-picture JPEG: only the primary image is converted")
			}
		case 0xED: // APP13: Photoshop/IPTC
			if _, err := readN(br, payloadLen); err != nil {
				return nil, nil, corrupt(path, err, "cannot read JPEG APP13")
			}
			warnings = append(warnings, "IPTC/Photoshop metadata is not carried")
		default:
			if _, err := io.CopyN(io.Discard, br, int64(payloadLen)); err != nil {
				return nil, nil, corrupt(path, err, "truncated JPEG segment")
			}
		}
	}

	if iccCount > 0 {
		var profile []byte
		for i := 1; i <= iccCount; i++ {
			part, ok := iccParts[i]
			if !ok {
				warnings = append(warnings, "ICC profile is incomplete; it will not be carried")
				profile = nil
				break
			}
			profile = append(profile, part...)
		}
		meta.ICC = profile
	}
	return meta, warnings, nil
}

var (
	xmpJPEGPrefix = []byte("http://ns.adobe.com/xap/1.0/\x00")
)

func nextJPEGMarker(r *bufio.Reader) (byte, error) {
	for {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if b != 0xFF {
			return 0, fmt.Errorf("expected marker byte, got 0x%02x", b)
		}
		m, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		switch {
		case m == 0xFF: // fill byte
			continue
		case m == 0x00:
			return 0, fmt.Errorf("unexpected stuffed byte")
		default:
			return m, nil
		}
	}
}

func readJPEGSegmentSize(r io.Reader) (int, error) {
	var b [2]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(b[:]))
	if n < 2 {
		return 0, fmt.Errorf("invalid segment length %d", n)
	}
	return n, nil
}

func readN(r io.Reader, n int) ([]byte, error) {
	if n < 0 {
		return nil, fmt.Errorf("negative read length")
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	return b, nil
}

func addJPEGMetadata(data []byte, m *Metadata) ([]byte, error) {
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, siphonerr.New(siphonerr.Internal, "encoder produced an invalid JPEG")
	}

	var segments [][]byte
	if len(m.EXIF) > 0 {
		payload := make([]byte, 0, 6+len(m.EXIF))
		payload = append(payload, []byte("Exif\x00\x00")...)
		payload = append(payload, m.EXIF...)
		seg, err := jpegSegment(0xE1, payload, "EXIF")
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	if len(m.XMP) > 0 {
		payload := make([]byte, 0, len(xmpJPEGPrefix)+len(m.XMP))
		payload = append(payload, xmpJPEGPrefix...)
		payload = append(payload, m.XMP...)
		seg, err := jpegSegment(0xE1, payload, "XMP")
		if err != nil {
			return nil, err
		}
		segments = append(segments, seg)
	}
	if len(m.ICC) > 0 {
		segs, err := jpegICCSegments(m.ICC)
		if err != nil {
			return nil, err
		}
		segments = append(segments, segs...)
	}

	pos := 2
	// Keep the encoder's APP0/JFIF segment first, which a few old readers like.
	if len(data) >= 6 && data[2] == 0xFF && data[3] == 0xE0 {
		n := int(binary.BigEndian.Uint16(data[4:6]))
		if n >= 2 && 4+n <= len(data) {
			pos = 4 + n
		}
	}

	total := len(data)
	for _, seg := range segments {
		total += len(seg)
	}
	out := make([]byte, 0, total)
	out = append(out, data[:pos]...)
	for _, seg := range segments {
		out = append(out, seg...)
	}
	out = append(out, data[pos:]...)
	return out, nil
}

func jpegICCSegments(profile []byte) ([][]byte, error) {
	const (
		iccHeader = 12 // "ICC_PROFILE\0"
		iccSeq    = 2  // sequence number + count
		maxData   = 0xFFFF - 2 - iccHeader - iccSeq
	)
	chunks := (len(profile) + maxData - 1) / maxData
	if chunks > 255 {
		return nil, siphonerr.New(siphonerr.MetadataDrop,
			"ICC profile is too large for JPEG (%d bytes)", len(profile))
	}
	segs := make([][]byte, 0, chunks)
	for i := 0; i < chunks; i++ {
		end := (i + 1) * maxData
		if end > len(profile) {
			end = len(profile)
		}
		payload := make([]byte, 0, iccHeader+iccSeq+end-i*maxData)
		payload = append(payload, []byte("ICC_PROFILE\x00")...)
		payload = append(payload, byte(i+1), byte(chunks))
		payload = append(payload, profile[i*maxData:end]...)
		seg, err := jpegSegment(0xE2, payload, "ICC profile")
		if err != nil {
			return nil, err
		}
		segs = append(segs, seg)
	}
	return segs, nil
}

func jpegSegment(marker byte, payload []byte, what string) ([]byte, error) {
	if len(payload)+2 > 0xFFFF {
		return nil, siphonerr.New(siphonerr.MetadataDrop,
			"%s is too large for a JPEG APP%d segment", what, marker&0x0F)
	}
	seg := make([]byte, 4+len(payload))
	seg[0] = 0xFF
	seg[1] = marker
	binary.BigEndian.PutUint16(seg[2:4], uint16(len(payload)+2))
	copy(seg[4:], payload)
	return seg, nil
}

// --- PNG ------------------------------------------------------------------

func extractPNG(path string) (*Metadata, []string, error) {
	f, err := platform.OpenReadOnly(path)
	if err != nil {
		return nil, nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
	}
	defer f.Close()

	var sig [8]byte
	if _, err := io.ReadFull(f, sig[:]); err != nil {
		return nil, nil, corrupt(path, err, "cannot read PNG signature")
	}
	if !bytes.Equal(sig[:], pngSignature) {
		return nil, nil, siphonerr.New(siphonerr.SourceCorrupt, "%s is not a PNG", path)
	}

	meta := &Metadata{}
	var warnings []string
	for {
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return nil, nil, corrupt(path, err, "truncated PNG chunk header")
		}
		length := int64(binary.BigEndian.Uint32(hdr[0:4]))
		if length > maxPNGChunkBytes {
			return nil, nil, siphonerr.New(siphonerr.SourceCorrupt,
				"%s has an implausible PNG chunk length %d", path, length)
		}
		typ := string(hdr[4:8])

		switch typ {
		case "iCCP", "eXIf", "iTXt", "acTL":
			data, err := readN(f, int(length))
			if err != nil {
				return nil, nil, corruptf(path, err, "cannot read PNG %s chunk", typ)
			}
			if _, err := io.CopyN(io.Discard, f, 4); err != nil {
				return nil, nil, corrupt(path, err, "cannot read PNG chunk CRC")
			}
			switch typ {
			case "iCCP":
				profile, err := decodeICCP(data)
				if err != nil {
					warnings = append(warnings, "ignoring an unreadable PNG ICC profile: "+err.Error())
				} else {
					meta.ICC = profile
				}
			case "eXIf":
				if len(meta.EXIF) == 0 {
					meta.EXIF = normalizeEXIF(data)
				}
			case "iTXt":
				keyword, text, err := decodeITXt(data)
				if err != nil {
					warnings = append(warnings, "ignoring an unreadable PNG iTXt chunk: "+err.Error())
					break
				}
				if keyword == "XML:com.adobe.xmp" && len(meta.XMP) == 0 {
					meta.XMP = text
				}
			case "acTL":
				meta.Animated = true
			}
		default:
			if _, err := io.CopyN(io.Discard, f, length+4); err != nil {
				return nil, nil, corrupt(path, err, "truncated PNG chunk")
			}
		}
		if typ == "IEND" {
			break
		}
	}
	return meta, warnings, nil
}

func decodeICCP(data []byte) ([]byte, error) {
	i := bytes.IndexByte(data, 0)
	if i < 0 || i+2 > len(data) {
		return nil, fmt.Errorf("missing profile name or compression method")
	}
	if data[i+1] != 0 {
		return nil, fmt.Errorf("unsupported compression method %d", data[i+1])
	}
	zr, err := zlib.NewReader(bytes.NewReader(data[i+2:]))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	profile, err := io.ReadAll(io.LimitReader(zr, maxMetadataBytes+1))
	if err != nil {
		return nil, err
	}
	if len(profile) > maxMetadataBytes {
		return nil, fmt.Errorf("ICC profile exceeds %d bytes", maxMetadataBytes)
	}
	return profile, nil
}

func decodeITXt(data []byte) (string, []byte, error) {
	i := bytes.IndexByte(data, 0)
	if i < 0 {
		return "", nil, fmt.Errorf("missing keyword terminator")
	}
	keyword := string(data[:i])
	rest := data[i+1:]
	if len(rest) < 2 {
		return "", nil, fmt.Errorf("missing compression fields")
	}
	compressed := rest[0] == 1
	method := rest[1]
	rest = rest[2:]

	for n := 0; n < 2; n++ { // language tag, translated keyword
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			return "", nil, fmt.Errorf("missing text field terminator")
		}
		rest = rest[j+1:]
	}
	if !compressed {
		return keyword, rest, nil
	}
	if method != 0 {
		return "", nil, fmt.Errorf("unsupported iTXt compression method %d", method)
	}
	zr, err := zlib.NewReader(bytes.NewReader(rest))
	if err != nil {
		return "", nil, err
	}
	defer zr.Close()
	text, err := io.ReadAll(io.LimitReader(zr, maxMetadataBytes+1))
	if err != nil {
		return "", nil, err
	}
	if len(text) > maxMetadataBytes {
		return "", nil, fmt.Errorf("iTXt payload exceeds %d bytes", maxMetadataBytes)
	}
	return keyword, text, nil
}

func addPNGMetadata(data []byte, m *Metadata) ([]byte, error) {
	if !bytes.HasPrefix(data, pngSignature) || len(data) < 33 {
		return nil, siphonerr.New(siphonerr.Internal, "encoder produced an invalid PNG")
	}
	ihdrLen := int(binary.BigEndian.Uint32(data[8:12]))
	insert := 8 + 12 + ihdrLen
	if ihdrLen < 0 || insert > len(data) || string(data[12:16]) != "IHDR" {
		return nil, siphonerr.New(siphonerr.Internal, "encoder produced an invalid PNG header")
	}

	var chunks [][]byte
	if len(m.ICC) > 0 {
		var z bytes.Buffer
		zw := zlib.NewWriter(&z)
		if _, err := zw.Write(m.ICC); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		payload := make([]byte, 0, 12+len(z.Bytes()))
		payload = append(payload, []byte("ICC profile\x00\x00")...)
		payload = append(payload, z.Bytes()...)
		chunks = append(chunks, pngChunk("iCCP", payload))
	}
	if len(m.EXIF) > 0 {
		chunks = append(chunks, pngChunk("eXIf", m.EXIF))
	}
	if len(m.XMP) > 0 {
		payload := make([]byte, 0, 32+len(m.XMP))
		payload = append(payload, []byte("XML:com.adobe.xmp")...)
		payload = append(payload, 0, 0, 0, 0, 0)
		payload = append(payload, m.XMP...)
		chunks = append(chunks, pngChunk("iTXt", payload))
	}

	total := len(data)
	for _, c := range chunks {
		total += len(c)
	}
	out := make([]byte, 0, total)
	out = append(out, data[:insert]...)
	for _, c := range chunks {
		out = append(out, c...)
	}
	out = append(out, data[insert:]...)
	return out, nil
}

func pngChunk(typ string, payload []byte) []byte {
	c := make([]byte, 12+len(payload))
	binary.BigEndian.PutUint32(c[0:4], uint32(len(payload)))
	copy(c[4:8], typ)
	copy(c[8:], payload)
	crc := crc32Checksum(c[4 : 8+len(payload)])
	binary.BigEndian.PutUint32(c[8+len(payload):], crc)
	return c
}

func corrupt(path string, err error, msg string) error {
	return siphonerr.Wrap(siphonerr.SourceCorrupt, err, "%s: %s", path, msg)
}

func corruptf(path string, err error, format string, args ...any) error {
	return corrupt(path, err, fmt.Sprintf(format, args...))
}

// normalizeEXIF removes the JPEG-style "Exif\0\0" prefix, leaving raw TIFF.
func normalizeEXIF(b []byte) []byte {
	if len(b) >= 6 && bytes.Equal(b[:4], []byte("Exif")) && b[4] == 0 && b[5] == 0 {
		b = b[6:]
	}
	return append([]byte(nil), b...)
}
