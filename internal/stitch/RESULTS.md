# Video stitch spike — results (phase 4)

Status: **PASS**. The spike proves the pieces `RelocateEngine` needs: a
Cues-based (or scanned) segment plan, tail-first extraction from a cached EBML
header plus byte ranges with no seeking, a byte-exact fragmented-MP4 assembly
written backwards into a sparse reserve, per-track timestamp re-anchoring, and
a verifier whose checks demonstrably catch injected stitching faults.

It is a lab, not a CLI feature: no journal, no recovery, not wired into
`internal/convert`.

## How to run

```sh
make build
go test ./internal/stitch/ -v          # unit + real-ffmpeg E2E + fault suite

go run ./internal/stitch/cmd/stitch \
  -segment-duration 4s -full-decode \
  -out /tmp/out.mp4 input.mkv
```

`-force-scan` ignores Cues and uses the `ffprobe -show_packets` fallback;
`-verify-frames N` sets the sampled decode gate (default 3, 0 disables).

## What the spike contains

| File | Role |
|---|---|
| `ebml.go` | EBML header/index walk, Cues, Tracks, Info; cached-header rewrite |
| `plan.go` | keyframes (Cues → scan fallback), boundary selection, synthetic pieces |
| `anchor.go` | first-block PTS per track in a segment's opening cluster |
| `extract.go` | one ffmpeg stream-copy per segment, header + pieces on stdin |
| `mp4.go` | fragmented-MP4 box parsing (trun/tfdt), init strip, free box |
| `timing.go` | moof patch pass: tfdt re-anchor, cts normalization, mfhd renumber |
| `verify.go` | assembled-file checks (boxes, timeline, counts, sync, one pass) |
| `*_test.go` | byte-level builders, unit tests, real-fixture E2E, fault injection |
| `cmd/stitch` | lab harness and report |

## Fixture and plan

The E2E fixture is 12.023 s of 320×240 H.264 (30 fps, `-bf 2`, 1 s GOPs) +
AAC 44.1 kHz, written as Matroska with 1 s clusters: 360 video and 518 audio
packets, 620.0 KiB. Video timescale 16000, audio 44100, TimecodeScale 1 ms.

- 12 keyframes → 3 segments at 4 s (the tail rule lets the last segment run to
  EOF).
- Cues and `-force-scan` produce **identical** keyframes, segments, pieces, and
  cached header (757 B). Cues coverage must reach 90 % of Duration, else the
  scan is used automatically.
- Segment source spans (cluster start → next keyframe block):
  `757..211230` (120 v / 171 a), `207882..426757` (120 v / 172 a),
  `426451..634930` (120 v / 175 a). The spans overlap because a boundary
  cluster is split: its prefix rides at the previous segment's tail and its
  keyframe onward opens the next segment. The **copied** bytes are disjoint.
- `Plan.ExtractBytes` = 634148 B, 99.9 % of the file.

## Findings that shaped the design

### 1. The fragmented-MP4 muxer rebases each track per run

Every extraction run is a separate ffmpeg process. Its fragmented muxer
rebases each track to that track's own first DTS, so tfdt and PTS restart near
zero per segment. `-copyts`, `-output_ts_offset`, and `+dash` do not preserve
absolute time, and `+frag_discont` breaks the initial moov. Assembly therefore
re-anchors every segment/track itself (see §4).

### 2. The mov demuxer derives a per-track `dts_shift`

ffmpeg's mov demuxer computes `dts_shift = max(0, −min_cts)` for a track
(`mov_update_dts_shift`) and reconstructs PTS as `tfdt + cts + dts_shift`,
accumulated in parse order. The absolute timeline would then depend on how much
of the file a demuxer had parsed. The patch pass **normalizes every traf's
composition offsets so none is negative**; `dts_shift` is then 0 for every
demuxer and parse order.

### 3. Cluster prefixes can start with the previous GOP's B-frames

