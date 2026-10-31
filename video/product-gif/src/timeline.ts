export const T = {
  fps: 30,
  duration: 408,

  // Dolphin interaction.
  cursorToFile: 14,
  rightClick: 38,
  menuOpen: 39,
  cursorToSiphon: 44,
  siphonHover: 58,
  submenuOpen: 60,
  cursorToAction: 64,
  actionHover: 76,
  actionClick: 80,
  menuClose: 84,
  dolphinOut: 84,
  infoIn: 88,

  // Infographic introduction.
  barsIn: 96,
  diskIn: 112,

  // Segmented processing: 12 × 16 frames = 192 frames.
  processStart: 132,
  cycle: 16,
  cycles: 12,
  processEnd: 324,

  // Completion and loop reset.
  finalIn: 324,
  dolphinReturn: 332,
  loopStart: 384,
  loopEnd: 408,
} as const;

export type SegmentPhase =
  | 'pending'
  | 'read'
  | 'move'
  | 'process'
  | 'verify'
  | 'reclaim'
  | 'done';

export const segmentTiming = (index: number) => {
  // Relocate is tail-first: the rightmost segment is processed first.
  const position = 11 - index;
  const start = T.processStart + position * T.cycle;
  return {position, start};
};

export const phaseAt = (local: number): SegmentPhase => {
  if (local < 0) return 'pending';
  if (local < 3) return 'read';
  if (local < 6) return 'move';
  if (local < 11) return 'process';
  if (local < 13) return 'verify';
  if (local < 16) return 'reclaim';
  return 'done';
};
