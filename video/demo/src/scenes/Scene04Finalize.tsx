import React from 'react';
import {AbsoluteFill, useCurrentFrame, useVideoConfig} from 'remotion';
import {CameraMotion} from '../components/CameraMotion';
import {Stage3D} from '../components/Stage3D';
import {VerificationEffect} from '../components/ProcessEffects';
import {PlayMark, WorldDiamond, WorldLine, WorldPolygon, WorldPolyline} from '../components/World';
import {Display, Dot, SceneTag, Tech} from '../components/Type';
import {COLORS} from '../theme';
import {clamp01, smoothstep} from '../lib/anim';
import {mixHex, shade} from '../lib/color';
import {boxAt, type BoxDef} from '../lib/geometry';
import {PIPELINE} from '../lib/pipeline';
import {projectPoint, type CameraFrame, type Vec3} from '../lib/vec';

const OBJ_X = -0.2;
const OBJ_Y = -0.22;
const FINAL_W = 4.2;
const FINAL_H = 1.65;
const FINAL_D = 1.32;
const SEG_W = FINAL_W / 6;
const FLOOR_Y = -1.12;
const PLINTH_TOP = -1.12;

type Piece = {
  i: number;
  center: Vec3;
  rotation: Vec3;
  p: number;
};

const pieceAt = (i: number, f: number): Piece => {
  const start = 9 + (5 - i) * 0.9;
  const p = clamp01((f - start) / 27);
  const eased = 1 - Math.pow(1 - p, 3.4);
  const from: Vec3 = [PIPELINE.outputX(i), PIPELINE.baseY, 0];
  const to: Vec3 = [OBJ_X + (i - 2.5) * SEG_W, OBJ_Y, 0];
  const lift = Math.sin(Math.PI * clamp01(p)) * 0.56;
  const center: Vec3 = [
    from[0] + (to[0] - from[0]) * eased,
    from[1] + (to[1] - from[1]) * eased + lift,
    from[2] + (to[2] - from[2]) * eased,
  ];
  const rotation: Vec3 = [0, (1 - eased) * (i % 2 === 0 ? 0.26 : -0.2), (1 - eased) * 0.035];
  return {i, center, rotation, p};
};

const PlinthShadow: React.FC<{camera: CameraFrame}> = ({camera}) => {
  const points = [
    projectPoint([OBJ_X - 2.7, FLOOR_Y - 0.12, -1.08], camera),
    projectPoint([OBJ_X + 2.7, FLOOR_Y - 0.12, -1.08], camera),
    projectPoint([OBJ_X + 2.7, FLOOR_Y - 0.12, 1.08], camera),
    projectPoint([OBJ_X - 2.7, FLOOR_Y - 0.12, 1.08], camera),
  ];
  return (
    <polygon
      points={points.map((p) => `${p.x},${p.y}`).join(' ')}
      fill={COLORS.ink}
      opacity={0.1}
      filter="url(#stage-shadow-soft)"
    />
  );
};

/**
 * A memory of where the source sat. Only the front silhouette and the floor
 * footprint are drawn, so it reads as absence rather than a second object.
 */
const SourceGhost: React.FC<{camera: CameraFrame; opacity: number}> = ({camera, opacity}) => {
  const floor: Vec3[] = [
    [OBJ_X - FINAL_W / 2, PLINTH_TOP + 0.008, -FINAL_D / 2],
    [OBJ_X + FINAL_W / 2, PLINTH_TOP + 0.008, -FINAL_D / 2],
    [OBJ_X + FINAL_W / 2, PLINTH_TOP + 0.008, FINAL_D / 2],
    [OBJ_X - FINAL_W / 2, PLINTH_TOP + 0.008, FINAL_D / 2],
  ];
  return (
    <g opacity={opacity}>
      <WorldPolygon
        camera={camera}
        points={floor}
        color={COLORS.red}
        fill={COLORS.red}
        fillOpacity={0.035}
        stroke={COLORS.red}
        strokeOpacity={0.42}
        strokeWidth={1.2}
        dash="5 8"
      />
      <WorldDiamond
        camera={camera}
        center={[OBJ_X, PLINTH_TOP + 0.012, 0]}
        size={0.075}
        color={COLORS.red}
        fillOpacity={0.55}
        opacity={0.6}
      />
    </g>
  );
};

