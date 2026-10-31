# Siphon — v0.2 Media Format Addendum

**Document version:** 0.2-draft  
**Status:** Draft for implementation  
**Parent contract:** `spec.md` v1.7 — frozen and unchanged  
**Target OS:** Linux (KDE Plasma primary)  
**Scope:** PDF/image, video container, and video/audio conversions  
**Adds:** `.pdf ↔ .png/.jpg`, `.mp4/.mov → .mkv`, `.mkv → .mov`, `.webm ↔ .mp4`, video → `.mp3`, `.gif ↔ .mp4`  
**Hard constraints:** PDF → image requires exactly one page; all new pairs are `safe` only.

---

## 1. Purpose

This addendum extends Siphon's format matrix without changing the v1.7 engine contract. It covers two groups:

**Document and image pairs**

- one-page PDF → PNG/JPG
- one image → one-page PDF

**Video and audio pairs**

- MP4 → MKV
- MOV → MKV
- MKV → MOV
- WebM → MP4
- MP4 → WebM
- video → MP3
- GIF → MP4
- MP4 → GIF

MKV → MP4 already exists in v1.7 and is unchanged. The new pairs complete the obvious reverse directions:

| Requested direction | Forward | Reverse |
|---|---|---|
| MP4/MOV ↔ MKV | MP4 → MKV, MOV → MKV | MKV → MP4 (v1.7), MKV → MOV |
| WebM ↔ MP4 | WebM → MP4 | MP4 → WebM |
| GIF ↔ MP4 | GIF → MP4 | MP4 → GIF |
| Video → MP3 | MP4/MKV/MOV/WebM → MP3 | MP3 → video is not supported; it is not a conversion inverse |

The single-page PDF restriction is deliberate: it keeps PDF → image a **one source file in, one target file out** conversion. PDF → image is rasterization, not a lossless inverse of image → PDF.

All new pairs are one-file-in/one-file-out and **`safe` only**. They do not use `relocate`, `reclaim`, the reverse cursor, or the segment journal.

---

## 2. Relationship to v1.7

`spec.md` v1.7 remains the frozen contract for all existing pairs and behavior. This document is a format addendum:

- Existing pairs, modes, cursor invariants, journal rules, and error semantics are unchanged.
- MKV → MP4 retains its v1.7 behavior: `relocate` by default, `safe` for classic MP4.
- All new pairs are **`safe` only**.
- New pairs reuse v1.7's same-directory temp rule, atomic finalisation, source locking, metadata transfer, signal handling, `--json`, `inspect`, and KDE wrapper conventions.
- If this addendum and v1.7 conflict, v1.7 wins for existing behavior; this addendum wins only for the new pairs.
- New error codes are additive and stable. They do not change existing codes.

No new cursor invariant is introduced. There is no 1× claim for any new pair: peak space is `source + output`, and the pre-flight gate refuses when that cannot fit.

---

## 3. Format Matrix

### 3.1 Document and image pairs

| Source | Target | Default mode | Other modes | Peak space | Output kind |
|---|---|---|---|---|---|
| `.pdf` | `.png` | `safe` | — | source + output | 8-bit sRGB PNG |
| `.pdf` | `.jpg` | `safe` | — | source + output | 8-bit sRGB JPEG |
| `.png` | `.pdf` | `safe` | — | source + output | PDF 1.7, one page |
| `.jpg` | `.pdf` | `safe` | — | source + output | PDF 1.7, one page |

### 3.2 Video container pairs

| Source | Target | Default mode | Other modes | Peak space | Output kind |
|---|---|---|---|---|---|
| `.mkv` | `.mp4` | `relocate` | `safe` | v1.7 rules | fragmented or classic MP4 |
| `.mp4` | `.mkv` | `safe` | — | source + output | classic MKV |
| `.mov` | `.mkv` | `safe` | — | source + output | classic MKV |
| `.mkv` | `.mov` | `safe` | — | source + output | classic MOV |
| `.webm` | `.mp4` | `safe` | — | source + output | classic MP4 |
| `.mp4` | `.webm` | `safe` | — | source + output | WebM |
| `.gif` | `.mp4` | `safe` | — | source + output | classic MP4, no audio |
| `.mp4` | `.gif` | `safe` | — | source + output | animated GIF, no audio |

### 3.3 Audio extraction

| Source | Target | Default mode | Other modes | Peak space | Output kind |
|---|---|---|---|---|---|
| `.mp4` | `.mp3` | `safe` | — | source + output | MP3 |
| `.mkv` | `.mp3` | `safe` | — | source + output | MP3 |
| `.mov` | `.mp3` | `safe` | — | source + output | MP3 |
| `.webm` | `.mp3` | `safe` | — | source + output | MP3 |

**MP3 → video is not supported.** Adding a still image or visualizer to an MP3 is an authoring operation, not the reverse of audio extraction. It is a v0.3+ feature, not part of this addendum.

### 3.4 Pair rules

- The target is the source's sibling with the same stem: `film.mp4 → film.mkv`, `clip.webm → clip.mp4`, `movie.mp4 → movie.mp3`.
- The source is consumed only after the target passes verification and is atomically installed.
- `.jpeg` is accepted as an input alias for `.jpg`; `.jpg` remains the canonical output extension.
- PDF → image output has **no alpha**; the page is composited on white.
- Image → PDF preserves alpha with a PDF soft mask (`SMask`).
- Image → PDF does not resample, crop, or re-encode JPEG pixels. PNG pixels are decoded and stored losslessly with Flate compression.
- Container remuxes copy compatible streams; they do not transcode by default. Unsupported tracks are refused unless `--drop-incompatible` is given.
- WebM ↔ MP4 requires an explicit `--transcode` when the codecs are incompatible. Siphon never silently re-encodes.
- Video → MP3 is a transcode. The selected audio track is encoded to MP3; video is dropped.
- GIF → MP4 and MP4 → GIF are always transcodes; choosing the target is the explicit lossy decision. GIF transparency is flattened onto `--bg`; MP4 → GIF drops audio.
- All new pairs use classic containers, not fragmented output. There is no relocate path for them.

### 3.5 PDF → image: single-page gate

A PDF is eligible only if **all** of the following are true:

1. `pdfinfo` reports `Pages: 1`.
2. The PDF is not encrypted.
3. A usable page box exists (`CropBox` when present, otherwise `MediaBox`).
4. Page width and height are positive.
5. Page rotation is one of `0`, `90`, `180`, `270`.
6. The rendered pixel count does not exceed `--max-pixels`.
7. `pdfinfo` and `pdftoppm` are available and meet the minimum version.

If `Pages` is greater than one, the refusal is `E_PDF_MULTIPAGE`. Siphon **never** silently converts the first page, and there is no `--pages` or `--first-page` escape hatch in v0.2.

### 3.6 Container remux policy

The container remux pairs are:

- MP4 → MKV
- MOV → MKV
- MKV → MOV

They use `ffmpeg -c copy` and a declarative stream map. The target must be a classic container; no fragmented output and no relocate.

- Compatible video, audio, and subtitle streams, plus chapters/attachments the target can represent, are copied.
- Incompatible streams are refused with `E_CODEC_INCOMPATIBLE` unless `--drop-incompatible` is given.
- `--allow-nonstandard` permits codecs that are muxable but poorly supported by the target container in the MP4/WebM remux paths. It does not permit transcoding, and it does not make VP9/Opus/FLAC standard in MOV.
- The source and target durations must match within 1% or two frames.
- Per-stream packet counts must match the source for every copied stream.
- No stream is silently transcoded.

### 3.7 Transcode policy

The transcode pairs are:

- WebM → MP4 (when remux is not possible or not requested)
- MP4 → WebM (when remux is not possible or not requested)
- Video → MP3
- GIF → MP4
- MP4 → GIF

Transcoding is lossy. It is **never** implicit:

- For WebM ↔ MP4, if the pair can remux, Siphon remuxes by default.
- For WebM ↔ MP4, if the pair requires re-encoding, `inspect`/`convert` refuses with `E_TRANSCODE_REQUIRED` unless `--transcode` is given.
- `--transcode` is explicit and is reflected in `inspect`, JSON, and the KDE warning.
- GIF ↔ MP4 and video → MP3 are inherently transcodes; they do not take `--transcode` because the target choice is the explicit lossy decision.
- The source remains untouched until the transcoded output passes verification.

Default transcode settings:

| Target | Video codec | Audio codec | Default quality |
|---|---|---|---|
| MP4 | H.264 (`libx264`), yuv420p | AAC, 128 kbps | CRF 23 |
| WebM | VP9 (`libvpx-vp9`) | Opus, 96 kbps | CRF 32 |
| MP3 | — | MP3 (`libmp3lame`), VBR V2 | V2; CBR via `--audio-bitrate` |
| GIF | GIF palette | — | 480 px wide, 12 fps, loop forever |
| MP4 from GIF | H.264 (`libx264`), yuv420p | — | CRF 23, 30 fps |

The exact stream selection and verification rules are in §9 and §10.

---

## 4. Dependencies and Size Budget

### 4.1 External tools

| Binary | Used for | Minimum | Required when |
|---|---|---|---|
| `ffmpeg` | demux, remux, transcode, GIF palette, MP3 encode | ≥ 5.0 | all video/audio pairs |
| `ffprobe` | track/codec/duration/packet inspection | ≥ 5.0 | all video/audio pairs |
| `lame` | WAV PCM → MP3 (v1.7 path) | ≥ 3.100 | `.wav` source |
| `pdfinfo` | page count, encryption state, page box, rotation | poppler-utils ≥ 22.01 | `.pdf` source |
| `pdftoppm` | rendering the single PDF page | poppler-utils ≥ 22.01 | `.pdf` source |
| `cwebp` | WebP encode (v1.7 path) | ≥ 1.0 | `.webp` target |
| `dwebp` | WebP decode (v1.7 path) | ≥ 1.0 | `.webp` source |

