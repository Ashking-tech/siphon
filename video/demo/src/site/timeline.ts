export const LOOP = {
  fps: 30,
  width: 1280,
  height: 720,
  durationInFrames: 390,
} as const;

export const SITE_SCENES = [
  {id: 'dolphin', from: 0, duration: 92},
  {id: 'engine', from: 92, duration: 212},
  // Holds underneath the closing loop card so the reset never exposes a cut.
  {id: 'success', from: 304, duration: 86},
] as const;

export const SITE_TRANSITIONS = [
  {from: 84, duration: 16, direction: 'left' as const},
  {from: 296, duration: 16, direction: 'up' as const},
];

export const SITE_CUES = {
  loopInHold: 6,
  loopInFade: 10,
  loopOutStart: 378,
  loopOutFade: 10,

  cursorIn: 14,
  hoverFile: 30,
  rightClick: 36,
  menuOpen: 36,
  toSiphon: 42,
  siphonHover: 50,
  submenuOpen: 52,
  toAction: 54,
  actionHover: 72,
  click: 78,
  menuClose: 82,

  engineIntro: 0,
  engineBars: 8,
  engineCard: 16,
  cyclesStart: 58,
  cycleFrames: 18,
  cycles: 8,
  cyclesDone: 58 + 18 * 8,

  successSettle: 8,
  successCheck: 18,
  successLine: 26,
  successSub: 34,
} as const;

export const SOURCE_NAME = 'interstellar_4k.mkv';
export const OUTPUT_NAME = 'interstellar_4k.mp4';
export const FILE_SIZE = '60 GB';
