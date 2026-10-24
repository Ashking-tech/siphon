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
./bin/siphon inspect movie.mkv --to mp4
```

Runtime dependencies: `ffmpeg`/`ffprobe` ≥ 5.0, `lame` ≥ 3.100;
`cwebp`/`dwebp` for WebP pairs; `kdialog` for the KDE integration.

## Status

Foundation only: CLI, pre-flight gate, journal, platform layer. Engines are
landing in the order documented in `context.md`.
