# Protocol fixtures v1

This is a proposed native protocol, not proof of compatibility with the missing pinned Core snapshot.

Canonical v1: UTF-8 JSON, sorted object keys, no whitespace, literal Unicode, standard JSON control escapes, integers within +/- 9007199254740991, no floating point/exponents/negative zero, no duplicate keys, no U+FFFD, at most 32 nested levels and 64 KiB per frame. Hash exactly these canonical bytes with SHA-256. Go and Rust run the same fixture file.

Rust admission requires ACP v1 and fixed task/execution binding/operation identity/host effect v1, exclusive nekobot profile and the trusted root digest. Receipt identities must match trusted policy, including owner, space, bot, request, attempt, epoch and lease fence. This validator does not authenticate a transport or persist execution receipts. CommandGate is an in-memory preflight check only: production dispatch must first persist the receipt/command identity and verify trusted transport.

No runtime process, model call or tool effect is dispatched by this milestone. Core absence is surfaced as core_unavailable. True foreground replies, background completion and artifact gates remain blocked until the actual compatible Core adapter is integrated.
