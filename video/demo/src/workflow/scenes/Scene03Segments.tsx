import React from 'react';
import {AbsoluteFill, interpolate, useCurrentFrame} from 'remotion';
import {C, FONTS} from '../theme';
import {SEGMENT_CUES as Q} from '../timeline';
import {clamp, ramp} from '../lib/motion';
import {CheckIcon, ChevronRight, InfoIcon} from '../components/Icons';
import {Chip, Hairline, Kicker, MonoLabel, StageTitle} from '../components/Stage';

const SEG_SIZE = 6;
const SEG_WIDTH = (1120 - (SEG_SIZE - 1) * 8) / SEG_SIZE;
const ORDER = [5, 4, 3, 2, 1, 0];

const segmentTiming = (index: number) => {
  const position = SEG_SIZE - 1 - index;
  const start = Q.firstCycle + position * Q.cycleFrames;
  return {
    start,
    local: (frame: number) => frame - start,
  };
};

const SourceSegment: React.FC<{index: number; frame: number}> = ({index, frame}) => {
  const {start, local} = segmentTiming(index);
  const l = local(frame);
  const done = l >= Q.cycleFrames;
  const active = l >= 0 && !done;

  const blue = active ? 1 : done ? 1 - ramp(frame, start + 16, 4) : 0;
  const grey = done ? ramp(frame, start + 16, 5) : 0;
  const enter = ramp(frame, index * 2, 8);
  const reclaimed = grey > 0.55;

  return (
    <div
      style={{
        position: 'relative',
        width: SEG_WIDTH,
        height: 88,
        borderRadius: 8,
        background: C.red,
        overflow: 'hidden',
        opacity: enter,
        transform: `translateY(${(active ? -4 : 0) + (1 - enter) * 10}px)`,
        outline: active ? `2px solid ${C.blue}` : '2px solid transparent',
        outlineOffset: 2,
        boxShadow: active ? '0 8px 22px rgba(49,85,231,0.16)' : undefined,
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: 0,
          background: C.blue,
          opacity: blue,
        }}
      />
      <div
        style={{
          position: 'absolute',
          inset: 0,
          background: C.grey,
          opacity: grey,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 13,
          top: 12,
          fontFamily: FONTS.mono,
          fontSize: 10.5,
          fontWeight: 700,
          letterSpacing: '.08em',
          color: reclaimed ? '#7D8289' : '#FFFFFF',
        }}
      >
        SEG {String(index + 1).padStart(2, '0')}
      </div>
      <div
        style={{
          position: 'absolute',
          left: 13,
          bottom: 12,
          fontFamily: FONTS.mono,
          fontSize: 8,
          letterSpacing: '.08em',
          color: reclaimed ? '#92979D' : 'rgba(255,255,255,0.72)',
        }}
      >
        {reclaimed ? 'RECLAIMED' : active ? 'IN PROCESS' : '60 s'}
      </div>
      {active ? (
        <div
          style={{
            position: 'absolute',
            right: 11,
            top: 11,
            width: 7,
            height: 7,
            borderRadius: 4,
            background: C.white,
            opacity: 0.55 + 0.45 * Math.sin(frame / 2.5),
          }}
        />
      ) : null}
    </div>
  );
};