- Missing mandatory binary → `E_DEPENDENCY`; present but older than minimum → `E_DEPENDENCY_OUTDATED`.
- Image → PDF does **not** require poppler. It uses only the Go standard library.
- Container remuxes do not require any new external tool beyond `ffmpeg`/`ffprobe`.
- `siphon doctor` reports every tool, its version, and the package hint when missing.

### 4.2 ffmpeg build features

The distro `ffmpeg` must include the encoders and filters used by the selected pair. These are checked at pre-flight with `ffmpeg -hide_banner -encoders` and `-filters`:

| Feature | Used by |
|---|---|
| `libx264` | WebM → MP4 transcode, GIF → MP4 |
| `aac` | WebM → MP4 transcode |
| `libvpx-vp9` | MP4 → WebM transcode |
| `libopus` | MP4 → WebM transcode |
| `libmp3lame` | video → MP3 |
| `gif` encoder, `palettegen`, `paletteuse` | MP4 → GIF |

A missing encoder/filter is `E_DEPENDENCY` with a build-specific hint. Fedora users may need the full `ffmpeg` package rather than `ffmpeg-free`; Debian/Ubuntu users may need `ffmpeg` from the standard repository. The doctor command names the missing feature.

Package hints in user-facing messages:

```text
Fedora:        sudo dnf install poppler-utils ffmpeg
Debian/Ubuntu: sudo apt install poppler-utils ffmpeg
Arch:          sudo pacman -S poppler ffmpeg
```

`lame`, `libwebp-tools`, and poppler remain optional per pair, exactly as in v1.7.

### 4.3 Go dependencies and binary budget

- No cgo. `CGO_ENABLED=0` remains mandatory.
- No embedded PDF renderer, no embedded poppler, and no embedded ffmpeg.
- No new external Go module is required. The image → PDF writer and the PDF structural verifier are small internal packages using `compress/zlib`, `image/png`, `image/jpeg`, and the standard library.
- Target stripped binary size: **≤ 10 MiB**.
- Hard ceiling: **15 MiB**.
- Any proposed Go dependency that adds more than 1 MiB to the stripped binary must be rejected or replaced with internal code.

All new media work is subprocess-based. The expected binary growth from the new format table, CLI flags, and orchestration code is well under 1 MiB.

---

## 5. CLI Surface

Existing command shapes are unchanged:

```bash
siphon convert <source> --to <fmt> [flags]
siphon inspect <source> --to <fmt>
siphon doctor
```

Examples:

```bash
# PDF / image
siphon convert scan.pdf --to png
siphon convert scan.pdf --to jpg --dpi 300 --quality 85
siphon convert photo.png --to pdf
siphon convert photo.jpg --to pdf --drop-metadata
siphon inspect report.pdf --to png

# Container remux
siphon convert film.mp4 --to mkv
siphon convert camera.mov --to mkv
siphon convert film.mkv --to mov

# Transcode-required or explicit transcode
siphon inspect clip.webm --to mp4
siphon convert clip.webm --to mp4 --transcode
siphon convert film.mp4 --to webm --transcode

# Audio extraction and GIF
siphon convert movie.mp4 --to mp3
siphon convert animation.gif --to mp4
siphon convert clip.mp4 --to gif --width 640 --fps 15
```

### 5.1 New flags

| Flag | Applies to | Meaning |
|---|---|---|
| `--dpi <n>` | PDF → image | Render resolution; default `150`; valid range `36..600` |
| `--max-pixels <n>` | PDF → image | Maximum output pixels; default `100000000`; must be positive |
| `--quality <q>` | PDF → JPG | JPEG quality; default `90`; valid range `1..100`; existing image flag reused |
| `--transcode` | WebM ↔ MP4 | Permit re-encoding when remux is impossible or not requested; never implicit |
| `--video-quality <crf>` | MP4/WebM transcode, GIF → MP4 | CRF `0..51`; default `23` for H.264 and `32` for VP9 |
| `--audio-bitrate <kbps>` | WebM ↔ MP4, video → MP3 | AAC/Opus/MP3 bitrate; defaults `128` AAC, `96` Opus; MP3 uses VBR V2 unless set |
| `--fps <n>` | GIF → MP4, MP4 → GIF | Output frame rate; default `30` for GIF → MP4, `12` for MP4 → GIF |
| `--width <px>` | MP4 → GIF | Output width; default `480`; height follows aspect ratio |
| `--bg <color>` | GIF → MP4 | Background for GIF transparency; existing image flag reused; required when the GIF has transparency |
| `--audio-track <n>` | video → MP3 | Zero-based audio-track ordinal; default `0` (first audio stream) |
| `--drop-incompatible` | container remux | Drop streams the target container cannot carry; otherwise refuse |
| `--allow-nonstandard` | MP4/WebM remux | Permit VP9/Opus combinations that are muxable but poorly supported in MP4 |

`--quality` has no effect on PNG, PDF, or video output. `--video-quality` applies only to video transcodes. `--dpi` and `--max-pixels` apply only to PDF → image. Passing a flag outside its applicable pair is `E_USAGE`, not a silent ignore.

`--mode auto` resolves to `safe` for all new pairs. MKV → MP4 retains v1.7's `relocate` default. Explicit `--mode relocate` or `--mode reclaim` on any new pair is `E_MODE_UNSUPPORTED` with the supported mode list.

`--drop-metadata` applies to image → PDF when EXIF/ICC cannot be translated, and to container/transcode pairs when tags or chapters cannot be carried. See §11.

There is deliberately no `--pages`, `--password`, `--page-size`, `--margin`, `--staging`, or MP3 → video mode in v0.2. GIF output always loops forever; a loop-count flag is deferred.

### 5.2 `inspect` output

PDF → image:

```text
scan.pdf → scan.png
  mode        safe (only mode for PDF/image)
  pages       1 (single-page requirement met)
  page        612 × 792 pt, rotation 0
  render      150 DPI → 1275 × 1650 px
  output      8-bit sRGB PNG, white background
  free needed 12 MB     you have 3.1 GB
  metadata    PDF document metadata is not carried; reported
```

Image → PDF:

```text
photo.png → photo.pdf
  mode        safe (only mode for PDF/image)
  page        4000 × 3000 px at 96 DPI → 41.7 × 31.3 in
  alpha       preserved with SMask
  color       ICC profile preserved
  output      PDF 1.7, one page
  free needed 48 MB     you have 3.1 GB
```

Container remux:

```text
film.mp4 → film.mkv
  mode        safe (only mode for new container pairs)
  remux       copy H.264 video, AAC audio
  streams     1 video, 1 audio, 0 subtitles
  output      classic MKV
  free needed 1.2 GB     you have 3.1 GB
```

Transcode-required refusal:

```text
clip.webm → clip.mp4
  refusal     E_TRANSCODE_REQUIRED
  mode        safe (only mode for new container pairs)
  transcode   required: VP8 video, Vorbis audio
  video       H.264 CRF 23, yuv420p
  audio       AAC 128 kbps
  output      classic MP4
  free needed 780 MB     you have 3.1 GB
  hint        rerun with --transcode to accept re-encoding
```

Audio extraction:

```text
movie.mp4 → movie.mp3
  mode        safe (only mode for new audio pairs)
  audio       track 0 (AAC) → MP3 VBR V2
  video       dropped
  output      MP3
  free needed 24 MB     you have 3.1 GB
```

For a multi-page PDF, `inspect` exits 2 with `E_PDF_MULTIPAGE`, `source_untouched: true`, and a message naming the actual page count:

```text
E_PDF_MULTIPAGE: report.pdf has 7 pages; PDF → image supports exactly one page
```

### 5.3 JSON plan additions

`--json` keeps the v1.7 envelope:

```json
{"ok":true,"plan":{...},"source_untouched":true}
```

New optional plan fields:

```json
{
  "pdf": {
    "pages": 1,
    "page_width_pt": 612,
    "page_height_pt": 792,
    "rotation": 0,
    "encrypted": false
  },
  "render": {
    "dpi": 150,
    "width_px": 1275,
    "height_px": 1650,
    "max_pixels": 100000000
  },
  "page": {
    "dpi": 96,
    "width_pt": 3000,
    "height_pt": 2250
  },
  "media": {
    "source_container": "mp4",
    "target_container": "mkv",
    "remux": true,
    "transcode": false,
    "streams": [
      {"type": "video", "codec": "h264", "action": "copy"},
      {"type": "audio", "codec": "aac", "action": "copy"}
    ]
  },
  "transcode": {
    "video_codec": "h264",
    "video_quality": 23,
    "audio_codec": "aac",
    "audio_bitrate": 128
  },
  "audio": {
    "track": 0,
    "source_codec": "aac",
    "target_codec": "mp3"
  },
  "gif": {
    "width": 480,
    "fps": 12,
    "loop": 0
  }
}
```

- The example combines fields from different pairs for illustration; a real plan includes only the groups relevant to its source/target.
- PDF sources include `pdf` and, for PDF → image, `render`.
- Image sources include `page` for image → PDF.
- Video/audio sources include `media`; transcode plans include `transcode`.
- Video → MP3 includes `audio`; GIF output includes `gif`.
- Existing fields remain unchanged.
- On refusal, `ok` is false, `code` is the stable `E_*` code, and `source_untouched` is true.
- When `--transcode` is required but absent, `ok` is false, `code` is `E_TRANSCODE_REQUIRED`, and the plan still includes the stream map and the proposed transcode settings.
- Refusals may include an additive `hints` object with booleans such as `transcode_required`, `drop_incompatible`, `drop_metadata`, and `bg_required`. The KDE wrapper uses these booleans, never human prose, to decide whether to prompt and retry.

