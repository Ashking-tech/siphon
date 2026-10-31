import React from 'react';
import {C, FONTS} from '../theme';

type IconProps = {
  size?: number;
  color?: string;
  strokeWidth?: number;
  style?: React.CSSProperties;
};

const Svg: React.FC<IconProps & {children: React.ReactNode; viewBox?: string}> = ({
  size = 18,
  color = 'currentColor',
  strokeWidth = 1.7,
  style,
  viewBox = '0 0 24 24',
  children,
}) => (
  <svg
    width={size}
    height={size}
    viewBox={viewBox}
    fill="none"
    stroke={color}
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    style={{display: 'block', flex: 'none', ...style}}
  >
    {children}
  </svg>
);

export const MenuIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4 7h16M4 12h16M4 17h16" />
  </Svg>
);

export const ArrowLeft: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M19 12H5M11 18l-6-6 6-6" />
  </Svg>
);

export const ArrowRight: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M5 12h14M13 6l6 6-6 6" />
  </Svg>
);

export const ArrowUp: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M12 19V5M6 11l6-6 6 6" />
  </Svg>
);

export const ChevronRight: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M9 5l7 7-7 7" />
  </Svg>
);

export const ChevronDown: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M5 9l7 7 7-7" />
  </Svg>
);

export const SearchIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <circle cx="10.7" cy="10.7" r="6.4" />
    <path d="M15.4 15.4L21 21" />
  </Svg>
);

export const GridIcon: React.FC<IconProps> = (p) => (
  <Svg {...p} strokeWidth={1.5}>
    <rect x="4" y="4" width="6.5" height="6.5" rx="1" />
    <rect x="13.5" y="4" width="6.5" height="6.5" rx="1" />
    <rect x="4" y="13.5" width="6.5" height="6.5" rx="1" />
    <rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1" />
  </Svg>
);

export const SplitIcon: React.FC<IconProps> = (p) => (
  <Svg {...p} strokeWidth={1.5}>
    <rect x="3.5" y="5" width="17" height="14" rx="2" />
    <path d="M12 5v14" />
  </Svg>
);

export const MoreIcon: React.FC<IconProps> = (p) => (
  <Svg {...p} strokeWidth={0} color="currentColor">
    <circle cx="5" cy="12" r="1.7" fill="currentColor" />
    <circle cx="12" cy="12" r="1.7" fill="currentColor" />
    <circle cx="19" cy="12" r="1.7" fill="currentColor" />
  </Svg>
);

export const HomeIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4 10.5L12 4l8 6.5V20a1 1 0 0 1-1 1h-4.5v-6.5h-5V21H5a1 1 0 0 1-1-1z" />
  </Svg>
);

export const ClockIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <circle cx="12" cy="12" r="8.2" />
    <path d="M12 7.5v5l3.2 1.7" />
  </Svg>
);

export const TrashIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4.5 7h15M9.5 7V4.6h5V7M7 7l.9 13h8.2L17 7M10.2 10.5v6M13.8 10.5v6" />
  </Svg>
);

export const VideoIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="3" y="5" width="13" height="14" rx="2" />
    <path d="M16 10l5-2.8v9.6L16 14z" />
  </Svg>
);

export const PictureIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="3.5" y="4.5" width="17" height="15" rx="2" />
    <circle cx="9" cy="10" r="1.6" />
    <path d="M4 17l4.5-4.5 3.4 3.2 2.2-2.2L20 19" />
  </Svg>
);

export const DocumentIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M6 3h8l4 4v14H6z" />
    <path d="M14 3v5h4M9 12h6M9 16h6" />
  </Svg>
);

export const DownloadIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M12 3v12M7 11l5 5 5-5M4 20h16" />
  </Svg>
);

export const NetworkIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <circle cx="12" cy="12" r="8.4" />
    <path d="M3.8 12h16.4M12 3.6c2.2 2.3 3.4 5.2 3.4 8.4S14.2 18.1 12 20.4C9.8 18.1 8.6 15.2 8.6 12S9.8 5.9 12 3.6" />
  </Svg>
);

export const WifiIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4 9.5a12.5 12.5 0 0 1 16 0M7 13a8 8 0 0 1 10 0M10 16.4a3.3 3.3 0 0 1 4 0" />
    <circle cx="12" cy="20" r=".8" fill="currentColor" />
  </Svg>
);

export const VolumeIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4 9.5h3L12 5v14L7 14.5H4zM15.5 9a4 4 0 0 1 0 6M18 6.5a8 8 0 0 1 0 11" />
  </Svg>
);

export const BatteryIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="3" y="7" width="16" height="10" rx="2" />
    <path d="M21 10.5v3" />
    <rect x="5" y="9" width="10" height="6" rx="1" fill="currentColor" stroke="none" />
  </Svg>
);

export const CheckIcon: React.FC<IconProps> = ({size = 18, color = C.white, strokeWidth = 2.4, style}) => (
  <svg
    width={size}
    height={size}
    viewBox="0 0 24 24"
    fill="none"
    stroke={color}
    strokeWidth={strokeWidth}
    strokeLinecap="round"
    strokeLinejoin="round"
    style={{display: 'block', flex: 'none', ...style}}
  >
    <path d="M5 12.5l4.2 4.3L19 7" />
  </svg>
);

export const CheckCircle: React.FC<IconProps> = ({size = 20, color = C.green, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', flex: 'none', ...style}}>
    <circle cx="12" cy="12" r="10" fill={color} />
    <path d="M7.2 12.3l3.1 3.1 6.6-6.8" fill="none" stroke="#fff" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const LockIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="5" y="10.5" width="14" height="10" rx="2" />
    <path d="M8 10.5V8a4 4 0 0 1 8 0v2.5M12 14.5v2" />
  </Svg>
);

export const InfoIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <circle cx="12" cy="12" r="8.4" />
    <path d="M12 10.5v5M12 7.6v.1" />
  </Svg>
);

