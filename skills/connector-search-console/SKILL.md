---
name: search-console-ops
description: "Google Search Console + Indexing API toolkit. Query top queries/pages/devices/countries, inspect URL index status, and submit reindex requests."
---

# Search Console Toolkit

This toolkit focuses on Google Search Console and Indexing API operations.

## Setup

```bash
cd scripts
npm install
```

Create `scripts/.env` (or export variables):

```env
GA4_CLIENT_EMAIL=service-account@project.iam.gserviceaccount.com
GA4_PRIVATE_KEY="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n"
SEARCH_CONSOLE_SITE_URL=https://example.com
GA4_DEFAULT_DATE_RANGE=30d
```

## Quick usage

```ts
import {
  searchConsoleOverview,
  keywordAnalysis,
  reindexUrls,
  checkIndexStatus,
} from './scripts/src/index.js';

await searchConsoleOverview('30d');
await keywordAnalysis('7d');
await reindexUrls(['https://example.com/page']);
await checkIndexStatus(['https://example.com/page']);
```

## Validation

```bash
cd scripts
npm test
```
