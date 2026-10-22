// Regenerate the iOS app icon (1024, opaque) and splash (2732², 3 scales) from the Wiii/Neko icon.
// Uses ffmpeg (free). Replace ../native/internal/web/assets/app/icon-512.png with a 1024 master when available.
import {execFileSync as x} from 'node:child_process';
const src='../native/internal/web/assets/app/icon-512.png',A='ios/App/App/Assets.xcassets';
const ff=(...a)=>x('ffmpeg',['-y','-loglevel','error',...a]);
ff('-i',src,'-vf','scale=1024:1024:flags=lanczos,format=rgb24','-frames:v','1',`${A}/AppIcon.appiconset/AppIcon-512@2x.png`);
for(const n of ['splash-2732x2732.png','splash-2732x2732-1.png','splash-2732x2732-2.png'])
 ff('-f','lavfi','-i','color=c=0xf7f7f7:s=2732x2732','-i',src,'-filter_complex','[1]scale=360:360:flags=lanczos[i];[0][i]overlay=(W-w)/2:(H-h)/2','-frames:v','1',`${A}/Splash.imageset/${n}`);
console.log('icon + splash written');
