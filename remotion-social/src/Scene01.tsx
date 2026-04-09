import React from 'react';
import {AbsoluteFill, Audio, Easing, interpolate, random, spring, staticFile, useCurrentFrame, useVideoConfig} from 'remotion';

const SEGMENTS = [
  {frames: 60, voice: 'GTA V is leaving Game Pass in 7 days.'},
  {frames: 120, voice: 'April 15 — it’s gone. All tiers. Cloud, console, PC.'},
  {frames: 90, voice: 'And with GTA 6 coming, it’s not coming back.'},
  {frames: 90, voice: 'Cheapest move: Xbox gift card — get it up to 20% off.'},
  {frames: 90, voice: 'Takes 30 seconds. Code is instant. Link in bio.'},
] as const;

const TOTAL_FRAMES = SEGMENTS.reduce((sum, s) => sum + s.frames, 0);
const BG = 'radial-gradient(circle at 50% 20%, rgba(255,0,0,0.16) 0%, rgba(0,0,0,0.3) 30%, #050505 65%, #000 100%)';

function sceneAtFrame(frame: number) {
  let acc = 0;
  for (let i = 0; i < SEGMENTS.length; i++) {
    const seg = SEGMENTS[i];
    if (frame < acc + seg.frames) {
      return {index: i, local: frame - acc, frames: seg.frames};
    }
    acc += seg.frames;
  }
  return {index: SEGMENTS.length - 1, local: SEGMENTS[SEGMENTS.length - 1].frames - 1, frames: SEGMENTS[SEGMENTS.length - 1].frames};
}

function Noise() {
  const frame = useCurrentFrame();
  return (
    <>
      {Array.from({length: 36}).map((_, i) => {
        const left = (i * 17 + (frame * 3) % 100) % 100;
        const top = (i * 29 + (frame * 7) % 100) % 100;
        const opacity = 0.025 + ((i * 13) % 8) / 100;
        return (
          <div
            key={i}
            style={{
              position: 'absolute',
              left: `${left}%`,
              top: `${top}%`,
              width: 2,
              height: 2,
              background: 'white',
              opacity,
            }}
          />
        );
      })}
    </>
  );
}

function ExactText({lines, size = 86, color = '#fff'}: {lines: string[]; size?: number; color?: string}) {
  return (
    <div
      style={{
        fontWeight: 900,
        textAlign: 'center',
        textTransform: 'uppercase',
        color,
        fontSize: size,
        lineHeight: 1.02,
        letterSpacing: -1,
        whiteSpace: 'pre-line',
      }}
    >
      {lines.map((line, idx) => (
        <div key={idx}>{line}</div>
      ))}
    </div>
  );
}

function GlitchTitle({text, size = 92}: {text: string; size?: number}) {
  const frame = useCurrentFrame();
  const shake = interpolate(frame % 6, [0, 1, 2, 3, 4, 5], [0, 2, -2, 1, -1, 0]);
  const offset = Math.sin(frame * 0.9) * 2;
  return (
    <div style={{position: 'relative', textAlign: 'center'}}>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          transform: `translate(${shake}px, 0px)`,
          color: 'rgba(255,0,0,0.7)',
          fontWeight: 900,
          fontSize: size,
          letterSpacing: -1,
          textTransform: 'uppercase',
          opacity: 0.8,
        }}
      >
        {text}
      </div>
      <div
        style={{
          position: 'absolute',
          inset: 0,
          transform: `translate(${-shake}px, ${offset}px)`,
          color: 'rgba(255,255,255,0.75)',
          fontWeight: 900,
          fontSize: size,
          letterSpacing: -1,
          textTransform: 'uppercase',
          mixBlendMode: 'screen',
          opacity: 0.55,
        }}
      >
        {text}
      </div>
      <div
        style={{
          position: 'relative',
          color: '#ff2a2a',
          textShadow: '0 0 20px rgba(255,0,0,0.75), 0 0 50px rgba(255,0,0,0.25)',
          fontWeight: 900,
          fontSize: size,
          letterSpacing: -1,
          textTransform: 'uppercase',
          lineHeight: 0.95,
        }}
      >
        {text}
      </div>
    </div>
  );
}

function BrokenCard() {
  return (
    <div
      style={{
        width: '82%',
        height: 420,
        borderRadius: 30,
        background: 'linear-gradient(180deg, rgba(18,18,18,0.96), rgba(34,0,0,0.94))',
        border: '2px solid rgba(255,0,0,0.45)',
        boxShadow: '0 0 60px rgba(255,0,0,0.22)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          inset: 24,
          borderRadius: 22,
          border: '1px solid rgba(255,255,255,0.08)',
        }}
      />
      <div style={{fontSize: 56, color: '#888', fontWeight: 800, opacity: 0.45}}>GAME PASS UI</div>
      <div
        style={{
          position: 'absolute',
          width: '78%',
          height: 20,
          borderRadius: 999,
          background: 'rgba(255,255,255,0.07)',
          top: 110,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: '78%',
          height: 20,
          borderRadius: 999,
          background: 'rgba(255,255,255,0.07)',
          top: 150,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: '78%',
          height: 20,
          borderRadius: 999,
          background: 'rgba(255,255,255,0.07)',
          top: 190,
        }}
      />
      <div
        style={{
          position: 'absolute',
          width: '86%',
          height: 220,
          borderRadius: 24,
          background: 'linear-gradient(135deg, rgba(255,255,255,0.08), rgba(255,0,0,0.12))',
          transform: 'rotate(-10deg)',
          border: '8px solid rgba(255,0,0,0.7)',
          opacity: 0.8,
        }}
      />
      <div style={{position: 'absolute', fontSize: 170, color: 'rgba(255,0,0,0.92)', fontWeight: 900}}>X</div>
    </div>
  );
}

