import type {CSSProperties, ReactNode} from 'react';
import {useCurrentFrame} from 'remotion';
import {EASE, range} from '../lib/anim';

export type SliceTransitionProps = {
  outgoing: ReactNode;
  incoming: ReactNode;
  start: number;
  duration?: number;
  slices?: number;
  style?: CSSProperties;
};

/**
 * Editorial shredder. Both typographic states are sliced into horizontal
 * bands; the bands trade places with a mechanical stagger. Used once, for the
 * 60 GB → 340 MB payoff.
 */
export const SliceTransition: React.FC<SliceTransitionProps> = ({
  outgoing,
  incoming,
  start,
  duration = 18,
  slices = 12,
  style,
}) => {
  const frame = useCurrentFrame();

  return (
    <div style={{position: 'absolute', inset: 0, ...style}}>
      {Array.from({length: slices}, (_, i) => {
        const localStart = start + i * 0.55;
        const p = range(frame, localStart, localStart + duration, 0, 1, EASE.inOut);
        const direction = i % 2 === 0 ? 1 : -1;
        const clip = `inset(${(i / slices) * 100}% 0 ${100 - ((i + 1) / slices) * 100}% 0)`;
        const outX = direction * p * 1150;
        const inX = -direction * (1 - p) * 900;
        const incomingVisible = p > 0.001;

        return (
          <div key={i} style={{position: 'absolute', inset: 0, clipPath: clip}}>
            <div
              style={{
                position: 'absolute',
                inset: 0,
                transform: `translateX(${outX}px)`,
                opacity: 1 - p * 0.96,
                willChange: 'transform',
              }}
            >
              {outgoing}
            </div>
            {incomingVisible ? (
              <div
                style={{
                  position: 'absolute',
                  inset: 0,
                  transform: `translateX(${inX}px)`,
                  opacity: p,
                  willChange: 'transform',
                }}
              >
                {incoming}
              </div>
            ) : null}
          </div>
        );
      })}
    </div>
  );
};
