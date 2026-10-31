import type {CSSProperties} from 'react';
import {spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {EASE, hash, range, signedHash} from '../lib/anim';
import {COLORS} from '../theme';

export type AnimatedTextProps = {
  text: string;
  start?: number;
  stagger?: number;
  duration?: number;
  mode?: 'char' | 'word';
  animation?: 'rise' | 'scale' | 'slide' | 'flip';
  accentWords?: number[];
  accentChars?: number[];
  style?: CSSProperties;
  unitStyle?: CSSProperties;
  className?: string;
};

/**
 * Per-character / per-word entrance with a short spring. This is the workhorse
 * for lines that need to feel physically set rather than cross-faded in.
 */
export const AnimatedText: React.FC<AnimatedTextProps> = ({
  text,
  start = 0,
  stagger = 1.4,
  duration = 18,
  mode = 'word',
  animation = 'rise',
  accentWords = [],
  accentChars = [],
  style,
  unitStyle,
  className,
}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const units = mode === 'word' ? text.split(' ') : [...text];

  return (
    <div
      className={className}
      style={{
        display: 'flex',
        flexWrap: 'wrap',
        alignItems: 'baseline',
        whiteSpace: 'pre',
        ...style,
      }}
    >
      {units.map((unit, i) => {
        const local = frame - start - i * stagger;
        const p = spring({
          frame: local,
          fps,
          config: {
            damping: animation === 'scale' ? 18 : 26,
            stiffness: animation === 'scale' ? 160 : 220,
            mass: animation === 'scale' ? 0.9 : 0.7,
          },
          durationInFrames: duration,
        });
        const clamped = p < 0 ? 0 : p;
        const opacity = range(frame, start + i * stagger, start + i * stagger + Math.min(8, duration), 0, 1, EASE.outQuint);

        let transform = 'none';
        if (animation === 'rise') {
          transform = `translateY(${(1 - clamped) * 115}%)`;
        } else if (animation === 'scale') {
          transform = `scale(${0.62 + clamped * 0.38})`;
        } else if (animation === 'slide') {
          transform = `translateX(${signedHash(i + 7) * (1 - clamped) * 140}px)`;
        } else {
          transform = `perspective(900px) rotateX(${(1 - clamped) * 82}deg) translateY(${(1 - clamped) * 30}%)`;
        }

        const isAccent =
          (mode === 'word' && accentWords.includes(i)) ||
          (mode === 'char' && accentChars.includes(i));

        return (
          <span
            key={`${unit}-${i}`}
            style={{
              display: 'inline-block',
              transformOrigin: animation === 'scale' ? '50% 80%' : '50% 100%',
              transform,
              opacity,
              color: isAccent ? COLORS.accent : undefined,
              willChange: 'transform',
              ...unitStyle,
            }}
          >
            {unit === ' ' ? '\u00A0' : unit}
            {mode === 'word' && i < units.length - 1 ? '\u00A0' : ''}
          </span>
        );
      })}
    </div>
  );
};

/**
 * Small helper for type that needs one or two deliberate rotated / offset
 * fragments instead of a completely regular line.
 */
export const RogueMark: React.FC<{
  index: number;
  children: React.ReactNode;
  style?: CSSProperties;
}> = ({index, children, style}) => (
  <span
    style={{
      display: 'inline-block',
      transform: `translate(${signedHash(index) * 3}px, ${hash(index) * 2}px)`,
      ...style,
    }}
  >
    {children}
  </span>
);
