# connector-ga4-analytics

TypeScript (Node.js) connector toolkit for:
- Google Analytics 4 Data API
- Google Search Console API
- Google Indexing API

This package is extracted from the local OpenClaw GA4 skill and prepared as a standalone repository.

## Features

- GA4 reports (traffic, content, user behavior, realtime, comparisons)
- Search Console reports (queries, pages, devices, countries)
- Indexing operations (request indexing, inspect URL status)
- Auto-save result snapshots in `results/`

## Requirements

- Node.js >= 18
- A Google service account with access to:
  - GA4 property
  - Search Console property
  - Indexing API (when used)

## Quick start

```bash
git clone https://github.com/RA-AI-Internal-Tools/connector-ga4-analytics.git
cd connector-ga4-analytics/scripts
cp ../.env.example .env
npm install
npm test
npx tsx src/index.ts
```

## Environment variables

Configure in `scripts/.env` (or export in shell):

- `GA4_PROPERTY_ID`
- `GA4_CLIENT_EMAIL`
- `GA4_PRIVATE_KEY`
- `SEARCH_CONSOLE_SITE_URL`
- `GA4_DEFAULT_DATE_RANGE` (optional, default `30d`)

## Notes on recent GA4 schema updates

This package is updated to use the modern key-event metric naming:
- `keyEvents` (instead of legacy `conversions`)

## CI

GitHub Actions workflow runs:
- `npm ci`
- `npm test` (TypeScript typecheck)
