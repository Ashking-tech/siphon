import type {CSSProperties} from 'react';
import {interpolate, useCurrentFrame} from 'remotion';
import {MaskedText} from '../components/MaskedText';
import {SplitText} from '../components/SplitText';
import {WordFragment} from '../components/WordFragment';
import {EASE, range} from '../lib/anim';
import {COLORS, FONTS} from '../theme';

const display: CSSProperties = {
  fontFamily: FONTS.display,
  fontWeight: 900,
  lineHeight: 0.84,
  color: COLORS.ink,
};

const micro: CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 11,
  letterSpacing: '0.2em',
  textTransform: 'uppercase',
  color: COLORS.muted,
};

export const Scene02Question: React.FC = () => {
  const frame = useCurrentFrame();

  if (frame >= 108) {
    return <div style={{position: 'absolute', inset: 0, background: COLORS.bg}} />;
  }

  const apart = interpolate(frame, [54, 86], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });
  const anotherScale = interpolate(frame, [36, 62], [1, 1.055], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.outExpo,
  });
  const anotherTracking = interpolate(frame, [20, 44], [0.015, 0.055], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.outExpo,
  });
  const lineExit = interpolate(frame, [94, 108], [0, -120], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inExpo,
  });

  return (
    <div style={{position: 'absolute', inset: 0, background: COLORS.bg, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 136,
          ...micro,
          opacity: range(frame, 4, 20, 0, 1),
        }}
      >
        <span style={{color: COLORS.accent}}>02</span> / the question
      </div>

      <div
        style={{
          position: 'absolute',
          right: 96,
          top: 136,
          ...micro,
          textAlign: 'right',
          opacity: range(frame, 16, 32, 0, 0.8),
        }}
      >
        peak space / 2× source
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 212 - apart * 42,
          transform: `translateY(${lineExit}px)`,
        }}
      >
        <MaskedText start={4} duration={13} direction="up">
          <div style={{...display, fontSize: 126, letterSpacing: '-0.055em', whiteSpace: 'nowrap'}}>WHY DOES</div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 342 - apart * 34,
          transform: `translateX(${apart * 64}px) translateY(${lineExit}px)`,
        }}
      >
        <MaskedText start={9} duration={13} direction="up">
          <div
            style={{
              ...display,
              fontSize: 126,
              letterSpacing: '-0.055em',
              wordSpacing: '0.06em',
              whiteSpace: 'nowrap',
            }}
          >
            CONVERSION NEED
          </div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 42,
          top: 470 + apart * 60,
          transform: `scale(${anotherScale}) translateY(${lineExit}px)`,
          transformOrigin: '20% 65%',
          letterSpacing: `${anotherTracking}em`,
        }}
      >
        <WordFragment
          text="ANOTHER"
          start={14}
          duration={21}
          slices={11}
          mode="assemble"
          style={{
            ...display,
            fontSize: 350,
            fontWeight: 900,
            letterSpacing: 'inherit',
            color: COLORS.accent,
          }}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          right: 112,
          top: 790 + apart * 50,
          transform: `translateY(${lineExit}px)`,
        }}
      >
        <SplitText
          text="COPY?"
          start={24}
          stagger={1.6}
          duration={20}
          from="right"
          accentIndices={[4]}
          style={{...display, fontSize: 190, letterSpacing: '-0.06em'}}
        />
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 690,
          width: 380,
          height: 1,
          background: COLORS.hair,
          transformOrigin: '0 50%',
          transform: `scaleX(${range(frame, 30, 54, 0, 1, EASE.outExpo)}) translateY(${lineExit}px)`,
        }}
      />

      <div
        style={{
          position: 'absolute',
          left: 96,
          bottom: 110,
          opacity: apart * 0.8,
          transform: `translateY(${lineExit}px)`,
          display: 'flex',
          gap: 18,
          alignItems: 'center',
        }}
      >
        <span style={{...micro, color: COLORS.accent}}>the duplication tax</span>
        <span style={{width: 38, height: 1, background: COLORS.dim}} />
        <span style={{...micro, color: COLORS.inkSoft}}>one file in. one full-size copy out.</span>
      </div>
    </div>
  );
};
