import React from 'react';
import {AbsoluteFill} from 'remotion';
import {C, FONTS} from '../theme';
import {
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  BatteryIcon,
  ChevronDown,
  ChevronRight,
  ClockIcon,
  DocumentIcon,
  DownloadIcon,
  FileFilm,
  FolderIcon,
  GridIcon,
  HomeIcon,
  KdeLogo,
  MenuIcon,
  MoreIcon,
  NetworkIcon,
  PictureIcon,
  SearchIcon,
  SiphonMark,
  SplitIcon,
  TrashIcon,
  VideoIcon,
  VolumeIcon,
  WifiIcon,
} from './Icons';

export const WINDOW = {
  x: 100,
  y: 60,
  width: 1720,
  height: 930,
  titlebar: 42,
  toolbar: 56,
  statusbar: 28,
  sidebar: 240,
} as const;

export const MAIN = {
  x: WINDOW.x + WINDOW.sidebar,
  y: WINDOW.y + WINDOW.titlebar + WINDOW.toolbar,
  width: WINDOW.width - WINDOW.sidebar,
  height: WINDOW.height - WINDOW.titlebar - WINDOW.toolbar - WINDOW.statusbar,
} as const;

export const FILE_TILE = {
  x: 120,
  y: 92,
  width: 176,
  height: 176,
} as const;

export const FILE_CENTER = {
  x: MAIN.x + FILE_TILE.x + FILE_TILE.width / 2,
  y: MAIN.y + FILE_TILE.y + 72,
} as const;

const TitleButton: React.FC<{kind: 'min' | 'max' | 'close'}> = ({kind}) => (
  <div
    style={{
      width: 19,
      height: 19,
      borderRadius: 10,
      border: `1px solid ${C.hairline}`,
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      background: kind === 'close' ? '#FDF1F0' : '#FFFFFF',
    }}
  >
    {kind === 'min' ? <div style={{width: 7, height: 1.4, background: '#60646B'}} /> : null}
    {kind === 'max' ? <div style={{width: 7, height: 7, border: '1.4px solid #60646B', borderRadius: 1.5}} /> : null}
    {kind === 'close' ? (
      <svg width="9" height="9" viewBox="0 0 9 9">
        <path d="M1 1l7 7M8 1L1 8" stroke="#B63A34" strokeWidth="1.4" strokeLinecap="round" />
      </svg>
    ) : null}
  </div>
);

const ToolButton: React.FC<{
  children: React.ReactNode;
  active?: boolean;
  width?: number;
  background?: string;
}> = ({children, active = true, width = 34, background}) => (
  <div
    style={{
      width,
      height: 34,
      borderRadius: 6,
      background: background ?? 'transparent',
      color: active ? '#3C4047' : '#C4C8CD',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
    }}
  >
    {children}
  </div>
);

const NavItem: React.FC<{
  icon: React.ReactNode;
  label: string;
  selected?: boolean;
}> = ({icon, label, selected = false}) => (
  <div
    style={{
      position: 'relative',
      height: 35,
      padding: '0 13px',
      borderRadius: 6,
      display: 'flex',
      alignItems: 'center',
      gap: 11,
      color: selected ? '#2645C6' : '#42464D',
      background: selected ? C.blueSelection : 'transparent',
      fontWeight: selected ? 600 : 400,
      fontSize: 14.5,
    }}
  >
    {selected ? (
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 8,
          bottom: 8,
          width: 3,
          borderRadius: 2,
          background: C.blue,
        }}
      />
    ) : null}
    <span style={{color: selected ? C.blue : '#737980', display: 'flex'}}>{icon}</span>
    <span>{label}</span>
  </div>
);

const SectionLabel: React.FC<{children: React.ReactNode}> = ({children}) => (
  <div
    style={{
      margin: '14px 13px 7px',
      fontFamily: FONTS.mono,
      fontSize: 10.5,
      letterSpacing: '0.12em',
      color: '#8D9299',
      textTransform: 'uppercase',
    }}
  >
    {children}
  </div>
);

