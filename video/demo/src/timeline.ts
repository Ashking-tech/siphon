export const VIDEO = {
  fps: 30,
  width: 1920,
  height: 1080,
  durationInFrames: 540,
} as const;

export type SceneSpec = {
  id: string;
  code: string;
  title: string;
  from: number;
  duration: number;
};

export const SCENES: SceneSpec[] = [
  {id: 'file', code: '01', title: 'THE FILE', from: 0, duration: 90},
  {id: 'preflight', code: '02', title: 'PREFLIGHT', from: 90, duration: 60},
  {id: 'processing', code: '03', title: 'SEGMENTED PROCESSING', from: 150, duration: 210},
  {id: 'finalize', code: '04', title: 'FINALISATION', from: 360, duration: 120},
  {id: 'close', code: '05', title: 'SIPHON', from: 480, duration: 60},
];

export const sceneAt = (frame: number): SceneSpec => {
  for (let i = SCENES.length - 1; i >= 0; i--) {
    if (frame >= SCENES[i].from) return SCENES[i];
  }
  return SCENES[0];
};

/**
 * Shared cue sheet, in absolute seconds. Mirrors scripts/build_audio.py.
 */
export const CUES = {
  fileEnter: 0.08,
  fileSettle: 0.72,
  targetIn: 1.94,
  split: 2.95,
  scanStart: 3.15,
  scanEnd: 4.45,
  headroom: 3.95,
  preflightDone: 4.5,
  toProcessing: 5.0,
  gateOn: 5.55,
  cycles: [5.8, 6.6, 7.4, 8.2, 9.0, 9.8],
  cycleTick: 0.3,
  verifyOffset: 0.54,
  reclaimOffset: 0.64,
  impactOffset: 0.73,
  processingDone: 10.6,
  toFinalize: 12.0,
  assembly: [12.33, 12.41, 12.49, 12.57, 12.65, 12.73],
  lock: 13.1,
  commit: 14.0,
  verify: 14.4,
  filename: 14.8,
  toClose: 16.0,
  tagline: 16.65,
  end: 18.0,
} as const;