A Matroska cluster is not required to start on a keyframe. In the fixture,
cluster 4 begins `B(3.967), B(3.933), I(4.0)` in decode order: the two trailing
B-frames of GOP 3 were written after the previous cluster. Two wrong fixes were
rejected empirically:

- **Plain stream copy** drops leading non-keyframes (ffmpeg `of_streamcopy`:
  `!copy_initial_nonkeyframes`). The 4 s boundary lost 2 video packets and the
  retained keyframe landed 33 ms early, because the anchor was the dropped B.
- **`-copyinkf`** keeps the packets, but the muxer's start-of-stream logic
  gives the leading B-frames 1-tick durations (30 samples: durations
  528, 1, 1, …), compressing the first GOP and shifting the boundary by 33 ms.
  It is an output option; as an input option ffmpeg refuses it.

**Chosen design — synthetic cluster boundaries.** Every extraction starts with
a keyframe:

- The opening cluster is rebuilt from its original Timecode element plus
  `[keyframe block, cluster end)`. A fresh Cluster header is encoded around
  that payload.
- When the next boundary cluster starts with a prefix (B-frames, audio), that
  prefix is appended to the *previous* segment as a second synthetic cluster
  (Timecode + prefix). Its DTS follows the previous cluster's last block and
  precedes the next keyframe, preserving decode order. The prefix's leading
  CRC-32 is dropped because it covered the whole source cluster.
- The segment in between is copied as raw cluster bytes.
- The first segment keeps its whole opening cluster (including any AAC priming
  blocks before the keyframe); only later segments skip the prefix.

Result: exact packet counts and an exact source timeline (below). The sampled
decode gate starts every fragment at a keyframe; the assembled full decode is
the authoritative cross-fragment check.

### 4. CodecDelay must be subtracted from anchors

Matroska block timecodes are pre-delay; demuxers subtract `CodecDelay`
(`0x56AA`). The fixture's AAC delay is 23 219 955 ns (1024 samples at 44.1
kHz). Anchors are therefore `block PTS − CodecDelay`, which reproduces the
source's −23 ms priming and keeps A/V sync.

### 5. Tick math is integer and split

`roundTicks` (nearest, halves away from zero), `ceilTicks` (true ceiling), and
`ceilNS` split at 1 s so the ns×timescale product cannot overflow. The global
pad is `max ceilNS(rawFirst − desired)` over every segment/track, so no patched
decode time is negative; each traf's delta is
`roundTicks(anchor) + ceilTicks(pad) − rawFirst`.

## Assembly

1. `os.CreateTemp` in the output directory; `Truncate(reserve)` with
   `reserve = source×1.05 + 16 MiB` (spec §4.5.4.1 step 6).
2. Extract segments **tail-first** (N−1 → 0) so the read cursor and write
   cursor approach each other. Each fragment is written at `outCursor` after
   subtracting its body length.
3. The init segment (`ftyp`+`moov`, stripped from the last-processed
   extraction) is written directly in front of the fragments.
4. Any leftover front hole gets one legal `free` box header; the file is a
   valid MP4 that begins with `free`, then `ftyp`.
5. The patch pass reads/writes only moofs (a few hundred KB per hour): per
   traf, normalize cts, add the tfdt delta, renumber mfhd 1..N.
6. `fdatasync`, close, rename into place, then verify.

The leading `free` box means the file's length is the reserve (16.6 MiB for
this small fixture); it is sparse, so disk usage is the media size. The real
engine can keep it (legal, bounded overhead) or compact it; the spike keeps it.

## Verification and fault sensitivity

Checks on every run:

- box chain: first box `free`/`ftyp`, moof count = sum of fragment moofs;
- `mfhd` sequence 1..N;
- per-track tfdt non-decreasing across moofs;
- no negative composition offsets;
- exact packet counts per stream vs the source (`ffprobe -count_packets`);
- video start anchor = source + pad (±10 ms), A/V offset vs source (±10 ms);
- one-pass accounting: `ExtractRead == Plan.ExtractBytes`;
- sampled decode: 3 video frames per fragment with `-xerror`;
- optional full decode with `-xerror`.

