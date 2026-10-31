import React from 'react';
import {AbsoluteFill} from 'remotion';
import {BAR, C, FONTS, SEG_WIDTH, SEGMENTS, segCenterX, segX} from '../theme';
import {T, segmentTiming} from '../timeline';
import {clamp, lerp, ramp} from '../motion';
import {CheckIcon, FileGlyph, SiphonMark} from './Icons';

const smooth = (t: number) => {
  const x = clamp(t);
  return x * x * (3 - 2 * x);
};

const MonoLabel: React.FC<{
  children: React.ReactNode;
  x: number;
  y: number;
  color?: string;
  size?: number;
  letterSpacing?: string;
  align?: 'left' | 'right';
}> = ({children, x, y, color = C.inkFaint, size = 10, letterSpacing = '.12em', align = 'left'}) => (
  <div
    style={{
      position: 'absolute',
      left: align === 'left' ? x : undefined,
      right: align === 'right' ? 1280 - x : undefined,
      top: y,
      fontFamily: FONTS.mono,
      fontSize: size,
      lineHeight: 1.2,
      letterSpacing,
      color,
      whiteSpace: 'nowrap',
    }}
  >
    {children}
  </div>
);

const SourceSegment: React.FC<{index: number; frame: number}> = ({index, frame}) => {
  const {start} = segmentTiming(index);
  const l = frame - start;
  const done = frame >= T.processEnd || l >= 16;
  const active = l >= 0 && l < 16;
  const reveal = ramp(frame, 98 + index * 1.6, 8);
  const reclaim = done ? 1 : l >= 13 ? ramp(frame, start + 13, 3) : 0;
  const lift = active && l < 3 ? -2 : 0;

  return (
    <div
      style={{
        position: 'absolute',
        left: segX(index),
        top: BAR.sourceY,
        width: SEG_WIDTH,
        height: BAR.sourceH,
        borderRadius: 2,
        background: C.reclaimed,
        opacity: reveal,
        transform: `translateY(${(1 - reveal) * 5 + lift}px)`,
        outline: active ? `1.5px solid ${C.blue}` : '1px solid transparent',
        outlineOffset: 1.5,
        boxSizing: 'border-box',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: 0,
          background: C.ink,
          opacity: 1 - reclaim,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 7,
          top: 5,
          fontFamily: FONTS.mono,
          fontSize: 7,
          letterSpacing: '.06em',
          color: reclaim > 0.55 ? '#9A9EA5' : 'rgba(255,255,255,0.62)',
        }}
      >
        {String(index + 1).padStart(2, '0')}
      </div>
      {reclaim > 0.45 ? (
        <div
          style={{
            position: 'absolute',
            right: 6,
            bottom: 5,
            width: 9,
            height: 1,
            background: '#B8BCC2',
            opacity: reclaim,
          }}
        />
      ) : null}
    </div>
  );
};

const OutputSlot: React.FC<{index: number; frame: number}> = ({index, frame}) => {
  const {start} = segmentTiming(index);
  const l = frame - start;
  const done = frame >= T.processEnd || l >= 13;
  const fill = done ? 1 : l >= 13 ? ramp(frame, start + 13, 2) : 0;
  const reveal = ramp(frame, 104 + index * 1.6, 8);
  const checkIn = done ? ramp(frame, start + 13, 4) : 0;

  return (
    <div
      style={{
        position: 'absolute',
        left: segX(index),
        top: BAR.outputY,
        width: SEG_WIDTH,
        height: BAR.outputH,
        borderRadius: 2,
        border: `1px dashed ${done ? C.green : '#D4D5D8'}`,
        background: '#FAFAFA',
        opacity: reveal,
        transform: `translateY(${(1 - reveal) * 5}px)`,
        boxSizing: 'border-box',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: 0,
          background: C.green,
          clipPath: `inset(0 ${(1 - fill) * 100}% 0 0)`,
        }}
      />
      {fill > 0.1 ? (
        <div
          style={{
            position: 'absolute',
            inset: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            opacity: checkIn,
          }}
        >
          <CheckIcon size={14} strokeWidth={3} />
        </div>
      ) : (
        <div
          style={{
            position: 'absolute',
            left: 7,
            top: 5,
            fontFamily: FONTS.mono,
            fontSize: 7,
            letterSpacing: '.06em',
            color: '#B9BDC3',
          }}
        >
          {String(index + 1).padStart(2, '0')}
        </div>
      )}
    </div>
  );
};

