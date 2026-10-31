import React from 'react';
import {C, FONTS} from '../theme';

type IconProps = {
  size?: number;
  color?: string;
  strokeWidth?: number;
  style?: React.CSSProperties;
};

export const CheckIcon: React.FC<IconProps> = ({size = 14, color = C.white, strokeWidth = 2.6, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path
      d="M5 12.5l4.2 4.2L19 7"
      fill="none"
      stroke={color}
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

export const ChevronRight: React.FC<IconProps> = ({size = 13, color = C.kdeMuted, strokeWidth = 1.7, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path
      d="M9 5l7 7-7 7"
      fill="none"
      stroke={color}
      strokeWidth={strokeWidth}
      strokeLinecap="round"
      strokeLinejoin="round"
    />
  </svg>
);

export const ArrowLeft: React.FC<IconProps> = ({size = 14, color = C.kdeMuted, strokeWidth = 1.7, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M19 12H5m6-6l-6 6 6 6" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const ArrowRight: React.FC<IconProps> = ({size = 14, color = C.kdeMuted, strokeWidth = 1.7, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M5 12h14m-6-6l6 6-6 6" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const SearchIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.6, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <circle cx="10.5" cy="10.5" r="6" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M15 15l5 5" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" />
  </svg>
);

export const MenuIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    {[6, 12, 18].map((y) => (
      <path key={y} d={`M4 ${y}h16`} stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" />
    ))}
  </svg>
);

export const HomeIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.6, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M4 10.5L12 4l8 6.5V20H4z" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinejoin="round" />
    <path d="M9.5 20v-5.5h5V20" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinejoin="round" />
  </svg>
);

export const FolderIcon: React.FC<IconProps> = ({size = 16, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path
      d="M3 7.5A1.5 1.5 0 0 1 4.5 6H9l2 2h8.5A1.5 1.5 0 0 1 21 9.5v8A1.5 1.5 0 0 1 19.5 19h-15A1.5 1.5 0 0 1 3 17.5z"
      fill="none"
      stroke={color}
      strokeWidth={strokeWidth}
      strokeLinejoin="round"
    />
  </svg>
);

export const DownloadIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.6, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M12 4v11m-4.5-4.5L12 15l4.5-4.5M5 20h14" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const TrashIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.6, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M5 7h14M9 7V4.5h6V7m-8 0l1 13h8l1-13M10 10.5v6M14 10.5v6" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const StarIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M12 3.8l2.5 5.2 5.7.8-4.1 4 1 5.7-5.1-2.7-5.1 2.7 1-5.7-4.1-4 5.7-.8z" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinejoin="round" />
  </svg>
);

export const KdeLogo: React.FC<{size?: number}> = ({size = 18}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', flex: 'none'}}>
    <rect x="1" y="1" width="22" height="22" rx="5" fill={C.blue} />
    <path d="M7 5.6v12.8M7 12.2l5.1-6.6M7 12.2l5.5 6.2M13.1 9.4l4-4.1M13.1 14.8l4.2 3.7" stroke="#fff" strokeWidth="1.8" strokeLinecap="round" />
  </svg>
);

export const SiphonMark: React.FC<{size?: number; color?: string}> = ({size = 18, color = C.ink}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', flex: 'none'}}>
    <path d="M4 5.5h16l-5.6 7.6v5.3l-4.8 2.3v-7.6z" fill={color} />
  </svg>
);

export const FilmIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <rect x="3" y="5" width="18" height="14" rx="2" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M7 5v14M17 5v14M3 9h4M3 15h4M17 9h4M17 15h4" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" />
  </svg>
);

export const VideoIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <rect x="3" y="5.5" width="12.5" height="13" rx="2" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M15.5 10.5l5.5-3v9l-5.5-3z" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinejoin="round" />
  </svg>
);

export const CutIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <circle cx="6" cy="7" r="2.4" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <circle cx="6" cy="17" r="2.4" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M8.2 8.4L19 17M8.2 15.6L19 7" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" />
  </svg>
);

