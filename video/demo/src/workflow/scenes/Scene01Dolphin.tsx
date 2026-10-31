import React from 'react';
import {AbsoluteFill, useCurrentFrame} from 'remotion';
import {Cursor} from '../components/Cursor';
import {ContextMenu} from '../components/ContextMenu';
import {Desktop, DolphinWindow, FILE_CENTER} from '../components/Kde';
import {ProgressDialog} from '../components/ProgressDialog';
import {DOLPHIN_CUES as Q} from '../timeline';
import {cursorAt, fall, pulse, ramp} from '../lib/motion';
import {C} from '../theme';

const CURSOR_KEYS = [
  {frame: 12, x: 1660, y: 952},
  {frame: 26, x: 1230, y: 770},
  {frame: 40, x: FILE_CENTER.x, y: FILE_CENTER.y},
  {frame: 54, x: 610, y: 646},
  {frame: 80, x: 700, y: 646},
  {frame: 92, x: 806, y: 648},
  {frame: 108, x: 968, y: 648},
  {frame: 122, x: 1160, y: 820},
  {frame: 136, x: 1490, y: 920},
];

/**
 * Scene 01 — the real user path:
 * hover → right-click → Siphon submenu → Convert to MP4 (saves space).
 *
 * The ServiceMenu is one Siphon submenu with two actions. There is no invented
 * intermediate “conversion” submenu; the action labels are copied verbatim
 * from kde/servicemenus/siphon-mkv-to-mp4.desktop.
 */
export const Scene01Dolphin: React.FC = () => {
  const frame = useCurrentFrame();

  const windowIn = ramp(frame, 0, 12);
  const fileState =
    frame >= Q.rightClick ? 'selected' : frame >= Q.hoverFile ? 'hover' : 'idle';

  const menuOpacity = ramp(frame, Q.menuOpen, 5) * fall(frame, Q.menuClose, 6);
  const menuScale = 0.965 + ramp(frame, Q.menuOpen, 7) * 0.035;
  const siphonHover = frame >= Q.siphonHover && menuOpacity > 0;

  const submenuOpacity = ramp(frame, Q.submenuOpen, 5) * fall(frame, Q.menuClose, 5);
  const submenuScale = 0.97 + ramp(frame, Q.submenuOpen, 6) * 0.03;
  const actionHover = frame >= Q.actionHover && submenuOpacity > 0.2;
  const actionClick = pulse(frame, Q.actionClick, 10);

  const rightClickPulse = pulse(frame, Q.rightClick, 11);
  const clickPulse = pulse(frame, Q.actionClick, 12);

  const cursor = cursorAt(frame, CURSOR_KEYS);
  const cursorOpacity = ramp(frame, Q.cursorIn, 5) * fall(frame, 126, 12);

  const dialogOpacity = ramp(frame, Q.dialogOpen, 7) * fall(frame, 194, 8);
  const dialogScale = 0.975 + ramp(frame, Q.dialogOpen, 8) * 0.025;
  const progress = 0.04 + 0.07 * ramp(frame, Q.dialogProgressStart, 46);

  return (
    <AbsoluteFill style={{overflow: 'hidden', background: C.desktop}}>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          opacity: windowIn,
          transform: `translateY(${(1 - windowIn) * 10}px)`,
        }}
      >
        <Desktop>
          <DolphinWindow
            fileName="interstellar_4k.mkv"
            fileSize="60 GB"
            fileFormat="MKV"
            fileAccent={C.red}
            fileState={fileState}
          />

          <ContextMenu
            opacity={menuOpacity}
            scale={menuScale}
            siphonHover={siphonHover}
            submenuOpacity={submenuOpacity}
            submenuScale={submenuScale}
            actionHover={actionHover}
            actionClick={actionClick}
          />

          <ProgressDialog
            opacity={dialogOpacity}
            scale={dialogScale}
            progress={progress}
            fileName="interstellar_4k.mkv"
          />

          <Cursor
            x={cursor.x}
            y={cursor.y}
            opacity={cursorOpacity}
            pulse={Math.max(rightClickPulse, clickPulse)}
          />
        </Desktop>
      </div>

      {/* A very quiet cue that the click landed, without inventing a new UI. */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 0,
          height: 3,
          background: C.blue,
          opacity: 0.12 + 0.18 * clickPulse,
        }}
      />
    </AbsoluteFill>
  );
};
