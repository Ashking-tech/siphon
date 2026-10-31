import React from 'react';
import {AbsoluteFill} from 'remotion';
import {C} from '../theme';

type CursorProps = {
  x: number;
  y: number;
  opacity?: number;
  /** 0 → 1 → 0 click ring, expanded over ~10 frames. */
  pulse?: number;
  color?: string;
};

/**
 * A flat Breeze-like pointer, drawn as one SVG. The hotspot sits at (2, 2).
 * No 3D, no perspective: a white keyline around near-black fill.
 */
export const Cursor: React.FC<CursorProps> = ({x, y, opacity = 1, pulse = 0, color = C.blue}) => {
  if (opacity <= 0) return null;
  const ringScale = 0.45 + pulse * 1.5;
  return (
    <AbsoluteFill style={{pointerEvents: 'none', zIndex: 90}}>
      {pulse > 0 ? (
        <div
          style={{
            position: 'absolute',
            left: x - 26,
            top: y - 26,
            width: 52,
            height: 52,
            borderRadius: 26,
            border: `2px solid ${color}`,
            transform: `scale(${ringScale})`,
            opacity: 0.55 * pulse,
          }}
        />
      ) : null}
      <svg
        width={31}
        height={41}
        viewBox="0 0 31 41"
        style={{
          position: 'absolute',
          left: x - 3,
          top: y - 2,
          opacity,
          filter: 'drop-shadow(0 1px 1px rgba(23,23,23,0.18))',
        }}
      >
        <path
          d="M3 3v27.6l6.9-6.4 5.3 12.6 4.2-1.8-5.2-12.4 9.4-.6z"
          fill={C.ink}
          stroke="#fff"
          strokeWidth="2"
          strokeLinejoin="round"
        />
      </svg>
    </AbsoluteFill>
  );
};
