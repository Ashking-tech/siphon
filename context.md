# Siphon — Working Context (checkpoint)

> Living document. Update this file whenever a module, pipeline, decision, or
> command lands, then commit. It is the resume point after context loss.
> Read `spec.md` (v1.7) for the full contract; this file records **what exists**.

**Last updated:** 2026-10-24
**Repo:** `/home/ashking/Projects/siphon` (local git, branch `main`)
**Go module:** `siphon` (provisional; rename before publishing)

---

## 1. Product in one paragraph

Siphon converts media **in place** on Linux without needing 2× free space.
CLI + KDE ServiceMenu + first-class GUI are all first-class surfaces. Media
work is delegated to `ffmpeg`/`ffprobe`/`lame`/`cwebp`/`dwebp`; Siphon owns
crash-safe, low-space file manipulation. Video uses `relocate` (tail-first,
keyframe-aligned segments, verify-before-truncate). Audio `safe` uses a
same-directory temp; audio `reclaim` overwrites behind the read cursor (opt-in).
Images always use `safe` (temp + atomic rename).

Supported pairs: WAV→MP3, MKV→MP4, PNG/JPEG/WebP matrix. See `spec.md` §3.1.

---

## 2. Current status

| Area | State |
|---|---|
| Repo / module | initialized, `go.mod` (cobra, x/sys) |
| Context checkpoint | this file |
| Error model / exit codes | implemented + tested |
| Format table / mode resolution | implemented + tested |
| Journal (CRC, atomic save, lock) | implemented + tested |
| Platform layer (temp, rename, fsync, xattr, sparse probe, magic, temp reap) | implemented + tested |
| Progress sinks | implemented (noop, terminal, plain, JSON) |
| Preflight gate | checks 1–7, 14–18, 20 + WAV gate + space + image probe |
| CLI skeleton | `convert`, `inspect`, `recover` (driver wired), `doctor`, `version` |
| Imaging layer (decode, orientation, ICC/EXIF/XMP, WebP, verify) | implemented + tested |
| Video stitch spike (`internal/lab/stitch`) | implemented + tested: Cues/scan plan, synthetic cluster boundaries, sparse reverse assembly, fault suite (`RESULTS.md`) |
| Engines | AtomicCopy (images), Sliding (WAV→MP3 safe + reclaim) implemented + tested; Relocate / Remux not yet |
| WAV parser (`internal/wav`) | implemented + tested (PCM + extensible, INFO/bext detection, header synth) |
| Recovery driver (`internal/recover`) | implemented + tested (sliding splice-resume) |
| GUI | not yet (KDE wrapper planned; GUI toolkit decision pending) |

`go build ./...` and `go test ./...` pass. `siphon doctor` and `siphon inspect`
work. `siphon convert` converts the image pairs and WAV→MP3 (safe and reclaim)
end to end. A killed reclaim job is finished by `siphon recover <file>`
(splice-resume, ~26 ms seam), including crashes after truncate or rename.
`siphon recover` on a video/relocate journal still exits 4.

The phase-4 video stitch spike passes on the 12 s H.264/AAC fixture: 3 segments,
360/360 video and 518/518 audio packets, every output packet at source+34 ms
(video worst deviation 0.000 ms), one-pass extraction (634148/634880 B), and
sampled + full decode clean. `internal/lab/stitch/RESULTS.md` records the
design and the fault-sensitivity matrix. It is a lab, not wired into the CLI.

---

## 3. Decisions