export const OpenWithIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4 20l4.5-1 10-10a2.1 2.1 0 0 0-3-3l-10 10zM13 7l3 3" />
  </Svg>
);

export const NewTabIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="4" y="6" width="13" height="14" rx="2" />
    <path d="M8 6V4h12v12h-2M10.5 13h6M13.5 10v6" />
  </Svg>
);

export const NewWindowIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="3" y="5" width="14" height="12" rx="2" />
    <path d="M7 8.5h6M17 9h2a2 2 0 0 1 2 2v8H9v-2" />
  </Svg>
);

export const CutIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <circle cx="6.5" cy="17.5" r="2.2" />
    <circle cx="17.5" cy="17.5" r="2.2" />
    <path d="M8.2 16L18 4M15.8 16L6 4" />
  </Svg>
);

export const CopyIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="8" y="8" width="11" height="12" rx="1.5" />
    <path d="M16 8V5.5A1.5 1.5 0 0 0 14.5 4h-10A1.5 1.5 0 0 0 3 5.5v10A1.5 1.5 0 0 0 4.5 17H8" />
  </Svg>
);

export const RenameIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M4 20h4L19 9a2.8 2.8 0 0 0-4-4L4 16zM13.5 6.5l4 4" />
  </Svg>
);

export const PropertiesIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <circle cx="12" cy="12" r="3" />
    <path d="M12 3.5v2M12 18.5v2M20.5 12h-2M5.5 12h-2M18 6l-1.4 1.4M7.4 16.6L6 18M18 18l-1.4-1.4M7.4 7.4L6 6" />
  </Svg>
);

export const FilmIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <rect x="3" y="4.5" width="18" height="15" rx="2" />
    <path d="M7 4.5v15M17 4.5v15M3 9h4M3 15h4M17 9h4M17 15h4" />
  </Svg>
);

export const FileFilm: React.FC<{size?: number; accent?: string; format?: string}> = ({
  size = 100,
  accent = C.red,
  format = 'MKV',
}) => (
  <svg width={size} height={size} viewBox="0 0 80 96" style={{display: 'block', flex: 'none'}}>
    <rect x="4" y="2" width="72" height="92" rx="7" fill={C.white} stroke={C.ink} strokeWidth="2.2" />
    <path d="M4 9a7 7 0 0 1 7-7h58a7 7 0 0 1 7 7v12H4z" fill={C.ink} />
    <text
      x="40"
      y="16.6"
      textAnchor="middle"
      fill={C.white}
      fontFamily={FONTS.mono}
      fontSize="8.4"
      fontWeight="700"
      letterSpacing="1.2"
    >
      {format}
    </text>
    <circle cx="66" cy="15" r="3" fill={accent} />
    <rect x="14" y="35" width="52" height="34" rx="3.5" fill={C.ink} />
    <g fill={C.white}>
      <rect x="18" y="39" width="4" height="4" rx=".8" />
      <rect x="18" y="47" width="4" height="4" rx=".8" />
      <rect x="18" y="55" width="4" height="4" rx=".8" />
      <rect x="58" y="39" width="4" height="4" rx=".8" />
      <rect x="58" y="47" width="4" height="4" rx=".8" />
      <rect x="58" y="55" width="4" height="4" rx=".8" />
    </g>
    <path d="M35 45.3l7.5 4.4-7.5 4.4z" fill={C.white} />
    <rect x="14" y="78" width="35" height="3" rx="1.5" fill={C.ink} opacity=".72" />
    <rect x="14" y="84" width="24" height="3" rx="1.5" fill={C.ink} opacity=".26" />
  </svg>
);

export const FolderIcon: React.FC<IconProps> = (p) => (
  <Svg {...p}>
    <path d="M3 7.5A2.5 2.5 0 0 1 5.5 5H10l2 2h6.5A2.5 2.5 0 0 1 21 9.5v8A2.5 2.5 0 0 1 18.5 20h-13A2.5 2.5 0 0 1 3 17.5z" />
  </Svg>
);

export const SiphonMark: React.FC<{size?: number; color?: string; stroke?: string}> = ({
  size = 20,
  color = C.blue,
  stroke = C.white,
}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', flex: 'none'}}>
    <rect x="1.5" y="1.5" width="21" height="21" rx="6" fill={color} />
    <path
      d="M7 7.2h8.6a2.4 2.4 0 0 1 0 4.8H9.4a2.4 2.4 0 0 0 0 4.8H18"
      fill="none"
      stroke={stroke}
      strokeWidth="2.2"
      strokeLinecap="round"
      strokeLinejoin="round"
    />
    <path d="M15.8 14.2L18.4 16.8L15.8 19.4" fill="none" stroke={stroke} strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const KdeLogo: React.FC<{size?: number}> = ({size = 20}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block'}}>
    <rect x="1" y="1" width="22" height="22" rx="6" fill={C.blue} />
    <path d="M7 5.5v13M7 12.2l5.2-6.7M7 12.2l5.6 6.3M13.2 9.4l4-4.1M13.2 14.8l4.2 3.7" stroke="#fff" strokeWidth="1.9" strokeLinecap="round" />
  </svg>
);

export const PlayIcon: React.FC<IconProps> = ({size = 16, color = C.white, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M7 4.8l11 7.2-11 7.2z" fill={color} />
  </svg>
);
