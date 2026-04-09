import React from 'react';
import {AbsoluteFill, Easing, interpolate, spring, useCurrentFrame, useVideoConfig} from 'remotion';

export const Scene01: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();

  const zoom = interpolate(frame, [0, 45], [1.2, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
    easing: Easing.out(Easing.cubic),
  });

  const titleScale = spring({
    frame,
    fps,
    config: {damping: 12, stiffness: 150},
  });

  const countdownOpacity = interpolate(frame, [18, 35], [0, 1], {
    extrapolateLeft: 'clamp',
    extrapolateRight: 'clamp',
  });

  return (
    <AbsoluteFill
      style={{
        background: 'radial-gradient(circle at center, #330000 0%, #120000 45%, #000000 100%)',
        transform: `scale(${zoom})`,
        overflow: 'hidden',
        fontFamily: 'Arial, Helvetica, sans-serif',
      }}
    >
      <AbsoluteFill
        style={{
          justifyContent: 'center',
          alignItems: 'center',
          padding: 80,
          textAlign: 'center',
          color: 'white',
        }}
      >
        <div
          style={{
            fontSize: 118,
            lineHeight: 0.95,
            fontWeight: 900,
            color: '#ff2a2a',
            textTransform: 'uppercase',
            textShadow: '0 0 20px rgba(255,0,0,0.55), 0 0 60px rgba(255,0,0,0.25)',
            transform: `scale(${titleScale})`,
            letterSpacing: -2,
          }}
        >
          GTA V LEAVING
          <br />
          GAME PASS
        </div>

        <div
          style={{
            marginTop: 64,
            fontSize: 92,
            fontWeight: 900,
            opacity: countdownOpacity,
            color: '#ffffff',
            background: 'rgba(255,0,0,0.16)',
            border: '3px solid rgba(255,70,70,0.8)',
            borderRadius: 24,
            padding: '22px 36px',
            boxShadow: '0 0 35px rgba(255,0,0,0.28)',
          }}
        >
          7 DAYS LEFT
        </div>
      </AbsoluteFill>
    </AbsoluteFill>
  );
};
