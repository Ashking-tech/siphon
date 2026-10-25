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

## Status

Working end to end: CLI (`convert`, `inspect`, `recover`, `doctor`,
`version`), pre-flight gate, journal, platform layer, the image engine
(`AtomicCopy`, PNG/JPEG/WebP with ICC/EXIF/XMP transplant and alpha policy),
the audio engine (`Sliding`, WAV→MP3 `safe` and `reclaim` with crash-safe
splice-resume), and both video engines: `Remux` (MKV→MP4 `safe`) and
`Relocate` (default MKV→fragmented MP4, tail-first journaled segments with
exact `siphon recover`). KDE integration and the GUI are next; see
`context.md` for the phase status and open verification items.
