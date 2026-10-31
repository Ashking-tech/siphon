import type {CSSProperties, ReactNode} from 'react';
import {spring, useCurrentFrame, useVideoConfig} from 'remotion';

export type TypeScaleProps = {
  children: ReactNode;
  start?: number;
  duration?: number;
  from?: number;
  to?: number;
  x?: number;
  y?: number;
  rotate?: number;
  origin?: string;
  style?: CSSProperties;
  damping?: number;
  stiffness?: number;
};

/**
 * Spring-driven type scaling for the "number expands beyond the frame" beats.
 * Transform origin is explicit so every composition decision is intentional.
 */
export const TypeScale: React.FC<TypeScaleProps> = ({
  children,
  start = 0,
  duration = 24,
  from = 1.16,
  to = 1,
  x = 0,
  y = 0,
  rotate = 0,
  origin = '50% 60%',
  style,
  damping = 22,
  stiffness = 130,
}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const p = spring({
    frame: frame - start,
    fps,
    config: {damping, stiffness, mass: 1.05},
    durationInFrames: duration,
  });
  const scale = from + (to - from) * p;

  return (
    <div
      style={{
        transform: `translate(${x}px, ${y}px) scale(${scale}) rotate(${rotate}deg)`,
        transformOrigin: origin,
        willChange: 'transform',
        ...style,
      }}
    >
      {children}
    </div>
  );
};
