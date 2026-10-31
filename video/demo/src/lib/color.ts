const clamp = (value: number) => Math.min(255, Math.max(0, value));

export const parseHex = (hex: string): [number, number, number] => {
  const clean = hex.replace('#', '');
  const full =
    clean.length === 3
      ? clean
          .split('')
          .map((c) => c + c)
          .join('')
      : clean;
  return [
    parseInt(full.slice(0, 2), 16),
    parseInt(full.slice(2, 4), 16),
    parseInt(full.slice(4, 6), 16),
  ];
};

export const toHex = (rgb: [number, number, number]) =>
  `#${rgb.map((v) => Math.round(clamp(v)).toString(16).padStart(2, '0')).join('')}`;

/** Mix two hex colours. t=0 returns a, t=1 returns b. */
export const mixHex = (a: string, b: string, t: number) => {
  const ca = parseHex(a);
  const cb = parseHex(b);
  const k = Math.min(1, Math.max(0, t));
  return toHex([ca[0] + (cb[0] - ca[0]) * k, ca[1] + (cb[1] - ca[1]) * k, ca[2] + (cb[2] - ca[2]) * k]);
};

/** amount > 0 lightens toward white, amount < 0 darkens toward black. */
export const shade = (hex: string, amount: number) => {
  const target = amount >= 0 ? '#FFFFFF' : '#000000';
  return mixHex(hex, target, Math.min(1, Math.abs(amount)));
};

export const rgba = (hex: string, alpha: number) => {
  const [r, g, b] = parseHex(hex);
  return `rgba(${r}, ${g}, ${b}, ${Math.min(1, Math.max(0, alpha))})`;
};
