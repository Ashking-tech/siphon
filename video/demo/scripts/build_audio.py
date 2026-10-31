#!/usr/bin/env python3
"""Siphon abstract demo — original 18 s electronic soundscape.

Everything is synthesised here: sub drone, machine air, the inspection sweep,
six segment handovers, assembly clacks, the atomic commit and the closing
chord. No samples, no external audio.

    python3 scripts/build_audio.py

Writes public/audio/soundtrack.wav (48 kHz, 16-bit stereo).
Cue times mirror src/timeline.ts and src/lib/pipeline.ts.
"""

from pathlib import Path
import wave

import numpy as np
from scipy.signal import butter, sosfilt

SR = 48_000
DUR = 18.0
N = int(SR * DUR)
rng = np.random.default_rng(20261031)
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "public" / "audio" / "soundtrack.wav"
OUT.parent.mkdir(parents=True, exist_ok=True)


def t_axis(duration: float) -> np.ndarray:
    return np.arange(max(1, int(duration * SR))) / SR


def sine(freq: float, duration: float, phase: float = 0.0) -> np.ndarray:
    t = t_axis(duration)
    return np.sin(2 * np.pi * freq * t + phase)


def gliss(f0: float, f1: float, duration: float) -> np.ndarray:
    t = t_axis(duration)
    freqs = f0 * (f1 / f0) ** (t / duration)
    phase = 2 * np.pi * np.cumsum(freqs) / SR
    return np.sin(phase)


def lowpass(x: np.ndarray, cutoff: float, order: int = 2) -> np.ndarray:
    sos = butter(order, cutoff / (SR / 2), btype="low", output="sos")
    return sosfilt(sos, x)


def highpass(x: np.ndarray, cutoff: float, order: int = 2) -> np.ndarray:
    sos = butter(order, cutoff / (SR / 2), btype="high", output="sos")
    return sosfilt(sos, x)


def bandpass(x: np.ndarray, low: float, high: float, order: int = 2) -> np.ndarray:
    sos = butter(order, [low / (SR / 2), high / (SR / 2)], btype="band", output="sos")
    return sosfilt(sos, x)


def env_exp(duration: float, decay: float, attack: float = 0.002) -> np.ndarray:
    t = t_axis(duration)
    return (1.0 - np.exp(-t / max(attack, 1e-5))) * np.exp(-t / max(decay, 1e-5))


def stereo(sig: np.ndarray, pan: float = 0.0) -> np.ndarray:
    pan = float(np.clip(pan, -1.0, 1.0))
    angle = (pan + 1.0) * np.pi / 4.0
    return np.column_stack([sig * np.cos(angle), sig * np.sin(angle)])


def add(buf: np.ndarray, sig: np.ndarray, at: float, gain: float = 1.0, pan: float = 0.0) -> None:
    indexed = stereo(sig * gain, pan)
    start = int(round(at * SR))
    if start >= N or start < 0:
        return
    end = min(N, start + indexed.shape[0])
    buf[start:end] += indexed[: end - start]


# --------------------------------------------------------------------------- #
# Voices
# --------------------------------------------------------------------------- #


def impact(intensity: float = 1.0, pitch: float = 92.0, tail: float = 0.85) -> np.ndarray:
    duration = tail + 0.15
    t = t_axis(duration)
    attack = 1.0 - np.exp(-t / 0.0028)
    body = gliss(pitch, 32.0, duration) * attack * np.exp(-t / (tail * 0.46))
    sub = sine(44.0, duration) * attack * np.exp(-t / (tail * 0.7)) * 0.7
    noise = lowpass(rng.standard_normal(len(t)) * attack * np.exp(-t / 0.055), 360.0, 2) * 0.8
    transient = highpass(rng.standard_normal(len(t)), 1900.0, 2) * env_exp(duration, 0.0065, 0.0008) * 0.2
    return (body * 0.85 + sub + noise * 0.3 + transient) * intensity


def soft_hit(intensity: float = 0.5, pitch: float = 112.0, tail: float = 0.3) -> np.ndarray:
    duration = tail + 0.2
    t = t_axis(duration)
    body = gliss(pitch, 48.0, duration) * (1 - np.exp(-t / 0.004)) * np.exp(-t / tail)
    click = highpass(rng.standard_normal(len(t)), 2300.0, 2) * env_exp(duration, 0.004, 0.0008)
    return (body * 0.55 + click * 0.1) * intensity


def click(gain: float = 1.0, high: float = 3200.0) -> np.ndarray:
    duration = 0.04
    t = t_axis(duration)
    noise = highpass(rng.standard_normal(len(t)), high, 2) * np.exp(-t / 0.0047)
    tick = np.sin(2 * np.pi * (high * 0.82) * t) * np.exp(-t / 0.0030) * 0.24
    return (noise * 0.8 + tick) * gain


