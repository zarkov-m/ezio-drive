# connector-search-console

Standalone TypeScript connector for:
- Google Search Console API
- Google Indexing API

## Features

- Search query/page/device/country performance
- SEO overview and keyword analysis helpers
- URL inspection status checks
- Indexing submission (URL_UPDATED)

## Quick start

```bash
git clone https://github.com/RA-AI-Internal-Tools/connector-search-console.git
cd connector-search-console/scripts
cp ../.env.example .env
npm install
npm test
npx tsx src/index.ts
```

## Environment variables

- `GA4_CLIENT_EMAIL`
- `GA4_PRIVATE_KEY`
- `SEARCH_CONSOLE_SITE_URL`
- `GA4_DEFAULT_DATE_RANGE` (optional)

## CI

GitHub Actions workflow runs `npm ci` and `npm test`.
