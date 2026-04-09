---
name: connector-freepik
description: Use Freepik through the website workflow for asset search, AI image generation, AI video generation, prompt reuse, and reference-image based creative work. Use when Mitko wants help operating Freepik from this workspace, especially for finding assets, generating images or videos, maintaining character/environment consistency, or preparing download-ready outputs.
---

# Freepik connector

Use this skill for Mitko's Freepik workflow.

## Default operating mode

Prefer the normal logged-in browser profile on this machine.

Assume Mitko is already logged in to Freepik there unless the browser session shows otherwise. Current browser login identity for Freepik: `a.pavlov@hksglobal.group`.

Use this skill for all of the following:
- search assets
- generate AI images
- generate AI videos
- reuse reference images
- download outputs

Freepik API access is also available locally when `FREEPIK_API_KEY` is present in `.secrets/freepik.env`. Prefer browser workflow for actions tied to the website UI, session state, or interactive generation flow. Prefer API usage later for deterministic automation if scripts are added.

## Core workflow

1. Confirm the goal: search, generate image, generate video, download, or API-backed automation.
2. Use the normal logged-in browser profile by default.
3. If the task is better suited to automation and API coverage is sufficient, load `.secrets/freepik.env` first.
4. Identify the deliverable format.
5. Gather prompts, references, and constraints.
6. Generate the image first when the task will later become a video.
7. Reuse the strongest image or character reference for consistency.
8. Execute the browser or API workflow carefully.
9. Report results briefly and clearly.

## Minimum brief to collect

Collect or infer:
- task type
- final format
- platform or use case
- visual style
- key subject
- prompt text
- reference images, if any
- consistency requirements
- output size or orientation

## Search workflow

When searching assets:
- identify the exact subject or event
- prefer results that match the requested style and usage
- summarize the strongest options briefly
- avoid claiming licenses or permissions unless explicitly verified in Freepik

## AI image workflow

When generating images:
- treat image generation as the first step for video work whenever possible
- lock the subject, style, and framing before generating many variants
- reuse the strongest prompt wording across iterations
- keep track of which prompt variation changed which result
- when references exist, keep fixed identity traits stable
- choose the strongest reference image before moving into video generation

## AI video workflow

When generating videos:
- start from the chosen image reference when possible
- separate fixed visual identity from shot-specific movement
- keep character, wardrobe, environment, and lighting consistent unless asked to change them
- change as few variables as possible between iterations
- reuse character references for consistency
- report the exact prompt or prompt delta used

## Downloads and delivery

When downloading outputs:
- confirm which final file Mitko wants
- preserve filenames when useful
- mention where the file was saved if downloaded locally

## Safety

- Do not publish externally without approval.
- Do not assume licensing details that are not visible in Freepik.
- Do not overwrite local creative files without asking.
- Keep `FREEPIK_API_KEY` in local secret storage only, never commit or paste it back into chat.
- If the logged-in browser session is missing or expired, stop and ask Mitko to log in first.

## References

Read `references/workflow-notes.md` for the standard browser workflow checklist.
Read `references/input-checklist.md` for the minimum info to collect before generation.
