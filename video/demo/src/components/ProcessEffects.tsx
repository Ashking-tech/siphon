import React from 'react';
import {COLORS} from '../theme';
import type {CameraFrame, Vec3} from '../lib/vec';
import {WorldLine, WorldPolygon, WorldRing} from './World';

export const ScannerPlane: React.FC<{
  camera: CameraFrame;
  x: number;
  y?: number;
  height?: number;
  depth?: number;
  opacity?: number;
  color?: string;
  lineCount?: number;
  glow?: boolean;
}> = ({
  camera,
  x,
  y = 0.18,
  height = 2.45,
  depth = 2.1,
  opacity = 1,
  color = COLORS.blue,
  lineCount = 10,
  glow = true,
}) => {
  const top = y + height / 2;
  const bottom = y - height / 2;
  const front = depth / 2;
  const back = -depth / 2;
  const plane: Vec3[] = [
    [x, bottom, back],
    [x, bottom, front],
    [x, top, front],
    [x, top, back],
  ];
  const lines = [];
  for (let i = 0; i <= lineCount; i++) {
    const yy = bottom + (height * i) / lineCount;
    lines.push(
      <WorldLine
        key={i}
        camera={camera}
        from={[x, yy, back]}
        to={[x, yy, front]}
        color={color}
        opacity={0.18 * opacity}
        strokeWidth={0.8}
      />,
    );
  }
  return (
    <g>
      <WorldPolygon
        camera={camera}
        points={plane}
        color={color}
        fill={color}
        fillOpacity={0.075 * opacity}
        stroke={color}
        strokeOpacity={0.34 * opacity}
        strokeWidth={1}
      />
      {lines}
      <WorldLine
        camera={camera}
        from={[x + 0.003, bottom, front]}
        to={[x + 0.003, top, front]}
        color={COLORS.white}
        opacity={0.84 * opacity}
        strokeWidth={2.2}
        filter={glow ? 'url(#stage-glow-blue)' : undefined}
      />
      <WorldLine
        camera={camera}
        from={[x + 0.001, bottom, back]}
        to={[x + 0.001, top, back]}
        color={color}
        opacity={0.52 * opacity}
        strokeWidth={1.4}
      />
      <WorldLine
        camera={camera}
        from={[x, top, back]}
        to={[x, top, front]}
        color={COLORS.white}
        opacity={0.4 * opacity}
        strokeWidth={1.2}
      />
    </g>
  );
};

export const VerificationEffect: React.FC<{
  camera: CameraFrame;
  center: Vec3;
  progress: number;
  opacity?: number;
  color?: string;
  radius?: number;
  floorY?: number;
}> = ({camera, center, progress, opacity = 1, color = COLORS.green, radius = 1.15, floorY = -0.72}) => {
  const p = Math.min(1, Math.max(0, progress));
  const ringRadius = 0.28 + p * radius;
  const ringOpacity = (1 - p) * opacity;
  const second = Math.max(0, p - 0.18);
  return (
    <g>
      <WorldRing
        camera={camera}
        center={center}
        u={[0, 1, 0]}
        v={[0, 0, 1]}
        radius={ringRadius}
        color={color}
        opacity={ringOpacity * 0.95}
        strokeWidth={3.2}
        filter="url(#stage-glow-green)"
      />
      <WorldRing
        camera={camera}
        center={center}
        u={[0, 1, 0]}
        v={[0, 0, 1]}
        radius={0.18 + second * (radius * 0.72)}
        color={COLORS.white}
        opacity={ringOpacity * 0.4}
        strokeWidth={1.2}
      />
      <WorldRing
        camera={camera}
        center={[center[0], floorY, center[2]]}
        u={[1, 0, 0]}
        v={[0, 0, 1]}
        radius={0.35 + p * 1.2}
        color={color}
        opacity={ringOpacity * 0.3}
        strokeWidth={1}
      />
    </g>
  );
};

export const ReclaimAnimation: React.FC<{
  camera: CameraFrame;
  origin: Vec3;
  progress: number;
  opacity?: number;
  floorY?: number;
}> = ({camera, origin, progress, opacity = 1, floorY = -0.72}) => {
  const p = Math.min(1, Math.max(0, progress));
  const floorCenter: Vec3 = [origin[0], floorY, origin[2]];
  return (
    <g opacity={opacity}>
      <WorldRing
        camera={camera}
        center={floorCenter}
        u={[1, 0, 0]}
        v={[0, 0, 1]}
        radius={0.12 + p * 0.62}
        color={COLORS.white}
        opacity={(1 - p) * 0.5}
        strokeWidth={1.4}
      />
      <WorldLine
        camera={camera}
        from={[origin[0] - 0.28, floorY + 0.02, origin[2]]}
        to={[origin[0] + 0.28, floorY + 0.02, origin[2]]}
        color={COLORS.yellow}
        opacity={(1 - p) * 0.9}
        strokeWidth={2}
        filter="url(#stage-glow-yellow)"
      />
    </g>
  );
};
