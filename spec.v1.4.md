Done. "Siphon" is a great name for this—it perfectly captures the idea of drawing the data out of the source container and leaving the original drained and replaced.

Here is the completely updated, production-ready specification with the new name.

---

# Siphon — Production Specification (Linux MVP)

**Version:** 1.4 (Hardened)
**Status:** Frozen for v0.1 implementation
**Target OS:** Linux (KDE Plasma primary)
**Audience:** Implementing engineers, reviewers, and future maintainers

---

## 1. Executive Summary

Siphon is a single-binary CLI and OS-integrated conversion tool that transforms media files **in place** — the source file is consumed and replaced by the output file of the same stem, in the same directory, with no persistent intermediate copy and no `2 × size` peak disk requirement on supported filesystems.

The product thesis is narrow and load-bearing:

> **Converting a large file should not require twice the disk space, a second copy on disk at the end, or any silent loss of quality.**

Everything in this document derives from that thesis. When a design decision could go two ways, the tie-breaker is always: *which option makes the above statement most true, and which option makes failures safest?*

---

## 2. Design Principles

**P1 — The source is sacred until the output is proven.**
No byte of the source is deleted, truncated, or hole-punched until the corresponding byte of the output has been written, `fsync`ed, and journalled.

**P2 — Refusal is a first-class outcome.**
Every operation has a pre-flight gate. If the gate cannot prove that the conversion will complete safely, the tool refuses and exits non-zero.

**P3 — No silent quality loss, no silent space cost.**
Lossy decisions require an explicit flag or UI confirmation.

**P4 — Same-directory temp, atomic rename.**
Cross-filesystem `rename` is detected at pre-flight and refused.

**P5 — Every destructive operation is journalled and resumable.**
A crash leaves a state from which `siphon recover` can complete the job.

**P6 — No network, no telemetry, no hidden state.**
All state lives in the source directory.

**P7 — The user's file is never the test subject.**
Every path is covered by automated round-trip tests.

**P8 — Leverage existing, battle-tested tools (The Orchestrator Philosophy).**
Siphon does not write media encoders, decoders, or muxers from scratch. Instead, it orchestrates highly mature CLI tools (`ffmpeg`, `lame`, `ffprobe`) as subprocesses. This drastically reduces the codebase surface area, eliminates entire classes of memory-safety bugs, and allows the engineering team to focus exclusively on what makes Siphon unique: **atomic, crash-safe, in-place file manipulation via syscalls.**

---

## 3. Scope & Supported Formats (v0.1)

### 3.1 Format Pairs

| Source | Target | Engine | Space guarantee | Lossy? |
|---|---|---|---|---|
| `.wav` | `.mp3` | Reverse | ≤ 1 × output | Yes (audio) |
| `.mkv` | `.mp4` | PunchThrough (remux only) | ≈ 1 × output on hole-punch FS | No |
| `.png` | `.jpg` | AtomicCopy | 2 × source (small files) | Yes (alpha) |
| `.jpg` | `.png` | AtomicCopy | 2 × source | No, size inflates |
| `.png` | `.webp` | AtomicCopy | 2 × source | No (lossless by default) |
| `.webp` | `.png` | AtomicCopy | 2 × source | No, size inflates |
| `.jpg` | `.webp` | AtomicCopy | 2 × source | Yes (quality) |
| `.webp` | `.jpg` | AtomicCopy | 2 × source | Yes (quality, alpha) |

### 3.2 Explicit Non-Goals (v0.1)

- Video transcoding (VP9→H.264, etc.). MKV→MP4 is **remux only**.
- Audio transcoding to anything other than AAC (via `--transcode-audio`).
- Subtitle transcoding. Dropped via `--drop-incompatible` or refused.
- FLAC→MP3 (planned for v0.2).
- Multi-channel (>2) WAV.
- Windows and macOS support.

---

## 4. Technology Stack & Architecture

### 4.1 Language Choice: Go (Golang)

Go is the chosen implementation language due to its fast compilation, easy static binaries (`CGO_ENABLED=0`), excellent syscall access via `golang.org/x/sys/unix`, and native `os/exec` for subprocess management.

### 4.2 Dependency & Library Strategy

**External Binaries (Managed as Subprocesses):**
- **`ffmpeg`**: Handles all MKV demuxing, MP4 muxing, and WAV decoding.
- **`ffprobe`**: Handles all MKV track inspection during pre-flight.
- **`lame`**: Handles WAV PCM to MP3 encoding.
- **`kdialog`**: Handles native KDE error popups and progress bars.

