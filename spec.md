# Siphon — Production Specification (Linux MVP)

**Version:** 1.7
**Status:** Frozen for v0.1 implementation
**Target OS:** Linux (KDE Plasma primary)
**Audience:** Implementing engineers, reviewers, and future maintainers
**Supersedes:** v1.5 (`spec.v1.4.md` retained for history)

---

## 1. Executive Summary

Siphon is a single-binary CLI and OS-integrated conversion tool that transforms media files **in place** — the source file is consumed and replaced by the output file of the same stem, in the same directory, with no network call, no hidden state outside the source directory, and no silent quality loss.

The product thesis is narrow and load-bearing:

> **Converting a large file should not require twice the disk space, a second copy on disk at the end, or any silent loss of quality.**

The target user is explicitly **someone with very little free space** — a nearly full laptop, a single-drive NAS, a phone with a 64 GB card. A second disk is not part of the product. Every mode in this document is sized against that user, not against a workstation with 500 GB spare.

Everything below derives from that thesis. When a design decision could go two ways, the tie-breaker is always: *which option makes the above statement most true, and which option makes failures safest?*

Where the thesis and crash-safety genuinely conflict, this specification **does not paper over the conflict**. It exposes it as explicit, named modes (§3.3) with different guarantees, and it refuses to run when no mode is provably safe.

### 1.1 The space promise, in one table

| Media | Default mode | Peak space | Free space you need |
|---|---|---|---|
| Audio (WAV→MP3) | `safe` | ≈ 1.15 × source | ~15% of the file |
| Video (MKV→MP4) | `relocate` | ≈ 1.00 × source **+ one segment** | **30–600 MB**, regardless of file size |
| Images | `safe` | source + output | ~1–8 × the (small) file |

Video free space depends on the segment window, not the file:

| Your MKV | Resolution / bitrate | Segment | Free needed | Total disk used |
|---|---|---|---|---|
| 4 GB | 1080p, 10 Mbps | 60 s | ≈ 75 MB | ≈ 4.1 GB |
| 20 GB | 1080p, 20 Mbps | 60 s | ≈ 150 MB | ≈ 20.2 GB |
| 60 GB | 4K, 45 Mbps | 60 s | ≈ 340 MB | ≈ 60.3 GB |
| 60 GB | 4K, 45 Mbps | `--segment-duration 15s` | ≈ 85 MB | ≈ 60.1 GB |
| 200 GB | 4K, 80 Mbps | 60 s | ≈ 600 MB | ≈ 200.6 GB |

Formula: `free = segment_duration × total_bitrate / 8 + 16 MB`. A 60 GB film converts with 340 MB free. That is the product.

### 1.2 What changed in v1.6 and v1.7, and why

v1.5 solved video at 2× space, which is a non-product for the target user. v1.6 replaced the video engine; v1.7 made it fast (§4.5.4.6).

**v1.6 — the engine:**

1. **Video default changed from `safe` (2×) to `relocate` (≈1× + one segment).** `RelocateEngine` processes the source **tail-first**, muxing one keyframe-aligned segment at a time, writing it into the output at its final offset, then truncating that segment's bytes off the end of the source. The source and the output trade bytes rather than duplicating them. See §4.5.4.
2. **`safe` mode retained for video as a compatibility escape hatch**, because it is the only path that produces a *classic* (non-fragmented) MP4.
3. **Video `reclaim` (fragmented overwrite-behind) deleted.** Its margin is under 1% when output ≈ input, so it would frequently spill to 2× *after* already damaging the source. Relocate has no such failure mode. Recorded in Appendix A.
4. **`--staging` considered and rejected.** It solves the space problem only for users who have a second disk, which is precisely the user this product is not for. No code, no flag.
5. **A pre-flight stitch probe was added** (§4.9 check 19): the tail segment is encoded and verified *before* anything is truncated, so a stitching incompatibility is discovered with the source untouched instead of halfway through a film.

**v1.7 — the two performance fixes:**

6. **Planning reads the container index, not the whole file.** v1.6 enumerated every packet with `ffprobe -show_packets` — a full extra read pass, 33% of total I/O, to learn something MKV's `Cues` element already stores. v1.7 parses `Cues` (a few MB at the end of the file) and falls back to the scan only when the index is missing or truncated.
7. **Verification defaults to sampled decode, not full decode.** v1.6's default decoded every frame of every segment: ~10 min for a 1080p film and **30–60 min for 4K**, ten times the cost of the copy itself. v1.7's default decodes three frames per fragment and parses the sample tables, which catches every systematic stitching fault, since such a fault appears on every fragment. Full decode remains available as `--verify full`.

With both changes, `relocate` performs exactly two file-sized I/Os — the same as `safe` mode. **The space saving costs no time.**

Carried forward from v1.5: the SlidingEngine replacement for the impossible "ReverseEngine", the honest per-mode recoverability table, the journal schema, the verification gate, and metadata/xattr transfer.

---

## 2. Design Principles

**P1 — The source is sacred until the output is proven.**
No byte of the source is deleted, truncated, or overwritten until the corresponding byte of the output has been written, `fsync`ed, verified, and journalled. "Proven" means passed the verification gate (§4.7), not merely `fsync`ed.

**P2 — Refusal is a first-class outcome.**
Every operation has a pre-flight gate. If the gate cannot prove that the conversion will complete safely, the tool refuses and exits non-zero without touching the file (§5.2).

**P3 — No silent quality loss, no silent space cost.**
Lossy decisions require an explicit flag or UI confirmation. Space cost is stated before work begins, in absolute megabytes, not as a multiplier the user has to compute.

**P4 — Same-directory output, atomic finalisation.**
Temps live in the source directory, and finalisation is a single `renameat2(2)` call. Cross-filesystem placement is refused.

**P5 — Every destructive operation is journalled, and its recoverability is stated honestly.**
A crash leaves a state from which `siphon recover` can either (a) complete the job, or (b) deterministically salvage a valid partial result. Which applies is a function of engine and mode, documented per-path in §4.6.5. Siphon never claims resumability it does not have.

**P6 — No network, no telemetry, no hidden state.**
All state lives in the source directory and is removed on success or clean refusal.

**P7 — The user's file is never the test subject.**
Every path is covered by automated round-trip tests, including crash injection and segment stitching (§7).

**P8 — Leverage existing, battle-tested tools (The Orchestrator Philosophy).**
Siphon does not write media encoders, decoders, or muxers from scratch. It orchestrates `ffmpeg`, `lame`, `cwebp`, and `dwebp` as subprocesses. The engineering focus is exclusively what makes Siphon unique: **atomic, crash-safe, low-space in-place file manipulation via syscalls.**

**P9 — Destructive is a flag, not a default — except where the product is the destruction.**
`relocate` is the default for video because a 2× default would refuse to run on the disks this product exists for. It destroys source bytes continuously, so it must compensate with stronger guarantees: per-segment verification *before* truncation, a pre-flight stitch probe, and exact resumability. Where no such compensation is possible (`reclaim`), the mode stays opt-in.

---

## 3. Scope & Supported Formats (v0.1)

### 3.1 Format Pairs, Modes, and Space Guarantees

| Source | Target | Default mode | Other mode | Peak space | Output kind |
|---|---|---|---|---|---|
| `.wav` | `.mp3` | `safe` | `reclaim` | safe ≈1.15 ×; reclaim ≈1.00 × | MP3 |
| `.mkv` | `.mp4` | **`relocate`** | `safe` | relocate ≈1.00 × + one segment; safe ≈2 × | relocate: fragmented MP4 · safe: classic MP4 |
| `.png` | `.jpg` | `safe` | — | source + output (≈1–2 ×) | JPEG |
| `.jpg` | `.png` | `safe` | — | source + output (**≈3–8 ×**, lossless inflates) | PNG |
| `.png` | `.webp` | `safe` | — | source + output (≈1–2 ×, lossless) | WebP |
| `.webp` | `.png` | `safe` | — | source + output (**≈2–8 ×**, lossless inflates) | PNG |
| `.jpg` | `.webp` | `safe` | — | source + output (≈1–2 ×) | WebP |
| `.webp` | `.jpg` | `safe` | — | source + output (≈1–2 ×) | JPEG |

