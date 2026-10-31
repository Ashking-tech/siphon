import React from 'react';
import {Composition} from 'remotion';
import {FRAME} from './theme';
import {SiphonProductGif} from './SiphonProductGif';

export const Root: React.FC = () => (
  <Composition
    id="SiphonProductGif"
    component={SiphonProductGif}
    durationInFrames={FRAME.durationInFrames}
    fps={FRAME.fps}
    width={FRAME.width}
    height={FRAME.height}
  />
);
