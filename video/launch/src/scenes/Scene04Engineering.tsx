import type {CSSProperties} from 'react';
import {interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {MaskedText} from '../components/MaskedText';
import {NumberMorph} from '../components/NumberMorph';
import {EASE, hash, range} from '../lib/anim';
import {COLORS, FONTS, GRID} from '../theme';

const micro: CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 11,
  letterSpacing: '0.17em',
  textTransform: 'uppercase',
  color: COLORS.muted,
  lineHeight: 1.5,
};

const N = 5;
const GAP = 14;
const BLOCK_W = (GRID.contentWidth - GAP * (N - 1)) / N;
const SOURCE_TOP = 346;
const OUTPUT_TOP = 616;
const BLOCK_H = 104;

const Ticks: React.FC<{index: number; color: string}> = ({index, color}) => (
  <div
    style={{
      position: 'absolute',
      right: 14,
      bottom: 14,
      height: 34,
      display: 'flex',
      alignItems: 'flex-end',
      gap: 3,
    }}
  >
    {Array.from({length: 13}, (_, i) => (
      <span
        key={i}
        style={{
          display: 'block',
          width: 2,
          height: `${24 + hash(index * 31 + i * 7) * 76}%`,
          background: color,
          opacity: 0.65,
        }}
      />
    ))}
  </div>
);

const SourceBlock: React.FC<{index: number; frame: number; processStart: number}> = ({
  index,
  frame,
  processStart,
}) => {
  const left = index * (BLOCK_W + GAP);
  const leaving = frame >= processStart;
  const arrived = frame >= processStart + 14;

  return (
    <div
      style={{
        position: 'absolute',
        left,
        top: SOURCE_TOP,
        width: BLOCK_W,
        height: BLOCK_H,
        border: leaving ? `1px dashed ${COLORS.hair}` : `1px solid ${COLORS.hair}`,
        background: leaving ? 'transparent' : 'rgba(243, 241, 232, 0.045)',
        opacity: leaving ? 0.44 : 1,
        transition: undefined,
      }}
    >
      {!leaving ? (
        <>
          <div
            style={{
              position: 'absolute',
              left: 14,
              top: 6,
              fontFamily: FONTS.mono,
              fontWeight: 700,
              fontSize: 70,
              lineHeight: 1,
              letterSpacing: '-0.08em',
              color: COLORS.ink,
              opacity: 0.86,
            }}
          >
            0{index + 1}
          </div>
          <div style={{position: 'absolute', right: 14, top: 12, ...micro, fontSize: 9, color: COLORS.inkSoft}}>
            MKV
          </div>
          <Ticks index={index} color={COLORS.ink} />
        </>
      ) : (
        <>
          <div
            style={{
              position: 'absolute',
              left: 14,
              top: 22,
              ...micro,
              color: COLORS.dim,
              letterSpacing: '0.14em',
            }}
          >
            {arrived ? 'truncated' : 'releasing'}
          </div>
          <div
            style={{
              position: 'absolute',
              right: 14,
              bottom: 16,
              fontFamily: FONTS.mono,
              fontSize: 20,
              color: COLORS.dim,
            }}
          >
            {arrived ? '—' : '···'}
          </div>
        </>
      )}
    </div>
  );
};

const OutputSlot: React.FC<{index: number}> = ({index}) => {
  const left = index * (BLOCK_W + GAP);
  return (
    <div
      style={{
        position: 'absolute',
        left,
        top: OUTPUT_TOP,
        width: BLOCK_W,
        height: BLOCK_H,
        border: `1px dashed ${COLORS.hairSoft}`,
      }}
    >
      <div style={{position: 'absolute', right: 12, bottom: 10, ...micro, fontSize: 9, color: COLORS.dim}}>
        MP4
      </div>
    </div>
  );
};

