import {Composition} from 'remotion';
import {Launch} from './Launch';
import {VIDEO} from './timeline';
import './fonts';

export const RemotionRoot: React.FC = () => {
  return (
    <Composition
      id="SiphonLaunch"
      component={Launch}
      durationInFrames={VIDEO.durationInFrames}
      fps={VIDEO.fps}
      width={VIDEO.width}
      height={VIDEO.height}
    />
  );
};
