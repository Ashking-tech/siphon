import type {CSSProperties} from 'react';
import {interpolate, useCurrentFrame} from 'remotion';
import {MaskedText} from '../components/MaskedText';
import {TrackingReveal} from '../components/TrackingReveal';
import {EASE, range} from '../lib/anim';
import {COLORS, FONTS} from '../theme';

const micro: CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 11,
  letterSpacing: '0.22em',
  textTransform: 'uppercase',
  color: COLORS.muted,
};

export const Scene03Reveal: React.FC = () => {
  const frame = useCurrentFrame();

  if (frame >= 108) {
    return <div style={{position: 'absolute', inset: 0, background: COLORS.bg}} />;
  }

  const nameScale = interpolate(frame, [82, 106], [1, 1.34], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inExpo,
  });
  const lineWidth = range(frame, 12, 42, 0, 1, EASE.outExpo);
  const glow = interpolate(frame, [0, 40, 90], [0, 0.42, 0.15], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });
  const fadeOut = interpolate(frame, [94, 106], [1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        background: COLORS.bg,
        overflow: 'hidden',
        opacity: fadeOut,
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '46%',
          width: 1300,
          height: 700,
          transform: 'translate(-50%, -50%)',
          background: `radial-gradient(ellipse at center, rgba(201,242,77,${0.04 * glow}) 0%, rgba(243,241,232,${0.028 * glow}) 30%, transparent 70%)`,
          filter: 'blur(22px)',
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 136,
          ...micro,
          opacity: range(frame, 18, 42, 0, 0.85),
        }}
      >
        <span style={{color: COLORS.accent}}>03</span> / the name
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 330,
          transform: `translateX(-50%) scale(${nameScale})`,
          transformOrigin: '50% 50%',
          width: 1810,
          textAlign: 'center',
        }}
      >
        <TrackingReveal
          text="SIPHON"
          trailing={<span style={{color: COLORS.accent}}>.</span>}
          start={5}
          duration={34}
          fromTracking={0.46}
          toTracking={0.035}
          style={{
            fontFamily: FONTS.display,
            fontWeight: 900,
            fontSize: 302,
            lineHeight: 0.84,
            letterSpacing: '0.035em',
            color: COLORS.ink,
          }}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 646,
          width: 1728 * lineWidth,
          height: 1,
          background: COLORS.hair,
          transform: 'translateX(-50%)',
        }}
      />
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 646,
          width: 132,
          height: 2,
          background: COLORS.accent,
          transform: `translateX(-50%) scaleX(${lineWidth})`,
          opacity: range(frame, 40, 54, 0, 1),
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 720,
          transform: 'translateX(-50%)',
          textAlign: 'center',
        }}
      >
        <MaskedText start={28} duration={15} direction="up">
          <div
            style={{
              fontFamily: FONTS.mono,
              fontWeight: 700,
              fontSize: 30,
              letterSpacing: '0.38em',
              color: COLORS.ink,
              whiteSpace: 'nowrap',
              paddingLeft: '0.38em',
            }}
          >
            CONVERT IN PLACE
            <span style={{color: COLORS.accent}}>.</span>
          </div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 790,
          transform: 'translateX(-50%)',
          textAlign: 'center',
          opacity: range(frame, 38, 60, 0, 0.7),
        }}
      >
        <span style={{...micro, letterSpacing: '0.2em'}}>one file · one directory · no second copy</span>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          bottom: 116,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'flex-end',
          opacity: range(frame, 46, 68, 0, 0.5),
        }}
      >
        <div style={{...micro}}>native linux utility</div>
        <div style={{...micro, color: COLORS.inkSoft}}>siphon / 0.1</div>
      </div>
    </div>
  );
};
