# Siphon — workflow explainer

A 22-second, silent product film built with Remotion. It shows the real Dolphin
workflow and then the relocate engine's preflight and segmented processing.

| | |
|---|---|
| Duration | 22.000 s — 660 frames |
| Picture | 1920 × 1080, 30 fps |
| Master | [`out/siphon-workflow.mp4`](out/siphon-workflow.mp4) |
| Preview | [`out/siphon-workflow-preview.mp4`](out/siphon-workflow-preview.mp4) — 960 × 540 |

## Structure

| Time | Scene |
|---|---|
| 0–7 s | Dolphin: right-click `interstellar_4k.mkv` → **Siphon** → **Convert to MP4 (saves space)** |
| 7–11 s | Preflight: inspect source, check space, determine strategy |
| 11–17 s | Segmented relocate: tail-first segment → remux → verify → reclaim |
| 17–22 s | Finalize: `interstellar_4k.mp4` appears; “One file in. One file out.” |

The ServiceMenu labels match
[`kde/servicemenus/siphon-mkv-to-mp4.desktop`](../../kde/servicemenus/siphon-mkv-to-mp4.desktop):
`Siphon` is one submenu with `Convert to MP4 (saves space)` and
`Convert to classic MP4 (needs 2× space)`.

The engine is shown tail-first, remux-only, with verification before each
source-tail truncation. The `≈ 340 MB` figure is qualified on screen as the
peak headroom for one 60-second segment of a 45 Mbps source (plus the 16 MiB
reserve), not a second 60 GB copy.

## Render

```sh
cd video/demo
npm install
npm run workflow:render    # 1080p master
npm run workflow:preview   # 960 × 540 review copy
npm run workflow:still     # frame 360 still
```

The composition is registered as `SiphonWorkflow` in `src/Root.tsx`. All timing
lives in `src/workflow/timeline.ts`; the Dolphin recreation is in
`src/workflow/components/Kde.tsx` and is shared by the first and last scenes.
