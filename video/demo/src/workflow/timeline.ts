export const FPS = 30;

export type WorkflowScene = {
  id: 'dolphin' | 'preflight' | 'segments' | 'finalize';
  code: string;
  from: number;
  duration: number;
};

/** 22.000 s at 30 fps. */
export const WORKFLOW = {
  fps: FPS,
  width: 1920,
  height: 1080,
  durationInFrames: 660,
} as const;

export const SCENES: WorkflowScene[] = [
  {id: 'dolphin', code: '01', from: 0, duration: 210},
  {id: 'preflight', code: '02', from: 210, duration: 120},
  {id: 'segments', code: '03', from: 330, duration: 180},
  {id: 'finalize', code: '04', from: 510, duration: 150},
];

export const TRANSITIONS = [
  {from: 202, duration: 16, direction: 'left' as const},
  {from: 322, duration: 16, direction: 'right' as const},
  {from: 502, duration: 16, direction: 'up' as const},
];

/** Dolphin interaction cue sheet, local to scene 01. */
export const DOLPHIN_CUES = {
  windowIn: 0,
  cursorIn: 14,
  hoverFile: 38,
  rightClick: 44,
  menuOpen: 44,
  cursorToSiphon: 54,
  siphonHover: 84,
  submenuOpen: 88,
  cursorToAction: 92,
  actionHover: 108,
  actionClick: 112,
  menuClose: 118,
  dialogOpen: 126,
  dialogProgressStart: 140,
  dialogLeave: 190,
} as const;

/** Preflight cue sheet, local to scene 02 (120 frames). */
export const PREFLIGHT_CUES = {
  cardIn: 4,
  panelIn: 0,
  connector: 8,
  token: 12,
  check1: 20,
  check2: 40,
  check3: 60,
  results: 78,
  ready: 100,
} as const;

/** Segmented processing, local to scene 03 (180 frames). */
export const SEGMENT_CUES = {
  stripsIn: 0,
  firstCycle: 18,
  cycleFrames: 23,
  cycles: 6,
  done: 18 + 23 * 6,
} as const;

/** Finalize, local to scene 04 (150 frames). */
export const FINAL_CUES = {
  windowIn: 0,
  fileIn: 8,
  success: 16,
  toastIn: 22,
  toastOut: 66,
  pullBack: 74,
  statement: 94,
  subtitle: 106,
} as const;
