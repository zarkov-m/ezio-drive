---
name: connector-claude-remotion
description: Use Claude for ideation, scripting, shot planning, or prompt drafting, then use Remotion for structured video assembly and rendering. Use when Mitko wants a Claude-plus-Remotion workflow for short-form videos, scene-by-scene builds, captions, motion graphics planning, or script-to-video assembly.
---

# Claude + Remotion connector

Use this skill for a two-part workflow:
- Claude for planning, copy, scene structure, and creative iteration
- Remotion for actual video composition and rendering

## Current machine status

- Node and npm are available.
- Remotion can be installed locally in a project workspace.
- Claude CLI is not installed on this machine yet.

## Recommended operating mode

1. Use Claude-style prompting or ACP Claude session for:
- scene writing
- hook/caption generation
- pacing
- CTA refinement
- shot-by-shot structure

2. Use Remotion for:
- reusable video templates
- scene timing
- typography animation
- captions
- export/render

## What this skill supports right now

- defining a Claude-to-Remotion workflow
- scaffolding a Remotion project
- turning scene plans into structured Remotion composition notes
- preparing assets, prompts, and timing per scene

## What still requires setup

- Claude CLI installation and auth, if Mitko wants local `claude` command usage
- or an ACP Claude session path instead of local CLI
- local Remotion package install inside a project folder before rendering

## Practical workflow

1. Define the video goal.
2. Break it into scenes.
3. Generate or collect assets.
4. Write scene text and timing.
5. Build the composition in Remotion.
6. Render and revise.

## Safety

- Do not claim Claude local execution unless Claude CLI is actually installed and authenticated.
- Do not claim a Remotion render until dependencies are installed and a render succeeds.
- Keep API keys and auth tokens out of git.

## References

Read `references/workflow.md` for the scene-by-scene pipeline.
Read `references/setup.md` for the current setup constraints and next steps.
