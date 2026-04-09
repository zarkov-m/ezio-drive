import React from 'react';
import {Composition} from 'remotion';
import {Scene01, totalFrames} from './Scene01';

export const RemotionRoot: React.FC = () => {
  return (
    <>
      <Composition
        id="Scene01"
        component={Scene01}
        durationInFrames={totalFrames}
        fps={30}
        width={1080}
        height={1920}
      />
    </>
  );
};