function NeonCity() {
  const frame = useCurrentFrame();
  const drift = interpolate(frame, [0, 60], [0, -120], {extrapolateRight: 'clamp'});
  return (
    <div
      style={{
        width: '100%',
        height: '100%',
        background:
          'linear-gradient(180deg, rgba(0,0,0,0.75), rgba(10,0,10,0.8)), radial-gradient(circle at 50% 20%, rgba(255,0,0,0.14), transparent 30%)',
        position: 'relative',
        overflow: 'hidden',
      }}
    >
      {Array.from({length: 18}).map((_, i) => (
        <div
          key={i}
          style={{
            position: 'absolute',
            bottom: `${(i % 4) * 12}%`,
            left: `${(i * 11) % 100}%`,
            width: 64 + ((i * 7) % 80),
            height: 200 + ((i * 13) % 260),
            background: i % 2 ? 'linear-gradient(180deg, rgba(255,0,0,0.14), rgba(255,0,0,0.04))' : 'linear-gradient(180deg, rgba(255,255,255,0.08), rgba(255,0,0,0.12))',
            opacity: 0.65,
            transform: `translateY(${drift * (i % 3 === 0 ? 0.15 : 0.07)}px)`,
            boxShadow: '0 0 40px rgba(255,0,0,0.08)',
          }}
        />
      ))}
      <div
        style={{
          position: 'absolute',
          inset: 0,
          background:
            'linear-gradient(90deg, rgba(255,0,0,0.0) 0%, rgba(255,0,0,0.08) 45%, rgba(255,0,0,0.0) 100%)',
          mixBlendMode: 'screen',
          opacity: 0.6,
        }}
      />
    </div>
  );
}

