# Neko Aurora: iOS shell (Capacitor 8)

A native iOS wrapper around the Aurora `/app/` PWA. It loads the owner's deployed Aurora host (`server.url`) and adds what a web clip can't do (App Store guideline 4.2):

- **APNs push**: when a bot needs approval or finishes a long task, a notification reaches the lock screen even when the app is closed. Web Push on iOS only works after "Add to Home Screen".
- **Native Taptic haptics**: Safari has no `navigator.vibrate`, so `app.js` uses `@capacitor/haptics` when running in the shell.
- **Offline screen** (`www/offline.html`, `errorPath`) shows when the host can't be reached; the in-app pill shows when the connection drops.
- **App-Bound Domains**: the webview can navigate only to the Aurora host (`WKAppBoundDomains`).

## Build (needs a Mac)
```sh
cd app/ios && npm ci
AURORA_IOS_URL=https://<your-aurora-host>/app/ npm run sync   # writes server.url + WKAppBoundDomains
npm run assets      # icon 1024 (opaque) + splash from the Neko icon (ffmpeg)
npm run open        # Xcode 26+: pick Team, then Product → Archive
```
`cap add ios` has already been run. The project uses Swift Package Manager (no CocoaPods); iOS 15+, iPhone only, portrait.

## APNs plan
1. The JS shell asks for permission after login → `PushNotifications.register()` → the token goes to `POST /app/api/push/apns`. The server stores it in the encrypted vault as `APNS_DEVICE_TOKEN` (done, tested).
2. **Still needed (requires an Apple Developer account):** create an APNs Auth Key (.p8), then set `AURORA_APNS_KEY_ID`, `AURORA_APNS_TEAM_ID`, `AURORA_APNS_TOPIC=online.holilihu.nekoaurora` and the .p8 in the vault (never in env/logs). The server signs an ES256 JWT and POSTs to `api.push.apple.com/3/device/<token>`, with the same triggers as the current Web Push `notify()`.
3. Switch `App.entitlements` `aps-environment` from `development` → `production` for TestFlight/App Store.

## App Store checklist
| Item | Status |
|---|---|
| Native value beyond a website (4.2): APNs, haptics, offline, app-bound | ✅ code, ⏳ APNs sender needs .p8 |
| Privacy manifest `PrivacyInfo.xcprivacy` (UserDefaults CA92.1, no tracking) | ✅ |
| Privacy policy URL `/app/privacy.html`, Terms `/app/terms.html` (public, vi+en) | ✅ |
| Usage strings: microphone, speech recognition | ✅ |
| Export compliance `ITSAppUsesNonExemptEncryption=false` (only HTTPS/standard crypto) | ✅ (re-check when filing) |
| Account deletion (5.1.1(v)) | ℹ️ No account signup (owner code on a self-hosted server). If multi-user signup is added, an in-app delete flow is required |
| Sign in for review: demo host + owner code for App Review notes | ⏳ needs a deploy host |
| Icon 1024 opaque, no alpha | ✅ generated (upscaled from 512; replace with a 1024 master before release) |
| Screenshots 6.9" + 6.5" | ⏳ take from the simulator after deploy |
| Apple Developer Program ($99/yr), bundle id `online.holilihu.nekoaurora`, signing | ⏳ Darren |
| Age rating, category Productivity, support URL | ⏳ in App Store Connect |
