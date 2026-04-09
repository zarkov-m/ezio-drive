# Shared Skills

This repository is the team sharing layer for selected OpenClaw skills and connectors.

## Purpose

Use this repo to share reusable OpenClaw components across team-managed machines without exposing the full main workspace.

It is intended for:
- shared connectors
- shared skills
- reusable references
- team-wide operational building blocks

## What belongs here

Good candidates for this repo:
- connectors used by multiple team members or machines
- reusable skills that should stay aligned across environments
- shared setup references needed by several OpenClaw installations

Current included examples:
- `connector-email-hks`
- `connector-figma`
- `connector-ga4-analytics`
- `connector-onedrive`
- `connector-openproject`
- `connector-search-console`

## What should not go here

Do not use this repo for:
- private user context
- personal memory files
- machine-specific secrets
- temporary experiments
- incomplete local-only work
- internal notes that should stay in the main OpenClaw workspace

## How to use it

Typical model:
1. keep active development in the main OpenClaw repo
2. copy or promote only the selected reusable skills/connectors into this repo
3. push this repo to GitHub
4. clone or pull it on the target machines

This keeps sharing selective and controlled.

## Team rule

When a shared connector or skill changes and the change should be available on other machines:
1. update it here
2. commit it
3. push it
4. pull it on the target machines

## Security note

Never commit:
- tokens
- passwords
- `.env` files with secrets
- machine-specific credential caches

Shared means reusable, not sensitive.