const OutputSlot: React.FC<{index: number; frame: number}> = ({index, frame}) => {
  const {start, local} = segmentTiming(index);
  const l = local(frame);
  const fill = interpolate(l, [4, 12], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });
  const verified = l >= 12;
  const checkIn = verified ? ramp(frame, start + 12, 5) : 0;
  const enter = ramp(frame, index * 2 + 2, 8);

  return (
    <div
      style={{
        position: 'relative',
        width: SEG_WIDTH,
        height: 88,
        borderRadius: 8,
        border: `1px dashed ${verified ? C.green : '#D8DBE0'}`,
        background: '#FAFAFB',
        overflow: 'hidden',
        opacity: enter,
        transform: `translateY(${(1 - enter) * 10}px)`,
        boxSizing: 'border-box',
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: 0,
          background: C.green,
          clipPath: `inset(0 0 0 ${(1 - fill) * 100}%)`,
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: 13,
          top: 12,
          fontFamily: FONTS.mono,
          fontSize: 9,
          letterSpacing: '.08em',
          color: verified ? 'rgba(255,255,255,0.82)' : '#B3B7BD',
        }}
      >
        {String(index + 1).padStart(2, '0')}
      </div>
      {verified ? (
        <div
          style={{
            position: 'absolute',
            left: 13,
            bottom: 11,
            display: 'flex',
            alignItems: 'center',
            gap: 6,
            opacity: checkIn,
            transform: `scale(${0.8 + checkIn * 0.2})`,
            transformOrigin: 'left bottom',
          }}
        >
          <div
            style={{
              width: 17,
              height: 17,
              borderRadius: 9,
              background: '#FFFFFF',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <CheckIcon size={11} color={C.green} strokeWidth={3} />
          </div>
          <span style={{fontFamily: FONTS.mono, fontSize: 8, letterSpacing: '.08em', color: '#FFFFFF', fontWeight: 700}}>
            VERIFIED
          </span>
        </div>
      ) : (
        <div
          style={{
            position: 'absolute',
            left: 13,
            bottom: 12,
            fontFamily: FONTS.mono,
            fontSize: 8,
            letterSpacing: '.08em',
            color: '#B3B7BD',
          }}
        >
          EMPTY
        </div>
      )}
      {fill > 0 && !verified ? (
        <div
          style={{
            position: 'absolute',
            right: 12,
            top: 12,
            width: 6,
            height: 6,
            borderRadius: 3,
            background: C.white,
          }}
        />
      ) : null}
    </div>
  );
};

type StepState = 'pending' | 'active' | 'done';

const StepRow: React.FC<{
  number: string;
  label: string;
  detail: string;
  state: StepState;
  frame: number;
  tone: string;
  activeWash?: string;
}> = ({number, label, detail, state, frame, tone, activeWash = '#F7F9FF'}) => (
  <div
    style={{
      position: 'relative',
      height: 92,
      padding: '0 18px',
      background: state === 'active' ? activeWash : 'transparent',
      borderLeft: state === 'pending' ? '3px solid transparent' : `3px solid ${state === 'done' ? C.green : tone}`,
      display: 'flex',
      alignItems: 'center',
      gap: 13,
      boxSizing: 'border-box',
    }}
  >
    <div
      style={{
        width: 26,
        height: 26,
        borderRadius: 13,
        border: `1.5px solid ${state === 'pending' ? '#D7DAE0' : state === 'done' ? C.green : tone}`,
        background: state === 'done' ? C.green : state === 'active' ? '#FFFFFF' : '#FFFFFF',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flex: 'none',
      }}
    >
      {state === 'done' ? (
        <CheckIcon size={14} color={C.white} strokeWidth={2.8} />
      ) : (
        <span style={{fontFamily: FONTS.mono, fontSize: 8.5, fontWeight: 700, color: state === 'active' ? tone : '#A8ACB2'}}>
          {number}
        </span>
      )}
    </div>
    <div style={{flex: 1}}>
      <div
        style={{
          fontFamily: FONTS.ui,
          fontSize: 15,
          fontWeight: 600,
          color: state === 'pending' ? '#8C9198' : C.ink,
        }}
      >
        {label}
      </div>
      <div style={{marginTop: 3, fontFamily: FONTS.ui, fontSize: 11, color: C.muted}}>{detail}</div>
    </div>
    <div style={{width: 36, display: 'flex', justifyContent: 'flex-end'}}>
      {state === 'active' ? (
        <svg width="20" height="20" viewBox="0 0 20 20">
          <circle cx="10" cy="10" r="8" fill="none" stroke={tone} strokeWidth="1.2" opacity=".2" />
          <g transform={`rotate(${(frame * 15) % 360} 10 10)`}>
            <path d="M10 2a8 8 0 0 1 8 8" fill="none" stroke={tone} strokeWidth="2" strokeLinecap="round" />
          </g>
        </svg>
      ) : state === 'done' ? (
        <span style={{fontFamily: FONTS.mono, fontSize: 8, letterSpacing: '.08em', color: C.green}}>DONE</span>
      ) : (
        <span style={{fontFamily: FONTS.mono, fontSize: 8, letterSpacing: '.08em', color: '#C2C5CA'}}>WAIT</span>
      )}
    </div>
  </div>
);

const CycleItem: React.FC<{
  label: string;
  color: string;
  active: boolean;
  done: boolean;
}> = ({label, color, active, done}) => (
  <div
    style={{
      flex: 1,
      height: 54,
      borderRadius: 9,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      gap: 9,
      background: active ? `${color}14` : 'transparent',
      border: `1px solid ${active ? `${color}3D` : 'transparent'}`,
      boxSizing: 'border-box',
    }}
  >
    <span
      style={{
        width: 9,
        height: 9,
        borderRadius: 2,
        background: done || active ? color : '#D5D8DD',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      {done ? <CheckIcon size={7} color={C.white} strokeWidth={3} /> : null}
    </span>
    <span
      style={{
        fontFamily: FONTS.mono,
        fontSize: 10,
        fontWeight: 700,
        letterSpacing: '.09em',
        color: active ? color : done ? '#6E737A' : '#A7ABB1',
      }}
    >
      {label}
    </span>
  </div>
);

const SupportColumn: React.FC<{
  label: string;
  body: string;
  color: string;
  opacity: number;
}> = ({label, body, color, opacity}) => (
  <div style={{flex: 1, opacity, transform: `translateY(${(1 - opacity) * 8}px)`}}>
    <div style={{height: 3, width: 34, borderRadius: 2, background: color}} />
    <div
      style={{
        marginTop: 12,
        fontFamily: FONTS.mono,
        fontSize: 9.5,
        fontWeight: 700,
        letterSpacing: '.11em',
        color,
      }}
    >
      {label}
    </div>
    <div style={{marginTop: 7, fontFamily: FONTS.ui, fontSize: 13.5, lineHeight: 1.45, color: C.muted, maxWidth: 300}}>
      {body}
    </div>
  </div>
);

export const Scene03Segments: React.FC = () => {
  const frame = useCurrentFrame();
  const cyclesElapsed = Math.floor((frame - Q.firstCycle) / Q.cycleFrames);
  const cycleIndex = clamp(cyclesElapsed, 0, SEG_SIZE - 1);
  const segmentIndex = ORDER[cycleIndex];
  const cycleStart = Q.firstCycle + cycleIndex * Q.cycleFrames;
  const local = frame - cycleStart;
  const allDone = frame >= Q.done;
  const activeSegment = frame >= Q.firstCycle && !allDone ? segmentIndex : null;
  const displaySegment = allDone ? 0 : activeSegment ?? ORDER[0];
  const cycleLocal = Math.max(0, local);

  const processState: StepState = allDone || cycleLocal >= 12 ? 'done' : frame < Q.firstCycle ? 'pending' : cycleLocal >= 2 ? 'active' : 'pending';
  const verifyState: StepState = allDone || cycleLocal >= 16 ? 'done' : frame < Q.firstCycle ? 'pending' : cycleLocal >= 12 ? 'active' : 'pending';
  const reclaimState: StepState = allDone || cycleLocal >= 21 ? 'done' : frame < Q.firstCycle ? 'pending' : cycleLocal >= 16 ? 'active' : 'pending';

  const railStage = allDone ? 4 : frame < Q.firstCycle ? -1 : cycleLocal < 4 ? 0 : cycleLocal < 12 ? 1 : cycleLocal < 16 ? 2 : 3;
  const railDone = allDone;

  const supportIn = ramp(frame, 6, 14);

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      <AbsoluteFill
        style={{
          backgroundImage:
            'linear-gradient(to right, rgba(23,23,23,0.026) 1px, transparent 1px), linear-gradient(to bottom, rgba(23,23,23,0.026) 1px, transparent 1px)',
          backgroundSize: '64px 64px',
        }}
      />
      <div style={{position: 'absolute', left: 0, right: 0, top: 0, height: 6, background: C.blue}} />

      <div style={{position: 'absolute', left: 120, top: 54}}>
        <Kicker code="03" label="Segmented processing" />
      </div>
      <StageTitle style={{position: 'absolute', left: 120, top: 84}}>One segment at a time.</StageTitle>
      <div style={{position: 'absolute', left: 120, top: 138, fontFamily: FONTS.ui, fontSize: 15, color: C.muted}}>
        The source and the output trade bytes. The film is never copied whole.
      </div>
      <div style={{position: 'absolute', right: 120, top: 80, display: 'flex', alignItems: 'center', gap: 10}}>
        <Chip tone="blue">RELOCATE</Chip>
        <Chip tone="neutral">REMUX · NO RE-ENCODE</Chip>
      </div>
      <div style={{position: 'absolute', right: 120, top: 121, display: 'flex', alignItems: 'center', gap: 8}}>
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '.09em', color: C.blue}}>TAIL-FIRST</span>
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, color: '#A0A4AA'}}>06 → 01</span>
        <span style={{width: 1, height: 11, background: C.hairline}} />
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, color: C.muted}}>60 s WINDOWS</span>
      </div>
      <Hairline style={{position: 'absolute', left: 120, right: 120, top: 176}} />

      {/* Source row */}
      <div style={{position: 'absolute', left: 120, top: 213, width: 1120, display: 'flex', alignItems: 'center'}}>
        <span style={{width: 7, height: 7, borderRadius: 2, background: C.red}} />
        <span style={{marginLeft: 9, fontFamily: FONTS.mono, fontSize: 9.5, fontWeight: 700, letterSpacing: '.09em', color: C.ink}}>
          SOURCE · INTERSTELLAR_4K.MKV
        </span>
        <div style={{flex: 1}} />
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '.07em', color: C.muted}}>
          ORIGINAL 60 GB · {SEG_SIZE} OF ~178 SEGMENTS
        </span>
      </div>
      <div style={{position: 'absolute', left: 120, top: 244, width: 1120, display: 'flex', gap: 8}}>
        {Array.from({length: SEG_SIZE}, (_, i) => (
          <SourceSegment key={i} index={i} frame={frame} />
        ))}
      </div>

      {/* Output row */}
      <div style={{position: 'absolute', left: 120, top: 441, width: 1120, display: 'flex', alignItems: 'center'}}>
        <span style={{width: 7, height: 7, borderRadius: 2, background: C.green}} />
        <span style={{marginLeft: 9, fontFamily: FONTS.mono, fontSize: 9.5, fontWeight: 700, letterSpacing: '.09em', color: C.ink}}>
          OUTPUT · INTERSTELLAR_4K.MP4
        </span>
        <div style={{flex: 1}} />
        <span style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '.07em', color: C.muted}}>
          FRAGMENTED MP4 · WRITTEN ONCE
        </span>
      </div>
      <div style={{position: 'absolute', left: 120, top: 472, width: 1120, display: 'flex', gap: 8}}>
        {Array.from({length: SEG_SIZE}, (_, i) => (
          <OutputSlot key={i} index={i} frame={frame} />
        ))}
      </div>

      {/* Cycle rail */}
      <div
        style={{
          position: 'absolute',
          left: 120,
          top: 609,
          width: 1120,
          height: 96,
          borderRadius: 14,
          border: `1px solid ${C.hairline}`,
          background: '#FFFFFF',
          display: 'flex',
          alignItems: 'center',
          padding: '0 18px',
          gap: 6,
          boxSizing: 'border-box',
          opacity: ramp(frame, 2, 10),
        }}
      >
        <CycleItem label="SEGMENT" color={C.red} active={railStage === 0} done={railDone || railStage > 0} />
        <ChevronRight size={15} color="#C3C6CB" />
        <CycleItem label="PROCESS" color={C.blue} active={railStage === 1} done={railDone || railStage > 1} />
        <ChevronRight size={15} color="#C3C6CB" />
        <CycleItem label="VERIFY" color={C.green} active={railStage === 2} done={railDone || railStage > 2} />
        <ChevronRight size={15} color="#C3C6CB" />
        <CycleItem label="RECLAIM" color="#8D9298" active={railStage === 3} done={railDone || railStage > 3} />
      </div>

      {/* Three supporting facts */}
      <div style={{position: 'absolute', left: 120, top: 745, width: 1120, display: 'flex', gap: 52}}>
        <SupportColumn
          label="ONE PASS"
          body="Every source byte is read once and written once."
          color={C.blue}
          opacity={supportIn}
        />
        <SupportColumn
          label="ONE SEGMENT"
          body="Peak extra space is one segment, not a second full film."
          color={C.yellow}
          opacity={ramp(frame, 9, 14)}
        />
        <SupportColumn
          label="VERIFY FIRST"
          body="The source tail is truncated only after the output segment checks out."
          color={C.green}
          opacity={ramp(frame, 12, 14)}
        />
      </div>

      {/* Processing station */}
      <div
        style={{
          position: 'absolute',
          left: 1300,
          top: 206,
          width: 500,
          height: 620,
          borderRadius: 16,
          border: `1px solid ${C.hairline}`,
          background: '#FFFFFF',
          boxSizing: 'border-box',
          overflow: 'hidden',
          boxShadow: '0 10px 34px rgba(23,23,23,0.035)',
        }}
      >
        <div
          style={{
            height: 132,
            padding: '22px 24px 0',
            borderBottom: `1px solid ${C.hairlineSoft}`,
            boxSizing: 'border-box',
            position: 'relative',
          }}
        >
          <div style={{display: 'flex', alignItems: 'center'}}>
            <MonoLabel color={C.blue}>{allDone ? 'Complete' : frame < Q.firstCycle ? 'Queued' : 'In flight'}</MonoLabel>
            <div style={{flex: 1}} />
            <Chip tone={allDone ? 'green' : 'blue'} style={{height: 24, fontSize: 8.5}}>
              {allDone ? '6 / 6 VERIFIED' : `${SEG_SIZE - cycleIndex} LEFT`}
            </Chip>
          </div>
          <div
            style={{
              marginTop: 9,
              fontFamily: FONTS.display,
              fontSize: 35,
              lineHeight: 1,
              fontWeight: 800,
              letterSpacing: '-0.03em',
              color: C.ink,
            }}
          >
            SEG {String(displaySegment + 1).padStart(2, '0')}
          </div>
          <div style={{marginTop: 6, fontFamily: FONTS.mono, fontSize: 9, letterSpacing: '.07em', color: C.muted}}>
            60 s · 4K · 45 Mbps
          </div>
          <div
            style={{
              position: 'absolute',
              left: 24,
              right: 24,
              bottom: 0,
              height: 2,
              background: '#EEF0F3',
            }}
          >
            <div
              style={{
                width: `${allDone ? 100 : frame < Q.firstCycle ? 0 : (Math.min(cycleLocal, Q.cycleFrames) / Q.cycleFrames) * 100}%`,
                height: '100%',
                background: allDone ? C.green : C.blue,
              }}
            />
          </div>
        </div>

        <StepRow
          number="01"
          label="Process"
          detail="remux · copy streams"
          state={processState}
          frame={frame}
          tone={C.blue}
        />
        <StepRow
          number="02"
          label="Verify"
          detail="fragment checks · sample decode"
          state={verifyState}
          frame={frame}
          tone={C.green}
        />
        <StepRow
          number="03"
          label="Reclaim"
          detail="truncate source tail"
          state={reclaimState}
          frame={frame}
          tone="#8D9298"
          activeWash="#F7F7F8"
        />

        <div style={{position: 'absolute', left: 24, right: 24, bottom: 26, paddingTop: 20, borderTop: `1px solid ${C.hairlineSoft}`}}>
          <div style={{display: 'flex', alignItems: 'flex-end'}}>
            <div>
              <MonoLabel color={C.muted}>Peak extra space</MonoLabel>
              <div
                style={{
                  marginTop: 7,
                  fontFamily: FONTS.display,
                  fontSize: 30,
                  fontWeight: 700,
                  letterSpacing: '-0.03em',
                  color: C.ink,
                }}
              >
                ≈ 340 MB
              </div>
            </div>
            <div style={{flex: 1}} />
            <Chip tone="yellow" style={{height: 24, fontSize: 8.5}}>
              NOT A FULL COPY
            </Chip>
          </div>
          <div style={{marginTop: 7, fontFamily: FONTS.ui, fontSize: 11, color: C.muted}}>
            one 60 s segment · 45 Mbps source
          </div>
        </div>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 120,
          right: 120,
          bottom: 36,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          fontFamily: FONTS.mono,
          fontSize: 9.5,
          letterSpacing: '.08em',
          color: '#9A9EA5',
        }}
      >
        <span>ILLUSTRATIVE · {SEG_SIZE} OF ~178 SEGMENTS · 60 s WINDOWS</span>
        <span style={{display: 'flex', alignItems: 'center', gap: 7}}>
          <InfoIcon size={13} color={C.muted} />
          REMUX COPIES STREAMS — NO VIDEO OR AUDIO IS RE-ENCODED
        </span>
      </div>
    </AbsoluteFill>
  );
};
