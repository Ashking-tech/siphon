import {interpolate, type EasingFunction} from 'remotion';
import {EASE, clamp01} from '../../lib/anim';

export {EASE, clamp01};
export {mixHex, rgba, shade} from '../../lib/color';
export {range, appear, smoothstep, stagger} from '../../lib/anim';

export type Point = {x: number; y: number};
export type Keyframe = {frame: number; x: number; y: number; ease?: EasingFunction};

/** Piecewise eased motion through a list of cursor waypoints. */
export const cursorAt = (frame: number, keys: Keyframe[]): Point => {
  if (frame <= keys[0].frame) return {x: keys[0].x, y: keys[0].y};
  for (let i = 1; i < keys.length; i++) {
    const a = keys[i - 1];
    const b = keys[i];
    if (frame <= b.frame) {
      const p = interpolate(frame, [a.frame, b.frame], [0, 1], {
        extrapolateLeft: 'clamp',
        extrapolateRight: 'clamp',
        easing: b.ease ?? EASE.inOut,
      });
      return {x: a.x + (b.x - a.x) * p, y: a.y + (b.y - a.y) * p};
    }
  }
  return {x: keys[keys.length - 1].x, y: keys[keys.length - 1].y};
};

/** A 0→1→0 pulse. */
export const pulse = (frame: number, start: number, duration: number) => {
  const local = (frame - start) / duration;
  if (local < 0 || local > 1) return 0;
  return Math.sin(Math.PI * local);
};

/** 0 → 1 over a frame window, clamped. */
export const ramp = (frame: number, start: number, duration: number, easing = EASE.outExpo) =>
  interpolate(frame, [start, start + duration], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing,
  });

/** 1 → 0 over a frame window, clamped. */
export const fall = (frame: number, start: number, duration: number, easing = EASE.inOut) =>
  interpolate(frame, [start, start + duration], [1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing,
  });

export const clamp = (value: number, min = 0, max = 1) => Math.min(max, Math.max(min, value));

/** Deterministic, tiny per-item offsets for flat decorative layouts. */
export const hash = (n: number) => {
  const x = Math.sin(n * 127.1 + 311.7) * 43758.5453123;
  return x - Math.floor(x);
};