---

## 6. Pre-Flight Gate

All v1.7 checks that apply to safe image work remain in force: regular file, no symlink, writable directory, same device, target collision, free space, magic bytes, dependency versions, root refusal, and source-change detection.

The following checks are added or specialized.

### 6.1 PDF source checks

| # | Check | Refusal code |
|---|---|---|
| P1 | Extension is `.pdf`; `%PDF-` appears within the first 1024 bytes | `E_SOURCE_CORRUPT` |
| P2 | `pdfinfo -box -upw ''` runs under `LC_ALL=C`, with stdin from `/dev/null` and a deadline; a password/encryption diagnostic is classified as encrypted | `E_PDF_ENCRYPTED` when a password is required; `E_PDF_INVALID` otherwise |
| P3 | `Encrypted: yes` is absent | `E_PDF_ENCRYPTED` |
| P4 | `Pages` is exactly `1` | `E_PDF_MULTIPAGE` when >1; `E_PDF_INVALID` when 0 or unparseable |
| P5 | `CropBox` (or `MediaBox`) has positive width and height | `E_PDF_INVALID` |
| P6 | Page rotation is 0, 90, 180, or 270 | `E_PDF_INVALID` |
| P7 | `width_px × height_px ≤ --max-pixels` | `E_PDF_TOO_LARGE` |
| P8 | `pdfinfo` and `pdftoppm` meet the minimum version | `E_DEPENDENCY` / `E_DEPENDENCY_OUTDATED` |
| P9 | Free space covers source + estimated output | `E_NO_SPACE` |

The page count is taken from `pdfinfo -box`, not guessed from the rendered output. `-upw ''` lets `pdfinfo` identify a PDF that opens with an empty user password so it can be refused explicitly; a PDF that requires a non-empty password is classified as encrypted from the failure diagnostic. `pdfinfo` is never allowed to prompt for a password, and an encrypted PDF is refused before rendering.

### 6.2 Image source checks

| # | Check | Refusal code |
|---|---|---|
| I1 | PNG or JPEG magic bytes match the extension | `E_SOURCE_CORRUPT` |
| I2 | Image decodes successfully | `E_SOURCE_CORRUPT` |
| I3 | Image is 8 bits per channel | `E_FORMAT_UNSUPPORTED` |
| I4 | JPEG is RGB or grayscale; CMYK JPEG is not supported in v0.2 | `E_CODEC_INCOMPATIBLE` |
| I5 | Image dimensions are positive and `width_px × height_px ≤ 100000000` | `E_SOURCE_INVALID` |
| I6 | ICC profile is valid and embeddable, or `--drop-metadata` is given | `E_METADATA_DROP` |
| I7 | EXIF fields with no PDF equivalent are reported and accepted only with `--drop-metadata` | `E_METADATA_DROP` |
| I8 | Free space covers source + estimated PDF | `E_NO_SPACE` |

EXIF `Orientation` is not a refusal: it is applied to the page geometry. The other EXIF fields follow §11.

### 6.3 Common checks

- `--to` must be one of `png`, `jpg`, `pdf`, `mkv`, `mov`, `mp4`, `webm`, `mp3`, `gif`.
- The source extension and `--to` must form one of the supported pairs in §3.
- Target path must differ from source path.
- Target must not exist unless `--force`.
- Source and target directory must be on the same device.
- Source must not be a symlink; hardlinks require `--allow-hardlinks`.
- UID 0 requires `--allow-root`, as in v1.7.
- A stale temp from a previous run for these pairs is reaped; there is no journal for them.

On any refusal, the source is byte-identical and the directory listing is unchanged.

### 6.4 Container remux checks

| # | Check | Refusal code |
|---|---|---|
| C1 | Source is MP4, MOV, or MKV as claimed; target is MKV or MOV | `E_FORMAT_UNSUPPORTED` |
| C2 | `ffprobe` reports a non-empty video stream and a valid duration | `E_SOURCE_CORRUPT` |
| C3 | Every retained stream is copyable to the target container, or `--drop-incompatible` is given | `E_CODEC_INCOMPATIBLE` |
| C4 | At least one video stream remains after the map | `E_CODEC_INCOMPATIBLE` |
| C5 | `--allow-nonstandard` is present when a retained codec is marked non-standard in §9.2 or §9.3 | `E_CODEC_INCOMPATIBLE` when it is absent |
| C6 | `--transcode` is not combined with a remux-only pair | `E_USAGE` |
| C7 | Unsupported subtitle streams are refused or `--drop-incompatible` is given; chapters/attachments are refused or `--drop-metadata` is given | `E_CODEC_INCOMPATIBLE` / `E_METADATA_DROP` |
| C8 | Free space covers source + estimated output | `E_NO_SPACE` |

### 6.5 WebM ↔ MP4 checks

| # | Check | Refusal code |
|---|---|---|
| W1 | Source/target are WebM/MP4 as claimed | `E_FORMAT_UNSUPPORTED` |
| W2 | `ffprobe` reports a video stream and valid duration | `E_SOURCE_CORRUPT` |
| W3 | If the streams are remux-compatible, build a copy plan; otherwise a transcode plan | — |
| W4 | Transcode is required but `--transcode` is absent | `E_TRANSCODE_REQUIRED` |
| W5 | `--transcode` is present but a required encoder/filter is missing | `E_DEPENDENCY` |
| W6 | For VP9/Opus-in-MP4 remux, `--allow-nonstandard` is present | `E_CODEC_INCOMPATIBLE` when it is absent |
| W7 | Free space covers source + estimated output | `E_NO_SPACE` |

`E_TRANSCODE_REQUIRED` includes the proposed settings in the JSON plan, so the KDE wrapper can show the lossy cost and retry with `--transcode`.

W6 applies only to the remux path. With `--transcode`, VP9/Opus are decoded and re-encoded to H.264/AAC, so `--allow-nonstandard` is irrelevant.

### 6.6 Audio extraction and GIF checks

| # | Check | Refusal code |
|---|---|---|
| A1 | Video source has at least one audio stream | `E_NO_AUDIO` |
| A2 | `--audio-track` is within the audio-stream ordinal range | `E_AUDIO_TRACK` |
| A3 | `libmp3lame` is available | `E_DEPENDENCY` |
| A4 | GIF source decodes and has at least one frame | `E_SOURCE_CORRUPT` |
| A5 | GIF transparency is present and `--bg` is absent | `E_ALPHA_UNSUPPORTED` |
| A6 | MP4 → GIF source has a video stream | `E_SOURCE_CORRUPT` |
| A7 | `--width`, `--fps`, and `--video-quality` are in range | `E_USAGE` |
| A8 | Free space covers source + estimated output | `E_NO_SPACE` |

For GIF → MP4, H.264 requires even dimensions. Siphon pads by at most one pixel on the right/bottom with the `--bg` color; this is reported in `inspect`.

### 6.7 New error codes

| Code | Exit | Meaning |
|---|---|---|
| `E_PDF_MULTIPAGE` | 2 | PDF has more than one page; PDF → image supports exactly one page |
| `E_PDF_ENCRYPTED` | 2 | PDF is encrypted or requires a password |
| `E_PDF_INVALID` | 2 | PDF is malformed, zero-page, unparseable, or lacks a usable page box |
| `E_PDF_TOO_LARGE` | 2 | Rendered PDF page would exceed `--max-pixels` |
| `E_TRANSCODE_REQUIRED` | 2 | The WebM ↔ MP4 pair needs re-encoding; rerun with `--transcode` |
| `E_NO_AUDIO` | 2 | Video source has no audio stream |
| `E_AUDIO_TRACK` | 2 | Requested audio track does not exist |

Existing codes are reused where they already fit: `E_SOURCE_CORRUPT` for bad magic bytes, `E_DEPENDENCY` / `E_DEPENDENCY_OUTDATED` for poppler or ffmpeg features, `E_MODE_UNSUPPORTED` for `relocate`/`reclaim`, `E_NO_SPACE` for the pre-flight space gate, `E_METADATA_DROP` for metadata refusal, `E_VERIFY_FAILED` for output verification, and `E_SOURCE_CHANGED` when the source changes during work.

---

## 7. PDF → Image Protocol

### 7.1 Render

1. Open the source with `O_RDONLY|O_NOFOLLOW`, take an advisory `flock`, and snapshot `dev`, `ino`, `size`, `mtime`, and `ctime`.
2. Run the §6.1 pre-flight.
3. Create a unique same-directory temp prefix, for example `.siphon-scan-<random>`. `pdftoppm` will create `<prefix>.png` or `<prefix>.jpg`.
4. Invoke `pdftoppm` with an argument vector, never a shell:

   ```text
   pdftoppm -f 1 -l 1 -singlefile -cropbox -r <dpi> -png <absolute-source> <prefix>
   pdftoppm -f 1 -l 1 -singlefile -cropbox -r <dpi> -jpeg -jpegopt quality=<q>,optimize=y <absolute-source> <prefix>
   ```

   - The source is passed as an absolute path, so a path beginning with `-` cannot be parsed as an option.
   - `LC_ALL=C`.
   - stdin is `/dev/null`; stderr is captured and sanitized.
   - The subprocess shares Siphon's signal context and `--timeout`.
   - `-cropbox` makes the visible page match the PDF's CropBox. If no CropBox exists, poppler uses the MediaBox.