| Decision | Choice | Why / note |
|---|---|---|
| Language | Go, `CGO_ENABLED=0` for core | spec P8 / §4.1 |
| Module path | `siphon` | provisional; change when publishing |
| CLI | cobra | spec §4.2 |
| Errors | typed `siphonerr.Error` with `Code` + exit override | one structured contract for CLI/GUI |
| Journal schema | spec §4.6 schema 2 | CRC32C Castagnoli over struct minus CRC field |
| Journal lock | dedicated `<journal>.lock` file | atomic rename replaces the journal inode, so locking the journal itself would lose serialisation |
| Metadata | identical xattrs skipped before set | unprivileged users cannot set `security.selinux`; same-dir temps already inherit it |
| Temp files | `.siphon-<stem>-<8hex>.tmp` / `.journal` | spec §4.6.1, §6.11 |
| Atomic finalise | `renameat2(RENAME_NOREPLACE)` | spec §6.10 |
| Image engine | temp + verify + metadata, no journal | spec §4.5.1; a crash can only leave a temp |
| Image metadata | ICC/EXIF/XMP transplanted at container level; EXIF orientation applied at decode and normalised to 1 | spec §4.2; re-encoding drops markers, so they are re-injected |
| PNG → WebP | `cwebp -lossless` (spec table marks the pair lossless); JPG → WebP uses `-q` | `formats` marks PNG→WEBP non-lossy |
| Animated sources | APNG / animated WebP refused with `E_FORMAT_UNSUPPORTED` | converting one frame silently would be unasked data loss |
| Stale temp reaping | same-stem `.tmp` older than 1 h, only when not flocked | live temps are flocked; a concurrent conversion is never disturbed |
| Non-TTY progress | `progress.Plain` (no ANSI) unless stderr is a character device | keeps logs and the KDE wrapper clean |
| LAME (safe) | `lame --silent -V 2 - <tmp-path>` (or `-b <kbps>`) | file output gets a proper Xing header; byte-identical to path-input + path-output |
| LAME (reclaim) | `lame --silent -V 2 - -` with pipes | pipe VBR output carries no Xing header by design; recovery scans frames instead of trusting a header |
| MP3 gate | ffprobe stream props + full decode byte-count duration | headerless VBR container estimates are ~6 % off; decode count is the ground truth |
| Reclaim resume | scan complete MP3 frames in the written prefix, back off one frame of PCM | encoder delay is not observable from the stream; the splice is a bounded ≤26 ms echo, never a gap |
| Reclaim margin | spec margin while feeding; plain `write ≤ read` after EOF | the literal §4.4.1 margin would stall files smaller than 16 MiB forever |
| Reclaim checkpoints | `fdatasync` + journal save every 8 MiB read or 1 s with dirty writes | spec §4.4.1 |
| Reclaim finalise | `finalising` checkpoint → ftruncate → fdatasync → rename to target → journal remove | rename is what puts the MP3 "in place"; §4.4.1 omits it, recovery accepts both paths |
| WAV metadata | LIST/INFO and `bext` refused with `E_METADATA_DROP` unless `--drop-metadata` | no tag transplant into MP3 yet; never silently dropped |
| WAV formats | PCM (tag 1) and WAVE_FORMAT_EXTENSIBLE with PCM subformat; other tags `E_FORMAT_UNSUPPORTED`; >2 ch refused | spec §4.5.2, §6.7 |
| Video boundaries | synthetic cluster split: every extraction starts at a keyframe; the previous segment carries the next cluster's leading B-frame/audio prefix | plain copy drops leading non-keyframes; `-copyinkf` gives them 1-tick durations and shifts the boundary 33 ms |
| Video timestamps | re-anchor each segment/track; normalize cts to non-negative | the fragmented muxer rebases per run; the mov demuxer's `dts_shift` depends on parse order |
| Video anchors | block PTS − CodecDelay | Matroska block timecodes are pre-delay (AAC priming 23 ms in the fixture) |
| GUI architecture | engine remains UI-agnostic; GUI consumes `--json` + progress events | decision still open on toolkit; KDE wrapper first |
| KDE progress | `kdialog --progressbar` via `qdbus6`, fallback `--passivepopup` | `qdbus6` missing on this host, fallback matters |

---

## 4. File map

