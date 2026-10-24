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
./bin/siphon inspect movie.mkv --to mp4
```

Runtime dependencies: `ffmpeg`/`ffprobe` ≥ 5.0, `lame` ≥ 3.100;
`cwebp`/`dwebp` for WebP pairs; `kdialog` for the KDE integration.

## Status

Working end to end: CLI (`convert`, `inspect`, `recover`, `doctor`,
`version`), pre-flight gate, journal, platform layer, and the image engine
(`AtomicCopy`, PNG/JPEG/WebP with ICC/EXIF/XMP transplant and alpha policy).
Audio (`Sliding`) and video (`Remux`/`Relocate`) engines are next, in the
order documented in `context.md`.
