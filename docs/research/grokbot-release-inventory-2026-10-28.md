# Grok Bot release inventory — 2026-10-28

Source: [official changelog](https://x.ai/changelog/bot). The captured page contains 55 release sections and 820 change bullets, spanning 0.17.0–0.77.0. Bullets are not independent features.

The [machine-readable index](grokbot-adoption-index-2026-10-28.json) gives each bullet a release/section/ordinal, source line, SHA256 and routing tags. It deliberately omits the original prose. Source snapshot SHA256: `2867c15909b70226035a2513e38c8905abf91f5849bc4f0b3d8179354c472e4d`.

Coverage is conservative: fourteen entries have partial local validation, one behavior is intentionally not adopted, and 805 entries have no feature-specific implementation evidence recorded in these changes. The connector entries cover only Aurora's remote MCP/OAuth lifecycle fixtures, not live provider integrations. The new attachment entry covers local draft preservation and upload-failure feedback. The question entry now also has local evidence for multi-step cards and durable structured-answer receipts; it was already counted, so the coverage totals do not increase. Existing Aurora capabilities are not automatically credited. See the [implementation and remaining work](../grokbot-adoption-2026-10-28.md).

| Release | Source bullets | Validation |
|---|---:|---|
| [0.77.0](https://x.ai/changelog/bot#v0.77.0) | 133 | 8 partially validated entries |
| [0.75.8](https://x.ai/changelog/bot#v0.75.8) | 19 | No parity claim |
| [0.75.5](https://x.ai/changelog/bot#v0.75.5) | 25 | No parity claim |
| [0.75.2](https://x.ai/changelog/bot#v0.75.2) | 77 | 2 partially validated entries |
| [0.71.2](https://x.ai/changelog/bot#v0.71.2) | 102 | 1 partially validated entry |
| [0.70.2](https://x.ai/changelog/bot#v0.70.2) | 67 | 2 partially validated entries |
| [0.68.1](https://x.ai/changelog/bot#v0.68.1) | 16 | No parity claim |
| [0.68.0](https://x.ai/changelog/bot#v0.68.0) | 15 | 1 partially validated entry |
| [0.67.0](https://x.ai/changelog/bot#v0.67.0) | 15 | No parity claim |
| [0.66.0](https://x.ai/changelog/bot#v0.66.0) | 5 | No parity claim |
| [0.65.0](https://x.ai/changelog/bot#v0.65.0) | 5 | No parity claim |
| [0.64.0](https://x.ai/changelog/bot#v0.64.0) | 5 | No parity claim |
| [0.63.0](https://x.ai/changelog/bot#v0.63.0) | 14 | No parity claim |
| [0.62.0](https://x.ai/changelog/bot#v0.62.0) | 15 | No parity claim |
| [0.61.0](https://x.ai/changelog/bot#v0.61.0) | 11 | No parity claim |
| [0.60.0](https://x.ai/changelog/bot#v0.60.0) | 10 | No parity claim |
| [0.59.0](https://x.ai/changelog/bot#v0.59.0) | 11 | No parity claim |
| [0.58.0](https://x.ai/changelog/bot#v0.58.0) | 14 | No parity claim |
| [0.57.0](https://x.ai/changelog/bot#v0.57.0) | 11 | No parity claim |
| [0.56.0](https://x.ai/changelog/bot#v0.56.0) | 9 | No parity claim |
| [0.55.0](https://x.ai/changelog/bot#v0.55.0) | 5 | No parity claim |
| [0.54.0](https://x.ai/changelog/bot#v0.54.0) | 7 | No parity claim |
| [0.53.0](https://x.ai/changelog/bot#v0.53.0) | 9 | No parity claim |
| [0.52.0](https://x.ai/changelog/bot#v0.52.0) | 9 | No parity claim |
| [0.51.0](https://x.ai/changelog/bot#v0.51.0) | 7 | No parity claim |
| [0.50.0](https://x.ai/changelog/bot#v0.50.0) | 2 | No parity claim |
| [0.49.0](https://x.ai/changelog/bot#v0.49.0) | 9 | No parity claim |
| [0.48.0](https://x.ai/changelog/bot#v0.48.0) | 7 | No parity claim |
| [0.46.0](https://x.ai/changelog/bot#v0.46.0) | 7 | No parity claim |
| [0.45.0](https://x.ai/changelog/bot#v0.45.0) | 7 | No parity claim |
| [0.44.0](https://x.ai/changelog/bot#v0.44.0) | 9 | No parity claim |
| [0.41.0](https://x.ai/changelog/bot#v0.41.0) | 7 | No parity claim |
| [0.40.0](https://x.ai/changelog/bot#v0.40.0) | 6 | No parity claim |
| [0.39.0](https://x.ai/changelog/bot#v0.39.0) | 9 | No parity claim |
| [0.38.0](https://x.ai/changelog/bot#v0.38.0) | 2 | No parity claim |
| [0.37.0](https://x.ai/changelog/bot#v0.37.0) | 10 | No parity claim |
| [0.36.0](https://x.ai/changelog/bot#v0.36.0) | 4 | No parity claim |
| [0.35.0](https://x.ai/changelog/bot#v0.35.0) | 5 | No parity claim |
| [0.34.0](https://x.ai/changelog/bot#v0.34.0) | 6 | No parity claim |
| [0.33.0](https://x.ai/changelog/bot#v0.33.0) | 8 | No parity claim |
| [0.32.0](https://x.ai/changelog/bot#v0.32.0) | 2 | No parity claim |
| [0.31.0](https://x.ai/changelog/bot#v0.31.0) | 12 | No parity claim |
| [0.30.0](https://x.ai/changelog/bot#v0.30.0) | 12 | No parity claim |
| [0.29.0](https://x.ai/changelog/bot#v0.29.0) | 4 | No parity claim |
| [0.28.0](https://x.ai/changelog/bot#v0.28.0) | 3 | No parity claim |
| [0.27.0](https://x.ai/changelog/bot#v0.27.0) | 4 | No parity claim |
| [0.26.0](https://x.ai/changelog/bot#v0.26.0) | 7 | No parity claim |
| [0.25.0](https://x.ai/changelog/bot#v0.25.0) | 12 | No parity claim |
| [0.24.0](https://x.ai/changelog/bot#v0.24.0) | 9 | No parity claim |
| [0.22.0](https://x.ai/changelog/bot#v0.22.0) | 2 | No parity claim |
| [0.21.0](https://x.ai/changelog/bot#v0.21.0) | 12 | No parity claim |
| [0.20.0](https://x.ai/changelog/bot#v0.20.0) | 6 | No parity claim |
| [0.19.0](https://x.ai/changelog/bot#v0.19.0) | 11 | No parity claim |
| [0.18.0](https://x.ai/changelog/bot#v0.18.0) | 1 | No parity claim |
| [0.17.0](https://x.ai/changelog/bot#v0.17.0) | 9 | No parity claim |