**Go Libraries (Internal Logic):**
- **`github.com/spf13/cobra`**: CLI parsing.
- **`golang.org/x/sys/unix`**: Syscalls (`Fallocate`, `Flock`, `Fsync`).
- **`github.com/disintegration/imaging`**: Pure-Go image processing and conversion.
- **`golang.org/x/image/webp`**: WebP decoding.
- **`github.com/rwcarlsen/goexif/exif`**: EXIF metadata preservation.
- **`schollz/progressbar/v3`**: Terminal progress bars.

### 4.3 High-Level Architecture

```text
┌─────────────────────────────────────────────────────────────┐
│  Interface Layer                                            │
│  ┌──────────────┐   ┌──────────────────────────────────┐    │
│  │ CLI (cobra)  │   │ KDE ServiceMenu (siphon.desktop) │    │
│  └──────┬───────┘   └──────────────┬───────────────────┘    │
└─────────┼──────────────────────────┼────────────────────────┘
          │                          │
          ▼                          ▼
┌─────────────────────────────────────────────────────────────┐
│  Orchestrator                                               │
│  - parses flags into a Job                                  │
│  - runs preflight() → Plan | Refusal                        │
│  - selects engine                                           │
│  - drives engine, handles signals, owns journal lifecycle   │
└─────────┬───────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│  Preflight Gate (Uses ffprobe, O_NOFOLLOW, and FS probes)   │
└─────────┬───────────────────────────────────────────────────┘
          │
          ▼
┌─────────────────────────────────────────────────────────────┐
│  Engine Layer                                               │
│  ┌────────────────────┐  ┌─────────────────┐  ┌───────────┐ │
│  │ AtomicCopyEngine   │  │ ReverseEngine   │  │ PunchThrough│ │
│  └─────────┬──────────┘  └────────┬────────┘  └─────┬─────┘ │
└────────────┼──────────────────────┼─────────────────┼───────┘
             │                      │                 │
             ▼                      ▼                 ▼
┌─────────────────────────────────────────────────────────────┐
│  Platform Layer (Linux)                                     │
│  - fs: same-dir temp, fsync, rename, unlink                 │
│  - hole-punch: unix.Fallocate(FALLOC_FL_PUNCH_HOLE)         │
│  - lock: unix.Flock(LOCK_EX | LOCK_NB)                      │
└─────────────────────────────────────────────────────────────┘
```

### 4.4 The Engine Subprocess Strategy

**PunchThroughEngine (MKV→MP4):**
Go opens source, creates temp file. Launches `ffmpeg -i pipe:0 -c copy -f mp4 pipe:1`. Go reads 10MB blocks from source, writes to stdin, reads MP4 bytes from stdout into temp file. After block is written, Go punches a hole in the source. Go journals progress. At EOF: fsync, rename, unlink.

**ReverseEngine (WAV→MP3):**
Go opens source, creates temp file. Seeks to end of WAV. Reads last 10 seconds of PCM, pipes to `lame`. Appends MP3 frames to temp, truncates source, journals. At start-of-file: reverses MP3 frames in-place.

**AtomicCopyEngine (Images):**
Go uses `imaging` library to decode, applies alpha compositing (`--bg`), encodes, writes to temp. fsync, rename, unlink.

### 4.5 The Pre-Flight Gate

Runs in order. On refusal, prints structured error and exits. **No files are touched.**

| # | Check | Refusal code |
|---|---|---|
| 1 | Source exists, is regular, not symlink (Checked again with `O_NOFOLLOW` on open) | `E_SOURCE_INVALID` |
| 2 | Format pair supported | `E_FORMAT_UNSUPPORTED` |
| 3 | Codec compatibility (Via `ffprobe` JSON output) | `E_CODEC_INCOMPATIBLE` |
| 4 | FS supports hole-punch (for MKV) | `E_FS_NO_PUNCH` |
| 5 | Target doesn't exist, or `--force` | `E_TARGET_EXISTS` |
| 6 | Free-space sanity | `E_NO_SPACE` |
| 7 | Source not locked by another writer | `E_SOURCE_LOCKED` |
| 8 | No stale journal | `E_STALE_JOURNAL` |
| 9 | Source header valid (Magic bytes) | `E_SOURCE_CORRUPT` |
| 10 | Dependencies installed and version >= minimum | `E_DEPENDENCY` |
| 11 | Not running as root without `--allow-root` | `E_ROOT_DENIED` |

