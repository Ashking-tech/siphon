import {loadFont} from '@remotion/fonts';
import {staticFile} from 'remotion';

for (const font of [
  {file: 'InterDisplay-Regular.ttf', weight: '400'},
  {file: 'InterDisplay-Medium.ttf', weight: '500'},
  {file: 'InterDisplay-Bold.ttf', weight: '700'},
] as const) {
  loadFont({
    family: 'Inter Display',
    url: staticFile(`fonts/${font.file}`),
    weight: font.weight,
    style: 'normal',
  });
}

for (const font of [
  {file: 'NotoSans-Regular.ttf', weight: '400'},
  {file: 'NotoSans-Medium.ttf', weight: '500'},
  {file: 'NotoSans-SemiBold.ttf', weight: '600'},
] as const) {
  loadFont({
    family: 'Noto Sans',
    url: staticFile(`fonts/${font.file}`),
    weight: font.weight,
    style: 'normal',
  });
}

for (const font of [
  {file: 'Hack-Regular.ttf', weight: '400'},
  {file: 'Hack-Bold.ttf', weight: '700'},
] as const) {
  loadFont({
    family: 'Hack',
    url: staticFile(`fonts/${font.file}`),
    weight: font.weight,
    style: 'normal',
  });
}
