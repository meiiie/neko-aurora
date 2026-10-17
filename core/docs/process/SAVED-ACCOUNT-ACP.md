# Internal saved-account ACP startup

This staged internal adapter is not reachable from `bin/neko.ts`. Native preparation
metadata and saved-account ACP CLI activation remain denied pending Windows native
acceptance and trusted host launch integration. No UI capability is enabled.

The future exact argv is `saved-account acp --broker-stdin`. The private first line
is canonical JSON (sorted object keys), UTF-8, at most 16384 bytes including LF,
received within two seconds. It does not wait for EOF. Remaining bytes, including
ACP frames coalesced into the same chunk, pass unchanged to the existing ACP server.
The bootstrap never becomes an ACP message or transcript entry.

Exact fields: `version: 1`, `purpose: "saved_account_acp_v1"`, `binding`, `capsuleId`,
`capsulePath`, `stateHome`, `revisionKey`, `witnessPath`, `serviceGeneration`,
`authoritySHA256`, `root`, and `nativeWitness`. The last object contains exactly
`executable` and `executableSHA256`, supplied by the trusted host's retained current
image/ancestor guards, never browser input. Paths are absolute. Profile selection
and credential-home environment mixing are denied. Errors are generic.

The internal preparation envelope additionally accepts that same optional
`nativeWitness` descriptor through the shared witness factory. Its existing public
EOF-delimited CLI still rejects the descriptor. See [preparation](SAVED-ACCOUNT-PREPARE.md)
and [native reader](SAVED-ACCOUNT-NATIVE-WITNESS.md).

Startup opens live authority, consumes the encrypted capsule through the existing
bridge and supplies its sole configuration through `configForRoot`. It never loads
home profiles as fallback. The restricted NekoBot agent loop admits one canonical
root, one session and one exact primary-text prompt. Context binding is the entire
canonical manifest digest (`RuntimeContextSHA256`), not `contextPinSHA256`.
List/load/resume, configuration changes, extra sessions and command prompts are
denied. Failed admission burns the in-process session slot. Witness loss closes
the connection; teardown awaits in-flight admission and closes any late runtime
before awaiting helper reaping.

Capsule rereads are integrity checks, not a global exactly-once claim. The trusted
host must retain the existing durable `BindRuntimeAttempt`, `Admit`, admitted and
prompt_ready acknowledgments before sending session/prompt. Those acknowledgments
are not independently verified by this Core adapter. Account lease fence and
executor fence remain distinct. Changed-attempt recovery and restarting with a
fresh state home must remain denied; neither may reset a binding's budget.

Focused synthetic tests cover framing, private failure bounds, one-root/session/
prompt admission, revocation during construction, cleanup, and the unchanged public
gates. They establish no provider connectivity or Windows native acceptance.
