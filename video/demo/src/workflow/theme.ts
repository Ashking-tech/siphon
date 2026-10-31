import {COLORS} from '../theme';

/**
 * Siphon workflow film — palette from the product brief, plus the neutral
 * greys a light Breeze/Plasma desktop needs to feel like a real KDE session.
 *
 * Signal colours have fixed jobs:
 *   red    = source data
 *   blue   = active processing / selection
 *   green  = verified output
 *   yellow = attention / measured value
 *   grey   = reclaimed space
 */
export const C = {
  ...COLORS,
  white: '#FFFFFF',
  ink: '#171717',
  inkSoft: 'rgba(23, 23, 23, 0.62)',
  inkFaint: 'rgba(23, 23, 23, 0.38)',
  blue: '#3155E7',
  red: '#E7473F',
  green: '#42A987',
  yellow: '#F2C94C',
  grey: '#E5E5E5',

  // Light KDE / Breeze surfaces.
  desktop: '#F0F2F6',
  desktopCool: '#E9EDF5',
  desktopWarm: '#F5F2EC',
  window: '#FFFFFF',
  chrome: '#FBFBFC',
  chromeRaised: '#F7F8FA',
  panel: 'rgba(255, 255, 255, 0.94)',
  hairline: '#E2E4E8',
  hairlineSoft: '#EDEEF1',
  muted: '#70747B',
  mutedSoft: '#969AA1',
  blueWash: '#EEF2FF',
  blueSelection: '#E4EAFF',
  blueLine: '#B9C8FF',
  greenWash: '#EAF6F1',
  redWash: '#FDEDEC',
  yellowWash: '#FEF8E6',
} as const;

export const FONTS = {
  ui: '"Noto Sans", "Inter Display", sans-serif',
  display: '"Inter Display", "Noto Sans", sans-serif',
  mono: '"Hack", "Noto Sans Mono", monospace',
} as const;

export const FRAME = {
  width: 1920,
  height: 1080,
} as const;
