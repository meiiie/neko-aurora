# Core profile bridge v1

This bridge lets a trusted NekoBot launcher select an existing Core-owned connection without copying the account key into Go or Rust. The credential source is distinct from `USERPROFILE`/`HOME`, which remains the task, memory, trust and artifact state boundary. Existing CLI behavior is unchanged when the bridge resolver is absent.

## Inspection

`neko profiles --json --credential-home <absolute-source-home>` emits one JSON object:

```json
{"version":1,"profiles":[{"id":"example","provider":"openai_compat","model":"model-id","authKind":"api_key","active":true,"configurationStatus":"configured","credentialReadiness":"not_checked","profileRevision":"pr1_<64-lowercase-hex>","providerOrigin":"https://api.example.test","bridgeTransport":"supported"}]}
```

The trusted parent supplies `NEKO_PROFILE_REVISION_KEY`, a private random 32-byte key encoded as 64 lowercase hex characters. The output has at most 64 profiles. Profile IDs are ASCII `[A-Za-z0-9][A-Za-z0-9_-]{0,63}`; model IDs are bounded ASCII tokens of at most 120 characters; origins are at most 320 UTF-8 bytes. Origin includes scheme, host and port only. Endpoint userinfo, query and fragment are refused. No raw endpoint path, key, token, environment name, account identifier, config object or home path is emitted. Known private values smuggled into the public projection are refused with a generic error.

Inspection reads only `.neko-core/config.json`, `neko.json` and bounded Core-owned credential bytes for opaque revision binding. It does not load project configuration, trust, MCP, plugins, memory or global policies; instantiate a provider; refresh a token; install support; check updates; delete a stale installed executable; or open login. `credentialReadiness` always means `not_checked`; configuration and transport support do not prove a live account or model works.

## Trusted launch

The only execution entrypoint accepting a separated source is `neko acp --host-profile nekobot --profile <exact-id> --credential-home <absolute-source-home>`, with existing context-memory/admission flags unchanged. Ordinary ACP, `nekocut`, chat/run/login commands cannot adopt this resolver.

Trusted process environment fields are:

- `NEKO_CREDENTIAL_HOME`: source home, overridden by the explicit CLI argument.
- `NEKO_PROFILE_REVISION_KEY`: private parent-owned HMAC key.
- `NEKO_PROFILE_REVISION`: exact selected public `pr1_` token.
- `NEKO_PROFILE_ORIGIN`: exact selected public origin, rechecked before dispatch.
- `NEKO_PROFILE_LIMITS`: JSON with exactly `maxInputBytes`, `maxOutputTokens`, `maxRequests`, `maxSteps`, `maxSeconds`.

All fields are stripped from model-driven child environments and excluded from printable ordinary config. Ambient provider/model/endpoint/key overrides are refused in this mode. The parent must clear credentials and unrelated execution variables, attest its selected source handle and obtain consent before launching. Roots and keys never enter the context manifest or public launch envelope. NekoBot's actor/device/audience authority remains host-enforced.

The keyed revision binds canonical source-home identity, exact selected connection fields, credential material and selected credential environment values internally. Its HMAC key is not an account key and is never printed. Key rotation invalidates old pins. Before provider construction, each completion and every wire attempt, Core rechecks the source revision and selected provider/model/endpoint. Source/profile changes fail closed; Core never silently re-pins an admitted task. HTTP redirects are refused and every wire request must keep the pinned provider origin.

Only file-backed API-key or local no-key direct `openai_compat`, `responses`, `anthropic` and non-OAuth `kimi` transports are supported in v1. No provider-specific readiness is inferred. Incomplete rows may have empty provider, model or origin, and always carry `unsupported`; no fallback destination is guessed. OAuth, environment-only keys, Gemini sidecars, OpenCode dynamic transports, MOA and installer-required routes report `unsupported`. Native OAuth readers now resolve Core-owned token files through the explicit credential source, but execution remains blocked because single-origin consent does not include token renewal destinations. MCP OAuth remains task-local; the source's MCP authority is never imported. Gemini credential discovery can resolve the source token path, but a separated-home Gemini sidecar is refused because its credential and session/settings homes are currently combined.

## Limits and recovery

All limit fields are positive safe integers; maxima are input 4 MiB, output 1,000,000, requests 1,000, steps 512 and seconds 86,400. `maxInputBytes` bounds each actual UTF-8 provider JSON body, including Core system/context/tools. `maxSteps` is passed to the existing agent loop. Requests include every inference wire attempt and retry. Each attempt reserves a conservative share of the cumulative output allowance and sends that as the provider completion-token limit. Streamed UTF-8 bytes and returned content/reasoning/tool JSON bytes must fit the reservation; provider-reported completion usage is also checked. This conservative accounting can stop earlier than a tokenizer would. An attempt retains its reservation after an error or lost ACK; there is no invisible refund.

The private task-home file `.neko-core/profile-bridge-budget-v1.json` records the original revision/limits, first-use clock, attempted requests and reserved output. A file sync and rename precede network dispatch. Reopening the same task home preserves the caps and deadline. Unknown/malformed/aliased budget state or a leftover exclusive lock refuses further dispatch. A trusted host must keep one live Core writer per task home. A new task uses a fresh isolated task home; replacing a source pin is not a budget reset within an existing task. Wall-clock rollback refuses recovery, and a monotonic timer aborts active requests. Machine power-loss and malicious filesystem-race durability are outside the fixture validation scope.

## Validation boundary

Tests use synthetic 1.7-compatible profile/config shapes in E scratch, fake tokens and a loopback provider. They validate secret-free metadata, source/state separation, pin/origin/limit denials, persisted unknown-attempt reservations and legacy defaults. Existing manifest v1/v2, admission, ACP and writer tests are regression gates. No installed Core, actual account/config, OAuth login, provider, production app state, grants or deployment is accessed by this slice. Root owns the new binary trust pin, host rebuild, source registry, consent and actual native integration.

## Known-credential public output boundary

Canonical ACP now filters the selected credential before publishing provider content/thought chunks. A linear prefix matcher retains only a possible credential prefix across chunks and channels; full matches are masked, and unfinished prefixes are masked at tool/final/error/cancel/flush boundaries. Non-secret text is preserved. Whole public notifications, tool input/output projections, permission requests and replay metadata are sanitized. Unexpected provider errors keep generic Internal error semantics; explicit ACP errors redact their message and discard untrusted error data. Known credential in actual in-band host tool arguments is refused locally before any parent request. The parent never receives a credential to perform redaction.

The causal intermediate ebf606a/af3 executable and five literal/full/split/mixed/tool/error RED fixtures are retained. The replacement is tested using file-backed fake keys and owned SDK connections; no actual profile is used. This is a known-literal credential guard, including split literal chunks, rather than a claim to infer arbitrary encoded exfiltration or evaluate model behavior.
