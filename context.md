# Siphon — Working Context (checkpoint)

> Living document. Update this file whenever a module, pipeline, decision, or
> command lands, then commit. It is the resume point after context loss.
> Read `spec.md` (v1.7) for the full contract; this file records **what exists**.

**Last updated:** 2026-10-25
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
| Preflight gate | ordered checks incl. WAV/image/MKV, MKV track policy, relocate 3-point config + 3-segment gate + space; stitch plan |
| CLI | `convert`, `inspect`, `recover`, `doctor`, `version` |
| Imaging layer (decode, orientation, ICC/EXIF/XMP, WebP, verify) | implemented + tested |
| Video stitch core (`internal/stitch`) | production planner/extractor/assembler/verifier; standalone `cmd/stitch` harness + `RESULTS.md` + fault suite |
| Engines | AtomicCopy (images), Sliding (WAV→MP3 safe + reclaim), Remux (MKV→MP4 safe), Relocate (MKV→MP4 fragmented, journaled) — all implemented + tested |
| WAV parser (`internal/wav`) | implemented + tested (PCM + extensible, INFO/bext detection, header synth) |
| Recovery driver (`internal/recover`) | sliding splice-resume, relocate exact resume, completed-rename cleanup; tested |
| GUI | not yet (KDE wrapper planned; GUI toolkit decision pending) |

`go build ./...`, `go vet ./...`, and `go test ./...` pass; `make build` keeps
`CGO_ENABLED=0`. `siphon doctor` and `siphon inspect` work. `siphon convert`
converts image pairs, WAV→MP3 (safe/reclaim), and MKV→MP4 (`safe` remux or the
default `relocate`). A killed reclaim or relocate job is finished by
`siphon recover <file>`; relocate resume is exact (no seam) and handles a
crash before/after each truncate and after the output rename.

The production stitch path passes on the 12 s H.264/AAC fixture: 4 segments,
360/360 video and 518/518 audio packets, output duration 12.038299 s, exact
one-pass extraction, full gate checks (stream identity, duration, size,
counts, anchors, sync, one-pass) and full decode clean. Manual crash tests at
segment-3-done and after the finish checkpoint both recovered to a clean
360/518-packet output. `internal/stitch/RESULTS.md` records the design and
fault-sensitivity matrix; `internal/engine/relocate/` is the production driver.

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
| Relocate plan | Cues first when they cover ≥90 % of Duration; otherwise `ffprobe -show_packets`; a Cues read/parse error also falls back | spec §4.5.4.1; `plan_source` is journaled and shown by `inspect` |
| Relocate boundaries | target every `--segment-duration`, snapped to a keyframe on the single retained video track within `--segment-tolerance`; ≥3 segments or `E_INPLACE_INFEASIBLE` | one video + one optional audio only; multi-stream and A/V drops are refused |
| Relocate gate | remux-only, estimated output ≤1.03× source, 3-point constant track config, reserve = source×1.05 + 16 MiB | spec §4.5.4.2, §7.1; `--transcode-audio` is `E_INPLACE_INFEASIBLE` |
| Relocate tail cache | before truncating segment i, cache `[TCOff,TCEnd)` + `[KeyPos, …)` for segment i−1 in the journal (`TailFrom`/`TailTo`/`TailPrefix`) | the next cluster's leading B-frames/audio prefix are needed by the previous segment and would otherwise be destroyed |
| Relocate truncate protocol | body → `fdatasync(temp)` → checkpoint `truncating` + `TruncateFrom` + `TailCursor=src_off` → `ftruncate(source)` → `fdatasync` → checkpoint done + mtime | `TruncateFrom` distinguishes the pre-/post-truncate crash windows; `reconcileTruncate` completes either |
| Relocate resume | done suffix is the trailing run of `done`; re-plan `[0,tail_cursor)` with `EndNS` = suffix's first keyframe; reattach the old tail prefix to the new last pending segment | exact continuation, no re-encode/seam; all-done resumes reuse the durable init/anchors |
| Relocate finalise | verify → checkpoint `finalising` → `CopyMetadata` → fsync → rename → truncate/unlink source → journal remove | post-rename errors have `Untouched(false)` and no recover hint |
| Relocate output identity | journal `TempDev`/`TempIno`; recovery accepts a finalising target only if its inode and reserve size match | size alone could mistake a pre-existing target for our output and delete the source |
| Relocate source duration | journal the max retained A/V stream duration before truncation | a delayed audio track can extend past Matroska Info duration; the full gate compares against the real source timeline |
| Relocate codec policy | only copyable single video + optional audio; `--drop-incompatible` may drop subtitles/metadata, never A/V | extraction uses a fixed `-map 0:v -map 0:a?`, so a dropped A/V would still be muxed; use `--mode safe` |
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
internal/engine/remux/          MKV→MP4 safe remux: track map, ffmpeg, metadata, verify (+ tests)
internal/engine/relocate/       MKV→MP4 fragmented relocate: journal, tail-first segments, resume, finalise (+ tests)
internal/stitch/                production planner/extractor/assembler/verifier; `cmd/stitch` lab harness (+ RESULTS.md, fault suite)
internal/recover/               recovery driver: journal discovery, inode probe, empty/finalising cleanup, engine dispatch (+ tests)
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
- Journal fields match spec §4.6.2; nullable `out_off`/`out_len`. Relocate
  additions: `Segment.FirstPTSNS`, `TailFrom`/`TailTo`/`TailPrefix`,
  `Cursors.TruncateFrom`, `Init`, `PadNS`, `ExpectedCounts`, `CountsExact`,
  `SourceStarts`, `SourceDurationNS`, `Job.TempDev`/`TempIno`.
