import React from 'react';
import {C, FONTS} from '../theme';
import {
  ChevronRight,
  CopyIcon,
  CutIcon,
  FilmIcon,
  NewTabIcon,
  NewWindowIcon,
  OpenWithIcon,
  PropertiesIcon,
  RenameIcon,
  TrashIcon,
  VideoIcon,
} from './Icons';

export const MENU = {
  x: 556,
  y: 330,
  width: 304,
  itemHeight: 37,
  pad: 6,
  separator: 11,
} as const;

type Row =
  | {kind: 'item'; id: string; label: string; icon: React.ReactNode; shortcut?: string; submenu?: boolean}
  | {kind: 'sep'; id: string};

const menuIcon = (node: React.ReactNode) => (
  <span style={{width: 18, height: 18, display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
    {node}
  </span>
);

const ROWS: Row[] = [
  {kind: 'item', id: 'openwith', label: 'Open With', icon: menuIcon(<OpenWithIcon size={16} />), submenu: true},
  {kind: 'item', id: 'newtab', label: 'Open in New Tab', icon: menuIcon(<NewTabIcon size={16} />)},
  {kind: 'item', id: 'newwindow', label: 'Open in New Window', icon: menuIcon(<NewWindowIcon size={16} />)},
  {kind: 'sep', id: 'sep1'},
  {kind: 'item', id: 'cut', label: 'Cut', icon: menuIcon(<CutIcon size={16} />), shortcut: 'Ctrl+X'},
  {kind: 'item', id: 'copy', label: 'Copy', icon: menuIcon(<CopyIcon size={16} />), shortcut: 'Ctrl+C'},
  {kind: 'item', id: 'rename', label: 'Rename…', icon: menuIcon(<RenameIcon size={16} />), shortcut: 'F2'},
  {kind: 'sep', id: 'sep2'},
  {kind: 'item', id: 'trash', label: 'Move to Trash', icon: menuIcon(<TrashIcon size={16} />), shortcut: 'Del'},
  {kind: 'sep', id: 'sep3'},
  {kind: 'item', id: 'siphon', label: 'Siphon', icon: menuIcon(<FilmIcon size={16} />), submenu: true},
  {kind: 'sep', id: 'sep4'},
  {kind: 'item', id: 'properties', label: 'Properties', icon: menuIcon(<PropertiesIcon size={16} />), shortcut: 'Alt+Return'},
];

const findRowTop = (id: string) => {
  let y = MENU.y + MENU.pad;
  for (const row of ROWS) {
    if (row.kind === 'item' && row.id === id) return y;
    y += row.kind === 'sep' ? MENU.separator : MENU.itemHeight;
  }
  return y;
};

export const SIPHON_ROW_TOP = findRowTop('siphon');
export const SIPHON_ROW_CENTER = SIPHON_ROW_TOP + MENU.itemHeight / 2;
export const SUBMENU = {
  x: MENU.x + MENU.width + 2,
  y: SIPHON_ROW_TOP - MENU.pad,
  width: 392,
  rowHeight: 40,
  pad: 6,
} as const;
export const ACTION_ROW_CENTER = SUBMENU.y + SUBMENU.pad + SUBMENU.rowHeight / 2;

const MenuRow: React.FC<{
  label: string;
  icon: React.ReactNode;
  shortcut?: string;
  submenu?: boolean;
  hovered: boolean;
  height: number;
  fontSize: number;
  click?: number;
}> = ({label, icon, shortcut, submenu, hovered, height, fontSize, click = 0}) => (
  <div
    style={{
      height,
      display: 'flex',
      alignItems: 'center',
      gap: 10,
      borderRadius: 4,
      margin: '0 4px',
      padding: '0 9px',
      background: hovered ? C.blue : 'transparent',
      color: hovered ? '#FFFFFF' : C.ink,
      boxShadow: click > 0 && hovered ? `inset 0 0 0 1.5px rgba(255,255,255,${0.55 * click})` : undefined,
    }}
  >
    <span style={{color: hovered ? '#FFFFFF' : '#6F747B'}}>{icon}</span>
    <span style={{fontSize, whiteSpace: 'nowrap'}}>{label}</span>
    <div style={{flex: 1}} />
    {shortcut ? (
      <span
        style={{
          fontFamily: FONTS.mono,
          fontSize: 10,
          letterSpacing: '.01em',
          color: hovered ? 'rgba(255,255,255,.72)' : '#9A9EA5',
        }}
      >
        {shortcut}
      </span>
    ) : null}
    {submenu ? <ChevronRight size={13} color={hovered ? '#FFFFFF' : '#7C8188'} /> : null}
  </div>
);

type ContextMenuProps = {
  opacity: number;
  scale: number;
  siphonHover: boolean;
  submenuOpacity: number;
  submenuScale: number;
  actionHover: boolean;
  actionClick: number;
};

export const ContextMenu: React.FC<ContextMenuProps> = ({
  opacity,
  scale,
  siphonHover,
  submenuOpacity,
  submenuScale,
  actionHover,
  actionClick,
}) => {
  if (opacity <= 0) return null;
  return (
    <>
      <div
        style={{
          position: 'absolute',
          left: MENU.x,
          top: MENU.y,
          width: MENU.width,
          zIndex: 70,
          padding: `${MENU.pad}px 0`,
          background: '#FFFFFF',
          border: `1px solid #D6D9DE`,
          borderRadius: 7,
          boxShadow: '0 12px 32px rgba(23,23,23,0.17), 0 2px 8px rgba(23,23,23,0.08)',
          opacity,
          transform: `scale(${scale})`,
          transformOrigin: 'top left',
          boxSizing: 'border-box',
          fontFamily: FONTS.ui,
        }}
      >
        {ROWS.map((row) =>
          row.kind === 'sep' ? (
            <div
              key={row.id}
              style={{
                height: 1,
                margin: '5px 12px',
                background: C.hairlineSoft,
              }}
            />
          ) : (
            <MenuRow
              key={row.id}
              label={row.label}
              icon={row.icon}
              shortcut={row.shortcut}
              submenu={row.submenu}
              hovered={row.id === 'siphon' && siphonHover}
              height={MENU.itemHeight}
              fontSize={14.5}
              click={row.id === 'siphon' ? 0 : 0}
            />
          ),
        )}
      </div>

      {submenuOpacity > 0 ? (
        <div
          style={{
            position: 'absolute',
            left: SUBMENU.x,
            top: SUBMENU.y,
            width: SUBMENU.width,
            zIndex: 71,
            padding: `${SUBMENU.pad}px 0`,
            background: '#FFFFFF',
            border: `1px solid #D6D9DE`,
            borderRadius: 7,
            boxShadow: '0 12px 32px rgba(23,23,23,0.17), 0 2px 8px rgba(23,23,23,0.08)',
            opacity: submenuOpacity,
            transform: `scale(${submenuScale})`,
            transformOrigin: 'top left',
            boxSizing: 'border-box',
            fontFamily: FONTS.ui,
          }}
        >
          <MenuRow
            label="Convert to MP4 (saves space)"
            icon={<VideoIcon size={17} />}
            hovered={actionHover}
            height={SUBMENU.rowHeight}
            fontSize={14.5}
            click={actionClick}
          />
          <MenuRow
            label="Convert to classic MP4 (needs 2× space)"
            icon={<VideoIcon size={17} />}
            hovered={false}
            height={SUBMENU.rowHeight}
            fontSize={14.5}
          />
        </div>
      ) : null}
    </>
  );
};
