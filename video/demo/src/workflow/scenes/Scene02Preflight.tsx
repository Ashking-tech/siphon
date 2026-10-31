import React from 'react';
import {AbsoluteFill, interpolate, useCurrentFrame} from 'remotion';
import {C, FONTS} from '../theme';
import {PREFLIGHT_CUES as Q} from '../timeline';
import {fall, ramp} from '../lib/motion';
import {Chip, Hairline, Kicker, MonoLabel, StageTitle} from '../components/Stage';
import {CheckIcon, FileFilm, InfoIcon, LockIcon} from '../components/Icons';

type CheckState = 'pending' | 'active' | 'done';

const CHECK_START = [Q.check1, Q.check2, Q.check3];
const CHECK_LENGTH = 18;

const stateAt = (frame: number, index: number): CheckState => {
  const start = CHECK_START[index];
  if (frame >= start + CHECK_LENGTH) return 'done';
  if (frame >= start) return 'active';
  return 'pending';
};

const progressAt = (frame: number, index: number) =>
  interpolate(frame, [CHECK_START[index], CHECK_START[index] + CHECK_LENGTH], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });

const StatusDot: React.FC<{state: CheckState; frame: number}> = ({state, frame}) => {
  if (state === 'done') {
    return (
      <div
        style={{
          width: 32,
          height: 32,
          borderRadius: 16,
          background: C.green,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          flex: 'none',
        }}
      >
        <CheckIcon size={17} color={C.white} strokeWidth={2.6} />
      </div>
    );
  }
  if (state === 'active') {
    return (
      <div style={{width: 32, height: 32, flex: 'none', position: 'relative'}}>
        <svg width="32" height="32" viewBox="0 0 32 32">
          <circle cx="16" cy="16" r="13" fill={C.blueWash} stroke="#CDD8FF" strokeWidth="1.5" />
          <g transform={`rotate(${(frame * 13) % 360} 16 16)`}>
            <path d="M16 5.5a10.5 10.5 0 0 1 10.5 10.5" fill="none" stroke={C.blue} strokeWidth="2.2" strokeLinecap="round" />
          </g>
        </svg>
      </div>
    );
  }
  return (
    <div
      style={{
        width: 32,
        height: 32,
        borderRadius: 16,
        border: `1.5px solid #D7DAE0`,
        background: '#FFFFFF',
        flex: 'none',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <div style={{width: 5, height: 5, borderRadius: 3, background: '#C9CCD1'}} />
    </div>
  );
};

const CheckRow: React.FC<{
  index: number;
  frame: number;
  label: string;
  detail: string;
  result: React.ReactNode;
  resultText: string;
}> = ({index, frame, label, detail, result, resultText}) => {
  const state = stateAt(frame, index);
  const progress = progressAt(frame, index);
  const active = state === 'active';
  const resultIn = ramp(frame, CHECK_START[index] + CHECK_LENGTH, 7);
  return (
    <div
      style={{
        position: 'relative',
        height: 104,
        background: active ? '#F7F9FF' : 'transparent',
        borderLeft: active || state === 'done' ? `3px solid ${state === 'done' ? C.green : C.blue}` : '3px solid transparent',
        padding: '0 20px 0 24px',
        boxSizing: 'border-box',
        display: 'flex',
        alignItems: 'center',
        gap: 17,
      }}
    >
      <StatusDot state={state} frame={frame} />
      <div style={{flex: 1, minWidth: 0}}>
        <div
          style={{
            fontFamily: FONTS.ui,
            fontSize: 18.5,
            fontWeight: 600,
            letterSpacing: '-0.01em',
            color: state === 'pending' ? '#8A8F96' : C.ink,
          }}
        >
          {label}
        </div>
        <div style={{marginTop: 5, fontFamily: FONTS.ui, fontSize: 12.5, color: C.muted}}>{detail}</div>
      </div>
      <div style={{width: 264, display: 'flex', justifyContent: 'flex-end', alignItems: 'center'}}>
        {state === 'done' ? (
          <div style={{opacity: resultIn, transform: `translateX(${(1 - resultIn) * 8}px)`}}>{result}</div>
        ) : state === 'active' ? (
          <div
            style={{
              fontFamily: FONTS.mono,
              fontSize: 9.5,
              letterSpacing: '0.12em',
              color: C.blue,
              display: 'flex',
              alignItems: 'center',
              gap: 7,
            }}
          >
            <span style={{width: 6, height: 6, borderRadius: 3, background: C.blue, opacity: 0.55 + 0.45 * Math.sin(frame / 3)}} />
            CHECKING
          </div>
        ) : (
          <div style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '0.1em', color: '#C2C5CA'}}>WAITING</div>
        )}
      </div>
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 0,
          height: 2,
          background: state === 'done' ? C.green : C.blue,
          width: `${(state === 'done' ? 1 : progress) * 100}%`,
          opacity: state === 'pending' ? 0 : 1,
        }}
      />
      <span style={{display: 'none'}}>{resultText}</span>
    </div>
  );
};

