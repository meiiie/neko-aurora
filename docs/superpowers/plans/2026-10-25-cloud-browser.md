# Cloud browser implementation contract v1

Chosen B: dedicated cloud worker, private profile storage, no local-user browser fallback. Docker uses a mounted volume; the measured Railway runtime choice and its VM disk limitation are recorded below. Base B19/25 vs A17/25; graft A protected login phase, crash/takeover honesty, max-one-active-browser and idle close. Wiii reference 6b1c30b15b4c67e3c841ae870e6989577fc228f8 read-only; independently authored code, no Wiii implementation/assets copied. Current code branch feat/cloud-browser-workstation contains preserved previous WIP. Dated continuation sections retain the earlier observations; the final Railway acceptance section records current status.

## Ownership

- Node worker owns browser processes, private /data profiles, control epoch, observations and durable input receipts. Go owns effective owner/work authorization, Vault and approval. Existing Core/model loop unchanged.
- Go app never launches a browser locally. Missing cloud endpoint/key returns not_configured. Production URL HTTPS public or HTTP *.railway.internal or *.aurora.internal only; reject localhost/private literals/userinfo/query/fragment. Tests inject a wire client/HTTP transport, never a production localhost fallback.
- Worker serves only signed typed commands; no public CDP, VNC, shell, eval, cookies, profile-file, arbitrary selector or debug routes. Auth frames only through authenticated Aurora owner routes. No user-browser/profile mounts.
- Each Aurora state dir has a stable generated opaque namespace file (only create when cloud configured), each bot gets separate profile. Hub's distinct state dirs remain distinct; HTTP JSON never accepts namespace, actor, bot override or worker URL. Missing RequestIdentity fails closed; unsupported fixture Principal modes fail closed. api/aurora_public.go must inject local-owner only AFTER verified owner cookie. New feature read-only restored stores denied.
- Source Work must be current running epoch+NetworkAttempt, same bot; peer-origin Work forbidden, including any raw cloud approval. Owner approvals are separate from dispatch receipts. Every agent mutation needs immutable server-created cloud approval. Direct human UI input is an explicitly authenticated owner operation; Vault UI fill has an explicit confirmation with current origin/item binding.

## Wire: POST /v1/command

Client headers X-Aurora-Timestamp=Unix seconds; X-Aurora-Signature=hex HMAC-SHA256(masterKey, timestamp+'\n'+hexSHA256(exactBody)). Reject wrong/missing/expired timestamp (>30sec), key shorter32bytes, unknown JSON fields, >1MiB bodies. Secret is service-only, never in browser env or pages. Plain root healthz may reveal healthy/protocol only.

JSON fields (all optional except namespace/profile/kind/actor, validated per kind):
`{namespace,profile,kind,actor,actorId,workId,workEpoch,attempt,nonce,epoch,observationId,frameId,targetId,text,key,x,y,deltaY,url,mode,expectedOrigin,sealed}`

namespace = 64 lowercase hex digest generated Go side from stable namespace and key; profile = validated bot ID (`bot-...`). actor owner|agent, actorId server-derived. kind status|open|observe|frame|control|navigate|click|fill|press|scroll|close|vault_fill. mode paused|owner|agent. Mutations nonce 8..120 ASCII identifier, stable payload hash (no auth timestamp in payload). duplicate same nonce+same digest returns original receipt; changed payload409. A started receipt after crash yields unknown and never redispatches.

Response:
`{profileId,status,control,epoch,bootId,authState,origin,title,observationId,text,targets,frame,receipt,lastSavedAt,error}`.
status stopped|ready|starting|recovery_required|unavailable, control paused|owner|agent. authState login_required|reauth_required|authenticated|unknown; no false authenticated from HTTP200/cookie presence/missing QR/fill/Continue. Current unverified Zalo adapter may remain unknown; login/challenge evidence protects model output. Never claim real Zalo acceptance from fixtures.
targets `[{id,role,name,type,protected}]`, bounded256 targets,16KiB text; no current input values. frame `{id,mime,data,width,height}` base64 PNG/JPEG <=1MiB, returned ONLY owner. No frames/tool responses while model-in-protected-login phase. receipt `{nonce,status,code}` status dispatched|rejected|unknown; dispatched is input delivered, not sent message/login success.

## Browser actor

