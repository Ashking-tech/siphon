import React from 'react';
import {AbsoluteFill} from 'remotion';
import {C, FONTS} from '../theme';
import {T} from '../timeline';
import {cursorAt, fall, pulse, ramp} from '../motion';
import {
  ArrowLeft,
  ArrowRight,
  CheckIcon,
  ChevronRight,
  CopyIcon,
  CutIcon,
  DownloadIcon,
  FileGlyph,
  FilmIcon,
  GridIcon,
  HomeIcon,
  KdeLogo,
  MenuIcon,
  NewTabIcon,
  OpenWithIcon,
  PropertiesIcon,
  RenameIcon,
  SearchIcon,
  StarIcon,
  TrashIcon,
  VideoIcon,
} from './Icons';

export const WIN = {
  x: 96,
  y: 68,
  width: 1088,
  height: 470,
  titlebar: 30,
  toolbar: 38,
  statusbar: 24,
  sidebar: 180,
} as const;

export const CONTENT = {
  x: WIN.x + WIN.sidebar,
  y: WIN.y + WIN.titlebar + WIN.toolbar,
  width: WIN.width - WIN.sidebar,
  height: WIN.height - WIN.titlebar - WIN.toolbar - WIN.statusbar - 2,
} as const;

export const FILE_TILE = {
  x: CONTENT.x + 54,
  y: CONTENT.y + 56,
  width: 124,
  height: 126,
} as const;

export const FILE_CENTER = {
  x: FILE_TILE.x + FILE_TILE.width / 2,
  y: FILE_TILE.y + 60,
} as const;

export const MENU = {
  x: 392,
  y: 222,
  width: 274,
  itemHeight: 28,
  sep: 7,
  pad: 5,
} as const;

type Row =
  | {kind: 'item'; id: string; label: string; icon: React.ReactNode; shortcut?: string; submenu?: boolean}
  | {kind: 'sep'; id: string};

const icon = (node: React.ReactNode) => (
  <span style={{width: 16, height: 16, display: 'flex', alignItems: 'center', justifyContent: 'center'}}>{node}</span>
);

const ROWS: Row[] = [
  {kind: 'item', id: 'openwith', label: 'Open With', icon: icon(<OpenWithIcon size={14} />), submenu: true},
  {kind: 'item', id: 'newtab', label: 'Open in New Tab', icon: icon(<NewTabIcon size={14} />)},
  {kind: 'sep', id: 'sep1'},
  {kind: 'item', id: 'cut', label: 'Cut', icon: icon(<CutIcon size={14} />), shortcut: 'Ctrl+X'},
  {kind: 'item', id: 'copy', label: 'Copy', icon: icon(<CopyIcon size={14} />), shortcut: 'Ctrl+C'},
  {kind: 'item', id: 'rename', label: 'Rename…', icon: icon(<RenameIcon size={14} />), shortcut: 'F2'},
  {kind: 'sep', id: 'sep2'},
  {kind: 'item', id: 'trash', label: 'Move to Trash', icon: icon(<TrashIcon size={14} />), shortcut: 'Del'},
  {kind: 'sep', id: 'sep3'},
  {kind: 'item', id: 'siphon', label: 'Siphon', icon: icon(<FilmIcon size={14} />), submenu: true},
  {kind: 'item', id: 'properties', label: 'Properties', icon: icon(<PropertiesIcon size={14} />)},
];

const rowTop = (id: string) => {
  let y = MENU.y + MENU.pad;
  for (const row of ROWS) {
    if (row.kind === 'item' && row.id === id) return y;
    y += row.kind === 'sep' ? MENU.sep : MENU.itemHeight;
  }
  return y;
};

const SIPHON_TOP = rowTop('siphon');
const SIPHON_CENTER = SIPHON_TOP + MENU.itemHeight / 2;
const ACTION_TOP = SIPHON_TOP - MENU.pad;
const ACTION_CENTER = ACTION_TOP + MENU.pad + 16;

export const CURSOR_START = {x: 1158, y: 514} as const;
export const CURSOR_KEYS = [
  {frame: 0, ...CURSOR_START},
  {frame: T.cursorToFile, ...CURSOR_START},
  {frame: 36, x: FILE_CENTER.x + 2, y: FILE_CENTER.y + 2},
  {frame: 39, x: FILE_CENTER.x + 2, y: FILE_CENTER.y + 2},
  {frame: 56, x: 522, y: SIPHON_CENTER},
  {frame: 60, x: 522, y: SIPHON_CENTER},
  {frame: 76, x: 800, y: ACTION_CENTER + 2},
  {frame: 80, x: 800, y: ACTION_CENTER + 2},
  {frame: 96, x: 1010, y: 500},
].map((key) => ({...key}));