```text
cmd/siphon/main.go              process entry, exit code
internal/siphonerr/             structured errors + exit mapping (+ tests)
internal/formats/               format table, pairs, modes, engine names (+ tests)
internal/core/                  Job, Plan, Result shared types
internal/platform/              temp files, fsync, renameat2, xattr, sparse, magic, reap (+ tests)
internal/journal/               schema 2 journal, CRC, atomic checkpoint, flock (+ tests)
internal/progress/              Sink interface + noop/terminal/plain/JSON
internal/media/                 tool discovery/version gates, ffprobe summary
internal/wav/                   RIFF/WAVE parser, INFO/bext detection, header synthesis (+ tests)
internal/preflight/             ordered refusal gate, plan + space estimate + image/WAV probes (+ tests)
internal/imaging/               image decode/encode, orientation, ICC/EXIF/XMP, WebP, verify (+ tests)
internal/engine/                Engine interface + registry
internal/engine/atomiccopy/     image engine: temp, alpha policy, verify, atomic rename (+ tests)
internal/engine/sliding/        audio engine: lame safe/reclaim, invariant, MP3 frames, splice-resume (+ tests)
internal/lab/stitch/            phase-4 video spike: EBML plan, synthetic segments, sparse assembly, verify, fault suite (+ RESULTS.md)
internal/recover/               recovery driver: journal discovery, inode probe, engine dispatch (+ tests)
internal/engines/               composition root: registers concrete engines
internal/convert/               orchestrator: preflight → engine (+ tests)
internal/cli/                   cobra commands, JSON/human output (+ tests)
internal/version/               version string
Makefile, README.md, context.md
```

---

## 5. Implemented API notes

### Errors (`internal/siphonerr`)
- `siphonerr.New(code, format, args...)`, `Wrap(code, cause, ...)`.
- `ExitCode(err)` maps refusal→2, verification/stitch→3, recovery→4,
  runtime dependency→5, interrupted→130, else 1. Preflight dependency
  refusals use `WithExit(2)`.
- `CodeOf(err)`, `IsCode(err, code)`.

### Formats (`internal/formats`)
- `Parse`, `FromPath`, `Format.Ext()`.
- `Lookup(src, dst) (Pair, bool)`; `ResolveMode(pair, requested)`.
- `EngineFor(pair, mode)`; `SupportedTargets(src)`.
- Pair metadata: default mode, allowed modes, lossy flag, output kind.

### Journal (`internal/journal`)
- `Create(dir, stem) (*Store, *Journal, error)` — `O_EXCL|O_NOFOLLOW`, 0600, flock.
- `Open(path, mode)` / `Find(dir, stem)`; `Store.Save(*Journal)` atomic
  (`tmp` → fsync → rename → dir fsync), increments `seq`, sets CRC.
- `Store.Close`, `Store.Remove`.
- Journal fields match spec §4.6.2; nullable `out_off`/`out_len`.
- CRC is computed over `json.Marshal` of the journal with `crc32c` blanked.

### Platform (`internal/platform`)
- `CreateTemp(dir, prefix)`; `Fsync`; `Fdatasync`; `FsyncDir`.
- `RenameNoReplace`, `RenameReplace`.
- `SameDevice`, `FreeSpace`, `SparseProbe`.
- `OpenSource` (`O_RDWR|O_NOFOLLOW`), `OpenReadOnly`, `ValidateMagic`.
- `CopyMetadata` (mode/uid/gid/xattrs), `HasImmutable`.
- `Snapshot`/`TakeSnapshot` (dev/ino/size/mtime), `Check`, `CheckOpen`,
  `SameIdentity` — safe-mode identity gates shared by both engines.

### WAV (`internal/wav`)
- `Parse(path) (*Info, error)` / `ParseReader` — RIFF/WAVE walk; accepts PCM
  (tag 1) and WAVE_FORMAT_EXTENSIBLE with PCM subformat; refuses anything else
  as `E_FORMAT_UNSUPPORTED`. `Info` carries channels, rate, bits, byte
  rate, block align, data offset/size, duration, INFO tags, `bext`, warnings.
  A streamed `0xFFFFFFFF` data size is clamped with a warning.
- `Info.MetadataNames`, `HasMetadata` — drives the `E_METADATA_DROP` gate.
- `Info.FmtChunk` / `Info.Header(payload)` — canonical header synthesis for
  the reclaim splice-resume (the original header is overwritten by then).

