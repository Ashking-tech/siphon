import React from 'react';
import {COLORS, FONTS} from '../theme';

type TechProps = {
  children: React.ReactNode;
  size?: number;
  color?: string;
  opacity?: number;
  tracking?: number;
  weight?: 400 | 700;
  style?: React.CSSProperties;
};

export const Tech: React.FC<TechProps> = ({
  children,
  size = 12,
  color = COLORS.ink,
  opacity = 1,
  tracking = 0.16,
  weight = 400,
  style,
}) => (
  <span
    style={{
      fontFamily: FONTS.mono,
      fontSize: size,
      lineHeight: 1.35,
      letterSpacing: `${tracking}em`,
      textTransform: 'uppercase',
      color,
      opacity,
      fontWeight: weight,
      whiteSpace: 'nowrap',
      WebkitFontSmoothing: 'antialiased',
      ...style,
    }}
  >
    {children}
  </span>
);

type DisplayProps = {
  children: React.ReactNode;
  size?: number;
  color?: string;
  weight?: 400 | 500 | 700 | 800 | 900;
  tracking?: number;
  style?: React.CSSProperties;
};

export const Display: React.FC<DisplayProps> = ({
  children,
  size = 56,
  color = COLORS.ink,
  weight = 800,
  tracking = -0.035,
  style,
}) => (
  <span
    style={{
      fontFamily: FONTS.display,
      fontSize: size,
      lineHeight: 0.95,
      letterSpacing: `${tracking}em`,
      color,
      fontWeight: weight,
      whiteSpace: 'nowrap',
      WebkitFontSmoothing: 'antialiased',
      ...style,
    }}
  >
    {children}
  </span>
);

export const Rule: React.FC<{
  width?: number | string;
  color?: string;
  opacity?: number;
  thickness?: number;
  style?: React.CSSProperties;
}> = ({width = '100%', color = COLORS.ink, opacity = 0.18, thickness = 1, style}) => (
  <div style={{width, height: thickness, backgroundColor: color, opacity, ...style}} />
);

export const Dot: React.FC<{color: string; size?: number; style?: React.CSSProperties}> = ({
  color,
  size = 6,
  style,
}) => <div style={{width: size, height: size, borderRadius: size, backgroundColor: color, ...style}} />;

export const SceneTag: React.FC<{
  index: string;
  title: string;
  detail?: string;
  color?: string;
  dark?: boolean;
  style?: React.CSSProperties;
}> = ({index, title, detail, color = COLORS.ink, dark = false, style}) => (
  <div style={{position: 'absolute', display: 'flex', alignItems: 'flex-start', gap: 14, ...style}}>
    <Tech size={11} color={color} opacity={dark ? 0.42 : 0.34} tracking={0.12} style={{paddingTop: 2}}>
      {index}
    </Tech>
    <div style={{width: 1, height: 34, backgroundColor: color, opacity: 0.18}} />
    <div style={{display: 'flex', flexDirection: 'column', gap: 5}}>
      <Tech size={11} color={color} opacity={0.86} tracking={0.2}>
        {title}
      </Tech>
      {detail ? (
        <Tech size={10} color={color} opacity={0.38} tracking={0.14}>
          {detail}
        </Tech>
      ) : null}
    </div>
  </div>
);
