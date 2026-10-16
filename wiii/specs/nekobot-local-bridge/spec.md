# Wiii → Neko Bot local bridge

Authorized local implementation on 2026-10-11. Publication, external grants,
providers and user production state are outside this work. No issue/PR is
created under the user's explicit local-only scope.

Wiii remains the ADE. Neko Bot owns its bots, selected account authority,
shared-channel publication, task journal and Core runtime. A trusted local
caller injects an authenticated request transport and an explicit owner/space,
source session/thread and channel revision binding. The bridge does not obtain
cookies, discover an unauthenticated endpoint, enable CORS, start a process,
consume provider credentials or convert a file-import source ID into authority.

The restored `nekobot.task-handoff.v1` decoder and reviewed unsent-draft flow
remain unchanged. The new local panel is separate from Wiii's ACP draft and
appears only when the caller supplies a surface for that exact Project.

## Acceptance

- Construction, hydration and importing a handoff cannot register/send/launch.
- Registration is bound to server-derived owner, space and return route. The
  registration nonce is durable before transport and remains stable after a
  lost ACK or restart.
  Exact HTTP ACK is `{session,duplicate}`; alternate aliases reject.
- Each source message is immutable. Account/project/file choices and nonce
  survive retry; a transport error does not silently create a new request.
- Permanently rejected intent is held independently; later valid intent can
  continue. Uncertain delivery must be reconciled before discard.
- Replies match the exact registered source/session/thread/channel and original
  source message/request. Cumulative replies use a stable Wiii answer block.
  The entire page and replay cursor share one strict-storage commit.
- A stale/revoked audience hides cached replies without changing the registered
  route or overwriting preserved history. No private Wiii transcript or local
  workspace path is exported automatically.
- The original Wiii storage, handoff decoder and ContentBlock vocabulary are
  exercised directly. Actual Go protocol testing, frontend syntax, full Wiii
  build and native transport/UI acceptance are recorded separately.

## Remaining boundary

The caller-authenticated transport is an explicit seam, not implemented OS
identity attestation or a native Wiii pairing/bootstrap. An offline module or
loopback test cannot prove a packaged Wiii GUI, provider quality, phone UX,
cloud sync, external connector publishing or production grants.
