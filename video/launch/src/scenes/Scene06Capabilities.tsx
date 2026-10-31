import type {CSSProperties, ReactNode} from 'react';
import {spring, useCurrentFrame, useVideoConfig} from 'remotion';
import {MaskedText} from '../components/MaskedText';
import {TextTransition} from '../components/TextTransition';
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
  lineHeight: 0.82,
  color: COLORS.ink,
  letterSpacing: '-0.06em',
};

type SlideProps = {
  from: string;
  to: string;
  note: string;
  index: number;
  align: 'left' | 'right';
  top: number;
  size: number;
  enterStart: number;
};

const Arrow: React.FC<{start: number; size: number}> = ({start, size}) => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const p = spring({
    frame: frame - start - 5,
    fps,
    config: {damping: 16, stiffness: 200, mass: 0.7},
    durationInFrames: 16,
  });

  return (
    <span
      style={{
        display: 'inline-block',
        fontFamily: FONTS.display,
        fontWeight: 500,
        fontSize: size * 0.62,
        lineHeight: 1,
        color: COLORS.accent,
        margin: `0 ${size * 0.12}px`,
        transform: `translateX(${(1 - p) * -22}px) scaleX(${0.5 + p * 0.5})`,
        opacity: p,
        transformOrigin: '0 50%',
      }}
    >
      →
    </span>
  );
};

const FormatSlide: React.FC<SlideProps> = ({
  from,
  to,
  note,
  index,
  align,
  top,
  size,
  enterStart,
}) => {
  const frame = useCurrentFrame();
  const justify = align === 'right' ? 'flex-end' : 'flex-start';
  const textAlign = align;

  return (
    <div style={{position: 'absolute', inset: 0}}>
      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          top,
          display: 'flex',
          alignItems: 'baseline',
          justifyContent: justify,
          whiteSpace: 'nowrap',
        }}
      >
        <MaskedText start={enterStart} duration={10} direction={align === 'right' ? 'right' : 'left'}>
          <span style={{...display, fontSize: size}}>{from}</span>
        </MaskedText>
        <Arrow start={enterStart} size={size} />
        <MaskedText start={enterStart + 2} duration={10} direction={align === 'right' ? 'left' : 'right'}>
          <span style={{...display, fontSize: size}}>{to}</span>
        </MaskedText>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          top: top + size * 1.08,
          display: 'flex',
          justifyContent: justify,
          alignItems: 'center',
          gap: 18,
          textAlign,
        }}
      >
        <span
          style={{
            display: 'inline-block',
            height: 1,
            width: 72,
            background: COLORS.accent,
            transformOrigin: align === 'right' ? '100% 50%' : '0 50%',
            transform: `scaleX(${range(frame, enterStart + 4, enterStart + 22, 0, 1, EASE.outExpo)})`,
          }}
        />
        <span style={{...micro, color: COLORS.inkSoft}}>{note}</span>
        <span style={{...micro, color: COLORS.dim}}>
          format 0{index} / 04
        </span>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 96,
          bottom: 126,
          textAlign: 'right',
          ...micro,
          color: COLORS.dim,
          opacity: range(frame, enterStart + 6, enterStart + 24, 0, 1),
        }}
      >
        native pair / explicit target
      </div>
    </div>
  );
};

const PhilosophyFrame: React.FC<{
  start: number;
  note: (frame: number) => ReactNode;
  children: ReactNode;
  align: 'left' | 'right' | 'center';
  top: number;
}> = ({start, note, children, align, top}) => {
  const frame = useCurrentFrame();
  const textAlign = align;
  const justify = align === 'right' ? 'flex-end' : align === 'center' ? 'center' : 'flex-start';

  return (
    <div style={{position: 'absolute', inset: 0}}>
      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          top,
          display: 'flex',
          justifyContent: justify,
          textAlign,
        }}
      >
        {children}
      </div>

      <div
        style={{
          position: 'absolute',
          left: 96,
          right: 96,
          top: top + 300,
          display: 'flex',
          justifyContent: justify,
          textAlign,
          opacity: range(frame, start + 3, start + 12, 0, 1, EASE.outExpo),
        }}
      >
        {note(frame)}
      </div>

      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          width: `${range(frame, start, start + 20, 0, 100, EASE.outExpo)}%`,
          height: 2,
          background: COLORS.accent,
          opacity: 0.55,
        }}
      />
    </div>
  );
};

/**
 * Principles are cut in at full legibility with a short scale settle. At
 * roughly two-thirds of a second each, a masked entrance would spend more
 * time hiding the line than showing it.
 */
const Slam: React.FC<{
  start: number;
  align: 'left' | 'right' | 'center';
  children: ReactNode;
}> = ({start, align, children}) => {
  const frame = useCurrentFrame();
  const scale = range(frame, start, start + 7, 1.045, 1, EASE.outExpo);
  const opacity = range(frame, start, start + 3, 0.55, 1, EASE.outQuint);
  const origin = align === 'right' ? '100% 60%' : align === 'center' ? '50% 60%' : '0% 60%';

  return (
    <div style={{transform: `scale(${scale})`, transformOrigin: origin, opacity}}>
      {children}
    </div>
  );
};