### Engines (`internal/engine/sliding`)
`New()` registers `sliding` (WAV→MP3). `safe`: LAME to a same-directory temp
(`--silent -V 2 - <tmp>`, file output keeps the Xing header) fed through a
counting reader, then fsync → ffprobe/decode-count verify → `CopyMetadata` →
snapshot re-check → `renameat2` → unlink source → dir fsync. `reclaim`:
journal `prepared`→`encoding`, producer/collector goroutines with `pread` at
the read cursor and `pwrite` at the write cursor under
`write + margin ≤ read` (plain `write ≤ read` after EOF), 8 MiB/1 s
`fdatasync` checkpoints, `verifying` (ffprobe + decode count on `[0,w)`)
→ `finalising` → ftruncate → rename → journal remove. Failures before the
first write remove the journal and report an untouched source; afterwards
`E_RECOVERY_AVAILABLE` (exit 4) or `E_INTERRUPTED` (exit 130) point at
`siphon recover`. `ResumeReclaim(store, journal, sink)` scans complete frames
in the written prefix, backs off one PCM frame, encodes the remainder through
a synthetic WAV header, and finishes with the same idempotent finalisation.

### Video stitch spike (`internal/lab/stitch`)
`BuildPlan(ctx, path, Options, *IOStats) (*Plan, error)` parses the MKV
header/Info/Tracks/Cues, picks one video + optional one audio track, derives
keyframes from Cues (coverage ≥90 % of Duration) or an `ffprobe -show_packets`
scan, snaps boundaries to the nearest keyframe within tolerance, and builds
synthetic pieces. `Keyframe` carries the cluster/keyframe-block/Timecode/cluster-end
offsets; `Segment.Pieces` are raw cluster ranges or wrapped Timecode+block
ranges; `Plan.ExtractBytes` is the exact one-pass byte count. `Run(ctx, plan,
outPath, Options) (*Report, error)` extracts tail-first (ffmpeg stream copy,
cached header + pieces on stdin, no seeking), analyzes each fragment, strips the
init, writes bodies backwards into a sparse `reserve = source×1.05 + 16 MiB`,
writes a leading `free` box, re-anchors every traf's tfdt and normalizes cts,
fsyncs/renames, and verifies. `Report.Checks` covers box chain, mfhd order, tfdt
monotonicity, cts non-negativity, packet counts, start anchor, A/V sync,
one-pass I/O, sampled decode, and optional full decode. `cmd/stitch` is the lab
harness; `RESULTS.md` records the findings and fault matrix.

### Recovery (`internal/recover`)
`Run(ctx, source, sink) (*core.Result, journalPath, error)` — finds the
journal next to the source, probes source-or-target for the inode, runs
`journal.VerifySource` (mode/phase-aware), and dispatches to the owning
engine. Sliding is wired; relocate/remux answer `E_RECOVERY_AVAILABLE` until
their resume lands.

### Imaging (`internal/imaging`)
- `Probe(ctx, path, format) (*Info, error)` — full decode at pre-flight;
  refuses animated APNG/WebP and corrupt files; reports width/height,
  transparency, metadata classes, warnings.
- `Decode(ctx, path, format, scratchDir) (image.Image, *Metadata, []string, error)`
  — applies EXIF orientation and normalises it to 1.
- `Encode(ctx, img, target, opts, meta, destPath, scratchDir)` — PNG/JPEG in
  process; WebP via an intermediate PNG and `cwebp -metadata all`
  (`-lossless -z 6` for the PNG→WebP pair, `-q` otherwise). JPEG metadata is
  re-injected into APP1/APP2; PNG into iCCP/eXIf/iTXt.
- `Verify(ctx, path, target, info, srcMeta, dropMetadata) ([]string, error)`
  — re-decodes, checks dimensions/alpha and class-by-class metadata
  (ICC byte-equal, EXIF present + orientation 1, XMP present).
- `Metadata{ICC, EXIF, XMP, Animated}`; EXIF kept as raw TIFF (prefix
  stripped). `Extract`, `AddMetadata`, `Classes`, `Empty`.
- `Orientation` / `SetOrientation` / `Orient` (all 8 EXIF orientations),
  `HasTransparency`, `Composite`, `ParseBackground` (names, `#rgb(a)`,
  `rgb()/rgba(a)`, `transparent`).

