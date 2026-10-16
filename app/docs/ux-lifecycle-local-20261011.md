# Local user-facing lifecycle completion

Baseline: Root022a06a17966b7665d9d41658ffba8bc9a0aab46 (runtime tested179a). This is an isolated frontend-only slice. Approved light layout, account secure fields, message/group durable journal, context/service recovery proof, and backend source remain unchanged.

## Supported controls

| Control | Product surface and boundary |
| --- | --- |
| Trusted builtin enable/disable | Existing plugin card toggle, original descriptor revision/nonce; affects currently pinned authority and never repins old tasks. |
| Trusted builtin remove | Added explicit keyboard button. Existing backend tombstone retained; status says removed. Removed builtin can be explicitly enabled for new requests at a new revision; remove is hidden while already removed. |
| External descriptor import/update/remove | Existing local file import; update is now a semantic keyboard button opening the platform file chooser. Import/update remain manifest_only_disabled; no external execution or grant. Removed descriptor has no fake enable/remove action. |
| Queued pause/resume | Existing task controls; pause only never-launched waiting_host. Resume now also requires observable original bot/account/channel/resource/plugin authority. Backend remains final authority for hidden ancestry/leases. |
| Supplement/replace | Existing named textarea dialog. New linked obligation uses original scope and policy. Known changed bot/account/channel/resource/plugin pins now hide these controls. No mutation of the running Core prompt. |
| Cancel | Existing scoped card/chat action and canonical durable V1 command. Remains available even when original authority is withdrawn. Does not cancel unrelated work or roll back effects. |
| Service recovery | Existing d7 capability/stopped native generation/current executor/original-context gate and durable V2 command unchanged. Positive stop proof in owned UI suite is typed fixture; Root native179a-r2 is separate recorded acceptance. |
| Group create/cancel | Existing authorized exact-channel three-stage controls and original nonce/scope unchanged. Group stage task controls stay hidden. Group recovery awaits separately sealed backend capability/receipt; no proposed or fake action is added here. |

The plugin update/remove buttons expose data-plugin-action=update/remove, and toggle uses data-plugin-action=toggle. Existing pluginCards IDs and task [data-action] selectors remain stable. Button accessible names begin with visible action plus plugin name; mutation focus falls back to the dialog close control through the existing page/navigation guards.

## Pending-command behavior

Plugin lifecycle retains original body/revision/nonce in page memory and only a bounded metadata marker in localStorage. Lost ACK page retry uses the exact original body. After reload the marker has no manifest/body/receipt-kind reconstruction; only read/review is offered, with no automatic POST or newly minted nonce. Task/group commands keep the existing canonical durable journal, exact two-counter CAS and receipt lookup. This change does not persist hidden context, file excerpts, raw plugin manifests or credentials.

## Validation history

RED inventory on baseline app source: builtin remove unavailable; external update not keyboard button; known missing account still allowed resume and linked controls. Other inventory checks passed. Evidence under .build/evidence-lifecycle-red retains the four failing assertions.

Working01: lifecycle30 PASS. Working02/03 retained failures for the truthful-status assertion because the Windows script path encoded its Vietnamese test literal as mojibake. Authored source string was correct; assertion uses explicit Unicode escapes. The async test completion wait was also tightened to observe the new plugin revision and retired marker rather than an initially empty marker map. These are harness changes, not product fixes.

Working04: lifecycle36 PASS with actual owned Go journal/API/embedded assets and headless Chrome. Keyboard Enter opened Chrome's intercepted native file chooser; update lost-ACK retry posted identical request bytes and stored only the bounded marker. Additional typed public-metadata probes check known account/bot/plugin revocation hides continuation while scoped cancel remains available. Existing actualGo/headless plugin64, durable51, recovery61 and full Go tests passed in the working tree. Exact clean-commit reruns, source/runtime blob bindings and artifact hashes are recorded in the sealed handoff packet.

No Core process/model/provider call, paid service, native Tauri window, physical keyboard/phone, cloud, external installation or grant is exercised by this slice. The reused standalone Rust executable provides unchanged Windows DPAPI sealing only; it is not current Root native-host acceptance. Upload reload receipt UI is a separate follow-up after immutable backend proof; no proposed endpoint is called here.
A bounded follow-up on the short dialog confirmed the update button's full 44px rectangle and all four corner hit points are inside the scrollport after explicit scrolling. The post-scroll screenshot is retained. No production CSS correction was needed; candidate0c assets remain identical. Final exact lifecycle suite contains37 gates.