5. Require exactly one output file: `<prefix>.png` or `<prefix>.jpg`. Any page-number suffix, extra file, or missing file is a failure.
6. Re-stat the source. If `dev`, `ino`, `size`, `mtime`, or `ctime` changed since the snapshot, discard the temp and return `E_SOURCE_CHANGED`.
7. Verify the output:
   - It is a regular file and non-empty.
   - It decodes as PNG or JPEG with the standard library.
   - Its pixel dimensions equal the expected dimensions within 1 pixel.
   - Expected dimensions are computed from the rotated page box:

     ```text
     width_px  = ceil(width_pt  / 72 × dpi)
     height_px = ceil(height_pt / 72 × dpi)
     ```

     For rotation 90 or 270, swap width and height before rendering.
   - For PNG, the decoded image has no alpha channel. For JPEG, the decoded image is valid.
8. `fsync` the temp, transfer mode/uid/gid/xattrs per v1.7 §6.5, install with `renameat2` (or the v1.7 `--force` path), unlink the source, and `fsync` the directory.
9. Remove the temp prefix on every failure path.

### 7.2 Output properties

- PNG: 8-bit sRGB, no alpha, white background.
- JPEG: 8-bit sRGB, baseline-valid, quality `--quality` (default 90), no alpha.
- PDF page annotations and form appearances are rendered according to poppler's page rendering; interactive form data is not filled.
- PDF outlines, links, attachments, page labels, signatures, and document metadata are not carried. This is reported in `inspect`.
- Output DPI metadata is not part of the guarantee; pixel dimensions are authoritative.

### 7.3 Failure behavior

| Condition | Result |
|---|---|
| Multi-page PDF | `E_PDF_MULTIPAGE`, exit 2, source untouched, no output |
| Encrypted PDF | `E_PDF_ENCRYPTED`, exit 2, source untouched |
| Malformed PDF or renderer parse failure | `E_PDF_INVALID`, exit 2, source untouched |
| Pixel cap exceeded | `E_PDF_TOO_LARGE`, exit 2, source untouched |
| Missing/old poppler | `E_DEPENDENCY` / `E_DEPENDENCY_OUTDATED`, exit 2 |
| Target exists without `--force` | `E_TARGET_EXISTS`, exit 2 |
| Source changed during render | `E_SOURCE_CHANGED`, exit 2, temp removed, source untouched |
| Renderer killed by signal | exit 130, temp removed, source untouched |
| Renderer killed by timeout | exit 1, temp removed, source untouched |
| Output fails verification | `E_VERIFY_FAILED`, exit 3, temp removed, source untouched |
| Disk full during render | `E_NO_SPACE_RUNTIME`, exit 1, temp removed, source untouched |

---

## 8. Image → PDF Protocol

### 8.1 Decode and geometry

1. Open and snapshot the source as in §7.1.
2. Decode with the Go standard library.
3. Apply EXIF orientation as a page transform, not by resampling pixels.
4. Determine the source DPI:
   - JPEG JFIF density or PNG `pHYs`, when present and within `36..1200`.
   - Otherwise `96 DPI`.
5. Page size is the image's pixel dimensions at that DPI:

   ```text
   width_pt  = width_px  / dpi × 72
   height_pt = height_px / dpi × 72
   ```

   For EXIF orientation 90 or 270, swap the page width and height. There are no margins; the image fills the page.

### 8.2 PDF construction

The output is a valid PDF 1.7 document with exactly one page.

- **JPEG source:** embed the original JPEG bitstream as a `DCTDecode` image XObject. Do not re-encode. Support 8-bit RGB and grayscale.
- **PNG source:** decode to 8-bit samples and embed as a `FlateDecode` image XObject. Support grayscale, RGB, palette (expanded to RGB), and alpha.
- **Alpha:** store as an `SMask`; if every pixel is opaque, omit the mask.
- **Color:** embed an ICC profile as an `ICCBased` color space when present; otherwise use `DeviceRGB` or `DeviceGray`.
- **EXIF orientation:** apply with the page content transformation matrix. The embedded pixels are unchanged.
- **Page content:** a single `q ... cm /Im0 Do Q` stream; `MediaBox` equals the page size.
- **Document info:** `Producer = Siphon <version>`; `CreationDate` from EXIF `DateTimeOriginal` when valid, otherwise the current UTC time; `ModDate` equal to `CreationDate`; `Creator` is the EXIF camera make/model when both are present, otherwise `Siphon <version>`.
- **No** encryption, outlines, attachments, JavaScript, or incremental updates.
- **No** PDF/A or PDF/X claim.

### 8.3 Verification

Before installation, parse the generated PDF structurally:

1. Header is `%PDF-1.7`.
2. The xref table offsets resolve to the expected objects.
3. The catalog has exactly one page; `/Count 1`.
4. The page has a positive `MediaBox`.
5. The image XObject has the expected `/Width`, `/Height`, `/BitsPerComponent`, and `/ColorSpace`.
6. Every stream length matches its declared `/Length`.
7. An `SMask` is present exactly when the source had non-opaque alpha.
8. The file ends with `%%EOF`.

Tests may smoke-render the generated PDF with `pdftoppm` when poppler is available, but production image → PDF success does **not** depend on poppler. The structural verifier is the contract.

### 8.4 Installation

After verification: re-stat the source and refuse with `E_SOURCE_CHANGED` if `dev`, `ino`, `size`, `mtime`, or `ctime` changed; then `fsync`, transfer metadata, install atomically, unlink the source, and `fsync` the directory. On any failure, remove the temp and leave the source untouched.

---

## 9. Container Remux Protocol

Container remuxes copy compressed streams. They never re-encode, never change resolution or frame rate, and never create a fragmented container.

### 9.1 Pipeline

1. Open the source with `O_RDONLY|O_NOFOLLOW`, take an advisory `flock`, and snapshot `dev`, `ino`, `size`, `mtime`, and `ctime`.
2. Run the §6.4 pre-flight.
3. `ffprobe -v error -show_streams -show_format -show_chapters -of json` to build an explicit stream map.
4. Create a same-directory temp with the target extension, for example `.siphon-film-<random>.mkv`.
5. Run `ffmpeg` with an argument vector, never a shell:

   ```text
   # illustrative map; the implementation maps every retained stream explicitly
   ffmpeg -nostdin -hide_banner -loglevel error -xerror \
     -i <absolute-source> \
     -map 0:v:0 -map 0:a:0 -map 0:a:1? \
     -c copy -map_metadata 0 -map_chapters 0 \
     -f matroska <temp>
   ```

   - MP4/MOV → MKV uses `-f matroska`.
   - MKV → MOV uses `-f mov`; `-movflags +faststart` is allowed.
   - Only streams permitted by §9.2 are mapped. There is no wildcard copy of an unsupported subtitle or attachment stream.
   - Every compatible audio track is copied; this is not a “first audio only” remux. The example shows two audio maps. Subtitle maps are added only when §9.2 permits them.
   - `-map_metadata 0` copies global and track tags where the target can represent them.
   - `-map_chapters 0` applies only when the target supports chapters. MKV → MOV chapters are dropped only with `--drop-metadata`; otherwise the pre-flight refuses.
   - `--drop-incompatible` removes refused subtitle/video/audio streams from the map; without it, the pre-flight refuses.
   - `--drop-metadata` removes chapters/attachments that the target cannot carry; without it, the pre-flight refuses.
   - `--allow-nonstandard` applies only where §9.2 or §9.3 says it does.
6. Verify per §9.4.
7. Re-stat the source; on change, remove the temp and return `E_SOURCE_CHANGED`.
8. `fsync` the temp, transfer mode/uid/gid/xattrs per v1.7 §6.5, install atomically, unlink the source, and `fsync` the directory.
9. Remove the temp on every failure path.

### 9.2 Codec matrix

`copy` means `-c copy`. `unsupported` means `E_CODEC_INCOMPATIBLE` unless `--drop-incompatible` is given.

| Stream | MP4 → MKV | MOV → MKV | MKV → MOV |
|---|---|---|---|
| H.264 / AVC | copy | copy | copy |
| HEVC / H.265 | copy | copy | copy |
| AV1 | copy | copy | unsupported |
| MPEG-4 Part 2 | copy | copy | copy |
| ProRes | copy | copy | copy |
| VP8 / VP9 | copy | copy | unsupported |
| AAC | copy | copy | copy |
| ALAC | copy | copy | copy |
| AC-3 / E-AC-3 | copy | copy | copy |
| PCM | copy | copy | copy |
| FLAC / Opus / Vorbis | copy | copy | unsupported |
| mov_text subtitles | unsupported | unsupported | — |
| SRT / ASS / SSA / PGS / VobSub | — | — | unsupported |
| Chapters | copy when exposed by ffmpeg | copy when exposed | drop and report with `--drop-metadata` |
| Attachments | — | — | drop and report with `--drop-metadata` |
| Timecode tracks | drop and report with `--drop-metadata` | drop and report with `--drop-metadata` | drop and report with `--drop-metadata` |

Notes:

- MP4 and MOV can contain streams that are not typical for the container; the matrix is by codec, not by source extension.
- MKV accepts all listed video/audio codecs. The MKV → MOV restrictions are about QuickTime compatibility, not muxability.
- `--allow-nonstandard` does not make VP9/Opus/FLAC standard in MOV; those streams remain unsupported in MOV.
- Subtitle conversion is not a remux. A mov_text subtitle is not copied into MKV, and SRT/ASS/PGS is not copied into MOV.
- A file with no video stream is not a valid video remux; it is refused even if `--drop-incompatible` is given.

### 9.3 WebM ↔ MP4 remux path

WebM and MP4 share some codecs but not the common ones.

| Direction | Remux-compatible | Requires `--transcode` |
|---|---|---|
| WebM → MP4 | VP9/AV1 video + Opus audio, with `--allow-nonstandard` for VP9/Opus-in-MP4 | VP8/Vorbis, or any stream the MP4 target cannot carry |
| MP4 → WebM | VP9/AV1 video + Opus audio | H.264/HEVC/MPEG-4 video, AAC/AC-3 audio, or any stream WebM cannot carry |