### Engines (`internal/engine/atomiccopy`)
`New()` registers an `Engine` (`siphon/atomiccopy`) with the image pipeline:
pre-flight re-checks (source snapshot vs. decode), temp creation + flock,
alpha policy (`--bg` compositing or `E_ALPHA_UNSUPPORTED`), encode, metadata
transplant, fsync, verify, `CopyMetadata`, `renameat2` (force/noreplace),
source unlink, dir fsync, stale-temp reap. No journal: a crash can only leave
a reapable temp. `internal/engines` is the composition root (blank-imported
by `internal/convert`).

### Convert (`internal/convert`)
`Inspect(ctx, job)` = gate only; `Run(ctx, job, sink)` = gate → engine
dispatch → uniform `core.Result` (also returned on error). Unknown engine →
`E_INTERNAL` naming the registered set.

### Preflight (`internal/preflight`)
`Run(job) (*core.Plan, error)` checks source validity, pair/mode, hardlinks,
permissions, target existence, same device, stale journal, magic, immutable
flag, root, dependencies, and a conservative free-space estimate. Images get
a full probe (dimensions, alpha gate, metadata classes) and the image
estimate (`source + max(2×source, decoded_raw×1.5) + 2 MiB`). WAV sources are
parsed from their own header (`internal/wav`, no ffprobe): >2 channels is
`E_FORMAT_UNSUPPORTED`, INFO/bext metadata is `E_METADATA_DROP` unless
`--drop-metadata`, and reclaim applies the R ≤ 0.90 gate (`E_INPLACE_INFEASIBLE`
otherwise). Estimates per spec §7.1: safe = source + duration × ceiling
bitrate / 8 × 1.05 (+1 MiB); reclaim = source × 1.01 (+1 MiB); ceiling is the
`--bitrate` value or 190 kbps. Codec, sparse, relocate gate, stitch probe,
and confirmation gates are still TODO (documented in §7).

### CLI
```text
siphon convert <src> --to <fmt> [flags]
siphon inspect <src> --to <fmt> [flags]
siphon recover <src>
siphon doctor
siphon version
```
Global: `--json`, `--allow-root`, `--allow-hardlinks`, `--timeout`.
Convert/inspect flags include `--bg`, `--quality`, `--drop-metadata`.
Progress sink selection: TTY stderr → `progress.Terminal`; otherwise
`progress.Plain` (or `progress.JSON` with `--json`); `--dry-run` aliases
`inspect`. Exit codes per spec §5.2.

---

## 6. Environment (this host)

- Fedora 44 KDE, btrfs (`/home`), 117 GB free.
- Go 1.26.9; `CGO_ENABLED=1` by default (core builds should force 0).
- ffmpeg/ffprobe 8.1.3; lame 3.100.
- **cwebp/dwebp missing** (`libwebp-tools`) → WebP code paths are
  unexercised end to end on this host; WebP unit tests skip without the
  tools, and parser tests use synthetic RIFF containers. Install
  `libwebp-tools` before trusting WebP pairs.
- `kdialog` and `kbuildsycoca6` present; **qdbus6 missing** → progress fallback.
- Qt6 Core/Quick present; Kirigami dev package not confirmed.

---

## 7. Next steps (ordered)

1. **RemuxEngine** (MKV→MP4 `safe`) — ffmpeg remux, track policy flags, verification.
2. **RelocateEngine** — planner, per-segment protocol, sparse reserve, free-box assembly, pre-flight stitch probe, exact resume.
3. **KDE integration** — `.desktop` files, `siphon-gui-kde`, `inspect`-driven warnings.
4. **GUI app** — toolkit decision; consume JSON/progress; queue + per-file progress.
5. **Hardening** — fault injection (`SIPHON_FAULT`), crash matrix, FS matrix, fuzz journal/box parsers.

Done: platform/journal (`7f43f93`), image engine `AtomicCopy` +
`internal/imaging` (`b842dbf`), audio engine `Sliding` + `internal/wav` +
`internal/recover` (`942d259`), phase-4 video stitch spike
(`internal/lab/stitch`, `RESULTS.md`; this checkpoint's working tree,
committed alongside). The spike gates `RelocateEngine`.
Phase 3 end-to-end checks on this host: safe/reclaim conversion; SIGKILL
mid-reclaim → `siphon recover` (1800 s file decodes to 1800.07 s); SIGINT →
exit 130, journal kept, recover finishes; crash windows before truncate,
after truncate, and after rename covered by tests.

