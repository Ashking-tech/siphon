# Siphon

Convert media files in place on Linux without needing twice the disk space.

- `spec.md` — frozen v1.7 product specification (the contract).
- `context.md` — living implementation checkpoint; read this first when resuming.
- `spec.v1.4.md` — historical design, superseded.

## Build

```sh
make build          # bin/siphon, CGO_ENABLED=0
make test
./bin/siphon doctor
./bin/siphon inspect photo.png --to jpg
./bin/siphon convert photo.png --to jpg --bg white
./bin/siphon convert song.wav --to mp3                    # safe
./bin/siphon convert song.wav --to mp3 --mode reclaim --yes  # in-place
./bin/siphon convert movie.mkv --to mp4                   # relocate (default)
./bin/siphon convert movie.mkv --to mp4 --mode safe       # classic MP4
./bin/siphon recover movie.mkv                            # after a crash
./bin/siphon inspect movie.mkv --to mp4
```

Runtime dependencies: `ffmpeg`/`ffprobe` ≥ 5.0, `lame` ≥ 3.100;
`cwebp`/`dwebp` for WebP pairs; `kdialog` for the KDE integration.

## KDE / Dolphin

Install the Dolphin ServiceMenus and the `siphon-gui-kde` wrapper for the
current user:

```sh
make install-kde
```

This installs the wrapper and `bin/siphon` under `~/.local/bin`, the menus in
both the Plasma 6 (`kio/servicemenus`) and Plasma 5
(`kservices5/ServiceMenus`) locations, and rebuilds the KDE service cache.
KIO de-duplicates the two paths by filename, so only one menu appears. The
installed actions use the absolute wrapper path, so they work even when
Dolphin's session PATH omits `~/.local/bin`. The menus are installed
executable (`0755`) because KDE rejects user-owned ServiceMenu files without
the execute bit. Right-click a supported file in Dolphin and choose
**Siphon**.

Useful options:

```sh
make install-kde PREFIX=/tmp/siphon-demo KDEPLASMA=6
make install-kde KDEPLASMA=both   # install both Plasma 5 and 6 menu paths (default)
make uninstall-kde
make test-kde
```

The wrapper uses `kdialog` for prompts and batch summaries. When `qdbus6`,
`qdbus`, or `qdbus-qt6` is available it drives a `kdialog --progressbar`
(supporting both the older `setValue` method and the newer D-Bus property
API); otherwise it falls back to passive popups. It reads `siphon --json`
results and progress events, never human-readable CLI prose. Install `jq` or
`python3` for the wrapper's JSON parsing. The WebP menu entries require
`libwebp-tools`.

## Status

Working end to end: CLI (`convert`, `inspect`, `recover`, `doctor`,
`version`), pre-flight gate, journal, platform layer, the image engine
(`AtomicCopy`, PNG/JPEG/WebP with ICC/EXIF/XMP transplant and alpha policy),
the audio engine (`Sliding`, WAV→MP3 `safe` and `reclaim` with crash-safe
splice-resume), and both video engines: `Remux` (MKV→MP4 `safe`) and
`Relocate` (default MKV→fragmented MP4, tail-first journaled segments with
exact `siphon recover`). The KDE/Dolphin integration is available via
`make install-kde`; the first-class GUI is next. See `context.md` for the
phase status and open verification items.
