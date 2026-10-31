import {loadFont} from '@remotion/fonts';
import {staticFile} from 'remotion';

/**
 * Fonts are bundled locally so renders are reproducible offline.
 * Inter Display (SIL OFL) + Hack (MIT) are copied into public/fonts.
 */
const interDisplay = [
  {file: 'InterDisplay-Black.ttf', weight: '900'},
  {file: 'InterDisplay-ExtraBold.ttf', weight: '800'},
  {file: 'InterDisplay-Bold.ttf', weight: '700'},
  {file: 'InterDisplay-Medium.ttf', weight: '500'},
  {file: 'InterDisplay-Regular.ttf', weight: '400'},
] as const;

for (const font of interDisplay) {
  loadFont({
    family: 'Inter Display',
    url: staticFile(`fonts/${font.file}`),
    weight: font.weight,
    style: 'normal',
  });
}

loadFont({
  family: 'Hack',
  url: staticFile('fonts/Hack-Regular.ttf'),
  weight: '400',
  style: 'normal',
});

loadFont({
  family: 'Hack',
  url: staticFile('fonts/Hack-Bold.ttf'),
  weight: '700',
  style: 'normal',
});