- `VerifySource(j, path)` checks dev/ino and the mode-specific size/mtime;
  relocate accepts either side of a pending truncate (`TruncateFrom` pre,
  `TailCursor` post). `OutputMatches(j, path)` proves a finalising target is
  the inode the journal created (size + dev/ino), not merely the reserve size.
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

### Video stitch core (`internal/stitch`)
`BuildPlan(ctx, path, Options, *IOStats) (*Plan, error)` parses the MKV
header/Info/Tracks/Cues, picks one video + optional one audio track, derives
keyframes from Cues (coverage ≥90 % of Duration; parse/read errors fall back)
or an `ffprobe -show_packets` scan, snaps boundaries to the nearest keyframe
within tolerance, and builds synthetic pieces. `Options.EndNS` bounds a resume
re-plan; `Options.ForceScan` is used by the lab harness. `Segment.Pieces` are
raw cluster ranges or wrapped Timecode+block ranges; `Plan.ExtractBytes` is the
exact one-pass byte count; `Plan.SourceDurationNS` carries the pre-truncation
source duration for the full gate.

`Session` owns one journaled run: `Prepare(i, frames)` extracts through
ffmpeg's stdin (cached header + pieces, no seeking), checks the source first
video block against `FirstPTSNS`, analyzes the fragment, strips the init, and
runs the sampled decode. `CacheTail(i)` caches the next cluster's prefix for
the previous segment; `Place(tmp, body)` writes backwards; `Finish` writes the
init and leading `free` box, patches every traf's tfdt/cts; `Verify` runs the
full gate. `Report` supports source count/start overrides and tolerant counts
for recovery. `Run(ctx, plan, outPath, Options)` remains the standalone lab
harness. `Report.Checks` covers box chain, stream identity, duration, size,
mfhd order, tfdt monotonicity, cts non-negativity, packet counts, start anchor,
A/V sync, one-pass I/O, sampled decode, and optional full decode. `cmd/stitch`
and `RESULTS.md` record the design and fault matrix.

