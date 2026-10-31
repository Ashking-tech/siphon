export const VIDEO = {
  fps: 30,
  width: 1920,
  height: 1080,
  durationInFrames: 35 * 30,
} as const;

export type SceneSpec = {
  id: string;
  code: string;
  label: string;
  from: number;
  duration: number;
};

/**
 * Absolute scene boundaries. The soundtrack's cue sheet uses the same
 * seconds so that every type cut lands on a physical sound.
 */
export const SCENES: SceneSpec[] = [
  {id: 'problem', code: '01', label: 'PRESSURE', from: 0, duration: 150},
  {id: 'question', code: '02', label: 'THE QUESTION', from: 150, duration: 120},
  {id: 'reveal', code: '03', label: 'THE NAME', from: 270, duration: 120},
  {id: 'engineering', code: '04', label: 'THE METHOD', from: 390, duration: 210},
  {id: 'payoff', code: '05', label: 'THE PAYOFF', from: 600, duration: 150},
  {id: 'capabilities', code: '06', label: 'CAPABILITIES', from: 750, duration: 180},
  {id: 'close', code: '07', label: 'SIPHON', from: 930, duration: 120},
];

export const sceneAt = (frame: number): SceneSpec => {
  for (let i = SCENES.length - 1; i >= 0; i--) {
    if (frame >= SCENES[i].from) {
      return SCENES[i];
    }
  }
  return SCENES[0];
};

/**
 * The film's shared cue sheet, in absolute seconds.
 * Kept in sync with scripts/build_audio.py.
 */
export const CUES = {
  problemNumber: 0.4,
  notEnough: 1.5,
  secondCopy: 2.0,
  questionCut: 3.0,
  blackOne: 4.5,
  whyDoes: 5.1,
  another: 6.3,
  blackTwo: 8.9,
  productName: 9.1,
  convertInPlace: 10.3,
  engineering: 13.0,
  segmentWord: 13.35,
  verifyWord: 13.65,
  reclaimWord: 13.95,
  segmentPulses: [14.6, 15.27, 15.93, 16.6, 17.27],
  oneAtATime: 18.0,
  payoff: 20.0,
  slice: 21.35,
  payoffReveal: 22.15,
  qualifier: 23.2,
  formats: [25.15, 25.88, 26.61, 27.34],
  philosophy: [28.1, 28.78, 29.46, 30.14],
  closeIn: 31.25,
  tagline: 32.2,
  end: 35.0,
} as const;