export const CopyIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <rect x="8" y="8" width="11" height="12" rx="1.5" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M16 8V5.5A1.5 1.5 0 0 0 14.5 4h-9A1.5 1.5 0 0 0 4 5.5v9A1.5 1.5 0 0 0 5.5 16H8" fill="none" stroke={color} strokeWidth={strokeWidth} />
  </svg>
);

export const RenameIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <path d="M4 19l3.5-.8L19 6.7a1.6 1.6 0 0 0 0-2.3l-.4-.4a1.6 1.6 0 0 0-2.3 0L4.8 15.5z" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinejoin="round" />
  </svg>
);

export const OpenWithIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <rect x="3.5" y="4.5" width="12" height="9" rx="1.5" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M10 20h10M15 14.5l5 5.5-5 5.5" fill="none" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
  </svg>
);

export const NewTabIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <rect x="3.5" y="6" width="17" height="14" rx="1.5" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M3.5 10h17M8 6V3.5" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" />
  </svg>
);

export const PropertiesIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    <circle cx="12" cy="12" r="3" fill="none" stroke={color} strokeWidth={strokeWidth} />
    <path d="M12 3.5v3M12 17.5v3M3.5 12h3M17.5 12h3M6 6l2.1 2.1M15.9 15.9L18 18M18 6l-2.1 2.1M8.1 15.9L6 18" stroke={color} strokeWidth={strokeWidth} strokeLinecap="round" />
  </svg>
);

export const GridIcon: React.FC<IconProps> = ({size = 15, color = C.kdeMuted, strokeWidth = 1.5, style}) => (
  <svg width={size} height={size} viewBox="0 0 24 24" style={{display: 'block', ...style}}>
    {[5, 12, 19].map((x) =>
      [5, 12, 19].map((y) => <rect key={`${x}-${y}`} x={x === 5 ? 3 : x === 12 ? 10 : 17} y={y === 5 ? 3 : y === 12 ? 10 : 17} width="4" height="4" rx="1" fill={color} />),
    )}
  </svg>
);

/**
 * A tiny 2D file glyph. The film perforations read as video at 720p.
 */
export const FileGlyph: React.FC<{
  format: 'MKV' | 'MP4';
  size?: number;
  accent?: string;
  verified?: boolean;
}> = ({format, size = 52, accent = C.ink, verified = false}) => {
  const w = size * 0.78;
  const h = size;
  return (
    <div style={{width: w, height: h, position: 'relative', flex: 'none'}}>
      <svg width={w} height={h} viewBox="0 0 40 52" style={{display: 'block'}}>
        <path d="M3 2h21l13 13v35H3z" fill="#FFFFFF" stroke={accent} strokeWidth="1.7" strokeLinejoin="round" />
        <path d="M24 2v13h13" fill="none" stroke={accent} strokeWidth="1.7" strokeLinejoin="round" />
        <rect x="8" y="20" width="24" height="15" rx="1.2" fill={accent} />
        <path d="M11 21.5v12M15.5 21.5v12M24.5 21.5v12M29 21.5v12" stroke="#FFFFFF" strokeWidth="1.2" strokeOpacity="0.78" />
        <rect x="17.5" y="24.5" width="5" height="5" rx="0.8" fill="#FFFFFF" fillOpacity="0.82" />
        <text x="20" y="46" textAnchor="middle" fontFamily={FONTS.mono} fontSize="6.4" fontWeight="700" fill={accent} letterSpacing="0.5">
          {format}
        </text>
      </svg>
      {verified ? (
        <div
          style={{
            position: 'absolute',
            right: -7,
            bottom: -4,
            width: 18,
            height: 18,
            borderRadius: 9,
            background: C.green,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            boxShadow: '0 0 0 2.5px #FFFFFF',
          }}
        >
          <CheckIcon size={11} strokeWidth={3.2} />
        </div>
      ) : null}
    </div>
  );
};
