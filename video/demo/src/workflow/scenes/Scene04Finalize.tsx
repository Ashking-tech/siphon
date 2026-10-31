import React from 'react';
import {AbsoluteFill, interpolate, useCurrentFrame} from 'remotion';
import {C, FONTS} from '../theme';
import {FINAL_CUES as Q} from '../timeline';
import {EASE, fall, ramp} from '../lib/motion';
import {DolphinWindow, TopPanel, Wallpaper} from '../components/Kde';
import {Notification} from '../components/Notification';
import {SiphonMark} from '../components/Icons';

export const Scene04Finalize: React.FC = () => {
  const frame = useCurrentFrame();

  const windowIn = ramp(frame, Q.windowIn, 10);
  const pull = interpolate(frame, [Q.pullBack, Q.pullBack + 22], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: EASE.inOut,
  });

  const notifyIn = ramp(frame, Q.toastIn, 8);
  const notifyOut = fall(frame, Q.toastOut, 10);
  const notifyOpacity = notifyIn * notifyOut;
  const notifyX = 1500 + (1 - notifyIn) * 24;

  const statementIn = ramp(frame, Q.statement, 14);
  const subtitleIn = ramp(frame, Q.subtitle, 12);

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      <Wallpaper />
      <TopPanel opacity={1 - pull} />
      <AbsoluteFill style={{background: C.white, opacity: pull}} />

      <div
        style={{
          position: 'absolute',
          inset: 0,
          zIndex: 2,
          transformOrigin: '960px 525px',
          transform: `translateY(${(1 - windowIn) * 10}px) translateY(${-70 * pull}px) scale(${1 - 0.16 * pull})`,
        }}
      >
        <DolphinWindow
          fileName="interstellar_4k.mp4"
          fileSize="60 GB"
          fileFormat="MP4"
          fileAccent={C.green}
          fileState="selected"
          showCheck={frame >= Q.success}
        />
      </div>

      <Notification
        opacity={notifyOpacity}
        x={notifyX}
        fileName="interstellar_4k.mp4"
      />

      {/* End card sits in the white space revealed by the pull-back. */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          top: 874,
          zIndex: 3,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          opacity: statementIn,
          transform: `translateY(${(1 - statementIn) * 14}px)`,
        }}
      >
        <div style={{display: 'flex', alignItems: 'center', gap: 9}}>
          <SiphonMark size={22} />
          <span
            style={{
              fontFamily: FONTS.mono,
              fontSize: 10,
              fontWeight: 700,
              letterSpacing: '0.19em',
              color: C.blue,
            }}
          >
            SIPHON
          </span>
        </div>
        <div
          style={{
            marginTop: 15,
            fontFamily: FONTS.display,
            fontSize: 64,
            lineHeight: 1,
            fontWeight: 800,
            letterSpacing: '-0.045em',
            color: C.ink,
            whiteSpace: 'nowrap',
          }}
        >
          One file in. One file out.
        </div>
        <div
          style={{
            marginTop: 20,
            fontFamily: FONTS.ui,
            fontSize: 22,
            fontWeight: 400,
            color: C.muted,
            opacity: subtitleIn,
            transform: `translateY(${(1 - subtitleIn) * 8}px)`,
          }}
        >
          Convert in place. Keep your space.
        </div>
      </div>
    </AbsoluteFill>
  );
};
