#!/usr/bin/env python3
"""Siphon launch film — original 35 s electronic soundscape.

The cue sheet mirrors src/timeline.ts. Everything is synthesised here: sub
drone, mechanical ticks, impacts, the 60 GB -> 340 MB accent, and the closing
chord. No samples, no external audio.

    python3 scripts/build_audio.py

Writes public/audio/soundtrack.wav (48 kHz, 16-bit stereo).
"""

from pathlib import Path
import wave

import numpy as np
from scipy.signal import butter, sosfilt

SR = 48_000
DUR = 35.0
N = int(SR * DUR)
rng = np.random.default_rng(20261031)
ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / "public" / "audio" / "soundtrack.wav"
OUT.parent.mkdir(parents=True, exist_ok=True)


def t_axis(duration: float) -> np.ndarray:
    return np.arange(max(1, int(duration * SR))) / SR


def env_exp(duration: float, decay: float, attack: float = 0.002) -> np.ndarray:
    t = t_axis(duration)
    attack_curve = 1.0 - np.exp(-t / max(attack, 1e-5))
    return attack_curve * np.exp(-t / max(decay, 1e-5))


def sine(freq: float, duration: float, phase: float = 0.0) -> np.ndarray:
    t = t_axis(duration)
    return np.sin(2 * np.pi * freq * t + phase)


def gliss(f0: float, f1: float, duration: float, exponential: bool = True) -> np.ndarray:
    t = t_axis(duration)
    if exponential and f0 > 0 and f1 > 0:
        freqs = f0 * (f1 / f0) ** (t / duration)
    else:
        freqs = np.linspace(f0, f1, len(t))
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


def stereo(sig: np.ndarray, pan: float = 0.0) -> np.ndarray:
    """Constant-power pan, returned as (n, 2)."""
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
    body = gliss(pitch, 34.0, duration) * attack * np.exp(-t / (tail * 0.46))
    sub = sine(46.0, duration) * attack * np.exp(-t / (tail * 0.7)) * 0.7
    noise = rng.standard_normal(len(t)) * attack * np.exp(-t / 0.055)
    noise = lowpass(noise, 360.0, 2) * 0.8
    transient = highpass(rng.standard_normal(len(t)), 1800.0, 2) * env_exp(duration, 0.007, 0.0008) * 0.22
    return (body * 0.85 + sub + noise * 0.35 + transient) * intensity


def soft_hit(intensity: float = 0.5, pitch: float = 110.0) -> np.ndarray:
    duration = 0.6
    t = t_axis(duration)
    body = gliss(pitch, 48.0, duration) * (1 - np.exp(-t / 0.004)) * np.exp(-t / 0.25)
    click = highpass(rng.standard_normal(len(t)), 2200.0, 2) * env_exp(duration, 0.004, 0.0008)
    return (body * 0.6 + click * 0.12) * intensity


def click(gain: float = 1.0, high: float = 3200.0) -> np.ndarray:
    duration = 0.035
    t = t_axis(duration)
    noise = highpass(rng.standard_normal(len(t)), high, 2) * np.exp(-t / 0.0047)
    tick = np.sin(2 * np.pi * (high * 0.85) * t) * np.exp(-t / 0.0032) * 0.25
    return (noise * 0.8 + tick) * gain


def digital_click(gain: float = 1.0, freq: float = 2400.0) -> np.ndarray:
    duration = 0.025
    t = t_axis(duration)
    return (
        np.sign(np.sin(2 * np.pi * freq * t)) * np.exp(-t / 0.0025)
        + rng.standard_normal(len(t)) * np.exp(-t / 0.0015) * 0.2
    ) * gain