Playwright pinned1.64.1 (npm registry verified). Node24 built-in SQLite allowed, no extra db dependency. Persistent Chromium profile per namespace/bot on private /data, outside shell bot home. One process lock/replica. On graceful close preserve full profile; no live tar. Startup increments durable epoch, invalidates old observations and marks recovery_required/paused for previously active profiles; fresh site evidence required. Idle close10min; max active1 default; bounded page/action timeouts. Unprivileged browser env built from PATH/LANG/HOME only. No Railway/worker/Vault credentials inherited.

All operations serialized per profile. Taking owner control fences queued agent input immediately and drains in-flight action before acknowledging owner control; every mutation (including navigation/Vault/keys) rechecks actor/control/epoch at actual dispatch. If cleanup uncertain, paused/recovery_required; do not grant overlapping control. Expired owner lease pauses, no silent agent resume. Each action invalidates old observation; navigation/tab change invalidates targets. Owner coordinates bind frame ID/dimensions/document. Agent only opaque observed target IDs. Target validation rechecks current document/origin and target role/name/type/connectedness before input. No blind auto-retry on error/timeout.

Default egress only Zalo vendor HTTPS/WSS domains (`zalo.me`, `zaloapp.com`, `zdn.vn`, subdomains); no arbitrary external domains. Worker-owned CONNECT proxy resolves/pins public addresses and denies localhost/private/linklocal/metadata/internal addresses and bad schemes for every connection. Disable direct UDP/QUIC/WebRTC bypass, proxy no-bypass, block serviceworkers/default downloads and popup escape. Test origins permitted only by dependency-injected test harness, not runtime production env flags.

## Secrets

Vault values never in model/directives/receipt/URL/audit plaintext. New cloud Vault fill login items only; match item exact canonical HTTPS origin against live observed origin, scope to current bot/profile/epoch/observation + item version fingerprint, never automatic submit. Agent calls create existing immutable approval with kind cloud (or kind vault with signed cloud binding); consumer verifies server-issued proposal HMAC, linked Work source and all binding before fetching current item/decrypting.

For wire `sealed`: AES256GCM; derive key HMAC-SHA256(masterKey, 'aurora-cloud-vault-v1\n'+namespace+'\n'+profile), 12byte randomIV, AAD UTF8(namespace+'\n'+profile+'\n'+nonce+'\n'+expectedOrigin+'\n'+observationId); base64(iv+ciphertext+16byteTag). JSON plaintext only username/password/totp. Worker fills matching currently observed login fields, no submit, no outward action. Known secret redactions retained encrypted if persistence needed, protected inputs never in text and model cannot see login frames. Worker never receives Vault master key. Actual profile cookies protected by dedicated private volume/mode700; do not falsely claim a new full-disk encryption layer.

## Go adapter / UI endpoints

New internal/cloudbrowser typed client, aurora/cloudbrowser*.go integration; Service.Cloud plus stable namespace. API same authenticated owner/same-origin wrapper:
GET `/app/api/agents/{id}/cloud` -> state, configured flag; no worker config values.
POST suffix `/cloud/open` `{clientNonce}`; `/cloud/observe` `{}`; `/cloud/frame` `{}`; `/cloud/control` `{mode,epoch,clientNonce}`; `/cloud/action` `{kind,epoch,observationId,frameId,targetId,text,key,x,y,deltaY,url,clientNonce}`; `/cloud/close` `{epoch,clientNonce}`; `/cloud/vault` `{itemId,epoch,observationId,expectedOrigin,clientNonce,confirmed:true}`.
All body fields strict/size-limited, group IDs rejected. Screenshots Cache-Control no-store. Owner input forwarded once, pending nonce retained on transport failure; UI queries/re-observes instead of generates resend. Vault request accepts no plaintext fields. UI shows storage/privacy facts and honest lifecycle/auth/dispatch states.
Agent tools `cloud_browser_observe`, `cloud_browser_action`, `cloud_browser_vault`: same profile from Work.BotID, no model-selected bot/scope/URL endpoint. Observe never auto-opens browser. Actions park existing pendingCmd with `\x00cloud:` server-generated signed proposal; extend parked() -> immutable kind cloud and approved handler executes. Raw model APPROVAL kind cloud without server proposal proof rejected. `networkPeerToolAllowed` remains default deny; filter peer cloud direct approvals. Cloud actions excluded from displayArg dedupe; nonce owns retry semantics. Tool prompt concise, exact top-level schema; full toolReq marshaled only for known cloud names.