const MovingBlock: React.FC<{index: number; frame: number}> = ({index, frame}) => {
  const {fps} = useVideoConfig();
  const start = 42 + (N - 1 - index) * 20;
  const move = spring({
    frame: frame - start,
    fps,
    config: {damping: 19, stiffness: 120, mass: 0.9},
    durationInFrames: 17,
  });
  const visible = frame >= start;
  const landed = frame >= start + 14;
  const top = SOURCE_TOP + (OUTPUT_TOP - SOURCE_TOP) * move;
  const left = index * (BLOCK_W + GAP);
  const active = frame >= start && frame < start + 18;

  if (!visible) {
    return null;
  }

  return (
    <div
      style={{
        position: 'absolute',
        left,
        top,
        width: BLOCK_W,
        height: BLOCK_H,
        background: landed ? COLORS.accent : COLORS.ink,
        color: landed ? COLORS.bg : COLORS.bg,
        zIndex: 4,
        boxShadow: active ? `0 0 0 1px ${COLORS.accent}, 0 0 38px rgba(201,242,77,${0.12 + (landed ? 0.1 : 0)})` : 'none',
        transform: `scale(${1 + Math.sin(Math.PI * Math.min(1, Math.max(0, move))) * 0.035})`,
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 14,
          top: 6,
          fontFamily: FONTS.mono,
          fontWeight: 700,
          fontSize: 70,
          lineHeight: 1,
          letterSpacing: '-0.08em',
        }}
      >
        0{index + 1}
      </div>

      <div
        style={{
          position: 'absolute',
          right: 14,
          top: 12,
          fontFamily: FONTS.mono,
          fontSize: 9,
          letterSpacing: '0.14em',
          textTransform: 'uppercase',
          color: landed ? 'rgba(8,9,9,0.62)' : 'rgba(8,9,9,0.55)',
        }}
      >
        {landed ? 'verified' : 'moving'}
      </div>

      <Ticks index={index} color={COLORS.bg} />
    </div>
  );
};