### Engines (`internal/engine/relocate`)
`New()` registers `relocate` (MKV→fragmented MP4). `Convert` builds the plan,
enforces ≥3 segments and the A/V track policy, snapshots/locks the source, runs
the pre-flight stitch probe (unless `--skip-probe`), captures exact packet
counts + source starts + source duration, creates the reserved temp and
journal, then runs `runSegments` tail-first: `Prepare` → `CacheTail` → `Place`
→ `fdatasync(temp)` → checkpoint `truncating` → `ftruncate(source)` →
`fdatasync(source)` → checkpoint `done`. `finish` assembles, verifies, checks
`finalising`, copies metadata, fsyncs, renames (noreplace/force), truncates and
unlinks the source, then removes the journal. `Resume` reconciles a pending
truncate, re-plans `[0,tail_cursor)` with `EndNS`, reattaches the cached tail,
skips the done suffix, and uses the journal's init/anchors; a finalising journal
whose temp is gone is accepted only when `journal.OutputMatches` proves the
target inode. `checkRelocateTrackPlan` refuses A/V drops; the fixed extraction
map cannot honor `--drop-incompatible` for video/audio. `internal/recover`
dispatches to `Resume`.

### Recovery (`internal/recover`)
`Run(ctx, source, sink) (*core.Result, journalPath, error)` — finds the
journal next to the source, drops an empty create-only journal, handles a
relocate finalising target after rename (inode proof), probes source-or-target
for the inode, runs `journal.VerifySource` (mode/phase-aware), and dispatches
to the owning engine. Sliding splice-resume and relocate exact resume are
wired; remux safe has no resume (delete the temp and re-run).

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
otherwise). MKV sources are probed by ffprobe: the shared track policy
(`internal/media/trackplan.go`) refuses incompatible codecs/subtitles and
reports attachments/chapters/tags; `--drop-metadata` accepts the loss. For
relocate, the gate also requires a single video + optional single audio,
refuses dropping an A/V stream (the fixed extraction map cannot honor it),
runs the 3-point head/mid/tail constant-config probe, builds the Cues/scan
stitch plan (≥3 segments), and checks the 1.05×+16 MiB reserve. The expensive
stitch probe itself runs in the engine before any truncation (check 19,
skipped by `--skip-probe`). Estimates per spec §7.1: MKV safe = source×1.03 +
streams×1.1/8 + 4 MiB; relocate = one segment + 16 MiB; WAV safe = source +
duration × ceiling bitrate / 8 × 1.05 (+1 MiB); WAV reclaim = source × 1.01
(+1 MiB); ceiling is `--bitrate` or 190 kbps. Confirmation is handled by the
CLI for non-TTY reclaim.

### CLI
```text
siphon convert <src> --to <fmt> [flags]
siphon inspect <src> --to <fmt> [flags]
siphon recover <src>
siphon doctor
siphon version
```
Global: `--json`, `--allow-root`, `--allow-hardlinks`, `--timeout`.
Convert/inspect flags include `--to`, `--mode`, `--bg`, `--quality`,
`--bitrate`, `--drop-metadata`, `--drop-incompatible`, `--transcode-audio`,
`--allow-nonstandard`, `--segment-duration`, `--segment-tolerance`, `--verify`,
`--verify-frames`, `--probe-segments`, `--skip-probe`, `--force`, `--yes`.
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

1. **KDE integration** — `.desktop` files, `siphon-gui-kde`, `inspect`-driven warnings.
2. **GUI app** — toolkit decision; consume JSON/progress; queue + per-file progress.
3. **Hardening** — fault injection (`SIPHON_FAULT`), crash matrix, FS matrix, fuzz journal/box parsers, and the open verification items below.

Done: platform/journal (`7f43f93`), image engine `AtomicCopy` +
`internal/imaging` (`b842dbf`), audio engine `Sliding` + `internal/wav` +
`internal/recover` (`942d259`), phase-4 video stitch spike (`1087aec`),
safe `RemuxEngine` (`13d0ed2`), and phase-5 `RelocateEngine` (committed with
this checkpoint). `internal/stitch` is now the shared production core, not a
lab-only package; `cmd/stitch` remains the harness.