export const TopPanel: React.FC<{opacity: number}> = ({opacity}) => (
  <div
    style={{
      position: 'absolute',
      left: 0,
      right: 0,
      top: 0,
      height: 32,
      zIndex: 20,
      opacity,
      background: C.panel,
      borderBottom: `1px solid ${C.hairline}`,
      display: 'flex',
      alignItems: 'center',
      padding: '0 12px',
      boxShadow: '0 1px 2px rgba(23,23,23,0.035)',
    }}
  >
    <KdeLogo size={19} />
    <div
      style={{
        marginLeft: 9,
        height: 24,
        padding: '0 9px',
        borderRadius: 5,
        background: '#FFFFFF',
        border: `1px solid ${C.hairlineSoft}`,
        display: 'flex',
        alignItems: 'center',
        gap: 7,
        color: '#3F4349',
        fontFamily: FONTS.ui,
        fontSize: 12.5,
      }}
    >
      <span
        style={{
          width: 15,
          height: 15,
          borderRadius: 4,
          background: C.blueWash,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: C.blue,
        }}
      >
        <FolderIcon size={12} />
      </span>
      <span>interstellar — Dolphin</span>
    </div>
    <div style={{flex: 1}} />
    <div style={{display: 'flex', alignItems: 'center', gap: 12, color: '#656A72'}}>
      <WifiIcon size={15} />
      <VolumeIcon size={15} />
      <BatteryIcon size={16} />
      <span style={{fontFamily: FONTS.ui, fontSize: 12.5, color: '#44484F', letterSpacing: '.01em'}}>14:32</span>
      <ChevronDown size={12} />
    </div>
  </div>
);

export const Wallpaper: React.FC = () => (
  <AbsoluteFill style={{background: C.desktop, overflow: 'hidden'}}>
    <div
      style={{
        position: 'absolute',
        width: 980,
        height: 980,
        borderRadius: '50%',
        right: -310,
        top: -420,
        background: '#E4E9F4',
      }}
    />
    <div
      style={{
        position: 'absolute',
        width: 760,
        height: 760,
        borderRadius: '50%',
        left: -260,
        bottom: -430,
        background: '#F4EFE4',
      }}
    />
    <div
      style={{
        position: 'absolute',
        width: 460,
        height: 460,
        borderRadius: '50%',
        left: 1040,
        bottom: -310,
        background: '#E9F2EF',
      }}
    />
  </AbsoluteFill>
);

export type DolphinFileState = 'idle' | 'hover' | 'selected';

type DolphinWindowProps = {
  fileName: string;
  fileSize: string;
  fileFormat?: string;
  fileAccent?: string;
  fileState?: DolphinFileState;
  showCheck?: boolean;
  dimmed?: boolean;
  statusExtra?: React.ReactNode;
};

