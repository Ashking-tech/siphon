import type {CSSProperties, ReactNode} from 'react';
import {useCurrentFrame} from 'remotion';
import {EASE, range} from '../lib/anim';

type Direction = 'up' | 'down' | 'left' | 'right';

export type MaskedTextProps = {
  children: ReactNode;
  start?: number;
  duration?: number;
  direction?: Direction;
  exitStart?: number;
  exitDuration?: number;
  exitDirection?: Direction;
  style?: CSSProperties;
  innerStyle?: CSSProperties;
};

const hiddenTransform = (direction: Direction, distance: number) => {
  switch (direction) {
    case 'up':
      return `translateY(${distance}%)`;
    case 'down':
      return `translateY(${-distance}%)`;
    case 'left':
      return `translateX(${distance}%)`;
    case 'right':
      return `translateX(${-distance}%)`;
  }
};

/**
 * A true typographic mask: the outer box is the clip, the inner line travels.
 * Used for hard editorial entrances and exits, never as a plain fade.
 */
export const MaskedText: React.FC<MaskedTextProps> = ({
  children,
  start = 0,
  duration = 16,
  direction = 'up',
  exitStart,
  exitDuration = 10,
  exitDirection = 'up',
  style,
  innerStyle,
}) => {
  const frame = useCurrentFrame();
  const enter = range(frame, start, start + duration, 0, 1, EASE.outExpo);
  const exit =
    exitStart === undefined ? 0 : range(frame, exitStart, exitStart + exitDuration, 0, 1, EASE.inExpo);

  const enterTransform = hiddenTransform(direction, (1 - enter) * 108);
  const exitTransform = hiddenTransform(exitDirection, -exit * 108);

  return (
    <div
      style={{
        display: 'block',
        overflow: 'hidden',
        paddingTop: '0.03em',
        paddingBottom: '0.1em',
        marginBottom: '-0.1em',
        ...style,
      }}
    >
      <div
        style={{
          display: 'block',
          transform: `${enterTransform} ${exitTransform}`,
          willChange: 'transform',
          ...innerStyle,
        }}
      >
        {children}
      </div>
    </div>
  );
};
