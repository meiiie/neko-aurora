# Long-chat resize validation — 2026-10-29

Test commit: `9c9c36dc0f1b335f5a7569b8c026e34ecb076778` on `codex/grokbot-adoption`; production runtime unchanged from `2107c7e513a71cc4e7a14fcb8d45f7baea672d1e`. This validates existing behavior corresponding to [Grok Bot 0.78.1](https://x.ai/changelog/bot#v0.78.1), Fixed-2. One agent followed pstack Feature / Prove It Works / Show Me Your Work and webapp-testing, with implementation decisions and self-review in the local decisions.tsv trail. No subagent, model/provider request, account, grant, push or deployment.

The initial 100-message probe kept message 70 within 0.25 pixels on desktop/mobile/desktop resize. A broader regression fixture passes without changing the existing browser anchoring or application scroll handlers. The result is validation of existing behavior, not a claim that a reproduced product defect was repaired.

## Coverage

Seven scenario groups pass in the committed [browser fixture](../../app/tests/aurora/chat-resize-browser.py): repeated width and height changes keep the visible message within two pixels; resize within a long paragraph keeps its offset; a subsequent wheel moves locally without a late jump; expanding the rendered window from 60 to 100 messages preserves context; DOM text selection survives resize without jumping to the bottom; explicit bottom-follow stays at the latest message; and switching chats does not reuse the old reading location or send a message. Viewports include widths 390, 460, 900 and 1280 pixels and heights 550 through 900 pixels. The selected-reading screenshot was inspected.

These checks use real Chrome, synthetic loopback messages and disabled EventSource connections. They do not validate physical-device scrolling, native WebView behavior, Firefox/WebKit, live model streaming, asynchronous image layout or arbitrary font loads. No additional runtime code was justified by the tested cases.

## Validation

Command from repository root: `python -u app/tests/aurora/chat-resize-browser.py`. Windows amd64, PowerShell 7.6, Python 3.13.7 and Chrome 156.0.8078.49. A credential-excluding environment is used by the local runner. `resize-browser-134940.log`: **PASS**, seven groups, 6.4 seconds. All browser page errors and unexpected POSTs must be absent. Exact source snapshot and command are in `.audit/2026-10-29-chat-resize/validation.jsonl`; measured positions are in reading-positions.json.

Final source manifest: `b47ef6020ef36174027022368c9c609a8b769924ce9c704d446c1d03fe356336`; 1840 source files verified against Git with LF normalization accounted for. validation-source-equivalence.json confirms the only source change since the prior browser validation is this new test. The [IME report](grokbot-ime-input-2026-10-29.md) retains 168 browser groups, 27 Node checks and build/asset evidence; the [Vault report](grokbot-vault-contacts-2026-10-29.md) retains 2048 Go PASS / 38 SKIP across 25 packages. Those suites were not repeated for a test-only change. Documentation uses `python tools/check_docs.py` and explicit local-link resolution.

The pre-checkpoint recognizable-secret scan covered 1 candidate file and 639 newly reachable history blobs, with zero matches. It is not exhaustive secret detection. The unrelated pre-existing adoption-document edit remains excluded.

The [incremental adoption index](grokbot-adoption-index-2026-10-29.json) now marks Fixed-2 as partial local validation. Across the two indexes, 31 source bullets have partial evidence, one is excluded to preserve safeguards and 888 remain unvalidated. These 920 bullets are not independent feature counts. The broader Grok Bot backlog remains open.
