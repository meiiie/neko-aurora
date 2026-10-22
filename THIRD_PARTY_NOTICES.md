# Third-party notices

Code and assets below are included under their original permissive licenses.
License headers are kept in the files that carry derived code.

## AG-UI protocol — MIT
- Source: https://github.com/ag-ui-protocol/ag-ui (commit 8aa86867fb2918001dca1c64db3976cd37f50703)
- Used: `EventType` enum from `sdks/typescript/packages/core/src/generated/types.ts`
- In: `app/native/internal/duet/agui.go` (Go translation), `app/native/internal/web/assets/duet/agui.js`

## OpenMuse (CopilotKit) — MIT, Copyright (c) 2026 OpenMuse contributors
- Source: https://github.com/CopilotKit/openmuse (commit 1ac68f3909f2478ab6280883f1ab5ea65eb5719d)
- Used: `TaskStatus`, `TaskStep`, `RunEvent` shapes from `packages/domain/src/agent.ts`
- In: `app/native/internal/duet/agui.go`, `app/native/internal/ledger/direct_runtime.go` (step statuses)

## OpenDots (CopilotKit) — MIT
- Source: https://github.com/CopilotKit/OpenDots
- Used: UX patterns only (human-in-the-loop review cards, tool-rendered cards). No code copied.

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