---

## 5. Behavior Specification

### 5.1 CLI Surface

```bash
siphon convert <source> --to <format> [flags]
siphon inspect <source> [--to <format>] # Dry run
siphon recover <source>                 # Resume crashed job
```

**Flags:**
- `--to <fmt>` (required for convert)
- `--force` (overwrite existing target)
- `--drop-incompatible` (MKV: skip incompatible tracks)
- `--transcode-audio` (MKV: encode FLAC/DTS to AAC)
- `--bg <color>` (Images: background color for alpha stripping)
- `--bitrate <kbps>` (WAV: CBR bitrate for lame)
- `--allow-root` (Required to run as UID 0)
- `--dry-run` (alias for inspect)

### 5.2 KDE Context Menu Integration

Installed to `~/.local/share/kio/servicemenus/` (Plasma 6) and `~/.local/share/kservices5/ServiceMenus/` (Plasma 5). Three files (`-video`, `-audio`, `-image`) filter the menu natively.

The `.desktop` files call `siphon-gui-kde "$1" "$2"`.

### 5.3 The `siphon-gui-kde` Wrapper Script

1. Runs `siphon inspect "$1" --to "$2"`. If fail, `kdialog --error`.
2. If confirmation needed, `kdialog --warningcontinuecancel`.
3. Runs `siphon convert "$1" --to "$2"` piping progress to `kdialog --progressbar`.
4. `kdialog --passivepopup "Converted successfully"`.

### 5.4 Format-Specific Rules

- **MKV → MP4:** Remux only. Attachments (fonts) dropped.
- **WAV → MP3:** >2 channels refused. Default LAME VBR `-V 2`.
- **Images:** Alpha stripping requires `--bg <color>`. EXIF/ICC preserved.

---

## 6. Operational Edge Cases & Hardening

Because Siphon deletes the original source, the engineering implementation must account for adversarial and failure conditions beyond normal happy-path conversions.

### 6.1 Subprocess Lifecycle & Timeouts
**Risk:** A corrupt media file causes `ffmpeg` or `lame` to enter an infinite loop, hanging the terminal and leaving `.tmp` files indefinitely.
**Mitigation:** All `exec.Command` calls must use `exec.CommandContext`. The context must have a configurable timeout (default: 1 hour) and must be tied to the `SIGINT`/`SIGTERM` context. If the subprocess exceeds the timeout or is cancelled, Go sends `SIGKILL` to the subprocess, cleans up the `.tmp` file, and exits gracefully.

### 6.2 Disk Space Exhaustion (`ENOSPC`)
**Risk:** Disk fills up mid-conversion due to external factors. 
**Mitigation:** All `Write()` operations to the temp file must be wrapped to catch `ENOSPC`. If caught, the engine aborts the subprocess, leaves the journal intact, deletes the temp file, and exits with a clear error instructing the user to free space and run `siphon recover`. The source is never touched.

### 6.3 Security: TOCTOU & Symlink Protection
**Risk:** Time-of-Check to Time-of-Use (TOCTOU) attack where a symlink is swapped in after pre-flight.
**Mitigation:** When opening the source file for writing/truncation, use `os.OpenFile(path, os.O_RDWR|os.O_NOFOLLOW, 0)`. This syscall guarantees the kernel will fail immediately if the path is a symlink, preventing traversal attacks.

### 6.4 Privileged Execution (Root Detection)
**Risk:** User runs `sudo siphon convert /boot/vmlinuz --to mp4` and corrupts the system.
**Mitigation:** Check `os.Geteuid() == 0` at startup. If true, refuse to run unless the `--allow-root` flag is explicitly passed. Print a severe warning.

### 6.5 File Permissions & Ownership
**Risk:** Converting a file in a shared directory, or a file owned by another user.
**Mitigation:** Pre-flight checks write access to both the *directory* (to create temp and rename) and the *source file* (to truncate/punch). If running under `sudo`, the resulting output file must inherit the original file's `UID` and `GID` via `os.Chown`, never root's.

### 6.6 Shell Quoting & Weird Filenames
**Risk:** Filenames with spaces, quotes, or emojis break the KDE wrapper script.
**Mitigation:** The `siphon-gui-kde` script must double-quote all variables (`"$1"`, `"$2"`). The Go code must handle non-UTF8 filenames gracefully (using raw bytes if necessary) and strictly parse `ffprobe` JSON.