UI new cloud.js/css modal with per-bot selector, authenticated frame viewer, target/action controls, take/resume/pause, explicit Vault fill, no user-PC attachment. Entry in profile computer card and/or top accessible icon. Keep existing tokens, focus/Escape/mobile, textContent. app.js hooks minimal; Root handles embed.go+SW routes/cache. Approval cloud body immutable rendered exact action/origin, never mistaken email delivery. Contract changes coordinate before implementation.

## Work split

A owns Go client+Go integration/hooks/API auth+tests; B owns Node worker/browser/egress/journal+tests/package lock; UI worker owns cloud.js/css/index minimalapphooks. Root owns Docker/deployment/acceptance/docs/embedding/independentreview. No Core changes, no Wiii code copied, no unrelated fixes. New source tests must fail before implementation; run meaningful fixtures. Root deployment waits Railway reauthentication, but complete all code/fixtures independently. Real account QR/OTP human acceptance remains pending until user performs it.

## Execution checklist

- [x] Ground current Aurora/Vault and fresh Wiii source.
- [x] Compare A/B designs and independent judge; choose B with focused grafts.
- [x] Implement private worker with durable profile/lease/receipt fixture tests (30 Node checks).
- [x] Integrate authenticated Go owner/work/Vault boundary and tests, including actual Go–Node signed/sealed wire fixture.
- [x] Build owner viewer/control UI and preserve mobile/focus behavior (13 fixture checks and integrated owner-cookie UI).
- [x] Prepare Dockerfile and persistent-volume configuration. Docker image build and real Chromium fixtures now PASS after the owner authorized temporary Docker use. Railway deployment remains blocked by reauthentication.
- [x] Run real Chromium fixture, worker and container restart persistence, namespace isolation and Vault checks in Docker. Unknown outcomes remain covered by injected-driver tests, not real-account effects.
- [x] Deploy cloud candidate after Railway connection; verify private connectivity and fixture persistence on the same VM. VM destruction persistence is not implemented.
- [ ] Owner verifies actual Zalo QR/login; never claim this from fixture.

2026-10-25 review closed agent keyboard target bypass, Vault replacement-field binding, unknown-outcome UI recovery, and unbounded observation. Windows and Linux Go builds and `go vet ./...` passed. Full Windows suite retains existing platform-fixture failures; documentworker failed one concurrent run and passed three isolated repeats. Production Zalo adapter still reports identity unverified, so Go withholds model targets/content until a genuine authenticated adapter is available. This is a capability gate, not a completed authenticated Zalo agent integration.

## Docker continuation authorized 2026-10-25

The owner requested temporary Docker execution while Railway authentication remains unavailable. Docker Desktop on this PC is a fixture environment only; the account browser still belongs on a separate server for continuous use.

- [x] Build the existing worker image and run real Chromium fixtures as the unprivileged image user. Preserve sandbox enforcement with the upstream Playwright seccomp profile and its license.
- [x] Add private Docker DNS `*.aurora.internal` to the same explicit trusted-worker URL policy as `*.railway.internal`. Configuration regression failed first and passed after the minimal endpoint change. Localhost, literal IPs and arbitrary plaintext HTTP remain denied.
- [x] Supply Compose with an unpublished worker port and a named profile volume. Keep test fixtures in a separate service/profile and volume. No personal account or real Vault values.
- [x] Extend the Chromium fixture to prepare/check a fixed fixture profile across two containers on the same test volume. Persistent cookie, localStorage, IndexedDB, nonce journal and namespace isolation PASS.
- [x] Verify image health, UID 10001, restricted browser environment, real screenshots, signed control protocol and no duplicate website effect. Exact Docker image digest and source hashes recorded in local docker-validation.json.
- [x] Document Docker commands, actual PASS/FAIL/BLOCKED gates and the separate-host requirement. Previous user state and running Aurora servers preserved.

Docker execution exposed a capture race hidden by the original fake driver. The rejected premise was that the new-document event always precedes execution-context destruction. A bounded fixed-handle DOM snapshot plus a read-only stale-observation error boundary now covers both orders, with two failing-then-passing regressions. Mutations are not retried. Current Node suite PASS32/32, real Chromium single-container checks PASS6/6, prepare PASS5/5 and distinct-container persistence PASS3/3. These are fixture results, not Zalo acceptance. Docker is running locally at the owner's latest request; the real account still needs a separate server and verified Zalo adapter.

## Complete the product path, 2026-10-25

