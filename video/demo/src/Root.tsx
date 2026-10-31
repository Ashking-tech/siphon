import React from 'react';
import {Composition} from 'remotion';
import {SiphonDemo} from './Demo';
import {SiphonWorkflow} from './workflow/Workflow';
import {VIDEO} from './timeline';
import {WORKFLOW} from './workflow/timeline';
import './fonts';

export const RemotionRoot: React.FC = () => (
  <>
    <Composition
      id="SiphonWorkflow"
      component={SiphonWorkflow}
      durationInFrames={WORKFLOW.durationInFrames}
      fps={WORKFLOW.fps}
      width={WORKFLOW.width}
      height={WORKFLOW.height}
    />
    <Composition
      id="SiphonDemo"
      component={SiphonDemo}
      durationInFrames={VIDEO.durationInFrames}
      fps={VIDEO.fps}
      width={VIDEO.width}
      height={VIDEO.height}
    />
  </>
);
