import React from 'react';
import {AbsoluteFill, Audio, Sequence, staticFile} from 'remotion';
import {FilmGrain, FrameMarks, Vignette} from './components/Atmosphere';
import {SceneTransition} from './components/SceneTransition';
import {Scene01File} from './scenes/Scene01File';
import {Scene02Preflight} from './scenes/Scene02Preflight';
import {Scene03Processing} from './scenes/Scene03Processing';
import {Scene04Finalize} from './scenes/Scene04Finalize';
import {Scene05Close} from './scenes/Scene05Close';
import {COLORS} from './theme';
import {SCENES} from './timeline';

const SCENE_COMPONENTS: Record<string, React.FC> = {
  file: Scene01File,
  preflight: Scene02Preflight,
  processing: Scene03Processing,
  finalize: Scene04Finalize,
  close: Scene05Close,
};

export const SiphonDemo: React.FC = () => {
  return (
    <AbsoluteFill style={{backgroundColor: COLORS.paper, overflow: 'hidden'}}>
      {SCENES.map((scene) => {
        const Scene = SCENE_COMPONENTS[scene.id];
        return (
          <Sequence key={scene.id} from={scene.from} durationInFrames={scene.duration} name={`${scene.code}-${scene.id}`}>
            <Scene />
          </Sequence>
        );
      })}

      {/* The blue inspection plane becomes the wipe into the dark processing stage. */}
      <SceneTransition from={142} duration={16} variant="scan" />
      {/* The verified pipeline becomes the white assembly stage. */}
      <SceneTransition from={352} duration={16} variant="blade" />
      {/* Assembly becomes the end card in a clean white bloom. */}
      <SceneTransition from={472} duration={16} variant="bloom" />

      <Vignette strength={0.14} />
      <FilmGrain opacity={0.042} />
      <FrameMarks opacity={0.12} />
      <Audio src={staticFile('audio/soundtrack.wav')} />
    </AbsoluteFill>
  );
};