The owner rejected stopping at worker-only acceptance. Definition of done for this continuation: the actual Aurora owner UI controls the Docker browser, the real Go Vault fills a fixture form through the signed worker, the running user-facing Aurora is updated with preserved state, and the actual Zalo owner login is presented for human completion. Remote independence still requires an authenticated deployment destination. Do not call the full account integration complete before positive identity evidence.

Two independent design sketches considered a complete Docker controller with HTTPS proxy versus connecting the existing Windows controller to the private browser container. Both prefer an explicit local Docker transport for the immediate product path, avoiding state migration, new trusted certificates and owner-origin changes. The same worker and volumes remain deployable on a separate server later.

- [x] Add server-only `Config.LocalDocker` / `AURORA_CLOUD_BROWSER_DOCKER=1`. In that mode accept only HTTP literal `127.0.0.1` with an explicit canonical port 1024..65535; pin direct dial, disable environment proxies, retain no redirects/HMAC/Vault/nonce. Default production policy stays strict. Never launch host Chrome.
- [x] Add a separate local Compose override publishing `127.0.0.1:8877:8080`. Base cloud Compose stays unpublished. Surface local Docker location honestly in the owner UI.
- [x] Run the complete owner API/UI and real Go Vault against an isolated real-Chromium HTTPS fixture with separate state/volume/key. No real account secrets in tests. Keep TLS test exceptions constructor-only. Latest protocol container replacement passes; model runtime invoked cloud_browser_observe successfully on the separate fixture.
- [x] Build and replace the user-facing Aurora at port8765, preserving network-live state, Vault and model configuration. Connect its cloud panel to Docker and verify owner control/screenshot/open/close through the actual UI.
- [x] Present actual Zalo Web login from the container to its owner, without recording QR/credentials in logs/artifacts. Owner reports completion; page usability and actual profile persistence still need final visual acceptance.
- [ ] Verify the usable account page after owner login. Keep provider identity unknown unless positively verified. Obtain explicit profile-use consent through the owner UI before allowing the agent access; do not infer authentication from missing QR/cookies/HTTP200.
- [ ] Deploy the same system on an available authorized separate server and verify persistence with the user PC uninvolved. Railway currently returns reauthentication; no separate VPS access has been supplied.

The original authenticated-only gate above is superseded by a separate owner profile permission. The real driver cannot verify provider identity; pretending otherwise or blocking all profile use forever would both be incorrect. Wire responses now require an explicit protected boolean. Agent text/targets require ready + agent control + protected=false and no login/reauth state. Fresh owner handback rejects QR, challenge, blank/unusable pages and stopped/recovering state. New protected evidence immediately pauses and fences agent input. Frames remain owner-only. Node46/46 and focused Go cloud suites pass; UI24 checks and actual model fixture observation pass. Independent review remains part of final acceptance.

Actual Zalo diagnostics, with host/resource types only and no private content, identified five exact zadn.vn asset hosts. The allowlist includes those hosts only; no wildcard zadn.vn or private DNS destination is admitted. Root image context now excludes reference clones, build/state directories and credential files. No production upload or deployment has occurred while Railway authentication is unavailable.

## Cloud predeployment continuation

- [x] Build root controller Dockerfile (Go control plane + embedded UI + unchanged Core). Candidate image sha256:c2cb8494aad9233011bcf8bbd93be0cb9d42e0e01a4c1a219430f5abeeaaff13.
- [x] Run Linux controller with isolated tmpfs state and fake public owner. Five actual HTTP checks pass: anonymous denial, secure owner cookie, cloud API identity, embedded viewer assets and foreign-Origin denial. Local fixture only, not an internet deployment.
- [x] Reproduce production worker IPv6 refusal, then remove the IPv4-only listen host. Node suite47/47 passes on Windows; production listener accepts IPv4/IPv6 and rejects unsigned commands in UID10001 Linux container with network:none. New worker candidate sha256:778b8dbba0cd807a7f84a31bb810ff965483d4670cc92e72ec502b3fa47f12e2. Existing real-profile container was preserved rather than replaced during this preflight.
- [x] Add root and worker .railwayignore rules for private artifacts and reference bundles. No upload has been performed; ignore rules do not replace inspecting/scanning the actual candidate archive.
- [x] Independent focused review finds no P1/P2 in listener/test/upload-filter delta. Existing issue form schema passes. Docs checker retains the same two unrelated missing asset/license links.
- [ ] Railway still reports Unauthorized in CLI and requires reauthentication in the connector. The browserless login is waiting for the human owner. Authenticate first, inventory the existing project/service/volume configuration, preserve original secrets and state, then upload to explicit known IDs. Do not run an unlinked first-run up that might create unintended resources.
- [ ] In the actual remote runtime, prove Chromium can launch with sandbox enabled using the temporary fixture test before allowing real account login. A healthy Node endpoint alone does not satisfy this gate.