const menuIconColor = (hovered: boolean) => (hovered ? '#FFFFFF' : '#6F747B');

const MenuRow: React.FC<{
  label: string;
  iconNode: React.ReactNode;
  shortcut?: string;
  submenu?: boolean;
  hovered: boolean;
  pressed?: number;
  height: number;
}> = ({label, iconNode, shortcut, submenu, hovered, pressed = 0, height}) => (
  <div
    style={{
      height,
      display: 'flex',
      alignItems: 'center',
      gap: 8,
      margin: '0 4px',
      padding: '0 9px',
      borderRadius: 3,
      background: hovered ? C.blue : 'transparent',
      boxShadow: pressed > 0 ? `inset 0 0 0 1.5px rgba(255,255,255,${0.5 * pressed})` : undefined,
      color: hovered ? '#FFFFFF' : C.kdeText,
      boxSizing: 'border-box',
      fontFamily: FONTS.ui,
      fontSize: 12,
    }}
  >
    <span style={{color: menuIconColor(hovered)}}>{iconNode}</span>
    <span style={{whiteSpace: 'nowrap'}}>{label}</span>
    <div style={{flex: 1}} />
    {shortcut ? (
      <span
        style={{
          fontFamily: FONTS.mono,
          fontSize: 8.5,
          letterSpacing: '.02em',
          color: hovered ? 'rgba(255,255,255,.7)' : '#A0A4AA',
        }}
      >
        {shortcut}
      </span>
    ) : null}
    {submenu ? <ChevronRight size={12} color={hovered ? '#FFFFFF' : '#8A8F96'} /> : null}
  </div>
);

export const ContextMenu: React.FC<{
  opacity: number;
  scale: number;
  siphonHover: boolean;
  submenuOpacity: number;
  submenuScale: number;
  actionHover: boolean;
  actionClick: number;
}> = ({opacity, scale, siphonHover, submenuOpacity, submenuScale, actionHover, actionClick}) => {
  if (opacity <= 0) return null;
  return (
    <>
      <div
        style={{
          position: 'absolute',
          left: MENU.x,
          top: MENU.y,
          width: MENU.width,
          padding: `${MENU.pad}px 0`,
          background: '#FFFFFF',
          border: '1px solid #D6D9DE',
          borderRadius: 6,
          boxShadow: '0 10px 28px rgba(23,23,23,0.15), 0 2px 7px rgba(23,23,23,0.07)',
          opacity,
          transform: `scale(${scale})`,
          transformOrigin: 'top left',
          boxSizing: 'border-box',
          zIndex: 70,
        }}
      >
        {ROWS.map((row) =>
          row.kind === 'sep' ? (
            <div key={row.id} style={{height: 1, margin: '3px 11px', background: C.kdeLineSoft}} />
          ) : (
            <MenuRow
              key={row.id}
              label={row.label}
              iconNode={row.icon}
              shortcut={row.shortcut}
              submenu={row.submenu}
              hovered={row.id === 'siphon' && siphonHover}
              height={MENU.itemHeight}
            />
          ),
        )}
      </div>

      {submenuOpacity > 0 ? (
        <div
          style={{
            position: 'absolute',
            left: MENU.x + MENU.width + 2,
            top: ACTION_TOP,
            width: 312,
            padding: `${MENU.pad}px 0`,
            background: '#FFFFFF',
            border: '1px solid #D6D9DE',
            borderRadius: 6,
            boxShadow: '0 10px 28px rgba(23,23,23,0.15), 0 2px 7px rgba(23,23,23,0.07)',
            opacity: submenuOpacity,
            transform: `scale(${submenuScale})`,
            transformOrigin: 'top left',
            boxSizing: 'border-box',
            zIndex: 71,
          }}
        >
          <MenuRow
            label="Convert to MP4 (saves space)"
            iconNode={<VideoIcon size={14} />}
            hovered={actionHover}
            pressed={actionClick}
            height={32}
          />
          <MenuRow
            label="Convert to classic MP4 (needs 2× space)"
            iconNode={<VideoIcon size={14} />}
            hovered={false}
            height={32}
          />
        </div>
      ) : null}
    </>
  );
};

