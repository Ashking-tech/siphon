#!/usr/bin/env python3
"""Generate a deterministic full-frame film-grain plate for the Remotion film.

The plate is full-frame rather than a tiled 512px texture: tiling produced
visible 512px seams through large flat typography at 1080p.
"""

from pathlib import Path

import numpy as np
from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "public" / "grain.png"

W, H = 1920, 1080
rng = np.random.default_rng(20261031)

# Multi-octave luminance noise, smooth enough to read as photochemical grain
# rather than digital static at 5% soft-light.
fine = rng.normal(0, 1, (H, W))
coarse = rng.normal(0, 1, (H // 2, W // 2))
coarse = np.repeat(np.repeat(coarse, 2, axis=0), 2, axis=1)
yy, xx = np.mgrid[0:H, 0:W]
cloud = (
    np.sin(xx / 83.0 + 1.2) * 0.22
    + np.sin(yy / 121.0 + 2.7) * 0.18
    + np.sin((xx + yy) / 197.0) * 0.12
)
field = fine * 0.68 + coarse * 0.32 + cloud
field = (field - field.mean()) / max(1e-6, field.std())
lum = np.clip(128 + field * 22, 0, 255).astype(np.uint8)
alpha = np.clip(172 + field * 18, 0, 255).astype(np.uint8)
rgba = np.dstack([lum, lum, lum, alpha])
Image.fromarray(rgba, "RGBA").save(OUT, optimize=True)
print(f"wrote {OUT} ({OUT.stat().st_size / 1024 / 1024:.1f} MiB)")
