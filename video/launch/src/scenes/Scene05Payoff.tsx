import type {CSSProperties} from 'react';
import {interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {MaskedText} from '../components/MaskedText';
import {SliceTransition} from '../components/SliceTransition';
import {TypeScale} from '../components/TypeScale';
import {EASE, range} from '../lib/anim';
import {COLORS, FONTS} from '../theme';

const micro: CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 11,
  letterSpacing: '0.19em',
  textTransform: 'uppercase',
  color: COLORS.muted,
  lineHeight: 1.6,
};

const display: CSSProperties = {
  fontFamily: FONTS.display,
  fontWeight: 900,
  lineHeight: 0.8,
  color: COLORS.ink,
};

const SourceNumber: React.FC = () => {
  const frame = useCurrentFrame();
  const breath = interpolate(frame, [10, 38], [1, 1.024], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });

  return (
    <div
      style={{
        position: 'absolute',
        left: 50,
        top: 158,
        transform: `scale(${breath})`,
        transformOrigin: '28% 55%',
      }}
    >
      <TypeScale start={0} from={1.13} to={1} duration={22} origin="20% 70%">
        <div style={{...display, fontSize: 650, letterSpacing: '-0.078em', whiteSpace: 'nowrap'}}>
          60<span style={{letterSpacing: '-0.01em'}}> </span>GB
          <span
            style={{
              color: COLORS.accent,
              fontSize: '0.22em',
              letterSpacing: '0',
              marginLeft: '0.05em',
            }}
          >
            .
          </span>
        </div>
      </TypeScale>
    </div>
  );
};

const PayoffNumber: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const pop = spring({
    frame: frame - 54,
    fps,
    config: {damping: 18, stiffness: 170, mass: 0.9},
    durationInFrames: 20,
  });

  return (
    <div
      style={{
        position: 'absolute',
        left: 96,
        top: 286,
        transform: `scale(${0.97 + pop * 0.03})`,
        transformOrigin: '18% 62%',
      }}
    >
      <div style={{display: 'flex', alignItems: 'baseline'}}>
        <span style={{...display, fontSize: 388, letterSpacing: '-0.075em'}}>340</span>
        <span
          style={{
            ...display,
            fontSize: 388,
            color: COLORS.accent,
            letterSpacing: '-0.052em',
            marginLeft: 28,
          }}
        >
          MB
        </span>
      </div>
    </div>
  );
};

export const Scene05Payoff: React.FC = () => {
  const frame = useCurrentFrame();

  if (frame >= 142) {
    return <div style={{position: 'absolute', inset: 0, background: COLORS.bg}} />;
  }

  const payoff = frame >= 62;
  const transition = frame >= 40 && frame < 66;
  const sweep = range(frame, 60, 82, 0, 1, EASE.outExpo);
  const sourceOpacity = frame < 62 ? 1 : 0;

  return (
    <div style={{position: 'absolute', inset: 0, background: COLORS.bg, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: -260,
          top: 60,
          width: 1550,
          height: 900,
          background:
            'radial-gradient(ellipse at 36% 46%, rgba(201,242,77,0.055) 0%, rgba(243,241,232,0.022) 38%, transparent 70%)',
          filter: 'blur(28px)',
          opacity: payoff ? 1 : 0.5,
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 118,
          display: 'flex',
          gap: 16,
          alignItems: 'center',
          ...micro,
          opacity: range(frame, 4, 20, 0, 0.9),
        }}
      >
        <span style={{...micro, color: COLORS.accent}}>05</span>
        <span style={{width: 26, height: 1, background: COLORS.hair}} />
        <span style={{...micro, color: COLORS.inkSoft}}>the payoff</span>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 96,
          top: 118,
          textAlign: 'right',
          opacity: range(frame, 8, 24, 0, 1),
        }}
      >
        {payoff ? (
          <>
            <div style={{...micro, color: COLORS.accent}}>additional free space required</div>
            <div style={{...micro, color: COLORS.inkSoft}}>≈ 340 MB · 60 s segment</div>
          </>
        ) : (
          <>
            <div style={{...micro, color: COLORS.inkSoft}}>source / 4K mkv</div>
            <div style={{...micro}}>60 GB · 45 Mbps</div>
          </>
        )}
      </div>

      {frame < 40 ? <SourceNumber /> : null}
      {transition ? (
        <SliceTransition
          outgoing={<SourceNumber />}
          incoming={<PayoffNumber />}
          start={40}
          duration={18}
          slices={14}
        />
      ) : null}
      {frame >= 66 ? <PayoffNumber /> : null}

      <div style={{opacity: sourceOpacity}}>
        <div
          style={{
            position: 'absolute',
            left: 96,
            top: 786,
            ...micro,
            color: COLORS.dim,
            opacity: range(frame, 12, 30, 0, 1),
          }}
        >
          one file · no second disk
        </div>
        <div
          style={{
            position: 'absolute',
            right: 96,
            top: 786,
            width: 240,
            height: 1,
            background: COLORS.hairSoft,
          }}
        />
      </div>

      {payoff ? (
        <>
          <div
            style={{
              position: 'absolute',
              left: 96,
              top: 716,
              width: 1728,
              height: 1,
              background: COLORS.hairSoft,
            }}
          >
            <div
              style={{
                width: `${sweep * 100}%`,
                height: 2,
                background: COLORS.accent,
                transform: 'translateY(-0.5px)',
              }}
            />
          </div>

          <div style={{position: 'absolute', left: 96, top: 752}}>
            <MaskedText start={64} duration={12} direction="up">
              <div
                style={{
                  fontFamily: FONTS.display,
                  fontWeight: 800,
                  fontSize: 45,
                  letterSpacing: '0.08em',
                  color: COLORS.ink,
                  textTransform: 'uppercase',
                  whiteSpace: 'nowrap',
                }}
              >
                additional free space required
                <span style={{color: COLORS.accent}}>.</span>
              </div>
            </MaskedText>
          </div>

          <div
            style={{
              position: 'absolute',
              right: 96,
              top: 394,
              width: 300,
              height: 1,
              background: COLORS.hairSoft,
              opacity: range(frame, 70, 90, 0, 1, EASE.outExpo),
            }}
          />

          <div
            style={{
              position: 'absolute',
              left: 96,
              right: 96,
              bottom: 100,
              display: 'flex',
              justifyContent: 'space-between',
              alignItems: 'flex-end',
              opacity: range(frame, 90, 104, 0, 1, EASE.outExpo),
            }}
          >
            <div
              style={{
                fontFamily: FONTS.mono,
                fontSize: 15,
                letterSpacing: '0.01em',
                lineHeight: 1.6,
                color: COLORS.muted,
                maxWidth: 1180,
              }}
            >
              Illustrative MKV → MP4 remux using{' '}
              <span style={{color: COLORS.accent}}>relocate</span> mode with a 60-second segment window.
            </div>
            <div style={{...micro, textAlign: 'right', color: COLORS.dim}}>
              peak = source + one segment
            </div>
          </div>
        </>
      ) : null}

      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: 1,
          height: `${range(frame, 62, 84, 0, 62, EASE.outExpo)}%`,
          background: COLORS.accent,
          opacity: 0.65,
        }}
      />
    </div>
  );
};
