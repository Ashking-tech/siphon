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
| Preflight gate | partial (checks 1–7, 14–18, 20 + space + image probe) |
| CLI skeleton | `convert`, `inspect`, `recover`, `doctor`, `version` |
| Imaging layer (decode, orientation, ICC/EXIF/XMP, WebP, verify) | implemented + tested |
| Engines | AtomicCopy (images) implemented + tested; Sliding / Relocate / Remux not yet |
| GUI | not yet (KDE wrapper planned; GUI toolkit decision pending) |

`go build ./...` and `go test ./...` pass. `siphon doctor` and `siphon inspect`
work. `siphon convert` converts the image pairs end to end; audio/video pairs
still refuse with `E_INTERNAL: engine ... not implemented yet` after a
successful pre-flight. `siphon recover` validates and summarises a journal,
then exits 4 until the recovery driver lands.

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
internal/preflight/             ordered refusal gate, plan + space estimate + image probe (+ tests)
internal/imaging/               image decode/encode, orientation, ICC/EXIF/XMP, WebP, verify (+ tests)
internal/engine/                Engine interface + registry
internal/engine/atomiccopy/     image engine: temp, alpha policy, verify, atomic rename (+ tests)
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
- `CreateTemp(dir, prefix)`; `Fsync`; `FsyncDir`.
- `RenameNoReplace`, `RenameReplace`.
- `SameDevice`, `FreeSpace`, `SparseProbe`.
- `OpenSource` (`O_RDWR|O_NOFOLLOW`), `ValidateMagic`.
- `CopyMetadata` (mode/uid/gid/xattrs), `HasImmutable`.

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
`Run(job) (*core.Plan, error)` currently checks source validity, pair/mode,
hardlinks, permissions, target existence, same device, stale journal, magic,
immutable flag, root, dependencies, and a conservative free-space estimate;
images additionally get a full probe (dimensions, alpha gate, metadata
classes) and the image estimate
(`source + max(2×source, decoded_raw×1.5) + 2 MiB`). Codec, metadata,
sparse, relocate gate, stitch probe, and confirmation are still TODO
(documented in context §7).

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

1. **Audio engine** (`Sliding`) — WAV validation, `safe` via lame, `reclaim`
   with forward-cursor invariant; splice-resume.
2. **Video stitch spike** (`internal/lab/stitch`) — MKV `Cues` plan, cached-header byte ranges, tail-first assembly, fault sensitivity. This gates `RelocateEngine`.
3. **RemuxEngine** (MKV→MP4 `safe`) — ffmpeg remux, track policy flags, verification.
4. **RelocateEngine** — planner, per-segment protocol, sparse reserve, free-box assembly, pre-flight stitch probe, exact resume.
5. **KDE integration** — `.desktop` files, `siphon-gui-kde`, `inspect`-driven warnings.
6. **GUI app** — toolkit decision; consume JSON/progress; queue + per-file progress.
7. **Hardening** — fault injection (`SIPHON_FAULT`), crash matrix, FS matrix, fuzz journal/box parsers.

Done: platform/journal (`7f43f93`), image engine `AtomicCopy` +
`internal/imaging` (this checkpoint's working tree, committed alongside).

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
- WebP end-to-end paths are untested on this host (`libwebp-tools` missing);
  `cwebp` dropping metadata surfaces as `E_METADATA_DROP` per §4.2.
