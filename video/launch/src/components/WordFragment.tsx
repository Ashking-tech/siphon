import type {CSSProperties} from 'react';
import {useCurrentFrame} from 'remotion';
import {EASE, range} from '../lib/anim';

export type WordFragmentProps = {
  text: string;
  start?: number;
  duration?: number;
  slices?: number;
  mode?: 'assemble' | 'shatter';
  style?: CSSProperties;
  sliceOffset?: number;
};

/**
 * Splits a word into horizontal bands with independent transforms. It is a
 * typographic object, not a picture: every band is the same live text.
 */
export const WordFragment: React.FC<WordFragmentProps> = ({
  text,
  start = 0,
  duration = 22,
  slices = 9,
  mode = 'assemble',
  style,
  sliceOffset = 0.65,
}) => {
  const frame = useCurrentFrame();
  const bands = Array.from({length: slices}, (_, i) => i);

  return (
    <div style={{position: 'relative', display: 'inline-block', ...style}}>
      <div style={{visibility: 'hidden', whiteSpace: 'nowrap'}}>{text}</div>
      {bands.map((i) => {
        const p = range(frame, start + i * sliceOffset, start + i * sliceOffset + duration, 0, 1, EASE.outExpo);
        const direction = i % 2 === 0 ? 1 : -1;
        const amount = mode === 'assemble' ? 1 - p : p;
        const x = direction * amount * (110 + (i % 4) * 34);
        const y = ((i % 3) - 1) * amount * 18;
        const opacity = mode === 'assemble' ? Math.min(1, p * 2) : 1 - p;

        return (
          <div
            key={i}
            style={{
              position: 'absolute',
              inset: 0,
              whiteSpace: 'nowrap',
              clipPath: `inset(${(i / slices) * 100}% 0 ${100 - ((i + 1) / slices) * 100}% 0)`,
              transform: `translate(${x}px, ${y}px)`,
              opacity,
              willChange: 'transform',
            }}
          >
            {text}
          </div>
        );
      })}
    </div>
  );
};