export const DolphinWindow: React.FC<DolphinWindowProps> = ({
  fileName,
  fileSize,
  fileFormat = 'MKV',
  fileAccent = C.red,
  fileState = 'idle',
  showCheck = false,
  dimmed = false,
  statusExtra,
}) => {
  const tileBg =
    fileState === 'selected' ? C.blueSelection : fileState === 'hover' ? '#F1F5FF' : 'transparent';
  const tileBorder =
    fileState === 'selected' ? `1px solid ${C.blueLine}` : '1px solid transparent';

  return (
    <div
      style={{
        position: 'absolute',
        left: WINDOW.x,
        top: WINDOW.y,
        width: WINDOW.width,
        height: WINDOW.height,
        borderRadius: 13,
        overflow: 'hidden',
        background: C.window,
        border: `1px solid ${C.hairline}`,
        boxShadow: '0 28px 70px rgba(23,23,23,0.15), 0 4px 14px rgba(23,23,23,0.07)',
        opacity: dimmed ? 0.55 : 1,
        fontFamily: FONTS.ui,
      }}
    >
      {/* Window decoration */}
      <div
        style={{
          height: WINDOW.titlebar,
          background: C.chrome,
          borderBottom: `1px solid ${C.hairline}`,
          position: 'relative',
          display: 'flex',
          alignItems: 'center',
        }}
      >
        <div
          style={{
            position: 'absolute',
            left: 0,
            right: 0,
            textAlign: 'center',
            fontSize: 13.5,
            fontWeight: 500,
            color: '#555A61',
            letterSpacing: '.005em',
          }}
        >
          interstellar — Dolphin
        </div>
        <div style={{position: 'absolute', right: 13, display: 'flex', gap: 8}}>
          <TitleButton kind="min" />
          <TitleButton kind="max" />
          <TitleButton kind="close" />
        </div>
      </div>

      {/* Toolbar */}
      <div
        style={{
          height: WINDOW.toolbar,
          background: '#FFFFFF',
          borderBottom: `1px solid ${C.hairlineSoft}`,
          display: 'flex',
          alignItems: 'center',
          padding: '0 10px',
          gap: 2,
        }}
      >
        <ToolButton width={38} background="#F5F6F8">
          <MenuIcon size={18} />
        </ToolButton>
        <div style={{width: 1, height: 22, background: C.hairlineSoft, margin: '0 6px'}} />
        <ToolButton>
          <ArrowLeft size={17} />
        </ToolButton>
        <ToolButton active={false}>
          <ArrowRight size={17} />
        </ToolButton>
        <ToolButton>
          <ArrowUp size={17} />
        </ToolButton>

        {/* Breadcrumb */}
        <div
          style={{
            marginLeft: 12,
            height: 34,
            minWidth: 430,
            flex: '0 1 560px',
            borderRadius: 7,
            background: '#F7F8FA',
            border: `1px solid ${C.hairlineSoft}`,
            display: 'flex',
            alignItems: 'center',
            padding: '0 7px',
            gap: 3,
            color: '#666B73',
          }}
        >
          <div style={{width: 26, height: 26, display: 'flex', alignItems: 'center', justifyContent: 'center'}}>
            <HomeIcon size={15} />
          </div>
          <ChevronRight size={12} color="#A0A4AA" />
          <div style={{fontSize: 14, padding: '0 7px'}}>Videos</div>
          <ChevronRight size={12} color="#A0A4AA" />
          <div
            style={{
              display: 'flex',
              alignItems: 'center',
              gap: 7,
              padding: '4px 9px',
              borderRadius: 5,
              background: C.blueWash,
              border: `1px solid #D9E1FF`,
              color: '#2C47B4',
              fontSize: 14,
              fontWeight: 600,
            }}
          >
            <FolderIcon size={14} />
            interstellar
          </div>
        </div>

        <div style={{flex: 1}} />
        <ToolButton width={36}>
          <SearchIcon size={17} />
        </ToolButton>
        <ToolButton width={36}>
          <GridIcon size={17} />
        </ToolButton>
        <ToolButton width={36}>
          <SplitIcon size={17} />
        </ToolButton>
        <div style={{width: 1, height: 22, background: C.hairlineSoft, margin: '0 5px'}} />
        <ToolButton width={32}>
          <MoreIcon size={17} />
        </ToolButton>
      </div>

      {/* Body */}
      <div style={{display: 'flex', height: WINDOW.height - WINDOW.titlebar - WINDOW.toolbar - WINDOW.statusbar}}>
        <div
          style={{
            width: WINDOW.sidebar,
            background: C.chromeRaised,
            borderRight: `1px solid ${C.hairlineSoft}`,
            padding: '11px 10px',
            boxSizing: 'border-box',
          }}
        >
          <NavItem icon={<HomeIcon size={17} />} label="Home" />
          <NavItem icon={<ClockIcon size={17} />} label="Recent" />
          <NavItem icon={<TrashIcon size={17} />} label="Trash" />
          <SectionLabel>Places</SectionLabel>
          <NavItem icon={<VideoIcon size={17} />} label="Videos" selected />
          <NavItem icon={<PictureIcon size={17} />} label="Pictures" />
          <NavItem icon={<DocumentIcon size={17} />} label="Documents" />
          <NavItem icon={<DownloadIcon size={17} />} label="Downloads" />
          <SectionLabel>Network</SectionLabel>
          <NavItem icon={<NetworkIcon size={17} />} label="Network" />
        </div>

        <div style={{position: 'relative', flex: 1, background: '#FFFFFF', overflow: 'hidden'}}>
          <div
            style={{
              position: 'absolute',
              left: FILE_TILE.x,
              top: FILE_TILE.y,
              width: FILE_TILE.width,
              height: FILE_TILE.height,
              borderRadius: 10,
              background: tileBg,
              border: tileBorder,
              boxSizing: 'border-box',
            }}
          >
            <div
              style={{
                position: 'absolute',
                left: (FILE_TILE.width - 92) / 2,
                top: 14,
                width: 92,
                height: 110,
                display: 'flex',
                justifyContent: 'center',
              }}
            >
              <FileFilm size={92} accent={fileAccent} format={fileFormat} />
              {showCheck ? (
                <div
                  style={{
                    position: 'absolute',
                    right: -8,
                    bottom: -2,
                    width: 30,
                    height: 30,
                    borderRadius: 15,
                    background: C.green,
                    border: '3px solid #fff',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    boxShadow: '0 2px 7px rgba(23,23,23,0.16)',
                  }}
                >
                  <svg width="14" height="14" viewBox="0 0 24 24">
                    <path d="M5 12.5l4.2 4.3L19 7" fill="none" stroke="#fff" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
                  </svg>
                </div>
              ) : null}
            </div>
            <div
              style={{
                position: 'absolute',
                left: 4,
                right: 4,
                top: 126,
                textAlign: 'center',
                fontSize: 14,
                fontWeight: 600,
                lineHeight: 1.25,
                color: C.ink,
                letterSpacing: '-.005em',
              }}
            >
              {fileName}
            </div>
            <div
              style={{
                position: 'absolute',
                left: 4,
                right: 4,
                top: 150,
                textAlign: 'center',
                fontSize: 12.5,
                color: C.muted,
              }}
            >
              {fileSize}
            </div>
          </div>

          <div
            style={{
              position: 'absolute',
              right: 28,
              top: 32,
              display: 'flex',
              alignItems: 'center',
              gap: 8,
              color: C.mutedSoft,
              fontSize: 12,
            }}
          >
            <span>Icons</span>
            <ChevronDown size={12} />
          </div>

          {statusExtra}
        </div>
      </div>

      {/* Status bar */}
      <div
        style={{
          height: WINDOW.statusbar,
          background: C.chromeRaised,
          borderTop: `1px solid ${C.hairlineSoft}`,
          display: 'flex',
          alignItems: 'center',
          padding: '0 12px',
          color: C.muted,
          fontSize: 12.5,
          gap: 12,
        }}
      >
        <span>1 item</span>
        <span style={{color: '#B3B7BD'}}>•</span>
        <span>{fileSize}</span>
        <div style={{flex: 1}} />
        <span style={{display: 'flex', alignItems: 'center', gap: 8}}>
          <span style={{fontSize: 14}}>−</span>
          <div style={{width: 72, height: 4, borderRadius: 2, background: '#DADDE2', position: 'relative'}}>
            <div
              style={{
                position: 'absolute',
                left: 30,
                top: -4,
                width: 12,
                height: 12,
                borderRadius: 6,
                background: '#FFFFFF',
                border: `1px solid #B6BAC1`,
              }}
            />
          </div>
          <span style={{fontSize: 14}}>+</span>
        </span>
        <span style={{ width: 1, height: 14, background: C.hairline, marginLeft: 4 }} />
        <GridIcon size={14} />
      </div>
    </div>
  );
};

export const Desktop: React.FC<{
  children?: React.ReactNode;
  panelOpacity?: number;
  style?: React.CSSProperties;
}> = ({children, panelOpacity = 1, style}) => (
  <AbsoluteFill style={{overflow: 'hidden', ...style}}>
    <Wallpaper />
    <TopPanel opacity={panelOpacity} />
    {children}
  </AbsoluteFill>
);

/** The small Siphon chip used in the end-card and notification. */
export const SiphonChip: React.FC<{label?: string; dark?: boolean}> = ({label = 'SIPHON', dark = false}) => (
  <div style={{display: 'flex', alignItems: 'center', gap: 9}}>
    <SiphonMark size={20} color={dark ? C.ink : C.blue} />
    <span
      style={{
        fontFamily: FONTS.ui,
        fontWeight: 700,
        fontSize: 15,
        letterSpacing: '.02em',
        color: dark ? C.ink : C.blue,
      }}
    >
      {label}
    </span>
  </div>
);