"Peak space" is the maximum of `(blocks used by the source) + (blocks used by the output)` at any instant during the run.

- `relocate` rows are enforced by the reverse-cursor invariant of §4.4.2, not estimated.
- `reclaim` rows are enforced by the forward-cursor invariant of §4.4.1.
- `safe` rows are `source + output` — the image rows state both terms because a lossless transcode can produce output several times larger than the source.

Images are `safe`-only: they are small, and a 1× in-place image write would trade a real safety property for a rounding error.

### 3.2 Explicit Non-Goals (v0.1)

- Video transcoding (VP9→H.264, etc.). MKV→MP4 is **remux only**.
- Output that is a *classic* MP4 in the low-space path. Classic MP4 requires `safe` (2×).
- Audio transcoding to anything other than AAC (via `--transcode-audio`).
- Subtitle transcoding. Unsupported subtitle codecs are dropped via `--drop-incompatible` or refused.
- FLAC→MP3 (v0.2).
- Multi-channel (>2) WAV input.
- A second disk, network staging, or any out-of-tree temp. Users do not have one.
- Windows and macOS.
- Network sources, URLs, or stdin/stdout as the primary path.

### 3.3 Modes

**`--mode auto` (default).** Resolves per pair to the default in §3.1.

**`--mode safe`.** Output is built in a same-directory temp. The source is read-only until the output has passed verification and been `fsync`ed and renamed. A crash at any instant leaves the source byte-identical. Peak `source + output`. For video this is the compatibility path: it is the only mode that produces a classic MP4.

**`--mode relocate`** (video only, default). Output is written into a sparse same-directory temp at its final offsets, working backwards through the source one segment at a time; each segment's source bytes are truncated away after that segment is verified and durable. The source and output occupy the *same* blocks at different times. Peak `source + one segment`. See §4.5.4. Crash outcome is exact-resume (§4.6.5).

**`--mode reclaim`** (audio only, opt-in). Output is written into the source's own head, behind the read cursor. Peak `source`. Crash outcome is a splice-resume. See §4.4.1, §4.5.3.

Requesting a mode a pair does not support is `E_MODE_UNSUPPORTED`, with the supported list in the message.

### 3.4 Feasibility Gates

**`relocate` gate (video).** Refuses with `E_INPLACE_INFEASIBLE` unless all of:

1. The plan is remux-only (`-c copy` for every stream). Any transcode would change output size unpredictably, breaking the reverse cursor.
2. `estimated_output ≤ 1.03 × source`. Remux normally lands at 0.95–1.02×; audio transcoding to AAC can exceed it, so `--transcode-audio` is refused in `relocate` mode and the message names `--mode safe`.
3. At least 3 keyframe-aligned segment boundaries exist.
4. Every retained video stream has a keyframe within `--segment-tolerance` (default 0.5 s) of every planned boundary.
5. Track configuration (codec, extradata, time base, channel layout) is constant across the file, verified by comparing a head probe, a mid probe, and a tail probe (§4.9 check 13).

**`reclaim` gate (audio).** Refuses unless `R = estimated_output / source ≤ 0.90`. WAV PCM is ≥4× larger than the MP3 in realistic cases; an 8 kHz mono 16-bit WAV (128 kbps) against the ~190 kbps VBR ceiling is correctly refused.

---

## 4. Technology Stack & Architecture

### 4.1 Language: Go

Chosen for fast compilation, static binaries (`CGO_ENABLED=0`), direct syscall access via `golang.org/x/sys/unix`, and `os/exec`. All media work is subprocess-based, so `CGO_ENABLED=0` costs nothing.

### 4.2 Dependency & Library Strategy

**External binaries (subprocesses, version-gated at pre-flight):**

| Binary | Used for | Minimum | Required when |
|---|---|---|---|
| `ffmpeg` | MKV demux, MP4 mux, WAV decode, segment muxing | ≥ 5.0 | always |
| `ffprobe` | Track/packet/keyframe inspection, verification | ≥ 5.0 | always |
| `lame` | WAV PCM → MP3 | ≥ 3.100 | `.wav` sources |
| `cwebp` | WebP encode (`-metadata all`) | ≥ 1.0 (libwebp-tools) | `.webp` targets |
| `dwebp` | WebP decode (`-metadata`) | ≥ 1.0 (libwebp-tools) | `.webp` sources |

Missing mandatory binary → `E_DEPENDENCY`; present but below minimum → `E_DEPENDENCY_OUTDATED`. Optional binaries are required only by the pairs that use them, so a user without `libwebp-tools` can still convert PNG→JPEG.

**Go libraries:**

| Package | Purpose | Note |
|---|---|---|
| `spf13/cobra` | CLI parsing | |
| `golang.org/x/sys/unix` | `Flock`, `Renameat2`, `Fsync`, `Ftruncate`, xattrs | |
| `golang.org/x/image/webp` | **Decode only** | no encoder exists; encoding goes through `cwebp` |
| `disintegration/imaging` | PNG/JPEG decode+encode, resampling, orientation | **no WebP support** |
| `rwcarlsen/goexif` | **Read-only** EXIF inspection | cannot write EXIF |
| `schollz/progressbar/v3` | Terminal progress | behind `ProgressSink` |

**Metadata strategy:**

- **ICC profiles**: byte-level transplant of JPEG APP2 (`ICC_PROFILE`) segments and PNG `iCCP` chunks. Well-specified, ~200 lines, no dependency.
- **EXIF across WebP edges**: `cwebp -metadata all` / `dwebp -metadata all`, which copy EXIF and ICC as opaque blobs.
- **EXIF across JPEG↔PNG**: APP1 transplant where the target supports it; `Orientation` applied at decode and `DateTimeOriginal` translated where it does not.
- If a metadata class cannot be carried: warn and refuse with `E_METADATA_DROP` unless `--drop-metadata`. Never silent.

> **Explicitly rejected:** `imaging` for `.png → .webp` (no encoder); assuming EXIF survives re-encoding (it does not).

### 4.3 High-Level Architecture

```text
┌──────────────────────────────────────────────────────────────┐
│  Interface Layer                                             │
│  ┌──────────────┐   ┌────────────────────────────────────┐   │
│  │ CLI (cobra)  │   │ KDE ServiceMenu (siphon*.desktop)  │   │
│  └──────┬───────┘   └───────────────┬────────────────────┘   │
└─────────┼───────────────────────────┼────────────────────────┘
          │                           │
          ▼                           ▼
┌──────────────────────────────────────────────────────────────┐
│  Orchestrator                                                │
│  - flags → Job; mode resolution per pair                     │
│  - preflight() → Plan | Refusal   (§4.9)                     │
│  - segment planner (§4.5.4.1)                                │
│  - owns Journal, ProgressSink, signal ctx, fault hook        │
└─────────┬────────────────────────────────────────────────────┘
          │
          ▼
┌──────────────────────────────────────────────────────────────┐
│  Engine Layer                                                │
│  ┌───────────────┐ ┌───────────────┐ ┌────────────────────┐  │
│  │ AtomicCopy    │ │ Sliding       │ │ Relocate           │  │
│  │ (images)      │ │ (audio)       │ │ (video, default)   │  │
│  └───────┬───────┘ └───────┬───────┘ └─────────┬──────────┘  │
│          │                 │                   │             │
│          │                 │        ┌──────────▼──────────┐  │
│          │                 │        │ Remux worker        │  │
│          │                 │        │ (per-segment ffmpeg)│  │
│          │                 │        └─────────────────────┘  │
└──────────┼─────────────────┼───────────────────┼─────────────┘
           │                 │                   │
           ▼                 ▼                   ▼
┌──────────────────────────────────────────────────────────────┐
│  Platform Layer (Linux)                                      │
│  - fs: same-dir temp (O_EXCL|O_NOFOLLOW), fsync, renameat2,  │
│        dir-fsync, ftruncate, sparse reservation              │
│  - meta: xattr/ACL/SELinux label + mode/uid/gid transfer     │
│  - lock: Flock(LOCK_EX|LOCK_NB) on source and journal        │
│  - probe: statfs, same-device, FS capabilities               │
└──────────────────────────────────────────────────────────────┘
```

