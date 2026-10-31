import {shade} from './color';
import {
  add,
  dot,
  hull2d,
  lerp3,
  normalize,
  type CameraFrame,
  type ProjectedPoint,
  type Vec3,
  projectPoint,
  rotateEuler,
  sub,
} from './vec';

export type FaceName = 'front' | 'back' | 'left' | 'right' | 'top' | 'bottom';
export type BoxStyle = 'solid' | 'glass' | 'wire';

export type BoxDef = {
  id: string;
  center: Vec3;
  size: Vec3;
  color: string;
  rotation?: Vec3;
  opacity?: number;
  style?: BoxStyle;
  castShadow?: boolean;
  shadowStrength?: number;
  edge?: string;
  edgeOpacity?: number;
  faceColors?: Partial<Record<FaceName, string>>;
  /** CSS drop-shadow colour, used sparingly for active objects. */
  glow?: string;
};

export type StageFace = {
  boxId: string;
  name: FaceName;
  points: ProjectedPoint[];
  normal: Vec3;
  depth: number;
  color: string;
  edge: string;
  edgeOpacity: number;
  opacity: number;
  style: BoxStyle;
  glow?: string;
};

export type WireBox = {
  boxId: string;
  color: string;
  opacity: number;
  segments: Array<{a: ProjectedPoint; b: ProjectedPoint}>;
};

const LIGHT = normalize([-0.42, 0.86, 0.52]);

export const boxCorners = (box: BoxDef): Vec3[] => {
  const [sx, sy, sz] = box.size;
  const locals: Vec3[] = [
    [-sx / 2, -sy / 2, -sz / 2],
    [sx / 2, -sy / 2, -sz / 2],
    [sx / 2, sy / 2, -sz / 2],
    [-sx / 2, sy / 2, -sz / 2],
    [-sx / 2, -sy / 2, sz / 2],
    [sx / 2, -sy / 2, sz / 2],
    [sx / 2, sy / 2, sz / 2],
    [-sx / 2, sy / 2, sz / 2],
  ];
  return locals.map((p) => add(box.center, rotateEuler(p, box.rotation ?? [0, 0, 0])));
};

export const boxFaceGeometry = (box: BoxDef) => {
  const c = boxCorners(box);
  return [
    {name: 'front' as const, indices: [4, 5, 6, 7], normal: [0, 0, 1] as Vec3},
    {name: 'back' as const, indices: [1, 0, 3, 2], normal: [0, 0, -1] as Vec3},
    {name: 'right' as const, indices: [5, 1, 2, 6], normal: [1, 0, 0] as Vec3},
    {name: 'left' as const, indices: [0, 4, 7, 3], normal: [-1, 0, 0] as Vec3},
    {name: 'top' as const, indices: [7, 6, 2, 3], normal: [0, 1, 0] as Vec3},
    {name: 'bottom' as const, indices: [0, 1, 5, 4], normal: [0, -1, 0] as Vec3},
  ].map((face) => ({
    ...face,
    points: face.indices.map((i) => c[i]),
    normal: rotateEuler(face.normal, box.rotation ?? [0, 0, 0]),
  }));
};

const faceLitColor = (base: string, normal: Vec3, topBias = 0) => {
  const diffuse = Math.max(0, dot(normal, LIGHT));
  const intensity = 0.70 + 0.34 * diffuse + topBias;
  return shade(base, (intensity - 1) * 0.92);
};

export const computeFaces = (boxes: BoxDef[], cam: CameraFrame): StageFace[] => {
  const faces: StageFace[] = [];
  for (const box of boxes) {
    if ((box.style ?? 'solid') === 'wire') continue;
    const opacity = box.opacity ?? 1;
    if (opacity <= 0.002) continue;
    const geometry = boxFaceGeometry(box);
    for (const face of geometry) {
      const centerWorld = face.points.reduce<Vec3>((acc, p) => add(acc, p), [0, 0, 0]);
      const center = [centerWorld[0] / 4, centerWorld[1] / 4, centerWorld[2] / 4] as Vec3;
      const toCamera = sub(cam.position, center);
      if (dot(face.normal, toCamera) <= 0.001) continue;
      const projected = face.points.map((p) => projectPoint(p, cam));
      if (projected.some((p) => !p.visible)) continue;
      const depth = projected.reduce((sum, p) => sum + p.depth, 0) / 4;
      const base = box.faceColors?.[face.name] ?? box.color;
      const lit = faceLitColor(base, face.normal, face.name === 'top' ? 0.055 : 0);
      faces.push({
        boxId: box.id,
        name: face.name,
        points: projected,
        normal: face.normal,
        depth,
        color: lit,
        edge: box.edge ?? shade(lit, -0.32),
        edgeOpacity: box.edgeOpacity ?? 0.55,
        opacity: (box.style ?? 'solid') === 'glass' ? opacity * 0.34 : opacity,
        style: box.style ?? 'solid',
        glow: box.glow,
      });
    }
  }
  return faces.sort((a, b) => b.depth - a.depth);
};

const EDGES: Array<[number, number]> = [
  [0, 1],
  [1, 2],
  [2, 3],
  [3, 0],
  [4, 5],
  [5, 6],
  [6, 7],
  [7, 4],
  [0, 4],
  [1, 5],
  [2, 6],
  [3, 7],
];

