import React from 'react';
import {AbsoluteFill, useVideoConfig} from 'remotion';
import {CameraMotion} from '../components/CameraMotion';
import {Stage3D} from '../components/Stage3D';
import {WorldDiamond, WorldLine, WorldPolyline, PlayMark} from '../components/World';
import {ScannerPlane} from '../components/ProcessEffects';
import {Display, Dot, SceneTag, Tech} from '../components/Type';
import {COLORS} from '../theme';
import {appear, clamp01, fadeWindow, mix, smoothstep} from '../lib/anim';
import {mixHex, shade} from '../lib/color';
import {boxAt} from '../lib/geometry';
import {projectPoint, rotateY, type CameraFrame, type Vec3} from '../lib/vec';

const SEG_COUNT = 6;
const SEG_W = 0.70;
const SEG_H = 1.65;
const SEG_D = 1.32;
const BASE_Y = -0.22;
const GROUP_X = -0.2;
const FLOOR_Y = -1.12;
const PLINTH_TOP = -1.12;
const TILE_SIZE = 0.82;

type Seg = {
  i: number;
  x: number;
  y: number;
  z: number;
  rotY: number;
  color: string;
  scanned: boolean;
  markerColor: string;
};

const fileLayout = (absFrame: number) => {
  const enter = appear(absFrame, 0, 28);
  const groupYaw = -0.24 * (1 - enter);
  const groupX = GROUP_X + 1.5 * (1 - enter);
  const groupY = (1 - enter) * -2.7;
  const gap = 0.014 + 0.058 * smoothstep(88, 112, absFrame);
  const step = SEG_W + gap;
  const scanIn = smoothstep(92, 98, absFrame);
  const scanOut = 1 - smoothstep(130, 138, absFrame);
  const scanActive = scanIn * scanOut;
  const scanX = mix(-2.55, 2.55, smoothstep(94, 134, absFrame));
  const split = smoothstep(88, 108, absFrame);

  const segs: Seg[] = [];
  for (let i = 0; i < SEG_COUNT; i++) {
    const localX = (i - (SEG_COUNT - 1) / 2) * step;
    const rotated = rotateY([localX, 0, 0], groupYaw);
    const segX = groupX + rotated[0];
    const segZ = rotated[2] + (i % 2 === 0 ? 0.015 : -0.015) * split;
    const scanned = absFrame > 94 && scanX > segX + 0.12;
    const passT = 94 + ((segX + 2.55) / 5.1) * 40;
    const bump = Math.sin(Math.PI * clamp01((absFrame - passT) / 13)) * 0.045;
    const color = mixHex(COLORS.red, COLORS.redDeep, i % 2 === 0 ? 0.02 : 0.055);
    segs.push({
      i,
      x: segX,
      y: BASE_Y + groupY + bump,
      z: segZ,
      rotY: groupYaw,
      color,
      scanned,
      markerColor: scanned ? COLORS.green : COLORS.yellow,
    });
  }

  return {enter, groupYaw, groupX, groupY, gap, segs, scanActive, scanX};
};

