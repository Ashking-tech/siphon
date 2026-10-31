import React from 'react';
import {AbsoluteFill, useCurrentFrame, useVideoConfig} from 'remotion';
import {Display, Tech} from '../components/Type';
import {COLORS} from '../theme';
import {smoothstep} from '../lib/anim';
import {mixHex, shade} from '../lib/color';
import {buildCamera, projectPoint, type CameraFrame, type Vec2, type Vec3} from '../lib/vec';

const MARK_POINTS: Vec2[] = [
  [7, 8],
  [25, 8],
  [18.5, 17],
  [18.5, 23.5],
  [13.5, 26],
  [13.5, 17],
];

const pointInPolygon = (p: Vec2, polygon: Vec2[]) => {
  let inside = false;
  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const xi = polygon[i][0];
    const yi = polygon[i][1];
    const xj = polygon[j][0];
    const yj = polygon[j][1];
    const intersect = yi > p[1] !== yj > p[1] && p[0] < ((xj - xi) * (p[1] - yi)) / (yj - yi) + xi;
    if (intersect) inside = !inside;
  }
  return inside;
};

const mapMarkPoint = (p: Vec2, scale: number): Vec2 => [
  ((p[0] - 16) / 32) * scale,
  ((16 - p[1]) / 32) * scale,
];

type MarkFace = {
  key: string;
  points: Vec3[];
  normal: Vec3;
  color: string;
  edge: string;
};

const buildMarkFaces = (position: Vec3, scale: number, depth: number): MarkFace[] => {
  const pts = MARK_POINTS.map((p) => mapMarkPoint(p, scale));
  const front = depth / 2;
  const back = -depth / 2;
  const faces: MarkFace[] = [];

  const addFace = (key: string, points2d: Vec2[], z: number, normal: Vec3, color: string, edge: string) => {
    faces.push({
      key,
      points: points2d.map((p) => [position[0] + p[0], position[1] + p[1], position[2] + z] as Vec3),
      normal,
      color,
      edge,
    });
  };

  addFace(
    'front',
    pts,
    front,
    [0, 0, 1],
    mixHex(COLORS.blue, COLORS.white, 0.02),
    shade(COLORS.blueDeep, -0.15),
  );

  addFace(
    'back',
    pts,
    back,
    [0, 0, -1],
    mixHex(COLORS.green, COLORS.greenDeep, 0.35),
    shade(COLORS.greenDeep, -0.2),
  );

  for (let i = 0; i < pts.length; i++) {
    const a = pts[i];
    const b = pts[(i + 1) % pts.length];
    let normal: Vec2 = [b[1] - a[1], -(b[0] - a[0])];
    const len = Math.hypot(normal[0], normal[1]) || 1;
    normal = [normal[0] / len, normal[1] / len];
    const midpoint: Vec2 = [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2];
    const probe: Vec2 = [midpoint[0] + normal[0] * 0.002, midpoint[1] + normal[1] * 0.002];
    if (pointInPolygon(probe, pts)) normal = [-normal[0], -normal[1]];
    const side = i % 2 === 0 ? mixHex(COLORS.green, COLORS.white, 0.08) : mixHex(COLORS.green, COLORS.greenDeep, 0.22);
    faces.push({
      key: `side-${i}`,
      points: [
        [position[0] + a[0], position[1] + a[1], position[2] + front],
        [position[0] + b[0], position[1] + b[1], position[2] + front],
        [position[0] + b[0], position[1] + b[1], position[2] + back],
        [position[0] + a[0], position[1] + a[1], position[2] + back],
      ],
      normal: [normal[0], normal[1], 0],
      color: side,
      edge: shade(COLORS.greenDeep, -0.15),
    });
  }

  return faces;
};

