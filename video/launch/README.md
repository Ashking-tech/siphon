# Siphon — launch film

A 35-second, typography-led launch film for [Siphon](../../README.md), the
Linux-native in-place media converter. Built with Remotion, React and
TypeScript.

The film argues one idea in type: converting a file should not require a
second full-size copy. It moves from the pressure of `60 GB` and
`NOT ENOUGH SPACE`, through the question of why conversion needs a duplicate,
into Siphon's tail-first relocate engine and its payoff: one 60-second segment
of headroom instead of another whole file.

## Film

| | |
|---|---|
| Duration | **35.000 s** — 1050 frames |
| Picture | **1920 × 1080**, 16:9, **30 fps** |
| Video | H.264, High profile, yuv420p, CRF 16 |
| Colour | BT.709, limited range |
| Audio | AAC-LC, 48 kHz stereo, 320 kbps |
| Master | [`out/siphon-launch.mp4`](out/siphon-launch.mp4) |
| Preview | [`out/siphon-launch-preview.mp4`](out/siphon-launch-preview.mp4) — 960 × 540 |

The soundtrack is original and synthesised for this film. There are no stock
assets, samples or external audio.

## Requirements

- Node.js 20+ and npm
- A Chromium/Chrome binary. `remotion.config.ts` uses `/usr/bin/google-chrome`
  when it exists; otherwise Remotion can use its own browser, or pass
  `--browser-executable=/path/to/chrome`.
- Python 3 only if you want to regenerate the soundtrack or grain plate:
  `numpy`, `scipy` and `Pillow`.

```sh
cd video/launch
npm install
```

## Render

```sh
# Type-check the whole project
npm run typecheck

# Fast review copy: 960 × 540, H.264 CRF 22
npm run preview

# Final 1080p master: H.264 CRF 16 + AAC 320k
npm run render
```

Other useful commands:

```sh
npm run dev       # Remotion Studio, scrubbable timeline
npm run still     # one 1080p frame at frame 640
npm run assets    # regenerate grain.png and soundtrack.wav
npm run audio     # regenerate only the soundtrack
npm run grain     # regenerate only the grain plate
```

A specific still can be rendered directly:

```sh
npx remotion still src/index.ts SiphonLaunch out/preview/still.png --frame=640
```

The output directory is intentionally ignored by git. The finished master and
preview stay local under `out/`.

## Structure

```text
video/launch/
├── src/
│   ├── Launch.tsx          # scene assembly, grain, vignette, soundtrack
│   ├── Root.tsx            # Remotion composition
│   ├── timeline.ts         # scene boundaries + shared sound cue sheet
│   ├── theme.ts            # palette, type stacks, grid
│   ├── fonts.ts            # bundled font loading
│   ├── scenes/             # seven scenes, in story order
│   ├── components/         # reusable type and transition primitives
│   └── lib/anim.ts         # easing, frame ranges, deterministic hashes
├── scripts/
│   ├── build_audio.py      # original 35 s score, synthesised end to end
│   └── build_grain.py      # deterministic full-frame grain plate
├── public/
│   ├── audio/soundtrack.wav
│   ├── fonts/              # Inter Display + Hack, bundled locally
│   └── grain.png           # 1920 × 1080 grain plate
├── remotion.config.ts
└── package.json
```

## Story and timing

| Time | Scene | Beat |
|---|---|---|
| 0.0–5.0 s | 01 / Pressure | `60 GB.` → `NOT ENOUGH SPACE.` → `JUST TO CONVERT ONE FILE?` |
| 5.0–9.0 s | 02 / The question | `WHY DOES CONVERSION NEED ANOTHER COPY?` |
| 9.0–13.0 s | 03 / The name | `SIPHON.` / `CONVERT IN PLACE.` |
| 13.0–20.0 s | 04 / The method | `SEGMENT → VERIFY → RECLAIM`, tail-first grid, `ONE SEGMENT AT A TIME.` |
| 20.0–25.0 s | 05 / The payoff | `60 GB → 340 MB`, qualified as peak additional free space required |
| 25.0–31.0 s | 06 / Capabilities | `MKV → MP4`, `WAV → MP3`, `PNG → WEBP`, `JPG → PNG`; no network, telemetry, hidden state or silent quality loss |
| 31.0–35.0 s | 07 / Close | `SIPHON.` / `LESS SPACE. MORE CONTROL.` |

All timings live in [`src/timeline.ts`](src/timeline.ts). The last frame is
intentionally black, so the 35.0 s container ends on silence and darkness.

## Sound design

The score is written in Python with numpy/scipy: a low A drone, filtered
machine air, short mechanical clicks, impacts with sub-bass tails, five
segment pulses for the five handovers, and a brighter closing chord.

