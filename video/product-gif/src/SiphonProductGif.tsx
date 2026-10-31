import React from 'react';
import {AbsoluteFill, useCurrentFrame} from 'remotion';
import {C} from './theme';
import {T} from './timeline';
import {fall, ramp} from './motion';
import {DolphinScene, DolphinStartFrame} from './components/Dolphin';
import {InfographicScene} from './components/Infographic';
import {FinalScene} from './components/FinalScene';

/**
 * Siphon — website product animation.
 *
 * 00.0–02.8  KDE Dolphin: right-click → Siphon → Convert to MP4 (saves space)
 * 02.8–04.4  the interface resolves into the conversion infographic
 * 04.4–10.8  twelve tail-first handovers: read, process, verify, reclaim
 * 10.8–12.8  the converted file returns to the folder
 * 12.8–13.6  the exact start frame fades back in, so the loop is seamless
 */
export const SiphonProductGif: React.FC = () => {
  const frame = useCurrentFrame();

  const dolphinOpacity = fall(frame, T.dolphinOut, 18);
  const infoOpacity = ramp(frame, T.infoIn, 12) * fall(frame, T.finalIn, 18);
  const finalOpacity = ramp(frame, T.dolphinReturn, 14);
  const loopOpacity = ramp(frame, T.loopStart, T.loopEnd - T.loopStart - 2);

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      <div style={{position: 'absolute', inset: 0, opacity: dolphinOpacity}}>
        <DolphinScene frame={frame} />
      </div>

      <div style={{position: 'absolute', inset: 0, opacity: infoOpacity}}>
        <InfographicScene frame={frame} />
      </div>

      <div style={{position: 'absolute', inset: 0, opacity: finalOpacity}}>
        <FinalScene frame={frame} />
      </div>

      {loopOpacity > 0 ? (
        <div style={{position: 'absolute', inset: 0, opacity: loopOpacity, zIndex: 100}}>
          <DolphinStartFrame />
        </div>
      ) : null}
    </AbsoluteFill>
  );
};