const Token: React.FC<{frame: number}> = ({frame}) => {
  const rel = frame - T.processStart;
  if (rel < 0 || rel >= T.cycles * T.cycle) return null;
  const cycleIndex = Math.floor(rel / T.cycle);
  const local = rel - cycleIndex * T.cycle;
  const index = 11 - cycleIndex;
  if (local < 3 || local >= 16) return null;

  const sourceCenter = BAR.sourceY + BAR.sourceH / 2;
  const processCenter = BAR.processY;
  const outputCenter = BAR.outputY + BAR.outputH / 2;

  let y = processCenter;
  if (local < 6) {
    y = lerp(sourceCenter, processCenter, smooth((local - 3) / 3));
  } else if (local >= 13) {
    y = lerp(processCenter, outputCenter, smooth((local - 13) / 3));
  }

  const isVerified = local >= 11;
  const progress = ramp(local, 6, 5);
  const fade = local >= 15 ? 1 - ramp(local, 15, 1) : 1;
  const color = isVerified ? C.green : C.blue;
  const x = segCenterX(index);

  return (
    <>
      <div
        style={{
          position: 'absolute',
          left: x - 0.5,
          top: BAR.sourceY + BAR.sourceH,
          width: 1,
          height: BAR.outputY - BAR.sourceY - BAR.sourceH,
          borderLeft: `1px dashed ${isVerified ? 'rgba(66,169,135,0.32)' : 'rgba(49,85,231,0.28)'}`,
          zIndex: 1,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: x - 34,
          top: y - 16,
          width: 68,
          height: 32,
          borderRadius: 2,
          background: color,
          opacity: fade,
          zIndex: 4,
          boxShadow: `0 0 0 1.5px ${isVerified ? 'rgba(66,169,135,0.22)' : 'rgba(49,85,231,0.22)'}`,
          overflow: 'hidden',
        }}
      >
        {isVerified ? (
          <div style={{position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
            <CheckIcon size={15} strokeWidth={3} />
          </div>
        ) : (
          <>
            <div
              style={{
                position: 'absolute',
                top: 0,
                bottom: 0,
                width: 2,
                left: `${8 + progress * 76}%`,
                background: 'rgba(255,255,255,0.6)',
              }}
            />
            <div
              style={{
                position: 'absolute',
                left: 0,
                bottom: 0,
                height: 2,
                width: `${progress * 100}%`,
                background: 'rgba(255,255,255,0.86)',
              }}
            />
          </>
        )}
      </div>
    </>
  );
};

const StepLegend: React.FC<{frame: number}> = ({frame}) => {
  const rel = frame - T.processStart;
  const active = rel >= 0 && rel < T.cycles * T.cycle;
  const local = active ? rel % T.cycle : 0;
  const activeStep = !active ? -1 : local < 3 ? 0 : local < 11 ? 1 : local < 13 ? 2 : 3;
  const doneAll = frame >= T.processEnd;
  const steps = ['READ', 'PROCESS', 'VERIFY', 'RECLAIM'];

  return (
    <div
      style={{
        position: 'absolute',
        left: BAR.x,
        top: 214,
        width: BAR.width,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 13,
        zIndex: 3,
      }}
    >
      {steps.map((step, i) => {
        const done = doneAll || activeStep > i;
        const current = activeStep === i;
        const color = done ? C.green : current ? (i === 2 ? C.green : C.blue) : '#B2B6BC';
        return (
          <React.Fragment key={step}>
            {i > 0 ? (
              <span style={{fontFamily: FONTS.mono, fontSize: 9, color: '#C9CCD1', marginTop: 1}}>→</span>
            ) : null}
            <div style={{display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 3}}>
              <span
                style={{
                  fontFamily: FONTS.mono,
                  fontSize: 9.5,
                  letterSpacing: '.12em',
                  color,
                  fontWeight: current ? 700 : 400,
                }}
              >
                {step}
              </span>
              <div
                style={{
                  width: current ? 22 : 0,
                  height: 1.5,
                  background: color,
                  opacity: current ? 1 : 0,
                }}
              />
            </div>
          </React.Fragment>
        );
      })}
    </div>
  );
};

const DiskSpace: React.FC<{frame: number}> = ({frame}) => {
  const draw = ramp(frame, T.diskIn, 14);
  const rel = frame - T.processStart;
  const processing = rel >= 0 && rel < T.cycles * T.cycle;
  const local = processing ? rel % T.cycle : 0;
  const sliceActive = processing && local >= 3 && local < 14;
  const doneAll = frame >= T.processEnd;
  const used = 960;
  const boundary = BAR.x + used;

  return (
    <div style={{opacity: draw}}>
      <MonoLabel x={64} y={468} color={C.inkFaint}>
        DISK SPACE
      </MonoLabel>
      <MonoLabel x={1216} y={468} align="right" color={C.inkSoft} letterSpacing=".02em">
        60.0 GB used · 4.0 GB free
      </MonoLabel>

      <div
        style={{
          position: 'absolute',
          left: BAR.x,
          top: BAR.diskY,
          width: BAR.width,
          height: BAR.diskH,
          borderRadius: 2,
          background: C.reclaimed,
          transform: `scaleX(${draw})`,
          transformOrigin: 'left center',
          overflow: 'hidden',
        }}
      >
        <div style={{position: 'absolute', left: 0, top: 0, width: used, height: '100%', background: C.ink}} />
        <div
          style={{
            position: 'absolute',
            left: boundary - 14,
            top: 0,
            width: 14,
            height: '100%',
            background: C.blue,
            opacity: sliceActive ? 1 : 0,
          }}
        />
        <div
          style={{
            position: 'absolute',
            left: boundary,
            top: 0,
            bottom: 0,
            width: 1,
            background: 'rgba(255,255,255,0.8)',
          }}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          left: boundary - 7,
          top: BAR.diskY - 5,
          width: 0,
          height: 0,
          borderLeft: '4px solid transparent',
          borderRight: '4px solid transparent',
          borderTop: `4px solid ${sliceActive ? C.blue : 'transparent'}`,
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: 64,
          top: 516,
          fontFamily: FONTS.mono,
          fontSize: 9.5,
          letterSpacing: '.02em',
          color: doneAll ? C.green : C.blue,
        }}
      >
        {doneAll ? '✓ space reclaimed · no second copy' : 'peak extra ≈ 340 MB · one segment in flight'}
      </div>
      <div
        style={{
          position: 'absolute',
          right: 64,
          top: 516,
          fontFamily: FONTS.mono,
          fontSize: 9.5,
          letterSpacing: '.02em',
          color: C.inkFaint,
        }}
      >
        classic copy would add <span style={{color: C.red}}>+60 GB</span>
      </div>
    </div>
  );
};

