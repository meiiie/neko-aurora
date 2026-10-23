# Third-party notices

Code and assets below are included under their original permissive licenses.
License headers are kept in the files that carry derived code.

## AG-UI protocol — MIT
- Source: https://github.com/ag-ui-protocol/ag-ui (commit 8aa86867fb2918001dca1c64db3976cd37f50703)
- Used: `EventType` enum from `sdks/typescript/packages/core/src/generated/types.ts`
- In: `app/native/internal/aurora/agui.go` (Go translation; the server emits AG-UI run/step events on the `/app/api/events` stream)

## OpenMuse (CopilotKit) — MIT, Copyright (c) 2026 OpenMuse contributors
- Source: https://github.com/CopilotKit/openmuse (commit 1ac68f3909f2478ab6280883f1ab5ea65eb5719d)
- Used: `TaskStatus`, `TaskStep`, `RunEvent` shapes from `packages/domain/src/agent.ts`
- In: `app/native/internal/aurora/agui.go`, `app/native/internal/ledger/direct_runtime.go` (step statuses)
- Patterns (re-implemented in Go, no code copied): serial ordered commit of parallel tool calls (`engine/model.ts` `serial()`), user-confirmed memories (`remember_fact`), capped untrusted page text — in `app/native/internal/aurora/tools.go`, `harness.go`

## OpenDots (CopilotKit) — MIT
- Source: https://github.com/CopilotKit/OpenDots
- Used: UX patterns only (human-in-the-loop review cards, tool-rendered cards), and the per-result character cap with a truncation marker (`src/server/connections.ts`) in `harness.go`. No code copied.

### MIT License text

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.


## Fonts (SIL Open Font License 1.1)

Bundled as WOFF2 subsets (Latin + Vietnamese, one `unicode-range` per face) in
`app/native/internal/web/assets/app/aurora/fonts/`. They are subsets of the
upstream files (glyph subsetting, plus a fixed-width instance of Noto Serif
Display); none of these fonts declares a Reserved Font Name. Full license texts sit beside the fonts.

| Font | Copyright | License file |
|---|---|---|
| Be Vietnam Pro 400/600/700 | Lâm Bảo, Tony Le, ViệtAnh Nguyễn | `fonts/bevietnampro-OFL.txt` |
| Noto Serif Display (wght 500–700, wdth 100 instance) | Google LLC | `fonts/notoserifdisplay-OFL.txt` |
| JetBrains Mono (variable wght) | JetBrains s.r.o. | `fonts/jetbrainsmono-OFL.txt` |
| Geist (pre-existing, `assets/fonts/`) | Vercel | SIL OFL 1.1 |

## Lucide icons — ISC License

Path data for a small set of Lucide icons (v0.468.0, https://lucide.dev) is
embedded in `app/native/internal/web/assets/app/aurora/icons.js`, rendered at
stroke-width 1.75. License text also at `assets/app/aurora/lucide-LICENSE.txt`.

```
ISC License

Copyright (c) for portions of Lucide are held by Cole Bemis 2013-2022 as part of Feather (MIT). All other copyright (c) for Lucide are held by Lucide Contributors 2022.

Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
```

## Neko Aurora design system

`assets/app/aurora/aurora.css`, the logo mark and wordmark are original
HoLiLiHu work (from the aurora-design package); no third-party code.

## Simple Icons — CC0 1.0 Universal

Brand marks for Gmail, Google Calendar, Google Drive, Notion, GitHub, Telegram,
Zalo, Discord and X in `app/native/internal/web/assets/app/aurora/brands.js`
come from Simple Icons v16.35.0 (https://simpleicons.org), dedicated to the
public domain under CC0 1.0 (full text in `aurora/simple-icons-LICENSE.txt`).
The marks remain trademarks of their owners and are used only to identify the
services in the connectors sheet. Slack is not in Simple Icons and uses a
neutral Lucide glyph instead.

## CopilotKit/OpenDots — MIT License

`app/native/internal/tools/netsafe.go` ports `isPublicAddress` (src/browser/security.ts)
and `requestedUrls` (src/server/research.ts) to Go, and the tool-result grounding
rules in `internal/aurora/tools.go` adapt the research prompt (sources are untrusted
data, cite, state gaps, do not fabricate). The secret card's busy label and inline
`role="alert"` error follow src/client/ConnectionsSection.tsx.
https://github.com/CopilotKit/OpenDots (commit 8091324)

Copyright (c) Atai Barkai

Permission is hereby granted, free of charge, to any person obtaining a copy of this
software and associated documentation files (the "Software"), to deal in the Software
without restriction, including without limitation the rights to use, copy, modify,
merge, publish, distribute, sublicense, and/or sell copies of the Software, and to
permit persons to whom the Software is furnished to do so, subject to the following
conditions: The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software. THE SOFTWARE IS PROVIDED "AS IS",
WITHOUT WARRANTY OF ANY KIND.

## CopilotKit/openmuse — additional borrowings (MIT, © 2026 OpenMuse contributors)

- Tool-call card states (running / stopped / failed / "N sources") and URL de-duplication
  of sources from apps/mobile/src/search-tool-card.tsx (commit 1ac68f3).
- Versioned, domain-separated AES-GCM envelope (packages/integrations/src/vault.ts):
  our vault entries now carry `v: 1` with AAD `neko-aurora:secret:v1:<name>`.

## Neko Core (Meiiie / The Wiii Lab) — AGPL-3.0-only (outside sdk/)
- Source: https://github.com/meiiie/neko-core
- Used: design patterns only, re-implemented in Go/JS (non-blocking input queue that drains in order, abort-at-step-boundary steering, checkpointed continuation after interruption). No code copied. See docs/runtime-decision-2026-10-23.md.
