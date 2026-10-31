import React, {useId} from 'react';
import {shade} from '../lib/color';
import {
  boxAt,
  computeFaces,
  computeShadows,
  computeWireBoxes,
  polygon,
  polyline,
  type BoxDef,
} from '../lib/geometry';
import {projectPoint, type CameraFrame, type Vec3} from '../lib/vec';

type GridSpec = {
  color: string;
  opacity: number;
  step?: number;
  extentX?: number;
  extentZ?: number;
  majorEvery?: number;
  blend?: 'multiply' | 'screen';
};

type Stage3DProps = {
  camera: CameraFrame;
  boxes: BoxDef[];
  floorY?: number;
  shadowColor?: string;
  shadowOpacity?: number;
  grid?: GridSpec | null;
  gridY?: number;
  behind?: React.ReactNode;
  children?: React.ReactNode;
  style?: React.CSSProperties;
};

const Grid: React.FC<{
  camera: CameraFrame;
  y: number;
  spec: GridSpec;
  uid: string;
}> = ({camera, y, spec, uid}) => {
  const step = spec.step ?? 0.5;
  const extentX = spec.extentX ?? 12;
  const extentZ = spec.extentZ ?? 10;
  const majorEvery = spec.majorEvery ?? 4;
  const lines: React.ReactNode[] = [];
  let idx = 0;
  for (let x = -extentX; x <= extentX + 1e-6; x += step) {
    const a = projectPoint([x, y, -extentZ], camera);
    const b = projectPoint([x, y, extentZ], camera);
    if (!a.visible || !b.visible) continue;
    const major = Math.abs(Math.round(x / step)) % majorEvery === 0;
    lines.push(
      <line
        key={`x-${idx++}`}
        x1={a.x}
        y1={a.y}
        x2={b.x}
        y2={b.y}
        stroke={spec.color}
        strokeWidth={major ? 1.15 : 0.7}
        opacity={(major ? 1 : 0.55) * spec.opacity}
      />,
    );
  }
  for (let z = -extentZ; z <= extentZ + 1e-6; z += step) {
    const a = projectPoint([-extentX, y, z], camera);
    const b = projectPoint([extentX, y, z], camera);
    if (!a.visible || !b.visible) continue;
    const major = Math.abs(Math.round(z / step)) % majorEvery === 0;
    lines.push(
      <line
        key={`z-${idx++}`}
        x1={a.x}
        y1={a.y}
        x2={b.x}
        y2={b.y}
        stroke={spec.color}
        strokeWidth={major ? 1.15 : 0.7}
        opacity={(major ? 1 : 0.55) * spec.opacity}
      />,
    );
  }

  return (
    <g mask={`url(#grid-mask-${uid})`} style={{mixBlendMode: spec.blend ?? 'multiply'}}>
      {lines}
    </g>
  );
};

