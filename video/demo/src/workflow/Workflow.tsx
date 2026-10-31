import React from 'react';
import {AbsoluteFill, Sequence} from 'remotion';
import {Scene01Dolphin} from './scenes/Scene01Dolphin';
import {Scene02Preflight} from './scenes/Scene02Preflight';
import {Scene03Segments} from './scenes/Scene03Segments';
import {Scene04Finalize} from './scenes/Scene04Finalize';
import {SweepTransition} from './components/SweepTransition';
import {SCENES, TRANSITIONS} from './timeline';
import {C} from './theme';

const SCENE_COMPONENTS: Record<string, React.FC> = {
  dolphin: Scene01Dolphin,
  preflight: Scene02Preflight,
  segments: Scene03Segments,
  finalize: Scene04Finalize,
};

/**
 * Siphon — product workflow explainer.
 *
 * 0–7   Dolphin: right-click → Siphon → Convert to MP4 (saves space)
 * 7–11  Preflight: inspect, space, strategy
 * 11–17 Relocate: tail-first segment → process → verify → reclaim
 * 17–22 Finalize: one file in, one file out
 */
export const SiphonWorkflow: React.FC = () => (
  <AbsoluteFill style={{background: C.white, overflow: 'hidden'}}>
    {SCENES.map((scene) => {
      const Scene = SCENE_COMPONENTS[scene.id];
      return (
        <Sequence
          key={scene.id}
          from={scene.from}
          durationInFrames={scene.duration}
          name={`${scene.code}-${scene.id}`}
        >
          <Scene />
        </Sequence>
      );
    })}

    {TRANSITIONS.map((transition) => (
      <SweepTransition
        key={transition.from}
        from={transition.from}
        duration={transition.duration}
        direction={transition.direction}
      />
    ))}
  </AbsoluteFill>
);
