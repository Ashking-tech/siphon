import React, {useId} from 'react';
import {AbsoluteFill, staticFile} from 'remotion';

export const FilmGrain: React.FC<{opacity?: number}> = ({opacity = 0.045}) => (
  <AbsoluteFill
    style={{
      backgroundImage: `url(${staticFile('grain.png')})`,
      backgroundSize: 'cover',
      backgroundPosition: 'center',
      opacity,
      mixBlendMode: 'soft-light',
      pointerEvents: 'none',
      zIndex: 70,
    }}
  />
);

export const Vignette: React.FC<{strength?: number}> = ({strength = 0.16}) => (
  <AbsoluteFill
    style={{
      background: `radial-gradient(ellipse at 50% 47%, rgba(0,0,0,0) 46%, rgba(0,0,0,${strength * 0.5}) 76%, rgba(0,0,0,${strength}) 100%)`,
      pointerEvents: 'none',
      zIndex: 71,
    }}
  />
);

export const StudioGlow: React.FC<{
  x: string;
  y: string;
  size: string;
  color: string;
  opacity: number;
  blur?: number;
}> = ({x, y, size, color, opacity, blur = 80}) => (
  <AbsoluteFill style={{pointerEvents: 'none'}}>
    <div
      style={{
        position: 'absolute',
        left: x,
        top: y,
        width: size,
        height: size,
        transform: 'translate(-50%, -50%)',
        borderRadius: '50%',
        background: color,
        filter: `blur(${blur}px)`,
        opacity,
      }}
    />
  </AbsoluteFill>
);

/** A very light crop-mark frame that keeps the composition feeling printed. */
export const FrameMarks: React.FC<{color?: string; opacity?: number}> = ({
  color = '#171717',
  opacity = 0.16,
}) => {
  const uid = useId().replace(/[^a-zA-Z0-9_-]/g, '');
  const len = 18;
  const inset = 46;
  const marks = [
    {x: inset, y: inset, h: [1, 1]},
    {x: 1920 - inset, y: inset, h: [-1, 1]},
    {x: inset, y: 1080 - inset, h: [1, -1]},
    {x: 1920 - inset, y: 1080 - inset, h: [-1, -1]},
  ];
  return (
    <svg
      viewBox="0 0 1920 1080"
      width={1920}
      height={1080}
      style={{position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 60}}
      aria-hidden
    >
      <g data-id={uid} stroke={color} strokeOpacity={opacity} strokeWidth={1}>
        {marks.map((m, i) => (
          <g key={i}>
            <line x1={m.x} y1={m.y} x2={m.x + len * m.h[0]} y2={m.y} />
            <line x1={m.x} y1={m.y} x2={m.x} y2={m.y + len * m.h[1]} />
          </g>
        ))}
      </g>
    </svg>
  );
};