def digital_click(gain: float = 1.0, freq: float = 2400.0) -> np.ndarray:
    duration = 0.028
    t = t_axis(duration)
    return (
        np.sign(np.sin(2 * np.pi * freq * t)) * np.exp(-t / 0.0024)
        + rng.standard_normal(len(t)) * np.exp(-t / 0.0014) * 0.18
    ) * gain


def bell(freq: float, duration: float = 1.4, index: float = 2.2, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    modulator = np.sin(2 * np.pi * freq * 2.01 * t) * index * np.exp(-t / 0.32)
    carrier = np.sin(2 * np.pi * freq * t + modulator)
    sparkle = np.sin(2 * np.pi * freq * 3.01 * t) * np.exp(-t / 0.18) * 0.17
    return (carrier * np.exp(-t / 0.52) + sparkle) * gain


def blip(freq: float, duration: float = 0.4, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    return (
        (np.sin(2 * np.pi * freq * t) * 0.8 + np.sin(2 * np.pi * freq * 2.0 * t) * 0.2)
        * (1 - np.exp(-t / 0.002))
        * np.exp(-t / 0.085)
        * gain
    )


def whoosh(duration: float = 0.75, gain: float = 1.0, reverse: bool = False) -> np.ndarray:
    t = t_axis(duration)
    body = bandpass(rng.standard_normal(len(t)), 380.0, 5200.0, 2)
    air = highpass(rng.standard_normal(len(t)), 1700.0, 2)
    shape = np.sin(np.pi * np.clip(t / duration, 0, 1)) ** 2
    rise = np.linspace(0.0, 1.0, len(t)) ** (1.3 if not reverse else 2.8)
    env = shape * (rise if not reverse else 1 - rise)
    return (body * 0.56 + air * 0.32) * env * gain


def riser(duration: float = 1.2, gain: float = 1.0, f0: float = 80.0, f1: float = 380.0) -> np.ndarray:
    t = t_axis(duration)
    tone = gliss(f0, f1, duration) * (t / duration) ** 2.2
    air = highpass(rng.standard_normal(len(t)), 2300.0, 2) * (t / duration) ** 3.0 * 0.28
    return (tone * 0.34 + air) * gain


def sweep(duration: float = 1.3, gain: float = 1.0) -> np.ndarray:
    """A clean inspection-plane sweep: narrow noise band gliding upward."""
    t = t_axis(duration)
    noise = rng.standard_normal(len(t))
    out = np.zeros_like(t)
    # Crossfade a few fixed bands to imply a rising filter without a huge CPU cost.
    bands = [(450, 1500), (800, 2600), (1300, 4200), (2100, 6500)]
    for i, (lo, hi) in enumerate(bands):
        seg = bandpass(noise, lo, hi, 2)
        weight = np.exp(-((t / duration - (i + 0.5) / len(bands)) ** 2) / 0.02)
        out += seg * weight
    env = np.sin(np.pi * np.clip(t / duration, 0, 1)) ** 1.4
    return out * env * gain


def chord(notes: list[float], duration: float, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    out = np.zeros_like(t)
    for i, freq in enumerate(notes):
        for detune in (-0.0035, 0.0035):
            out += np.sin(2 * np.pi * freq * (1 + detune) * t + i * 0.7) * 0.22
        out += np.sin(2 * np.pi * freq * 2 * t) * 0.035
    env = (1 - np.exp(-t / 0.5)) * np.exp(-t / (duration * 0.64))
    movement = 1.0 + 0.12 * np.sin(2 * np.pi * 0.13 * t + 0.4)
    return lowpass(out * env * movement, 2800.0, 2) * gain


# --------------------------------------------------------------------------- #
# Bed: D drone + machine air, automated across the four acts
# --------------------------------------------------------------------------- #

timeline = np.arange(N) / SR
bed = np.zeros(N)
bed += sine(73.42, DUR) * 0.34          # D2
bed += sine(110.0, DUR) * 0.11          # A2
bed += sine(146.83, DUR) * 0.055        # D3
bed += sine(220.0, DUR) * 0.026         # A3
bed += lowpass(rng.standard_normal(N), 160.0, 2) * 0.016

air = highpass(rng.standard_normal(N), 4200.0, 2)
air *= 0.6 + 0.4 * np.sin(2 * np.pi * 0.057 * timeline + 0.8)
bed += air * 0.009

bed_gain = np.interp(
    timeline,
    [0.0, 0.7, 2.8, 3.0, 4.8, 5.0, 5.8, 10.6, 11.6, 12.0, 13.2, 15.6, 16.0, 17.0, 18.0],
    [0.0, 0.75, 0.72, 0.55, 0.62, 1.05, 1.15, 1.1, 0.55, 0.95, 1.0, 0.6, 0.5, 0.28, 0.0],
)
# Subtle 75 BPM breathing through the processing act.
bed_pulse = 0.84 + 0.16 * np.sin(2 * np.pi * (1.25 / 0.8) * timeline - np.pi / 2)
bed_gain *= np.where((timeline >= 5.0) & (timeline <= 12.0), bed_pulse, 1.0)
bed_out = bed * bed_gain

major_hits = [0.72, 2.95, 4.5, 5.0, 10.6, 12.0, 13.1, 14.0, 16.0]
duck = np.ones(N)
for hit in major_hits:
    start = int(hit * SR)
    length = int(0.55 * SR)
    end = min(N, start + length)
    if start < N:
        tt = np.arange(end - start) / SR
        duck[start:end] *= 1.0 - 0.4 * np.exp(-tt / 0.18)

master = np.column_stack([bed_out, bed_out]) * duck[:, None] * 0.32

# --------------------------------------------------------------------------- #
# Cue sheet (seconds)
# --------------------------------------------------------------------------- #

# Scene 1 — the file
add(master, whoosh(0.9, 0.5), 0.0, 0.8, 0.0)
add(master, riser(0.85, 0.3, 70.0, 220.0), 0.0, 0.5, -0.1)
add(master, impact(0.85, 88.0, 0.9), 0.72, 0.8, -0.08)
add(master, soft_hit(0.32, 120.0, 0.35), 0.78, 0.55, 0.15)
add(master, click(0.24, 3600.0), 0.9, 0.45, 0.2)
add(master, digital_click(0.4, 1900.0), 1.94, 0.55, 0.35)
add(master, blip(523.25, 0.35, 0.12), 1.96, 0.4, 0.35)
# Layer separation
add(master, whoosh(0.6, 0.42), 2.9, 0.7, -0.1)
add(master, soft_hit(0.4, 74.0, 0.4), 2.96, 0.65, -0.2)
for i in range(6):
    add(master, click(0.17, 3300.0 + i * 240), 2.98 + i * 0.045, 0.5, -0.4 + i * 0.16)

# Scene 2 — preflight
add(master, sweep(1.35, 0.28), 3.12, 0.65, -0.05)
add(master, riser(1.3, 0.16, 120.0, 440.0), 3.15, 0.45, 0.0)
for i in range(6):
    add(master, click(0.16, 3900.0 + i * 150), 3.3 + i * 0.17, 0.44, -0.45 + i * 0.18)
add(master, bell(659.25, 0.9, 1.4, 0.16), 3.95, 0.5, 0.2)
add(master, digital_click(0.22, 2200.0), 3.95, 0.45, 0.2)
add(master, soft_hit(0.55, 100.0, 0.6), 4.5, 0.7, 0.0)
add(master, bell(880.0, 1.3, 1.6, 0.2), 4.5, 0.6, 0.05)
add(master, click(0.28, 2800.0), 4.52, 0.5, 0.2)

# Into the processing stage
add(master, impact(1.0, 78.0, 1.15), 5.0, 0.9, 0.0)
add(master, whoosh(0.8, 0.5, reverse=True), 4.72, 0.7, 0.0)
add(master, riser(0.5, 0.4, 60.0, 260.0), 5.0, 0.6, 0.0)
add(master, digital_click(0.3, 1500.0), 5.55, 0.5, 0.0)
add(master, soft_hit(0.35, 84.0, 0.5), 5.56, 0.6, 0.0)

# Six tail-first handovers — one segment at a time.
cycle_notes = [329.63, 392.0, 440.0, 523.25, 587.33, 659.25]
for i, cue in enumerate([5.8, 6.6, 7.4, 8.2, 9.0, 9.8]):
    pan = -0.32 + i * 0.13
    add(master, click(0.34, 2600.0 + i * 120), cue, 0.6, pan)
    add(master, soft_hit(0.28, 96.0 + i * 4, 0.28), cue, 0.52, pan)
    add(master, whoosh(0.26, 0.26), cue, 0.5, pan)
    add(master, sweep(0.3, 0.14), cue + 0.17, 0.45, pan)
    add(master, digital_click(0.22, 2400.0 + i * 140), cue + 0.32, 0.5, pan)
    add(master, bell(cycle_notes[i], 0.55, 1.2, 0.13), cue + 0.54, 0.54, 0.2 - i * 0.08)
    add(master, blip(cycle_notes[i], 0.3, 0.12), cue + 0.55, 0.5, 0.2 - i * 0.08)
    add(master, whoosh(0.38, 0.3, reverse=True), cue + 0.62, 0.5, pan)
    add(master, click(0.22, 5200.0), cue + 0.64, 0.4, -0.1)
    add(master, soft_hit(0.4, 92.0 + i * 5, 0.34), cue + 0.73, 0.55, 0.32 - i * 0.08)

# Completion of processing
add(master, impact(0.8, 104.0, 0.9), 10.6, 0.78, 0.0)
add(master, bell(659.25, 1.6, 2.0, 0.22), 10.62, 0.62, 0.0)
add(master, bell(987.77, 1.3, 1.7, 0.13), 10.64, 0.55, 0.0)
add(master, riser(1.1, 0.28, 120.0, 500.0), 10.9, 0.55, 0.0)

# Scene 4 — assembly
add(master, impact(0.9, 84.0, 0.9), 12.0, 0.8, 0.0)
add(master, whoosh(0.7, 0.42), 11.75, 0.65, 0.0)

assembly = [12.33, 12.41, 12.49, 12.57, 12.65, 12.73]
lock_notes = [392.0, 440.0, 523.25, 587.33, 659.25, 783.99]
for i, cue in enumerate(assembly):
    add(master, soft_hit(0.34, 100.0 + i * 6, 0.22), cue, 0.5, -0.15 + i * 0.06)
    add(master, click(0.28, 2800.0 + i * 180), cue, 0.55, -0.15 + i * 0.06)
    add(master, blip(lock_notes[i], 0.2, 0.06), cue + 0.01, 0.4, -0.15 + i * 0.06)

add(master, impact(0.7, 78.0, 0.6), 13.1, 0.72, 0.0)
add(master, soft_hit(0.45, 130.0, 0.35), 13.1, 0.6, 0.0)
add(master, bell(523.25, 1.2, 1.6, 0.13), 13.12, 0.5, 0.0)
add(master, whoosh(0.5, 0.25), 13.35, 0.55, 0.0)

# Atomic commit + verification + filename
add(master, impact(1.0, 112.0, 1.2), 14.0, 0.9, 0.0)
add(master, riser(0.5, 0.3, 90.0, 420.0), 13.75, 0.55, 0.0)
add(master, bell(659.25, 1.9, 2.2, 0.26), 14.02, 0.7, 0.0)
add(master, bell(987.77, 1.4, 1.8, 0.14), 14.05, 0.6, 0.05)
add(master, digital_click(0.3, 2500.0), 14.4, 0.5, 0.15)
add(master, bell(880.0, 0.9, 1.3, 0.1), 14.42, 0.45, 0.15)
add(master, click(0.26, 3200.0), 14.8, 0.48, 0.2)

# Scene 5 — close
add(master, whoosh(0.55, 0.3, reverse=True), 15.65, 0.55, 0.0)
add(master, impact(0.62, 72.0, 1.4), 16.0, 0.58, 0.0)
add(master, bell(440.0, 2.0, 1.9, 0.2), 16.02, 0.6, 0.0)
add(master, bell(880.0, 1.7, 1.4, 0.1), 16.04, 0.5, 0.0)

closing = chord([73.42, 110.0, 146.83, 220.0, 329.63], 2.2, 0.22)
add(master, closing, 16.0, 0.9, 0.0)
add(master, sine(36.71, 2.0) * env_exp(2.0, 1.2, 0.3) * 0.32, 16.0, 0.8, 0.0)
add(master, bell(1318.5, 1.2, 1.1, 0.07), 16.65, 0.5, 0.0)
add(master, click(0.16, 4200.0), 16.66, 0.4, 0.3)

# --------------------------------------------------------------------------- #
# Master
# --------------------------------------------------------------------------- #

master[:, 0] = highpass(master[:, 0], 24.0, 2)
master[:, 1] = highpass(master[:, 1], 24.0, 2)
master = np.tanh(master * 1.16) / 1.16
master *= 0.86 / max(1e-9, np.max(np.abs(master)))

fade = np.ones(N)
fade_in = int(0.012 * SR)
fade[:fade_in] = np.linspace(0, 1, fade_in) ** 2
fade_out = int(1.1 * SR)
fade[-fade_out:] = np.linspace(1, 0, fade_out) ** 2.2
master *= fade[:, None]

dither = (rng.random((N, 2)) - rng.random((N, 2))) / 32768.0
pcm = np.clip(master + dither, -1, 1)
pcm16 = np.round(pcm * 32767).astype(np.int16)

with wave.open(str(OUT), "wb") as wav:
    wav.setnchannels(2)
    wav.setsampwidth(2)
    wav.setframerate(SR)
    wav.writeframes(pcm16.tobytes())

peak = float(np.max(np.abs(master)))
rms = float(np.sqrt(np.mean(master**2)))
print(f"wrote {OUT} ({OUT.stat().st_size / 1024 / 1024:.1f} MiB)")
print(f"peak {20 * np.log10(peak):.2f} dBFS · rms {20 * np.log10(rms):.2f} dBFS")
