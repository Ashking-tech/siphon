import React from 'react';
import {buildCamera, lerp3, type CameraFrame, type CameraSpec} from '../lib/vec';
import {EASE} from '../lib/anim';
import {interpolate, type EasingFunction} from 'remotion';

type CameraMotionProps = {
  frame: number;
  width: number;
  height: number;
  from: CameraSpec;
  to: CameraSpec;
  start?: number;
  end?: number;
  easing?: EasingFunction;
  /** Optional deterministic drift, in world units, layered on the target. */
  drift?: {x?: number; y?: number; period?: number};
  children: (camera: CameraFrame) => React.ReactNode;
};

/**
 * Interpolates the whole camera rig — target, yaw, pitch, distance and fov —
 * then hands the resolved camera to the scene as a render prop. Every value is
 * a pure function of the frame, so a re-render matches a re-render.
 */
export const CameraMotion: React.FC<CameraMotionProps> = ({
  frame,
  width,
  height,
  from,
  to,
  start = 0,
  end = 100,
  easing = EASE.inOut,
  drift,
  children,
}) => {
  const t = interpolate(frame, [start, end], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing,
  });
  const driftX = drift?.x ? Math.sin((frame / (drift.period ?? 120)) * Math.PI * 2) * drift.x : 0;
  const driftY = drift?.y ? Math.cos((frame / (drift.period ?? 120)) * Math.PI * 2) * drift.y : 0;
  const target = lerp3(from.target, to.target, t);
  const camera = buildCamera(
    {
      target: [target[0] + driftX, target[1] + driftY, target[2]],
      yaw: from.yaw + (to.yaw - from.yaw) * t,
      pitch: from.pitch + (to.pitch - from.pitch) * t,
      distance: from.distance + (to.distance - from.distance) * t,
      fov: (from.fov ?? 34) + ((to.fov ?? 34) - (from.fov ?? 34)) * t,
    },
    width,
    height,
  );
  return <>{children(camera)}</>;
};
