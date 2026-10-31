import React from 'react';
import {C, FONTS} from '../theme';
import {SiphonMark} from './Icons';

type NotificationProps = {
  opacity: number;
  x: number;
  fileName: string;
};

/** KDE passive popup in the spirit of `kdialog --passivepopup "Converted 1 file" 5`. */
export const Notification: React.FC<NotificationProps> = ({opacity, x, fileName}) => {
  if (opacity <= 0) return null;
  return (
    <div
      style={{
        position: 'absolute',
        left: x,
        top: 48,
        width: 356,
        height: 94,
        zIndex: 60,
        borderRadius: 9,
        border: `1px solid ${C.hairline}`,
        background: '#FFFFFF',
        boxShadow: '0 16px 38px rgba(23,23,23,0.16), 0 2px 8px rgba(23,23,23,0.06)',
        opacity,
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        padding: '0 17px',
        boxSizing: 'border-box',
        fontFamily: FONTS.ui,
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 3,
          background: C.green,
        }}
      />
      <div
        style={{
          width: 40,
          height: 40,
          borderRadius: 10,
          background: C.blueWash,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <SiphonMark size={27} />
      </div>
      <div style={{minWidth: 0}}>
        <div style={{fontSize: 14.5, fontWeight: 600, color: C.ink}}>Converted 1 file</div>
        <div
          style={{
            marginTop: 3,
            fontSize: 12.5,
            color: C.muted,
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            maxWidth: 240,
          }}
        >
          {fileName}
        </div>
        <div style={{marginTop: 4, display: 'flex', alignItems: 'center', gap: 6}}>
          <div style={{width: 6, height: 6, borderRadius: 3, background: C.green}} />
          <span style={{fontFamily: FONTS.mono, fontSize: 9.5, letterSpacing: '.05em', color: C.green}}>
            VERIFIED OUTPUT
          </span>
        </div>
      </div>
    </div>
  );
};
