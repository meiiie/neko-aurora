# Local channel provenance and colleague routing

New intakes pin a server-derived local owner, platform, source channel/message, reply channel/thread and audience revision. A colleague tag chooses the bot; it keeps the reply in the source conversation. The client cannot supply actor, provenance, permission or reply destination fields. Existing historical events retain their accepted destination and remain readable; replay does not rewrite prior routing.

Local shared channels have stable random IDs, an owner/space, a revision and a bounded explicit bot membership. Only the authenticated local-owner API can edit them using CAS and a retained nonce receipt. This is a local ACL record, not a third-party connector grant. An unknown channel or an absent target bot rejects before claim. An implicit private bot lane permits an owner tag for the current request; it imports neither the source bot's history nor the target's private conversation.

Context retrieval remains partitioned by owner, bot and channel. Only completed, allowed sources from that exact scope enter the bounded pin. Same-bot history from another channel is excluded. Bot identity remains the selected bot while the reply destination comes from the origin envelope. Artifact bytes can be downloaded through the exact pinned channel path; the owner-wide artifact endpoint remains an authenticated owner's audit/download surface.

An audience revision change fences queued/active work before the edit ACK. Late output, tools and completion reject. Stop/effect reconciliation receipts remain writable, and the original source message/thread survives restart. Re-adding a bot does not revive work pinned to an older revision. A new intake explicitly targets the current channel.

This implements local API/runtime semantics. A channel UI, cross-platform identity mapping, connector participation checks, approved private-context declassification, handoffs spanning separate jobs, real Slack/Discord posting and multi-owner/cloud ACLs are pending. No external message, grant or deployment is performed by these tests.