function SplitSolution() {
  return (
    <div style={{display: 'flex', width: '100%', height: '100%'}}>
      <div
        style={{
          flex: 1,
          background: 'linear-gradient(180deg, rgba(32,0,0,0.95), rgba(6,6,6,0.98))',
          borderRight: '2px solid rgba(255,255,255,0.08)',
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        <div style={{position: 'absolute', top: 56, left: 40, color: '#7efc7e', fontSize: 80, fontWeight: 900}}>-20%</div>
        <div style={{position: 'absolute', bottom: 110, left: 36, right: 36, color: '#fff', fontSize: 64, fontWeight: 900, lineHeight: 1.05, textTransform: 'uppercase'}}>
          BUY XBOX CARD ✓
          <br />
          LOAD ACCOUNT ✓
          <br />
          BUY GTA V ✓
        </div>
      </div>
      <div
        style={{
          flex: 1,
          background: 'linear-gradient(180deg, rgba(0,0,0,0.95), rgba(20,0,0,0.95))',
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            position: 'absolute',
            top: 120,
            left: 60,
            right: 60,
            height: 260,
            borderRadius: 32,
            border: '2px solid rgba(0,255,255,0.22)',
            background: 'linear-gradient(135deg, rgba(80,255,255,0.14), rgba(255,255,255,0.06))',
            boxShadow: '0 0 40px rgba(0,255,255,0.15)',
          }}
        >
          <div style={{position: 'absolute', top: 24, left: 24, width: 150, height: 150, borderRadius: 24, border: '2px solid rgba(255,255,255,0.3)', boxShadow: '0 0 30px rgba(255,255,255,0.12)'}} />
          <div style={{position: 'absolute', top: 30, right: 26, color: '#fff', fontSize: 40, fontWeight: 800}}>GIFT CARD</div>
          <div style={{position: 'absolute', bottom: 24, left: 24, color: '#8df', fontSize: 30, fontWeight: 700}}>GLOWING EDGES</div>
        </div>
      </div>
    </div>
  );
}

function PhoneCTA() {
  return (
    <div
      style={{
        width: '100%',
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        position: 'relative',
        background: 'linear-gradient(180deg, rgba(0,0,0,0.94), rgba(30,0,0,0.96))',
      }}
    >
      <div
        style={{
          width: 320,
          height: 620,
          borderRadius: 42,
          background: 'linear-gradient(180deg, rgba(16,16,16,1), rgba(6,6,6,1))',
          border: '4px solid rgba(255,255,255,0.18)',
          boxShadow: '0 0 60px rgba(255,0,0,0.16)',
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        <div style={{position: 'absolute', inset: 22, borderRadius: 28, background: 'linear-gradient(180deg, rgba(0,220,255,0.14), rgba(255,0,0,0.08))'}} />
        <div style={{position: 'absolute', bottom: 18, left: 50, right: 50, height: 8, borderRadius: 999, background: 'rgba(255,255,255,0.3)'}} />
        <div style={{position: 'absolute', top: 120, left: 40, right: 40, color: '#fff', fontSize: 52, fontWeight: 900, textAlign: 'center'}}>BUY</div>
      </div>
      <div
        style={{
          position: 'absolute',
          bottom: 140,
          width: '100%',
          textAlign: 'center',
          color: '#fff',
        }}
      >
        <div style={{fontSize: 80, fontWeight: 900, textTransform: 'uppercase'}}>CODE IN 30 SECONDS</div>
        <div style={{fontSize: 92, fontWeight: 900, color: '#ff2a2a', textTransform: 'uppercase', marginTop: 12}}>LINK IN BIO</div>
      </div>
    </div>
  );
}

export const Scene01: React.FC = () => {
  const frame = useCurrentFrame();
  const {fps} = useVideoConfig();
  const {index, local} = sceneAtFrame(frame);

  const titlePulse = spring({frame: local, fps, config: {damping: 11, stiffness: 150}});
  const introFade = interpolate(local, [48, 59], [1, 0], {extrapolateRight: 'clamp'});
  const vignette = interpolate(Math.sin(frame * 0.15), [-1, 1], [0.2, 0.36]);

  const scenes = [
    <AbsoluteFill key="s1">
      <AbsoluteFill style={{background: BG}} />
      <Noise />
      <div style={{position: 'absolute', inset: 0, background: 'linear-gradient(180deg, transparent, rgba(0,0,0,0.65))'}} />
      <div style={{display: 'flex', height: '100%', alignItems: 'center', justifyContent: 'center', padding: 40}}>
        <div style={{transform: `scale(${0.9 + titlePulse * 0.12})`, opacity: introFade}}>
          <GlitchTitle text="GTA V LEAVING GAME PASS" size={104} />
          <div style={{height: 26}} />
          <div style={{display: 'flex', justifyContent: 'center'}}>
            <div style={{background: 'rgba(255,0,0,0.17)', border: '3px solid rgba(255,60,60,0.75)', borderRadius: 24, padding: '18px 28px', boxShadow: '0 0 30px rgba(255,0,0,0.35)'}}>
              <ExactText lines={['7 DAYS LEFT']} size={84} />
            </div>
          </div>
        </div>
      </div>
    </AbsoluteFill>,
    <AbsoluteFill key="s2">
      <AbsoluteFill style={{background: BG}} />
      <Noise />
      <div style={{position: 'absolute', inset: 0, opacity: 0.85}}>
        <BrokenCard />
      </div>
      <div style={{position: 'absolute', bottom: 0, left: 0, right: 0, height: '52%'}}>
        <NeonCity />
      </div>
      <div style={{position: 'absolute', inset: 0, background: 'linear-gradient(180deg, rgba(0,0,0,0.12), rgba(0,0,0,0.7))'}} />
      <div style={{position: 'absolute', top: 70, width: '100%'}}>
        <ExactText lines={['APRIL 15 — ALL TIERS', 'CLOUD ✗ CONSOLE ✗ PC ✗']} size={68} />
      </div>
    </AbsoluteFill>,
    <AbsoluteFill key="s3">
      <AbsoluteFill style={{background: 'linear-gradient(180deg, #030303, #100000 60%, #000)'}} />
      <Noise />
      <NeonCity />
      <div style={{position: 'absolute', inset: 0, background: 'linear-gradient(180deg, transparent, rgba(0,0,0,0.78))'}} />
      <div style={{position: 'absolute', top: 190, width: '100%'}}>
        <ExactText lines={['GTA 6 IS COMING', 'NOT COMING BACK']} size={76} />
      </div>
    </AbsoluteFill>,
    <AbsoluteFill key="s4">
      <AbsoluteFill style={{background: BG}} />
      <Noise />
      <SplitSolution />
      <div style={{position: 'absolute', top: 46, left: 40, color: '#7efc7e', fontSize: 86, fontWeight: 900}}>-20%</div>
    </AbsoluteFill>,
    <AbsoluteFill key="s5">
      <AbsoluteFill style={{background: BG}} />
      <Noise />
      <PhoneCTA />
    </AbsoluteFill>,
  ];

  return (
    <AbsoluteFill style={{background: '#000'}}>
      {scenes[index]}
      <AbsoluteFill style={{pointerEvents: 'none', opacity: vignette, boxShadow: 'inset 0 0 180px rgba(0,0,0,0.65)'}} />
      <Audio src={staticFile('voiceover.mp3')} />
    </AbsoluteFill>
  );
};

export const totalFrames = TOTAL_FRAMES;
