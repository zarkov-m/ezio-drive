import 'dotenv/config';
import { getTopQueries, getTopPages, getDevicePerformance, getCountryPerformance } from './src/api/searchConsole.js';
async function main(){
  const current={startDate:'2026-03-09',endDate:'2026-04-05'};
  const previous={startDate:'2026-02-09',endDate:'2026-03-08'};
  const out:any={};
  for (const [label,range] of Object.entries({current,previous})) {
    out[label]={
      topQueries: await getTopQueries(range as any),
      topPages: await getTopPages(range as any),
      devicePerformance: await getDevicePerformance(range as any),
      countryPerformance: await getCountryPerformance(range as any),
    };
  }
  console.log(JSON.stringify(out,null,2));
}
main().catch(err=>{console.error(err);process.exit(1)});
