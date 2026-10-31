import React from 'react';
import {C, FONTS} from '../theme';

export const Kicker: React.FC<{
  code?: string;
  label: string;
  color?: string;
  style?: React.CSSProperties;
}> = ({code, label, color = C.blue, style}) => (
  <div
    style={{
      display: 'flex',
      alignItems: 'center',
      gap: 9,
      fontFamily: FONTS.mono,
      fontSize: 11,
      fontWeight: 700,
      letterSpacing: '0.14em',
      color: C.muted,
      textTransform: 'uppercase',
      ...style,
    }}
  >
    <span style={{width: 7, height: 7, borderRadius: 1.5, background: color}} />
    {code ? <span style={{color}}>{code}</span> : null}
    {code ? <span style={{color: '#B7BBC1'}}>/</span> : null}
    <span>{label}</span>
  </div>
);

export const StageTitle: React.FC<{children: React.ReactNode; style?: React.CSSProperties}> = ({
  children,
  style,
}) => (
  <div
    style={{
      fontFamily: FONTS.display,
      fontWeight: 800,
      fontSize: 40,
      lineHeight: 1.04,
      letterSpacing: '-0.025em',
      color: C.ink,
      ...style,
    }}
  >
    {children}
  </div>
);

export const Chip: React.FC<{
  children: React.ReactNode;
  tone?: 'neutral' | 'blue' | 'green' | 'red' | 'yellow' | 'grey';
  mono?: boolean;
  style?: React.CSSProperties;
}> = ({children, tone = 'neutral', mono = true, style}) => {
  const tones = {
    neutral: {bg: '#F4F5F7', fg: '#5E636B', bd: C.hairline},
    blue: {bg: C.blueWash, fg: '#2844BD', bd: '#D5DDFF'},
    green: {bg: C.greenWash, fg: '#2A7D63', bd: '#CBEADF'},
    red: {bg: C.redWash, fg: '#B7352F', bd: '#F5D2CF'},
    yellow: {bg: C.yellowWash, fg: '#8A6A10', bd: '#F1E1A8'},
    grey: {bg: '#F2F3F4', fg: '#7A7F86', bd: '#E1E3E6'},
  } as const;
  const t = tones[tone];
  return (
    <span
      style={{
        height: 27,
        padding: '0 11px',
        borderRadius: 14,
        display: 'inline-flex',
        alignItems: 'center',
        gap: 7,
        background: t.bg,
        border: `1px solid ${t.bd}`,
        color: t.fg,
        fontFamily: mono ? FONTS.mono : FONTS.ui,
        fontSize: mono ? 10 : 12,
        fontWeight: mono ? 700 : 600,
        letterSpacing: mono ? '0.055em' : '0',
        whiteSpace: 'nowrap',
        ...style,
      }}
    >
      {children}
    </span>
  );
};

export const MonoLabel: React.FC<{
  children: React.ReactNode;
  color?: string;
  style?: React.CSSProperties;
}> = ({children, color = C.muted, style}) => (
  <div
    style={{
      fontFamily: FONTS.mono,
      fontSize: 10,
      letterSpacing: '0.11em',
      textTransform: 'uppercase',
      color,
      ...style,
    }}
  >
    {children}
  </div>
);

export const Hairline: React.FC<{style?: React.CSSProperties; color?: string}> = ({style, color = C.hairline}) => (
  <div style={{height: 1, background: color, ...style}} />
);
