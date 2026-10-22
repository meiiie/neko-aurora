// Tests must never touch the live server (8765): it holds Darren's real bots and history.
export const LIVE_PORT = 8765;
export function notLive(url) {
  for (const u of [url, process.env.AURORA_URL, process.env.URL].filter(Boolean)) {
    if (new URL(u).port === String(LIVE_PORT)) { console.error(`refusing to run against live port ${LIVE_PORT}: ${u}`); process.exit(2); }
  }
  return url;
}
