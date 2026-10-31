import React from 'react';
import {AbsoluteFill, useCurrentFrame, useVideoConfig} from 'remotion';
import {Stage3D} from '../components/Stage3D';
import {WorldDiamond, WorldLine, WorldPolygon, WorldRing} from '../components/World';
import {ReclaimAnimation, VerificationEffect} from '../components/ProcessEffects';
import {Dot, SceneTag, Tech} from '../components/Type';
import {COLORS} from '../theme';
import {clamp01, smoothstep} from '../lib/anim';
import {mixHex, shade} from '../lib/color';
import {boxAt, type BoxDef} from '../lib/geometry';
import {
  PIPELINE,
  activeCycle,
  allSegmentStates,
  reclaimProgress,
  sourceHuskVisible,
  verifiedCount,
} from '../lib/pipeline';
import {buildCamera, projectPoint, type Vec3} from '../lib/vec';

const GATE_Y = PIPELINE.baseY;
const GATE_HALF = 1.42;
const SEG = PIPELINE.segmentSize;

export const Scene03Processing: React.FC = () => {
  const f = useCurrentFrame();
  const {width, height} = useVideoConfig();

  const push = smoothstep(0, 150, f);
  const outro = smoothstep(168, 210, f);
  const settle = smoothstep(118, 190, f);
  const camera = buildCamera(
    {
      target: [-0.05 + 0.1 * settle, GATE_Y, 0],
      yaw: -32 + 7 * push + 2 * outro,
      pitch: 16 - 2.8 * push,
      distance: 13.2 - 1.5 * push + 0.8 * outro,
      fov: 34,
    },
    width,
    height,
  );

  const states = allSegmentStates(f);
  const gateK = activeCycle(f);
  const active = gateK >= 0 ? states.find((s) => s.cycle === gateK) : undefined;
  const gateEnergy = gateK >= 0 ? 1 : 0.4;
  const gateGlow = gateK >= 0 ? `rgba(49,85,231,${0.5 + 0.22 * Math.sin(f * 0.55)})` : 'rgba(49,85,231,0.2)';
  const gateColor = mixHex(COLORS.blueDeep, COLORS.blue, 0.48 + 0.52 * gateEnergy);
  const railIn = smoothstep(0, 14, f);
  const gateIn = smoothstep(3, 18, f);
  const outputGhostIn = smoothstep(10, 24, f);
  const verified = verifiedCount(f);
  const reclaimed = states.filter((s) => f >= s.cycleStart + PIPELINE.cycleLength).length;

  const boxes: BoxDef[] = [];

  if (railIn > 0.01) {
    boxes.push(
      boxAt({
        id: 'rail',
        position: [0, -0.62, 0],
        size: [10.8, 0.12, 1.5],
        color: COLORS.stageRaised,
        opacity: railIn,
        edge: '#3E434B',
        edgeOpacity: 0.42,
        castShadow: false,
      }),
    );
  }

  if (gateIn > 0.01) {
    const barOpacity = gateIn;
    boxes.push(
      boxAt({
        id: 'gate-top',
        position: [0, GATE_Y + GATE_HALF, 0],
        size: [0.18, 0.12, 2.92],
        color: gateColor,
        opacity: barOpacity,
        edge: shade(COLORS.blueDeep, -0.2),
        edgeOpacity: 0.65,
        castShadow: false,
        glow: gateGlow,
      }),
      boxAt({
        id: 'gate-back',
        position: [0, GATE_Y, -1.42],
        size: [0.18, GATE_HALF * 2 + 0.1, 0.14],
        color: gateColor,
        opacity: barOpacity,
        edge: shade(COLORS.blueDeep, -0.2),
        edgeOpacity: 0.65,
        castShadow: false,
        glow: gateGlow,
      }),
      boxAt({
        id: 'gate-front',
        position: [0, GATE_Y, 1.42],
        size: [0.18, GATE_HALF * 2 + 0.1, 0.14],
        color: gateColor,
        opacity: barOpacity,
        edge: shade(COLORS.blueDeep, -0.2),
        edgeOpacity: 0.65,
        castShadow: false,
        glow: gateGlow,
      }),
    );

    const backFrame = mixHex(COLORS.blueDeep, COLORS.stageRaised, 0.18);
    boxes.push(
      boxAt({
        id: 'gate2-top',
        position: [-0.62, GATE_Y + GATE_HALF + 0.2, 0],
        size: [0.12, 0.09, 3.16],
        color: backFrame,
        opacity: barOpacity * 0.55,
        edge: COLORS.blueDeep,
        edgeOpacity: 0.4,
        castShadow: false,
      }),
      boxAt({
        id: 'gate2-back',
        position: [-0.62, GATE_Y, -1.54],
        size: [0.12, GATE_HALF * 2 + 0.3, 0.09],
        color: backFrame,
        opacity: barOpacity * 0.55,
        edge: COLORS.blueDeep,
        edgeOpacity: 0.4,
        castShadow: false,
      }),
      boxAt({
        id: 'gate2-front',
        position: [-0.62, GATE_Y, 1.54],
        size: [0.12, GATE_HALF * 2 + 0.3, 0.09],
        color: backFrame,
        opacity: barOpacity * 0.55,
        edge: COLORS.blueDeep,
        edgeOpacity: 0.4,
        castShadow: false,
      }),
    );

    boxes.push(
      boxAt({
        id: 'gate-cap-l',
        position: [0, GATE_Y + GATE_HALF + 0.1, -1.42],
        size: [0.22, 0.1, 0.22],
        color: COLORS.yellow,
        opacity: barOpacity * 0.9,
        edge: COLORS.yellowDeep,
        edgeOpacity: 0.5,
        castShadow: false,
      }),
      boxAt({
        id: 'gate-cap-r',
        position: [0, GATE_Y + GATE_HALF + 0.1, 1.42],
        size: [0.22, 0.1, 0.22],
        color: COLORS.yellow,
        opacity: barOpacity * 0.9,
        edge: COLORS.yellowDeep,
        edgeOpacity: 0.5,
        castShadow: false,
      }),
    );
  }

  for (const state of states) {
    const isActive = state.active;
    const verifying = f >= state.verifiedAt && f < state.verifiedAt + 8;
    const color = state.color;
    const edge = shade(color, -0.3);
    boxes.push(
      boxAt({
        id: `process-seg-${state.sourceIndex}`,
        position: state.position,
        size: [SEG[0] * state.scale, SEG[1] * state.scale, SEG[2] * state.scale],
        rotation: state.rotation,
        color,
        opacity: state.opacity,
        edge,
        edgeOpacity: 0.6,
        castShadow: true,
        shadowStrength: 0.75,
        glow: verifying
          ? 'rgba(66,169,135,0.55)'
          : isActive
            ? 'rgba(49,85,231,0.5)'
            : undefined,
      }),
    );
  }

  const ghostBoxes: BoxDef[] = [];
  for (const state of states) {
    if (!sourceHuskVisible(state.sourceIndex, f)) continue;
    const p = reclaimProgress(state.sourceIndex, f);
    const origin: Vec3 = [PIPELINE.sourceX(state.sourceIndex), PIPELINE.baseY, 0];
    if (p < 0.995) {
      ghostBoxes.push(
        boxAt({
          id: `husk-${state.sourceIndex}`,
          position: [origin[0], origin[1], origin[2]],
          size: [SEG[0], SEG[1] * (1 - p * 0.94), SEG[2] * (1 - p * 0.94)],
          color: COLORS.red,
          opacity: (1 - p) * 0.62,
          style: 'wire',
          edge: COLORS.red,
          castShadow: false,
        }),
      );
    }
    if (p > 0.04) {
      ghostBoxes.push(
        boxAt({
          id: `empty-${state.sourceIndex}`,
          position: [origin[0], origin[1], origin[2]],
          size: [SEG[0], SEG[1], SEG[2]],
          color: '#AAB4C8',
          opacity: 0.24 * smoothstep(0.04, 0.45, p),
          style: 'wire',
          edge: '#D2D9E6',
          castShadow: false,
        }),
      );
    }
  }

  const enhancementBoxes: BoxDef[] = [];

  for (let k = 0; k < PIPELINE.count; k++) {
    const T = PIPELINE.cycleStart + k * PIPELINE.cycleLength;
    const filled = f >= T + 24;
    if (!filled) {
      enhancementBoxes.push(
        boxAt({
          id: `slot-${k}`,
          position: [PIPELINE.outputX(k), PIPELINE.baseY, 0],
          size: [SEG[0], SEG[1], SEG[2]],
          color: '#9AA6C0',
          opacity: 0.22 * outputGhostIn,
          style: 'wire',
          edge: '#CBD5E8',
          castShadow: false,
        }),
      );
    }
  }

  const allBoxes = [...boxes, ...ghostBoxes, ...enhancementBoxes];

  const gateCenter: Vec3 = [0, PIPELINE.baseY, 0];
  const scanY = active
    ? PIPELINE.baseY + SEG[1] / 2 - active.gateProgress * SEG[1]
    : PIPELINE.baseY;

  const verifyProgress =
    active && f >= active.verifiedAt ? clamp01((f - active.verifiedAt) / 9) : -1;

  const railTicks = [];
  for (let i = 0; i < PIPELINE.count; i++) {
    for (const x of [PIPELINE.sourceX(i), PIPELINE.outputX(i)]) {
      railTicks.push(
        <WorldLine
          key={`tick-${x.toFixed(2)}`}
          camera={camera}
          from={[x, -0.558, -0.72]}
          to={[x, -0.558, 0.72]}
          color="#A7B3CB"
          opacity={0.13 * railIn}
          strokeWidth={0.8}
        />,
      );
    }
  }

  const gateHalo = projectPoint(gateCenter, camera);
  const sourceBL = projectPoint([PIPELINE.sourceX(0) - 0.42, -0.48, 0.68], camera);
  const sourceBR = projectPoint([PIPELINE.sourceX(5) + 0.42, -0.48, 0.68], camera);
  const sourceBracketY = Math.max(sourceBL.y, sourceBR.y) + 34;

  return (
    <AbsoluteFill
      style={{
        background:
          'radial-gradient(ellipse at 50% 46%, #25282F 0%, #191B1F 34%, #111215 66%, #0A0B0C 100%)',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '52%',
          width: 900,
          height: 720,
          transform: 'translate(-50%, -50%)',
          borderRadius: '50%',
          background: `radial-gradient(ellipse, ${COLORS.blue}3d 0%, ${COLORS.blue}14 45%, transparent 72%)`,
          filter: 'blur(40px)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: '17%',
          top: '54%',
          width: 560,
          height: 460,
          transform: 'translate(-50%, -50%)',
          borderRadius: '50%',
          background: `radial-gradient(ellipse, ${COLORS.red}22 0%, transparent 70%)`,
          filter: 'blur(50px)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: '83%',
          top: '52%',
          width: 560,
          height: 460,
          transform: 'translate(-50%, -50%)',
          borderRadius: '50%',
          background: `radial-gradient(ellipse, ${COLORS.green}1c 0%, transparent 70%)`,
          filter: 'blur(50px)',
        }}
      />

      <Stage3D
        camera={camera}
        boxes={allBoxes}
        floorY={-0.56}
        shadowColor="#000000"
        shadowOpacity={0.75}
        grid={{
          color: '#8198C7',
          opacity: 0.17,
          step: 0.55,
          extentX: 14,
          extentZ: 8.5,
          majorEvery: 4,
          blend: 'screen',
        }}
        gridY={-1.72}
        behind={
          <g>
            <circle cx={gateHalo.x} cy={gateHalo.y} r={296} fill="none" stroke={COLORS.blue} strokeOpacity={0.22} strokeWidth={1} />
            <circle cx={gateHalo.x} cy={gateHalo.y} r={382} fill="none" stroke={COLORS.blue} strokeOpacity={0.06} strokeWidth={1} />
            <line
              x1={0}
              y1={gateHalo.y + 318}
              x2={1920}
              y2={gateHalo.y + 318}
              stroke={COLORS.stageInk}
              strokeOpacity={0.055}
              strokeWidth={1}
            />
          </g>
        }
      >
        {railTicks}

        <g opacity={gateIn * 0.55}>
          <WorldPolygon
            camera={camera}
            points={[
              [0.02, PIPELINE.baseY - GATE_HALF + 0.06, -1.4],
              [0.02, PIPELINE.baseY - GATE_HALF + 0.06, 1.4],
              [0.02, PIPELINE.baseY + GATE_HALF - 0.06, 1.4],
              [0.02, PIPELINE.baseY + GATE_HALF - 0.06, -1.4],
            ]}
            color={COLORS.blue}
            fill={COLORS.blue}
            fillOpacity={0.07}
            stroke={COLORS.blue}
            strokeOpacity={0.22}
            strokeWidth={1}
          />
        </g>

        <g opacity={gateIn}>
          <WorldRing
            camera={camera}
            center={[0, GATE_Y, 0]}
            u={[0, 1, 0]}
            v={[0, 0, 1]}
            radius={1.06}
            color={COLORS.blue}
            opacity={0.6 + 0.2 * Math.sin(f * 0.16)}
            strokeWidth={3}
            filter="url(#stage-glow-blue)"
          />
          <WorldRing
            camera={camera}
            center={[0, GATE_Y, 0]}
            u={[0, 1, 0]}
            v={[0, 0, 1]}
            radius={1.23}
            color={COLORS.white}
            opacity={0.28}
            strokeWidth={1}
            dash="2 9"
          />
          <WorldRing
            camera={camera}
            center={[-0.55, GATE_Y, 0]}
            u={[0, 1, 0]}
            v={[0, 0, 1]}
            radius={1.06}
            color={COLORS.blue}
            opacity={0.28}
            strokeWidth={1.6}
          />
          <WorldRing
            camera={camera}
            center={[0.55, GATE_Y, 0]}
            u={[0, 1, 0]}
            v={[0, 0, 1]}
            radius={1.06}
            color={COLORS.blue}
            opacity={0.22}
            strokeWidth={1.4}
          />
          {(
            [
              [[-0.55, GATE_Y + 1.06, 0], [0.55, GATE_Y + 1.06, 0]],
              [[-0.55, GATE_Y, -1.06], [0.55, GATE_Y, -1.06]],
              [[-0.55, GATE_Y, 1.06], [0.55, GATE_Y, 1.06]],
            ] as Array<[Vec3, Vec3]>
          ).map(([from, to], i) => (
            <WorldLine
              key={`gate-rib-${i}`}
              camera={camera}
              from={from}
              to={to}
              color={COLORS.blue}
              opacity={0.26}
              strokeWidth={1}
            />
          ))}
          <WorldRing
            camera={camera}
            center={[0, GATE_Y, 0]}
            u={[0, 1, 0]}
            v={[0, 0, 1]}
            radius={1.4}
            color={COLORS.blue}
            opacity={0.18}
            strokeWidth={1}
            dash="1 10"
          />
          <WorldRing
            camera={camera}
            center={[0, -0.555, 0]}
            u={[1, 0, 0]}
            v={[0, 0, 1]}
            radius={1.75 + 0.03 * Math.sin(f * 0.14)}
            color={COLORS.blue}
            opacity={0.14}
            strokeWidth={1.1}
          />
        </g>

        <WorldLine
          camera={camera}
          from={[-5.25, -0.555, 0.76]}
          to={[5.25, -0.555, 0.76]}
          color="#72809C"
          opacity={0.4 * railIn}
          strokeWidth={1}
        />

        {active ? (
          <g>
            <WorldPolygon
              camera={camera}
              points={[
                [0.03, scanY, -1.01],
                [0.03, scanY, 1.01],
                [0.03, scanY + 0.015, 1.01],
                [0.03, scanY + 0.015, -1.01],
              ]}
              color={COLORS.white}
              fill={COLORS.white}
              fillOpacity={0.62}
              stroke={COLORS.blue}
              strokeOpacity={0.32}
              strokeWidth={0.8}
              filter="url(#stage-glow-blue)"
            />
            <WorldLine
              camera={camera}
              from={[0.035, scanY, -1.02]}
              to={[0.035, scanY, 1.02]}
              color={COLORS.white}
              opacity={0.9}
              strokeWidth={1.8}
              filter="url(#stage-glow-blue)"
            />
            <WorldLine
              camera={camera}
              from={[-0.55, scanY, 1.03]}
              to={[0.55, scanY, 1.03]}
              color={COLORS.blue}
              opacity={0.55}
              strokeWidth={1}
            />
          </g>
        ) : null}

        {verifyProgress >= 0 ? (
          <VerificationEffect
            camera={camera}
            center={gateCenter}
            progress={verifyProgress}
            opacity={1}
            radius={1.2}
          />
        ) : null}

        {states.map((state) => {
          const p = reclaimProgress(state.sourceIndex, f);
          if (p <= 0 || p >= 1) return null;
          return (
            <ReclaimAnimation
              key={`reclaim-${state.sourceIndex}`}
              camera={camera}
              origin={[PIPELINE.sourceX(state.sourceIndex), PIPELINE.baseY, 0]}
              progress={p}
              opacity={1}
              floorY={-0.555}
            />
          );
        })}

        {states.map((state) => {
          const T = PIPELINE.cycleStart + state.cycle * PIPELINE.cycleLength;
          const landed = f - (T + 24);
          if (landed < 0 || landed > 10) return null;
          return (
            <VerificationEffect
              key={`land-${state.sourceIndex}`}
              camera={camera}
              center={[PIPELINE.outputX(state.outputIndex), PIPELINE.baseY, 0]}
              progress={clamp01(landed / 10)}
              opacity={0.8}
              radius={0.75}
              color={COLORS.green}
            />
          );
        })}

        {states.map((state) => {
          const T = PIPELINE.cycleStart + state.cycle * PIPELINE.cycleLength;
          if (f < T + 15 || f > T + 22) return null;
          const p = clamp01((f - (T + 15)) / 7);
          const top: Vec3 = [PIPELINE.outputX(state.outputIndex), PIPELINE.baseY + SEG[1] / 2 + 0.22, 0];
          return (
            <g key={`landmark-${state.sourceIndex}`} opacity={1 - p}>
              <WorldDiamond
                camera={camera}
                center={top}
                size={0.055 + p * 0.05}
                color={COLORS.green}
                fillOpacity={0.9}
                opacity={1 - p}
                filter="url(#stage-glow-green)"
              />
            </g>
          );
        })}

        <g opacity={smoothstep(166, 180, f) * (1 - smoothstep(198, 210, f))}>
          <path
            d={`M${sourceBL.x} ${sourceBracketY - 12} L${sourceBL.x} ${sourceBracketY} L${sourceBR.x} ${sourceBracketY} L${sourceBR.x} ${sourceBracketY - 12}`}
            fill="none"
            stroke={COLORS.stageInk}
            strokeOpacity={0.34}
            strokeWidth={1}
          />
          <text
            x={Math.max(124, sourceBL.x)}
            y={sourceBracketY + 20}
            fill={COLORS.stageInk}
            fillOpacity={0.56}
            fontFamily='"Hack", monospace'
            fontSize={9.5}
            letterSpacing="1.8"
          >
            SPACE RECLAIMED
          </text>
          <text
            x={sourceBR.x}
            y={sourceBracketY + 20}
            textAnchor="end"
            fill={COLORS.stageInk}
            fillOpacity={0.36}
            fontFamily='"Hack", monospace'
            fontSize={9.5}
            letterSpacing="1.6"
          >
            6 / 6
          </text>
        </g>
      </Stage3D>

      <SceneTag
        index="03"
        title="SEGMENTED PROCESSING"
        detail="TAIL-FIRST · REMUX ONLY"
        color={COLORS.stageInk}
        dark
        style={{left: 92, top: 62}}
      />

      <div style={{position: 'absolute', left: 92, top: 218, display: 'flex', alignItems: 'center', gap: 10}}>
        <Dot color={COLORS.red} size={6} />
        <Tech size={10} color={COLORS.stageInk} opacity={0.7} tracking={0.2}>
          Source
        </Tech>
        <Tech size={9} color={COLORS.stageInk} opacity={0.3} tracking={0.14}>
          60 GB · MKV
        </Tech>
      </div>

      <div style={{position: 'absolute', left: 92, top: 246, width: 220, height: 1, background: COLORS.stageInk, opacity: 0.12}} />
      <Tech
        size={8.5}
        color={COLORS.stageInk}
        opacity={0.38}
        tracking={0.16}
        style={{position: 'absolute', left: 92, top: 258}}
      >
        Reclaimed {reclaimed} / 6
      </Tech>

      <div style={{position: 'absolute', right: 92, top: 218, display: 'flex', alignItems: 'center', gap: 10}}>
        <Tech size={9} color={COLORS.stageInk} opacity={0.3} tracking={0.14}>
          MP4 · VERIFIED
        </Tech>
        <Tech size={10} color={COLORS.stageInk} opacity={0.7} tracking={0.2}>
          Output
        </Tech>
        <Dot color={COLORS.green} size={6} />
      </div>

      <div style={{position: 'absolute', right: 92, top: 246, width: 220, height: 1, background: COLORS.stageInk, opacity: 0.12}} />
      <div style={{position: 'absolute', right: 92, top: 258, display: 'flex', gap: 4}}>
        {Array.from({length: 6}).map((_, k) => {
          const T = PIPELINE.cycleStart + k * PIPELINE.cycleLength;
          const done = f >= T + 24;
          const activeSlot = gateK === k;
          return (
            <div
              key={k}
              style={{
                width: 33,
                height: 2,
                backgroundColor: done ? COLORS.green : activeSlot ? COLORS.blue : COLORS.stageInk,
                opacity: done ? 0.95 : activeSlot ? 0.9 : 0.16,
              }}
            />
          );
        })}
      </div>

      <div
        style={{
          position: 'absolute',
          left: 92,
          top: 334,
          display: 'flex',
          alignItems: 'center',
          gap: 9,
          opacity: smoothstep(56, 72, f) * (1 - smoothstep(186, 198, f)),
        }}
      >
        <div style={{width: 24, height: 1, background: COLORS.stageInk, opacity: 0.3}} />
        <Tech size={8.5} color={COLORS.stageInk} opacity={0.42} tracking={0.17}>
          Space returned to the source
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          bottom: 74,
          transform: 'translateX(-50%)',
          display: 'flex',
          alignItems: 'center',
          gap: 34,
          opacity: smoothstep(16, 30, f) * (1 - smoothstep(198, 210, f)),
        }}
      >
        {[
          {n: '01', label: 'Remux', color: COLORS.blue, on: true},
          {n: '02', label: 'Verify', color: COLORS.green, on: gateK >= 0},
          {
            n: '03',
            label: 'Reclaim',
            color: COLORS.stageInk,
            on: states.some((s) => {
              const p = reclaimProgress(s.sourceIndex, f);
              return p > 0 && p < 1;
            }),
          },
        ].map((step, i) => (
          <React.Fragment key={step.label}>
            {i > 0 ? <div style={{width: 22, height: 1, backgroundColor: COLORS.stageInk, opacity: 0.14}} /> : null}
            <div style={{display: 'flex', alignItems: 'center', gap: 9, opacity: step.on ? 1 : 0.32}}>
              <div style={{width: 5, height: 5, borderRadius: 3, backgroundColor: step.color}} />
              <Tech size={8.5} color={COLORS.stageInk} tracking={0.04} opacity={0.42}>
                {step.n}
              </Tech>
              <Tech size={9.5} color={COLORS.stageInk} tracking={0.2} weight={700}>
                {step.label}
              </Tech>
            </div>
          </React.Fragment>
        ))}
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          bottom: 128,
          transform: `translateX(-50%) translateY(${(1 - smoothstep(168, 182, f)) * 8}px)`,
          opacity: smoothstep(168, 182, f) * (1 - smoothstep(198, 210, f)),
        }}
      >
        <Tech size={9} color={COLORS.stageInk} opacity={0.44} tracking={0.26}>
          One segment at a time
        </Tech>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 92,
          top: 334,
          display: 'flex',
          alignItems: 'center',
          gap: 9,
          opacity: smoothstep(168, 182, f) * (1 - smoothstep(198, 210, f)),
        }}
      >
        <Tech size={8.5} color={COLORS.green} opacity={0.8} tracking={0.18} weight={700}>
          {verified} / 6 verified
        </Tech>
        <div style={{width: 1, height: 10, background: COLORS.stageInk, opacity: 0.2}} />
        <Tech size={8.5} color={COLORS.stageInk} opacity={0.36} tracking={0.16}>
          Atomic finalisation
        </Tech>
      </div>
    </AbsoluteFill>
  );
};