const ObjectWaveform: React.FC<{camera: CameraFrame; opacity: number}> = ({camera, opacity}) => {
  const points: Vec3[] = [];
  for (let k = 0; k <= 60; k++) {
    const t = k / 60;
    const x = OBJ_X - FINAL_W / 2 + 0.24 + t * (FINAL_W - 0.48);
    const env = Math.sin(Math.PI * t);
    const wave =
      Math.sin(t * Math.PI * 7.3 + 0.6) * 0.5 +
      Math.sin(t * Math.PI * 15.7) * 0.24 +
      Math.sin(t * Math.PI * 29.1 + 1.4) * 0.12;
    points.push([x, OBJ_Y - 0.32 + wave * 0.15 * env, FINAL_D / 2 + 0.034]);
  }
  return (
    <WorldPolyline
      camera={camera}
      points={points}
      color={COLORS.green}
      opacity={0.92 * opacity}
      strokeWidth={2.2}
    />
  );
};

const FinalDetail: React.FC<{camera: CameraFrame; assembled: number; verify: number}> = ({
  camera,
  assembled,
  verify,
}) => {
  if (assembled < 0.01) return null;
  const z = FINAL_D / 2 + 0.032;
  const corners: Vec3[] = [
    [OBJ_X - FINAL_W / 2, OBJ_Y - FINAL_H / 2, z],
    [OBJ_X + FINAL_W / 2, OBJ_Y - FINAL_H / 2, z],
    [OBJ_X + FINAL_W / 2, OBJ_Y + FINAL_H / 2, z],
    [OBJ_X - FINAL_W / 2, OBJ_Y + FINAL_H / 2, z],
  ];
  const inset = 0.25;
  const inner: Vec3[] = [
    [OBJ_X - FINAL_W / 2 + inset, OBJ_Y - FINAL_H / 2 + inset, z],
    [OBJ_X + FINAL_W / 2 - inset, OBJ_Y - FINAL_H / 2 + inset, z],
    [OBJ_X + FINAL_W / 2 - inset, OBJ_Y + FINAL_H / 2 - inset, z],
    [OBJ_X - FINAL_W / 2 + inset, OBJ_Y + FINAL_H / 2 - inset, z],
  ];
  const projected = corners.map((p) => projectPoint(p, camera));
  const xs = projected.map((p) => p.x);
  const ys = projected.map((p) => p.y);
  return (
    <g opacity={assembled}>
      <defs>
        <linearGradient
          id="final-sheen"
          gradientUnits="userSpaceOnUse"
          x1={Math.min(...xs)}
          y1={Math.min(...ys)}
          x2={Math.max(...xs)}
          y2={Math.max(...ys)}
        >
          <stop offset="0%" stopColor={COLORS.white} stopOpacity="0.28" />
          <stop offset="38%" stopColor={COLORS.white} stopOpacity="0.045" />
          <stop offset="70%" stopColor={COLORS.white} stopOpacity="0" />
        </linearGradient>
      </defs>
      <path
        d={projected.map((p, i) => `${i === 0 ? 'M' : 'L'}${p.x} ${p.y}`).join(' ') + ' Z'}
        fill="url(#final-sheen)"
        opacity={0.85}
      />
      <WorldPolyline
        camera={camera}
        points={[...inner, inner[0]]}
        color={shade(COLORS.blueDeep, -0.18)}
        opacity={0.55}
        strokeWidth={1.1}
      />
      <WorldPolyline
        camera={camera}
        points={[
          [OBJ_X - FINAL_W / 2 + 0.06, OBJ_Y + FINAL_H / 2 - 0.09, z],
          [OBJ_X + FINAL_W / 2 - 0.06, OBJ_Y + FINAL_H / 2 - 0.09, z],
        ]}
        color={COLORS.white}
        opacity={0.24}
        strokeWidth={1}
      />
      <WorldLine
        camera={camera}
        from={[OBJ_X + FINAL_W / 2 - 0.055, OBJ_Y - FINAL_H / 2 + 0.09, z]}
        to={[OBJ_X + FINAL_W / 2 - 0.055, OBJ_Y + FINAL_H / 2 - 0.09, z]}
        color={COLORS.green}
        opacity={0.72 * verify}
        strokeWidth={2}
        filter="url(#stage-glow-green)"
      />
      {[
        [-1, 1],
        [1, 1],
        [-1, -1],
        [1, -1],
      ].map(([sx, sy], i) => {
        const px = OBJ_X + sx * (FINAL_W / 2 - 0.22);
        const py = OBJ_Y + sy * (FINAL_H / 2 - 0.2);
        return (
          <WorldPolyline
            key={i}
            camera={camera}
            points={[
              [px - sx * 0.12, py, z],
              [px, py, z],
              [px, py - sy * 0.11, z],
            ]}
            color={COLORS.white}
            opacity={0.3}
            strokeWidth={1}
          />
        );
      })}
      <WorldLine
        camera={camera}
        from={[OBJ_X - FINAL_W / 2 + 0.06, OBJ_Y - FINAL_H / 2 + 0.07, z]}
        to={[OBJ_X + FINAL_W / 2 - 0.06, OBJ_Y - FINAL_H / 2 + 0.07, z]}
        color={COLORS.blueDeep}
        opacity={0.45}
        strokeWidth={1}
      />
    </g>
  );
};

