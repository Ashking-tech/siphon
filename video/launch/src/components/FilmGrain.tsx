import type {CSSProperties} from 'react';
import {staticFile} from 'remotion';

/**
 * A full-frame grain plate. It is static by design: a tiled or translated
 * texture creates visible seams through oversized flat typography.
 */
export const FilmGrain: React.FC<{opacity?: number; blend?: CSSProperties['mixBlendMode']}> = ({
  opacity = 0.055,
  blend = 'soft-light',
}) => {
  return (
    <div
      style={{
        position: 'absolute',
        inset: 0,
        backgroundImage: `url(${staticFile('grain.png')})`,
        backgroundSize: 'cover',
        backgroundPosition: 'center',
        backgroundRepeat: 'no-repeat',
        opacity,
        mixBlendMode: blend,
        pointerEvents: 'none',
      }}
    />
  );
};

export const Vignette: React.FC = () => (
  <div
    style={{
      position: 'absolute',
      inset: 0,
      background:
        'radial-gradient(ellipse at 50% 47%, rgba(0,0,0,0) 42%, rgba(0,0,0,0.14) 74%, rgba(0,0,0,0.42) 100%)',
      pointerEvents: 'none',
    }}
  />
);
