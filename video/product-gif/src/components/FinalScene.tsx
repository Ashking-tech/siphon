import React from 'react';
import {AbsoluteFill} from 'remotion';
import {C, FONTS} from '../theme';
import {T} from '../timeline';
import {ramp} from '../motion';
import {DolphinHeader, DolphinWindow} from './Dolphin';

export const FinalScene: React.FC<{frame: number}> = ({frame}) => {
  const enter = ramp(frame, T.dolphinReturn, 14);
  const statement = ramp(frame, 346, 12);
  const subtitle = ramp(frame, 352, 12);

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          opacity: enter,
          transform: `translateY(${(1 - enter) * 7}px)`,
        }}
      >
        <DolphinHeader kicker="04 / CONVERTED" right="MP4 · VERIFIED" rightColor={C.green} />
        <DolphinWindow file="MP4" verified complete />

        <div
          style={{
            position: 'absolute',
            left: 0,
            right: 0,
            top: 562,
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            opacity: statement,
          }}
        >
          <div style={{width: 34, height: 2, background: C.green, marginBottom: 14}} />
          <div
            style={{
              fontFamily: FONTS.display,
              fontSize: 25,
              lineHeight: 1.1,
              fontWeight: 500,
              letterSpacing: '-0.02em',
              color: C.ink,
            }}
          >
            One file in. One file out.
          </div>
          <div
            style={{
              marginTop: 9,
              fontFamily: FONTS.ui,
              fontSize: 11.5,
              letterSpacing: '.01em',
              color: C.inkSoft,
              opacity: subtitle,
            }}
          >
            Without a second full-size copy.
          </div>
        </div>
      </div>
    </AbsoluteFill>
  );
};