- If the streams are remux-compatible, §9.1 is used and the output is a classic MP4 or WebM.
- `--drop-incompatible` may remove extra streams in the remux path, but the primary video stream must remain, and the primary audio stream must remain when the source has audio. If dropping would leave an invalid target, the conversion is refused.
- If remux is impossible, `inspect`/`convert` refuses with `E_TRANSCODE_REQUIRED` and includes the proposed transcode settings.
- `--transcode` switches to §10. It is never implied by `--allow-nonstandard`.
- VP9/Opus in MP4 is muxable but poorly supported; `--allow-nonstandard` is required. The `inspect` output names this compatibility cost.
- Vorbis is not muxable in MP4; it is a transcode-only source.
- MP4 → WebM with H.264/AAC always requires `--transcode`.

### 9.4 Remux verification

1. `ffprobe -v error -show_streams -show_format -of json` on the candidate: stream types and codecs match the plan.
2. Duration is within 1% or two frames of the source.
3. Per-stream packet counts match the source exactly for every copied stream.
4. `ffmpeg -v error -xerror -i candidate -f null -` produces no errors.
5. Chapters, subtitles, and attachments are present exactly when the plan says they are.
6. Size is greater than zero and not absurdly small for the duration.
7. No unexpected stream was added.

A verification failure is `E_VERIFY_FAILED`, exit 3, temp removed, source untouched.

---

## 10. Transcode Protocol

Transcodes re-encode video and/or audio. They are used only when the pair cannot remux or when the user explicitly passes `--transcode` for WebM ↔ MP4. GIF ↔ MP4 and video → MP3 are inherently transcodes; choosing the target is the explicit lossy decision.

### 10.1 Common pipeline

1. Open, snapshot, and lock the source as in §9.1.
2. Run the relevant pre-flight from §6.5 or §6.6.
3. Create a same-directory temp with the target extension.
4. Run `ffmpeg` with `-nostdin -hide_banner -loglevel error -xerror`, an explicit map, and the codec settings below.
5. Copy global/track tags with `-map_metadata 0` where the target can represent them.
6. Subtitles and chapters follow the metadata policy in §11. They are not silently discarded.
7. Verify per §10.7, re-stat the source, `fsync`, transfer metadata, install atomically, unlink the source, and `fsync` the directory.
8. Remove the temp on every failure path.

H.264 output uses `yuv420p` and even dimensions. If the source has an odd dimension, Siphon pads by at most one pixel on the right/bottom with black; `inspect` reports the padded dimensions. VP9 output uses even dimensions for broad player compatibility.

### 10.2 WebM → MP4

```text
ffmpeg -nostdin -hide_banner -loglevel error -xerror \
  -i <source> -map 0:v:0 -map 0:a:0? \
  -c:v libx264 -preset medium -crf <video-quality> -pix_fmt yuv420p \
  -c:a aac -b:a <audio-bitrate>k \
  -movflags +faststart -map_metadata 0 \
  -f mp4 <temp>
```

- Default video quality: CRF 23.
- Default audio: AAC 128 kbps.
- One video stream and the first audio stream are retained. Additional audio tracks are dropped and reported; a future `--audio-track` extension can select them.
- Subtitles are not converted in v0.2; they are dropped and reported only with `--drop-incompatible`, otherwise the conversion refuses with `E_CODEC_INCOMPATIBLE`.
- The output is a classic MP4, never fragmented.

### 10.3 MP4 → WebM

```text
ffmpeg -nostdin -hide_banner -loglevel error -xerror \
  -i <source> -map 0:v:0 -map 0:a:0? \
  -c:v libvpx-vp9 -crf <video-quality> -b:v 0 -row-mt 1 -pix_fmt yuv420p \
  -c:a libopus -b:a <audio-bitrate>k \
  -map_metadata 0 -f webm <temp>
```

- Default video quality: CRF 32.
- Default audio: Opus 96 kbps.
- One video stream and the first audio stream are retained. Additional audio tracks are dropped and reported.
- Subtitles are not converted; they are dropped and reported only with `--drop-incompatible`, otherwise the conversion refuses with `E_CODEC_INCOMPATIBLE`.
- The output is WebM. No faststart flag is used.

### 10.4 Video → MP3

```text
ffmpeg -nostdin -hide_banner -loglevel error -xerror \
  -i <source> -map 0:a:<audio-track> -vn \
  -c:a libmp3lame -q:a 2 -map_metadata 0 \
  -f mp3 <temp>
```

- `--audio-track` is a zero-based ordinal among audio streams; default `0`.
- No audio stream → `E_NO_AUDIO`; an out-of-range ordinal → `E_AUDIO_TRACK`.
- Default is LAME VBR `-q:a 2`. `--audio-bitrate` switches to CBR (`-b:a <kbps>k`).
- Video, subtitles, chapters, and cover art are not carried. They are reported.
- MP3 → video is not supported; there is no reverse of this operation in v0.2.

### 10.5 GIF → MP4

```text
ffmpeg -nostdin -hide_banner -loglevel error -xerror \
  -i <source> -an \
  -vf "fps=<fps>,pad=ceil(iw/2)*2:ceil(ih/2)*2:0:0:color=<bg>,format=yuv420p" \
  -c:v libx264 -preset medium -crf <video-quality> \
  -movflags +faststart -f mp4 <temp>
```

- Default `--fps` is 30.
- GIF transparency is composited onto `--bg`. If the GIF has transparency and `--bg` is absent, refuse with `E_ALPHA_UNSUPPORTED`.
- `<bg>` is the `--bg` color when transparency is present, otherwise black.
- Odd dimensions are padded to even with the `--bg` color, or black when no transparency is present.
- The output has no audio. GIF timing is resampled to the requested constant frame rate; `inspect` reports the source frame rate and the output frame rate.

### 10.6 MP4 → GIF

```text
ffmpeg -nostdin -hide_banner -loglevel error -xerror \
  -i <source> -an \
  -vf "fps=<fps>,scale='min(<width>,iw)':-1:flags=lanczos,split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse" \
  -loop 0 -f gif <temp>
```

- Default `--fps` is 12; default `--width` is 480.
- `--width` is a maximum; the output is not upscaled.
- Audio, subtitles, chapters, and metadata are not carried. They are reported.
- The output loops forever. A loop-count flag is deferred.
- GIF output can be much larger than the source; §12 estimates the peak and `E_NO_SPACE` refuses before work begins.

### 10.7 Transcode verification

1. `ffprobe` on the candidate: expected container, one video stream for video targets, expected audio stream for MP3, and expected codec names.
2. Duration is within 1% or two frames of the source, except GIF output where frame timing is intentionally resampled.
3. `ffmpeg -v error -xerror -i candidate -f null -` produces no errors.
4. For video targets, dimensions are even and match the plan; GIF dimensions match the plan; audio sample rate/channels are valid.
5. MP3 duration is within 5% of the plan; for CBR, the bitrate is within 5%, and for VBR the stream is decodable and reports the planned VBR mode.
6. GIF has at least one frame, the planned dimensions, and the planned frame rate within one frame.
7. No unexpected audio stream appears in a silent output; no unexpected video stream appears in an audio output.

A verification failure is `E_VERIFY_FAILED`, exit 3, temp removed, source untouched.

---

## 11. Metadata and Color Policy

### 11.1 PDF and image

| Class | PDF → image | Image → PDF |
|---|---|---|
| ICC profile | Rasterized to sRGB; source PDF output intent is not embedded; reported | Embedded as `ICCBased`; invalid profile → `E_METADATA_DROP` unless `--drop-metadata`, in which case it is treated as sRGB and reported |
| EXIF | Not applicable | Orientation applied; `DateTimeOriginal` → `CreationDate`; `Make`/`Model` → `Creator`; remaining EXIF requires `--drop-metadata` |
| PDF Info/XMP | Not carried; reported; not an `E_METADATA_DROP` | Not applicable |
| Alpha | Composited on white; no alpha in output | Preserved with `SMask` |
| Color | Rendered to 8-bit sRGB | RGB/gray preserved; CMYK JPEG refused in v0.2 |
| DPI | Pixel dimensions authoritative | Source DPI when valid, otherwise 96 |
| Outlines, links, forms, annotations, signatures | Not preserved; reported | Not applicable |
| Attachments | Not preserved; reported | Not applicable |

The PDF → image metadata boundary is intentional: a raster image cannot represent a PDF page tree, outline, form, or signature. `inspect` reports the classes that will be lost. It is not treated as a media metadata drop because rasterization is the operation the user requested.

For image → PDF, EXIF is media metadata and follows the v1.7 refusal rule: if fields cannot be translated, `E_METADATA_DROP` is returned unless `--drop-metadata` is given. The KDE wrapper prompts before retrying with the flag.

### 11.2 Video and audio

