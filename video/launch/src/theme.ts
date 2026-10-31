/**
 * Siphon launch film — visual system
 *
 * Near-black field, warm off-white type, one acid-lime signal colour.
 * Inter Display carries the editorial voice; Hack carries the machine voice.
 */
export const COLORS = {
  bg: '#080909',
  ink: '#F3F1E8',
  inkSoft: 'rgba(243, 241, 232, 0.66)',
  muted: '#8D8E85',
  dim: 'rgba(243, 241, 232, 0.18)',
  hair: 'rgba(243, 241, 232, 0.22)',
  hairSoft: 'rgba(243, 241, 232, 0.10)',
  accent: '#C9F24D',
  accentSoft: 'rgba(201, 242, 77, 0.16)',
  accentDim: 'rgba(201, 242, 77, 0.38)',
} as const;

export const FONTS = {
  display: '"Inter Display", "Inter", sans-serif',
  mono: '"Hack", "SF Mono", monospace',
} as const;

export const GRID = {
  width: 1920,
  height: 1080,
  margin: 96,
  contentWidth: 1728,
  top: 76,
  bottom: 72,
  col: 144,
} as const;

export const WEIGHT = {
  black: 900,
  extraBold: 800,
  bold: 700,
  medium: 500,
  regular: 400,
} as const;