`ProgressSink` is an interface with terminal, `kdialog`, and `noop` implementations, so engine code never prints directly.

### 4.4 Cursor Invariants (core of the product)

Two invariants cover all in-place engines. Every write in an in-place path is governed by one of them.

#### 4.4.1 Forward (audio `reclaim`)

`read_cursor` is the highest source offset handed to the encoder; `write_cursor` is the highest offset overwritten with output. Both start at 0 and never decrease.

```
write_cursor + len(block) + MARGIN ≤ read_cursor
MARGIN = max(1 MiB, 4 × max_encoder_frame_bytes, 2 × journal_checkpoint_span)
```

The region `[write_cursor, read_cursor)` is the **shadow window**: the only source bytes ever destroyed, always *after* their content has been consumed.

Per-chunk protocol:

```
1. n = pread(source, read_cursor, CHUNK)
2. read_cursor += n
3. feed chunk to encoder stdin; collect encoder stdout
4. assert invariant for the bytes about to be written
5. pwrite(source, out_bytes, write_cursor)
6. write_cursor += len(out_bytes)
7. every CHECKPOINT_SPAN (8 MiB) or 1 s:
     fdatasync(source); journal.checkpoint(read_cursor, write_cursor)
8. on EOF: flush encoder, assert invariant, fdatasync, journal.complete()
9. ftruncate(source, write_cursor)
10. fdatasync; journal.unlink(); dir_fsync(dir)
```

`pread`/`pwrite` are mandatory: a shared file offset between the read and write halves of one descriptor is the easiest way to corrupt this algorithm.

#### 4.4.2 Reverse (video `relocate`)

`tail_cursor` is the source offset **below which** all bytes have been processed and truncated. `out_cursor` is the offset **above which** all output bytes have been written. Both start at their maxima and move down.

```
tail_cursor  = source_size
out_cursor   = reserve_size
Invariant:  out_cursor - len(fragment) ≥ 0                        (guaranteed by the reserve rule, §4.5.4.1 step 6)
            source_size_at_step_i = tail_cursor                   (§4.5.4.3)
            peak_blocks ≈ source_size + one_segment
```

The output file is **sparse**: `reserve_size` is reserved logical size, not allocated blocks. Only bytes actually written consume space, which is why the peak is `source + one segment` rather than `source + reserve`.

Because the write comes before the truncate, one segment's worth of bytes exists twice at the moment of handover. That is the entire space cost of the mode, and it is what `--segment-duration` tunes.

#### 4.4.3 Why not `FALLOC_FL_PUNCH_HOLE`

Punching frees blocks but leaves file size unchanged, cannot make a non-resumable mux resumable, and does not distinguish "reclaimed because consumed" from "unresumable". Truncation gives the same space back with clearer semantics and a valid file prefix. Not used in v0.1. See Appendix A.

### 4.5 Engine Specifications

#### 4.5.1 AtomicCopyEngine (images)

1. Decode source (`imaging` with `AutoOrientation(true)` for PNG/JPEG; `dwebp -metadata all` for WebP).
2. If the target cannot carry alpha (`.jpg`) and the image has alpha: require `--bg <color>` and composite. Refuse with `E_ALPHA_UNSUPPORTED` otherwise. `imaging`'s JPEG encoder flattens onto black by default — the composite is explicit, never implicit.
3. Encode to a same-directory temp (`imaging`, or `cwebp -metadata all -q <q>` fed a short-lived PNG intermediate).
4. Transplant ICC/EXIF per §4.2.
5. `fsync` → verify (§4.7) → transfer mode/uid/gid/xattrs (§6.5) → `renameat2(RENAME_NOREPLACE)` → `unlink` source remnant → `dir_fsync`.
6. No journal is required (the source is never modified); a crash can only leave a stray temp, which the next run reaps.

#### 4.5.2 SlidingEngine (WAV → MP3)

- Validates `RIFF....WAVE` (§6.7), channels ≤ 2, sample format, `R ≤ 0.90` (§3.4).
- `lame --silent -V 2 - -` (or `-b <kbps>`) under `exec.CommandContext` with stdin/stdout pipes.
- `safe`: identical pipeline, encoder stdout goes to a temp; source untouched until finalisation. Same code with a `dest` abstraction (`pwrite` sink vs temp sink).
- `reclaim`: implements §4.4.1. Resume is splice-resume (§4.6.5).

#### 4.5.3 RemuxEngine (MKV → MP4, `safe` mode)

**Track mapping.** Pre-flight builds an explicit `-map` list from `ffprobe -show_streams -show_format -of json`. Compatibility matrix (v0.1):

| Stream | MP4-copyable | Action |
|---|---|---|
| H.264 (AVC), HEVC (HVC1/HEV1), AV1, MPEG-4 Part 2 | yes | copy |
| VP8/VP9 | muxable, poorly supported | refuse unless `--allow-nonstandard` |
| AAC, ALAC, AC-3, E-AC-3 | yes (AC-3/E-AC-3 need `-strict -2`) | copy |
| FLAC, Opus | allowed but poorly supported | copy only with `--allow-nonstandard` |
| DTS, TrueHD, PCM | no | `--transcode-audio` (→ AAC) or `--drop-incompatible`, else refuse |
| SubRip/ASS/SSA, PGS, VobSub | no | `--drop-incompatible`, else refuse |
| Attachments (fonts), chapters | no | dropped; reported |
| Global/track tags | partial | `-map_metadata 0`; unmatched tags dropped and reported |

```bash
ffmpeg -i src.mkv -map ... -c copy -movflags +faststart tmp.mp4
```

Verify → metadata transfer → `renameat2` → unlink source. This is the **only** path that produces a classic, non-fragmented MP4, which is why it remains available.

#### 4.5.4 RelocateEngine (MKV → MP4, default)

##### 4.5.4.1 Planning (metadata only, no data written)

1. `ffprobe -show_streams -show_format` → bitrates, duration, track config sample A.
2. Probe track config at 3 points (head, mid, tail) via `-read_intervals`; must match (§4.9 check 13).
3. **Boundary source, in order of preference:**
   - **`Cues` (normal case).** Seek the `SeekHead` → `Cues` element near the end of the file and parse it directly (EBML, ~400 lines). It yields every keyframe's timestamp and cluster byte offset in a few MB — no scan of the media. Record `plan_source: "cues"` in the journal.
   - **`ffprobe -show_packets` (fallback).** Only when `Cues` is absent (streamed or truncated MKV) or fails to cover ≥90% of the file. This is a full read pass (~170 k packets, ~20 MB of JSON, parsed streaming) and is recorded as `plan_source: "scan"`, which `inspect` reports because it changes the expected runtime by 50%.
   
   Both paths produce the same structure: per video stream, keyframe `(pts, cluster_pos)`; per boundary, a byte range.
4. Choose boundaries: keyframe times spaced ≥ `--segment-duration` (default `60s`), snapped to a keyframe on **every** retained video stream within `--segment-tolerance`. Refuse if any boundary cannot be snapped. Each boundary is then converted to a **byte range** `[src_off_i, src_off_{i+1})` starting at the EBML `Cluster` element that contains that keyframe (§4.5.4.3), and the expected packet counts for the range are taken from step 3.
5. Cache the **EBML header** — offset 0 through the first `Cluster`, typically well under 2 MiB — and rewrite its `Segment` size field to "unknown" (`0x01FFFFFFFFFFFFFF`). This cached header plus one byte range is the synthetic input for every segment (§4.5.4.3).
6. Compute `reserve_size = ceil(max(estimated_output, source_size) × 1.05) + 16 MiB`; create the temp and `ftruncate` it to `reserve_size` (sparse — allocates no blocks). The 5% + 16 MiB over the largest permitted output means the reverse cursor can never underflow (§4.4.2), and the residual gap at the front is always ≥ 16 MiB, so the `free` box of §4.5.4.3 step 11 is always well formed. Reserving generously costs nothing because the file is sparse.
7. Journal `prepared` with the full boundary table (byte ranges, expected packet counts).

