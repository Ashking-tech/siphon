import React from 'react';
import {pathFromPoints, polylineFromPoints, projectPoints, type CameraFrame, type Vec3} from '../lib/vec';

type Common = {
  camera: CameraFrame;
  color: string;
  opacity?: number;
  strokeWidth?: number;
  dash?: string;
  filter?: string;
  linecap?: 'butt' | 'round' | 'square';
};

export const WorldLine: React.FC<Common & {from: Vec3; to: Vec3}> = ({
  camera,
  from,
  to,
  color,
  opacity = 1,
  strokeWidth = 1.2,
  dash,
  filter,
  linecap = 'round',
}) => {
  const [a, b] = projectPoints([from, to], camera);
  if (!a.visible || !b.visible) return null;
  return (
    <line
      x1={a.x}
      y1={a.y}
      x2={b.x}
      y2={b.y}
      stroke={color}
      strokeOpacity={opacity}
      strokeWidth={strokeWidth}
      strokeDasharray={dash}
      strokeLinecap={linecap}
      filter={filter}
    />
  );
};

export const WorldPolyline: React.FC<Common & {points: Vec3[]}> = ({
  camera,
  points,
  color,
  opacity = 1,
  strokeWidth = 1.5,
  dash,
  filter,
  linecap = 'round',
}) => {
  const projected = projectPoints(points, camera);
  if (projected.some((p) => !p.visible)) return null;
  return (
    <path
      d={polylineFromPoints(projected)}
      fill="none"
      stroke={color}
      strokeOpacity={opacity}
      strokeWidth={strokeWidth}
      strokeDasharray={dash}
      strokeLinecap={linecap}
      filter={filter}
    />
  );
};

export const WorldPolygon: React.FC<
  Common & {points: Vec3[]; fill?: string; fillOpacity?: number; stroke?: string; strokeOpacity?: number}
> = ({
  camera,
  points,
  color,
  fill,
  fillOpacity = 1,
  opacity = 1,
  stroke,
  strokeOpacity = 1,
  strokeWidth = 1.2,
  dash,
  filter,
}) => {
  const projected = projectPoints(points, camera);
  if (projected.some((p) => !p.visible)) return null;
  return (
    <path
      d={pathFromPoints(projected)}
      fill={fill ?? color}
      fillOpacity={fillOpacity * opacity}
      stroke={stroke ?? color}
      strokeOpacity={strokeOpacity * opacity}
      strokeWidth={strokeWidth}
      strokeDasharray={dash}
      filter={filter}
      strokeLinejoin="round"
    />
  );
};

export const WorldRing: React.FC<
  Common & {
    center: Vec3;
    u: Vec3;
    v: Vec3;
    radius: number;
    fill?: string;
    fillOpacity?: number;
    segments?: number;
  }
> = ({
  camera,
  center,
  u,
  v,
  radius,
  color,
  fill = 'none',
  fillOpacity = 0,
  opacity = 1,
  strokeWidth = 2,
  dash,
  segments = 72,
  filter,
}) => {
  const points: Vec3[] = [];
  for (let i = 0; i <= segments; i++) {
    const angle = (i / segments) * Math.PI * 2;
    const c = Math.cos(angle) * radius;
    const s = Math.sin(angle) * radius;
    points.push([
      center[0] + u[0] * c + v[0] * s,
      center[1] + u[1] * c + v[1] * s,
      center[2] + u[2] * c + v[2] * s,
    ]);
  }
  const projected = projectPoints(points, camera);
  if (projected.some((p) => !p.visible)) return null;
  return (
    <path
      d={pathFromPoints(projected)}
      fill={fill}
      fillOpacity={fillOpacity * opacity}
      stroke={color}
      strokeOpacity={opacity}
      strokeWidth={strokeWidth}
      strokeDasharray={dash}
      filter={filter}
      strokeLinejoin="round"
    />
  );
};

export const WorldDiamond: React.FC<
  Common & {center: Vec3; size: number; fillOpacity?: number}
> = ({center, size, color, fillOpacity = 0.9, opacity = 1, strokeWidth = 1, camera, filter}) => {
  const points: Vec3[] = [
    [center[0], center[1] + size, center[2]],
    [center[0] + size, center[1], center[2]],
    [center[0], center[1] - size, center[2]],
    [center[0] - size, center[1], center[2]],
  ];
  return (
    <WorldPolygon
      camera={camera}
      points={points}
      color={color}
      fill={color}
      fillOpacity={fillOpacity}
      opacity={opacity}
      stroke={color}
      strokeOpacity={0.9}
      strokeWidth={strokeWidth}
      filter={filter}
    />
  );
};

export const WorldCheck: React.FC<Common & {center: Vec3; size: number}> = ({
  center,
  size,
  color,
  opacity = 1,
  strokeWidth = 4,
  camera,
  filter,
}) => {
  const a: Vec3 = [center[0] - size * 0.55, center[1], center[2]];
  const b: Vec3 = [center[0] - size * 0.12, center[1] - size * 0.48, center[2]];
  const c: Vec3 = [center[0] + size * 0.62, center[1] + size * 0.42, center[2]];
  const points = projectPoints([a, b, c], camera);
  if (points.some((p) => !p.visible)) return null;
  return (
    <path
      d={polylineFromPoints(points)}
      fill="none"
      stroke={color}
      strokeOpacity={opacity}
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
      filter={filter}
    />
  );
};

export const PlayMark: React.FC<Common & {center: Vec3; size: number}> = ({
  center,
  size,
  color,
  opacity = 1,
  camera,
  filter,
}) => {
  const points: Vec3[] = [
    [center[0] - size * 0.36, center[1] + size * 0.58, center[2]],
    [center[0] + size * 0.6, center[1], center[2]],
    [center[0] - size * 0.36, center[1] - size * 0.58, center[2]],
  ];
  return (
    <WorldPolygon
      camera={camera}
      points={points}
      color={color}
      fill={color}
      fillOpacity={0.92}
      opacity={opacity}
      stroke="none"
      strokeWidth={0}
      filter={filter}
    />
  );
};
