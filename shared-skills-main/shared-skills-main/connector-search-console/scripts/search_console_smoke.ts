import { searchConsoleOverview } from './src/index.ts';
(async () => {
  const res = await searchConsoleOverview('30d');
  console.log('SEARCH_CONSOLE_SMOKE_OK');
  console.log(JSON.stringify(Object.keys(res)));
})().catch((err) => {
  console.error('SEARCH_CONSOLE_SMOKE_FAIL');
  console.error(err?.stack || err?.message || String(err));
  process.exit(1);
});