##### 4.5.4.2 Pre-flight stitch probe

Before any truncation, encode and fully verify a **sample of segments into memory**: always the last segment (the first one the engine will process) and the first, plus `--probe-segments N` (default 4 for films over 20 segments, otherwise all) spread evenly through the file. If any fails to produce a decodable fragment with matching packet counts, refuse with `E_STITCH_FAILED` and the source untouched. Cost: 4 segments of work, ~1–2 minutes. This is what converts "halfway through a film" from a plausible failure into a rare one.

##### 4.5.4.3 Per-segment protocol (tail-first)

For `i = N-1` down to `0`:

```
1. Write the cached header to ffmpeg stdin, then stream pread(source, src_off_i,
   src_end_i - src_off_i) into it:
       ffmpeg -nostdin -f matroska -i pipe:0 -map ... -c copy \
              -copyts -muxdelay 0 -avoid_negative_ts disabled \
              -f mp4 -movflags +empty_moov+default_base_moof+negative_cts_offsets \
              -frag_duration 1000000 pipe:1
2. Validate the fragment's first sample PTS against plan.first_pts_i (±1 frame),
   and that src_off_i is a Cluster start. A mismatch is E_SEGMENT_PLAN and
   nothing is truncated.
3. For i == N-1 keep ftyp+moov as the candidate init segment.
   For i < N-1 drop ftyp+moov; keep moof+mdat only.
4. Validate the fragment: track IDs and decoder config match the init; every
   tfdt non-decreasing versus the previous fragment; rewrite mfhd sequence_number
   to a monotonic counter of our own.
5. Verify (§4.7, per-segment gate) at the configured level; default
   `sampled` decodes 3 frames per fragment, not the whole fragment.
6. pwrite(temp, fragment_bytes, out_cursor - len); out_cursor -= len
7. fdatasync(temp); journal.checkpoint(segment i complete)
8. ftruncate(source, src_off_i); fdatasync(source)
9. journal.checkpoint(tail_cursor = src_off_i, source_mtime_ns = fstat.mtime_ns)
```

**No seeking, by construction.** Truncating the tail destroys the MKV's `Cues`/`SeekHead` on the very first segment, after which a time-based `-ss` seek would degrade into a linear scan from the start of the file — for 118 segments on a 20 GB film, roughly 2.3 TB of reads. Feeding an exact byte range behind a cached header removes seeking from the design entirely: every segment is read exactly once, in place, and the total I/O is one pass over the film. Cluster-level parsing is a byte scan for the EBML Cluster ID (`1F 43 B6 75`), not a muxer.

After the loop:

```
10. pwrite(temp, init, out_cursor - len); out_cursor -= len
11. if out_cursor > 0: pwrite(temp, free_box(out_cursor), 0)   // legal skip box over the sparse hole
12. fdatasync(temp)
13. full verification (§4.7) on the assembled file
14. transfer metadata (§6.5); renameat2(temp → target)
15. ftruncate(source, 0); unlink(source); dir_fsync(dir); journal.unlink()
```

Step 11 is the trick that removes the final compaction pass. The reserved region is written from the top down, so whatever is left over at the front is a hole; an MP4 `free` box of that exact size (an 8-byte header plus the hole) is legal and skipped by every parser. No data is ever moved, so the final assembly costs no space and no extra full-file I/O.

##### 4.5.4.4 Output characteristics

- The result is a **fragmented** MP4 (`ftyp` + `moov` + `free?` + repeated `moof`/`mdat`). Plays in VLC, mpv, browsers, iOS/Android, and modern TVs. Some hardware players and a few NLEs will not open it.
- One initialisation segment, `moof` per ~1 s, segment boundaries every `--segment-duration`. Overhead ≈0.1–0.5%.
- Seeking works (that is the point of `tfdt`); `faststart` is irrelevant because `moov` is already at the front.
- Users who need a classic MP4 are told, in the error for any incompatibility and in the man page: `--mode safe`.

##### 4.5.4.5 Failure handling

A segment that fails validation at step 4 or 5 has **not** caused any truncation of its own bytes yet. The engine stops, leaves the journal, and exits 3. The source still holds `[0, src_off_i)` and the temp holds verified fragments for `[src_off_i, end)` — nothing is lost.

`recover` then **re-plans the remaining range** `[0, tail_cursor)` and retries. This is safe because output offsets are assigned at write time from the running `out_cursor`, never precomputed: a different segmentation only moves the fragments that have not been written yet, and every fragment already on disk keeps its offset and stays valid. (The journal therefore records `out_off` only for segments in state `done`.)

The one unrecoverable case is a segment that fails *after* its own truncation, which the protocol makes impossible by ordering truncation last.

##### 4.5.4.6 Cost model

`relocate` performs **two file-sized I/Os**: one read of the source (each byte exactly once, §4.5.4.3) and one write of the output. Since output ≈ input, that is the I/O of `cp` — and *identical* to `safe` mode, which also reads once and writes once. **The space saving is free.**

The planning pass is not a pass when `Cues` is present: it reads a few MB from the end of the file. The `scan` fallback adds a third full read (+50%), which is why `inspect` reports `plan_source`.

Expected wall clock, default `sampled` verification:

| File | Disk (~read/write throughput) | I/O moved | Expected total | `cp` for comparison |
|---|---|---|---|---|
| 4 GB 1080p | HDD, 150 MB/s | 8 GB | ≈ 1 min | ≈ 55 s |
| 20 GB 1080p | HDD, 150 MB/s | 40 GB | ≈ 5 min | ≈ 4.5 min |
| 20 GB 1080p | SATA SSD, 500 MB/s | 40 GB | ≈ 1.5 min | ≈ 1.5 min |
| 60 GB 4K | NVMe, 2 GB/s | 120 GB | ≈ 1.5 min | ≈ 1 min |
| 60 GB 4K | HDD, 150 MB/s | 120 GB | ≈ 14 min | ≈ 13 min |

Fixed overheads, independent of file size: ~0.1–0.2 s per ffmpeg spawn (118 spawns for a 2-hour film ≈ 15 s), one `fdatasync` + one `truncate` per segment (≈2 s total on an HDD), and journal rewrites (a few KB each).

**Seek behaviour on spinning disks.** Because output ≈ input, the read and write positions stay within a few percent of each other for the whole run, so the head does not thrash between opposite ends of the file. Within each segment both streams are sequential. An HDD therefore gets near-sequential throughput, not the random-access throughput a naive "two cursors far apart" design would suffer.

**What makes it slow**, in order: `--verify full` (decode-bound, not I/O-bound — catastrophic on 4K), the `scan` planning fallback, and small `--segment-duration` values (more spawns, more fsyncs, more fragmentation). Nothing else in the design is a throughput concern.

### 4.6 Journal

#### 4.6.1 Location and creation

One file per job, same directory, hidden, mode `0600`, `O_RDWR|O_CREAT|O_NOFOLLOW`:

```
.siphon-<stem>-<8 hex random>.journal
```

#### 4.6.2 Format

UTF-8 JSON, atomically replaced at each checkpoint (write `.journal.tmp` → `fsync` → `rename` → `dir_fsync`). Records carry a monotonic `seq` and CRC32C; a torn or stale-seeded journal is rejected whole, never partially trusted.

```json
{
  "schema": 2,
  "seq": 42,
  "crc32c": "9f2c1e04",
  "job": {
    "engine": "relocate",
    "mode": "relocate",
    "source": {"path": "film.mkv", "dev": 2049, "ino": 173421, "size": 21474836480,
               "mtime_ns": 1729780000000000000, "mode": 420, "uid": 1000, "gid": 1000},
    "target": {"path": "film.mp4", "format": "mp4"},
    "params": {"segment_duration_ns": 60000000000, "segment_tolerance_ns": 500000000,
               "plan_source": "cues", "verify": "sampled",
               "drop_incompatible": false, "transcode_audio": false},
    "tools": {"ffmpeg": "6.1.1", "ffprobe": "6.1.1"},
    "reserve_size": 22565339136
  },
  "segments": [
    {"i": 41, "src_off": 20971520000, "src_end": 21474836480,
     "out_off": 20900000000, "out_len": 573000000, "packets": 14400, "state": "done"},
    {"i": 40, "src_off": 20447232000, "src_end": 20971520000,
     "out_off": 20330000000, "out_len": 568000000, "packets": 14380, "state": "done"},
    {"i": 39, "src_off": 19922944000, "src_end": 20447232000,
     "out_off": null, "out_len": null, "packets": 14395, "state": "pending"}
  ],
  "cursors": {"tail_cursor": 20447232000, "out_cursor": 20330000000,
              "source_mtime_ns": 1729780009000000000},
  "init": {"out_off": 20260000000, "len": 70214, "sha256": "…"},
  "phase": "encoding",
  "created_ns": 1789780000000000000,
  "updated_ns": 1789780009000000000
}
```