`fault_test.go` runs one shared completed Run and mutates a copy per case:

| Injected fault | Detector | Observed |
|---|---|---|
| `SrcOff` moved to the cluster payload (mid-cluster) | anchor walk start guard | `E_SOURCE_CORRUPT: segment start 764 is not a Cluster` |
| `avcC` payload zeroed | full decode | ffmpeg fails (`Nothing was written…`) |
| first trun sample_count − 1 | packet counts | video 359/360 |
| middle video tfdt − 2 s | tfdt monotonic | moof 5 tfdt 48000 < 64000 |
| video trun cts − 1000 | cts non-negative | min cts −1000 |

## Measurements (12 s fixture, 3 segments, 4 s window)

```
plan        cues, 12 keyframes, 3 segments, header 757 B
pad         34.000 ms (video cts min −544 at 16 kHz)
fragments   4 + 4 + 5 = 13 moofs
packets     video 360/360, audio 518/518
timeline    every output packet = source PTS + 34 ms
            video worst deviation 0.000 ms, audio worst 0.735 ms
start       video 0.034000 s vs source 0.000000 s (Δ 0.000000)
a/v sync    audio offset −0.023206 s vs source −0.023000 s (Δ −0.206 ms)
I/O         extract 634148 B (99.9 % of 634880)
            anchors 316 B, plan 16.6 KiB, header 757 B × 3
            total ≈ 1.027 passes; moof patch r/w 19.4/9.7 KiB
wall        ≈ 0.7 s extraction for 3 segments (process overhead dominates)
result      PASS (sampled + full decode)
```

The single-segment plan produces the same packet counts and a 0.000 ms video
deviation; the delayed-audio fixture (0.5 s A/V offset, 3 s segments) keeps
the offset to within 0.2 ms. `TestPlanCuesAndScanAgree` proves the fallback
path is equivalent on the fixture.

## Limits / notes for `RelocateEngine`

- MKV→MP4 only, **one video + one audio track** (track IDs video=1, audio=2).
  Multi-track files are refused with `E_FORMAT_UNSUPPORTED`.
- Sized clusters only. Cues must point at sized clusters; the scan fallback
  refuses unknown-size clusters. (Both are true of ffmpeg's MKV muxer.)
- Boundaries are one per cluster (Cues and scan dedup by cluster). The piece
  builder handles two boundaries in one cluster, but the planner does not
  produce that case.
- Non-cluster top-level elements between clusters are copied verbatim in the
  middle range (the last segment's trailing Cues is included). They are not
  position-rebased; ffmpeg ignores them without seeking. Mid-stream Cues would
  need a split-and-skip pass before this is production-grade.
- Only H.264/AAC were exercised. The design is codec-agnostic stream copy, but
  other codecs (HEVC, AV1, Opus) need their own fixtures.
- The sampled gate decodes from each fragment's keyframe; it cannot validate a
  cross-fragment reference chain. The full decode is the authoritative check
  (and is what the fault suite uses for `avcC`).
- `-force-scan` runs ffprobe over the whole file (O(n) JSON); the Cues path is
  the fast path.
- The output keeps the reserve-sized leading `free` box. Decide in phase 5
  whether to keep it or compact; the verifier accepts it either way.
- No journal or resume here. The plan's segment pieces are deterministic and
  are the natural journal payload for `RelocateEngine`.

## Gate

All phase-4 requirements are demonstrated: Cues plan + scan fallback, no-seek
byte-range extraction, exact packet counts, tfdt monotonicity, one-pass I/O,
sampled verification, and proven fault sensitivity. Proceed to phase 5
(`RemuxEngine` + `RelocateEngine`).
