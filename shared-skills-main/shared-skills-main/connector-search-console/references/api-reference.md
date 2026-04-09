# connector-search-console API reference

## High-level functions

- `searchConsoleOverview(dateRange?)`
- `keywordAnalysis(dateRange?)`
- `seoPagePerformance(dateRange?)`
- `reindexUrls(urls: string[])`
- `checkIndexStatus(urls: string[])`

## Low-level Search Console functions

- `querySearchAnalytics({ dimensions, dateRange, rowLimit, startRow })`
- `getTopQueries(dateRange?)`
- `getTopPages(dateRange?)`
- `getDevicePerformance(dateRange?)`
- `getCountryPerformance(dateRange?)`
- `getSearchAppearance(dateRange?)`

## Low-level Indexing functions

- `requestIndexing(url)`
- `requestIndexingBatch(urls)`
- `removeFromIndex(url)`
- `inspectUrl(url)`

## Date range format

- Shorthand: `"7d"`, `"30d"`, `"90d"`
- Explicit: `{ startDate: "YYYY-MM-DD", endDate: "YYYY-MM-DD" }`
