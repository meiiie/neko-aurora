# Local conversation, history export and retention preview UI

The approved light layout remains unchanged outside a compact **Hội thoại** item in the composer menu. It opens a modal for the current bot's private conversations. Shared channels keep their existing navigation and member controls.

## Behavior

- Create a named private conversation, choose a previous conversation, view archived conversations, and archive or restore one. Archiving changes metadata only. It does not cancel tasks, change aliases, move context pins, or transfer file bindings.
- The initial `conversation-<botId>` stays compatible with the existing empty `channelId` representation. A new private conversation uses its exact ID as `channelId` and a virtual audience revision of **1**. Its archive metadata revision is independent of that audience.
- The selected private conversation ID is remembered per bot. Draft, lane, scroll and following state use the existing exact conversation/project view key while the page is alive. A bot's sidebar activity includes tasks in its other private sessions.
- Archived sessions are readable and keep their existing task stop controls; fresh intake is disabled until restored. Returning to a prior session restores its task/result-file view. Creating a session never transfers a project's file permissions or source excerpts. Shared project, recipient/member and collaboration controls stay hidden for new private sessions in this slice.
- A leading `@bot` can route a request to another local bot within the original private conversation. Its reply is attributed to the actual bot; the owner session and header remain selected. The private mention roster stays available, while shared channels restrict it to current members. The browser's queued fixture has no completed target-private history; exclusion of such history belongs to ledger/native acceptance rather than that empty-history case.
- Export links download the server's scoped public history as JSON or plain text. The server supplies a fixed ASCII attachment filename and authenticated local download paths; the UI does not embed context pins, account secrets, tool prompts or bytes in the export.
- Retention is explicitly **Chỉ xem trước, không xóa**. The stored preference is only a number of days, from 0 to 3650. The UI requests a read-only preview, verifies `enabled:false`, displays request-record counts and estimated UTF-8 text bytes, and notes that protected records and files remain. It has no delete route, enable switch or scheduler.

## Response and navigation fences

Conversation and retention metadata are accepted monotonically. Same-revision conflicting values and changes to a conversation's original bot/creation identity are rejected. Snapshot/SSE conversation metadata refreshes do not become zero-ID work records. Metadata revision changes never rewrite work, aliases or authority pins.

The retention editor pins the preference revision displayed when details open. If a new snapshot advances that revision while a draft is being edited, the draft stays visible and saving is paused until explicit **Đọc lại**. It never silently adopts another client's revision to overwrite that update.

Create, archive and retention commands freeze their URL, original scope, revision, body and nonce while an uncertain response is retried. Known 400/404/409 rejections require an explicit **Đọc lại** before a new command can be built. Transport/5xx retries reuse the original intent. These lifecycle retry commands are page-lifetime memory; reload reconciles committed metadata from the server snapshot and does not automatically POST a new lifecycle command. No reload retry durability is claimed for this slice.

Late message or create ACKs cannot switch a session, close a newly opened modal, clear another bot's draft or move its focus. Native modal controls use 44px actions and 16px input text, with a bounded scrollport at low heights. Removing or disabling a focused archive/retention/retry control moves focus to the modal's close button before the DOM change. Closing the dialog restores focus only within the original view/navigation generation.

## Validation

`node scripts/ui_conversations_local.mjs` runs actual metadata and virtual-scope functions in an isolated Node VM. With `--go`, `--host` and an owned E `--output`, the same runner builds no code itself: it launches the supplied Go fixture with separate state, starts headless Chrome, verifies that the served assets match source bytes, and runs real local HTTP cases. `--base` can instead select an explicit `http://127.0.0.1:<port>` fixture endpoint.

The browser path covers new and prior sessions, separate drafts, private scope intake, late ACK navigation fences, archive/restore with active tasks, retention metadata/preview, scoped export attachments, lost create ACK retry, reload selection and low-height controls. Genuine HTTP response-stage fault injection is labelled separately from Core execution; the queued fixture does not launch Core, evaluate a model, operate a native Tauri window, or establish physical phone/AT/cloud acceptance.

Working development failures and their corrections are retained in `E:/CodexScratch/conversations-retention-20261012/ui-worker/`: the first harness observed lexical state before app startup; the next real integration run exposed a UI ACK-field mismatch (`preference` HTTP versus `retentionPreference` SSE). Final combined checkpoint results must be read from the integrator's exact commit receipt, rather than inferred from those development rounds.
