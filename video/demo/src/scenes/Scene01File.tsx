import React from 'react';
import {useCurrentFrame} from 'remotion';
import {FileStage} from './FileStage';

/**
 * Scene 01 — 0.0–3.0 s.
 * The source object enters, settles, and is named. The stage is shared with
 * Scene 02, so the cut at 3.0 s is invisible: only the annotation changes.
 */
export const Scene01File: React.FC = () => {
  const frame = useCurrentFrame();
  return <FileStage absFrame={frame} />;
};
