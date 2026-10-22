// Point the shell at the deployed Aurora host: AURORA_IOS_URL=https://aurora.your-host/app/ npm run sync
// Also registers the host as the only App-Bound Domain (WKAppBoundDomains), so the webview can't wander.
import fs from 'node:fs';
const url=process.env.AURORA_IOS_URL;if(!url){console.log('AURORA_IOS_URL not set; keeping placeholder');process.exit(0)}
const u=new URL(url);if(u.protocol!=='https:')throw new Error('AURORA_IOS_URL must be https');
const c=JSON.parse(fs.readFileSync('capacitor.config.json','utf8'));c.server.url=u.href;fs.writeFileSync('capacitor.config.json',JSON.stringify(c,null,2)+'\n');
const p='ios/App/App/Info.plist';let s=fs.readFileSync(p,'utf8');
s=s.replace(/<key>WKAppBoundDomains<\/key>\s*<array>[\s\S]*?<\/array>/,`<key>WKAppBoundDomains</key>\n\t<array>\n\t\t<string>${u.host}</string>\n\t</array>`);fs.writeFileSync(p,s);
console.log('server.url →',u.href);