export const Cursor: React.FC<{x: number; y: number; opacity?: number; click?: number}> = ({
  x,
  y,
  opacity = 1,
  click = 0,
}) => {
  if (opacity <= 0) return null;
  const ringScale = 0.45 + click * 1.55;
  return (
    <AbsoluteFill style={{pointerEvents: 'none', zIndex: 90}}>
      {click > 0 ? (
        <div
          style={{
            position: 'absolute',
            left: x - 25,
            top: y - 25,
            width: 50,
            height: 50,
            borderRadius: 25,
            border: `2px solid ${C.blue}`,
            transform: `scale(${ringScale})`,
            opacity: 0.5 * click,
          }}
        />
      ) : null}
      <svg
        width={28}
        height={37}
        viewBox="0 0 31 41"
        style={{position: 'absolute', left: x - 3, top: y - 2, opacity, filter: 'drop-shadow(0 1px 1px rgba(23,23,23,0.2))'}}
      >
        <path d="M3 3v27.6l6.9-6.4 5.3 12.6 4.2-1.8-5.2-12.4 9.4-.6z" fill={C.ink} stroke="#FFFFFF" strokeWidth="2" strokeLinejoin="round" />
      </svg>
    </AbsoluteFill>
  );
};

const TitleButton: React.FC<{kind: 'min' | 'max' | 'close'}> = ({kind}) => (
  <div
    style={{
      width: 17,
      height: 17,
      borderRadius: 9,
      border: `1px solid ${C.kdeLine}`,
      background: kind === 'close' ? '#FDF1F0' : '#FFFFFF',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    }}
  >
    {kind === 'min' ? <div style={{width: 6, height: 1.3, background: '#747980'}} /> : null}
    {kind === 'max' ? <div style={{width: 6, height: 6, border: '1.3px solid #747980', borderRadius: 1}} /> : null}
    {kind === 'close' ? (
      <svg width="8" height="8" viewBox="0 0 8 8">
        <path d="M1 1l6 6M7 1L1 7" stroke="#B63A34" strokeWidth="1.3" strokeLinecap="round" />
      </svg>
    ) : null}
  </div>
);

const SidebarItem: React.FC<{label: string; iconNode: React.ReactNode; selected?: boolean}> = ({
  label,
  iconNode,
  selected = false,
}) => (
  <div
    style={{
      position: 'relative',
      height: 30,
      margin: '0 9px',
      padding: '0 9px',
      borderRadius: 4,
      display: 'flex',
      alignItems: 'center',
      gap: 9,
      background: selected ? C.kdeSelection : 'transparent',
      color: selected ? '#2645C6' : '#44484E',
      fontFamily: FONTS.ui,
      fontSize: 11.5,
      fontWeight: selected ? 600 : 400,
    }}
  >
    {selected ? (
      <div style={{position: 'absolute', left: 0, top: 6, bottom: 6, width: 2.5, borderRadius: 2, background: C.blue}} />
    ) : null}
    <span style={{color: selected ? C.blue : '#7C8188', display: 'flex'}}>{iconNode}</span>
    <span>{label}</span>
  </div>
);

export type FileState = 'idle' | 'hover' | 'selected';

