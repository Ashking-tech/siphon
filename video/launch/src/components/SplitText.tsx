import type {CSSProperties} from 'react';
import {spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {EASE, range, signedHash} from '../lib/anim';
import {COLORS} from '../theme';

export type SplitTextProps = {
  text: string;
  start?: number;
  stagger?: number;
  duration?: number;
  from?: 'below' | 'scatter' | 'left' | 'right' | 'top';
  exitStart?: number;
  exitDuration?: number;
  accentIndices?: number[];
  style?: CSSProperties;
  charStyle?: CSSProperties;
  spaceWidth?: string;
};

/**
 * Character-level fragmentation. A line can assemble out of scattered shards or
 * dismantle back into them; the offsets are deterministic, so every render is
 * identical.
 */
export const SplitText: React.FC<SplitTextProps> = ({
  text,
  start = 0,
  stagger = 1.2,
  duration = 20,
  from = 'scatter',
  exitStart,
  exitDuration = 12,
  accentIndices = [],
  style,
  charStyle,
  spaceWidth = '0.26em',
}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const chars = [...text];

  return (
    <div
      style={{
        display: 'flex',
        flexWrap: 'nowrap',
        alignItems: 'baseline',
        whiteSpace: 'pre',
        ...style,
      }}
    >
      {chars.map((char, i) => {
        const p = spring({
          frame: frame - start - i * stagger,
          fps,
          config: {damping: 20, stiffness: 190, mass: 0.75},
          durationInFrames: duration,
        });
        const entered = p < 0 ? 0 : p;
        const exit =
          exitStart === undefined
            ? 0
            : range(frame, exitStart + i * 0.65, exitStart + exitDuration + i * 0.65, 0, 1, EASE.inExpo);

        if (char === ' ') {
          return (
            <span key={`space-${i}`} style={{display: 'inline-block', width: spaceWidth}}>
              {'\u00A0'}
            </span>
          );
        }

        let enterX = 0;
        let enterY = 0;
        let enterRotate = 0;
        if (from === 'scatter') {
          enterX = signedHash(i + 13) * 330 * (1 - entered);
          enterY = signedHash(i + 71) * 120 * (1 - entered);
          enterRotate = signedHash(i + 37) * 16 * (1 - entered);
        } else if (from === 'left') {
          enterX = -150 * (1 - entered);
        } else if (from === 'right') {
          enterX = 150 * (1 - entered);
        } else if (from === 'below') {
          enterY = 120 * (1 - entered);
        } else {
          enterY = -120 * (1 - entered);
        }

        const exitX = exit * signedHash(i + 17) * 240;
        const exitY = exit * signedHash(i + 53) * 90;
        const opacity = Math.min(
          1,
          range(frame, start + i * stagger, start + i * stagger + 8, 0, 1, EASE.outExpo) * (1 - exit),
        );

        return (
          <span
            key={`${char}-${i}`}
            style={{
              display: 'inline-block',
              transformOrigin: '50% 80%',
              transform: `translate(${enterX + exitX}px, ${enterY + exitY}px) rotate(${enterRotate}deg)`,
              opacity,
              color: accentIndices.includes(i) ? COLORS.accent : undefined,
              willChange: 'transform',
              ...charStyle,
            }}
          >
            {char}
          </span>
        );
      })}
    </div>
  );
};
