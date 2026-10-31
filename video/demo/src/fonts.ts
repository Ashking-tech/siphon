import {loadFont} from '@remotion/fonts';
import {staticFile} from 'remotion';

/**
 * Fonts are bundled locally for reproducible offline renders.
 * Inter Display — SIL Open Font License.
 * Hack — MIT License.
 */
const display = [
  {file: 'InterDisplay-Black.ttf', weight: '900'},
  {file: 'InterDisplay-ExtraBold.ttf', weight: '800'},
  {file: 'InterDisplay-Bold.ttf', weight: '700'},
  {file: 'InterDisplay-Medium.ttf', weight: '500'},
  {file: 'InterDisplay-Regular.ttf', weight: '400'},
] as const;

for (const font of display) {
  loadFont({
    family: 'Inter Display',
    url: staticFile(`fonts/${font.file}`),
    weight: font.weight,
    style: 'normal',
  });
}

for (const weight of ['400', '700'] as const) {
  loadFont({
    family: 'Hack',
    url: staticFile(`fonts/Hack-${weight === '700' ? 'Bold' : 'Regular'}.ttf`),
    weight,
    style: 'normal',
  });
}

/** KDE's default UI face. Bundled so the Dolphin recreation renders offline. */
const noto = [
  {file: 'NotoSans-Regular.ttf', weight: '400'},
  {file: 'NotoSans-Medium.ttf', weight: '500'},
  {file: 'NotoSans-SemiBold.ttf', weight: '600'},
  {file: 'NotoSans-Bold.ttf', weight: '700'},
] as const;

for (const font of noto) {
  loadFont({
    family: 'Noto Sans',
    url: staticFile(`fonts/${font.file}`),
    weight: font.weight,
    style: 'normal',
  });
}