export const DolphinWindow: React.FC<{
  file: 'MKV' | 'MP4';
  fileState?: FileState;
  verified?: boolean;
  complete?: boolean;
  children?: React.ReactNode;
}> = ({file, fileState = 'idle', verified = false, complete = false, children}) => {
  const isMkv = file === 'MKV';
  const name = isMkv ? 'interstellar_4k.mkv' : 'interstellar_4k.mp4';
  const size = isMkv ? '60 GB' : '60.0 GB';
  const tileBackground = fileState === 'selected' ? C.kdeSelection : fileState === 'hover' ? C.kdeHover : 'transparent';

  return (
    <div
      style={{
        position: 'absolute',
        left: WIN.x,
        top: WIN.y,
        width: WIN.width,
        height: WIN.height,
        background: C.kdeWindow,
        border: '1px solid #D9DDE3',
        borderRadius: 8,
        boxShadow: '0 16px 36px rgba(23,23,23,0.10), 0 2px 6px rgba(23,23,23,0.05)',
        overflow: 'hidden',
        boxSizing: 'border-box',
        zIndex: 20,
      }}
    >
      <div
        style={{
          height: WIN.titlebar,
          background: C.kdeChrome,
          borderBottom: `1px solid ${C.kdeLineSoft}`,
          display: 'flex',
          alignItems: 'center',
          padding: '0 10px',
          gap: 7,
          boxSizing: 'border-box',
        }}
      >
        <KdeLogo size={15} />
        <span style={{fontFamily: FONTS.ui, fontSize: 11, fontWeight: 500, color: C.kdeText}}>Dolphin</span>
        <div style={{flex: 1}} />
        <TitleButton kind="min" />
        <TitleButton kind="max" />
        <TitleButton kind="close" />
      </div>

      <div
        style={{
          height: WIN.toolbar,
          background: C.kdePanel,
          borderBottom: `1px solid ${C.kdeLineSoft}`,
          display: 'flex',
          alignItems: 'center',
          padding: '0 10px',
          gap: 8,
          boxSizing: 'border-box',
        }}
      >
        <ArrowLeft size={15} />
        <ArrowRight size={15} color="#B5B9BF" />
        <div
          style={{
            flex: 1,
            height: 27,
            background: '#FFFFFF',
            border: `1px solid ${C.kdeLine}`,
            borderRadius: 4,
            display: 'flex',
            alignItems: 'center',
            gap: 7,
            padding: '0 9px',
            boxSizing: 'border-box',
          }}
        >
          <HomeIcon size={13} color="#7D8288" />
          <span style={{fontFamily: FONTS.ui, fontSize: 10.5, color: '#5C6168'}}>Home</span>
          <span style={{fontFamily: FONTS.ui, fontSize: 10.5, color: '#B4B8BE'}}>/</span>
          <span style={{fontFamily: FONTS.ui, fontSize: 10.5, color: '#2E3237'}}>Videos</span>
        </div>
        <SearchIcon size={15} />
        <MenuIcon size={15} />
      </div>

      <div style={{display: 'flex', height: CONTENT.height}}>
        <div
          style={{
            width: WIN.sidebar,
            background: '#FBFBFC',
            borderRight: `1px solid ${C.kdeLineSoft}`,
            paddingTop: 11,
            boxSizing: 'border-box',
          }}
        >
          <div
            style={{
              margin: '0 18px 7px',
              fontFamily: FONTS.mono,
              fontSize: 8,
              letterSpacing: '.13em',
              color: '#9A9FA5',
            }}
          >
            PLACES
          </div>
          <SidebarItem label="Home" iconNode={<HomeIcon size={14} />} />
          <SidebarItem label="Videos" iconNode={<VideoIcon size={14} />} selected />
          <SidebarItem label="Downloads" iconNode={<DownloadIcon size={14} />} />
          <SidebarItem label="Trash" iconNode={<TrashIcon size={14} />} />
          <div
            style={{
              margin: '12px 18px 7px',
              fontFamily: FONTS.mono,
              fontSize: 8,
              letterSpacing: '.13em',
              color: '#9A9FA5',
            }}
          >
            TAGS
          </div>
          <SidebarItem label="Favourites" iconNode={<StarIcon size={14} />} />
        </div>

        <div style={{position: 'relative', flex: 1, background: '#FFFFFF'}}>
          <div
            style={{
              position: 'absolute',
              left: 26,
              right: 24,
              top: 14,
              height: 18,
              display: 'flex',
              alignItems: 'center',
              gap: 8,
            }}
          >
            <span style={{fontFamily: FONTS.ui, fontSize: 12, fontWeight: 600, color: C.kdeText}}>Videos</span>
            <span style={{fontFamily: FONTS.mono, fontSize: 8.5, color: '#9A9FA5'}}>1 item</span>
            <div style={{flex: 1}} />
            <GridIcon size={13} />
          </div>

          <div
            style={{
              position: 'absolute',
              left: FILE_TILE.x - WIN.x - WIN.sidebar,
              top: FILE_TILE.y - CONTENT.y,
              width: FILE_TILE.width,
              height: FILE_TILE.height,
              borderRadius: 6,
              background: tileBackground,
              outline: fileState === 'idle' ? '1px solid transparent' : `1.5px solid ${C.blue}`,
              outlineOffset: 1,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 7,
              boxSizing: 'border-box',
            }}
          >
            <FileGlyph format={file} size={54} verified={verified} accent={isMkv ? C.ink : C.green} />
            <div
              style={{
                width: 112,
                textAlign: 'center',
                fontFamily: FONTS.ui,
                fontSize: 10.5,
                fontWeight: 500,
                color: C.kdeText,
                whiteSpace: 'nowrap',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
              }}
            >
              {name}
            </div>
            <div style={{marginTop: -4, fontFamily: FONTS.mono, fontSize: 8.5, color: '#7C8188'}}>{size}</div>
            {complete ? (
              <div
                style={{
                  position: 'absolute',
                  left: 8,
                  top: 7,
                  display: 'flex',
                  alignItems: 'center',
                  gap: 4,
                  padding: '2px 5px',
                  borderRadius: 3,
                  background: C.greenWash,
                  color: C.green,
                  fontFamily: FONTS.mono,
                  fontSize: 7,
                  letterSpacing: '.06em',
                }}
              >
                <CheckIcon size={7} color={C.green} strokeWidth={3.2} />
                VERIFIED
              </div>
            ) : null}
          </div>

          {children}
        </div>
      </div>

      <div
        style={{
          height: WIN.statusbar,
          borderTop: `1px solid ${C.kdeLineSoft}`,
          display: 'flex',
          alignItems: 'center',
          padding: '0 12px',
          background: '#FFFFFF',
          boxSizing: 'border-box',
        }}
      >
        <span style={{fontFamily: FONTS.mono, fontSize: 8.5, color: '#8B9097'}}>
          {complete ? 'conversion complete · source reclaimed' : '1 item · 60 GB'}
        </span>
        <div style={{flex: 1}} />
        {complete ? (
          <span style={{fontFamily: FONTS.mono, fontSize: 8.5, color: C.green}}>verified</span>
        ) : (
          <span style={{fontFamily: FONTS.mono, fontSize: 8.5, color: '#A3A7AD'}}>Free space 4.0 GB</span>
        )}
      </div>
    </div>
  );
};

