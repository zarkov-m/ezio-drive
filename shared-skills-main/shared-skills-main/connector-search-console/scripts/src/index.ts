/**
 * Search Console Toolkit - Main Entry Point
 */

export * from './api/searchConsole.js';
export * from './api/indexing.js';
export { getSearchConsoleClient, getIndexingClient, getSiteUrl, resetClient } from './core/client.js';
export { saveResult, loadResult, listResults, getLatestResult } from './core/storage.js';
export { getSettings, validateSettings } from './config/settings.js';

import {
  getTopQueries,
  getTopPages as getSearchConsoleTopPages,
  getDevicePerformance,
  getCountryPerformance,
  type SearchConsoleDateRange,
} from './api/searchConsole.js';
import { requestIndexing, inspectUrl } from './api/indexing.js';
import { saveResult } from './core/storage.js';

export async function searchConsoleOverview(dateRange?: string | SearchConsoleDateRange) {
  console.log('\n🔍 Generating Search Console overview...');
  const results: Record<string, unknown> = {};

  console.log('  → Getting top queries...');
  results.topQueries = await getTopQueries(dateRange);

  console.log('  → Getting top pages...');
  results.topPages = await getSearchConsoleTopPages(dateRange);

  console.log('  → Getting device performance...');
  results.devicePerformance = await getDevicePerformance(dateRange);

  console.log('  → Getting country performance...');
  results.countryPerformance = await getCountryPerformance(dateRange);

  const dateStr = typeof dateRange === 'string' ? dateRange : 'custom';
  saveResult(results, 'searchconsole', 'overview', dateStr);

  console.log('✅ Search Console overview complete\n');
  return results;
}

export async function keywordAnalysis(dateRange?: string | SearchConsoleDateRange) {
  console.log('\n🔑 Analyzing keywords...');
  const results: Record<string, unknown> = {};

  results.queries = await getTopQueries(dateRange);
  results.deviceBreakdown = await getDevicePerformance(dateRange);

  const dateStr = typeof dateRange === 'string' ? dateRange : 'custom';
  saveResult(results, 'searchconsole', 'keyword_analysis', dateStr);

  console.log('✅ Keyword analysis complete\n');
  return results;
}

export async function seoPagePerformance(dateRange?: string | SearchConsoleDateRange) {
  console.log('\n📄 Analyzing SEO page performance...');
  const results: Record<string, unknown> = {};

  results.topPages = await getSearchConsoleTopPages(dateRange);
  results.countryBreakdown = await getCountryPerformance(dateRange);

  const dateStr = typeof dateRange === 'string' ? dateRange : 'custom';
  saveResult(results, 'searchconsole', 'seo_page_performance', dateStr);

  console.log('✅ SEO page performance analysis complete\n');
  return results;
}

export async function reindexUrls(urls: string[]) {
  console.log(`\n🔄 Requesting re-indexing for ${urls.length} URL(s)...`);
  const results: Array<{ url: string; status: string; error?: string; notifyTime?: string; type?: string }> = [];

  for (const url of urls) {
    try {
      const result = await requestIndexing(url, { save: false });
      results.push({ status: 'submitted', ...result });
    } catch (error) {
      results.push({ url, status: 'failed', error: String(error) });
    }
  }

  saveResult(results, 'indexing', 'reindex_batch');
  console.log('✅ Re-indexing requests complete\n');
  return results;
}

export async function checkIndexStatus(urls: string[]) {
  console.log(`\n🔎 Checking index status for ${urls.length} URL(s)...`);
  const results: Array<{ url: string; indexed: boolean; status: unknown }> = [];

  for (const url of urls) {
    try {
      const result = await inspectUrl(url, { save: false });
      results.push({
        url,
        indexed: result.indexStatus.verdict === 'PASS',
        status: result.indexStatus,
      });
    } catch (error) {
      results.push({ url, indexed: false, status: { error: String(error) } });
    }
  }

  saveResult(results, 'indexing', 'index_status_check');
  console.log('✅ Index status check complete\n');
  return results;
}

if (process.argv[1] === new URL(import.meta.url).pathname) {
  console.log(`
Search Console Toolkit
======================

Functions:
  - searchConsoleOverview(dateRange?)
  - keywordAnalysis(dateRange?)
  - seoPagePerformance(dateRange?)
  - getTopQueries(dateRange?)
  - getTopPages(dateRange?)
  - getDevicePerformance(dateRange?)
  - getCountryPerformance(dateRange?)
  - reindexUrls(urls)
  - checkIndexStatus(urls)
`);
}
