import {Easing, interpolate, type EasingFunction} from 'remotion';

export const EASE = {
  outExpo: Easing.bezier(0.16, 1, 0.3, 1),
  outQuint: Easing.bezier(0.22, 1, 0.36, 1),
  snap: Easing.bezier(0.2, 0.9, 0.1, 1),
  inOut: Easing.bezier(0.66, 0, 0.34, 1),
  inExpo: Easing.bezier(0.7, 0, 0.84, 0),
  outBack: Easing.bezier(0.34, 1.32, 0.64, 1),
} as const;

export const clamp01 = (value: number) => Math.min(1, Math.max(0, value));

export const range = (
  frame: number,
  start: number,
  end: number,
  from = 0,
  to = 1,
  easing: EasingFunction = EASE.outExpo,
) =>
  interpolate(frame, [start, end], [from, to], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing,
  });

export const smoothstep = (edge0: number, edge1: number, x: number) => {
  const t = clamp01((x - edge0) / Math.max(1e-6, edge1 - edge0));
  return t * t * (3 - 2 * t);
};

export const appear = (frame: number, start: number, duration = 14) =>
  range(frame, start, start + duration, 0, 1, EASE.outExpo);

export const fadeAt = (frame: number, start: number, end: number) =>
  range(frame, start, end, 0, 1, EASE.inOut);

export const fadeWindow = (frame: number, inAt: number, inDur: number, outAt: number, outDur: number) =>
  Math.min(appear(frame, inAt, inDur), 1 - appear(frame, outAt, outDur));

export const mix = (a: number, b: number, t: number) => a + (b - a) * t;

export const hash = (n: number) => {
  const x = Math.sin(n * 127.1 + 311.7) * 43758.5453123;
  return x - Math.floor(x);
};

export const signedHash = (n: number) => hash(n) * 2 - 1;

/** A deterministic ballistic arc: 0 → 1 with a raised midpoint. */
export const arc = (t: number, height = 0.4) => Math.sin(Math.PI * clamp01(t)) * height;

export const stagger = (frame: number, start: number, index: number, step = 2, duration = 16) =>
  range(frame, start + index * step, start + index * step + duration, 0, 1, EASE.outExpo);