export const computeWireBoxes = (boxes: BoxDef[], cam: CameraFrame): WireBox[] => {
  const out: WireBox[] = [];
  for (const box of boxes) {
    if ((box.style ?? 'solid') !== 'wire') continue;
    const opacity = box.opacity ?? 1;
    if (opacity <= 0.002) continue;
    const corners = boxCorners(box).map((p) => projectPoint(p, cam));
    if (corners.some((p) => !p.visible)) continue;
    out.push({
      boxId: box.id,
      color: box.edge ?? box.color,
      opacity,
      segments: EDGES.map(([a, b]) => ({a: corners[a], b: corners[b]})),
    });
  }
  return out;
};

const SHADOW_LIGHT = normalize([-0.38, -1, 0.22]);

export type ShadowPolygon = {
  boxId: string;
  cast: ProjectedPoint[];
  contact: ProjectedPoint[];
  height: number;
  strength: number;
  depth: number;
};

export const computeShadow = (box: BoxDef, cam: CameraFrame, floorY: number): ShadowPolygon | null => {
  if (box.castShadow === false) return null;
  const opacity = box.opacity ?? 1;
  if (opacity < 0.03) return null;
  const corners = boxCorners(box);
  const minY = Math.min(...corners.map((p) => p[1]));
  if (minY <= floorY + 0.01) {
    // Still allow a contact shadow for boxes resting on the floor.
  }
  const castPoints: Vec3[] = [];
  for (const p of corners) {
    const dy = p[1] - floorY;
    if (dy <= 0.002) {
      castPoints.push([p[0], floorY, p[2]]);
      continue;
    }
    const t = dy / -SHADOW_LIGHT[1];
    castPoints.push([p[0] + SHADOW_LIGHT[0] * t, floorY, p[2] + SHADOW_LIGHT[2] * t]);
  }
  const castProjected = castPoints.map((p) => projectPoint(p, cam));
  const contactWorld: Vec3[] = [
    [corners[0][0], floorY, corners[0][2]],
    [corners[1][0], floorY, corners[1][2]],
    [corners[2][0], floorY, corners[2][2]],
    [corners[3][0], floorY, corners[3][2]],
  ];
  const contactProjected = contactWorld.map((p) => projectPoint(p, cam));
  if (castProjected.some((p) => !p.visible) || contactProjected.some((p) => !p.visible)) return null;

  const castHull = hull2d(castProjected.map((p) => [p.x, p.y] as const));
  const contactHull = hull2d(contactProjected.map((p) => [p.x, p.y] as const));
  const toHull = (hull: readonly (readonly [number, number])[]): ProjectedPoint[] =>
    hull.map(([x, y]) => ({x, y, depth: 1, visible: true}));

  const height = Math.max(0, Math.max(...corners.map((p) => p[1])) - floorY);
  const depth = castProjected.reduce((s, p) => s + p.depth, 0) / castProjected.length;
  return {
    boxId: box.id,
    cast: toHull(castHull),
    contact: toHull(contactHull),
    height,
    strength: (box.shadowStrength ?? 1) * opacity * Math.max(0.25, 1 - height * 0.12),
    depth,
  };
};

export const computeShadows = (boxes: BoxDef[], cam: CameraFrame, floorY: number) =>
  boxes
    .map((box) => computeShadow(box, cam, floorY))
    .filter((s): s is ShadowPolygon => s !== null)
    .sort((a, b) => b.depth - a.depth);

/**
 * A 3D polyline helper used for waveforms and technical traces on faces.
 */
export const faceTrace = (
  a: Vec3,
  b: Vec3,
  count: number,
  height: number,
  phase: number,
  cam: CameraFrame,
) => {
  const points = [];
  for (let i = 0; i <= count; i++) {
    const t = i / count;
    const base = lerp3(a, b, t);
    const envelope = Math.sin(Math.PI * t);
    const wave =
      Math.sin(t * Math.PI * 8 + phase) * 0.55 +
      Math.sin(t * Math.PI * 17 + phase * 1.7) * 0.28 +
      Math.sin(t * Math.PI * 31 + phase * 0.6) * 0.17;
    points.push(projectPoint([base[0], base[1] + wave * height * envelope, base[2]], cam));
  }
  return points;
};

export const polyline = (points: ProjectedPoint[]) =>
  points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ');

export const polygon = (points: ProjectedPoint[]) =>
  points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(' ') + ' Z';

export const boxAt = ({
  id,
  position,
  size,
  color,
  rotation,
  opacity,
  style,
  faceColors,
  edge,
  edgeOpacity,
  castShadow,
  shadowStrength,
  glow,
}: {
  id: string;
  position: Vec3;
  size: Vec3;
  color: string;
  rotation?: Vec3;
  opacity?: number;
  style?: BoxStyle;
  faceColors?: Partial<Record<FaceName, string>>;
  edge?: string;
  edgeOpacity?: number;
  castShadow?: boolean;
  shadowStrength?: number;
  glow?: string;
}): BoxDef => ({
  id,
  center: position,
  size,
  color,
  rotation,
  opacity,
  style,
  faceColors,
  edge,
  edgeOpacity,
  castShadow,
  shadowStrength,
  glow,
});