#### 4.6.3 Rules

- `dev`/`ino` are re-verified on `recover`. `size` and `mtime` are **mode-dependent, because destructive modes change them**: `safe` requires the original `size` and `mtime`; `relocate` requires `size == cursors.tail_cursor` and `mtime == cursors.source_mtime_ns`; `reclaim` requires the original `size` until its final `ftruncate`. A mismatch is `E_SOURCE_CHANGED` and no recovery is attempted. (v1.5 checked `size` unconditionally, which would have made every `relocate` recovery fail by construction.)
- A journal present at pre-flight is `E_STALE_JOURNAL`; `recover` is the only entry point that consumes one.
- `flock(LOCK_EX|LOCK_NB)` on the journal serialises Siphon instances; the source flock is best-effort (§6.4).
- `phase` moves `prepared → encoding → verifying → finalising → complete`. Completed journals are deleted, never left behind.
- Only **`fdatasync`ed** cursors are recorded. Page-cache state after `SIGKILL` is not evidence.

#### 4.6.4 Why the journal is per-segment, not per-byte

`relocate` resumes at a segment boundary, not an arbitrary offset: a fragment is the smallest unit that is independently meaningful. Recording every segment's final offset before it is written is what makes resume exact and idempotent — a re-run rewrites a segment at the same offset, or skips it entirely if `state == done`.

#### 4.6.5 Recoverability, stated per path

| Path | Crash outcome | `siphon recover` |
|---|---|---|
| AtomicCopy | source intact, temp partial | delete temp; source untouched |
| Sliding (safe) | source intact, temp partial | delete temp; re-run from scratch |
| Sliding (reclaim) | source head is a valid MP3 prefix; tail intact | **splice-resume**: encode from `read_cursor` into `write_cursor`; one MP3 frame boundary with a ≈26 ms discontinuity, reported |
| Remux (safe) | source intact, temp partial | delete temp; re-run from scratch |
| **Relocate** | source is a valid **truncated MKV** holding `[0, tail_cursor)`; temp holds verified fragments for `[tail_cursor, end)`, each at its final offset | **exact resume**: continue at the next segment down; no data re-encoded, no seam introduced, nothing lost |

Relocate and Sliding(reclaim) are the only destructive paths. Relocate's crash state is strictly better than reclaim's: the source is a real, playable file (MKV without its Cues, so seeking is slow in some players but playback works), and the union of the two files is the complete media.

Resume correctness depends on the journal recording only `fdatasync`ed state (§4.6.3).

### 4.7 Verification Gate

Runs before any source mutation in `safe` mode; before each truncation in `relocate` mode; before final `ftruncate` in `reclaim` mode.

**Per-segment gate (`relocate`, mandatory, before truncation).**

Three levels, selected by `--verify`:

| Level | What it does | Cost per film | Use |
|---|---|---|---|
| `structural` | Steps 1–4 only | ≈0 | CI, `--skip-probe` runs |
| **`sampled`** (default) | Steps 1–5 | ≈0 | normal runs |
| `full` | Steps 1–4, then a complete decode of the fragment | +1 decode pass: ~10 min per 1080p film, 30–60 min per 4K film | paranoid, one-off archival |

1. **Structure:** every box length parses and is consistent; `mdat` payload equals its declared size; the sum of `trun` sample sizes equals the `mdat` payload.
2. **Timeline:** `tfdt` base media decode times are non-decreasing versus the previously written fragment; `mfhd` sequence numbers are monotonic (rewritten by us, so this is a self-check).
3. **Identity:** track IDs, codec, extradata, and timescale match the init segment.
4. **Coverage:** the fragment's declared duration matches `plan.duration_i` within one frame, and its sample count matches the plan's count for the byte range. When the plan came from `Cues` (no packet counts available), the sample count is instead checked against `duration × frame_rate` within ±1%, and exact counts are reconciled on the full gate.
5. **Sampled decode:** for each fragment, decode `--verify-frames` (default 3) frames — first, middle, last — by random access into the fragment bytes, not by decoding it end to end:

   ```
   ffmpeg -v error -xerror -f mp4 -i <init‖fragment> -ss <t> -frames:v 1 -f null -
   ```

   A systematic stitching fault (bad `tfdt`, missing sample, wrong decoder config, chopped GOP) corrupts *every* fragment, so three frames per fragment is as diagnostic as a full decode. A fault that only appears between the sampled frames would have to be a random medium error, which the filesystem's own checksums own.

Any failure → stop before truncation, exit 3 (§4.5.4.5).

**Full gate (all modes, on the assembled candidate):**

1. `ffprobe -v error -of json`: stream count and codecs match the plan; duration within 1% or 2 frames.
2. `ffmpeg -v error -xerror -i candidate -f null -`: no errors.
3. Remux only: per-stream packet counts match the source exactly (reconciling any ±1% tolerance accepted in per-segment step 4).
4. Sanity: size > 0, and not absurdly small (< 1 KiB per second of media).

**Why `sampled` is the default rather than `full`:** the failure modes this gate exists to catch — a boundary in the wrong GOP, a lost sample, a mismatched decoder config, a non-monotonic timeline — are all *systematic* and therefore present in every fragment. Full decode buys protection only against spontaneous bit rot between the sampled frames, at ten times the runtime, on a file that was just written from a local read. It is offered, documented as the stronger option, and not imposed.

### 4.8 Signal and Subprocess Rules

Covered in §6.1 and §6.9; stated here because both destructive engines depend on them: every subprocess shares the signal context, and no truncation, `rename`, or `unlink` may occur after the context is cancelled.

### 4.9 Pre-Flight Gate

Runs in order, before any write. On refusal: structured error, exit 2, **no files touched**.

| # | Check | Refusal code |
|---|---|---|
| 1 | Source exists, is regular, not a symlink (`O_NOFOLLOW` at open, re-checked) | `E_SOURCE_INVALID` |
| 2 | Format pair supported; `--to` ≠ source format; target stem differs from source stem | `E_FORMAT_UNSUPPORTED` |
| 3 | Requested mode is available for the pair | `E_MODE_UNSUPPORTED` |
| 4 | `st_nlink == 1`, or `--allow-hardlinks` | `E_MULTILINK` |
| 5 | Directory and source writable by the effective user | `E_PERMISSION` |
| 6 | Target does not exist, or `--force` | `E_TARGET_EXISTS` |
| 7 | Source and target on the same device | `E_CROSS_DEVICE` |
| 8 | Codec/track compatibility via `ffprobe` (§4.5.3) | `E_CODEC_INCOMPATIBLE` |
| 9 | Metadata classes carriable, or `--drop-metadata` | `E_METADATA_DROP` |
| 10 | Free space ≥ the §7.1 estimate for the chosen mode | `E_NO_SPACE` |
| 11 | Reclaim ratio `R ≤ 0.90` (audio `reclaim` only) | `E_INPLACE_INFEASIBLE` |
| 12 | Sparse-file support in the target directory: probe by `ftruncate`ing the temp to `reserve_size` and requiring `st_blocks × 512 < reserve_size / 10` (`relocate` only) | `E_FS_NO_SPARSE` |
| 13 | Relocate gate (§3.4): remux-only, output ratio, ≥3 boundaries, keyframe coverage, **constant track config** | `E_INPLACE_INFEASIBLE` |
| 14 | No stale journal for this pair | `E_STALE_JOURNAL` |
| 15 | Magic bytes match claimed extension (§6.7) | `E_SOURCE_CORRUPT` |
| 16 | No immutable attribute, not on a read-only mount | `E_PERMISSION` |
| 17 | Dependencies present and ≥ minimum version (§4.2, §6.8) | `E_DEPENDENCY` / `E_DEPENDENCY_OUTDATED` |
| 18 | Not UID 0 without `--allow-root` | `E_ROOT_DENIED` |
| 19 | **Stitch probe** passes for the sampled segments (`relocate` only, §4.5.4.2) | `E_STITCH_FAILED` |
| 20 | Mode's crash cost acknowledged (`--yes`) when stdin is not a TTY and mode is `reclaim` | `E_CONFIRMATION_REQUIRED` |

