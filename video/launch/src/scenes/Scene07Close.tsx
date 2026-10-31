import type {CSSProperties} from 'react';
import {interpolate, useCurrentFrame} from 'remotion';
import {MaskedText} from '../components/MaskedText';
import {TrackingReveal} from '../components/TrackingReveal';
import {EASE, range} from '../lib/anim';
import {COLORS, FONTS} from '../theme';

const micro: CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 10,
  letterSpacing: '0.24em',
  textTransform: 'uppercase',
  color: COLORS.muted,
};

export const Scene07Close: React.FC = () => {
  const frame = useCurrentFrame();

  const fade = interpolate(frame, [108, 119], [1, 0], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });
  const glow = interpolate(frame, [0, 30, 100], [0, 0.5, 0.17], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });
  const rule = range(frame, 22, 54, 0, 1, EASE.outExpo);

  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        background: COLORS.bg,
        overflow: 'hidden',
        opacity: frame >= 120 ? 0 : fade,
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: '48%',
          width: 1500,
          height: 820,
          transform: 'translate(-50%, -50%)',
          background: `radial-gradient(ellipse at center, rgba(201,242,77,${0.048 * glow}) 0%, rgba(243,241,232,${0.022 * glow}) 34%, transparent 70%)`,
          filter: 'blur(30px)',
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 250,
          width: 12,
          height: 12,
          transform: 'translateX(-50%) rotate(45deg)',
          background: COLORS.accent,
          opacity: range(frame, 10, 24, 0, 1, EASE.outExpo),
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 342,
          transform: 'translateX(-50%)',
          width: 1810,
          textAlign: 'center',
        }}
      >
        <TrackingReveal
          text="SIPHON"
          trailing={<span style={{color: COLORS.accent}}>.</span>}
          start={8}
          duration={32}
          fromTracking={0.4}
          toTracking={0.032}
          style={{
            fontFamily: FONTS.display,
            fontWeight: 900,
            fontSize: 270,
            lineHeight: 0.84,
            letterSpacing: '0.032em',
            color: COLORS.ink,
          }}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 654,
          width: 632 * rule,
          height: 1,
          background: COLORS.hair,
          transform: 'translateX(-50%)',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            position: 'absolute',
            left: '50%',
            top: -1,
            width: 100,
            height: 2,
            background: COLORS.accent,
            transform: 'translateX(-50%)',
          }}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 712,
          transform: 'translateX(-50%)',
          width: 1810,
          textAlign: 'center',
        }}
      >
        <MaskedText start={30} duration={16} direction="up">
          <div
            style={{
              fontFamily: FONTS.display,
              fontWeight: 700,
              fontSize: 48,
              letterSpacing: '0.14em',
              color: COLORS.ink,
              whiteSpace: 'nowrap',
            }}
          >
            LESS SPACE<span style={{color: COLORS.accent}}>.</span> MORE CONTROL
            <span style={{color: COLORS.accent}}>.</span>
          </div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          left: '50%',
          top: 802,
          transform: 'translateX(-50%)',
          width: 1810,
          textAlign: 'center',
          opacity: range(frame, 40, 58, 0, 0.8),
        }}
      >
        <span style={{...micro, color: COLORS.inkSoft}}>
          linux-native · kde plasma · in-place conversion
        </span>
      </div>

    </div>
  );
};
