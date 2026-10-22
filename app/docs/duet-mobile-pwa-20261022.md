# Neko Duet: mobile PWA (/duet/)

A mobile-first, installable web surface served by the Go control plane at `/duet/`. It covers the "proactive agent" flow: you hand off work by message, the agents keep working, and nothing goes out without your OK.

**Status: UI prototype.** Agent data and replies are simulated locally (localStorage). It is not wired to `/v1/*` yet. Approving an item only records the decision; nothing is sent anywhere.

## What it covers
- **Agents list:** pixel avatars, status snippet, timestamps, tags, and "N đang chờ bạn". Rows waiting on you sort first. Includes search.
- **Morning brief card:** lists what needs you, with links into each thread.
- **Thread:** messages, a steer/hand-off composer, and approval cards (Duyệt / Để sau). A request that goes outbound (send, post, pay) always produces an approval card first.
- **"Đang làm gì" activity sheet:** what the agents did and are doing.
- **New-agent creator:** 16×16 pixel character drawn in code (10 shell colours; hats, eyewear and extras, 5 options each), name, description, "Ngẫu nhiên", Create. All original art; no third-party logos.

## Mobile/iOS details
- `viewport-fit=cover` with `env(safe-area-inset-*)` and `100dvh`.
- Touch targets are at least 44px, with no horizontal overflow.
- Inputs use 16px text so iOS does not zoom on focus.
- `touch-action: manipulation`.
- Gestures:
  - iOS-style edge swipe back;
  - swipe a row left to pin or archive;
  - drag a sheet down to dismiss.
- Guards against ghost clicks and double taps.
- History-safe back that never navigates out of the app on rapid back.
- Web manifest (standalone) with Apple meta tags and a service worker that caches the app shell.
- `prefers-reduced-motion` is respected.

## Test
`app/tests/duet/mobile-touch.mjs` runs headless Chrome touch emulation at 360, 390 and 430 widths. It checks:
- fast repeated taps;
- row swipe;
- vertical flicks;
- edge-swipe back;
- 10 rapid open/back cycles;
- overflow and 44px targets;
- frame timing.

Last run: 61/61 PASS.
