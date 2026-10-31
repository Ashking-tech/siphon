import {AbsoluteFill, Audio, Sequence, staticFile} from 'remotion';
import {Chrome} from './components/Chrome';
import {FilmGrain, Vignette} from './components/FilmGrain';
import {Scene01Problem} from './scenes/Scene01Problem';
import {Scene02Question} from './scenes/Scene02Question';
import {Scene03Reveal} from './scenes/Scene03Reveal';
import {Scene04Engineering} from './scenes/Scene04Engineering';
import {Scene05Payoff} from './scenes/Scene05Payoff';
import {Scene06Capabilities} from './scenes/Scene06Capabilities';
import {Scene07Close} from './scenes/Scene07Close';
import {COLORS} from './theme';
import {SCENES} from './timeline';

const SCENE_COMPONENTS: Record<string, React.FC> = {
  problem: Scene01Problem,
  question: Scene02Question,
  reveal: Scene03Reveal,
  engineering: Scene04Engineering,
  payoff: Scene05Payoff,
  capabilities: Scene06Capabilities,
  close: Scene07Close,
};

export const Launch: React.FC = () => {
  return (
    <AbsoluteFill style={{backgroundColor: COLORS.bg, color: COLORS.ink}}>
      {SCENES.map((scene) => {
        const Scene = SCENE_COMPONENTS[scene.id];
        return (
          <Sequence key={scene.id} from={scene.from} durationInFrames={scene.duration} name={scene.id}>
            <Scene />
          </Sequence>
        );
      })}

      <Vignette />
      <FilmGrain />
      <Chrome />
      <Audio src={staticFile('audio/soundtrack.wav')} />
    </AbsoluteFill>
  );
};