const ResultCell: React.FC<{
  label: string;
  value: React.ReactNode;
  note: string;
  opacity: number;
  border?: boolean;
  accent?: string;
}> = ({label, value, note, opacity, border = true, accent = C.ink}) => (
  <div
    style={{
      flex: 1,
      padding: '0 25px',
      borderLeft: border ? `1px solid ${C.hairlineSoft}` : undefined,
      boxSizing: 'border-box',
    }}
  >
    <div style={{opacity: 0.35 + opacity * 0.65}}>
      <MonoLabel>{label}</MonoLabel>
    </div>
    <div
      style={{
        marginTop: 8,
        fontFamily: FONTS.display,
        fontSize: 31,
        fontWeight: 700,
        letterSpacing: '-0.03em',
        color: accent,
        whiteSpace: 'nowrap',
        opacity,
        transform: `translateY(${(1 - opacity) * 8}px)`,
      }}
    >
      {value}
    </div>
    <div style={{marginTop: 5, fontFamily: FONTS.ui, fontSize: 11.5, color: C.muted, opacity}}>{note}</div>
  </div>
);

export const Scene02Preflight: React.FC = () => {
  const frame = useCurrentFrame();

  const panelIn = ramp(frame, 0, 14);
  const cardIn = ramp(frame, Q.cardIn, 13);
  const connector = ramp(frame, Q.connector, 10);
  const tokenP = ramp(frame, Q.token, 12);
  const tokenOpacity = ramp(frame, Q.token, 4) * fall(frame, Q.token + 12, 6);
  const ready = ramp(frame, Q.ready, 8);

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      {/* A faint engineering grid, flat and very light. */}
      <AbsoluteFill
        style={{
          backgroundImage:
            'linear-gradient(to right, rgba(23,23,23,0.030) 1px, transparent 1px), linear-gradient(to bottom, rgba(23,23,23,0.030) 1px, transparent 1px)',
          backgroundSize: '64px 64px',
          opacity: 0.72,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          top: 0,
          height: 6,
          background: C.blue,
        }}
      />

      <div style={{position: 'absolute', left: 120, top: 54}}>
        <Kicker code="02" label="Preflight" />
      </div>
      <StageTitle style={{position: 'absolute', left: 120, top: 84}}>Plan before writing.</StageTitle>
      <div style={{position: 'absolute', left: 120, top: 138, fontSize: 15, color: C.muted, fontFamily: FONTS.ui}}>
        Metadata only. The source is untouched until the plan is sound.
      </div>
      <div style={{position: 'absolute', right: 120, top: 78, display: 'flex', flexDirection: 'column', alignItems: 'flex-end', gap: 9}}>
        <Chip tone="blue">MKV → MP4</Chip>
        <MonoLabel color="#9A9EA5">SIPHON ENGINE · PREFLIGHT</MonoLabel>
      </div>
      <Hairline style={{position: 'absolute', left: 120, right: 120, top: 176}} />

      {/* Source card */}
      <div
        style={{
          position: 'absolute',
          left: 120,
          top: 206,
          width: 400,
          height: 474,
          borderRadius: 16,
          border: `1px solid ${C.hairline}`,
          background: C.white,
          boxSizing: 'border-box',
          opacity: cardIn,
          transform: `translateX(${(1 - cardIn) * -22}px)`,
          overflow: 'hidden',
        }}
      >
        <div style={{height: 4, background: C.red}} />
        <div style={{padding: '24px 28px 0'}}>
          <MonoLabel color={C.red}>Selected source</MonoLabel>
          <div style={{display: 'flex', gap: 18, marginTop: 20}}>
            <FileFilm size={88} accent={C.red} format="MKV" />
            <div style={{paddingTop: 5, minWidth: 0}}>
              <div
                style={{
                  fontFamily: FONTS.ui,
                  fontSize: 18,
                  fontWeight: 600,
                  lineHeight: 1.3,
                  letterSpacing: '-0.01em',
                  color: C.ink,
                }}
              >
                interstellar_4k.mkv
              </div>
              <div style={{marginTop: 9, fontFamily: FONTS.mono, fontSize: 9.5, lineHeight: 1.7, color: C.muted, letterSpacing: '0.05em'}}>
                4K · 45 Mbps
                <br />
                ≈ 2 h 58 min
              </div>
            </div>
          </div>

          <div
            style={{
              marginTop: 27,
              fontFamily: FONTS.display,
              fontSize: 58,
              lineHeight: 1,
              fontWeight: 800,
              letterSpacing: '-0.045em',
              color: C.ink,
            }}
          >
            60 GB
          </div>
          <div style={{marginTop: 8, fontFamily: FONTS.ui, fontSize: 12.5, color: C.muted}}>Matroska container</div>

          <div style={{marginTop: 32, display: 'flex', alignItems: 'center', gap: 8}}>
            <div style={{flex: 1, height: 7, borderRadius: 4, background: '#F2D7D5', overflow: 'hidden'}}>
              <div style={{width: '100%', height: '100%', background: C.red}} />
            </div>
            <span style={{fontFamily: FONTS.mono, fontSize: 9, color: C.red, letterSpacing: '.07em'}}>SOURCE</span>
          </div>
        </div>
      </div>

      {/* A quiet assertion that preflight is read-only. */}
      <div
        style={{
          position: 'absolute',
          left: 124,
          top: 710,
          display: 'flex',
          alignItems: 'center',
          gap: 9,
          opacity: ramp(frame, 8, 10),
        }}
      >
        <LockIcon size={15} color={C.green} />
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '0.08em', color: C.green}}>NO BYTES WRITTEN</span>
        <span style={{fontFamily: FONTS.ui, fontSize: 12, color: C.muted}}>preflight is read-only</span>
      </div>

      {/* Connector and travelling source token */}
      <div
        style={{
          position: 'absolute',
          left: 520,
          top: 432,
          width: 130 * connector,
          height: 1,
          borderTop: `1px dashed ${C.blue}`,
          opacity: connector,
        }}
      />
      <div style={{position: 'absolute', left: 643, top: 425, opacity: connector}}>
        <svg width="11" height="14" viewBox="0 0 11 14">
          <path d="M1 1l8 6-8 6z" fill={C.blue} />
        </svg>
      </div>
      <div
        style={{
          position: 'absolute',
          left: 520 + tokenP * 130,
          top: 416,
          width: 25,
          height: 34,
          borderRadius: 4,
          background: C.red,
          opacity: tokenOpacity,
          transform: `translateY(${Math.sin(tokenP * Math.PI) * -7}px)`,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: '#fff',
          fontFamily: FONTS.mono,
          fontSize: 8,
          fontWeight: 700,
          letterSpacing: '.05em',
        }}
      >
        MKV
      </div>

      {/* Engine panel */}
      <div
        style={{
          position: 'absolute',
          left: 650,
          top: 206,
          width: 1150,
          height: 700,
          borderRadius: 18,
          border: `1px solid ${C.hairline}`,
          background: C.white,
          boxSizing: 'border-box',
          opacity: panelIn,
          transform: `translateY(${(1 - panelIn) * 14}px)`,
          overflow: 'hidden',
          boxShadow: '0 10px 34px rgba(23,23,23,0.035)',
        }}
      >
        <div
          style={{
            height: 82,
            padding: '0 36px',
            display: 'flex',
            alignItems: 'center',
            borderBottom: `1px solid ${C.hairlineSoft}`,
            boxSizing: 'border-box',
          }}
        >
          <div>
            <MonoLabel color={C.blue}>Preflight gate</MonoLabel>
            <div style={{marginTop: 7, fontFamily: FONTS.ui, fontSize: 16, fontWeight: 600, color: C.ink}}>
              Three checks, one plan.
            </div>
          </div>
          <div style={{flex: 1}} />
          <div style={{display: 'flex', alignItems: 'center', gap: 10}}>
            <Chip tone={ready > 0.5 ? 'green' : 'neutral'} style={{height: 26, fontSize: 9}}>
              {ready > 0.5 ? 'PLAN READY' : 'READING'}
            </Chip>
            <span style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '.08em', color: C.muted}}>
              {stateAt(frame, 0) === 'done' && stateAt(frame, 1) === 'done' && stateAt(frame, 2) === 'done'
                ? '3 / 3'
                : `${[0, 1, 2].filter((i) => stateAt(frame, i) === 'done').length} / 3`}
            </span>
          </div>
        </div>

        <CheckRow
          index={0}
          frame={frame}
          label="Inspect source file"
          detail="container · tracks · duration · bitrate"
          result={<Chip tone="grey">4K · 45 MBPS</Chip>}
          resultText="4K · 45 Mbps"
        />
        <CheckRow
          index={1}
          frame={frame}
          label="Check available disk space"
          detail="relocate needs one segment, not a second copy"
          result={<Chip tone="green">1.2 GB free</Chip>}
          resultText="1.2 GB free"
        />
        <CheckRow
          index={2}
          frame={frame}
          label="Determine the conversion strategy"
          detail="remux MKV → fragmented MP4 · tail-first"
          result={<Chip tone="blue">RELOCATE</Chip>}
          resultText="Relocate"
        />

        <div style={{position: 'absolute', left: 36, right: 36, top: 626}}>
          <Hairline />
        </div>

        <div style={{position: 'absolute', left: 36, right: 36, top: 650, display: 'flex'}}>
          <ResultCell
            label="Source"
            value="60 GB"
            note="Matroska · 4K"
            opacity={ramp(frame, Q.check1 + CHECK_LENGTH, 8)}
            border={false}
          />
          <ResultCell
            label="Target"
            value="MP4"
            note="fragmented output"
            opacity={ramp(frame, Q.check1 + CHECK_LENGTH + 2, 8)}
          />
          <ResultCell
            label="Mode"
            value="Relocate"
            note="in place · tail-first"
            opacity={ramp(frame, Q.check3 + CHECK_LENGTH, 8)}
            accent={C.blue}
          />
          <ResultCell
            label="Additional space *"
            value="≈ 340 MB"
            note="peak · one 60 s segment"
            opacity={ramp(frame, Q.check2 + CHECK_LENGTH, 8)}
            accent={C.ink}
          />
        </div>

        <div
          style={{
            position: 'absolute',
            left: 36,
            right: 36,
            bottom: 30,
            display: 'flex',
            alignItems: 'flex-start',
            gap: 8,
            opacity: ramp(frame, Q.results + 6, 8),
          }}
        >
          <span style={{color: C.yellow, marginTop: -1}}>
            <InfoIcon size={14} />
          </span>
          <div style={{fontFamily: FONTS.ui, fontSize: 11.5, lineHeight: 1.45, color: C.muted}}>
            <span style={{color: C.ink, fontWeight: 600}}>* Example only.</span> 60 s × 45 Mbps ≈ 337 MB, plus the
            16 MiB reserve. This is peak headroom for this example — not a second 60 GB copy.
          </div>
        </div>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 120,
          right: 120,
          bottom: 40,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontFamily: FONTS.mono,
          fontSize: 9.5,
          letterSpacing: '0.09em',
          color: '#9A9EA5',
        }}
      >
        <span>SIPHON / RELOCATE ENGINE</span>
        <span>SOURCE READ-ONLY DURING PREFLIGHT</span>
      </div>
    </AbsoluteFill>
  );
};