The cue sheet mirrors `src/timeline.ts`, so type cuts and sound hits share the
same clock. Notable cues:

| Time | Cue |
|---|---|
| 0.40 s | `60 GB.` impact |
| 1.50 s | `NOT ENOUGH SPACE.` impact |
| 3.00 s | hard cut to black |
| 5.10 s | `WHY DOES` entry |
| 6.30 s | `ANOTHER` impact |
| 9.10 s | `SIPHON.` reveal |
| 10.30 s | `CONVERT IN PLACE.` |
| 13.00 s | engineering scene enters |
| 14.60–17.27 s | five segment pulses, tail-first |
| 18.00 s | `ONE SEGMENT AT A TIME.` |
| 20.00 s | payoff scene |
| 21.35 s | slice transition begins |
| 22.15 s | `340 MB` reveal |
| 25.15–27.34 s | four format slides |
| 28.10–30.14 s | four principle statements |
| 31.25 s | final wordmark |
| 32.20 s | tagline |
| 35.00 s | end |

`npm run audio` rebuilds the 48 kHz/16-bit WAV. The mastered file measures
approximately −17.1 LUFS integrated, 7.8 LU LRA, and −1.3 dBFS true peak
before AAC encoding.

## Reusable components

| Component | Role |
|---|---|
| `AnimatedText` | Per-word or per-character spring entrances: rise, scale, slide, flip. |
| `SplitText` | Character-level fragmentation; assemble from shards or scatter on exit. |
| `MaskedText` | Overflow-clipped typographic reveals and exits. |
| `TrackingReveal` | Letter-spacing that settles into place, used for the wordmark. |
| `WordFragment` | Deliberately offset or rotated type fragments. |
| `TypeScale` | Scale a block from one size to another over a frame range. |
| `NumberMorph` | Mechanical digit roll, used for the pending-segment counter. |
| `TextTransition` | Directional handoff between two composed slides. |

Supporting primitives:

| Component | Role |
|---|---|
| `SliceTransition` | Horizontal slice shred between the `60 GB` and `340 MB` states. |
| `Chrome` | Persistent scene number / timecode / status rail. |
| `FilmGrain` / `Vignette` | Grain plate and edge falloff over the whole film. |

All motion is deterministic: springs and interpolations are driven by the
Remotion frame, and pseudo-random offsets use fixed hashes, so repeated
renders match.

## Technical accuracy

The film is deliberately specific about what Siphon does.

- **Remux, not transcode.** The `MKV → MP4` slide is a container change. The
  video and audio streams are copied, not re-encoded.
- **340 MB is illustrative, not a benchmark.** It is the approximate size of a
  single 60-second segment of a 45 Mbps file: `45 Mbps × 60 s ≈ 337.5 MB`,
  rounded to `≈ 340 MB`. Real peak headroom depends on the file's bitrate,
  keyframe layout and configured segment duration.
- **Peak, not permanent overhead.** The `relocate` engine needs the source plus
  one segment at peak, not a second complete copy. The on-screen label
  `additional free space required` and the note `peak = source + one segment`
  state the requirement, not space reclaimed by the conversion.
- **Safe and relocate differ.** `relocate` is the in-place path with
  segment-sized peak headroom. `safe` writes a full second output before the
  source is replaced, so it needs space for source + complete target.
- **Five segments is a visual simplification.** Production uses 60-second
  keyframe-aligned windows; the film uses five blocks for legibility.
- **Journaling is mode-specific.** The `journaled / exact resume` line refers to
  the relocate engine's journal. It is not a promise that every conversion path
  is equally resumable.
- **No cloud, no telemetry, no hidden state.** Siphon is a local Linux tool with
  KDE/Dolphin integration. The film makes no network or account claims because
  there is no network or account path.
- **No universal quality claims.** Lossy targets are explicit choices. The film
  says `NO SILENT QUALITY LOSS`, not that every conversion is lossless.

The product's canonical behaviour lives in the repository's
[`spec.md`](../../spec.md) and [`context.md`](../../context.md). If product
behaviour changes, update the film's microcopy and this section together.

## Type and palette

- **Inter Display** — editorial display voice, weights 900/800/700/500/400.
- **Hack** — machine voice for labels, counters and technical notes.
- **Palette** — near-black `#080909`, warm off-white `#F3F1E8`, one acid-lime
  accent `#C9F24D`.
- **Grid** — 1920 × 1080, 96 px side margins, 1728 px content width.

Fonts are bundled under `public/fonts/` so renders are reproducible offline.
Inter Display is licensed under the SIL Open Font License; Hack is licensed
under the MIT License.
