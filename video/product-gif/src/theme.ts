export const C = {
  white: '#FFFFFF',
  ink: '#171717',
  inkSoft: 'rgba(23, 23, 23, 0.64)',
  inkFaint: 'rgba(23, 23, 23, 0.38)',
  blue: '#3155E7',
  green: '#42A987',
  red: '#E7473F',
  reclaimed: '#E5E5E5',
  hairline: '#D9D9D9',
  hairlineSoft: '#EFEFEF',
  wash: '#FAFAFA',
  blueWash: '#EEF2FF',
  greenWash: '#EAF6F1',
  redWash: '#FDEDEC',

  // Light Breeze / Plasma surfaces for the Dolphin recreation.
  kdeWindow: '#FFFFFF',
  kdeChrome: '#FBFBFC',
  kdePanel: '#F6F7F9',
  kdeLine: '#E1E3E7',
  kdeLineSoft: '#ECEEF1',
  kdeText: '#232629',
  kdeMuted: '#7B8087',
  kdeSelection: '#E4EAFF',
  kdeHover: '#F1F4FA',
} as const;

export const FONTS = {
  display: '"Inter Display", "Noto Sans", sans-serif',
  ui: '"Noto Sans", "Inter Display", sans-serif',
  mono: '"Hack", "Noto Sans Mono", monospace',
} as const;

export const FRAME = {
  width: 1280,
  height: 720,
  fps: 30,
  durationInFrames: 408,
} as const;

export const BAR = {
  x: 192,
  width: 1024,
  sourceY: 164,
  sourceH: 40,
  outputY: 334,
  outputH: 40,
  processY: 258,
  diskY: 476,
  diskH: 22,
} as const;

export const SEGMENTS = 12;
export const SEG_GAP = 4;
export const SEG_WIDTH = (BAR.width - (SEGMENTS - 1) * SEG_GAP) / SEGMENTS;
export const segX = (index: number) => BAR.x + index * (SEG_WIDTH + SEG_GAP);
export const segCenterX = (index: number) => segX(index) + SEG_WIDTH / 2;