export const Stage3D: React.FC<Stage3DProps> = ({
  camera,
  boxes,
  floorY = -1.2,
  shadowColor = '#171717',
  shadowOpacity = 1,
  grid = null,
  gridY,
  behind,
  children,
  style,
}) => {
  const rawId = useId();
  const uid = rawId.replace(/[^a-zA-Z0-9_-]/g, '');
  const faces = computeFaces(boxes, camera);
  const wires = computeWireBoxes(boxes, camera);
  const shadows = computeShadows(boxes, camera, floorY);

  const gradId = (faceId: string, faceName: string) => `g-${uid}-${faceId}-${faceName}`;

  return (
    <svg
      viewBox={`0 0 ${camera.width} ${camera.height}`}
      width={camera.width}
      height={camera.height}
      style={{position: 'absolute', inset: 0, overflow: 'visible', ...style}}
    >
      <defs>
        <filter id="stage-shadow-soft" x="-80%" y="-80%" width="260%" height="260%">
          <feGaussianBlur stdDeviation="20" />
        </filter>
        <filter id="stage-shadow-tight" x="-80%" y="-80%" width="260%" height="260%">
          <feGaussianBlur stdDeviation="7" />
        </filter>
        <filter id="stage-glow-blue" x="-120%" y="-120%" width="340%" height="340%">
          <feGaussianBlur stdDeviation="12" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
        <filter id="stage-glow-green" x="-120%" y="-120%" width="340%" height="340%">
          <feGaussianBlur stdDeviation="12" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
        <filter id="stage-glow-white" x="-120%" y="-120%" width="340%" height="340%">
          <feGaussianBlur stdDeviation="9" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
        <filter id="stage-glow-yellow" x="-120%" y="-120%" width="340%" height="340%">
          <feGaussianBlur stdDeviation="10" result="blur" />
          <feMerge>
            <feMergeNode in="blur" />
            <feMergeNode in="SourceGraphic" />
          </feMerge>
        </filter>
        <radialGradient id={`grid-fade-${uid}`} cx="50%" cy="52%" r="62%">
          <stop offset="0%" stopColor="#FFFFFF" stopOpacity="1" />
          <stop offset="58%" stopColor="#FFFFFF" stopOpacity="0.62" />
          <stop offset="100%" stopColor="#FFFFFF" stopOpacity="0" />
        </radialGradient>
        <mask id={`grid-mask-${uid}`}>
          <rect x="0" y="0" width={camera.width} height={camera.height} fill={`url(#grid-fade-${uid})`} />
        </mask>
        {faces.map((face) => {
          const xs = face.points.map((p) => p.x);
          const ys = face.points.map((p) => p.y);
          const minX = Math.min(...xs);
          const maxX = Math.max(...xs);
          const minY = Math.min(...ys);
          const maxY = Math.max(...ys);
          return (
            <linearGradient
              key={gradId(face.boxId, face.name)}
              id={gradId(face.boxId, face.name)}
              gradientUnits="userSpaceOnUse"
              x1={minX}
              y1={minY}
              x2={maxX}
              y2={maxY}
            >
              <stop offset="0%" stopColor={shade(face.color, 0.1)} />
              <stop offset="52%" stopColor={face.color} />
              <stop offset="100%" stopColor={shade(face.color, -0.12)} />
            </linearGradient>
          );
        })}
      </defs>

      {behind}

      {grid ? <Grid camera={camera} y={(gridY ?? floorY) + 0.002} spec={grid} uid={uid} /> : null}

      {shadows.map((shadow) => (
        <g key={`shadow-${shadow.boxId}`}>
          <polygon
            points={shadow.cast.map((p) => `${p.x},${p.y}`).join(' ')}
            fill={shadowColor}
            opacity={0.13 * shadow.strength * shadowOpacity}
            filter="url(#stage-shadow-soft)"
          />
          <polygon
            points={shadow.contact.map((p) => `${p.x},${p.y}`).join(' ')}
            fill={shadowColor}
            opacity={0.18 * shadow.strength * shadowOpacity}
            filter="url(#stage-shadow-tight)"
          />
        </g>
      ))}

      {faces.map((face) => (
        <g key={`${face.boxId}-${face.name}`} style={face.glow ? {filter: `drop-shadow(0 0 18px ${face.glow})`} : undefined}>
          <path
            d={polygon(face.points)}
            fill={face.style === 'glass' ? face.color : `url(#${gradId(face.boxId, face.name)})`}
            fillOpacity={face.opacity}
            stroke={face.edge}
            strokeOpacity={face.edgeOpacity * face.opacity}
            strokeWidth={1.1}
            strokeLinejoin="round"
          />
          {face.name === 'top' ? (
            <path
              d={polyline([...face.points, face.points[0]])}
              fill="none"
              stroke={shade(face.color, 0.42)}
              strokeOpacity={0.34 * face.opacity}
              strokeWidth={0.8}
            />
          ) : null}
        </g>
      ))}

      {wires.map((wire) => (
        <g key={`wire-${wire.boxId}`} opacity={wire.opacity}>
          {wire.segments.map((segment, i) => (
            <line
              key={i}
              x1={segment.a.x}
              y1={segment.a.y}
              x2={segment.b.x}
              y2={segment.b.y}
              stroke={wire.color}
              strokeWidth={1.2}
              strokeDasharray="5 7"
              strokeLinecap="round"
            />
          ))}
        </g>
      ))}

      {children}
    </svg>
  );
};

/** Convenience helper for a flat thin plate that sits on the studio floor. */
export const plate = (
  id: string,
  center: Vec3,
  size: Vec3,
  color: string,
  options: Partial<Omit<BoxDef, 'id' | 'center' | 'size' | 'color'>> = {},
) => boxAt({id, position: center, size, color, ...options});