const PlinthShadow: React.FC<{camera: CameraFrame}> = ({camera}) => {
  const points = [
    projectPoint([GROUP_X - 2.65, FLOOR_Y - 0.12, -1.05], camera),
    projectPoint([GROUP_X + 2.65, FLOOR_Y - 0.12, -1.05], camera),
    projectPoint([GROUP_X + 2.65, FLOOR_Y - 0.12, 1.05], camera),
    projectPoint([GROUP_X - 2.65, FLOOR_Y - 0.12, 1.05], camera),
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

const SHELL_GAP = 0.014;
const SHELL_W = SEG_COUNT * SEG_W + (SEG_COUNT - 1) * SHELL_GAP;
const SHELL_H = SEG_H + 0.035;
const SHELL_D = SEG_D + 0.035;

/**
 * Surface detail for the intact source object. The shell is what makes the
 * first beat read as one mass; it dissolves as the file opens into segments.
 */
const ShellDetail: React.FC<{
  camera: CameraFrame;
  data: ReturnType<typeof fileLayout>;
  opacity: number;
}> = ({camera, data, opacity}) => {
  const center: Vec3 = [data.groupX, BASE_Y + data.groupY, 0];
  const yaw = data.groupYaw;
  const z = SHELL_D / 2 + 0.006;
  const halfW = SHELL_W / 2 - 0.15;
  const halfH = SHELL_H / 2 - 0.15;

  const at = (x: number, y: number): Vec3 => {
    const rotated = rotateY([x, y, z], yaw);
    return [center[0] + rotated[0], center[1] + rotated[1], center[2] + rotated[2]];
  };

  const panel = [
    at(-halfW, -halfH),
    at(halfW, -halfH),
    at(halfW, halfH),
    at(-halfW, halfH),
    at(-halfW, -halfH),
  ];

  const wave: Vec3[] = [];
  for (let k = 0; k <= 72; k++) {
    const t = k / 72;
    const env = Math.sin(Math.PI * t);
    const value =
      Math.sin(t * Math.PI * 6.3 + 0.8) * 0.5 +
      Math.sin(t * Math.PI * 14.1 + 1.9) * 0.26 +
      Math.sin(t * Math.PI * 25.3) * 0.14;
    wave.push(at(-SHELL_W / 2 + 0.24 + t * (SHELL_W - 0.48), -0.18 + value * 0.2 * env));
  }

  const topEdge = [at(-halfW, halfH), at(halfW, halfH)];

  return (
    <g opacity={opacity}>
      <WorldPolyline camera={camera} points={panel} color={shade(COLORS.red, -0.4)} opacity={0.5} strokeWidth={1.1} />
      <WorldPolyline camera={camera} points={wave} color={COLORS.redDeep} opacity={0.58} strokeWidth={1.9} />
      <WorldPolyline camera={camera} points={topEdge} color={COLORS.white} opacity={0.2} strokeWidth={1} />
    </g>
  );
};

const Waveform: React.FC<{camera: CameraFrame; seg: Seg}> = ({camera, seg}) => {
  const points: Vec3[] = [];
  for (let k = 0; k <= 26; k++) {
    const t = k / 26;
    const localX = -SEG_W / 2 + 0.075 + t * (SEG_W - 0.15);
    const env = Math.sin(Math.PI * t);
    const wave =
      Math.sin(t * Math.PI * 5.1 + seg.i * 1.9) * 0.5 +
      Math.sin(t * Math.PI * 11.7 + seg.i * 0.8) * 0.26 +
      Math.sin(t * Math.PI * 23.3) * 0.14;
    const local: Vec3 = [localX, -0.38 + wave * 0.16 * env, SEG_D / 2 + 0.004];
    const rotated = rotateY(local, seg.rotY);
    points.push([seg.x + rotated[0], seg.y + rotated[1], seg.z + rotated[2]]);
  }
  return (
    <WorldPolyline
      camera={camera}
      points={points}
      color={COLORS.redDeep}
      opacity={0.42}
      strokeWidth={1.7}
    />
  );
};

export const FileStage: React.FC<{absFrame: number}> = ({absFrame}) => {
  const {width, height} = useVideoConfig();
  const data = fileLayout(absFrame);
  const sourceLabelOpacity = 1 - smoothstep(88, 96, absFrame);
  const preflightOpacity = smoothstep(88, 96, absFrame);
  const complete = smoothstep(130, 137, absFrame);
  const headroomIn = smoothstep(110, 124, absFrame);
  const tileIn = appear(absFrame, 58, 24);
  const fileLabelIn = appear(absFrame, 14, 20);
  const sizeIn = appear(absFrame, 20, 20);
  const scanLabelIn = smoothstep(94, 103, absFrame);
  const shellOpacity = 1 - smoothstep(84, 98, absFrame);
  const segFadeIn = smoothstep(80, 92, absFrame);

  const tileX = mix(4.22, 3.34, tileIn);
  const tileY = mix(-0.7, -0.02, tileIn);
  const tileScale = mix(0.35, 1, tileIn);

  return (
    <AbsoluteFill
      style={{
        background:
          'radial-gradient(ellipse at 63% 37%, #FFFFFF 0%, #F8F7F2 40%, #EFEDE6 76%, #E8E6DE 100%)',
      }}
    >
      <AbsoluteFill
        style={{
          background:
            'linear-gradient(122deg, rgba(242,201,76,0.10) 0%, rgba(242,201,76,0) 33%), linear-gradient(250deg, rgba(49,85,231,0.08) 0%, rgba(49,85,231,0) 42%)',
        }}
      />

      <CameraMotion
        frame={absFrame}
        width={width}
        height={height}
        from={{target: [0.62, 0.02, 0], yaw: -33, pitch: 20, distance: 10.8, fov: 34}}
        to={{target: [0.62, -0.02, 0], yaw: -17, pitch: 14, distance: 8.9, fov: 34}}
        start={0}
        end={150}
      >
        {(camera) => {
          const boxes = [
            boxAt({
              id: 'plinth',
              position: [GROUP_X, PLINTH_TOP - 0.055, 0],
              size: [5.25, 0.11, 1.98],
              color: COLORS.grey,
              edge: shade(COLORS.grey, -0.22),
              edgeOpacity: 0.5,
              castShadow: false,
            }),
            ...(shellOpacity > 0.005
              ? [
                  boxAt({
                    id: 'source-shell',
                    position: [data.groupX, BASE_Y + data.groupY, 0],
                    size: [SHELL_W + 0.02, SHELL_H, SHELL_D],
                    rotation: [0, data.groupYaw, 0],
                    color: COLORS.red,
                    edge: shade(COLORS.red, -0.3),
                    edgeOpacity: 0.52,
                    opacity: shellOpacity,
                    shadowStrength: 1,
                  }),
                ]
              : []),
            ...data.segs.map((seg) =>
              boxAt({
                id: `seg-${seg.i}`,
                position: [seg.x, seg.y, seg.z],
                size: [SEG_W, SEG_H, SEG_D],
                rotation: [0, seg.rotY, 0],
                color: seg.color,
                edge: shade(COLORS.red, -0.28),
                edgeOpacity: 0.45,
                opacity: segFadeIn,
                shadowStrength: 0.85 * segFadeIn,
              }),
            ),
            boxAt({
              id: 'mp4-tile',
              position: [tileX, tileY, 0.3],
              size: [TILE_SIZE * tileScale, TILE_SIZE * tileScale, 0.34 * tileScale],
              color: COLORS.blue,
              opacity: tileIn,
              faceColors: {top: mixHex(COLORS.blue, COLORS.white, 0.16)},
              edge: COLORS.blueDeep,
              edgeOpacity: 0.5,
              shadowStrength: 0.5,
              glow: tileIn > 0.4 ? 'rgba(49,85,231,0.24)' : undefined,
            }),
          ];

          const fileBottomRight = projectPoint(
            [data.segs[SEG_COUNT - 1].x + SEG_W / 2 + 0.1, BASE_Y - SEG_H / 2, 0.4],
            camera,
          );
          const tileAnchor = projectPoint([tileX - 0.42, tileY + 0.24, 0.3], camera);
          const tileBottom = projectPoint([tileX, tileY - TILE_SIZE * 0.62, 0.3], camera);
          const rightSeg = data.segs[SEG_COUNT - 1];
          const bracketL = projectPoint([rightSeg.x - SEG_W / 2, BASE_Y + SEG_H / 2 + 0.2, 0.4], camera);
          const bracketR = projectPoint([rightSeg.x + SEG_W / 2, BASE_Y + SEG_H / 2 + 0.2, 0.4], camera);
          const bracketY = Math.min(bracketL.y, bracketR.y) - 18;
          const scanPercent = clamp01((data.scanX + 2.55) / 5.1);

          return (
            <Stage3D
              camera={camera}
              boxes={boxes}
              floorY={FLOOR_Y}
              shadowColor={COLORS.ink}
              shadowOpacity={0.9}
              behind={<PlinthShadow camera={camera} />}
            >
              <ShellDetail camera={camera} data={data} opacity={shellOpacity} />
              <g opacity={segFadeIn}>
                {data.segs.map((seg) => (
                  <Waveform key={`wf-${seg.i}`} camera={camera} seg={seg} />
                ))}
              </g>

              {data.segs.map((seg) => {
                const markerCenter: Vec3 = [
                  seg.x,
                  BASE_Y + SEG_H / 2 + 0.28,
                  seg.z + 0.42,
                ];
                const markerIn = preflightOpacity * (0.55 + 0.45 * smoothstep(92 + seg.i * 1.3, 101 + seg.i * 1.3, absFrame));
                return (
                  <g key={`marker-${seg.i}`} opacity={markerIn}>
                    <WorldLine
                      camera={camera}
                      from={[seg.x, BASE_Y + SEG_H / 2 + 0.06, seg.z + 0.3]}
                      to={[seg.x, BASE_Y + SEG_H / 2 + 0.21, seg.z + 0.38]}
                      color={seg.markerColor}
                      opacity={0.75}
                      strokeWidth={1.2}
                    />
                    <WorldDiamond
                      camera={camera}
                      center={markerCenter}
                      size={0.065}
                      color={seg.markerColor}
                      fillOpacity={0.95}
                      opacity={0.95}
                      filter={seg.scanned ? 'url(#stage-glow-green)' : 'url(#stage-glow-yellow)'}
                    />
                  </g>
                );
              })}

              {data.scanActive > 0.01 ? (
                <ScannerPlane
                  camera={camera}
                  x={data.scanX}
                  y={0.08}
                  height={2.55}
                  depth={2.25}
                  opacity={data.scanActive}
                />
              ) : null}

              <g opacity={preflightOpacity * smoothstep(110, 122, absFrame) * (1 - smoothstep(146, 150, absFrame))}>
                <path
                  d={`M${bracketL.x} ${bracketY + 12} L${bracketL.x} ${bracketY} L${bracketR.x} ${bracketY} L${bracketR.x} ${bracketY + 12}`}
                  fill="none"
                  stroke={COLORS.ink}
                  strokeOpacity={0.38}
                  strokeWidth={1}
                />
                <line
                  x1={bracketR.x + 6}
                  y1={bracketY}
                  x2={1380}
                  y2={bracketY}
                  stroke={COLORS.ink}
                  strokeOpacity={0.22}
                  strokeWidth={1}
                  strokeDasharray="2 6"
                />
                <circle cx={bracketR.x} cy={bracketY} r={2.3} fill={COLORS.green} />
              </g>

              <PlayMark
                camera={camera}
                center={[tileX, tileY, 0.3 + 0.17 * tileScale + 0.004]}
                size={0.27 * tileScale}
                color={COLORS.white}
                opacity={tileIn}
              />

              <g opacity={tileIn}>
                <line
                  x1={fileBottomRight.x + 12}
                  y1={fileBottomRight.y - 26}
                  x2={tileAnchor.x}
                  y2={tileAnchor.y}
                  stroke={COLORS.blue}
                  strokeOpacity={0.55}
                  strokeWidth={1.2}
                  strokeDasharray="3 7"
                  strokeDashoffset={-absFrame * 1.4}
                />
                <circle cx={tileAnchor.x} cy={tileAnchor.y} r={2.4} fill={COLORS.blue} />
                <path
                  d={`M${tileAnchor.x - 7} ${tileAnchor.y - 4} L${tileAnchor.x} ${tileAnchor.y} L${tileAnchor.x - 7} ${tileAnchor.y + 4}`}
                  fill="none"
                  stroke={COLORS.blue}
                  strokeOpacity={0.8}
                  strokeWidth={1.3}
                  strokeLinecap="round"
                  strokeLinejoin="round"
                />
              </g>

              <g opacity={preflightOpacity * scanLabelIn * (1 - complete)}>
                <line
                  x1={1408}
                  y1={798}
                  x2={1408}
                  y2={892}
                  stroke={COLORS.blue}
                  strokeOpacity={0.4}
                  strokeWidth={1}
                />
                <line x1={1408} y1={798} x2={1424} y2={798} stroke={COLORS.blue} strokeOpacity={0.4} strokeWidth={1} />
                <circle cx={1408} cy={798} r={2.3} fill={COLORS.blue} />
                <text
                  x={1418}
                  y={862}
                  fill={COLORS.blue}
                  fontFamily='"Hack", monospace'
                  fontSize={10}
                  letterSpacing="1.4"
                  opacity={0.72}
                >
                  {String(Math.round(scanPercent * 100)).padStart(2, '0')}%
                </text>
              </g>

              <g opacity={tileIn}>
                <line
                  x1={tileBottom.x}
                  y1={tileBottom.y - 4}
                  x2={tileBottom.x}
                  y2={tileBottom.y + 18}
                  stroke={COLORS.blue}
                  strokeOpacity={0.45}
                  strokeWidth={1}
                />
                <circle cx={tileBottom.x} cy={tileBottom.y + 22} r={2} fill={COLORS.blue} />
                <text
                  x={tileBottom.x}
                  y={tileBottom.y + 44}
                  textAnchor="middle"
                  fill={COLORS.blue}
                  fontFamily='"Hack", monospace'
                  fontSize={10}
                  letterSpacing="2.2"
                  fontWeight={700}
                >
                  MP4
                </text>
                <text
                  x={tileBottom.x}
                  y={tileBottom.y + 60}
                  textAnchor="middle"
                  fill={COLORS.ink}
                  fillOpacity={0.38}
                  fontFamily='"Hack", monospace'
                  fontSize={8.5}
                  letterSpacing="1.6"
                >
                  TARGET CONTAINER
                </text>
              </g>
            </Stage3D>
          );
        }}
      </CameraMotion>

      <SceneTag
        index="01"
        title="THE FILE"
        detail="SOURCE OBJECT"
        style={{left: 92, top: 62, opacity: sourceLabelOpacity}}
      />
      <SceneTag
        index="02"
        title="PREFLIGHT"
        detail="INSPECTION PLANE"
        color={COLORS.blue}
        style={{left: 92, top: 62, opacity: preflightOpacity}}
      />

      <div
        style={{
          position: 'absolute',
          left: 92,
          top: 158,
          display: 'flex',
          alignItems: 'center',
          gap: 11,
          opacity: fileLabelIn,
          transform: `translateY(${(1 - fileLabelIn) * 8}px)`,
        }}
      >
        <Dot color={COLORS.red} size={7} />
        <Tech size={16} tracking={0.015} weight={700} style={{textTransform: 'none'}}>
          interstellar_4k.mkv
        </Tech>
      </div>
      <Tech
        size={10}
        color={COLORS.ink}
        opacity={0.4 * fileLabelIn}
        tracking={0.18}
        style={{position: 'absolute', left: 110, top: 185}}
      >
        4K · MATROSKA · SINGLE SOURCE
      </Tech>

      <div
        style={{
          position: 'absolute',
          right: 400,
          top: 200,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-end',
          opacity: sizeIn * sourceLabelOpacity,
          transform: `translateY(${(1 - sizeIn) * 10}px)`,
        }}
      >
        <div style={{display: 'flex', alignItems: 'center', gap: 12}}>
          <Dot color={COLORS.red} size={8} />
          <Display size={64} weight={800} tracking={-0.055}>
            60 GB
          </Display>
        </div>
        <Tech size={10} color={COLORS.ink} opacity={0.42} tracking={0.18} style={{marginTop: 12}}>
          4K · 45 Mbps · 60 s segments
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 384,
          top: 200,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'flex-end',
          opacity: headroomIn * (1 - smoothstep(146, 150, absFrame)),
          transform: `translateY(${(1 - headroomIn) * 12}px)`,
        }}
      >
        <Tech size={10} color={COLORS.ink} opacity={0.44} tracking={0.2}>
          Est. peak additional space
        </Tech>
        <div style={{display: 'flex', alignItems: 'center', gap: 12, marginTop: 8}}>
          <Dot color={COLORS.yellow} size={8} />
          <Display size={48} weight={800} tracking={-0.045}>
            ≈ 340 MB
          </Display>
        </div>
        <Tech size={9} color={COLORS.ink} opacity={0.36} tracking={0.13} style={{marginTop: 10}}>
          One 60 s segment · 45 Mbps
        </Tech>
        <Tech size={8.5} color={COLORS.ink} opacity={0.3} tracking={0.12} style={{marginTop: 5}}>
          from a 60 GB source
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 92,
          bottom: 74,
          display: 'flex',
          alignItems: 'center',
          gap: 11,
          opacity: preflightOpacity * complete,
          transform: `translateY(${(1 - complete) * 8}px)`,
        }}
      >
        <div
          style={{
            width: 18,
            height: 18,
            borderRadius: 9,
            border: `1px solid ${COLORS.green}`,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <div
            style={{
              width: 7,
              height: 4,
              borderLeft: `1.5px solid ${COLORS.green}`,
              borderBottom: `1.5px solid ${COLORS.green}`,
              transform: 'rotate(-45deg) translate(0.5px, -0.5px)',
            }}
          />
        </div>
        <Tech size={11} color={COLORS.green} tracking={0.22} weight={700}>
          Preflight complete
        </Tech>
        <Tech size={9.5} color={COLORS.ink} opacity={0.36} tracking={0.14} style={{marginLeft: 8}}>
          No writes yet
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 92,
          bottom: 76,
          display: 'flex',
          alignItems: 'center',
          gap: 10,
          opacity: preflightOpacity * (1 - complete) * scanLabelIn,
        }}
      >
        <Dot color={COLORS.blue} size={5} />
        <Tech size={9.5} color={COLORS.blue} tracking={0.2}>
          Scanning source
        </Tech>
      </div>

      <div style={{position: 'absolute', left: 0, right: 0, top: 930, height: 1, background: COLORS.ink, opacity: 0.05}} />
      <Tech
        size={8.5}
        color={COLORS.ink}
        opacity={0.26 * fadeWindow(absFrame, 30, 24, 140, 10)}
        tracking={0.22}
        style={{position: 'absolute', left: 92, bottom: 46}}
      >
        Relocate · segmented, low-space conversion
      </Tech>
      <div
        style={{
          position: 'absolute',
          right: 92,
          bottom: 46,
          opacity: fadeWindow(absFrame, 24, 26, 142, 8),
        }}
      >
        <Tech size={8.5} color={COLORS.ink} opacity={0.28} tracking={0.18}>
          Siphon / Linux
        </Tech>
      </div>
    </AbsoluteFill>
  );
};
