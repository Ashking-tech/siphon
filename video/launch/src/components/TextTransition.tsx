import type {CSSProperties, ReactNode} from 'react';
import {useCurrentFrame} from 'remotion';
import {EASE, range} from '../lib/anim';

type Direction = 'up' | 'down' | 'left' | 'right';

export type TextTransitionProps = {
  outgoing: ReactNode;
  incoming: ReactNode;
  start: number;
  duration?: number;
  direction?: Direction;
  style?: CSSProperties;
};

/**
 * A reusable full-frame transition between two typographic states. Both
 * states are real composed layers; the transition moves and clips them rather
 * than fading a single element.
 */
export const TextTransition: React.FC<TextTransitionProps> = ({
  outgoing,
  incoming,
  start,
  duration = 9,
  direction = 'up',
  style,
}) => {
  const frame = useCurrentFrame();
  const p = range(frame, start, start + duration, 0, 1, EASE.inOut);

  const outgoingTransform = (() => {
    switch (direction) {
      case 'up':
        return `translateY(${-p * 104}%) skewY(${-p * 1.1}deg)`;
      case 'down':
        return `translateY(${p * 104}%) skewY(${p * 1.1}deg)`;
      case 'left':
        return `translateX(${-p * 104}%)`;
      case 'right':
        return `translateX(${p * 104}%)`;
    }
  })();

  const incomingTransform = (() => {
    switch (direction) {
      case 'up':
        return `translateY(${(1 - p) * 104}%) skewY(${(1 - p) * 1.1}deg)`;
      case 'down':
        return `translateY(${-(1 - p) * 104}%) skewY(${-(1 - p) * 1.1}deg)`;
      case 'left':
        return `translateX(${(1 - p) * 104}%)`;
      case 'right':
        return `translateX(${-(1 - p) * 104}%)`;
    }
  })();

  return (
    <div style={{position: 'absolute', inset: 0, overflow: 'hidden', ...style}}>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          transform: outgoingTransform,
          willChange: 'transform',
        }}
      >
        {outgoing}
      </div>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          transform: incomingTransform,
          willChange: 'transform',
        }}
      >
        {incoming}
      </div>
    </div>
  );
};
