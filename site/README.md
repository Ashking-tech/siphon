# Siphon site

The public site is static and has no build step, analytics, or external asset
requests. A small local script handles the persistent light/dark theme toggle.

## Preview

```sh
make site-serve
# http://127.0.0.1:8000/
```

Or without make:

```sh
python3 -m http.server 8000 --directory site
```

## Before publishing

The site is configured for `https://ashking-tech.github.io/siphon/`. The social
card source is `og.html`; regenerate `og.png` after changing the published URL.

## Releases

The download section pins the v0.2.0 GitHub release assets:

- `siphon_0.2.0_linux_amd64.tar.gz`
- `siphon_0.2.0_linux_arm64.tar.gz`
- `siphon_0.2.0_checksums.txt`

Update the version, archive names, and URLs in `index.html` and `llms.txt` when
a new release is tagged. Unversioned `latest` aliases do not exist yet, so each
release needs this small docs update.

## Files

| File | Purpose |
|---|---|
| `index.html` | the whole page |
| `style.css` | all styles; warm-paper light and dark themes |
| `theme.js` | light/dark toggle and saved theme preference |
| `favicon.svg` | cyan mark |
| `og.png` | 1200×630 social card |
| `og.html` | source used to render `og.png`; `noindex` |
| `media/` | product demo video, poster, and Dolphin ServiceMenu screenshot |
| `llms.txt` | compact product summary for language models |
| `robots.txt`, `sitemap.xml` | crawler metadata |
| `fonts/` | subset WOFF2 files and their licenses |

## Fonts

The page self-hosts subsets of:

- **Inter** and **Inter Display** — Copyright (c) 2016 The Inter Project
  Authors, SIL Open Font License 1.1. Full text in
  `fonts/LICENSE-Inter.txt`.
- **Hack** — Copyright (c) 2018 Source Foundry Authors, MIT License. The
  Bitstream Vera Sans Mono and DejaVu notices are included in the same file.
  Full text in `fonts/LICENSE-Hack.txt`.

The WOFF2 files were produced with `pyftsubset` and a Latin + punctuation +
arrows + symbols Unicode range. To regenerate, subset the upstream TTFs with:

```sh
pyftsubset Inter-Regular.ttf \
  --output-file=Inter-Regular.woff2 \
  --flavor=woff2 \
  --layout-features='*' \
  --unicodes='U+0000-00FF,U+0100-017F,U+2000-206F,U+20A0-20CF,U+2100-214F,U+2190-21FF,U+2200-22FF,U+25A0-25FF,U+2713-2714'
```

Repeat for `Inter-Medium`, `Inter-Bold`, `InterDisplay-Regular`,
`InterDisplay-Medium`, `InterDisplay-Bold`, `InterDisplay-Black`,
`Hack-Regular`, and `Hack-Bold`.

## Regenerate the social card

`og.html` is a 1200×630 fixed-size page. With Chrome installed:

```sh
cd site
google-chrome --headless --disable-gpu --no-sandbox \
  --allow-file-access-from-files --hide-scrollbars \
  --force-device-scale-factor=1 --window-size=1200,630 \
  --screenshot=og.png file://"$PWD"/og.html
```

Check the result at exactly 1200×630 before publishing.

## Content rules

These are intentional and should survive edits:

- **v0.2 pairs are shipped.** They are listed in the format catalog without
  `new` badges or in-progress language.
- **Downloads point only at the pinned v0.2.0 GitHub release assets.** Do not
  invent mirrors, Siphon package names, checksums, benchmarks, GUI screenshots,
  or license claims. The project license is MIT, copyright Ashking-tech.
- **The peak disk-use comparison is conceptual, not to scale.** It shows the
  default video relocate path: 2× for a normal converter versus source plus one
  segment for Siphon. The caption says so.
- **The 340 MB hero example is the spec's 60 GB calculation**, not a measured
  benchmark. `siphon inspect` reports the exact figure for a file.
- **The terminal card is an illustrative dry run.** Its numbers are plausible
  output, not a benchmark or a claim about a specific file.
- **Video relocate is remux-only.** The page does not claim transcoding,
  multi-stream support, or compatibility with every player.
- **Images, PDFs, and the v0.2 pairs are safe-only.** The page does not claim
  the video engine's 1× space saving for them.
- **Linux only, no first-class GUI yet.** The Dolphin ServiceMenu is the
  graphical surface today.

## Deploy

Upload the contents of `site/` to any static host (GitHub Pages, Netlify,
Cloudflare Pages, an object bucket, or a plain web server). The page has no
server-side requirements. `og.html` can be excluded from the upload; it is only
needed to regenerate the social card.