export const Scene06Capabilities: React.FC = () => {
  const frame = useCurrentFrame();

  const formatSlide1 = (
    <FormatSlide
      from="MKV"
      to="MP4"
      note="video / remux · no transcode"
      index={1}
      align="left"
      top={352}
      size={172}
      enterStart={5}
    />
  );
  const formatSlide2 = (
    <FormatSlide
      from="WAV"
      to="MP3"
      note="audio / lossy target"
      index={2}
      align="right"
      top={430}
      size={158}
      enterStart={27}
    />
  );
  const formatSlide3 = (
    <FormatSlide
      from="PNG"
      to="WEBP"
      note="image / lossless"
      index={3}
      align="left"
      top={306}
      size={160}
      enterStart={49}
    />
  );
  const formatSlide4 = (
    <FormatSlide
      from="JPG"
      to="PNG"
      note="image / lossless output"
      index={4}
      align="right"
      top={446}
      size={174}
      enterStart={71}
    />
  );

  const showFormats = frame >= 5 && frame < 93;

  return (
    <div style={{position: 'absolute', inset: 0, background: COLORS.bg, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          left: 96,
          top: 118,
          display: 'flex',
          gap: 16,
          alignItems: 'center',
          ...micro,
          opacity: range(frame, 3, 18, 0, 0.9),
        }}
      >
        <span style={{...micro, color: COLORS.accent}}>06</span>
        <span style={{width: 26, height: 1, background: COLORS.hair}} />
        <span style={{...micro, color: COLORS.inkSoft}}>
          {frame < 93 ? 'what it converts' : 'the contract'}
        </span>
      </div>

      <div
        style={{
          position: 'absolute',
          right: 96,
          top: 118,
          ...micro,
          opacity: range(frame, 3, 18, 0, 0.85),
        }}
      >
        {frame < 93 ? 'format matrix / explicit' : 'principles / enforced'}
      </div>

      {showFormats ? (
        <>
          {frame >= 5 && frame < 27 ? formatSlide1 : null}
          {frame >= 27 && frame < 35 ? (
            <TextTransition
              outgoing={formatSlide1}
              incoming={formatSlide2}
              start={27}
              duration={8}
              direction="left"
            />
          ) : null}
          {frame >= 35 && frame < 49 ? formatSlide2 : null}
          {frame >= 49 && frame < 57 ? (
            <TextTransition
              outgoing={formatSlide2}
              incoming={formatSlide3}
              start={49}
              duration={8}
              direction="up"
            />
          ) : null}
          {frame >= 57 && frame < 71 ? formatSlide3 : null}
          {frame >= 71 && frame < 79 ? (
            <TextTransition
              outgoing={formatSlide3}
              incoming={formatSlide4}
              start={71}
              duration={8}
              direction="right"
            />
          ) : null}
          {frame >= 79 && frame < 93 ? formatSlide4 : null}
        </>
      ) : null}

      {frame >= 93 && frame < 113 ? (
        <PhilosophyFrame
          start={93}
          align="left"
          top={344}
          note={() => (
            <span style={{...micro, color: COLORS.inkSoft}}>
              no sockets · no uploads · no cloud
            </span>
          )}
        >
          <Slam start={93} align="left">
            <div style={{...display, fontSize: 166, letterSpacing: '-0.025em', whiteSpace: 'nowrap'}}>
              NO NETWORK<span style={{color: COLORS.accent}}>.</span>
            </div>
          </Slam>
        </PhilosophyFrame>
      ) : null}

      {frame >= 113 && frame < 133 ? (
        <PhilosophyFrame
          start={113}
          align="right"
          top={388}
          note={() => (
            <span style={{...micro, color: COLORS.inkSoft}}>no usage reporting · no analytics</span>
          )}
        >
          <Slam start={113} align="right">
            <div style={{...display, fontSize: 150, letterSpacing: '-0.025em', whiteSpace: 'nowrap'}}>
              NO TELEMETRY<span style={{color: COLORS.accent}}>.</span>
            </div>
          </Slam>
        </PhilosophyFrame>
      ) : null}

      {frame >= 133 && frame < 153 ? (
        <PhilosophyFrame
          start={133}
          align="center"
          top={356}
          note={() => (
            <span style={{...micro, color: COLORS.inkSoft}}>
              state stays in the source directory
            </span>
          )}
        >
          <Slam start={133} align="center">
            <div style={{...display, fontSize: 158, letterSpacing: '-0.025em', whiteSpace: 'nowrap'}}>
              NO HIDDEN <span style={{color: COLORS.accent}}>STATE.</span>
            </div>
          </Slam>
        </PhilosophyFrame>
      ) : null}

      {frame >= 153 ? (
        <PhilosophyFrame
          start={153}
          align="left"
          top={290}
          note={() => (
            <span style={{...micro, color: COLORS.inkSoft}}>
              lossy paths require an explicit choice
            </span>
          )}
        >
          <Slam start={153} align="left">
            <div style={{...display, fontSize: 174, letterSpacing: '-0.025em', whiteSpace: 'nowrap'}}>NO SILENT</div>
            <div style={{...display, fontSize: 174, letterSpacing: '-0.025em', whiteSpace: 'nowrap'}}>
              QUALITY LOSS<span style={{color: COLORS.accent}}>.</span>
            </div>
          </Slam>
        </PhilosophyFrame>
      ) : null}

      {frame >= 5 ? (
        <div
          style={{
            position: 'absolute',
            left: 96,
            bottom: 126,
            display: 'flex',
            alignItems: 'center',
            gap: 12,
            opacity: range(frame, 6, 22, 0, 0.7),
          }}
        >
          <span style={{width: 7, height: 7, border: `1px solid ${COLORS.accent}`, transform: 'rotate(45deg)'}} />
          <span style={{...micro, color: COLORS.dim}}>linux-native · kde plasma integration</span>
        </div>
      ) : null}

      <TypeScale start={0} from={1.01} to={1} duration={60} style={{position: 'absolute', right: 96, bottom: 110}}>
        <span style={{...micro, color: COLORS.dim}}>siphon / capabilities</span>
      </TypeScale>
    </div>
  );
};
