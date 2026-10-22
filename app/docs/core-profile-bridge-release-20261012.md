# Core profile bridge candidate, 2026-10-12

Neko Aurora can use a supported profile from a Core 1.7-shaped configuration without asking for its key again. The compatible bundled Core reads the selected credential itself. Discovery, profile inspection and bounded execution have separate permissions. The installed Core and existing application are preserved.

## Delivery and identities

Candidate: `E:/CodexScratch/core-profile-bridge-20261012/candidate-bc10baf`.

Application code: `bc10baffe68b466683f1d23ccc9e5c4e2254cd68`; product code checkpoint: `069f1a7c53098a0a8887418ba4f4b01575377db5`. The later commit changes only the native test's deadline fixture and failure observations. The `native`, `host`, `desktop` and UI Git objects are identical between them. Core: `0aa0771ba47e6803018297a01d51427c9e61283f`.

The candidate's launch receipt pins every executable. The control binary is compiled at `bc10baf`; host and Tauri binaries are built at `069f1a7` from the identical product source. Core's executable SHA256 is `81e5facc66453fe15cfc98159b395f45bcfffb39980d724ade51d677eaca10ae`; its unchanged writer helper SHA256 is `f9f404a07a7fe1aa56a2b7d3c073a2330366de5ee26d572b37421338e2b99a3a`. See `release-summary.json` and `artifact-manifest.json` for commands, all binary identities and evidence hashes.

## Minimal user-owned trial

1. Run `Start-CoreProfile-Candidate.ps1 -CheckOnly` to verify the package. This starts no service and reads no profile.
2. When ready to open this candidate, run `Start-CoreProfile-Candidate.ps1`. It uses port 8767 and separate state. Open `http://127.0.0.1:8767` in your browser. An occupied port is refused; the launcher stops no other process.
3. Open **Kết nối của bạn → Dùng profile có sẵn**. Discovery lists the trusted source without reading its profiles. **Cho phép xem profile** lets the bundled Core inspect public metadata from your own Core home. Inspecting or saving this selection makes no provider request.
4. Select an eligible profile, review its provider origin, channel/bot scope and limits, then approve execution. A new message explicitly uses this consent. Sending that message is the step that calls the selected real provider; the development tests have not performed that real-provider trial.

No key needs to be entered into Neko Aurora for a supported stored profile. The task's memory, trust and artifact home are separate from the credential home. Revoke in Connections stops selected work; profile rotation and deadline expiry also fence its output/effects.

Initial support is file-backed API-key or explicit no-key direct transports: OpenAI-compatible, Responses, Anthropic and Kimi. Owner-home execution requires HTTPS. OAuth, environment-only credentials, installers, dynamic transports and sidecars sharing a user session home remain unsupported. `configured` and `not_checked` describe configuration and untested authentication readiness respectively. Discovery of an installation does not prove its profile is eligible.

The current Tauri shell uses port 8765. Its release build is included, but the isolated port-8767 candidate is opened in the browser. Opening a native window against a separate candidate, physical keyboard/phone/AT speech, cloud and real provider/model quality remain separate acceptance gates.

## Completed validation

| Gate | Exact identity | Result and scope |
| --- | --- | --- |
| Full Go test/vet/build | App `069f1a7` | 484 top-level + 479 subtests passed; two optional fixture tests skipped; the web package has no tests. Genuine old native profile journal replay included. |
| Final control build and targeted Go | App `bc10baf` | PASS; native/cmd/runtime/protocol sources match `069f1a7`. |
| Rust host fmt/test/release | App `069f1a7` | 30 passed; trusted profile, manifest and native-launch boundaries. |
| Tauri release build | App `069f1a7` | PASS offline; compile evidence only, no window acceptance. |
| Affected Core tests/types/lint/build | Core `0aa0771` | 257 tests/2003 assertions passed; compiled metadata/initialize fixture 7 passed. |
| Actual native profile bridge | App `bc10baf`, Core `0aa0771` | 24 passed using real Go/Rust/Core, synthetic old-style file profile and owned loopback model. Includes genuine completion, split credential echo, nonce retry/restart, rotation, explicit revoke, deadline and real stopped process HANDLEs. |
| Actual embedded UI journey | App `069f1a7`, Core `0aa0771` | 14 passed using real Go/Rust/Core and headless Chrome; four embedded asset hashes match source, file key/model used without reentry, no inference before request, short-mobile revoke reachable. |
| Authored profile UI catalog | UI `53fd864` | 50 Chrome checks passed; manual-account VM 13 passed. This is an authored endpoint fixture suite. |
| Independent bounded review | App `069f1a7`, Core `0aa0771` | Public ACP whole/split/error 3 passed; fresh/history registry 1 passed. Prior `9eacf45` composition probes 5 passed separately. No remaining confirmed P1/P2 in the reviewed scope. |

The failed eight-second native deadline round is retained. The fixture's Core process was created about 4.69 seconds after intake; the task session was finalized just after its original deadline and stopped before admission. The final test changes only its cap to 30 seconds to exercise expiry after a genuine running process. It does not weaken product expiry or grant rules. Earlier catalog/nonce-oracle failures, public-ACP resource/budget harness corrections and causal credential-echo negatives are also retained.

Known literal/split selected credentials are protected at public ACP sinks. No claim is made to infer arbitrary encoded exfiltration or to retroactively scrub historical unsafe output. Old runtime fingerprints remain readable for receipts/history, while fresh source adoption, binding and admission require the corrected pin. Private source/config/key/journal/browser-profile directories are excluded from the delivery evidence.

## Next acceptance

The next useful gate is the human-owned profile inspection above, followed by a separately authorized small real-provider request if the profile is eligible. Root has not read a real profile/key, renewed tokens, granted production access, changed the installed Core, deployed or pushed this candidate. Check `preservation-after.json` for the final comparison of protected files, ZIPs, old binaries and installed executable.