## Hosted deployment admission and erasure

Real inventory found existing production uses AURORA_MODE=hosted, not single-owner mode. Preserve that mode, its existing /data volume, Vault secret/model key and other settings. New private worker service b5e298cf-98e3-4abc-8d94-c6737840a0ce has separate /data volume ff5b2465-5fc8-4446-9d1a-fe56a03a5e40; initial deployment d40ea534-f173-4a30-946f-ad6cf3ac1b04 runs the temporary Chromium fixture as a pre-deploy gate. No controller variables were changed yet.

Hosted review requires two fixes before account use: explicit pilot email admission and erasure of the external profile on confirmed account deletion. No subscription redesign or cleanup daemon.

Private erasure contract: POST /v1/purge uses the same timestamp/exact-body HMAC as /v1/command, a <=1KiB strict body {namespace,nonce}, validated namespace64hex and existing nonce syntax. It has no owner UI route or agent tool. Go derives namespace from the current tenant's stable client, never from HTTP input. Success JSON is {namespace,nonce,status:"purged"}; validate exact matching values. A permanent SQLite namespace tombstone is recorded before cleanup, immediately rejects later/queued ordinary commands, fences active actors, drains/closes contexts before removing only profiles/<namespace> and namespace-scoped journal/redaction rows. Failure retains the tombstone and can safely retry deterministic cleanup. Duplicate completed purge succeeds without recreating data. Path symlinks/unsafe roots must fail closed; no browser or account values in response/logs.

Go writes a durable private used marker before a first browser open can be dispatched. Confirmed deletion purges a used namespace before deleting the controller's tenant locator/state. Failed or unconfigured cleanup retains tenant/namespace and reports pending deletion, allowing the owner to retry; it must not report successful erasure. Accounts with no browser use need no worker call. Standalone and hosted deletion share the service cleanup boundary; preserve existing confirmation semantics. Hosted cloud is enabled only for exact emails in server-only AURORA_CLOUD_BROWSER_EMAILS; absence disables it for hosted tenants. Standalone behavior remains unchanged. Fake tests must prove two-tenant isolation, no unconfirmed purge, failure retention, duplicate erasure and no resurrection; no real account deletion is authorized as a test.

## Measured Railway runtime choice

Standard service preflight failed before opening a page. A detailed UID10001 probe records Chromium credentials.cc:131 Permission denied, with its sandbox enabled; do not disable the sandbox. A separate clean Railway Sandbox VM e11ada6b-2e2c-40e0-8388-22b85f1d01a9 runs pinned Playwright1.64.1 / Chromium156.0.8078.4 successfully as UID1000, sandbox enabled, about:blank only. No real account data was used. Root first installed its public Node binary into /usr/local/bin because the stock root-only Node path was not executable by an ordinary user.

Use a dedicated persistent VM as the browser computer, not an agent shell VM. The existing worker code and signed/Vault protocol run there as a dedicated non-root user; Go targets the VM's private railway.internal hostname directly. The VM receives the worker HMAC key and runtime paths only; it never receives the Railway API token. Build a secret-free named base checkpoint after source/bootstrap checks, then create the actual worker VM with fresh server-provided variables. Before a profile backup checkpoint, close the browser gracefully. Do not auto-recreate, auto-restore, retry browser input or imply permanent volumes on this VM. Recovery from a named checkpoint is an explicit operator procedure with unknown outcomes reviewed.

The ordinary Railway service becomes a tiny VM keeper, with no browser/proxy/agent/control endpoints: Node builtin HTTP /healthz only, and a bounded, non-overlapping sandboxHeartbeat every60s to one configured AURORA_BROWSER_SANDBOX_ID in RAILWAY_ENVIRONMENT_ID. Reuse the existing server RAILWAY_TOKEN by Railway variable reference; empirical private API checks confirmed its Bearer authorization works (the Project-Access-Token header does not). Keep this token out of the VM and its browser. Health is ready only after a matching RUNNING heartbeat with valid private railway.internal domain, fresh within150s. Failure returns unhealthy; never provision/destroy/restore any VM. Timeout10s; sanitized fixed logs only; no raw GraphQL errors/tokens/response dumps. Separate minimal Dockerfile.keeper and railway.keeper.json, unprivileged runtime, no Chrome or user profile mounts. Root's bounded CLI heartbeat runs only during bootstrap and must stop once the keeper is proven. One VM at1CPU/1GiB; no auto-scale or plan upgrade.