| Class | Container remux | WebM ↔ MP4 transcode | Video → MP3 | GIF ↔ MP4 |
|---|---|---|---|---|
| Global/track tags | Copied where the target can represent them; otherwise `E_METADATA_DROP` unless `--drop-metadata` | Copied where possible; otherwise `E_METADATA_DROP` unless `--drop-metadata` | ID3 tags copied where possible; unsupported tags reported | Not carried; reported |
| Chapters | Copied for MP4/MOV → MKV and WebM ↔ MP4 when exposed; MKV → MOV drops only with `--drop-metadata` | Copied only when both containers support them; otherwise `E_METADATA_DROP` unless `--drop-metadata` | Dropped and reported | Dropped and reported |
| Attachments | MKV → MOV drops only with `--drop-metadata`; MP4/MOV → MKV has none | Dropped only with `--drop-metadata`; otherwise refused | Cover art dropped and reported | Not carried |
| Subtitles | Copied only when §9.2 permits; otherwise `E_CODEC_INCOMPATIBLE` unless `--drop-incompatible` | Not converted; `E_CODEC_INCOMPATIBLE` unless `--drop-incompatible` | Dropped and reported | Not applicable |
| Extra audio tracks | All compatible tracks copied | First audio retained; others dropped and reported | Selected track retained; others dropped and reported | Not applicable |
| Alpha | Preserved if the target codec/container carries it | Not preserved; output is opaque | Not applicable | GIF alpha flattened with `--bg`; MP4 → GIF is opaque |
| Color | Bitstream and color metadata copied | 8-bit yuv420p; 10-bit/HDR converted and reported; color tags copied best-effort | Not applicable | 256-color palette; no ICC |
| Resolution/frame rate | Unchanged | Video dimensions padded to even; frame rate unchanged unless the source is variable and the encoder normalizes it | Not applicable | GIF → MP4 resampled to `--fps`; MP4 → GIF resampled to `--fps` and `--width` |
| Audio channels | Unchanged | AAC/Opus preserve the selected track's layout where supported | MP3 is stereo; >2 channels are downmixed and reported | No audio |

- A subtitle is a stream, not metadata: unsupported subtitles use `E_CODEC_INCOMPATIBLE` / `--drop-incompatible`.
- Chapters and attachments are metadata: they use `E_METADATA_DROP` / `--drop-metadata`.
- Additional audio tracks in WebM ↔ MP4 transcodes are dropped by policy and reported; this is not a silent metadata drop.
- Video alpha is not preserved in transcode paths. The output is opaque; `inspect` reports the loss. GIF → MP4 uses `--bg`; other video alpha is composited on black.
- Container remux copies 10-bit/HDR streams and their color metadata. Transcode paths produce 8-bit yuv420p and report the conversion; HDR preservation is a non-goal for v0.2.

---

## 12. Space Estimation

All new pairs are `safe`; peak space is `source + output`. There is no 1× path and no sparse reservation. MKV → MP4 keeps the v1.7 relocate estimate.

### 12.1 PDF → image

```text
pixels = ceil(width_pt / 72 × dpi) × ceil(height_pt / 72 × dpi)
PNG estimate  = pixels × 4 + 1 MiB
JPEG estimate = pixels × 3 + 1 MiB
peak          = source_size + estimate + 2 MiB margin
```

The PNG estimate assumes 8-bit RGBA worst-case compression. The JPEG estimate is deliberately pessimistic. If the actual output exceeds the estimate, `E_NO_SPACE_RUNTIME` is returned, the temp is removed, and the source is untouched.

### 12.2 Image → PDF

```text
channels = 1 for grayscale, 3 for RGB, 4 for RGBA
raw      = width_px × height_px × channels
estimate = max(source_size, raw × 1.1) + 1 MiB
peak     = source_size + estimate + 2 MiB margin
```

The estimate covers both direct JPEG embedding and lossless PNG recompression. As with images in v1.7, over-refusal is acceptable; under-refusal is not.

A fixed **100,000,000-pixel** decoded-image cap applies to image → PDF. Larger images are refused with `E_SOURCE_INVALID` before allocation. `--max-pixels` controls the PDF → image cap and is checked before allocation and before rendering. A PDF page that would exceed the cap is refused even if disk space is available.

### 12.3 Container remux

Container remuxes use the v1.7 safe-video estimate:

```text
estimate = source × 1.03
         + 1.10 × Σ(stream_bitrate × duration) / 8
         + 4 MiB
peak     = source + estimate
```

This is deliberately pessimistic. Remuxed output normally lands at 0.95–1.02× the source. If the actual output is larger than the estimate, `E_NO_SPACE_RUNTIME` is returned, the temp is removed, and the source is untouched.

### 12.4 WebM ↔ MP4 transcode

```text
estimate = source × 1.5 + 64 MiB
peak     = source + estimate
```

The 1.5× factor covers H.264 output that is larger than an efficient VP9 source, and VP9 output that is larger than a low-bitrate H.264 source. The 64 MiB margin covers muxing overhead and keyframe bursts.

### 12.5 Video → MP3

```text
duration = source duration in seconds
estimate = duration × 320 kbps / 8 × 1.05 + 2 MiB
peak     = source + estimate
```

320 kbps is the MP3 worst-case CBR ceiling. VBR V2 output is normally much smaller. The estimate is intentionally pessimistic.

### 12.6 GIF → MP4

```text
estimate = source × 1.5 + 64 MiB
peak     = source + estimate
```

GIF is usually a poor compression format; the H.264 output is normally smaller, but the estimate covers a large, noisy GIF whose H.264 output grows.

### 12.7 MP4 → GIF

```text
pixels   = output_width × output_height × fps × duration
estimate = max(source_size, pixels × 1.5) + 32 MiB
peak     = source + estimate + 2 MiB margin
```

`pixels × 1.5` is the GIF LZW worst-case model (12-bit codes), not a typical-size estimate. The 32 MiB margin covers palette tables and frame headers. GIF output can be several times larger than the source; `E_NO_SPACE` refuses before work begins when the estimate cannot fit.

All estimates are checked before any write. Over-refusal is acceptable; under-refusal is not.

---

## 13. Verification and Finalisation

- PDF → image: verify by decoding the output and checking format, dimensions, and single-file output.
- Image → PDF: verify structurally as described in §8.3.
- Container remux: verify per §9.4.
- WebM ↔ MP4, GIF ↔ MP4, and video → MP3: verify per §10.7.
- A verification failure is `E_VERIFY_FAILED`, exit 3, with the source untouched and the temp removed.
- The source is unlinked only after the target has passed verification, been `fsync`ed, received metadata, and been installed.
- `--force` follows v1.7: the old target may be replaced, but the source is not removed until the new target is verified.
- Safe-mode crashes can leave a stray temp; the next run reaps it. There is no journal and no recovery claim for these pairs.
- SIGINT/SIGTERM kills any active `pdfinfo`, `pdftoppm`, or `ffmpeg` child before cleanup; no rename or unlink occurs after cancellation.
- Progress for ffmpeg-based pairs comes from `-progress pipe:1`; no percentage is fabricated when the encoder cannot provide one.

---

## 14. KDE Integration

New ServiceMenus, grouped where the target is the same:

| Source MIME | Action name | Target |
|---|---|---|
| `application/pdf` | Convert to PNG (single page) | `.png` |
| `application/pdf` | Convert to JPG (single page) | `.jpg` |
| `image/png` | Convert to PDF | `.pdf` |
| `image/jpeg` | Convert to PDF | `.pdf` |
| `video/mp4;video/quicktime` | Convert to MKV | `.mkv` |
| `video/x-matroska` | Convert to MOV | `.mov` |
| `video/webm` | Convert to MP4 (compatible) | `.mp4` |
| `video/mp4` | Convert to WebM | `.webm` |
| `video/mp4;video/quicktime;video/x-matroska;video/webm` | Convert to MP3 | `.mp3` |
| `image/gif` | Convert to MP4 (white background) | `.mp4` |
| `video/mp4` | Convert to GIF | `.gif` |

Requirements:

- Menus are installed in both Plasma 5 and Plasma 6 paths, as in v1.7.
- User-owned `.desktop` files are mode `0755`.
- `Exec` uses the absolute wrapper path.
- `%F` supports multi-selection.
- The wrapper calls `siphon inspect` first. A multi-page PDF is reported as a per-file `E_PDF_MULTIPAGE` failure; the batch continues.
- `E_TRANSCODE_REQUIRED` triggers a lossy-cost warning and a retry with `--transcode`.
- For WebM → MP4, the menu action prefers `--transcode` for maximum player compatibility; `--allow-nonstandard` remains a CLI-only remux choice.
- GIF → MP4 passes `--bg white`; the action name states the background so the flattening is not silent.
- `E_ALPHA_UNSUPPORTED` on the CLI reports a hint to rerun with `--bg`; the menu path already supplies white.
- `E_CODEC_INCOMPATIBLE` with a `drop_incompatible` hint prompts before retrying with `--drop-incompatible`. A missing `--allow-nonstandard` is not auto-retried; the WebM → MP4 menu prefers `--transcode` for compatibility.
- `E_METADATA_DROP` prompts before retrying with `--drop-metadata`.
- `E_NO_AUDIO` and `E_AUDIO_TRACK` are reported per file; the batch continues.
- Progress for ffmpeg-based pairs is parsed from `-progress pipe:1`. PDF/image conversions emit `progress 0` and `progress 100` only.
- The wrapper never guesses a target and never parses human prose.

---

## 15. Testing Strategy

### 15.1 Unit tests

- `pdfinfo -box` output parsing: pages, encryption, MediaBox, CropBox, rotation, page size.
- DPI and page-dimension math, including 90/270 swaps and ±1 px tolerance.
- Pixel-cap enforcement.
- PDF writer: header, xref offsets, page tree, image XObject, stream lengths, SMask, ICCBased.
- PDF structural verifier: valid file, truncated file, bad xref, wrong stream length, missing EOF.
- Image decoding: PNG gray/RGB/palette/alpha, 16-bit refusal, JPEG RGB/grayscale/CMYK refusal, EXIF orientation.
- Format-pair resolution and `--to` validation for every new pair.
- Container codec matrix: MP4/MOV → MKV and MKV → MOV decisions, `--drop-incompatible`, `--allow-nonstandard`.
- WebM ↔ MP4 remux-versus-transcode decision, including VP9/Opus-in-MP4 and H.264/AAC-in-WebM.
- Audio-track ordinal selection, no-audio classification, and MP3 downmix rules.
- GIF transparency detection, odd-dimension padding, frame-rate resampling, and palette command construction.
- Space-estimation formulas for PDF/image, container remux, WebM/MP4 transcode, video → MP3, GIF → MP4, and MP4 → GIF.
- ffmpeg argument construction: no shell, absolute paths, filenames with spaces/dashes/non-ASCII, and no wildcard maps.