### 6.7 Source Validation (Magic Bytes)
**Risk:** A `.wav` file is actually a renamed `.mp3`, causing the `ReverseEngine` to read garbage from the tail.
**Mitigation:** Pre-flight must read the first 12 bytes of the source. For WAV, verify the `RIFF....WAVE` header. For MKV, verify the EBML header (`0x1A45DFA3`). If the magic bytes mismatch the extension, refuse with `E_SOURCE_CORRUPT`.

### 6.8 Dependency Versioning
**Risk:** User has `ffmpeg` v3.4 (Ubuntu 18.04), which lacks modern MP4 muxing flags.
**Mitigation:** Pre-flight runs `ffmpeg -version` and `lame --version`, parses the output, and compares against the minimum tested versions (FFmpeg >= 5.0, LAME >= 3.100). If outdated, refuse with `E_DEPENDENCY_OUTDATED`.

### 6.9 Signal Handling & Graceful Shutdown
**Risk:** User hits `Ctrl+C`, leaving the terminal in a broken state and temp files orphaned.
**Mitigation:** Use `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`. The main loop catches the cancellation, kills the subprocess, restores terminal cursor state, prints `"Interrupted. Source safe. Run 'siphon recover <file>' to resume."`, and exits with code `130`.

---

## 7. Testing Strategy (Linux)

| Layer | What it covers | Runs in |
|---|---|---|
| Unit | Preflight logic, journal parsing, hole-punch probing, magic bytes | Every commit |
| Golden Corpus | Every format pair, every refusal reason, every flag combination | CI (Ubuntu 24.04, Fedora) |
| Crash Injection | `SIGKILL` at arbitrary offsets, followed by `recover` | Nightly |
| Adversarial | Symlinks, read-only dirs, fake headers, root execution | Nightly |
| Filesystem Matrix | ext4, XFS, Btrfs (success) and exFAT (refusal) | Nightly, via VMs |
| KDE Integration | Automated test using `kbuildsycoca` and `qdbus` | Release only |

**The Golden Corpus Invariant:** For every test, if the CLI refuses, the source `(size, mtime, sha256)` must be identical. If it succeeds, the source is gone, and the target matches a reference conversion.

---

## 8. Build Order & Deployment

### 8.1 Build Phases

1. **Skeleton (1 week):** CLI parser, Preflight gate, journal format, magic bytes validation, root detection.
2. **Image Engine (1 week):** AtomicCopyEngine, `imaging` integration, alpha refusal.
3. **WAV→MP3 Engine (2 weeks):** ReverseEngine, `lame` piping, timeout context.
4. **MKV→MP4 Engine (3 weeks):** PunchThroughEngine, `ffmpeg` piping, `ffprobe` parsing, ENOSPC handling.
5. **KDE Integration (1 week):** `.desktop` files, `kdialog` wrapper with strict quoting.
6. **Hardening (1 week):** Fuzz testing, crash injection, symlink/TOCTOU tests.

### 8.2 Distribution

- **Static Binary:** `CGO_ENABLED=0 go build -ldflags="-s -w"`.
- **Dependencies:** `ffmpeg`, `ffprobe`, and `lame` must be installed.
- **Packaging:** `.deb`/`.rpm` declaring `ffmpeg` and `lame` dependencies. Places the binary in `/usr/local/bin/` and triggers `kbuildsycoca6`.

---

## 9. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| FFmpeg/LAME missing or outdated | Pre-flight `exec.LookPath` and `-version` parsing. |
| Data loss on crash | Journal + `recover` command. CI runs `SIGKILL` tests. |
| Malicious symlink swap | `os.O_NOFOLLOW` on all source file opens. |
| Infinite subprocess hang | `exec.CommandContext` with timeouts and `SIGKILL`. |
| User accidentally runs as root | `--allow-root` flag required for UID 0. |
| Weird filenames break KDE script | Strict shell quoting (`"$1"`) in wrapper. |

---

## 10. The One-Paragraph Contract

Siphon converts large media files in place on Linux. It refuses before it touches anything it can't handle safely. On success, the source is gone and the output is in its place, byte-identical to a reference conversion. On refusal, the source is byte-identical to before the command ran, and if invoked from the KDE menu, a popup explains exactly why. On crash, a journal lets `recover` finish the job. It never uses 2× space without telling you, never strips alpha without your say-so, never transcodes a track without your flag, and never makes a network call.