export const Scene04Engineering: React.FC = () => {
  const frame = useCurrentFrame();

  if (frame >= 200) {
    return <div style={{position: 'absolute', inset: 0, background: COLORS.bg}} />;
  }

  const statementIn = range(frame, 146, 158, 0, 1, EASE.outExpo);
  const gridOpacity = 1 - statementIn * 0.82;
  const hintPulse = 0.5 + Math.sin(frame / 5) * 0.2;

  return (
    <div style={{position: 'absolute', inset: 0, background: COLORS.bg, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 118,
          display: 'flex',
          alignItems: 'center',
          gap: 16,
          opacity: range(frame, 2, 16, 0, 0.9),
        }}
      >
        <span style={{...micro, color: COLORS.accent}}>04</span>
        <span style={{width: 26, height: 1, background: COLORS.hair}} />
        <span style={{...micro, color: COLORS.inkSoft}}>relocate mode / mkv → mp4</span>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 96,
          top: 118,
          display: 'flex',
          alignItems: 'baseline',
          gap: 12,
          opacity: range(frame, 8, 24, 0, 1),
        }}
      >
        <span style={{...micro}}>pending</span>
        <NumberMorph
          from={5}
          to={0}
          start={44}
          duration={98}
          pad={2}
          style={{
            fontFamily: FONTS.mono,
            fontWeight: 700,
            fontSize: 24,
            color: COLORS.accent,
            letterSpacing: '-0.05em',
          }}
        />
        <span style={{...micro}}>segments</span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 168,
          right: 96,
          display: 'flex',
          alignItems: 'baseline',
          gap: 22,
          opacity: 0.35 + 0.65 * gridOpacity,
        }}
      >
        {['SEGMENT', 'VERIFY', 'RECLAIM'].map((word, i) => (
          <div key={word} style={{display: 'flex', alignItems: 'baseline', gap: 22}}>
            {i > 0 ? (
              <span
                style={{
                  fontFamily: FONTS.mono,
                  fontSize: 30,
                  color: COLORS.accent,
                  opacity: range(frame, 12 + i * 8, 22 + i * 8, 0, 0.75),
                }}
              >
                →
              </span>
            ) : null}
            <MaskedText start={8 + i * 8} duration={12} direction="up">
              <span
                style={{
                  fontFamily: FONTS.mono,
                  fontWeight: 700,
                  fontSize: 54,
                  letterSpacing: '-0.02em',
                  color: i === 2 ? COLORS.accent : COLORS.ink,
                  whiteSpace: 'nowrap',
                }}
              >
                {word}
                <span style={{color: i === 2 ? COLORS.ink : COLORS.accent}}>.</span>
              </span>
            </MaskedText>
          </div>
        ))}
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 300,
          display: 'flex',
          alignItems: 'baseline',
          gap: 16,
          opacity: range(frame, 20, 36, 0, 0.9) * (1 - statementIn),
        }}
      >
        <span style={{...micro, color: COLORS.inkSoft}}>source</span>
        <span style={{...micro, color: COLORS.dim}}>/ five keyframe-aligned segments</span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 574,
          display: 'flex',
          alignItems: 'baseline',
          gap: 16,
          opacity: range(frame, 30, 46, 0, 0.9) * (1 - statementIn),
        }}
      >
        <span style={{...micro, color: COLORS.accent}}>output</span>
        <span style={{...micro, color: COLORS.dim}}>/ assembled at final offsets</span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 512,
          display: 'flex',
          alignItems: 'center',
          gap: 14,
          ...micro,
          color: COLORS.dim,
          opacity: range(frame, 34, 50, 0, 1) * (1 - statementIn),
        }}
      >
        <span>tail-first</span>
        <span style={{color: COLORS.accent, letterSpacing: '0.08em'}}>05 → 01</span>
        <span
          style={{
            width: 7,
            height: 7,
            background: COLORS.accent,
            display: 'inline-block',
            opacity: hintPulse,
            transform: 'rotate(45deg)',
          }}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 0,
          width: GRID.contentWidth,
          height: OUTPUT_TOP + BLOCK_H,
          opacity: gridOpacity,
          transform: `translateY(${statementIn * -14}px) scale(${1 - statementIn * 0.018})`,
          transformOrigin: '50% 30%',
        }}
      >
        {Array.from({length: N}, (_, i) => {
          const processStart = 42 + (N - 1 - i) * 20;
          return (
            <SourceBlock key={`source-${i}`} index={i} frame={frame} processStart={processStart} />
          );
        })}
        {Array.from({length: N}, (_, i) => (
          <OutputSlot key={`out-${i}`} index={i} />
        ))}
        {Array.from({length: N}, (_, i) => (
          <MovingBlock key={`moving-${i}`} index={i} frame={frame} />
        ))}
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 786,
          display: 'flex',
          alignItems: 'center',
          gap: 18,
          opacity: range(frame, 52, 70, 0, 1) * (1 - statementIn * 0.6),
        }}
      >
        <span style={{width: 40, height: 1, background: COLORS.accent, display: 'inline-block'}} />
        <span style={{...micro, color: COLORS.inkSoft}}>peak disk = source + one segment</span>
        <span style={{...micro, color: COLORS.dim}}>/ not a second copy</span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          top: 282,
          opacity: statementIn,
          transform: `translateY(${(1 - statementIn) * 38}px)`,
        }}
      >
        <MaskedText start={146} duration={12} direction="up">
          <div
            style={{
              fontFamily: FONTS.display,
              fontWeight: 900,
              fontSize: 126,
              lineHeight: 0.9,
              letterSpacing: '-0.055em',
              color: COLORS.ink,
              whiteSpace: 'nowrap',
            }}
          >
            <span style={{color: COLORS.accent}}>ONE</span> SEGMENT
          </div>
        </MaskedText>
        <MaskedText start={151} duration={12} direction="up">
          <div
            style={{
              fontFamily: FONTS.display,
              fontWeight: 900,
              fontSize: 126,
              lineHeight: 0.9,
              letterSpacing: '-0.055em',
              color: COLORS.ink,
              whiteSpace: 'nowrap',
            }}
          >
            AT A TIME.
          </div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          top: 742,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'baseline',
          opacity: statementIn * 0.9,
        }}
      >
        <span style={{...micro, color: COLORS.inkSoft}}>verified before every truncate</span>
        <span style={{...micro, color: COLORS.accent}}>journaled / exact resume</span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          bottom: 110,
          opacity: 1 - statementIn,
        }}
      >
        <span style={{...micro, color: COLORS.dim}}>
          illustrative: five segments · production uses 60 s keyframe-aligned windows
        </span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: `${interpolate(frame, [146, 160], [0, 100], {
            extrapolateLeft: 'clamp',
            extrapolateRight: 'clamp',
            easing: EASE.outExpo,
          })}%`,
          height: 2,
          background: COLORS.accent,
          opacity: statementIn * 0.8,
        }}
      />
    </div>
  );
};
