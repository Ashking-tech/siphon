export const clamp = (value: number, min = 0, max = 1) =>
  Math.min(max, Math.max(min, value));

/** Smooth, non-elastic 0→1 ramp. */
export const ramp = (frame: number, start: number, duration: number) => {
  const t = clamp((frame - start) / Math.max(1, duration));
  return t * t * (3 - 2 * t);
};

/** 1→0 ramp. */
export const fall = (frame: number, start: number, duration: number) =>
  1 - ramp(frame, start, duration);

export const pulse = (frame: number, at: number, duration: number) => {
  if (frame < at - duration || frame > at + duration) return 0;
  const d = Math.abs(frame - at) / duration;
  return Math.max(0, 1 - d * d);
};

export const lerp = (a: number, b: number, t: number) => a + (b - a) * t;

export type CursorKey = {frame: number; x: number; y: number};

export const cursorAt = (frame: number, keys: CursorKey[]) => {
  if (frame <= keys[0].frame) return {x: keys[0].x, y: keys[0].y};
  for (let i = 0; i < keys.length - 1; i++) {
    const a = keys[i];
    const b = keys[i + 1];
    if (frame <= b.frame) {
      const t = ramp(frame, a.frame, b.frame - a.frame);
      return {x: lerp(a.x, b.x, t), y: lerp(a.y, b.y, t)};
    }
  }
  const last = keys[keys.length - 1];
  return {x: last.x, y: last.y};
};

/** A deterministic count that updates smoothly. */
export const countTo = (frame: number, start: number, end: number, from: number, to: number) =>
  lerp(from, to, ramp(frame, start, end - start));
