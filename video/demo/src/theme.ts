/**
 * Siphon — abstract product film
 *
 * White studio, near-black ink, and four signal colours with fixed jobs:
 * red = source data, blue = active processing, green = verified output,
 * yellow = highlights and commit events.
 */

export const COLORS = {
  white: '#FFFFFF',
  paper: '#F7F6F1',
  paperWarm: '#F2F0EA',
  paperDeep: '#E9E7DF',
  ink: '#171717',
  inkSoft: 'rgba(23, 23, 23, 0.58)',
  inkFaint: 'rgba(23, 23, 23, 0.26)',
  blue: '#3155E7',
  blueDeep: '#1A2F9E',
  blueSoft: 'rgba(49, 85, 231, 0.14)',
  red: '#E7473F',
  redDeep: '#A02520',
  redSoft: 'rgba(231, 71, 63, 0.14)',
  yellow: '#F2C94C',
  yellowDeep: '#C79A1C',
  green: '#42A987',
  greenDeep: '#246A56',
  greenSoft: 'rgba(66, 169, 135, 0.16)',
  grey: '#E5E5E5',
  greyMid: '#C8C6BE',
  stage: '#101112',
  stageRaised: '#17181A',
  stageInk: '#F4F2EC',
  stageMuted: 'rgba(244, 242, 236, 0.42)',
  stageFaint: 'rgba(244, 242, 236, 0.14)',
} as const;

export const FONTS = {
  display: '"Inter Display", "Inter", sans-serif',
  mono: '"Hack", "SF Mono", monospace',
} as const;

export const VIDEO = {
  width: 1920,
  height: 1080,
  fps: 30,
  durationInFrames: 18 * 30,
} as const;
