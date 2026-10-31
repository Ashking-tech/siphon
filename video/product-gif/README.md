# Siphon — website product animation

A 13.6-second, silent, 2D product animation for the Siphon website.
Built with Remotion. It is designed to loop continuously in a page.

## Deliverables

| File | Size | Notes |
|---|---:|---|
| `out/siphon-product.gif` | 2.7 MB | 1200 × 675, 20 fps, 256-colour palette, seamless loop |
| `out/siphon-product.webm` | 287 KB | 1280 × 720, VP9, preferred web delivery |
| `out/siphon-product.mp4` | 574 KB | 1280 × 720, H.264, CRF 10 |
| `out/siphon-product-hq.mp4` | 936 KB | 1920 × 1080, H.264, CRF 10 |
| `out/siphon-product-master.mp4` | 446 KB | 1280 × 720, CRF 14 master |

All outputs are silent. The composition is 408 frames at 30 fps.

## What it shows

1. **KDE Dolphin** — right-click `interstellar_4k.mkv` → **Siphon** →
   **Convert to MP4 (saves space)**. The labels and hierarchy are taken from
   `kde/servicemenus/siphon-mkv-to-mp4.desktop`.
2. **Convert in place** — 60 GB source bar, destination reservation, disk bar.
3. **Relocate processing** — 12 illustrative segments processed one at a
   time, tail-first: read → process → verify → reclaim. The source slot turns
   grey after its bytes are truncated; the verified slot turns green in the
   output.
4. **Completion** — `interstellar_4k.mp4` appears in the same Dolphin folder,
   followed by “One file in. One file out.” and “Without a second full-size
   copy.”
5. **Loop** — the final folder crossfades to the exact frame 0 composition.

The framing is deliberately illustrative, not a literal scale model of a
60 GB file. The disk callout and footnote state the example conditions:
60 GB MKV, 4K, 45 Mbps, 60 s segments, peak extra ≈ 340 MB. Relocate mode only;
the free-space requirement varies with bitrate and `--segment-duration`, and
other modes differ.

Technical basis:

- `internal/engine/relocate/` — tail-first, keyframe-aligned segments, sparse
  output reservation, verify-before-truncate.
- `spec.md` §1.1 and §4.5.4 — space table and protocol.
- `kde/servicemenus/siphon-mkv-to-mp4.desktop` — actual menu labels.

MKV → MP4 in relocate mode is a remux, not a video transcode. The animation
says “remux, no re-encode” and does not claim that every conversion mode uses
this strategy or that every mode is resumable.

## Build

```sh
cd video/product-gif
npm install
npm run render
```

Then build the web derivatives with `ffmpeg`:

```sh
# High-quality 1080p MP4
npx remotion render src/index.ts SiphonProductGif out/siphon-product-hq.mp4 \
  --scale=1.5 --crf=10 --concurrency=4

# High-quality 720p MP4
npx remotion render src/index.ts SiphonProductGif out/siphon-product.mp4 \
  --crf=10 --concurrency=4
```

```sh
ffmpeg -y -i out/siphon-product-master.mp4 -an \
  -c:v libvpx-vp9 -b:v 0 -crf 36 -row-mt 1 -cpu-used 2 \
  -pix_fmt yuv420p out/siphon-product.webm

ffmpeg -y -i out/siphon-product-master.mp4 -an \
  -c:v libx264 -crf 22 -preset slow -pix_fmt yuv420p \
  -movflags +faststart out/siphon-product.mp4

ffmpeg -y -i out/siphon-product-master.mp4 \
  -vf "fps=20,scale=1200:675:flags=lanczos,split[s0][s1];\
[s0]palettegen=max_colors=256:stats_mode=diff[p];\
[s1][p]paletteuse=dither=bayer:bayer_scale=2:diff_mode=rectangle" \
  -loop 0 out/siphon-product.gif
```

## Structure

- `src/SiphonProductGif.tsx` — timeline and crossfades.
- `src/components/Dolphin.tsx` — KDE Dolphin window, context menu, cursor.
- `src/components/Infographic.tsx` — segmented bars, process lane, disk bar.
- `src/components/FinalScene.tsx` — converted folder and closing statement.
- `src/theme.ts` — palette, type, geometry.
- `src/timeline.ts` — cues and tail-first segment timing.
