# App Store screenshots

- `6.9-*.png` 1320×2868 (iPhone 6.9"), `6.5-*.png` 1284×2778 (iPhone 6.5").
- Generated from the PWA with seeded Vietnamese content, no real user data:
  1. `node scripts/appstore-ai.mjs` (canned AI on :8902)
  2. run `neko-aurora` with an empty `-state`, `-listen 127.0.0.1:8812`, `AURORA_AI_BASE_URL=http://127.0.0.1:8902`, `ZAI_API_KEY=fake-test-key`
  3. `OUT=screenshots/ SEED=1 node scripts/appstore-shots.mjs` (needs playwright-core + Chrome)
