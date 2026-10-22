# Compatible Core profile bridge

## Assignment and baseline

The requested journey is to use a profile already managed by Core 1.7 without entering its credential again. Neko Aurora uses an upgraded, explicitly pinned bundled Core with the `nekobot` authority contract. It preserves the installed Core executable and its configuration. Ordinary ACP from the installed 1.7 runtime does not support this host profile; discovery alone cannot authorize execution.

Application base: `16e9daddd568450379877b75f94a954384beca8d` (tested application `900d17d435ece36ad790eaf4e3d6cd101dbf0671`). Canonical Core base: `867966dd7f057dd1c03547014c81df93470d28e7`. Work takes place on separate branches. The existing service on port 8765, its state, old binary folders, ZIPs and unrelated edits are excluded from writes. No actual user configuration, profile credentials, provider sessions or chat history are read during development.

## User flow and authority

1. Discovery returns a safe source descriptor. It reads no credential/config files and grants nothing.
2. The local owner explicitly permits Core to inspect that source's profile metadata for a bounded period. Neko Aurora invokes only its compatible pinned Core, with no provider, login, update or tool initialization.
3. Core returns a bounded public catalog. `configured` describes connection fields; `credentialReadiness: not_checked` does not promise working authentication. Unsupported transports remain unavailable.
4. The owner selects a profile and explicitly adopts a scope: actor/device, exact channel/current audience, optional project, bot set, provider/model/network origin, data/request/token/step/time caps and expiry. Saving this consent does not contact a provider.
5. A new message can explicitly select that consent. Each accepted Work pins the original nonce, consent/source/profile revisions, runtime digest, context/resource digests, destination and limits. Account selection and profile selection cannot be mixed. No adapter, child request or collaboration silently inherits this grant.
6. Current authority is checked before intake/launch/prompt/effects. Profile/credential rotation invalidates the revision. Revocation and expiry fence output/effects and use the existing genuine process-stop path. A stopped task cannot regain authority by replaying an old ACK or regranting a different profile.

First identity scope is the authenticated default local owner and local executor. Fixture actors cannot inherit the owner's source access. This is not a cloud login, remote executor broker or multi-owner profile sharing feature.

## Core contract

`profiles --json --credential-home <trusted absolute source home>` returns a version 1 catalog of at most 64 profiles. Allowlisted fields: `id`, `provider`, `model`, `authKind`, `active`, `configurationStatus`, `credentialReadiness`, `profileRevision`, `providerOrigin`, `bridgeTransport`.

The revision is `pr1_` followed by keyed HMAC-SHA256. Core computes it over selected connection/credential inputs internally. No raw credential or unkeyed credential digest appears in metadata. A bridge-owned private key is transported only in the trusted process environment, sealed with Windows-user-bound DPAPI in private app state, and excluded from tools, logs, launch envelopes and public snapshots. Replacing this private key invalidates previous pins.

The source root is resolved from a trusted native/CLI configuration; browser requests carry only a source ID. Core reads its own config/auth storage. Task home, memory, sessions, trust, artifacts and tool environment remain private to the task. Source project policy and MCP configuration are not imported. Source credentials are shielded from read tools and child processes. Inspection does not refresh authentication or start an installer. Transports that cannot isolate credentials from user sessions or require an installer fail closed.

The initial executable scope is file-backed API-key or explicit no-key direct transports: OpenAI-compatible, Responses, Anthropic and Kimi. OAuth, environment-only credentials, sidecars with a combined user session home, installers and dynamic transports remain unsupported. Owner-home execution permits HTTPS; owned fixtures permit only their exact explicit loopback origin. A configured profile is not proof of provider readiness.

Known selected credentials are filtered at public ACP sinks, including split streaming content, reasoning, tool/permission updates, replay and errors. Provider errors discard raw request/response data, and a known credential in host-tool arguments prevents that dispatch. These are literal/split protections, not a claim to infer every encoded exfiltration. Intermediate unsafe Core fingerprints remain recognizable solely for historical journal replay; fresh source adoption, binding and admission require the corrected executable pin.

`acp --host-profile nekobot --profile <id>` receives the expected profile revision and trusted limits. Core validates the revision before consuming credentials/provider transport. Existing v1 owner and v2 requester manifests and native writer behavior remain required. An incompatible installed runtime is not substituted for the bundled runtime.

## Application contract

`CoreProfileService` exposes safe source descriptors, consent-gated inspection and fresh pin verification. The runtime implementation privately owns source roots and revision keys. The ledger/API own durable inspection/execution consents, nonce/CAS validation, Work pins and current authority guards. The trusted Rust host validates the public pin against the selected compatible runtime and private source binding before configuring Core.

Owner endpoints: source discovery, explicit inspection consent, consent-gated catalog read, explicit scoped execution consent and CAS revocation. Message intake uses `coreProfileSelection: {consentId, expectedConsentRevision}`. Source roots and arbitrary executable/config paths are never accepted from HTTP input.

Fixture sources permit only an owned exact loopback destination. A real source exposes a normalized HTTPS network origin to the user and still requires their own explicit inspection and execution consent. Development exercises only fresh synthetic source homes and credentials. A real provider test, OAuth refresh, cost-bearing action, deployment or global installation change requires separate concrete user authorization.

## Acceptance evidence to produce

- Synthetic old-style Core profiles list without credentials, raw URLs, environment names or project/MCP side effects; inspection does not call the model.
- Missing consent, actor/device mismatch, malformed DTO, stale source/profile/runtime/consent/audience and mixed account/profile selection deny before mutation or provider use.
- A selected synthetic credential remains only in Core-owned source storage; private task state/memory does not enter that source. No credential appears in journal, API/SSE, browser storage, host frames, reports or tool child environment.
- A genuine native bundled Core completes a scoped loopback profile request without key reentry; bound budgets, same-nonce retry and restart remain correct.
- Rotation/revoke/expiry during work produces fenced effects/output and a genuine stopped process receipt; independent security review checks the combined boundary.
- Owner onboarding supports truthful unsupported/error states and preserves session/navigation/draft/focus fences at desktop and short mobile sizes.
- Tests, code commits, binary hashes, failures and remaining real-device/provider gates are recorded separately. Passing a catalog or fixture cannot establish real OAuth/provider/cloud readiness.

## Ownership

Core adapter worker owns canonical credential-root resolution, profile metadata/revision and provider budget enforcement. Backend worker owns consent ledger/API/Work pins. UI worker owns owner onboarding and presentation only. Root owns the private source registry, native host/manager/CLI, exact binary pairing and integration evidence. Independent review consumes immutable checkpoints. There is no production deployment or stable-live mutation in this assignment.