### 15.2 Golden corpus

- Single-page PDFs: Letter, A4, landscape, rotations 0/90/180/270, CropBox, transparent content, grayscale, CMYK.
- PDF → PNG and PDF → JPG at 72, 150, and 300 DPI.
- Image → PDF: RGB PNG, RGBA PNG, grayscale PNG, palette PNG, RGB JPEG, grayscale JPEG, progressive JPEG, EXIF orientations 1–8, ICC profile, transparent PNG.
- Container remux: MP4 H.264/AAC → MKV; MOV H.264/AAC → MKV; MOV ProRes/PCM → MKV; MKV H.264/AAC → MOV; MP4 HEVC/AAC → MKV.
- Container refusals: MP4 with mov_text subtitle; MKV with SRT/PGS subtitle → MOV; MKV with chapters → MOV. Verify the plan reports the drop/refusal before work begins.
- WebM ↔ MP4: WebM VP8/Vorbis → MP4 with `--transcode`; WebM VP9/Opus → MP4 remux with `--allow-nonstandard`; MP4 H.264/AAC → WebM with `--transcode`; MP4 AV1/Opus → WebM remux.
- Video → MP3: MP4 AAC stereo, MKV AC-3 5.1, MOV PCM, and WebM Opus sources; VBR and CBR; multiple audio tracks; metadata tags.
- GIF → MP4: static GIF, animated GIF, transparent GIF, odd dimensions, and variable frame delays.
- MP4 → GIF: default width/fps, `--width 640 --fps 15`, a narrow source that must not be upscaled, and a source with audio that must be dropped.
- Verify container/stream properties, packet counts for remuxes, duration, dimensions, frame count, audio absence/presence, and that the source was consumed.

### 15.3 Refusal and safety tests

- Multi-page PDF: `E_PDF_MULTIPAGE`, source and directory unchanged, no output, no temp.
- Encrypted PDF: `E_PDF_ENCRYPTED`.
- Zero-page, malformed, or unparseable PDF: `E_PDF_INVALID`.
- Huge page at high DPI: `E_PDF_TOO_LARGE`.
- Missing or old poppler: `E_DEPENDENCY` / `E_DEPENDENCY_OUTDATED`.
- `--mode relocate` / `--mode reclaim` on any new pair: `E_MODE_UNSUPPORTED`; MKV → MP4 still accepts `relocate`.
- `--transcode` on a remux-only pair: `E_USAGE`.
- WebM → MP4 with VP8/Vorbis and no `--transcode`: `E_TRANSCODE_REQUIRED`.
- MP4 → WebM with H.264/AAC and no `--transcode`: `E_TRANSCODE_REQUIRED`.
- WebM → MP4 with VP9/Opus and no `--allow-nonstandard`: `E_CODEC_INCOMPATIBLE`; with the flag, remux succeeds.
- Unsupported container stream without `--drop-incompatible`: `E_CODEC_INCOMPATIBLE`; with the flag, conversion succeeds and reports the drop only when a valid video stream remains.
- Chapters/attachments that cannot be carried without `--drop-metadata`: `E_METADATA_DROP`; with the flag, succeeds and reports the loss.
- Unsupported subtitle streams without `--drop-incompatible`: `E_CODEC_INCOMPATIBLE`; with the flag, succeeds and reports the drop.
- Video with no audio track: `E_NO_AUDIO`.
- `--audio-track` out of range: `E_AUDIO_TRACK`.
- Transparent GIF → MP4 without `--bg`: `E_ALPHA_UNSUPPORTED`; with `--bg white`, succeeds.
- Missing `libx264`, `libvpx-vp9`, `libopus`, `libmp3lame`, or GIF filters: `E_DEPENDENCY` naming the feature.
- MP4 → GIF whose estimated output cannot fit: `E_NO_SPACE`; runtime disk full: `E_NO_SPACE_RUNTIME`, temp removed.
- Existing target with and without `--force`.
- Filenames with spaces, leading dashes, and non-ASCII characters.
- Symlink, hardlink, read-only directory, root refusal.
- SIGINT during `pdftoppm` or `ffmpeg`: child killed, temp removed, source untouched.
- Corrupt generated PDF/video/GIF/MP3: `E_VERIFY_FAILED`, temp removed, source untouched.
- Batch with one refused file and several valid files: the valid files still convert; the refused source is untouched.

### 15.4 KDE tests

- Mock `kdialog`/`qdbus` suite covers every new menu action, including the combined MP4/MOV → MKV, multi-MIME video → MP3, and GIF → MP4 actions.
- Multi-page PDF in a batch: one error, remaining files continue.
- Metadata prompt for image → PDF retries with `--drop-metadata`.
- `E_TRANSCODE_REQUIRED` shows the lossy settings and retries with `--transcode`.
- GIF → MP4 menu passes `--bg white`; the action name states the white background.
- No duplicate Siphon submenu.
- Menu files are `0755` and use absolute `Exec`.

### 15.5 Gate

Every change must pass:

```sh
gofmt -w .
CGO_ENABLED=0 go build ./...
CGO_ENABLED=0 go vet ./...
CGO_ENABLED=0 go test ./...
make test-kde
```

PDF tests require poppler; video/audio tests require `ffmpeg`/`ffprobe` and the encoders/filters listed in §4.2. The suite must skip cleanly with a clear message if an optional dependency is absent, but CI installs the full set. All destructive tests use temporary copies; no user media is ever a test subject.

---

## 16. Build Order

1. **Format and CLI plumbing:** add every new pair to the format table, `--to` validation, mode resolution, all new flags, JSON fields, `doctor` checks, and `inspect` plans. Keep MKV → MP4 untouched.
2. **PDF → PNG:** single-page preflight, `pdftoppm` invocation, output verification, safe finalisation, refusal tests.
3. **PDF → JPG:** quality handling, JPEG verification, dimension tests.
4. **Image → PDF:** PDF writer for JPEG and PNG, alpha/ICC/EXIF handling, structural verifier, golden tests.
5. **Container remux:** MP4 → MKV, MOV → MKV, and MKV → MOV; codec matrix, `--drop-incompatible`, chapters/subtitles, packet-count verification.
6. **Video → MP3:** audio-track selection, LAME VBR/CBR, no-audio refusal, metadata handling.
7. **WebM ↔ MP4:** remux path with `--allow-nonstandard`, transcode path with `--transcode`, H.264/AAC and VP9/Opus settings, `E_TRANSCODE_REQUIRED` flow.
8. **GIF ↔ MP4:** transparency/background handling, odd-dimension padding, palette generation, width/fps controls, space estimate.
9. **KDE:** all new ServiceMenus, wrapper retry prompts, progress parsing, mock tests, install docs.
10. **Hardening:** large files, multiple audio tracks, subtitle/chapter edge cases, missing encoders, ENOSPC, SIGINT during ffmpeg, batch behavior, and fuzzing the parsers/argument builders.

The PDF/image path is intentionally first: it is smaller, uses an external renderer, and validates the safe-finalisation and single-page refusal contracts before the media work. Container remux follows because it shares the ffprobe/stream-map code and has no encoder dependency. Video → MP3 is next because it is a simpler ffmpeg invocation than the two-way video transcodes. GIF ↔ MP4 is last because palette generation and output-size estimation are the least predictable.

MKV → MP4 is not reimplemented; it gets regression coverage to prove the new format table and CLI plumbing did not disturb v1.7's relocate path.

---

## 17. Non-Goals and Rejected Designs

### 17.1 Non-goals for v0.2

**PDF and image**

- Multi-page PDF → image, page ranges, or one image per page.
- First-page fallback for multi-page PDFs.
- Multi-image PDFs or PDF merging.
- A4/page-size fitting, margins, or print layout.
- 16-bit PNG input.
- CMYK JPEG input.
- Encrypted/password-protected PDFs, even with an empty password.
- PDF editing, splitting, merging, redaction, OCR, or text extraction.
- PDF/A, PDF/X, or signature-preserving conversion.
- Transparent PNG output from a PDF page.
- SVG, HEIC, AVIF, TIFF, or Office document conversions; those are separate format packs.

**Video and audio**

- MP3 → video, audio + still image → video, or audio visualizers. These are authoring operations, not the reverse of audio extraction.
- Direct MP4 ↔ MOV remux. The requested reverse paths are MKV → MP4 (existing) and MKV → MOV (new); use an MKV intermediate or ffmpeg for direct MP4/MOV work.
- WebM ↔ MKV, GIF ↔ WebM, animated WebP, animated AVIF, or APNG. Those are separate format packs.
- AVI, FLV, WMV, TS/M2TS, VOB, OGV, or other legacy/streaming containers.
- Subtitle conversion, burn-in, or styling. Only codecs the target container can copy are remuxed; others are dropped with `--drop-incompatible` or refused.
- Selecting a non-first audio track for WebM ↔ MP4 transcodes. v0.2 keeps the first audio stream and reports additional tracks; `--audio-track` is only for video → MP3.
- Arbitrary codec conversion beyond the listed targets: no AV1 encode, no VP8 encode, no HEVC encode, no ProRes encode, and no general “make this play anywhere” mode.
- Hardware encoding (VA-API, NVENC, QSV), 10-bit/HDR transcode preservation, and color-management changes beyond v1.7. Remux paths copy 10-bit/HDR streams; transcode paths produce 8-bit yuv420p and report the loss.
- Variable-frame-rate preservation for GIF output. GIF is resampled to the requested constant frame rate.
- GIF loop counts other than infinite.
- `--allow-nonstandard` for MOV; VP9/Opus/FLAC remain unsupported in MOV.
- Embedding ffmpeg, poppler, libx264, libvpx, or any media codec in the Siphon binary.
- A pure-Go PDF renderer or a pure-Go video transcoder.
- 1× in-place writes for any new pair. Only v1.7's MKV → MP4 relocate keeps its 1× contract.
- `relocate` or `reclaim` for PDF/image, container remux, WebM ↔ MP4, video → MP3, or GIF ↔ MP4.
- Network sources, URLs, or stdin/stdout as the primary path.
- Keeping both the source and the target. Every successful conversion consumes the source.

