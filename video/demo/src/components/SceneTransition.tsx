import React from 'react';
import {AbsoluteFill, interpolate, useCurrentFrame} from 'remotion';
import {COLORS} from '../theme';
import {EASE, clamp01, smoothstep} from '../lib/anim';

type Variant = 'scan' | 'blade' | 'bloom';

type SceneTransitionProps = {
  from: number;
  duration: number;
  variant: Variant;
};

/**
 * A geometric wipe that hides a scene cut at its midpoint.
 * 0 → 0.5 covers the outgoing frame; 0.5 → 1 reveals the incoming frame.
 */
export const SceneTransition: React.FC<SceneTransitionProps> = ({from, duration, variant}) => {
  const frame = useCurrentFrame();
  const local = frame - from;
  if (local < 0 || local > duration) return null;
  const p = clamp01(local / duration);

  if (variant === 'bloom') {
    const scale = interpolate(p, [0, 0.5, 1], [0, 1.45, 3.4], {extrapolateLeft: 'clamp', extrapolateRight: 'clamp'});
    const opacity = interpolate(p, [0, 0.48, 0.58, 1], [1, 1, 0.92, 0], {
      extrapolateLeft: 'clamp',
      extrapolateRight: 'clamp',
    });
    return (
      <AbsoluteFill style={{pointerEvents: 'none', zIndex: 50, overflow: 'hidden'}}>
        <div
          style={{
            position: 'absolute',
            left: '50%',
            top: '50%',
            width: '210vmax',
            height: '210vmax',
            borderRadius: '50%',
            transform: `translate(-50%, -50%) scale(${scale})`,
            background: `radial-gradient(circle, ${COLORS.white} 0%, ${COLORS.white} 62%, ${COLORS.paperWarm} 100%)`,
            boxShadow: `0 0 0 3px ${COLORS.green}55, 0 0 120px 20px ${COLORS.green}22`,
            opacity,
          }}
        />
        <div
          style={{
            position: 'absolute',
            left: '50%',
            top: '50%',
            width: '74vmax',
            height: '74vmax',
            borderRadius: '50%',
            border: `1px solid ${COLORS.blue}33`,
            transform: `translate(-50%, -50%) scale(${scale * 0.96})`,
            opacity: Math.max(0, 1 - p * 1.4),
          }}
        />
      </AbsoluteFill>
    );
  }

  const travel = interpolate(p, [0, 0.5, 1], [-168, 0, 168], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });

  const isBlade = variant === 'blade';
  const background = isBlade
    ? `linear-gradient(104deg, ${COLORS.blueDeep} 0%, ${COLORS.blue} 55%, ${COLORS.green} 55.2%, ${COLORS.green} 100%)`
    : `linear-gradient(102deg, ${COLORS.blueDeep} 0%, ${COLORS.blue} 44%, ${COLORS.blue} 100%)`;

  return (
    <AbsoluteFill style={{pointerEvents: 'none', zIndex: 50, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '50%',
          width: '132vw',
          height: '165vh',
          transform: `translate(-50%, -50%) translateX(${travel}vw) skewX(-9deg)`,
          background,
          boxShadow: `0 0 90px 14px ${isBlade ? COLORS.green : COLORS.blue}55`,
        }}
      >
        <div
          style={{
            position: 'absolute',
            inset: 0,
            backgroundImage:
              'repeating-linear-gradient(90deg, rgba(255,255,255,0.055) 0px, rgba(255,255,255,0.055) 1px, transparent 1px, transparent 14px)',
            opacity: 0.8,
          }}
        />
        <div
          style={{
            position: 'absolute',
            right: 0,
            top: 0,
            bottom: 0,
            width: 2,
            backgroundColor: isBlade ? COLORS.yellow : COLORS.white,
            opacity: 0.7,
          }}
        />
        <div
          style={{
            position: 'absolute',
            left: 0,
            top: 0,
            bottom: 0,
            width: 1,
            backgroundColor: COLORS.white,
            opacity: 0.25,
          }}
        />
      </div>

      {variant === 'scan' ? (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            background: `radial-gradient(ellipse at 50% 52%, rgba(255,255,255,0.0) 20%, rgba(49,85,231,0.18) 100%)`,
            opacity: smoothstep(0.2, 0.5, p) * (1 - smoothstep(0.55, 0.9, p)),
          }}
        />
      ) : null}
    </AbsoluteFill>
  );
};
