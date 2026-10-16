# Implementation and recovery

The two checkpoint ZIPs restore 3,205 source files against the captured per-file
manifest, not merely the historical `6b1c30b` HEAD. Originals stay in the ZIPs;
the restoration manifest and file hashes are outside this additive overlay.
Root AGPL-3.0-only, split SDK Apache licensing and trademarks are preserved.

1. `nekobot-local-protocol.ts` validates bounded opaque source IDs, explicit
   pins, receipt/page identities and UTF-8 text. Unknown authority keys fail.
2. `nekobot-local-bridge.ts` owns the serial strict-storage barrier, immutable
   intent outbox, receipt correlation, replay projection and audience hold.
3. `nekobot-local-wiii.ts` composes the real Wiii strict storage implementation
   and AnswerBlockData vocabulary. It stores no raw Core receipt/key/Work blob.
4. `NekoChillApp` passes an optional trusted surface to `ProjectHome`; a dedicated
   panel handles explicit messages, retry/reconcile and replies. It never uses
   the ACP driver factory as a remote Neko Bot driver.

Storage is keyed by owner/space/source session/source thread/channel/revision.
Changing a channel revision requires a newly reviewed binding, rather than
rebinding old outbox intent. An existing saved snapshot with mismatched source
fails before any write. Response JSON size/depth/count are bounded before use.

Risk: cross-surface response confusion or a lost ACK could publish in a wrong
lane or duplicate intent. Tests cover foreign identities, original nonce
retries, durable page failure, empty cursor advance and concurrent duplicates.
The server remains the authority for ACLs, resources and bot routing.

This first local view accepts at most 100 original messages per binding and
fails closed when that store is full. Accepted messages remain history; this
slice does not delete or compact their receipt correlation. The Go journal is
the durable history authority. A general long-lived Wiii history view and
native pairing need separate implementation and acceptance.

Rollback: omit the optional surface prop and stop invoking the bridge factory.
Keep `nekobot-local-bridge-v1.json` and original archives intact; the existing
ACP/handoff paths remain available. No state migration or destructive cleanup
is needed.
