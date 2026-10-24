package imaging

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"strconv"

	"siphon/internal/platform"
	"siphon/internal/siphonerr"
)

// webpInfo is what Siphon needs to know about a WebP container before it
// decides whether the file is convertible.
type webpInfo struct {
	Width    int
	Height   int
	HasAlpha bool
	Animated bool
	Metadata *Metadata
	Warnings []string
}

// readWebP parses the RIFF container, collecting dimensions, alpha presence,
// animation, and the metadata chunks. It never decodes pixel data.
func readWebP(path string) (*webpInfo, error) {
	f, err := platform.OpenReadOnly(path)
	if err != nil {
		return nil, siphonerr.Wrap(siphonerr.SourceCorrupt, err, "cannot open %s", path)
	}
	defer f.Close()

	var hdr [12]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return nil, corrupt(path, err, "cannot read WebP header")
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WEBP" {
		return nil, siphonerr.New(siphonerr.SourceCorrupt, "%s is not a WebP file", path)
	}

	info := &webpInfo{Metadata: &Metadata{}}
	for {
		var ch [8]byte
		if _, err := io.ReadFull(f, ch[:]); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return nil, corrupt(path, err, "truncated WebP chunk header")
		}
		fourcc := string(ch[0:4])
		size := int64(binary.LittleEndian.Uint32(ch[4:8]))
		if size < 0 || size > 1<<31 {
			return nil, siphonerr.New(siphonerr.SourceCorrupt,
				"%s has an implausible WebP chunk size %d", path, size)
		}

		switch fourcc {
		case "VP8X":
			buf, err := readWebPChunk(f, size, 10)
			if err != nil {
				return nil, corrupt(path, err, "cannot read WebP VP8X chunk")
			}
			if len(buf) >= 1 {
				if buf[0]&0x10 != 0 {
					info.HasAlpha = true
				}
				if buf[0]&0x02 != 0 {
					info.Animated = true
				}
			}
			if len(buf) >= 10 {
				info.Width = 1 + int(buf[4]) + int(buf[5])<<8 + int(buf[6])<<16
				info.Height = 1 + int(buf[7]) + int(buf[8])<<8 + int(buf[9])<<16
			}
		case "VP8 ":
			buf, err := readWebPChunk(f, size, 10)
			if err != nil {
				return nil, corrupt(path, err, "cannot read WebP VP8 chunk")
			}
			if len(buf) >= 10 && buf[3] == 0x9D && buf[4] == 0x01 && buf[5] == 0x2A {
				if info.Width == 0 {
					info.Width = int(binary.LittleEndian.Uint16(buf[6:8]) & 0x3FFF)
					info.Height = int(binary.LittleEndian.Uint16(buf[8:10]) & 0x3FFF)
				}
			}
		case "VP8L":
			buf, err := readWebPChunk(f, size, 5)
			if err != nil {
				return nil, corrupt(path, err, "cannot read WebP VP8L chunk")
			}
			if len(buf) >= 5 && buf[0] == 0x2F {
				bits := binary.LittleEndian.Uint32(buf[1:5])
				if info.Width == 0 {
					info.Width = int(bits&0x3FFF) + 1
					info.Height = int((bits>>14)&0x3FFF) + 1
				}
				if (bits>>28)&1 != 0 {
					info.HasAlpha = true
				}
			}
		case "ALPH":
			info.HasAlpha = true
			if _, err := readWebPChunk(f, size, 0); err != nil {
				return nil, corrupt(path, err, "cannot skip WebP ALPH chunk")
			}
		case "ANIM", "ANMF":
			info.Animated = true
			if _, err := readWebPChunk(f, size, 0); err != nil {
				return nil, corrupt(path, err, "cannot skip WebP animation chunk")
			}
		case "ICCP", "EXIF", "XMP ":
			if size > maxMetadataBytes {
				return nil, siphonerr.New(siphonerr.SourceCorrupt,
					"%s has an oversized WebP metadata chunk (%d bytes)", path, size)
			}
			data, err := readN(f, int(size))
			if err != nil {
				return nil, corruptf(path, err, "cannot read WebP %s chunk", fourcc)
			}
			switch fourcc {
			case "ICCP":
				info.Metadata.ICC = data
			case "EXIF":
				info.Metadata.EXIF = normalizeEXIF(data)
			case "XMP ":
				info.Metadata.XMP = data
			}
		default:
			if _, err := readWebPChunk(f, size, 0); err != nil {
				return nil, corruptf(path, err, "cannot skip WebP %s chunk", fourcc)
			}
		}

		if size%2 == 1 {
			var pad [1]byte
			if _, err := io.ReadFull(f, pad[:]); err != nil {
				return nil, corrupt(path, err, "truncated WebP padding")
			}
		}
	}

	if info.Width <= 0 || info.Height <= 0 {
		return nil, siphonerr.New(siphonerr.SourceCorrupt, "%s has no WebP image header", path)
	}
	info.Metadata.Animated = info.Animated
	return info, nil
}

// readWebPChunk reads at most want bytes and discards the rest of the chunk.
func readWebPChunk(r io.Reader, size, want int64) ([]byte, error) {
	if want > size {
		want = size
	}
	buf := make([]byte, want)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	if size > want {
		if _, err := io.CopyN(io.Discard, r, size-want); err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// decodeWebP writes a PNG intermediate through dwebp and returns its path.
func decodeWebP(ctx context.Context, src, scratchDir string) (string, error) {
	out, err := platform.CreateTemp(scratchDir, "webp-decode", ".png")
	if err != nil {
		return "", err
	}
	path := out.Name()
	out.Close()

	if err := runDWebP(ctx, src, path); err != nil {
		platform.RemoveIfExists(path)
		return "", err
	}
	return path, nil
}

func runDWebP(ctx context.Context, src, dst string) error {
	bin, err := exec.LookPath("dwebp")
	if err != nil {
		return siphonerr.New(siphonerr.Dependency, "dwebp is not installed or not in PATH")
	}
	cmd := exec.CommandContext(ctx, bin, "-quiet", src, "-o", dst)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		msg := bytes.TrimSpace(stderr.Bytes())
		if len(msg) > 0 {
			return siphonerr.Wrap(siphonerr.SourceCorrupt, err, "dwebp failed: %s", msg)
		}
		return siphonerr.Wrap(siphonerr.SourceCorrupt, err, "dwebp failed")
	}
	return nil
}

func runCWebP(ctx context.Context, src, dst string, lossless bool, quality int) error {
	bin, err := exec.LookPath("cwebp")
	if err != nil {
		return siphonerr.New(siphonerr.Dependency, "cwebp is not installed or not in PATH")
	}
	args := []string{"-quiet", "-metadata", "all"}
	if lossless {
		args = append(args, "-lossless", "-z", "6")
	} else {
		if quality < 1 || quality > 100 {
			quality = 90
		}
		args = append(args, "-q", strconv.Itoa(quality))
	}
	args = append(args, src, "-o", dst)

	cmd := exec.CommandContext(ctx, bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		msg := bytes.TrimSpace(stderr.Bytes())
		if errors.Is(err, exec.ErrNotFound) {
			return siphonerr.New(siphonerr.Dependency, "cwebp is not installed or not in PATH")
		}
		return siphonerr.Wrap(siphonerr.Internal, err, "cwebp failed: %s", msg)
	}
	return nil
}