Check 19 is the expensive one and is ordered last among the probes so that cheap refusals happen first. It is skipped by `--skip-probe`, documented as "trust me".

---

## 5. Behavior Specification

### 5.1 CLI Surface

```bash
siphon convert <source> --to <fmt> [flags]
siphon inspect <source> [--to <fmt>]      # dry run: plan or refusal, changes nothing
siphon recover <source>                   # resume or salvage a crashed job
siphon doctor                             # tool versions, FS capabilities, SELinux status
```

**Flags:**

| Flag | Applies to | Meaning |
|---|---|---|
| `--to <fmt>` | convert, inspect | Target format (required for `convert`) |
| `--mode auto\|safe\|relocate\|reclaim` | convert, inspect | Space/crash trade-off (§3.3); default `auto` |
| `--segment-duration <dur>` | relocate | Segment window; default `60s`. The main lever on free space. |
| `--segment-tolerance <dur>` | relocate | Keyframe snap tolerance; default `0.5s` |
| `--verify sampled\|structural\|full` | relocate | Per-segment verification depth (§4.7); default `sampled` |
| `--verify-frames <n>` | relocate | Frames decoded per fragment by `--verify sampled`; default 3 |
| `--probe-segments <n>` | relocate | Segments sampled by the pre-flight stitch probe before any truncation; default 4 |
| `--skip-probe` | relocate | Skip the pre-flight stitch probe (unsafe) |
| `--force` | all | Overwrite an existing target |
| `--yes` | convert | Skip the reclaim cost confirmation |
| `--drop-incompatible` | MKV | Skip tracks MP4 cannot carry |
| `--transcode-audio` | MKV | Encode FLAC/DTS/TrueHD/PCM to AAC (`safe` only) |
| `--allow-nonstandard` | MKV | Permit VP9/Opus/FLAC-in-MP4 |
| `--drop-metadata` | images, MKV | Permit metadata loss |
| `--bg <color>` | images | Background for alpha stripping |
| `--quality <q>` | images | Encoder quality; default 90 |
| `--bitrate <kbps>` | WAV | CBR instead of VBR `-V 2` |
| `--timeout <dur>` | all | Subprocess deadline; default `1h` |
| `--allow-root` | all | Required for UID 0 |
| `--allow-hardlinks` | all | Permit `st_nlink > 1` |
| `--json` | all | Machine-readable result on stdout |
| `--dry-run` | — | Alias for `inspect` |

`inspect` prints the plan *including* the free-space requirement in absolute MB and the segment window, e.g.:

```
film.mkv → film.mp4
  mode        relocate (default for video)
  output      fragmented MP4
  plan        from Cues (no scan pass)
  segments    118 × ~60s (keyframe-aligned)
  verify      sampled (3 frames per fragment)
  free needed 342 MB     you have 3.1 GB
  crash cost  source stays playable; recover resumes exactly
```

### 5.2 Exit Codes

| Code | Meaning | Source untouched? |
|---|---|---|
| 0 | Success | source consumed |
| 1 | Internal error / runtime resource failure | yes (safe) / journalled (relocate) |
| 2 | Refusal (pre-flight) | yes, byte-identical |
| 3 | Verification or stitch failure | yes (safe) / no truncation past the failure point (relocate) |
| 4 | Recovery available (job found) | consistent but split across source + temp |
| 5 | Dependency problem (non-preflight) | yes |
| 130 | Interrupted | yes (safe) / journalled (destructive modes) |

`--json` emits `{"ok":bool,"code":"E_*","message":"…","plan":{…},"source_untouched":bool}`. The KDE wrapper reads `code` and `message`; it never parses prose.

### 5.3 KDE Context Menu Integration

Installed to `~/.local/share/kio/servicemenus/` (Plasma 6) and `~/.local/share/kservices5/ServiceMenus/` (Plasma 5). One `.desktop` per pair per family, `MimeType=` declared, `%F` so multi-selection works:

```ini
[Desktop Entry]
Type=Service
ServiceTypes=KonqPopupMenu/Plugin
MimeType=video/x-matroska;
Actions=convertToMp4;
X-KDE-Submenu=Siphon

[Desktop Action convertToMp4]
Name=Convert to MP4 (saves space)
Exec=siphon-gui-kde convert %F
```

The format pair is fixed per action; the wrapper never guesses a target.

### 5.4 The `siphon-gui-kde` Wrapper

`#!/usr/bin/env bash`, `set -euo pipefail`, every expansion quoted.

1. Per file: `siphon inspect "$f" --to "$fmt" --json`. Accumulate failures; one `kdialog --error` at the end, not one per file.
2. If the plan warns (alpha stripping, metadata drop, reclaim mode), `kdialog --warningcontinuecancel` quoting the specific cost from `plan`.
3. `siphon convert "$f" --to "$fmt" --yes --json` in the background, reading `progress` lines from a FIFO.
4. **Progress:** `--progressbar` returns a DBus reference, not a stream, and must be driven explicitly:

   ```bash
   ref=$(kdialog --title "Siphon" --progressbar "Converting $(basename "$f")" 100)
   svc=${ref%% *}; obj=${ref#* }   # kdialog prints "<service> <object-path>"
   qdbus6 "$svc" "$obj" org.kde.kdialog.ProgressDialog.setValue "$pct"
   qdbus6 "$svc" "$obj" org.kde.kdialog.ProgressDialog.close
   ```

   (`qdbus` on Plasma 5.) If `qdbus6` is absent, fall back to `--passivepopup` at start and end.
5. Batch-safe exit handling: the convert call is wrapped in `if … then … else … fi` with the code captured, so one failure does not abort the loop.
6. `--passivepopup "Converted N file(s)"`, or `--error` listing the `E_*` codes.

### 5.5 Format-Specific Rules

- **MKV → MP4:** remux only. Attachments and chapters dropped (reported). Subtitle/audio handling per §4.5.3. Default output is fragmented MP4; classic MP4 via `--mode safe`.
- **WAV → MP3:** >2 channels refused. Default LAME VBR `-V 2`; `--bitrate` switches to CBR.
- **Images:** alpha → JPEG requires `--bg`. Orientation applied at decode. ICC always preserved; EXIF per §4.2.

---

## 6. Operational Edge Cases & Hardening

### 6.1 Subprocess Lifecycle & Timeouts
**Risk:** A corrupt file sends `ffmpeg`/`lame` into an infinite loop.
**Mitigation:** `exec.CommandContext` with the `--timeout` deadline *and* a context derived from `signal.NotifyContext`, whichever fires first. On expiry: `SIGKILL`, wait, drain pipes, then run the mode's failure path (journal intact; temp removed in `safe`; **no truncation** after cancellation in `relocate`). A watchdog aborts if a segment produces no output for 60 s while input flows.

### 6.2 Disk Space Exhaustion (`ENOSPC`)
**Risk:** Disk fills mid-conversion.
**Mitigation:** Every write is wrapped to detect `ENOSPC`/`EDQUOT`.
- `safe`: kill subprocess, delete temp, journal intact, exit 1 with `E_NO_SPACE_RUNTIME`.
- `reclaim`: kill subprocess, `fdatasync`, journal checkpoint, exit 4; `recover` splice-resumes.
- `relocate`: the segment that hit `ENOSPC` has not been truncated, so the engine stops with the source intact up to `tail_cursor` and the verified fragments in the temp. Exit 4; `recover` resumes after space is freed. **Never truncate on the ENOSPC path** — the space accounting is no longer trustworthy.