### 17.2 Rejected designs

- **Convert only page 1 of a multi-page PDF.** Rejected: silent data loss and a false impression that the document was converted.
- **Output a directory of page images.** Rejected: breaks the one-source/one-target in-place model and creates ambiguous source-deletion rules.
- **Use a Go PDF rendering library.** Rejected: large binary, cgo or immature rasterizers, and no benefit over poppler.
- **Use a large PDF generation library.** Rejected: the image → PDF subset is small enough to write internally and keeps the binary under budget.
- **Flatten alpha silently.** Rejected: alpha is preserved with `SMask`; PDF 1.7 supports it.
- **Drop ICC silently.** Rejected: invalid/unembeddable ICC requires `E_METADATA_DROP` unless `--drop-metadata`.
- **Pretend PDF → image is lossless.** Rejected: rasterization is stated explicitly in `inspect` and the docs.
- **Reuse `relocate` for PDF/image.** Rejected: there is no segmented reverse-cursor benefit for small files, and output size is not predictable enough for a 1× claim.
- **Transcode every container conversion by default.** Rejected: silent quality loss and much slower conversions. Remux when possible; require `--transcode` when not.
- **Use wildcard `-map 0` with `-c copy`.** Rejected: it silently carries streams the target cannot represent and creates verification ambiguity. The stream map is explicit.
- **Use `relocate` for MP4/MOV → MKV or MKV → MOV.** Rejected: the reverse-cursor/segment engine is specific to v1.7 MKV → fragmented MP4. New pairs use safe mode.
- **Flatten GIF transparency without telling the user.** Rejected: the CLI refuses without `--bg`; the KDE action names its white background.
- **Use ImageMagick or gifski for GIF conversion.** Rejected: new external dependency, larger install surface, and no need over ffmpeg's palette filters.
- **Use `sh -c` or string concatenation for ffmpeg.** Rejected: quoting bugs and injection risk with spaces, dashes, and non-ASCII filenames. Siphon passes an argv slice.
- **Embed ffmpeg or a codec library.** Rejected: binary budget, cgo/licensing complexity, and no benefit over the existing external-tool model.
- **Treat `--allow-nonstandard` as permission to transcode.** Rejected: remux compatibility and lossy re-encoding are separate decisions.
- **Select the first audio track silently and hide the others.** Rejected: the plan reports every additional track and the one that will be kept.
- **Drop unsupported subtitles silently.** Rejected: `E_CODEC_INCOMPATIBLE` unless `--drop-incompatible` explicitly authorizes the loss. Chapters and attachments use `E_METADATA_DROP` / `--drop-metadata`.
- **Claim 1× for any new pair.** Rejected: transcode output size and remux overhead are not predictable enough; peak is source + output.
- **Add MP3 → video via a generated still image.** Rejected: it is not a conversion inverse and needs layout/visual-design decisions outside this addendum.

---

## 18. Acceptance Criteria

The addendum is implemented when all of the following hold:

1. `siphon inspect one.pdf --to png` reports one page, page box, DPI, and expected pixel dimensions without changing anything.
2. `siphon convert one.pdf --to png` produces exactly one PNG, verifies it, removes the PDF, and leaves no temp or journal.
3. `siphon convert one.pdf --to jpg --dpi 300 --quality 85` honors both flags and produces a decodable JPEG with the expected dimensions.
4. `siphon convert multi.pdf --to png` exits 2 with `E_PDF_MULTIPAGE`; the PDF, directory, and any existing target are unchanged.
5. `siphon convert encrypted.pdf --to png` exits 2 with `E_PDF_ENCRYPTED`.
6. `siphon convert malformed.pdf --to png` exits 2 with `E_PDF_INVALID`.
7. `siphon convert huge.pdf --to png --dpi 600` refuses with `E_PDF_TOO_LARGE` when the pixel cap is exceeded.
8. `siphon convert photo.png --to pdf` produces a one-page PDF with the image embedded losslessly, alpha preserved via `SMask`, and ICC preserved when present.
9. `siphon convert photo.jpg --to pdf` embeds the original JPEG bitstream without re-encoding; EXIF orientation is applied without resampling.
10. `siphon convert 16bit.png --to pdf` and `siphon convert cmyk.jpg --to pdf` refuse with clear errors.
11. `siphon convert photo.jpg --to pdf` without `--drop-metadata` refuses when untranslatable EXIF is present; with the flag it succeeds and reports what was dropped.
12. `siphon convert one.pdf --to png --mode relocate` refuses with `E_MODE_UNSUPPORTED`.
13. SIGINT during a render leaves the source byte-identical and removes all temps.
14. The KDE menus appear once, run the wrapper, report multi-page PDFs per file, and continue a batch.
15. The full Go gate and `make test-kde` pass, and the stripped binary remains ≤ 10 MiB.
16. `siphon convert film.mp4 --to mkv` copies H.264/AAC, preserves duration and per-stream packet counts, removes the MP4, and leaves one MKV.
17. `siphon convert camera.mov --to mkv` copies ProRes/PCM; `siphon convert film.mkv --to mov` copies H.264/AAC. A VP9/Opus-only MKV → MOV attempt refuses with `E_CODEC_INCOMPATIBLE`; `--drop-incompatible` cannot rescue it because no video stream would remain. An MKV with H.264 video and Opus audio succeeds with `--drop-incompatible` and reports the dropped audio.
18. An MP4 with a mov_text subtitle refuses with `E_CODEC_INCOMPATIBLE`; with `--drop-incompatible` it succeeds and reports the subtitle drop.
19. `siphon convert clip.webm --to mp4` with VP8/Vorbis exits 2 with `E_TRANSCODE_REQUIRED`; with `--transcode` it produces a classic H.264/AAC MP4 and consumes the WebM.
20. `siphon convert clip.webm --to mp4` with VP9/Opus and no `--allow-nonstandard` refuses with `E_CODEC_INCOMPATIBLE`; with the flag it remuxes; with `--transcode` it produces H.264/AAC.
21. `siphon convert film.mp4 --to webm` with H.264/AAC exits 2 with `E_TRANSCODE_REQUIRED`; with `--transcode` it produces VP9/Opus WebM.
22. `siphon convert film.mp4 --to webm` with AV1/Opus remuxes without `--transcode` and preserves packet counts.
23. `siphon convert movie.mp4 --to mp3` extracts audio track 0 to MP3 VBR V2; `--audio-track 1` selects the second audio stream; a no-audio video gives `E_NO_AUDIO`; an out-of-range ordinal gives `E_AUDIO_TRACK`; the MP3 has no video stream.
24. `siphon convert animation.gif --to mp4` without `--bg` on a transparent GIF refuses with `E_ALPHA_UNSUPPORTED`; with `--bg white` it produces H.264 with no audio and even dimensions.
25. `siphon convert clip.mp4 --to gif --width 640 --fps 15` produces an animated GIF with no audio, does not upscale a narrower source, consumes the MP4, and reports the output size.
26. `--mode relocate` and `--mode reclaim` on every new pair refuse with `E_MODE_UNSUPPORTED`; MKV → MP4 still plans `relocate` under v1.7.
27. `--transcode` on MP4 → MKV refuses with `E_USAGE`; missing `--allow-nonstandard` on a VP9/Opus WebM → MP4 remux refuses with `E_CODEC_INCOMPATIBLE`.
28. A chapter or attachment that cannot be carried refuses with `E_METADATA_DROP`; with `--drop-metadata` it succeeds and reports exactly what was lost. An unsupported subtitle stream follows the `E_CODEC_INCOMPATIBLE` / `--drop-incompatible` path in criterion 18.
29. SIGINT during `ffmpeg`, a full disk during MP4 → GIF, and a corrupt generated output all leave the source byte-identical and remove the temp; the KDE menus cover every new action, prompt for `E_TRANSCODE_REQUIRED`, pass white for GIF → MP4, handle the multi-MIME MP3 action, and continue a batch after a per-file failure.
30. All new pairs use `safe` mode with peak `source + output`, `CGO_ENABLED=0` still builds, and the stripped binary remains ≤ 10 MiB.

---

## 19. The Contract for These Pairs

Siphon converts one source file to one target file in the source directory. The new pairs are safe mode only: the source stays byte-identical until the output has been written, verified, `fsync`ed, and atomically installed. MKV → MP4 is not a new pair; it retains v1.7's relocate-or-safe behavior and its existing 1× contract.

PDF → image converts exactly one page of a one-page PDF to one PNG or JPEG; a multi-page PDF is refused untouched and is never silently reduced to its first page. Image → PDF produces exactly one one-page PDF and preserves pixels, alpha, and ICC without resampling. PDF → image is rasterization and says so.

MP4/MOV → MKV and MKV → MOV remux compatible streams with `-c copy`; they do not transcode by default, and unsupported tracks are refused unless explicitly dropped. WebM ↔ MP4 remuxes when the codecs fit and requires explicit `--transcode` otherwise. Video → MP3 extracts one selected audio track and drops video. GIF ↔ MP4 are explicit transcodes; GIF transparency is flattened only with `--bg`, and MP4 → GIF drops audio. MP3 → video is not supported.

Peak space for every new pair is source + output; there is no false 1× claim. On success the source is gone and the target is in its place. On refusal, verification failure, signal, or crash, the source is still there and no temp survives.