---

## 8. Deviations / open questions

- `spec.md` header says "Supersedes v1.5" but describes v1.7; likely meant v1.6.
- Stitch-probe depth: §4.5.4.2 says "fully verify" sampled segments while
  §4.7 defaults to `sampled`. Proposal: probe uses `full` on the sampled set.
- `--verify` is listed relocate-only though the full gate applies to all modes.
- GUI toolkit not chosen. Constraint: engine stays subprocess/CLI-first so the
  GUI can be replaced without touching conversion code.
- `qdbus6` absent locally; wrapper must test for it.
- FFmpeg 8.1.3 is far newer than the 5.0 minimum; CI should pin one version.
- WebP decode reads RIFF metadata chunks directly; `dwebp` is run without
  `-metadata` (§4.5.1 step 1 says `dwebp -metadata all`). Rationale: Siphon
  already parses the RIFF container to detect animation, and the raw chunk
  bytes are what the transplant needs — the `-metadata` sidecar would be a
  second parse of the same data.
- PNG→WebP uses `cwebp -lossless -z 6` because `formats` marks the pair
  non-lossy; §4.5.1 step 3 shows only `-q <q>`. JPG→WebP still uses `-q`.
- `platform.ReapTemps` deletes same-stem `.tmp` files older than 1 h only
  when they are not flocked. Contract for every engine: hold `platform.LockFile`
  on a temp for as long as it is live. Violating it risks losing an active
  temp (bounded by the 1 h age check).
- **Reclaim margin**: §4.4.1's literal `MARGIN = max(1 MiB, 4×1441, 2×8 MiB)
  = 16 MiB` invariant (`write + margin ≤ read`) can never hold for sources
  smaller than ~16 MiB plus output, so it would stall forever. Implementation:
  enforce the margin while the encoder is still being fed, then switch to the
  plain `write ≤ read` invariant at input EOF and flush the endgame backlog.
  The crash-safety property is unchanged; only the in-flight gap behind the
  read cursor is bounded by the margin.
- **Reclaim rename**: §4.4.1's step list ends at truncate + journal unlink, but
  "output in its place" requires moving the truncated inode from `foo.wav` to
  `foo.mp3`. Protocol: `finalising` checkpoint → ftruncate → fdatasync →
  `renameat2` → journal remove. Recovery accepts the inode at either path and
  `VerifySource` accepts both the original and the write-cursor size during
  `finalising`.
- **Reclaim pipe output has no Xing header** (LAME only writes it for file
  output). Deliberate: the frame scanner is authoritative for resume, and
  `verifyMP3` measures duration by full decode rather than the container
  estimate. `safe` output does carry Xing and decodes to source length.
- **WAV metadata** (LIST/INFO, `bext`) is not carried into MP3 yet: the file
  is refused with `E_METADATA_DROP` unless `--drop-metadata`, matching the
  spec's never-silent rule. A future tag transplant can lift this.
- WebP end-to-end paths are untested on this host (`libwebp-tools` missing);
  `cwebp` dropping metadata surfaces as `E_METADATA_DROP` per §4.2.
- **Video spike scope**: MKV→MP4, one video + one audio track, sized clusters
  only (Cues and scan both require them), one boundary per cluster. Only
  H.264/AAC were exercised; the stream-copy design is codec-agnostic but other
  codecs need fixtures. The spike keeps the reserve-sized leading `free` box;
  phase 5 must decide whether to keep or compact it.
- **Synthetic cluster boundaries** are mandatory for clusters that start with
  the previous GOP's trailing B-frames. `-copyinkf` is not a fix: the muxer
  gives those frames 1-tick durations and shifts the boundary by one frame
  interval. Non-cluster top-level elements between clusters are copied
  verbatim; a mid-stream Cues would need a split-and-skip pass before the
  relocate engine is production-grade.
- **Sampled verification** decodes from each fragment's keyframe, so it cannot
  validate a cross-fragment reference chain; the full assembled decode is the
  authoritative check (and what the fault suite uses for `avcC`).