## Railway acceptance, 2026-10-25

Base remains `ffab4500e3a7abab9e22d3d5375527190ff8f08b`; the working tree and explicit upload hash manifests define the uncommitted candidate. Existing WIP is preserved. No Git checkpoint or push was performed. Earlier statements about unavailable Railway authentication and the authenticated-only model gate are superseded by this deployment and the separate owner profile consent described above.

- [x] CLI authentication and explicit project/service/environment inventory. Preserve hosted mode, original controller volume, Vault secret and model key. Manual backup request was rejected as Pro-only; no plan upgrade or original-volume replacement.
- [x] Controller deployment `29d46dfe-5c41-4c07-afe9-470b7e9dac8e` SUCCESS, including hosted pilot admission, external-profile erasure and owner UI/SWv46. Public owner session loaded existing bots and opened the actual private browser.
- [x] Secret-free base checkpoint `aurora-browser-base-20261025-c14a83`; no real profile checkpoint. Dedicated VM `fd2c5f73-713a-4cfa-b19f-37a78f2d9a6f`, UID10001 worker, sandbox enabled. Only service HMAC key and runtime paths on VM.
- [x] Keeper deployment `df37fb48-b494-4c19-b0a9-a365cca87cde` SUCCESS and private health ready. Root PC heartbeat session97161 stopped. Keeper only targets that VM; no provision/restore/input APIs.
- [x] Real VM `node --test tests/cloud-browser-live.mjs` PASS6/6: screenshot, sealed Vault fixture, nonce effect, restart persistence and namespace isolation. Go–Node signed/sealed integration PASS separately. Fixtures do not establish Zalo account acceptance.
- [x] `node --test tests/*.test.mjs` PASS75/75. Focused Go client, hosted admission, deletion and adjacent Aurora tests/vet PASS. Independent final review reports no P1/P2 in this delta; full Windows-suite pre-existing fixture limitations remain.
- [x] Linux process-erasure proof: killing Node left seven Chromium descendants initially alive; they exited about104ms later. `/proc` guard now refuses purge acknowledgement while a matching profile writer remains or inspection is uncertain. Real signed purge probe PASS: pending before filesystem deletion, retry success after own fake process exits, other namespace preserved and duplicate success. No real account deletion.
- [x] Zalo QR presented through deployed owner UI at https://neko-aurora-production.up.railway.app/app/, selected existing bot Aurora. No QR/private chat saved in reports.
- [ ] Owner completes phone QR confirmation on the new VM; inspect account page, then verify real profile close/reopen when safe. Provider identity remains unknown unless positively established.
- [ ] Owner explicitly grants use of the real profile in the UI. No real messages are authorized or sent.

VM disk preserves data across browser/worker restarts while the VM exists. It is not a durable Railway volume across VM destruction. Cloud keeper maintains activity but cannot guarantee survival of platform reclamation or account/service failure. Recovery is a fresh VM from the secret-free base plus owner login, never automatic session restoration. No backup containing real profile data was created. The controller's existing volume is separate and unchanged.

Ignored operational evidence lives in `app/.build/cloud`: controller/keeper upload hash manifests, `railway-vm-live-fixture.log`, `railway-vm-crash-probe-result.json`, `railway-vm-purge-proc-proof.json`, `hosted-interop-check.log` and runtime-deployment-state.json. They contain operational metadata and stay out of commits. Do not treat the old standard-service deployment failure or old local Zalo login as the current cloud account status.

## Authorized GitHub integration continuation

The owner explicitly requested deployment, code push and merge. Pre-merge review found a crash window between durable cloud approval and durable delivery: completed Work could remain approved without a receipt forever. A new journal-reopen regression failed before the fix for both pre-dispatch and post-effect interruption. Startup now records `failed` / detail `unknown` without network dispatch before the first view sync, independent of RunTurns or endpoint configuration. Existing receipts, pending and declined approvals stay intact. The regression verifies another restart and a configured fake worker to detect replay; focused cloud tests pass, and read-only re-review reports no remaining P1/P2 in the delta. No real browser actions or account values were used in this regression.
