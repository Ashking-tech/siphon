import type {CSSProperties, ReactNode} from 'react';
import {useCurrentFrame} from 'remotion';
import {EASE, range} from '../lib/anim';

export type TrackingRevealProps = {
  text: string;
  trailing?: ReactNode;
  start?: number;
  duration?: number;
  fromTracking?: number;
  toTracking?: number;
  style?: CSSProperties;
  wipe?: boolean;
  exitStart?: number;
  exitDuration?: number;
  exitTracking?: number;
};

/**
 * The product-name reveal: massive tracking and a centre-out clip wipe, with
 * the type settling rather than fading.
 */
export const TrackingReveal: React.FC<TrackingRevealProps> = ({
  text,
  trailing,
  start = 0,
  duration = 30,
  fromTracking = 0.34,
  toTracking = 0.035,
  style,
  wipe = true,
  exitStart,
  exitDuration = 14,
  exitTracking = 0.24,
}) => {
  const frame = useCurrentFrame();
  const enter = range(frame, start, start + duration, 0, 1, EASE.outExpo);
  const tracking = fromTracking + (toTracking - fromTracking) * enter;
  const exit =
    exitStart === undefined ? 0 : range(frame, exitStart, exitStart + exitDuration, 0, 1, EASE.inExpo);
  const finalTracking = tracking + exit * (exitTracking - tracking);
  const inset = wipe ? (1 - enter) * 49 : 0;
  const opacity = range(frame, start, start + 8, 0, 1, EASE.outExpo) * (1 - exit * 0.9);

  return (
    <div
      style={{
        display: 'block',
        whiteSpace: 'nowrap',
        letterSpacing: `${finalTracking}em`,
        clipPath: `inset(-8% ${inset}% -8% ${inset}%)`,
        opacity,
        willChange: 'letter-spacing, clip-path',
        ...style,
      }}
    >
      {text}
      {trailing}
    </div>
  );
};
