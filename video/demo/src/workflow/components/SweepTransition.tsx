import React from 'react';
import {AbsoluteFill, interpolate, useCurrentFrame} from 'remotion';
import {EASE} from '../lib/motion';
import {C} from '../theme';

type Direction = 'left' | 'right' | 'up';

type SweepTransitionProps = {
  from: number;
  duration: number;
  direction: Direction;
};

/**
 * A flat white scene wipe. At the midpoint the frame is fully covered, so the
 * hard cut between scenes is never visible. The blue keyline is the only
 * decoration: it reads as a processing edge, not a camera move.
 */
export const SweepTransition: React.FC<SweepTransitionProps> = ({from, duration, direction}) => {
  const frame = useCurrentFrame();
  const local = frame - from;
  if (local < 0 || local > duration) return null;
  const p = interpolate(local, [0, duration], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });

  if (direction === 'up') {
    const top = interpolate(p, [0, 1], [100, -130]);
    return (
      <AbsoluteFill style={{pointerEvents: 'none', zIndex: 100, overflow: 'hidden'}}>
        <div
          style={{
            position: 'absolute',
            left: '-2vw',
            width: '104vw',
            height: '132vh',
            top: `${top}vh`,
            background: C.white,
            borderBottom: `2px solid ${C.blue}`,
          }}
        >
          <div
            style={{
              position: 'absolute',
              left: 0,
              right: 0,
              bottom: 18,
              height: 1,
              background: C.blue,
              opacity: 0.18,
            }}
          />
        </div>
      </AbsoluteFill>
    );
  }

  const travel = interpolate(p, [0, 1], direction === 'left' ? [-132, 102] : [102, -132]);
  const edgeOnRight = direction === 'left';
  const innerEdge: React.CSSProperties = edgeOnRight ? {right: 18} : {left: 18};
  return (
    <AbsoluteFill style={{pointerEvents: 'none', zIndex: 100, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          top: '-2vh',
          height: '104vh',
          width: '132vw',
          left: `${travel}vw`,
          background: C.white,
          borderRight: edgeOnRight ? `2px solid ${C.blue}` : undefined,
          borderLeft: edgeOnRight ? undefined : `2px solid ${C.blue}`,
        }}
      >
        <div
          style={{
            position: 'absolute',
            top: 0,
            bottom: 0,
            ...innerEdge,
            width: 1,
            background: C.blue,
            opacity: 0.18,
          }}
        />
      </div>
    </AbsoluteFill>
  );
};
