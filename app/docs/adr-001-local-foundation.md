# ADR 001: local native foundation

Status: accepted for development; production cutover is not authorized.

Use the direction's Go Native Lane. Native records have one writer: Go. They never enter legacy Graphile queues. An owner, space, bot, conversation, request, attempt and Core binding have distinct identities. A new message defaults to foreground and cannot cancel an existing background request.

For this offline foundation, use a checksummed append-only transaction journal with OS-exclusive writer locking and fsync before acknowledgement. This is a local development store, explicitly not PostgreSQL, cloud sync or a multi-host database. Production will require a separately tested PostgreSQL adapter and data migration.

Rust owns bounded protocol parsing and pinned Core capability admission. It must refuse incompatible Core rather than start another model loop. This checkout must not inherit secrets, profiles or broad machine authority from sibling repos.

First executable slice: persist the message/request/event atomically; reject nonce reuse with different content; separate lanes and bot identities; replay committed history on reconnect; recover a torn unacknowledged journal tail; refuse intact corruption. The UI shows waiting for Core honestly until trusted admission and a real runtime adapter are verified.

Acceptance for the complete stage 1 remains open until Core runs both sessions, emits genuine receipts and a real artifact, foreground replies before background completion, kill/restart avoids duplicate effects, and the desktop build and both clients are verified. Phone and provider gates are separate.