export const InfographicScene: React.FC<{frame: number}> = ({frame}) => {
  const enter = ramp(frame, T.infoIn, 14);
  const doneAll = frame >= T.processEnd;
  const headerY = (1 - enter) * 8;
  const verifiedCount = Array.from({length: SEGMENTS}, (_, i) => segmentTiming(i)).filter(
    ({start}) => frame >= T.processEnd || frame >= start + 13,
  ).length;

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      <div style={{opacity: enter, transform: `translateY(${headerY}px)`}}>
        <div style={{position: 'absolute', left: 64, top: 22, display: 'flex', alignItems: 'center', gap: 8}}>
          <SiphonMark size={14} color={C.ink} />
          <span style={{fontFamily: FONTS.mono, fontSize: 10, letterSpacing: '.12em', color: C.inkFaint}}>
            {frame < T.processStart ? '02 / CONVERT' : '03 / PROCESS'}
          </span>
        </div>
        <div
          style={{
            position: 'absolute',
            left: 64,
            top: 38,
            fontFamily: FONTS.display,
            fontSize: 31,
            lineHeight: 1.05,
            fontWeight: 500,
            letterSpacing: '-0.025em',
            color: C.ink,
          }}
        >
          Convert in place.
        </div>
        <div style={{position: 'absolute', right: 64, top: 28, textAlign: 'right'}}>
          <div style={{fontFamily: FONTS.mono, fontSize: 10.5, letterSpacing: '.08em', color: C.ink}}>
            MKV → MP4
          </div>
          <div style={{marginTop: 4, fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '.03em', color: C.inkFaint}}>
            remux, no re-encode
          </div>
        </div>
      </div>

      <MonoLabel x={64} y={162} color={C.inkFaint}>
        SOURCE
      </MonoLabel>
      <MonoLabel x={64} y={176} color="#A4A8AE" size={9} letterSpacing=".04em">
        tail-first
      </MonoLabel>

      <div style={{position: 'absolute', left: 192, top: 134, display: 'flex', alignItems: 'center', gap: 8, opacity: enter}}>
        <FileGlyph format="MKV" size={24} />
        <span
          style={{
            fontFamily: FONTS.ui,
            fontSize: 12.5,
            fontWeight: 500,
            color: doneAll ? C.inkFaint : C.ink,
            textDecoration: doneAll ? 'line-through' : 'none',
          }}
        >
          interstellar_4k.mkv
        </span>
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, color: doneAll ? '#A4A8AE' : '#7C8188'}}>
          {doneAll ? 'reclaimed' : '60 GB'}
        </span>
      </div>

      {Array.from({length: SEGMENTS}, (_, i) => (
        <SourceSegment key={i} index={i} frame={frame} />
      ))}

      <MonoLabel x={64} y={332} color={doneAll ? C.green : C.inkFaint}>
        OUTPUT
      </MonoLabel>
      <MonoLabel x={64} y={346} color={doneAll ? C.green : '#A4A8AE'} size={9} letterSpacing=".04em">
        {doneAll ? 'verified' : 'destination'}
      </MonoLabel>
      <MonoLabel
        x={1216}
        y={332}
        align="right"
        color={verifiedCount > 0 ? C.green : '#A4A8AE'}
        size={9.5}
        letterSpacing=".06em"
      >
        {String(verifiedCount).padStart(2, '0')} / 12 verified
      </MonoLabel>

      <div style={{position: 'absolute', left: 192, top: 304, display: 'flex', alignItems: 'center', gap: 8, opacity: enter}}>
        <FileGlyph format="MP4" size={24} accent={doneAll ? C.green : '#8D9299'} verified={doneAll} />
        <span style={{fontFamily: FONTS.ui, fontSize: 12.5, fontWeight: 500, color: doneAll ? C.green : '#6F747B'}}>
          interstellar_4k.mp4
        </span>
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, color: doneAll ? C.green : '#9A9EA5'}}>
          {doneAll ? '60.0 GB' : 'reserved'}
        </span>
      </div>

      {Array.from({length: SEGMENTS}, (_, i) => (
        <OutputSlot key={i} index={i} frame={frame} />
      ))}

      <StepLegend frame={frame} />
      <Token frame={frame} />

      <MonoLabel x={1188} y={284} align="right" color="#A8ACB2" size={9} letterSpacing=".04em">
        one segment in flight
      </MonoLabel>

      <DiskSpace frame={frame} />

      <div
        style={{
          position: 'absolute',
          left: 64,
          right: 64,
          bottom: 66,
          borderTop: `1px solid ${C.hairlineSoft}`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 64,
          bottom: 36,
          fontFamily: FONTS.mono,
          fontSize: 9.5,
          lineHeight: 1.65,
          letterSpacing: '.01em',
          color: '#9DA1A7',
        }}
      >
        Illustrative: 60 GB MKV · 4K · 45 Mbps · 60 s segments. Peak extra ≈ 340 MB.
        <br />
        Relocate mode only. Free space varies with bitrate and --segment-duration; other modes differ.
      </div>
    </AbsoluteFill>
  );
};
