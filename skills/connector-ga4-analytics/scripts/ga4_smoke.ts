import { siteOverview } from './src/index.ts';
(async () => {
  const res = await siteOverview('7d');
  console.log('GA4_SMOKE_OK');
  console.log(JSON.stringify(Object.keys(res)));
})().catch((err) => {
  console.error('GA4_SMOKE_FAIL');
  console.error(err?.stack || err?.message || String(err));
  process.exit(1);
});
