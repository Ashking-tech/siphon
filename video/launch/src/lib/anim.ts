import {Easing, interpolate, type EasingFunction} from 'remotion';

export const EASE = {
  /** Editorial snap: fast out, long settle. */
  outExpo: Easing.bezier(0.16, 1, 0.3, 1),
  outQuint: Easing.bezier(0.22, 1, 0.36, 1),
  /* Mechanical cuts need a sharper shoulder. */
  snap: Easing.bezier(0.2, 0.9, 0.1, 1),
  inOut: Easing.bezier(0.66, 0, 0.34, 1),
  inExpo: Easing.bezier(0.7, 0, 0.84, 0),
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

export const appear = (frame: number, start: number, duration = 14) =>
  range(frame, start, start + duration, 0, 1, EASE.outExpo);

export const fadeAt = (frame: number, start: number, end: number) =>
  range(frame, start, end, 0, 1, EASE.inOut);

export const hash = (n: number) => {
  const x = Math.sin(n * 127.1 + 311.7) * 43758.5453123;
  return x - Math.floor(x);
};

export const signedHash = (n: number) => hash(n) * 2 - 1;

/** Deterministic stagger that keeps late characters from arriving too late. */
export const staggerAt = (frame: number, start: number, index: number, stagger = 1.5, duration = 16) =>
  range(frame, start + index * stagger, start + index * stagger + duration, 0, 1, EASE.outExpo);
