import {interpolate, useCurrentFrame, useVideoConfig} from 'remotion';
import {EASE} from '../lib/anim';
import {COLORS, FONTS, GRID} from '../theme';
import {sceneAt} from '../timeline';

const pad = (n: number) => String(n).padStart(2, '0');

const timecode = (frame: number, fps: number) => {
  const totalSeconds = Math.floor(frame / fps);
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  const frames = frame % fps;
  return `${pad(minutes)}:${pad(seconds)}:${pad(frames)}`;
};

const micro: React.CSSProperties = {
  fontFamily: FONTS.mono,
  fontSize: 10,
  lineHeight: 1,
  letterSpacing: '0.18em',
  textTransform: 'uppercase',
  color: COLORS.inkSoft,
};

/**
 * The film's technical frame: four corner marks, a chapter slug, a timecode
 * and a hairline progress bar. It keeps the film in a single visual system
 * without turning it into a dashboard.
 */
export const Chrome: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps, durationInFrames} = useVideoConfig();
  const scene = sceneAt(frame);
  const hudOpacity = interpolate(
    frame,
    [0, 12, 22, 585, 600, 742, 750, 918, 936, 1012, 1030],
    [0, 0, 0.62, 0.62, 0.3, 0.3, 0.58, 0.58, 0.58, 0.18, 0],
    {extrapolateLeft: 'clamp', extrapolateRight: 'clamp', easing: EASE.inOut},
  );
  const progress = frame / (durationInFrames - 1);

  const cornerStyle = (position: React.CSSProperties): React.CSSProperties => ({
    position: 'absolute',
    width: 11,
    height: 11,
    opacity: 0.55,
    ...position,
  });

  return (
    <div style={{position: 'absolute', inset: 0, pointerEvents: 'none', opacity: hudOpacity}}>
      <div style={{position: 'absolute', left: 48, top: 40, display: 'flex', alignItems: 'center', gap: 10}}>
        <span
          style={{
            width: 6,
            height: 6,
            background: COLORS.accent,
            display: 'inline-block',
            transform: 'rotate(45deg)',
          }}
        />
        <span style={{...micro, color: COLORS.ink, letterSpacing: '0.22em'}}>Siphon</span>
        <span style={{...micro, color: COLORS.muted}}>/ in-place media</span>
      </div>

      <div style={{position: 'absolute', right: 48, top: 40, ...micro}}>Linux / KDE Plasma</div>

      <div style={{position: 'absolute', left: 48, bottom: 40, display: 'flex', gap: 16, alignItems: 'center'}}>
        <span style={{...micro, color: COLORS.accent}}>{scene.code}</span>
        <span style={{width: 24, height: 1, background: COLORS.hair}} />
        <span style={{...micro}}>{scene.label}</span>
      </div>

      <div style={{position: 'absolute', right: 48, bottom: 40, ...micro}}>{timecode(frame, fps)}</div>

      <div
        style={{
          position: 'absolute',
          left: 48,
          right: 48,
          bottom: 20,
          height: 1,
          background: COLORS.hairSoft,
        }}
      >
        <div
          style={{
            height: 1,
            width: '100%',
            background: COLORS.accentDim,
            transform: `scaleX(${progress})`,
            transformOrigin: '0 50%',
          }}
        />
      </div>

      <div style={cornerStyle({left: 18, top: 18, borderLeft: `1px solid ${COLORS.hair}`, borderTop: `1px solid ${COLORS.hair}`})} />
      <div style={cornerStyle({right: 18, top: 18, borderRight: `1px solid ${COLORS.hair}`, borderTop: `1px solid ${COLORS.hair}`})} />
      <div style={cornerStyle({left: 18, bottom: 18, borderLeft: `1px solid ${COLORS.hair}`, borderBottom: `1px solid ${COLORS.hair}`})} />
      <div style={cornerStyle({right: 18, bottom: 18, borderRight: `1px solid ${COLORS.hair}`, borderBottom: `1px solid ${COLORS.hair}`})} />

      <div
        style={{
          position: 'absolute',
          left: GRID.margin,
          top: 62,
          width: GRID.contentWidth,
          height: 1,
          background: COLORS.hairSoft,
          opacity: 0.3,
        }}
      />
    </div>
  );
};
