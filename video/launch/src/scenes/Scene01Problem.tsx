import type {CSSProperties} from 'react';
import {interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {AnimatedText} from '../components/AnimatedText';
import {MaskedText} from '../components/MaskedText';
import {TextTransition} from '../components/TextTransition';
import {TypeScale} from '../components/TypeScale';
import {EASE, range} from '../lib/anim';
import {COLORS, FONTS, GRID} from '../theme';

const micro: CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 12,
  letterSpacing: '0.17em',
  textTransform: 'uppercase',
  color: COLORS.muted,
  lineHeight: 1.5,
};

const display: CSSProperties = {
  fontFamily: FONTS.display,
  fontWeight: 900,
  letterSpacing: '-0.062em',
  lineHeight: 0.82,
  color: COLORS.ink,
};

const Crowd: React.FC<{frame: number}> = ({frame}) => {
  const {fps} = useVideoConfig();
  const expand = interpolate(frame, [34, 72], [1, 1.72], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });
  const dim = interpolate(frame, [43, 55], [1, 0.16], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });
  const drift = spring({
    frame: frame - 44,
    fps,
    config: {damping: 24, stiffness: 90, mass: 1.1},
    durationInFrames: 28,
  });
  const ghost = spring({
    frame: frame - 56,
    fps,
    config: {damping: 20, stiffness: 160, mass: 0.8},
    durationInFrames: 22,
  });
  const ghostTwo = spring({
    frame: frame - 66,
    fps,
    config: {damping: 20, stiffness: 140, mass: 0.9},
    durationInFrames: 22,
  });
  const lineExit = interpolate(frame, [88, 98], [0, -88], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inExpo,
  });

  return (
    <div style={{position: 'absolute', inset: 0}}>
      <div
        style={{
          position: 'absolute',
          left: 62,
          top: 238,
          width: 1810,
          transform: `scale(${expand}) translate(${-70 * drift}px, ${-28 * drift}px)`,
          transformOrigin: '18% 42%',
          opacity: dim,
        }}
      >
        <TypeScale start={0} from={1.12} to={1} duration={16} origin="20% 70%">
          <div
            style={{
              ...display,
              fontSize: 610,
              whiteSpace: 'nowrap',
            }}
          >
            60 GB<span style={{color: COLORS.accent}}>.</span>
          </div>
        </TypeScale>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 108,
          top: 126,
          fontFamily: FONTS.mono,
          fontSize: 120,
          fontWeight: 700,
          color: COLORS.accent,
          opacity: ghost,
          transform: `translateX(${(1 - ghost) * 50}px)`,
        }}
      >
        ×2
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 302,
          transform: `translateY(${lineExit}px)`,
        }}
      >
        <MaskedText start={42} duration={14} direction="up">
          <div style={{...display, fontSize: 166, letterSpacing: '-0.055em'}}>NOT ENOUGH</div>
        </MaskedText>
        <MaskedText start={46} duration={14} direction="up">
          <div style={{...display, fontSize: 166, letterSpacing: '-0.055em'}}>
            SPACE<span style={{color: COLORS.accent}}>.</span>
          </div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 126,
          top: 574,
          ...display,
          fontSize: 186,
          color: 'transparent',
          WebkitTextStroke: `1.4px ${COLORS.accent}`,
          opacity: 0.58 * ghost,
          transform: `translate(${(1 - ghost) * 120}px, ${(1 - ghost) * 26}px) rotate(-4deg)`,
        }}
      >
        60 GB.
      </div>

      <div
        style={{
          position: 'absolute',
          left: 662,
          top: 786,
          ...display,
          fontSize: 128,
          color: 'transparent',
          WebkitTextStroke: `1px ${COLORS.inkSoft}`,
          opacity: 0.22 * ghostTwo,
          transform: `translate(${(1 - ghostTwo) * -160}px, ${(1 - ghostTwo) * 20}px) rotate(2.5deg)`,
        }}
      >
        60 GB.
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          bottom: 106,
          display: 'flex',
          alignItems: 'baseline',
          gap: 22,
          opacity: ghost,
        }}
      >
        <span style={{...micro, color: COLORS.accent}}>second copy</span>
        <span style={{...micro}}>another 60 GB required</span>
        <span style={{...micro, color: COLORS.dim}}>for a container change</span>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 96,
          top: 102,
          textAlign: 'right',
          opacity: 0.7,
          transform: `translateY(${lineExit}px)`,
        }}
      >
        <div style={{...micro, color: COLORS.inkSoft}}>source / mkv</div>
        <div style={{...micro}}>4k · 45 Mbps · 60 GB</div>
      </div>
    </div>
  );
};

const Question: React.FC = () => {
  const frame = useCurrentFrame();

  return (
    <div style={{position: 'absolute', inset: 0}}>
      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 210,
          ...display,
          fontSize: 168,
          lineHeight: 0.86,
          letterSpacing: '-0.058em',
        }}
      >
        <MaskedText start={0} duration={7} direction="up">
          <div>JUST TO</div>
        </MaskedText>
        <MaskedText start={2} duration={7} direction="up">
          <div>
            CONVERT <span style={{color: COLORS.accent}}>ONE</span>
          </div>
        </MaskedText>
        <MaskedText start={4} duration={7} direction="up">
          <div>
            FILE<span style={{color: COLORS.accent}}>?</span>
          </div>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 100,
          top: 688,
          ...display,
          fontSize: 84,
          color: COLORS.inkSoft,
          letterSpacing: '-0.03em',
          transform: `translateX(${range(frame, 12, 34, 60, 0)}px)`,
          opacity: range(frame, 12, 26, 0, 1),
        }}
      >
        ANOTHER 60 GB.
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          bottom: 102,
          maxWidth: 1040,
          display: 'flex',
          gap: 24,
          alignItems: 'center',
        }}
      >
        <span style={{width: 46, height: 1, background: COLORS.accent, display: 'inline-block'}} />
        <AnimatedText
          text="A second full-size copy. Just to change the container."
          start={8}
          mode="word"
          stagger={1}
          animation="slide"
          style={{
            ...micro,
            fontSize: 13,
            color: COLORS.inkSoft,
            letterSpacing: '0.13em',
            textTransform: 'uppercase',
          }}
        />
      </div>
    </div>
  );
};

export const Scene01Problem: React.FC = () => {
  const frame = useCurrentFrame();

  if (frame >= 135) {
    return <div style={{position: 'absolute', inset: 0, background: COLORS.bg}} />;
  }

  return (
    <div style={{position: 'absolute', inset: 0, background: COLORS.bg, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: GRID.margin,
          top: 132,
          ...micro,
          opacity: range(frame, 8, 24, 0, 0.66),
        }}
      >
        <span style={{color: COLORS.accent}}>01</span> / one file. no room for two.
      </div>

      {frame < 88 ? (
        <Crowd frame={frame} />
      ) : (
        <TextTransition
          outgoing={<Crowd frame={88} />}
          incoming={<Question />}
          start={88}
          duration={10}
          direction="up"
        />
      )}
    </div>
  );
};
