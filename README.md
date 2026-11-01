# Siphon

**Convert media in place on Linux, without needing a second full-size copy.**

Siphon checks the conversion plan and available space before it starts. It
writes and verifies the output before consuming the source, and refuses when
it cannot safely handle a file.

![Siphon converting a video in Dolphin](video/product-gif/out/siphon-product.gif)

Siphon provides a command-line interface and an optional KDE Dolphin
ServiceMenu. It works offline and sends no telemetry.

[Website](https://ashking-tech.github.io/siphon/) ·
[Releases](https://github.com/Ashking-tech/siphon/releases)

## Install

### One command

Linux amd64 and arm64 builds are available:

```sh
curl -fsSL https://raw.githubusercontent.com/Ashking-tech/siphon/main/install.sh | sh
```

This installs the `siphon` CLI, the `siphon-gui-kde` wrapper, and the Dolphin
ServiceMenus under `~/.local`, then refreshes KDE's menu cache. The installer
verifies the release SHA256 before extracting. Restart Dolphin afterwards. If
`~/.local/bin` is not on your `PATH`, add it before running the commands below.

For a CLI-only install:

```sh
curl -fsSL https://raw.githubusercontent.com/Ashking-tech/siphon/main/install.sh | sh -s -- --no-kde
```

Pass `--version v0.2.1` to pin a release or `--prefix DIR` to install
elsewhere. Add `--help` after `sh -s --` for all options.

To inspect the script before running it:

```sh
curl -fsSLO https://raw.githubusercontent.com/Ashking-tech/siphon/main/install.sh
less install.sh
sh install.sh
```

### Manual download

Each archive contains the `siphon` CLI, `README.md`, `LICENSE`, the Dolphin
installer, the `siphon-gui-kde` wrapper, and the ServiceMenus.

| Platform | v0.2.1 archive |
| --- | --- |
| Linux amd64 | [siphon_0.2.1_linux_amd64.tar.gz](https://github.com/Ashking-tech/siphon/releases/download/v0.2.1/siphon_0.2.1_linux_amd64.tar.gz) |
| Linux arm64 | [siphon_0.2.1_linux_arm64.tar.gz](https://github.com/Ashking-tech/siphon/releases/download/v0.2.1/siphon_0.2.1_linux_arm64.tar.gz) |
| Checksums | [siphon_0.2.1_checksums.txt](https://github.com/Ashking-tech/siphon/releases/download/v0.2.1/siphon_0.2.1_checksums.txt) |

Verify the archive, extract it, and run the installer:

```sh
sha256sum --check --ignore-missing siphon_0.2.1_checksums.txt
mkdir -p siphon-0.2.1
tar -xzf siphon_0.2.1_linux_amd64.tar.gz -C siphon-0.2.1
./siphon-0.2.1/kde/install.sh
```

On arm64, replace `amd64` with `arm64` in the archive name.

## Use

```sh
# See available targets or a dry-run plan
siphon inspect photo.png
siphon inspect movie.mkv --to mp4

# Convert in place
siphon convert movie.mkv --to mp4
siphon convert photo.png --to jpg --bg white
siphon convert concert.mkv --to mp3

# Explicitly accept a lossy transcode
siphon convert clip.webm --to mp4 --transcode

# Opt into low-space WAV → MP3 mode
siphon convert recording.wav --to mp3 --mode reclaim --yes

# Resume a journaled conversion after interruption
siphon recover movie.mkv
```

`inspect` reports the plan, space requirement, and any refusal without
touching the source. Add `--json` for machine-readable output.

Successful conversions replace the source. Keep a backup if you need the
original.

## Supported conversions

| Category | Conversions |
| --- | --- |
| Video | MKV → MP4, MP4 → MKV, MOV → MKV, MKV → MOV, WebM ↔ MP4, GIF ↔ MP4 |
| Audio | WAV → MP3, video → MP3 (MP4, MKV, MOV, WebM) |
| Images | PNG ↔ JPG, PNG ↔ WebP, JPG ↔ WebP |
| Documents | Single-page PDF → PNG/JPG, PNG/JPG → single-page PDF |

Container conversions remux compatible streams without re-encoding. When a
conversion needs a lossy transcode, Siphon makes that choice explicit. PDF
rendering currently accepts single-page PDFs only.

## How the space-saving modes work

- **`safe`** writes a temporary output beside the source, verifies it, then
  atomically replaces the source. This is the default for images, documents,
  and most audio/video conversions; it needs room for both files while running.
- **`relocate`** is the default for MKV → MP4. It moves verified,
  keyframe-aligned segments one at a time, so extra space is one segment rather
  than another full film. If interrupted, `siphon recover` resumes the job.
- **`reclaim`** is an opt-in mode for WAV → MP3 that reuses space behind the
  read cursor. It requires `--yes` to confirm the in-place operation.

## Requirements

- Linux. Go 1.26.9 or newer is only needed to build from source.
- `ffmpeg` and `ffprobe` 5.0 or newer for video and audio conversions.
- `lame` 3.100 or newer for WAV → MP3.
- Poppler (`pdfinfo` and `pdftoppm`, from `poppler-utils`) for PDF → image.
- `cwebp` and `dwebp` (from `libwebp-tools`) for conversions involving WebP.

External tools are required only for the formats that use them. Run
`siphon doctor` to see which tools are installed and which conversions they
enable.

## KDE Dolphin

The quick install adds the Dolphin ServiceMenu for Plasma 5 and 6. Right-click
a file, open **Actions**, then **Siphon** to choose a target format.

![The Siphon submenu in Dolphin's Actions menu](site/media/siphon-dolphin-menu.png)

From a source checkout:

```sh
make install-kde                 # both Plasma versions
make install-kde KDEPLASMA=6     # Plasma 6 only
make uninstall-kde
```

The wrapper uses `kdialog` for prompts and batch summaries. It can use `jq` or
Python 3 to parse the CLI's JSON output; `qdbus6`, `qdbus`, or `qdbus-qt6`
enables a live progress bar. Install `libwebp-tools` to enable WebP actions.

## Build from source

```sh
git clone https://github.com/Ashking-tech/siphon.git
cd siphon
make build
./bin/siphon version
./bin/siphon doctor
```

`make build` creates `bin/siphon` with cgo disabled and stamps the version from
the nearest Git tag and the short commit.

## Development

```sh
make test
make vet
make test-kde
```

Useful references: [`spec.md`](spec.md) for the core behavior and safety
contract, [`spec.v0.2-media-formats.md`](spec.v0.2-media-formats.md) for
additional format rules, and [`context.md`](context.md) for engineering notes.
Website source is in [`site/`](site/); demo and launch-film sources are in
[`video/`](video/).

## License

MIT. Copyright (c) 2026 Ashking-tech. See [`LICENSE`](LICENSE).