### 6.3 Privileged Execution
`os.Geteuid() == 0` refuses at startup unless `--allow-root`, with a severe warning naming the file. The ServiceMenu never passes it.

### 6.4 Locking — Advisory, and Said So
`flock(LOCK_EX|LOCK_NB)` on the source and journal coordinates Siphon and well-behaved tools; it does not stop `vim` or a shell redirect. The real defence is the `dev`/`ino`/`mtime`/`size` re-check at open and every checkpoint (`E_SOURCE_CHANGED`). No exclusive-access claim is made.

### 6.5 Metadata, Ownership, and ACLs on Replacement
**Risk:** `rename(2)` is not metadata-preserving: the temp carries its own inode, so the result loses the source's SELinux label, POSIX ACLs, user xattrs, and capabilities. On Fedora/KDE with SELinux enforcing this can make the file unreadable by its owner's session.

**Mitigation:** before `renameat2`, copy onto the temp inode, in order:
1. `fchmod` to `source.st_mode & 07777`
2. `fchown` to source uid/gid (only under `--allow-root`)
3. `user.*`, `security.*`, `trusted.*` via `flistxattr`/`fgetxattr`/`fsetxattr` — `security.selinux` failure is fatal (`E_METADATA_DROP`), `user.*` failure warns
4. POSIX ACLs (exposed as `system.posix_acl_*` xattrs, so step 3 covers them)
5. `security.capability` present → refuse with `E_METADATA_DROP` (a capability-bearing media file means the wrong target was chosen)
6. `atime` preserved; `mtime` set to completion time

After `renameat2`, `dir_fsync` the directory. `relocate` does this once, at the very end, on a temp that already holds the whole output.

### 6.6 Shell Quoting & Weird Filenames
All expansions quoted; loop over `"$@"`; never rely on `IFS`. Go treats filenames as raw bytes, never assumes UTF-8, and validates every `ffprobe` field it reads.

### 6.7 Source Validation (Magic Bytes)
First 16 bytes: WAV `RIFF....WAVE`; MKV EBML `1A 45 DF A3`; PNG `89 50 4E 47 0D 0A 1A 0A`; JPEG `FF D8 FF`; WebP `RIFF....WEBP`. Mismatch → `E_SOURCE_CORRUPT`. Load-bearing because the engine is chosen from the extension: a renamed file would be fed to the wrong encoder, produce plausible garbage, and then have its source destroyed.

### 6.8 Dependency Versioning
`ffmpeg -version`, `lame --version`, `cwebp -version`, `dwebp -version` parsed at pre-flight against §4.2 minimums. Optional binaries are required only by the pairs that use them.

### 6.9 Signal Handling
`signal.NotifyContext(ctx, os.Interrupt, SIGTERM)` parents every subprocess and the segment loop. On cancel: kill the subprocess, drain, ensure no `truncate`/`rename` runs afterwards, restore the terminal, print `"Interrupted. Source safe. Run 'siphon recover <file>'."` — or, in destructive modes, `"Interrupted. Source is a valid shorter file; run 'siphon recover <file>' to finish."` — exit 130.

### 6.10 TOCTOU on the Target
`renameat2(AT_FDCWD, tmp, AT_FDCWD, target, RENAME_NOREPLACE)` closes the check-then-act window; with `--force`, flags 0. Never `os.Rename` followed by a `Stat`.

### 6.11 Temp File Creation
`O_RDWR|O_CREAT|O_EXCL|O_NOFOLLOW`, mode `0600`, random 8-hex suffix, same directory. Single deferred cleanup, disarmed only after a successful `renameat2`.

### 6.12 Segment Stitching Hazards (new in v1.6)

These are the ways a segmented remux silently produces a broken file. Each has a detection:

| Hazard | Symptom | Detection |
|---|---|---|
| Track config changes mid-file (SPS/PPS, sample rate) | Frames decode wrong after the change point | 3-point probe at pre-flight (§4.9 check 13); per-segment extradata comparison (§4.7) |
| Byte range starts mid-cluster or in the wrong GOP | Duplicated or missing frames; duration drift at the join | Cluster-ID scan at plan time; first-sample PTS check (§4.5.4.3 step 2) |
| Source index (`Cues`) destroyed by the first truncation | Every later seek degrades to a linear scan; runtime explodes | Never seek: synthetic-header byte-range input (§4.5.4.3) |
| `Cues` absent, stale, or covering only part of the file (streamed/truncated MKV) | Plan would be incomplete or wrong | Validate coverage ≥90% against `Segment` duration; fall back to the `scan` path and report `plan_source: "scan"` (§4.5.4.1 step 3) |
| `Segment` size larger than the bytes actually fed | Demuxer reports truncation, some builds abort | Cache and patch the header's `Segment` size to "unknown" at plan time (§4.5.4.1 step 5) |
| Open GOP: boundary frames reference the next GOP | Visual corruption for ~1 GOP at a boundary | Per-fragment sampled decode with `-xerror` (§4.7 step 5); if it surfaces repeatedly, boundary is moved one keyframe earlier |
| Non-monotonic `tfdt` across fragment joins | Players show wrong duration, seek lands wrong | Assertion at write time; `mfhd` renumbered by us |
| Multiple `moov` in one file | Some players stop after the first | Fragments are stripped of `ftyp`+`moov` for `i < N-1` (§4.5.4.3 step 3) |
| Duplicate or missing packets at a join | A/V drift accumulates | Coverage check against the plan's sample count, plus per-fragment sampled decode (§4.7 steps 4–5) |
| Edit list only valid for the first segment | Audio priming misapplied at every join | Only the init segment's `moov` is kept, so exactly one edit list exists |

The pre-flight stitch probe (§4.5.4.2) exercises the first and last segments against all of these before any truncation.

### 6.13 Failure Injection Hook
`SIPHON_FAULT="<point>:<n>"` (points: `read`, `write`, `journal`, `fsync`, `rename`, `truncate`, `segment`) compiled only under the `faultinject` build tag, absent otherwise. This is what makes §7's crash matrix deterministic.

---

## 7. Testing Strategy (Linux)

| Layer | What it covers | Runs in |
|---|---|---|
| Unit | Preflight, journal parse/replay, cursor invariants, ICC/EXIF transplant, magic bytes, `-map` builder | Every commit |
| Algorithm | `SlidingEngine` under a fake encoder (adversarial ratios, zero output, ENOSPC); `RelocateEngine` under a fake muxer | Every commit |
| Stitch Corpus | Real MKVs (H.264/HEVC/AV1, closed and open GOP, multi-audio, 4K, Cues present and absent) × segment durations; asserts one-pass I/O (no re-reads) | CI nightly |
| Plan Equivalence | `Cues`-derived plan vs `scan`-derived plan on the same file: identical boundaries within tolerance, identical segment counts | Every commit |
| Verify Sensitivity | Injected faults (dropped sample, shifted `tfdt`, wrong decoder config, chopped GOP) must be caught by `--verify sampled` on the first faulty fragment | Every commit |
| Performance Regression | 4 GB fixture on a tmpfs: total I/O ≤ 2.1 × file size with `Cues`, ≤ 3.1 × with `scan`; runtime within 20% of baseline | Nightly |
| Player Matrix | Each stitched output opened in VLC, mpv, Chrome, an Android device, and `ffprobe -count_packets` | Release only |
| Golden Corpus | Every pair, every refusal reason, every flag combination | CI (Ubuntu 24.04, Fedora 40) |
| Crash Injection | `SIPHON_FAULT` at every point × N offsets, then `recover` | Nightly |
| Adversarial | Symlinks, hardlinks, read-only dirs, immutable files, fake headers, root, sticky `/tmp` | Nightly |
| Filesystem Matrix | ext4, XFS, Btrfs (success) and exFAT/NTFS-3G (refusal at check 12, no sparse files) | Nightly, via VMs |
| KDE Integration | `kbuildsycoca6` + `qdbus6` progress drive, multi-select `%F` | Release only |

**The Golden Corpus invariants.**

