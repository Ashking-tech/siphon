export type Vec3 = readonly [number, number, number];
export type Vec2 = readonly [number, number];

export const v3 = (x: number, y: number, z: number): Vec3 => [x, y, z];

export const add = (a: Vec3, b: Vec3): Vec3 => [a[0] + b[0], a[1] + b[1], a[2] + b[2]];
export const sub = (a: Vec3, b: Vec3): Vec3 => [a[0] - b[0], a[1] - b[1], a[2] - b[2]];
export const scale = (a: Vec3, s: number): Vec3 => [a[0] * s, a[1] * s, a[2] * s];
export const dot = (a: Vec3, b: Vec3) => a[0] * b[0] + a[1] * b[1] + a[2] * b[2];
export const cross = (a: Vec3, b: Vec3): Vec3 => [
  a[1] * b[2] - a[2] * b[1],
  a[2] * b[0] - a[0] * b[2],
  a[0] * b[1] - a[1] * b[0],
];
export const length = (a: Vec3) => Math.sqrt(dot(a, a));
export const normalize = (a: Vec3): Vec3 => {
  const l = Math.max(1e-9, length(a));
  return [a[0] / l, a[1] / l, a[2] / l];
};
export const lerp3 = (a: Vec3, b: Vec3, t: number): Vec3 => [
  a[0] + (b[0] - a[0]) * t,
  a[1] + (b[1] - a[1]) * t,
  a[2] + (b[2] - a[2]) * t,
];
export const rotateX = (p: Vec3, angle: number): Vec3 => {
  const c = Math.cos(angle);
  const s = Math.sin(angle);
  return [p[0], p[1] * c - p[2] * s, p[1] * s + p[2] * c];
};
export const rotateY = (p: Vec3, angle: number): Vec3 => {
  const c = Math.cos(angle);
  const s = Math.sin(angle);
  return [p[0] * c + p[2] * s, p[1], -p[0] * s + p[2] * c];
};
export const rotateZ = (p: Vec3, angle: number): Vec3 => {
  const c = Math.cos(angle);
  const s = Math.sin(angle);
  return [p[0] * c - p[1] * s, p[0] * s + p[1] * c, p[2]];
};

export const rotateEuler = (p: Vec3, rotation: Vec3): Vec3 => {
  let out = rotateX(p, rotation[0]);
  out = rotateY(out, rotation[1]);
  out = rotateZ(out, rotation[2]);
  return out;
};

export type CameraSpec = {
  target: Vec3;
  /** Degrees. Positive moves the camera to the right of the target. */
  yaw: number;
  /** Degrees. Positive moves the camera above the target. */
  pitch: number;
  distance: number;
  fov?: number;
};

export type CameraFrame = {
  position: Vec3;
  target: Vec3;
  right: Vec3;
  up: Vec3;
  forward: Vec3;
  focal: number;
  cx: number;
  cy: number;
  width: number;
  height: number;
};

const DEG = Math.PI / 180;

export const buildCamera = (spec: CameraSpec, width: number, height: number): CameraFrame => {
  const yaw = spec.yaw * DEG;
  const pitch = spec.pitch * DEG;
  const offset: Vec3 = [
    Math.sin(yaw) * Math.cos(pitch) * spec.distance,
    Math.sin(pitch) * spec.distance,
    Math.cos(yaw) * Math.cos(pitch) * spec.distance,
  ];
  const position = add(spec.target, offset);
  const forward = normalize(sub(spec.target, position));
  const right = normalize(cross(forward, [0, 1, 0]));
  const up = normalize(cross(right, forward));
  const fov = spec.fov ?? 34;
  const focal = height / 2 / Math.tan((fov * DEG) / 2);
  return {
    position,
    target: spec.target,
    right,
    up,
    forward,
    focal,
    cx: width / 2,
    cy: height / 2,
    width,
    height,
  };
};

export type ProjectedPoint = {
  x: number;
  y: number;
  depth: number;
  visible: boolean;
};

export const projectPoint = (p: Vec3, cam: CameraFrame): ProjectedPoint => {
  const rel = sub(p, cam.position);
  const depth = dot(rel, cam.forward);
  const inv = depth > 0.001 ? cam.focal / depth : 0;
  return {
    x: cam.cx + dot(rel, cam.right) * inv,
    y: cam.cy - dot(rel, cam.up) * inv,
    depth,
    visible: depth > 0.15,
  };
};

export const projectPoints = (points: Vec3[], cam: CameraFrame): ProjectedPoint[] => points.map((p) => projectPoint(p, cam));

export const pathFromPoints = (points: Array<{x: number; y: number}>) =>
  points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ') + (points.length > 2 ? ' Z' : '');

export const polylineFromPoints = (points: Array<{x: number; y: number}>) =>
  points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ');

export const hull2d = (points: Vec2[]): Vec2[] => {
  if (points.length <= 2) return points;
  const pts = [...points].sort((a, b) => a[0] - b[0] || a[1] - b[1]);
  const cross2 = (o: Vec2, a: Vec2, b: Vec2) => (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0]);
  const lower: Vec2[] = [];
  for (const p of pts) {
    while (lower.length >= 2 && cross2(lower[lower.length - 2], lower[lower.length - 1], p) <= 0) lower.pop();
    lower.push(p);
  }
  const upper: Vec2[] = [];
  for (let i = pts.length - 1; i >= 0; i--) {
    const p = pts[i];
    while (upper.length >= 2 && cross2(upper[upper.length - 2], upper[upper.length - 1], p) <= 0) upper.pop();
    upper.push(p);
  }
  upper.pop();
  lower.pop();
  return [...lower, ...upper];
};
