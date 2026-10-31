import {clamp01, smoothstep} from './anim';
import {mixHex} from './color';
import {COLORS} from '../theme';
import {lerp3, type Vec3} from './vec';

export const PIPELINE = {
  count: 6,
  segmentSize: [0.7, 1.65, 1.32] as Vec3,
  sourceX: (i: number) => -4.6 + i * 0.78,
  outputX: (i: number) => 0.7 + i * 0.78,
  gateX: 0,
  baseY: 0.28,
  cycleStart: 24,
  cycleLength: 24,
  /** Process from the source tail (rightmost slot) back towards the head. */
  order: [5, 4, 3, 2, 1, 0] as const,
  introEnd: 22,
  finalLand: 168,
};

export type SegmentPhase = 'source' | 'inbound' | 'gate' | 'verified' | 'outbound' | 'output';

export type SegmentState = {
  index: number;
  sourceIndex: number;
  outputIndex: number;
  cycle: number;
  cycleStart: number;
  position: Vec3;
  rotation: Vec3;
  color: string;
  phase: SegmentPhase;
  gateProgress: number;
  verifiedAt: number;
  landedAt: number;
  active: boolean;
  scale: number;
  opacity: number;
};

const smoother = (t: number) => {
  const x = clamp01(t);
  return x * x * x * (x * (x * 6 - 15) + 10);
};

const arcPoint = (a: Vec3, b: Vec3, t: number, height: number, zArc: number): Vec3 => {
  const p = lerp3(a, b, smoother(t));
  const lift = Math.sin(Math.PI * clamp01(t)) * height;
  const z = Math.sin(Math.PI * clamp01(t)) * zArc;
  return [p[0], p[1] + lift, p[2] + z];
};

export const segmentState = (sourceIndex: number, frame: number): SegmentState => {
  const cycle = PIPELINE.order.indexOf(sourceIndex as (typeof PIPELINE.order)[number]);
  const outputIndex = cycle;
  const T = PIPELINE.cycleStart + cycle * PIPELINE.cycleLength;
  const source: Vec3 = [PIPELINE.sourceX(sourceIndex), PIPELINE.baseY, 0];
  const gateIn: Vec3 = [-1.22, PIPELINE.baseY, 0];
  const gateOut: Vec3 = [1.22, PIPELINE.baseY, 0];
  const output: Vec3 = [PIPELINE.outputX(outputIndex), PIPELINE.baseY, 0];

  const intro = smoothstep(1 + sourceIndex * 1.1, 15 + sourceIndex * 1.1, frame);
  const introOffset = -5.6 * (1 - intro);
  const baseSource: Vec3 = [source[0] + introOffset, source[1], source[2]];

  let position: Vec3 = baseSource;
  let phase: SegmentPhase = 'source';
  let gateProgress = 0;
  let rotation: Vec3 = [0, 0, 0];
  let scale = 1;
  let opacity = 1;

  if (frame >= T) {
    const t = frame - T;
    if (t < 9) {
      phase = 'inbound';
      const p = smoothstep(0, 1, t / 9);
      position = arcPoint(source, gateIn, t / 9, 0.52, 0.42);
      rotation = [0, 0.22 * Math.sin(Math.PI * p), 0];
    } else if (t < 18) {
      phase = 'gate';
      const p = smoothstep(0, 1, (t - 9) / 9);
      gateProgress = clamp01((t - 9) / 9);
      position = [gateIn[0] + (gateOut[0] - gateIn[0]) * p, PIPELINE.baseY, 0];
      rotation = [0, 0.05 * Math.sin(Math.PI * p), 0];
    } else if (t < 24) {
      phase = 'outbound';
      const p = smoothstep(0, 1, (t - 18) / 6);
      position = arcPoint(gateOut, output, (t - 18) / 6, 0.36, 0.24);
      rotation = [0, 0.2 * Math.sin(Math.PI * p), 0];
    } else {
      phase = 'output';
      position = output;
      scale = 1;
    }
  }

  const verifiedAt = T + 16;
  const landedAt = T + 24;
  const active = frame >= T + 9 && frame < T + 19;
  if (frame >= verifiedAt) phase = frame >= landedAt ? 'output' : 'verified';

  let color: string = COLORS.red;
  if (frame >= T + 8) {
    if (frame < T + 13) {
      color = mixHex(COLORS.red, COLORS.blue, smoothstep(T + 8, T + 13, frame));
    } else if (frame < T + 15) {
      color = COLORS.blue;
    } else if (frame < T + 18) {
      color = mixHex(COLORS.blue, COLORS.green, smoothstep(T + 15, T + 18, frame));
    } else {
      color = COLORS.green;
    }
  }

  if (frame < 0) opacity = 0;

  return {
    index: cycle,
    sourceIndex,
    outputIndex,
    cycle,
    cycleStart: T,
    position,
    rotation,
    color,
    phase,
    gateProgress,
    verifiedAt,
    landedAt,
    active,
    scale,
    opacity,
  };
};

export const allSegmentStates = (frame: number) =>
  [0, 1, 2, 3, 4, 5].map((sourceIndex) => segmentState(sourceIndex, frame));

/** The cycle that currently occupies the gate, if any. */
export const activeCycle = (frame: number) => {
  for (let k = 0; k < PIPELINE.count; k++) {
    const T = PIPELINE.cycleStart + k * PIPELINE.cycleLength;
    if (frame >= T + 9 && frame < T + 19) return k;
  }
  return -1;
};

/** Most recently completed cycle, for a small progress indicator. */
export const verifiedCount = (frame: number) => {
  let count = 0;
  for (let k = 0; k < PIPELINE.count; k++) {
    const T = PIPELINE.cycleStart + k * PIPELINE.cycleLength;
    if (frame >= T + 18) count++;
  }
  return count;
};

export const reclaimProgress = (sourceIndex: number, frame: number) => {
  const cycle = PIPELINE.order.indexOf(sourceIndex as (typeof PIPELINE.order)[number]);
  const T = PIPELINE.cycleStart + cycle * PIPELINE.cycleLength;
  return clamp01((frame - (T + 17)) / 7);
};

export const sourceHuskVisible = (sourceIndex: number, frame: number) => {
  const cycle = PIPELINE.order.indexOf(sourceIndex as (typeof PIPELINE.order)[number]);
  const T = PIPELINE.cycleStart + cycle * PIPELINE.cycleLength;
  return frame >= T;
};