export const Scene04Finalize: React.FC = () => {
  const f = useCurrentFrame();
  const {width, height} = useVideoConfig();

  const pieces = Array.from({length: 6}, (_, i) => pieceAt(i, f));
  const assembled = smoothstep(40, 49, f);
  const commit = smoothstep(50, 64, f);
  const verify = smoothstep(58, 68, f);
  const labelIn = smoothstep(70, 84, f);
  const ghostOut = smoothstep(50, 64, f);
  const glowIn = smoothstep(34, 52, f);

  return (
    <AbsoluteFill
      style={{
        background:
          'radial-gradient(ellipse at 55% 42%, #FFFFFF 0%, #F8F7F2 42%, #EFEDE5 78%, #E7E5DC 100%)',
      }}
    >
      <AbsoluteFill
        style={{
          background:
            'linear-gradient(120deg, rgba(66,169,135,0.075) 0%, rgba(66,169,135,0) 36%), linear-gradient(252deg, rgba(49,85,231,0.075) 0%, rgba(49,85,231,0) 44%)',
        }}
      />

      <CameraMotion
        frame={f}
        width={width}
        height={height}
        from={{target: [0.25, 0.1, 0], yaw: -27, pitch: 18, distance: 12.7, fov: 34}}
        to={{target: [0.12, 0.15, 0], yaw: -19, pitch: 13.5, distance: 10.8, fov: 34}}
        start={0}
        end={112}
      >
        {(camera) => {
          const boxes: BoxDef[] = [
            boxAt({
              id: 'final-plinth',
              position: [OBJ_X, PLINTH_TOP - 0.055, 0],
              size: [5.25, 0.11, 1.98],
              color: COLORS.grey,
              edge: shade(COLORS.grey, -0.22),
              edgeOpacity: 0.5,
              castShadow: false,
            }),
          ];

          for (const piece of pieces) {
            if (piece.p >= 1) continue;
            boxes.push(
              boxAt({
                id: `final-piece-${piece.i}`,
                position: piece.center,
                size: [SEG_W, FINAL_H, FINAL_D],
                rotation: piece.rotation,
                color: COLORS.green,
                edge: shade(COLORS.green, -0.26),
                edgeOpacity: 0.58,
                opacity: 1 - smoothstep(40, 48, f),
                castShadow: true,
                shadowStrength: 0.7,
              }),
            );
          }

          if (assembled > 0.01) {
            boxes.push(
              boxAt({
                id: 'final-object',
                position: [OBJ_X, OBJ_Y, 0],
                size: [FINAL_W, FINAL_H, FINAL_D],
                color: mixHex(COLORS.blue, COLORS.blueDeep, 0.3),
                faceColors: {
                  top: mixHex(COLORS.green, COLORS.greenDeep, 0.18),
                  left: mixHex(COLORS.blue, COLORS.blueDeep, 0.08),
                  right: mixHex(COLORS.blue, COLORS.blueDeep, 0.42),
                },
                opacity: assembled,
                edge: shade(COLORS.blue, -0.3),
                edgeOpacity: 0.62,
                castShadow: true,
                shadowStrength: 1,
                glow: 'rgba(66,169,135,0.16)',
              }),
              boxAt({
                id: 'final-top-band',
                position: [OBJ_X, OBJ_Y + FINAL_H / 2 + 0.008, 0],
                size: [FINAL_W - 0.22, 0.012, FINAL_D - 0.24],
                color: mixHex(COLORS.green, COLORS.white, 0.12),
                opacity: assembled * 0.9,
                castShadow: false,
              }),
              boxAt({
                id: 'final-panel',
                position: [OBJ_X, OBJ_Y - 0.02, FINAL_D / 2 + 0.012],
                size: [FINAL_W - 0.38, FINAL_H - 0.42, 0.02],
                color: mixHex(COLORS.blue, COLORS.blueDeep, 0.58),
                faceColors: {front: mixHex(COLORS.blue, COLORS.blueDeep, 0.66)},
                edge: shade(COLORS.blueDeep, -0.12),
                edgeOpacity: 0.6,
                opacity: assembled,
                castShadow: false,
              }),
            );
          }

          if (commit > 0.001 && commit < 0.999) {
            const commitX = -FINAL_W / 2 - 0.85 + commit * (FINAL_W + 1.7);
            boxes.push(
              boxAt({
                id: 'commit-blade',
                position: [OBJ_X + commitX, OBJ_Y, 0.72],
                size: [0.075, FINAL_H + 0.4, 0.46],
                color: mixHex(COLORS.yellow, COLORS.white, 0.12),
                edge: COLORS.yellowDeep,
                edgeOpacity: 0.4,
                opacity: 0.66,
                castShadow: false,
                glow: 'rgba(242,201,76,0.72)',
              }),
            );
          }

          if (verify > 0.01 && verify < 0.99) {
            boxes.push(
              boxAt({
                id: 'verify-frame',
                position: [OBJ_X, OBJ_Y, 0],
                size: [
                  FINAL_W + 0.09 + verify * 0.16,
                  FINAL_H + 0.09 + verify * 0.16,
                  FINAL_D + 0.09 + verify * 0.16,
                ],
                color: COLORS.green,
                opacity: (1 - verify) * 0.62,
                style: 'wire',
                edge: COLORS.green,
                castShadow: false,
              }),
            );
          }

          const labelAnchor = projectPoint([OBJ_X + FINAL_W / 2 + 0.1, OBJ_Y + 0.34, 0.55], camera);
          const topMid = projectPoint([OBJ_X, OBJ_Y + FINAL_H / 2 + 0.08, FINAL_D / 2], camera);

          return (
            <Stage3D
              camera={camera}
              boxes={boxes}
              floorY={FLOOR_Y}
              shadowColor={COLORS.ink}
              shadowOpacity={1}
              behind={
                <g>
                  <circle
                    cx={projectPoint([OBJ_X, OBJ_Y, 0], camera).x}
                    cy={projectPoint([OBJ_X, OBJ_Y, 0], camera).y}
                    r={455}
                    fill="url(#final-halo)"
                    opacity={glowIn}
                  />
                  <defs>
                    <radialGradient id="final-halo">
                      <stop offset="0%" stopColor={COLORS.blue} stopOpacity="0.1" />
                      <stop offset="42%" stopColor={COLORS.green} stopOpacity="0.055" />
                      <stop offset="100%" stopColor={COLORS.green} stopOpacity="0" />
                    </radialGradient>
                  </defs>
                  <PlinthShadow camera={camera} />
                  <SourceGhost camera={camera} opacity={(1 - ghostOut) * 0.95} />
                </g>
              }
            >
              {assembled > 0.01 ? <ObjectWaveform camera={camera} opacity={assembled} /> : null}
              <PlayMark
                camera={camera}
                center={[OBJ_X - FINAL_W / 2 + 0.55, OBJ_Y - 0.04, FINAL_D / 2 + 0.034]}
                size={0.24}
                color={COLORS.white}
                opacity={assembled * 0.86}
              />
              <FinalDetail camera={camera} assembled={assembled} verify={verify} />

              {verify > 0.01 ? (
                <g>
                  <VerificationEffect
                    camera={camera}
                    center={[OBJ_X, OBJ_Y, 0]}
                    progress={verify}
                    opacity={0.7}
                    radius={2.6}
                    floorY={PLINTH_TOP}
                  />
                </g>
              ) : null}

              <g opacity={labelIn}>
                <line
                  x1={labelAnchor.x + 10}
                  y1={labelAnchor.y}
                  x2={1282}
                  y2={labelAnchor.y}
                  stroke={COLORS.ink}
                  strokeOpacity={0.18}
                  strokeWidth={1}
                  strokeDasharray="2 6"
                />
                <circle cx={labelAnchor.x} cy={labelAnchor.y} r={2.4} fill={COLORS.green} />
              </g>

              <g opacity={commit > 0 && commit < 0.7 ? 0.9 : 0}>
                <line
                  x1={topMid.x}
                  y1={topMid.y - 44}
                  x2={topMid.x}
                  y2={topMid.y - 9}
                  stroke={COLORS.yellow}
                  strokeWidth={1.2}
                />
                <circle cx={topMid.x} cy={topMid.y - 50} r={2.6} fill={COLORS.yellow} />
              </g>
            </Stage3D>
          );
        }}
      </CameraMotion>

      <SceneTag
        index="04"
        title="FINALISATION"
        detail="ASSEMBLY · ATOMIC COMMIT"
        style={{left: 92, top: 62}}
      />

      <div
        style={{
          position: 'absolute',
          left: 92,
          top: 178,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          opacity: smoothstep(0, 12, f) * (1 - smoothstep(48, 56, f)),
        }}
      >
        <Dot color={COLORS.red} size={6} />
        <Tech size={9.5} color={COLORS.ink} opacity={0.44} tracking={0.18}>
          Source location · MKV
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 92,
          top: 178,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          opacity: smoothstep(52, 60, f) * (1 - smoothstep(84, 96, f)),
        }}
      >
        <Dot color={COLORS.yellow} size={6} />
        <Tech size={9.5} color={COLORS.ink} opacity={0.62} tracking={0.18} weight={700}>
          Atomic commit
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 92,
          top: 178,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          opacity: verify * (1 - smoothstep(92, 104, f)),
        }}
      >
        <div
          style={{
            width: 16,
            height: 16,
            borderRadius: 8,
            border: `1px solid ${COLORS.green}`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <div
            style={{
              width: 6,
              height: 3.5,
              borderLeft: `1.4px solid ${COLORS.green}`,
              borderBottom: `1.4px solid ${COLORS.green}`,
              transform: 'rotate(-45deg) translate(0.5px, -0.5px)',
            }}
          />
        </div>
        <Tech size={9.5} color={COLORS.green} opacity={0.9} tracking={0.2} weight={700}>
          Verified
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 92,
          top: 338,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-start',
          gap: 10,
          opacity: labelIn,
          transform: `translateY(${(1 - labelIn) * 10}px)`,
        }}
      >
        <div style={{display: 'flex', alignItems: 'center', gap: 11}}>
          <Dot color={COLORS.green} size={7} />
          <Tech size={20} tracking={0.01} weight={700} style={{textTransform: 'none'}}>
            interstellar_4k.mp4
          </Tech>
        </div>
        <Tech size={9.5} color={COLORS.ink} opacity={0.42} tracking={0.16}>
          MKV → MP4 · REMUXED · VERIFIED
        </Tech>
        <Tech size={8.5} color={COLORS.ink} opacity={0.3} tracking={0.14}>
          No re-encode · one file · atomic rename
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 92,
          bottom: 74,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          opacity: labelIn * 0.8,
        }}
      >
        <Tech size={8.5} color={COLORS.ink} opacity={0.36} tracking={0.18}>
          Same directory
        </Tech>
        <div style={{width: 28, height: 1, backgroundColor: COLORS.ink, opacity: 0.16}} />
        <Tech size={8.5} color={COLORS.ink} opacity={0.36} tracking={0.18}>
          One file in · one file out
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 92,
          bottom: 74,
          opacity: labelIn * 0.8,
        }}
      >
        <Tech size={8.5} color={COLORS.ink} opacity={0.28} tracking={0.16}>
          Siphon / Relocate
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          bottom: 128,
          transform: 'translateX(-50%)',
          opacity: smoothstep(70, 84, f) * (1 - smoothstep(104, 114, f)),
        }}
      >
        <Display size={24} weight={500} tracking={-0.015} color={COLORS.ink} style={{opacity: 0.48}}>
          Many pieces. One file.
        </Display>
      </div>
    </AbsoluteFill>
  );
};