- **Refusal:** source `(size, mtime, ctime, sha256, xattr set)` **and** the directory listing are unchanged.
- **Safe success:** source gone, no temp or journal remains, target matches the reference by **decoded frame hashes and stream properties**, never file bytes (encoder output and box ordering differ across `ffmpeg` versions; byte comparison would produce false failures). CI pins exact tool versions.
- **Relocate success:** same as safe success, plus the output must pass the full gate, plus `peak_blocks ≤ source_size + max_segment_bytes` measured from `/proc/self/io` or a loop device.
- **Relocate crash:** after a fault at *any* point, `recover` completes the job, and the completed output decodes identically to the uncrashed run. This is the invariant that `SIGKILL`-at-random-offset testing cannot establish and the fault hook can.

### 7.1 Free-Space Estimation

- **WAV→MP3 (`safe`):** `duration × ceiling_bitrate / 8 × 1.05`, plus source size.
- **WAV→MP3 (`reclaim`):** `source × 1.01`.
- **MKV→MP4 (`relocate`):** `segment_duration × total_bitrate / 8 + 16 MiB`. Independent of file size — this is the number shown to the user.
- **MKV→MP4 (`safe`):** `source × 1.03 + 1.10 × Σ(stream_bitrate × duration) / 8 + 4 MiB`.
- **Images:** `source + max(2 × source, decoded_raw_size × 1.5) + 2 MiB`.

All are deliberately pessimistic; over-refusal is acceptable, under-refusal is not.

---

## 8. Build Order & Deployment

### 8.1 Build Phases

0. **Platform & Journal (1 week):** same-dir temp with `O_EXCL|O_NOFOLLOW`, `renameat2`, `dir_fsync`, xattr/ACL transfer, journal with CRC + atomic checkpoint, `ProgressSink`, fault-injection harness. Lands before any engine.
1. **Skeleton (1 week):** CLI, preflight, magic bytes, root detection, exit codes, `--json`, `siphon inspect` planning output.
2. **Image Engine (1 week):** AtomicCopyEngine, `imaging` + `cwebp`/`dwebp`, ICC/EXIF transplant, alpha refusal.
3. **WAV→MP3 (2 weeks):** SlidingEngine `safe` first, then `reclaim` behind the forward invariant; splice-resume.
4. **Video: stitch spike (1 week, gating).** Standalone `internal/lab/stitch` that takes an MKV, builds the plan from `Cues` (with the `scan` fallback), produces the segments tail-first **from cached-header byte ranges with no seeking**, assembles them, and reports packet counts, `tfdt` monotonicity, elapsed I/O and pass count, and player results. The spike must also show that `--verify sampled` catches each injected stitching fault. **The phase-5 estimate is only valid if this spike passes**; if it does not, fall back to `safe`-only video and re-plan.
5. **RelocateEngine (3 weeks):** EBML `Cues` parser + `scan` fallback, segment planner, per-segment protocol, sparse reverse layout, `free` box assembly, pre-flight stitch probe, three-level verification, exact resume.
6. **KDE Integration (1 week):** `.desktop` files, `qdbus` wrapper, batch aggregation, inspect-driven warnings.
7. **Hardening (1 week):** fault matrix, FS matrix, adversarial suite, fuzzing of journal and box parsers.

### 8.2 Distribution

- **Static binary:** `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w"`.
- **Runtime dependencies:** `ffmpeg`/`ffprobe` ≥ 5.0 and `lame` ≥ 3.100 required; `libwebp-tools` only for WebP pairs; `kdialog`/`qdbus` optional.
- **Packaging:** `.deb`/`.rpm` with `Depends: ffmpeg, lame`, `Recommends: libwebp-tools`, post-install `kbuildsycoca6`. Fedora users may need RPM Fusion for `lame`; the error text names it.
- **Man page** documents §1.1, §3.1, and §4.6.5 verbatim. The guarantees are the product.

---

## 9. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| FFmpeg/LAME missing or outdated | Pre-flight `LookPath` + `-version`, per-pair optionality |
| Segmented remux produces a subtly broken file | 3-point config probe, per-segment decode, packet-count comparison, pre-flight stitch probe (§6.12) |
| Relocate fails mid-film | Truncation is always last; source stays a valid shorter MKV; exact resume (§4.5.4.3) |
| Fragmented MP4 refused by an old player | Stated in `inspect` and the man page; `--mode safe` produces classic MP4 |
| User sets a huge `--segment-duration` on a full disk | Pre-flight compares the computed requirement to actual free space and refuses with the maximum viable duration |
| Data loss on crash (audio `reclaim`) | Opt-in, cost printed, splice-resume documented per path |
| SELinux/ACL breakage after rename | Explicit metadata transfer, fatal on `security.selinux` failure |
| Wrong codec muxed into MP4 | Declarative matrix + `--allow-nonstandard` gate |
| Byte-comparison flakiness in CI | Decoded-frame-hash comparison, pinned tool versions |
| Verification is too slow on 4K and users disable it | Sampled default (§4.7), which is diagnostic for the faults that actually occur |
| `Cues` parser disagrees with ffmpeg's keyframe list | Plan Equivalence test (§7); the `scan` path is the reference |

---

## 10. The One-Paragraph Contract

Siphon converts media files in place on Linux, on disks that are nearly full. For audio it builds a verified MP3 beside the WAV and needs about 15% extra space, or — with `--mode reclaim` — none, at the cost of a splice if it crashes. For video it never duplicates the film: it works backwards from the end, muxing one keyframe-aligned segment at a time, verifying it, writing it at its final offset, and only then releasing those bytes from the source, so a 60 GB movie converts with a few hundred megabytes free. On success the source is gone and the output is in its place, verified. On refusal nothing was touched and the file is byte-identical, including its metadata. On a crash the source is still a playable file and `recover` finishes the job exactly where it stopped. It never requires a second disk, never needs 2× space by default, never strips alpha, drops a track, transcodes audio, or discards an ICC profile without a flag, never makes a network call, and never claims a guarantee that the filesystem or the container format cannot provide.

---

## Appendix A — Rejected Designs (and why)

**A.1 Reverse-encoded MP3 assembly (v1.4).** Encoding the WAV tail and "reversing MP3 frames" is impossible: MP3 frames are not independently decodable (bit reservoir, MDCT overlap).

**A.2 `FALLOC_FL_PUNCH_HOLE` during remux (v1.4).** Destroys bytes irreversibly, cannot make a non-resumable mux resumable, and buys nothing truncation does not, while leaving file size unchanged.

**A.3 Streaming MP4 to a pipe without fragmentation (v1.4).** The MP4 muxer needs a backward seek for `moov` and errors on non-seekable output.

**A.4 Video `reclaim` / fragmented overwrite-behind (v1.5).** With output ≈ input, the margin between the write and read cursors is under 1%. Any growth (AAC transcode, container overhead) forces a spill that lands at 2× **after** the source has already been damaged — the worst combination available. Replaced by `relocate`, which needs no margin because it moves bytes rather than overwriting them.

**A.5 `--staging <dir>` on a second disk (considered in v1.6).** It achieves 1× on the source disk by converting elsewhere, deleting the source, and copying back. Rejected: the target user does not have a second disk, so the flag would exist only to make the spec look better.

**A.6 End-of-job compaction of the reserved region (considered in v1.6).** After writing fragments top-down, the leftover gap at the front could be removed by shifting the whole output left in place. Rejected as unnecessary: an MP4 `free` box of exactly that size is legal, costs no blocks because the gap is a hole, and avoids a full extra read+write pass over the output.

**A.7 1× in-place operation for images.** Image encoders produce unpredictable sizes and have no valid intermediate state; small files do not justify the risk.

**A.8 Pure-Go WebP encoding.** No encoder exists in `x/image`; `cwebp` is the only dependable path and matches P8.

**A.9 Silent EXIF re-injection via re-encoding.** Re-encoding discards all markers; EXIF must be transplanted at segment level or explicitly dropped with a flag.

**A.10 `flock` as an exclusivity guarantee.** Advisory locks do not bind non-cooperating writers; re-checked `dev/ino/mtime/size` is the real defence.