const ExtrudedMark: React.FC<{camera: CameraFrame; scale: number; opacity: number}> = ({
  camera,
  scale,
  opacity,
}) => {
  const position: Vec3 = [0, 0.62, 0];
  const depth = 0.36 * scale;
  const faces = buildMarkFaces(position, 1.6 * scale, depth);
  const projectedFaces = faces
    .map((face) => {
      const center = face.points.reduce<Vec3>((acc, p) => [acc[0] + p[0] / 4, acc[1] + p[1] / 4, acc[2] + p[2] / 4], [0, 0, 0]);
      const visible =
        face.normal[0] * (camera.position[0] - center[0]) +
          face.normal[1] * (camera.position[1] - center[1]) +
          face.normal[2] * (camera.position[2] - center[2]) >
        0;
      const points = face.points.map((p) => projectPoint(p, camera));
      return {
        ...face,
        visible,
        points,
        depth: points.reduce((s, p) => s + p.depth, 0) / points.length,
      };
    })
    .filter((face) => face.visible)
    .sort((a, b) => b.depth - a.depth);

  const frontFace = projectedFaces.find((face) => face.key === 'front');
  const frontPoints = frontFace?.points ?? [];
  const xs = frontPoints.map((p) => p.x);
  const ys = frontPoints.map((p) => p.y);

  return (
    <g opacity={opacity} style={{filter: 'drop-shadow(0 24px 28px rgba(23,23,23,0.16))'}}>
      <defs>
        <linearGradient
          id="mark-front"
          gradientUnits="userSpaceOnUse"
          x1={Math.min(...xs)}
          y1={Math.min(...ys)}
          x2={Math.max(...xs)}
          y2={Math.max(...ys)}
        >
          <stop offset="0%" stopColor={mixHex(COLORS.blue, COLORS.white, 0.18)} />
          <stop offset="46%" stopColor={COLORS.blue} />
          <stop offset="100%" stopColor={COLORS.blueDeep} />
        </linearGradient>
      </defs>
      {projectedFaces.map((face) => (
        <path
          key={face.key}
          d={face.points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' ') + ' Z'}
          fill={
            face.key === 'front'
              ? 'url(#mark-front)'
              : face.key === 'back'
                ? shade(COLORS.greenDeep, -0.05)
                : shade(face.color, (0.68 + 0.42 * Math.max(0, face.normal[0] * -0.4 + face.normal[1] * 0.8) - 1) * 0.8)
          }
          stroke={face.edge}
          strokeOpacity={face.key === 'front' ? 0.55 : 0.38}
          strokeWidth={1}
          strokeLinejoin="round"
        />
      ))}
      {frontFace ? (
        <path
          d={frontFace.points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' ') + ' Z'}
          fill="none"
          stroke={mixHex(COLORS.white, COLORS.blue, 0.25)}
          strokeOpacity={0.3}
          strokeWidth={1}
        />
      ) : null}
    </g>
  );
};

export const Scene05Close: React.FC = () => {
  const f = useCurrentFrame();
  const {width, height} = useVideoConfig();
  const enter = smoothstep(2, 24, f);
  const wordIn = smoothstep(8, 24, f);
  const tagIn = smoothstep(15, 29, f);
  const ruleIn = smoothstep(14, 28, f);
  const camera = buildCamera(
    {
      target: [0, 0.2, 0],
      yaw: 12 + 11 * smoothstep(0, 60, f),
      pitch: 8 + 2 * smoothstep(0, 60, f),
      distance: 8.9 - 0.35 * smoothstep(0, 60, f),
      fov: 34,
    },
    width,
    height,
  );


  return (
    <AbsoluteFill style={{background: 'radial-gradient(ellipse at 50% 40%, #FFFFFF 0%, #FAF9F5 46%, #F2F0EA 100%)'}}>
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 452,
          width: 480,
          height: 480,
          transform: 'translate(-50%, -50%)',
          borderRadius: '50%',
          border: `1px solid ${COLORS.greyMid}`,
          opacity: enter * 0.3,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 452,
          width: 640,
          height: 640,
          transform: 'translate(-50%, -50%)',
          borderRadius: '50%',
          border: `1px solid ${COLORS.ink}`,
          opacity: enter * 0.04,
        }}
      />

      <svg
        viewBox={`0 0 ${width} ${height}`}
        width={width}
        height={height}
        style={{position: 'absolute', inset: 0, overflow: 'visible'}}
      >
        <ExtrudedMark camera={camera} scale={1.5 + 0.05 * enter} opacity={enter} />
      </svg>

      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          top: 648,
          display: 'flex',
          justifyContent: 'center',
          overflow: 'hidden',
          paddingBottom: 8,
        }}
      >
        <div
          style={{
            transform: `translateY(${(1 - wordIn) * 118}px)`,
            opacity: wordIn,
          }}
        >
          <Display size={112} weight={900} tracking={0.065} color={COLORS.ink}>
            SIPHON
          </Display>
        </div>
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 796,
          width: 210 * ruleIn,
          height: 1,
          transform: 'translateX(-50%)',
          backgroundColor: COLORS.ink,
          opacity: 0.16,
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          top: 824,
          display: 'flex',
          justifyContent: 'center',
          opacity: tagIn,
          transform: `translateY(${(1 - tagIn) * 8}px)`,
        }}
      >
        <Tech size={13} color={COLORS.ink} opacity={0.62} tracking={0.27} weight={700}>
          One file in. One file out.
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 58,
          display: 'flex',
          justifyContent: 'center',
          opacity: tagIn * 0.5,
        }}
      >
        <Tech size={8.5} color={COLORS.ink} opacity={0.38} tracking={0.2}>
          Linux-native media conversion · in place
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          inset: 0,
          background: 'radial-gradient(ellipse at 50% 45%, rgba(255,255,255,0) 42%, rgba(23,23,23,0.055) 100%)',
          pointerEvents: 'none',
        }}
      />
    </AbsoluteFill>
  );
};