export const DolphinHeader: React.FC<{kicker: string; right: string; rightColor?: string}> = ({
  kicker,
  right,
  rightColor = C.inkFaint,
}) => (
  <>
    <div
      style={{
        position: 'absolute',
        left: 64,
        top: 24,
        fontFamily: FONTS.mono,
        fontSize: 9,
        letterSpacing: '.12em',
        color: C.inkFaint,
      }}
    >
      {kicker}
    </div>
    <div
      style={{
        position: 'absolute',
        right: 64,
        top: 24,
        fontFamily: FONTS.mono,
        fontSize: 9,
        letterSpacing: '.06em',
        color: rightColor,
      }}
    >
      {right}
    </div>
  </>
);

/**
 * The exact initial frame. Also used as the final loop target so the
 * composition can restart without a cut.
 */
export const DolphinStartFrame: React.FC = () => (
  <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
    <DolphinHeader kicker="01 / KDE DOLPHIN" right="RIGHT-CLICK → SIPHON" />
    <DolphinWindow file="MKV" fileState="idle" />
    <Cursor x={CURSOR_START.x} y={CURSOR_START.y} />
    <div
      style={{
        position: 'absolute',
        left: 64,
        bottom: 18,
        fontFamily: FONTS.mono,
        fontSize: 8.5,
        letterSpacing: '.06em',
        color: C.inkFaint,
      }}
    >
      KDE DOLPHIN · SERVICEMENU
    </div>
  </AbsoluteFill>
);

export const DolphinScene: React.FC<{frame: number}> = ({frame}) => {
  const fileState: FileState = frame >= T.rightClick ? 'selected' : frame >= T.cursorToFile ? 'hover' : 'idle';
  const menuOpacity = ramp(frame, T.menuOpen, 5) * fall(frame, T.menuClose, 6);
  const menuScale = 0.97 + ramp(frame, T.menuOpen, 7) * 0.03;
  const siphonHover = frame >= T.siphonHover && menuOpacity > 0.1;
  const submenuOpacity = ramp(frame, T.submenuOpen, 5) * fall(frame, T.menuClose, 5);
  const submenuScale = 0.97 + ramp(frame, T.submenuOpen, 6) * 0.03;
  const actionHover = frame >= T.actionHover && submenuOpacity > 0.2;
  const actionClick = pulse(frame, T.actionClick, 9);
  const rightClick = pulse(frame, T.rightClick, 10);
  const cursor = cursorAt(frame, CURSOR_KEYS);
  const cursorOpacity = fall(frame, T.menuClose, 10);

  return (
    <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
      <DolphinHeader kicker="01 / KDE DOLPHIN" right="RIGHT-CLICK → SIPHON" />
      <DolphinWindow file="MKV" fileState={fileState} />
      <ContextMenu
        opacity={menuOpacity}
        scale={menuScale}
        siphonHover={siphonHover}
        submenuOpacity={submenuOpacity}
        submenuScale={submenuScale}
        actionHover={actionHover}
        actionClick={actionClick}
      />
      <Cursor
        x={cursor.x}
        y={cursor.y}
        opacity={cursorOpacity}
        click={Math.max(rightClick * 0.7, actionClick)}
      />
      <div
        style={{
          position: 'absolute',
          left: 64,
          bottom: 18,
          fontFamily: FONTS.mono,
          fontSize: 8.5,
          letterSpacing: '.06em',
          color: C.inkFaint,
        }}
      >
        KDE DOLPHIN · SERVICEMENU
      </div>
    </AbsoluteFill>
  );
};
