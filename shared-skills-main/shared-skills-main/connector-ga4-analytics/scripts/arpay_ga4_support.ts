import 'dotenv/config';
import { runReport } from './src/api/reports.js';
async function main(){
  const dateRange={startDate:'2026-03-09',endDate:'2026-04-05'};
  const reports:any={};
  reports.landingPages=await runReport({dimensions:['landingPage'],metrics:['sessions','activeUsers','keyEvents','totalRevenue'],dateRange,limit:200,save:false});
  reports.pagePaths=await runReport({dimensions:['pagePath','pageTitle'],metrics:['screenPageViews','activeUsers'],dateRange,limit:200,save:false});
  reports.events=await runReport({dimensions:['eventName'],metrics:['eventCount','keyEvents'],dateRange,limit:50,save:false});
  console.log(JSON.stringify(reports,null,2));
}
main().catch(err=>{console.error(err);process.exit(1)});
