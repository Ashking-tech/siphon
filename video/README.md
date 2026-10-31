# Siphon — 6-second Manim explainer

A silent, six-second motion graphic for the tail-first `relocate` engine.
It is built with **Manim Community** (PyPI package `manim`), not `manimlib`.

The separate **35-second launch film** — typography-led, scored, built with
Remotion — lives in [`launch/`](launch/README.md).

The **22-second workflow explainer** — a Dolphin interaction, preflight, and
tail-first segmented relocate, built with Remotion — lives in
[`demo/`](demo/README.md). Its master is
[`demo/out/siphon-workflow.mp4`](demo/out/siphon-workflow.mp4).

The **13.6-second website product animation** — a looping, white-background
Dolphin → relocate → Dolphin infographic with GIF/WebM/MP4 deliverables — lives
in [`product-gif/`](product-gif/README.md).

The animation shows:

1. `film.mkv` split into five keyframe-aligned chunks.
2. A sparse `.mp4` reservation, with one dashed slot per output chunk.
3. Chunks processed **tail-first** (4 → 0): each one is copied into its final
   output offset, verified, and journaled.
4. Immediately after the copy lands, that chunk is truncated off the source
   tail. The cut marker rides left as the source shrinks.
5. The output is framed as one file; the source is struck through.
6. The payoff: **peak extra space = one chunk**, which is the whole point of
   `relocate` — no 2× copy.

Five chunks are used for legibility; a real relocation has many more.

## Install (Fedora)

Manim's `pycairo` and `manimpango` dependencies build from source on Linux, so
the Cairo/Pango development headers are required. No LaTeX is needed for this
scene (it uses `Text`, not `MathTex`).

```sh
cd /home/ashking/Projects/siphon/video

sudo dnf install python3.13 python3.13-devel pkgconf-pkg-config \
                 cairo-devel pango-devel ffmpeg

uv venv --python 3.13 .venv
uv pip install --python .venv/bin/python manim

.venv/bin/manim --version
```

If `uv` is not installed:

```sh
curl -LsSf https://astral.sh/uv/install.sh | sh
```

Debian/Ubuntu equivalents for the system packages:

```sh
sudo apt install python3-venv python3-dev pkg-config \
                 libcairo2-dev libpango1.0-dev ffmpeg
```

The scene expects the **Inter** and **Hack** fonts. Both are present on this
host; if they are missing, install them or changed `SANS` / `MONO` at the top
of the script.

## Render

Preview at 720p30:

```sh
cd /home/ashking/Projects/siphon/video
.venv/bin/manim -qm --fps 30 siphon_explainer.py SiphonExplainer
```

Final at 1080p30:

```sh
.venv/bin/manim -qh --fps 30 -o siphon-explainer \
    siphon_explainer.py SiphonExplainer

cp media/videos/siphon_explainer/1080p30/siphon-explainer.mp4 \
   ./siphon-explainer.mp4
```

If your Manim names the quality folder differently, locate the file with:

```sh
find media -name '*.mp4'
```

A still of the final frame:

```sh
.venv/bin/manim -qh -s -o siphon-still \
    siphon_explainer.py SiphonExplainer
```

## Timeline

| Time (s) | Beat |
|---|---|
| 0.00–0.50 | Header, format chip, `≈ 1× DISK` chip |
| 0.50–1.08 | Source chunks and dashed output reservation enter |
| 1.08–4.28 | Five `0.64 s` handovers, tail-first |
| 4.28–4.83 | Final outline, check mark, source struck through |
| 4.83–5.30 | “peak extra space: one chunk” |
| 5.30–6.00 | Hold on the end state |

## Tweaks

- **Chunk count:** `N_CHUNKS` near the top. The geometry and loop adapt.
- **Colors:** `SRC_*`, `OUT_*`, `TXT`, `MUTED`, `GRID_C`.
- **Speed:** the per-chunk `run_time` values are in `construct()`. The final
  hold is the last `self.wait(0.70)`.
- **Resolution/FPS:** pass `--resolution 1920,1080` and `--fps 60`; all timings
  are in seconds, so the duration stays 6.0 s.
- **Audio:** the render is silent by design. Mux a track afterwards if needed:
  `ffmpeg -i siphon-explainer.mp4 -i music.mp3 -shortest -c:v copy -c:a aac out.mp4`.

## Files

- `siphon_explainer.py` — the scene.
- `media/` — generated renders (gitignored).
- `.venv/` — local Manim environment (gitignored).