Phase 3 end-to-end checks on this host: safe/reclaim conversion; SIGKILL
mid-reclaim → `siphon recover` (1800 s file decodes to 1800.07 s); SIGINT →
exit 130, journal kept, recover finishes; crash windows before truncate,
after truncate, and after rename covered by tests.

Phase 5 end-to-end checks on this host: fresh 12 s MKV relocation (4 segments,
360/518 packets, 12.038299 s, full decode clean); manual crash after segment 3
done and after the finish checkpoint both recovered; `go test ./...` covers
fresh E2E, metadata refusal/drop, interrupt+resume, truncate-window
reconciliation, finalising target inode proof (empty/missing/nonzero source),
empty-journal cleanup, and preflight relocate refusals (short, multi-stream,
config change, dropped A/V).

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
- **Video scope**: MKV→MP4, one video + one optional audio track, sized
  clusters only (Cues and scan both require them), one boundary per cluster.
  Only H.264/AAC were exercised; the stream-copy design is codec-agnostic but
  other codecs need fixtures. The output keeps the reserve-sized leading
  `free` box (spec §4.5.4.3 step 11), so file size is reserve-sized rather
  than tightly compacted.
- **Synthetic cluster boundaries** are mandatory for clusters that start with
  the previous GOP's trailing B-frames. `-copyinkf` is not a fix: the muxer
  gives those frames 1-tick durations and shifts the boundary by one frame
  interval. Non-cluster top-level elements between clusters are copied
  verbatim; a mid-stream Cues would need a split-and-skip pass before the
  relocate engine is production-grade.
- **Sampled verification** decodes from each fragment's keyframe, so it cannot
  validate a cross-fragment reference chain; the full assembled decode is the
  authoritative check (and what the fault suite uses for `avcC`).
- **Full gate duration**: measured as the longest retained video/audio stream
  duration, not Matroska Info duration. A delayed audio track legitimately
  extends past the video duration (the `TestDelayedAudioSync` fixture), and the
  output container reports that longer timeline. The engine journals this value
  before truncation.
- **Full gate stream identity**: skipped only on an all-done resume, where the
  source track table is gone; packet counts, anchors, timeline, and decode
  still run. A normal/fresh or partially-pending resume has the header and
  checks codecs/track count.
- **`--skip-probe` and metadata**: the init-tag metadata gate is part of the
  pre-flight stitch probe, so `--skip-probe` also skips it (documented as
  unsafe). A post-assembly metadata refusal has no recovery override; the
  pre-truncation check is what makes `--drop-metadata` meaningful.
- **Relocate `--drop-incompatible`**: may drop subtitles/data/attachments but
  never a video/audio stream. The fixed `-map 0:v -map 0:a?` extraction would
  still mux it; preflight and the engine both refuse with `E_INPLACE_INFEASIBLE`
  and point at `--mode safe`.
- **Completed-rename proof**: a finalising target is accepted only if its
  inode equals the journal's `TempDev`/`TempIno` and its size equals the
  reserve. This closes the false-positive where an unrelated target of the
  same size could cause the source to be deleted.
- **Empty journal**: a crash between `journal.Create` and the first `Save`
  leaves a create-only journal. `recover` drops it and reports the source
  untouched; a temp from that window is left to the 1 h stale-temp reaper.
- **Cues fallback**: a Cues read/parse error falls back to the packet scan, not
  a hard failure; `plan_source` records the path actually used. `ForceScan` is
  exposed only by the `cmd/stitch` lab harness, not the CLI.
- **Per-segment gate**: `Prepare` checks the source first video block against
  the planned keyframe (±1 frame) before truncation, plus box parsing,
  sampled decode, and init strip. Full structural mdat/trun-sum and per-track
  identity/coverage checks remain full-gate work (and are open hardening items).
