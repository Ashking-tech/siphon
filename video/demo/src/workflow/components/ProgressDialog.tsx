import React from 'react';
import {C, FONTS} from '../theme';
import {SiphonMark} from './Icons';

type ProgressDialogProps = {
  opacity: number;
  scale: number;
  progress: number;
  fileName: string;
};

/**
 * KDE kdialog progress window, recreated flat. The wrapper uses exactly this
 * message while `siphon convert` is running:
 *   "Converting <basename>"
 */
export const ProgressDialog: React.FC<ProgressDialogProps> = ({opacity, scale, progress, fileName}) => {
  if (opacity <= 0) return null;
  const pct = Math.round(progress * 100);
  return (
    <div
      style={{
        position: 'absolute',
        left: 664,
        top: 448,
        width: 592,
        height: 206,
        zIndex: 80,
        borderRadius: 9,
        overflow: 'hidden',
        border: `1px solid ${C.hairline}`,
        background: '#FFFFFF',
        boxShadow: '0 24px 56px rgba(23,23,23,0.22), 0 3px 12px rgba(23,23,23,0.1)',
        opacity,
        transform: `scale(${scale})`,
        transformOrigin: 'center center',
        fontFamily: FONTS.ui,
      }}
    >
      <div
        style={{
          height: 37,
          background: C.chrome,
          borderBottom: `1px solid ${C.hairline}`,
          display: 'flex',
          alignItems: 'center',
          padding: '0 14px',
          color: '#4F545B',
          fontSize: 13.5,
          fontWeight: 600,
        }}
      >
        Siphon
        <div style={{flex: 1}} />
        <div
          style={{
            width: 18,
            height: 18,
            borderRadius: 9,
            border: `1px solid ${C.hairline}`,
            background: '#FDF1F0',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
          }}
        >
          <svg width="9" height="9" viewBox="0 0 9 9">
            <path d="M1 1l7 7M8 1L1 8" stroke="#B63A34" strokeWidth="1.4" strokeLinecap="round" />
          </svg>
        </div>
      </div>

      <div style={{padding: '18px 22px 0', display: 'flex', gap: 16}}>
        <SiphonMark size={38} />
        <div style={{flex: 1}}>
          <div style={{fontSize: 15.5, fontWeight: 600, color: C.ink}}>Converting {fileName}</div>
          <div
            style={{
              marginTop: 21,
              height: 8,
              borderRadius: 4,
              background: '#E9EBEF',
              overflow: 'hidden',
              position: 'relative',
            }}
          >
            <div
              style={{
                position: 'absolute',
                left: 0,
                top: 0,
                bottom: 0,
                width: `${Math.max(2, progress * 100)}%`,
                borderRadius: 4,
                background: C.blue,
              }}
            />
          </div>
        </div>
      </div>

      <div
        style={{
          position: 'absolute',
          left: 22,
          right: 22,
          bottom: 15,
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <span style={{fontFamily: FONTS.mono, fontSize: 10.5, color: C.muted, letterSpacing: '.01em'}}>
          {pct}%
        </span>
        <div style={{flex: 1}} />
        <div
          style={{
            height: 30,
            padding: '0 17px',
            borderRadius: 5,
            border: `1px solid ${C.hairline}`,
            background: '#FBFBFC',
            color: '#4D525A',
            fontSize: 13,
            display: 'flex',
            alignItems: 'center',
          }}
        >
          Cancel
        </div>
      </div>
    </div>
  );
};
