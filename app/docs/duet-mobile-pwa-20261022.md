# Neko Duet: mobile PWA (/duet/) on the Go control plane

Neko Duet is a mobile-first, installable surface at `/duet/`. The Go control plane serves it behind the same loopback session cookie as the main UI (`neko_local`, set on `GET /` or `GET /duet/`).

You hand work to agents by message. They keep working in the background, and nothing goes out without your OK.

## Server (`app/native/internal/duet`)
- **Store:** `state/duet/duet.json` (0600, atomic rename). It holds:
  - agents (CRUD, pin, archive, pixel avatar spec);
  - messages;
  - activity log;
  - approvals;
  - morning brief;
  - push subscriptions.
- **AI:** z.ai GLM Coding Plan, OpenAI-compatible at `https://api.z.ai/api/coding/paas/v4`, model `glm-5.3`.
  - The key comes from `ZAI_API_KEY` in the server environment only. It is never sent to the client, and error text is scrubbed of it.
  - Overrides: `DUET_AI_BASE_URL` and `DUET_AI_MODEL`.
  - Thinking is disabled for latency.
- **Background tasks:** each agent has one worker. A message that arrives while the agent is running is picked up right after the current turn (steering).
  - Replies stream token by token over SSE (`delta` events), then persist as a `message`.
  - A server restart marks interrupted tasks as `error` honestly.
- **Approvals:** the model must end its reply with an `APPROVAL: {json}` line for any outbound action (send, post, pay). That becomes a pending approval card.
  - `POST /duet/api/approvals/{id}` records exactly one decision. A repeat or racing decision gets 409.
  - **Nothing is ever sent externally.** Approval only records the decision; there is no outbound channel yet, and the agent says so.
- **Morning brief:** AI-generated JSON summary of agent status, pending approvals and recent activity.
  - Runs daily at 05:30 server-local time, on first start, and on demand (`POST /duet/api/brief`).
  - Without AI it falls back to a deterministic summary.
- **Notifications:** Web Push (RFC 8291 aes128gcm, VAPID RFC 8292), stdlib only. The VAPID key is in `state/duet/duet-vapid.pem`.
  - Pushes go out for: an approval is needed, a task is done, the brief is ready.
  - Fallback: the service worker shows a notification from the SSE `notify` event while the app is open.
- **SSE:** `GET /duet/api/events` with a 15 s heartbeat. It clears the server read deadline so the long stream survives `ReadTimeout`. The client resyncs `/state` on reconnect or when the app becomes visible.

### API
| Method and path | Purpose |
|---|---|
| `GET /duet/api/state` | Agents, pending approvals, brief, AI info, VAPID public key |
| `POST /duet/api/agents` | Create an agent |
| `POST /duet/api/agents/{id}` | Patch an agent (name, desc, avatar, pin, archive) |
| `GET /duet/api/agents/{id}/messages` | Message history |
| `POST /duet/api/agents/{id}/messages` | Send a message; returns 202 and starts a background turn |
| `POST /duet/api/approvals/{id}` | Record a decision |
| `GET /duet/api/activity` | Activity log |
| `POST /duet/api/brief` | Regenerate the morning brief |
| `POST /duet/api/push/subscribe` | Register a push subscription |
| `POST /duet/api/push/test` | Send a test notification |
| `GET /duet/api/events` | SSE event stream |

## Client (`app/native/internal/web/assets/duet`)
- **Screens:** agents list, morning brief card (with "Làm mới"), threads with streaming replies and approval cards, activity sheet, and the pixel-agent creator.
- **Voice input:** Web Speech API (`vi-VN`) where available. The mic button is hidden where it isn't supported.
- **iOS/mobile:**
  - safe areas, `100dvh`, targets of at least 44px, no horizontal overflow, 16px inputs;
  - edge-swipe back, row swipe to pin or archive, drag-to-dismiss sheets;
  - ghost-tap and double-tap guards (create, approve, archive, send);
  - history-safe back;
  - manifest, Apple meta tags, and a service worker with push handlers.
- The CSP for `/duet/` adds `manifest-src` and `worker-src 'self'`.

## Tests
- `go test ./internal/duet` covers:
  - approval parsing;
  - decide-once;
  - persistence;
  - push encryption round-trip;
  - a streaming turn over SSE against a fake OpenAI endpoint, with the approval gate and 409 on a duplicate decision;
  - no-key behaviour.
- `app/tests/duet/mobile-touch.mjs` runs headless Chrome touch emulation at 360, 390 and 430 widths against a running server.
  - Use `app/tests/duet/fake-ai.mjs` with `DUET_AI_BASE_URL=http://127.0.0.1:8901 ZAI_API_KEY=fake-test-key` for a deterministic run.
  - Use `REAL_AI=1` with the real key for an end-to-end run.

## Known gaps
- The control plane only accepts its loopback `Host`. To use it from a phone you need HTTPS plus a host allow-list or tunnel. iOS Web Push additionally requires HTTPS and the app installed to the Home Screen (iOS 16.4+).
- There are no outbound channels (email, X, payments), so approved items are stored, not delivered.
- Duet agents are separate from ledger bots and channels in `/v1/*`; they are not merged yet.
- Duet has single-owner local auth only (the same cookie). Fixture or multi-actor identity is not applied to Duet.
