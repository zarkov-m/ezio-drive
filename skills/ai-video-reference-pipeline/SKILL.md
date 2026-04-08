---
name: ai-video-reference-pipeline
description: Build consistent AI video generation plans using image references for characters, scenes, props, and mood continuity. Use when preparing prompts, shot lists, reference packs, consistency rules, scene-to-scene variations, or generator-ready instructions for AI image and AI video workflows such as Freepik-based pipelines.
---

# AI Video Reference Pipeline

Create repeatable consistency before generating shots.

## Workflow

1. Define the final video goal.
2. Define the main character, environment, props, and mood.
3. Lock the visual constants.
4. Separate constants from per-shot changes.
5. Build shot prompts.
6. Review for continuity breaks.

## Capture the minimum brief

Collect or infer:
- video purpose
- duration or shot count
- format
- main subject
- environment
- style
- mood
- camera feel
- must-keep elements

## Build a consistency pack

Always separate the brief into:
- fixed identity traits
- fixed environment traits
- recurring props
- forbidden changes
- allowed shot variation

## Prompting rule

Write prompts in two layers:
1. base reference prompt, reused across the full sequence
2. shot prompt, changing only action, framing, camera, or motion

This reduces drift.

## Output formats

Return one of:
- consistency brief
- reference pack structure
- shot list
- base prompt plus shot prompts
- continuity review notes

## References

Read `references/consistency-template.md` for the default structure.
Read `references/shot-planning.md` when building multi-shot sequences.
