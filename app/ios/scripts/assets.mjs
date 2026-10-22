// Regenerate the iOS app icon (1024, opaque) and splash (2732², 3 scales) from the Wiii/Neko icon.
// Uses ffmpeg (free).
import {execFileSync as x} from 'node:child_process';
const src='brand/icon-1024.png',A='ios/App/App/Assets.xcassets'; // 1024 master from the 1254 px Wiii mascot (brand/make-icon-1024.py)
const ff=(...a)=>x('ffmpeg',['-y','-loglevel','error',...a]);
ff('-i',src,'-vf','format=rgb24','-frames:v','1',`${A}/AppIcon.appiconset/AppIcon-512@2x.png`);
for(const n of ['splash-2732x2732.png','splash-2732x2732-1.png','splash-2732x2732-2.png'])
 ff('-f','lavfi','-i','color=c=0xf7f7f7:s=2732x2732','-i',src,'-filter_complex','[1]scale=360:360:flags=lanczos[i];[0][i]overlay=(W-w)/2:(H-h)/2','-frames:v','1',`${A}/Splash.imageset/${n}`);
console.log('icon + splash written');
