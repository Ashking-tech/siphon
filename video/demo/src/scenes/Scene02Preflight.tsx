import React from 'react';
import {useCurrentFrame} from 'remotion';
import {FileStage} from './FileStage';

/**
 * Scene 02 — 3.0–5.0 s.
 * Same camera and object as Scene 01, now observed by a blue inspection plane.
 */
export const Scene02Preflight: React.FC = () => {
  const frame = useCurrentFrame();
  return <FileStage absFrame={90 + frame} />;
};
