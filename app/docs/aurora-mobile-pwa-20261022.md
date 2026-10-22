# Neko Aurora: mobile PWA (/app/) on the Go control plane

Neko Aurora is a mobile-first, installable surface at `/app/`. The Go control plane serves it behind the same loopback session cookie as the main UI (`neko_local`, set on `GET /` or `GET /app/`).

You hand work to agents by message. They keep working in the background, and nothing goes out without your OK.

## Server (`app/native/internal/aurora`)
- **One model:** the control-plane ledger (`state/transactions.jsonl`, hash-chained, replay-validated). There is no Aurora store.
  - Agents are ledger bots; a message is a ledger work item; the reply, plan, choices and approval are `work.direct_*` events on that work.
  - Avatar, pin and tag are `aurora.look` records; Web Push subscriptions are `aurora.push` records; the activity log is an append-only `aurora.activity` journal.
- **Turns:** run through Neko Core (`bun core/bin/neko.ts --profile zai --no-tools run`, GLM-5.3 on z.ai). Without Core the same provider is called over HTTP (`AURORA_AI_BASE_URL`, `AURORA_AI_MODEL`, or `AURORA_RUNTIME=http`).
  - `ZAI_API_KEY` stays in the server environment; it never reaches the client.
  - Work queued from the desktop is run by the same runner, so both UIs show one task.
  - A restart marks interrupted turns failed (`interrupted_by_restart`).
- **Approvals:** an `APPROVAL: {json}` line becomes a pending card; only the owner's journaled decision moves it, and repeat/racing decisions get 409.
- **Outbound (`internal/outbound`):** after approval the card goes to a channel adapter. Email over SMTP (`AURORA_SMTP_*`) is the first adapter. The router refuses any recipient the owner did not type in the thread; an unconfigured channel queues the card instead of sending.
- **Morning brief:** a ledger task on the brief bot; daily 05:30, on first start and on demand (`POST /app/api/brief`); deterministic fallback without AI.
- **Notifications:** Web Push (RFC 8291 aes128gcm, VAPID RFC 8292), stdlib only. VAPID key: `state/aurora/vapid.pem`.
- **SSE:** `GET /app/api/events` with a 15 s heartbeat; the client resyncs `/state` on reconnect or when visible.

### API
| Method and path | Purpose |
|---|---|
| `GET /app/api/state` | Agents, pending approvals, brief, AI info, VAPID public key |
| `POST /app/api/agents` | Create an agent |
| `POST /app/api/agents/{id}` | Patch an agent (name, desc, avatar, pin, archive) |
| `GET /app/api/agents/{id}/messages` | Message history |
| `POST /app/api/agents/{id}/messages` | Send a message; returns 202 and starts a background turn |
| `POST /app/api/approvals/{id}` | Record a decision |
| `GET /app/api/activity` | Activity log |
| `POST /app/api/brief` | Regenerate the morning brief |
| `POST /app/api/push/subscribe` | Register a push subscription |
| `POST /app/api/push/test` | Send a test notification |
| `GET /app/api/events` | SSE event stream |

## Client (`app/native/internal/web/assets/aurora`)
- **Screens:** agents list, morning brief card (with "Làm mới"), threads with streaming replies and approval cards, activity sheet, and the pixel-agent creator.
- **Voice input:** Web Speech API (`vi-VN`) where available. The mic button is hidden where it isn't supported.
- **iOS/mobile:**
  - safe areas, `100dvh`, targets of at least 44px, no horizontal overflow, 16px inputs;
  - edge-swipe back, row swipe to pin or archive, drag-to-dismiss sheets;
  - ghost-tap and double-tap guards (create, approve, archive, send);
  - history-safe back;
  - manifest, Apple meta tags, and a service worker with push handlers.
- The CSP for `/app/` adds `manifest-src` and `worker-src 'self'`.

## Tests
- `go test ./internal/aurora` covers:
  - approval parsing;
  - decide-once;
  - persistence;
  - push encryption round-trip;
  - a streaming turn over SSE against a fake OpenAI endpoint, with the approval gate and 409 on a duplicate decision;
  - no-key behaviour.
- `app/tests/aurora/mobile-touch.mjs` runs headless Chrome touch emulation at 360, 390 and 430 widths against a running server.
  - Use `app/tests/aurora/fake-ai.mjs` with `AURORA_AI_BASE_URL=http://127.0.0.1:8901 ZAI_API_KEY=fake-test-key` for a deterministic run.
  - Use `REAL_AI=1` with the real key for an end-to-end run.

## HTTPS exposure for a phone (`/app/` only)
Set `AURORA_PUBLIC_HOST=<https host>` and `AURORA_OWNER_CODE=<at least 12 chars>` in the server environment. Requests with that `Host`:
- reach only `/app/` (`/` redirects there; `/v1/*` and the desktop UI return 404);
- never receive the loopback `neko_local` cookie;
- need an owner login (`POST /app/login`) that sets an HttpOnly, Secure, SameSite=Strict cookie for 30 days;
- are rate-limited on login (5 failures lock it for 5 minutes);
- must have Origin equal to `https://<host>`;
- get HSTS.

Put a TLS-terminating tunnel in front of it, for example:
```
cloudflared tunnel --url http://127.0.0.1:8765
```

## Real AI run (2026-10-22)
- `/models` on the coding endpoint lists `glm-4.5`, `glm-4.5-air`, `glm-4.6`, `glm-4.7`, `glm-5`, `glm-5-turbo`, `glm-5.1`, `glm-5.2`, `glm-5.3`, `glm-5.3-flash` and `glm-5.3-flashx`. The default is `glm-5.3`.
- `REAL_AI=1` touch run at 360, 390 and 430: 85/85 PASS. This covered real streaming (80–141 deltas per reply), a real approval card, a single decision, and an AI morning brief.

## Known gaps
- The control plane only accepts its loopback `Host`. To use it from a phone you need HTTPS plus a host allow-list or tunnel. iOS Web Push additionally requires HTTPS and the app installed to the Home Screen (iOS 16.4+).
- There are no outbound channels (email, X, payments), so approved items are stored, not delivered.
- Aurora agents are separate from ledger bots and channels in `/v1/*`; they are not merged yet.
- Aurora has single-owner local auth only (the same cookie). Fixture or multi-actor identity is not applied to Aurora.

## Học từ CopilotKit OpenMuse / OpenDots (MIT) — chỉ học pattern, không chép mã
- OpenMuse: React Native + AG-UI, server/worker/computer; task bền vững có plan, approval, receipt; nút gửi→dừng, hàng đợi follow-up.
- OpenDots: agent "Dots" bền vững; thẻ review HITL (useHumanInTheLoop), thẻ do tool render (useRenderTool), Spaces, voice. Cả hai cần khoá CopilotKit Intelligence.
- Neko Aurora: Go + vanilla JS tự chứa, không phụ thuộc SaaS.
Đã áp dụng (viết lại từ đầu, ghi công trong `internal/app/turn.go`):
1. Kế hoạch nhiều bước với trạng thái sống (PLAN: 2–4 bước, mỗi bước 1 lần gọi model; pending/running/done/waiting/failed/cancelled). Bước cần Darren chọn sẽ tạm dừng kế hoạch.
2. Thẻ trong luồng: CHOICES (nút chọn, chỉ gửi 1 lần), thẻ duyệt có Sửa → Lưu & duyệt.
3. Nút gửi thành ■ dừng khi agent đang chạy; tin nhắn mới vào hàng đợi "Đang chờ lượt" và thay thế (supersede) thẻ duyệt đang chờ.
Xem triển khai: `aurora-deploy.md`.