def bell(freq: float, duration: float = 1.4, index: float = 2.2, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    modulator = np.sin(2 * np.pi * freq * 2.01 * t) * index * np.exp(-t / 0.32)
    carrier = np.sin(2 * np.pi * freq * t + modulator)
    sparkle = np.sin(2 * np.pi * freq * 3.01 * t) * np.exp(-t / 0.18) * 0.18
    return (carrier * np.exp(-t / 0.52) + sparkle) * gain


def blip(freq: float, duration: float = 0.4, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    return (np.sin(2 * np.pi * freq * t) * 0.8 + np.sin(2 * np.pi * freq * 2.0 * t) * 0.2) * (
        (1 - np.exp(-t / 0.002)) * np.exp(-t / 0.09)
    ) * gain


def whoosh(duration: float = 0.75, gain: float = 1.0, reverse: bool = False) -> np.ndarray:
    t = t_axis(duration)
    noise = rng.standard_normal(len(t))
    body = bandpass(noise, 420.0, 5200.0, 2)
    # A rising high-passed layer creates the sense of the frame being pulled.
    air = highpass(rng.standard_normal(len(t)), 1800.0, 2)
    shape = np.sin(np.pi * np.clip(t / duration, 0, 1)) ** 2
    rise = np.linspace(0.0, 1.0, len(t)) ** (1.4 if not reverse else 2.6)
    env = shape * (rise if not reverse else 1 - rise)
    return (body * 0.55 + air * 0.32) * env * gain


def riser(duration: float = 1.2, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    tone = gliss(90.0, 420.0, duration) * (t / duration) ** 2.2
    air = highpass(rng.standard_normal(len(t)), 2400.0, 2) * (t / duration) ** 3.0 * 0.3
    return (tone * 0.35 + air) * gain


def chord(notes: list[float], duration: float, gain: float = 1.0) -> np.ndarray:
    t = t_axis(duration)
    out = np.zeros_like(t)
    for i, freq in enumerate(notes):
        for detune in (-0.0035, 0.0035):
            out += np.sin(2 * np.pi * freq * (1 + detune) * t + i * 0.7) * 0.22
        out += np.sin(2 * np.pi * freq * 2 * t) * 0.035
    env = (1 - np.exp(-t / 0.48)) * np.exp(-t / (duration * 0.62))
    # Slow movement, not a static pad.
    movement = 1.0 + 0.12 * np.sin(2 * np.pi * 0.13 * t + 0.4)
    return lowpass(out * env * movement, 2600.0, 2) * gain


# --------------------------------------------------------------------------- #
# Bed: sub drone + air, with section automation and impact ducking
# --------------------------------------------------------------------------- #

timeline = np.arange(N) / SR

bed = np.zeros(N)
bed += sine(55.0, DUR) * 0.42
bed += sine(110.0, DUR) * 0.11
bed += sine(164.81, DUR) * 0.065
bed += lowpass(rng.standard_normal(N), 180.0, 2) * 0.018

# Very low, slowly moving machine air.
air = highpass(rng.standard_normal(N), 4200.0, 2)
air *= 0.65 + 0.35 * np.sin(2 * np.pi * 0.061 * timeline + 0.8)
bed += air * 0.011

bed_gain = np.interp(
    timeline,
    [0.0, 1.2, 4.4, 4.8, 5.0, 8.8, 9.05, 12.3, 13.0, 19.45, 19.95, 20.03, 24.6, 25.0, 30.9, 31.15, 31.3, 34.4, 35.0],
    [0.0, 1.0, 1.0, 0.35, 0.85, 0.55, 1.0, 1.0, 1.15, 0.6, 0.25, 1.25, 1.05, 0.8, 0.55, 0.22, 0.55, 0.4, 0.0],
)
# Gentle 90 BPM breathing while the engineering scene is on screen.
bed_pulse = 0.82 + 0.18 * np.sin(2 * np.pi * (1.5 / 0.6667) * timeline - np.pi / 2)
bed_gain *= np.where((timeline >= 13.0) & (timeline <= 20.0), bed_pulse, 1.0)
bed_out = bed * bed_gain

# Duck the bed under every major impact so the hits have a physical pocket.
major_hits = [0.40, 1.50, 3.00, 5.10, 6.30, 9.10, 13.00, 18.00, 20.00, 22.15, 28.10, 28.78, 29.46, 30.14, 31.25]
duck = np.ones(N)
for hit in major_hits:
    start = int(hit * SR)
    length = int(0.55 * SR)
    end = min(N, start + length)
    if start < N:
        tt = np.arange(end - start) / SR
        duck[start:end] *= 1.0 - 0.42 * np.exp(-tt / 0.19)

master = np.column_stack([bed_out, bed_out]) * duck[:, None] * 0.34

# --------------------------------------------------------------------------- #
# Cue sheet (seconds)
# --------------------------------------------------------------------------- #

# Scene 1 — pressure
add(master, impact(0.55, 80.0, 0.7), 0.40, 0.7, -0.12)
add(master, whoosh(0.9, 0.55), 1.15, 0.8, 0.18)
add(master, impact(1.0, 96.0, 0.95), 1.50, 0.88, -0.04)
add(master, digital_click(0.5, 1900.0), 2.00, 0.5, 0.34)
for i in range(7):
    add(master, click(0.22, 3800.0), 2.10 + i * 0.083, 0.6, -0.35 + i * 0.1)
add(master, riser(0.62, 0.6), 2.35, 0.7, 0.05)
add(master, impact(0.72, 84.0, 0.55), 2.90, 0.65, 0.1)
add(master, impact(0.78, 104.0, 0.6), 3.00, 0.6, -0.15)
add(master, click(0.3, 2400.0), 3.02, 0.5, 0.2)

# Scene 2 — the question
add(master, click(0.4, 3000.0), 5.10, 0.55, -0.2)
add(master, soft_hit(0.28, 120.0), 5.10, 0.5, 0.1)
add(master, click(0.32, 2600.0), 5.35, 0.55, 0.25)
add(master, impact(0.65, 72.0, 0.7), 6.30, 0.72, 0.0)
add(master, bell(440.0, 1.0, 1.8, 0.25), 6.31, 0.5, 0.0)
for i in range(5):
    add(master, click(0.16, 4200.0), 7.15 + i * 0.065, 0.45, -0.4 + i * 0.2)
add(master, whoosh(0.5, 0.38, reverse=True), 8.35, 0.7, 0.0)

# Scene 3 — the name
add(master, impact(0.62, 70.0, 1.2), 9.10, 0.55, 0.0)
add(master, bell(220.0, 2.3, 1.4, 0.22), 9.12, 0.7, 0.0)
for i, note in enumerate([329.63, 392.0, 440.0, 523.25, 659.25, 783.99]):
    add(master, digital_click(0.12, 1800.0 + i * 260), 9.18 + i * 0.082, 0.5, -0.5 + i * 0.2)
add(master, click(0.28, 2800.0), 10.30, 0.5, 0.12)
add(master, bell(523.25, 0.8, 1.4, 0.13), 10.31, 0.4, -0.1)
add(master, whoosh(0.7, 0.4), 11.85, 0.65, 0.0)
add(master, riser(0.6, 0.35), 12.35, 0.55, 0.0)

# Scene 4 — engineering
add(master, impact(0.72, 88.0, 0.7), 13.00, 0.75, -0.06)
add(master, click(0.45, 2600.0), 13.35, 0.6, 0.2)
add(master, click(0.45, 3000.0), 13.65, 0.6, -0.2)
add(master, click(0.55, 3400.0), 13.95, 0.62, 0.1)
add(master, bell(329.63, 0.7, 1.2, 0.12), 13.96, 0.4, 0.1)

pulse_notes = [329.63, 392.0, 440.0, 523.25, 659.25]
for i, cue in enumerate([14.6, 15.27, 15.93, 16.6, 17.27]):
    add(master, impact(0.42, 76.0 + i * 3, 0.38), cue, 0.55, -0.3 + i * 0.15)
    add(master, digital_click(0.34, 2100.0 + i * 180), cue, 0.6, -0.3 + i * 0.15)
    add(master, blip(pulse_notes[i], 0.35, 0.18), cue + 0.012, 0.55, 0.25 - i * 0.12)
    # Off-beat mechanical tick keeps the five handovers moving.
    add(master, click(0.12, 5200.0), cue + 0.333, 0.4, 0.35 if i % 2 == 0 else -0.35)

add(master, impact(0.9, 92.0, 0.8), 18.00, 0.8, 0.0)
add(master, bell(523.25, 1.5, 2.0, 0.2), 18.02, 0.55, 0.0)
add(master, whoosh(0.55, 0.28, reverse=True), 19.25, 0.45, 0.0)

# Scene 5 — the payoff
add(master, impact(1.0, 110.0, 1.1), 20.00, 0.95, 0.0)
add(master, bell(110.0, 2.2, 2.4, 0.18), 20.01, 0.55, 0.0)
add(master, riser(1.15, 0.55), 20.95, 0.55, 0.0)

# The shredder: a dense, panned mechanical cascade into the reveal.
for i in range(46):
    frac = i / 45.0
    at = 21.35 + frac * 0.78 + rng.uniform(-0.008, 0.008)
    add(
        master,
        digital_click(0.10 + 0.12 * frac, 1200.0 + rng.uniform(0, 3200)),
        at,
        0.45 + 0.4 * frac,
        -0.75 + 1.5 * rng.random(),
    )
add(master, whoosh(0.95, 0.55), 21.32, 0.7, 0.0)
add(master, riser(0.9, 0.5), 21.25, 0.7, 0.0)
add(master, impact(1.0, 132.0, 1.25), 22.15, 0.9, 0.0)
add(master, bell(659.25, 2.4, 2.8, 0.38), 22.16, 0.8, 0.0)
add(master, bell(987.77, 2.0, 2.0, 0.22), 22.21, 0.7, 0.0)
add(master, blip(1318.5, 0.9, 0.1), 22.24, 0.45, 0.0)
add(master, click(0.3, 3200.0), 23.20, 0.45, 0.2)

# Scene 6 — formats
format_notes = [523.25, 659.25, 783.99, 1046.5]
for i, cue in enumerate([25.15, 25.88, 26.61, 27.34]):
    add(master, click(0.4, 2600.0 + i * 220), cue, 0.55, -0.4 + i * 0.26)
    add(master, blip(format_notes[i], 0.36, 0.13), cue + 0.005, 0.5, 0.4 - i * 0.26)
    add(master, soft_hit(0.2, 90.0 + i * 8), cue, 0.4, 0.0)

# Philosophy: four hard statements, each a little heavier than the last.
philosophy = [
    (28.10, 0.55, 70.0),
    (28.78, 0.62, 74.0),
    (29.46, 0.72, 80.0),
    (30.14, 0.9, 90.0),
]
for i, (cue, gain, pitch) in enumerate(philosophy):
    add(master, impact(gain, pitch, 0.75), cue, 0.7 + i * 0.05, -0.1 + i * 0.06)
    add(master, click(0.35, 2800.0 + i * 300), cue, 0.6, 0.3 - i * 0.08)
    if i < 3:
        add(master, blip(440.0 * (2 ** (i / 4)), 0.4, 0.12), cue + 0.02, 0.45, 0.1)

# A final cut, then the brand.
add(master, whoosh(0.45, 0.22, reverse=True), 30.82, 0.45, 0.0)
add(master, soft_hit(0.45, 80.0), 31.25, 0.6, 0.0)
add(master, bell(440.0, 2.6, 1.8, 0.24), 31.27, 0.7, 0.0)
add(master, bell(880.0, 2.2, 1.4, 0.12), 31.29, 0.6, 0.0)

# Warm closing chord, A minor add9, opening slowly under the wordmark.
closing = chord([110.0, 164.81, 220.0, 261.63, 493.88], 3.7, 0.24)
add(master, closing, 31.30, 0.95, 0.0)
add(master, sine(55.0, 3.4) * env_exp(3.4, 1.8, 0.35) * 0.36, 31.30, 0.8, 0.0)
add(master, bell(1318.5, 2.0, 1.0, 0.08), 32.20, 0.6, 0.0)
add(master, click(0.18, 4200.0), 32.20, 0.4, 0.35)

# --------------------------------------------------------------------------- #
# Master
# --------------------------------------------------------------------------- #

# Remove DC / subsonic energy.
master[:, 0] = highpass(master[:, 0], 24.0, 2)
master[:, 1] = highpass(master[:, 1], 24.0, 2)

# Gentle glue: soft saturation just above unity, then peak-normalise.
master = np.tanh(master * 1.18) / 1.18
master *= 0.86 / max(1e-9, np.max(np.abs(master)))

# De-click the head and let the final chord dissolve.
fade = np.ones(N)
fade_in = int(0.012 * SR)
fade[:fade_in] = np.linspace(0, 1, fade_in) ** 2
fade_out = int(1.4 * SR)
fade[-fade_out:] = np.linspace(1, 0, fade_out) ** 2.2
master *= fade[:, None]

# TPDF dither for 16-bit output.
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
