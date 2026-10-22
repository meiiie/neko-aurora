# Neko Aurora — independent acceptance audit, 2026-10-11

## Kết luận và phạm vi

Bản local có foundation thật: Go journal trước ACK, Rust/ACP admission, Neko Core làm agent loop, context pin tại intake, bot registry, DPAPI account cards và artifact có receipt/hash. **Chưa đủ nghiệm thu sản phẩm, cross-channel, provider thật, cloud hay điện thoại.** Có một lỗi P1 đã tái hiện có thể chặn toàn bộ service sau lỗi ghi tệp rồi cancel; hai regression frontend P2 và một thiếu invariant completion cũng đã tái hiện.

Audit source commit **`3cb9b6ff12195b432e1c5c9d69b0db9b56699229`**, context-pin predecessor `551ed538983d1c8a07201f988bf719234d69bdf0`. Core pin đang dùng là **1.8.0 / source `3d5725f222815e15b12a212e372cf17057a459c0`**, Windows binary SHA `4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36`.

Không audit thay đổi permission/account-lease ở dev clone C hoặc Core writer-v2 chưa tích hợp. Không sửa app/runtime/frontend, .git, reference hoặc process của worker. Không mở browser/window, không chạy Grok, không gọi provider thật, không tạo grant, deploy, push hay checkpoint Git. Chỉ thêm báo cáo này vào E nếu chưa tồn tại; probe, build và cache nằm trong thư mục audit riêng.

Bản sao đọc từ E nằm tại `C:\Users\Admin\Documents\Codex\2026-10-11\task-2\acceptance-audit\snapshot`. Các file tracked được đối chiếu với `git show <commit>:<path>`: không khác nội dung sau chuẩn hóa CRLF/LF. Bốn generated Tauri schema không tracked được ghi riêng, không được xem là bằng chứng commit. Snapshot không phải transaction atomic toàn cây; manifest từng file bảo toàn byte/hash. Integrator báo E clean tại commit trên; `git status` trong sandbox của audit không đọc được worktree, nên auditor không tự xác nhận clean bằng status. `source-manifest.json` ghi chi tiết.

Đã đọc AGENTS, `neko-contest-quality`, phần liên quan của direction 2026-10-11, ADR, checkpoint Core Windows, `context-pin.md`, `connections-local.md`, `next-core-adapter.md`, `ux-reference-mapping.md` và `.build/HANDOFF-UI-20261011.md`. Quy trình contest/provider/deploy không được áp dụng cho audit offline này. Những tham chiếu trực tuyến trong direction không phải bằng chứng sản phẩm và không được kiểm lại ở lượt này.

## Findings và reproduction

### F1 — P1, xác nhận: cancel sau effect-write failure làm startup thất bại toàn cục

**Source:** `native/internal/ledger/runtime.go:378` (commit operation_started), `:381`–`:396` (IO sau intent), `:455`–`:456` (reconcile yêu cầu status running); `native/internal/ledger/store.go:379`–`:395` (cancel giữ operations, đổi status/epoch); `native/internal/runtime/manager.go:106` (startup gọi RecoverInterrupted). Validator `runtime.go:205`–`:231` cũng giới hạn receipt reconciliation ở running.

**Repro độc lập:** trong temp, tạo task bằng helper fixture `running`; tạo directory ở đúng đường artifact hash để `O_EXCL` thất bại sau commit intent; `ExecuteTool` trả `ErrUnavailable` và giữ operation `started`; gọi `Cancel`; đóng/mở store; gọi `RecoverInterrupted`. Kết quả: **`journal_corrupt`**. Đây là trạng thái do API hợp lệ tạo ra, không phải journal bytes bị sửa. Runtime.New sẽ từ chối khởi động trước khi API/snapshot được mở. Trong process thật, API cancel có cửa sổ khi transport đang shutdown hoặc task đã chuyển recovery_required; probe này kiểm trạng thái store, chưa inject race vào Core process thật.

**Probe:** `snapshot/native/internal/ledger/independent_audit_test.go`, `TestIndependentCancelAfterStartedEffectDoesNotBlockServiceRecovery`, log `go-all-with-probes.txt`.

**Hướng sửa:** tách reconciliation receipt khỏi lifecycle hiện hành của task. Cho phép đối soát một intent đã tồn tại trong cancelling/cancelled/recovery_required/suspended nếu identity/digest đúng, giữ nguyên epoch/desired state và không hồi sinh hay công bố kết quả bị thu hồi. Missing/partial/mismatched bytes là unresolved outcome của task; không được biến thành corruption toàn journal hoặc tự retry mutation. Mở rộng event validator nhất quán với đường reconciliation, không chỉ bỏ kiểm tra ở :455.

**Test phải thêm:** failure trước create/sau partial write/sau file sync/trước artifact ACK; cancel trong cửa sổ này; restart hai lần; bytes đúng/thiếu/sai; task khác vẫn intake/foreground; không tạo effect thứ hai, không đổi identity/epoch, không phát late result như kết quả hiện hành. Đã gửi integrator ngay khi tái hiện.

### F2 — P2, xác nhận invariant thiếu: completed chấp nhận operation còn started

**Source:** `native/internal/ledger/runtime.go:129`–`:132`, `:253`–`:258` chỉ kiểm running → completed, không kiểm outstanding operations.

**Repro:** dùng cùng fault injection của F1, rồi gọi `Finish(id, epoch, "completed", "")`: trả success dù operation còn `started`, không có artifact receipt. Probe `TestIndependentUnresolvedEffectCannotMarkTaskCompleted` FAIL.

**Giới hạn:** đây là invariant của store/internal trusted caller. Transport Go hiện ngắt khi ExecuteTool trả error và Rust còn kiểm Core hostEffects; chưa chứng minh model/UI có đường trực tiếp ép transition này. Vì vậy không gắn P1 hay tuyên bố exploitable remote.

**Hướng sửa/test:** completion chỉ hợp lệ khi mọi effect nội bộ đã terminal với kết quả cho phép completion. `started`/`unknown` phải giữ trạng thái blocked/recovery riêng. Kiểm cả đường update trực tiếp, replay journal và future permission-resume; không trộn "Core end_turn" với hoàn tất nghĩa vụ cần tệp.

### F3 — blocker requirement trước cross-channel: @prefix local chưa giữ nguồn/đích/audience

**Source:** `native/internal/ledger/store.go:41`–`:46` (Input chỉ bot/nonce/text/lane), `:347`–`:356` (đổi ConversationID sang bot đích trước captureContext); `native/internal/ledger/context.go:42`–`:49` (chọn toàn bộ completed history eligible của conversation đích); `native/internal/web/assets/app.js:41` và `:52` (lọc theo bot đích, chuyển bot sau ACK).

**Repro:** hoàn tất một cuộc trò chuyện Davin chứa canary private; từ Meiiie gửi `@davin answer here with this request only`. Work được gắn `conversation-bot-davin`, và prompt chứa reply canary từ history Davin. Probe `TestIndependentTagDoesNotUseUnrelatedTargetPrivateHistory` FAIL ở cả destination và context. Test registry sẵn có chỉ chứng minh **history của bot nguồn không đi sang bot đích**, chưa kiểm chiều private target → audience được tag.

**Phân loại đúng:** đây là direct-routing giữa hai cuộc trò chuyện private của cùng `local-owner`; tài liệu handoff gọi đúng scope đó. Nó chưa phải cross-owner leak, và không tự vi phạm quyền hiện hành nếu người owner chọn private bot đích. Nó **chưa đạt REQ-PB-004/005/006**, direction `:898`–`:906`, SEC04/06/42, nếu đem dùng làm @tag tại kênh chung. Không được mở Slack/cloud/Project bằng cách reuse giả định này.

**Hướng thiết kế và test:** giữ verified actor/channel/sourceConversation và publication destination độc lập BotID/runtimeConversation; một ContextResolver kiểm read ACL và audience ACL, ghi nguồn/version/hash và decision receipt. Tag không tạo grant. Thử source-private/target-private hai chiều, channel có người khác, bot con không phải owner, đổi destination, revoke giữa queue và dispatch, replay event không đổi audience. Chưa có connector hoặc delegated child work graph để thử end-to-end.

### F4 — P2, xác nhận offline: một outbox entry lỗi vĩnh viễn chặn replay entry hợp lệ phía sau

**Source:** `native/internal/web/assets/app.js:11` `resumeOutbox` bắt mọi error rồi `break`; `:52` giữ entry khi POST 400/404/409 và không có discard/edit state.

**Repro:** outbox gồm `@unknown` (server 404) rồi một message hợp lệ; gọi function nguyên bản với request stub trả 404 cho entry đầu. Chỉ nonce đầu được request, entry sau không được replay. Ngay cả khi sửa composer và gửi thành công, entry cũ vẫn còn; đủ 16 entry sai có thể khóa enqueue mới.

**Bằng chứng:** `frontend-probes.cjs`, `frontend-probes.json`, `outbox_permanent_404_does_not_block_later_valid_entry=false`. Đây là Node VM của function source thật; chưa phải browser acceptance.

**Hướng sửa/test:** phân biệt lỗi transient với rejection vĩnh viễn; giữ entry failed với nonce và thông báo sửa/hủy rõ, tiếp tục entry độc lập hợp lệ. Không silently drop việc người dùng. Test reload/reconnect sau invalid mention, invalid frame, nonce conflict, 5xx và mạng mất ACK; đúng nonce được giữ khi chưa biết đã nhận hay chưa.

### F5 — P2, xác nhận offline: ACK cũ đổi bot hiện hành sau khi người dùng đã chuyển

**Source:** `native/internal/web/assets/app.js:52` gọi `selectBot(result.work.botId)` nếu bot trả về khác bot **hiện tại**, không kiểm composer generation/source bot còn active.

**Repro:** submit Meiiie, giữ POST response; user chuyển Davin và nhập draft; trả ACK Meiiie. Function submit nguyên bản chọn lại Meiiie. Probe `late_message_ack_preserves_current_selected_bot=false`. Draft có thể còn trong view cache, nhưng context/focus của người dùng bị kéo sang bot khác.

**Hướng sửa/test:** luôn lưu ACK/work, chỉ auto-select destination do explicit routing khi user vẫn ở source context/generation của submit. ACK stale không đổi selection hoặc focus. Test chuyển bot, mở account dialog, sửa draft, hai request xen nhau và SSE đến trước ACK. Đã gửi integrator để chuyển UI owner; audit không sửa frontend.

## Requirement coverage: implemented / tested / proposed

| Yêu cầu | Implemented tại 3cb9b6f | Bằng chứng đã chạy / đã đọc | Chưa đạt hoặc chưa thử |
|---|---|---|---|
| DIR04, PB001/002: Tauri/Rust/Go/Core | Có shell Tauri, Go control plane, Rust process/ACP guard; Go không gọi model riêng | Audit Go build/vet, Rust unit/DPAPI/conformance PASS; process/UI evidence commit cũ và 3cb đọc riêng | Audit không tự build/run Tauri hay Core process; signed installer/update/rollback chưa có; UX chưa được user chấp nhận |
| PB003: bot lâu dài | Meiiie/Davin + create registry, ID/alias/nonce bền vững | Original registry/restart/conflict tests PASS | Chưa edit/rename/profile lifecycle, account/config authority pin đầy đủ; description là metadata, không thêm model authority |
| Durable task/intake context, PB005/007 một phần | Request/message/context pin cùng transaction/fsync trước ACK; copied source bytes, IDs/epochs/artifact refs, 8 sources/8KB; dispatch không đọc live history | Context queue race/restart/nonce/isolation/forgery/bounds tests PASS tại audit; integrator có Core prompt suite PASS3 tại 551ed53 | Không project ACL/retrieval/document selector/permission invalidation tại commit này; artifact bytes được giữ nhưng không có host read tool để Core đọc lại file chỉ từ hash; legacy records không pin cố ý không import ambient history |
| PB008 foreground/background | Core session/request riêng; slot bot:lane; max4 active, max3 background dự phòng 1 foreground | Code manager.go:121–:150; exact ef5dd4a fixture E 6.036s, physical E-state 5.970s; 3cb native UI/Core checks đọc được | Chưa stress saturation/fairness/p95/quota/model thật; queue vô hạn theo thời gian; một turn cùng foreground lane vẫn tuần tự. Không gọi fixture latency là SLA |
| PB004/006 tag/cooperation | @prefix đầu text resolve một bot + SourceBotID/RoutedAlias | Registry source-context exclusion PASS; audit F3 ghi behavior thực tế | Không source channel, verified actor, ACL nơi công bố, reply-in-place, many-tag delegation, child wake/work graph/stop aggregate |
| PB009 files | UTF-8 .md/.txt tối đa 24KB, 8 effect/task; intent trước IO, immutable ID/hash/receipt, download kiểm hash | Artifact dedup/conflict/restart/fence/path tests PASS; original effect-before-ACK unit PASS; historical actual Core artifact/download PASS | F1/F2; không upload/attachments, arbitrary file types, versioned input/file-read capability hoặc cloud object retention. Core process suite chưa kill đúng cửa sổ intent→IO→receipt→ACK |
| Replay/idempotency | Global ordered journal/SSE cursor/snapshot, one writer, nonce dedup, torn-tail discard và corruption refusal; browser outbox same nonce | Original Go ledger/API tests PASS, historical two-client local suite PASS; F4/F5 FAIL offline | Không external exactly-once, database/cloud sync, logout cache policy. Cần giữ permanent errors rõ và stress multiple concurrent clients |
| PB011 DPAPI/account/secure input | Nhiều stable account card, revision CAS, nonce matching secret trong Rust; stdin, DPAPI Windows user + account entropy; public projection không secretRef/plaintext | Audit DPAPI roundtrip/wrong account PASS; Go account/revision/plugin tests PASS; ef5dd4a actual host/API & 3cb UI canary evidence đã đọc | Tại commit này account vẫn stored_unverified, Core dùng fixture-only; **không account-bound runtime lease, credential generation/live rotation/OAuth/provider revoke**. Remove chỉ metadata local; encrypted orphan cleanup và provider revoke chưa có |
| Secure card lifecycle | Submit/provider/tab/close/Escape clear secret; generation/AbortController chống stale provider result; textContent render | UI tests :88–:104 tập trung provider-change race đã pass trong evidence 3cb | Endpoint/model/name change trong pending submit không bump generation; cần test destination/edit race. Không native secure credential broker surface, mobile app-switch preview hoặc real-model secret-echo eval |
| IPC/capabilities | Tauri capabilities=[], không invoke_handler/custom commands; HTTP loopback cookie + Host/Origin/content/frame checks; CSP/textContent; raw ACP receipt/root/activation/exact tool ceiling | Audit API boundary PASS3; Rust admission/operation/conformance PASS; static desktop main/config | Chưa negative native invoke from unauthorized WebView/navigation/iframe suite. CommandGate native fixture trong lib/conformance vẫn khác raw ACP; không lấy test envelope làm proof dispatch authority. Không OS sandbox hoặc multi-owner auth |
| Core crash/recovery | Go loss → recovery_required; resume giữ Core task/session, rotate activation; close/cancel trả disconnect và chặn tool mới | Historical native process suite PASS supervised Go-kill resume và fail-closed forced Core kill | **Forced Core kill recovery BLOCKED** v1 stale writer lock; lock không xóa mù. Writer-v2 patch ở clone khác chưa audit/pin/compile/integrate. F1 là blocker khác ở Go ledger |
| PB010 phone/cloud/device | Local SSE trên cùng service; responsive 390px và drawer | Headless19/native21 evidence tại 3cb ghi đúng scope; screenshot đọc trực tiếp | Không remote/device identity/cloud auth/DB/object sync/device lease; không physical phone, suspend/resume/keyboard/OAuth callback/push/reconcile, offline cross-device conflict |
| PB012, UX | Original graphite UI, keyed DOM, IME guard, draft/scroll preservation, tabs/safe form | Current 3cb desktop+secure sheet PNG đọc; tests ghi visualAcceptance=screenshots_require_review | User đã reject UI cũ; không dùng PASS15 cũ hoặc PASS21 mới để gọi visual accepted. Screenshot quality/flow feedback và redesign worker còn mở; F4/F5 có thể ảnh hưởng trải nghiệm |

## Evidence audit và lệnh chính xác

Môi trường audit: Windows amd64, Go **1.27.2**, Rust/Cargo **1.97.1**, Node **25.9.0**, Python **3.13.7**. Root riêng `C:\Users\Admin\Documents\Codex\2026-10-11\task-2\acceptance-audit`; TEMP/TMP=`<root>\temp`, GOCACHE=`<root>\go-cache`, GOMODCACHE=`<root>\go-mod-cache`, GOPROXY/GOSUMDB/GOWORK=`off`, GOENV=`off`; CARGO_TARGET_DIR=`<root>\cargo-target`. Go exe đọc từ `C:\Users\Admin\Documents\Codex\2026-10-11\task\.tooling\go\bin\go.exe`. Không tải dependency hay dùng provider ngoài.

| Command, cwd tương đối audit root | Kết quả | Scope/evidence |
|---|---|---|
| `go test -count=1 -v ./...`, `snapshot/native` + independent_audit_test.go | FAIL | 3 independent probes phát hiện F1/F2/F3; 3 API tests ban đầu BLOCKED connectex sandbox, không phải product assertion failure. `go-all-with-probes.txt` giữ nguyên kết quả đầu |
| `go test -count=1 -v ./internal/api`, `snapshot/native`, scoped sandbox escalation | PASS | 3 tests loopback sở hữu riêng; `go-api-elevated.txt`; không có auto-review rejection |
| `go test -count=1 -v ./internal/ledger -skip TestIndependent`, `snapshot/native` | PASS | 18 original ledger tests, không chạy probe của audit; `go-ledger-baseline.txt` |
| Original `connections` và `protocol` trong lệnh all đầu | PASS | 4 account/plugin tests + canonical/bound cases; không có network |
| `go vet ./...`, `snapshot/native` | PASS | Static analyzer; không phải acceptance functional mới |
| `go build -o ../../neko-control-audit.exe ./cmd/neko-control`, `snapshot/native` | PASS | Binary riêng trong audit, không dùng chung .build E |
| `cargo test --offline --locked`, `snapshot/host` | PASS | 3 unit (raw ACP + actual Windows DPAPI), 7 conformance; `rust-tests.txt`; không Core process/native UX |
| `node frontend-probes.cjs`, audit root | FAIL | F4/F5, function source thật với deterministic VM stubs; `frontend-probes.json`; không browser/window |

Lần thử đầu thiếu `go.mod` trong snapshot selector đã FAIL setup trước test; sau đó copy riêng go.mod và chạy lại. Không tính lỗi harness này là product defect. `validation-independent.json` và `source-manifest.json` giữ command, code revision, source/evidence hashes và scope. Go binary audit hash nằm trong validation JSON. Probe file chỉ tồn tại ở bản sao audit, không nằm trong source commit.

Evidence được **đọc**, không chạy lại để nhận công: `docs/evidence/2026-10-11-native/validation.json` với ef5dd4a; process/Core/DPAPI reports tương ứng; `.build/evidence/3cb9b6f/ui-headless.json` (19 checks), `ui-native-core.json` (21 checks) và PNGs. Header `pass` không xóa blocker `forced_Core_crash_resume`; forced kill check pass chỉ chứng minh từ chối an toàn, không chứng minh resume. Kill-Go suite `scripts/e2e_core.py:99`–`:111` chờ Rust/Core đóng qua ACP; không tương đương hard kill toàn execution chain. Suite artifact `:82`–`:98` hoàn tất rồi duplicate inbound; chưa fault-inject mất tool ACK tại process boundary.

Checkpoint ef5dd4a còn nói context đọc lúc dispatch/notImplemented immutable pin; điều đó đúng cho commit lịch sử ấy. `docs/context-pin.md` và source 551ed53/3cb mới đã thay đổi. Không được trích validation cũ như trạng thái current, cũng không tự gán process evidence cũ cho mọi thay đổi mới. Source bảo toàn reference/provenance còn vài dòng planned-work cũ; cần nhãn historical để tránh mâu thuẫn tài liệu.

Static Grok reference đã đọc trực tiếp ở `reference/extracted/grok-0.68.1/client/dist/renderer/assets`: scroll provider/create roster ở `index-ByKx0M82.js` offsets 647533/732079; create/select/send ở `chunk-view-C45JaL8U.js:58990`; account-key ở `chunk-plugin-detail-view-Bg5zQ-OF.js:11120`; safe-input outcomes ở `chunk-view-DAE5cvtv.js:15378`; interruption scoped agent/session ở `chunk-view-CGZ4P0Uu.js:23900`. Offsets là UTF-8 decoded character offsets, không dòng. `grok-static-read.json` giữ source hashes và đoạn đọc ngắn ở thư mục audit. Đây chỉ là tham chiếu UX; không chạy bundle hoặc chép code/font/icon/assets, không coi Grok là backend hay license tái sử dụng.

## Checklist còn thiếu theo thứ tự phụ thuộc

1. **Sửa F1 trước mở thêm permission states hoặc account lease.** Receipt reconciliation độc lập lifecycle, no duplicate/no resurrection, service khác không bị chặn; test fault boundaries + cancel/suspend/restart trên filesystem và actual Go/Rust/Core process riêng.
2. **Khóa completion/effect contract F2.** Task nghĩa vụ khác Core end_turn; internal started/unknown không hoàn tất; mất ACK đúng operation ID phải dùng receipt/hash, không sinh op mới. Kiểm activation change và actual Core unresolved effects, giữ typed blocked reason.
3. **Tích hợp safe Core writer recovery qua contract/pin mới.** Owner/process identity, lock ownership, clean versus dirty recovery, live writer không bị lấy khóa, fail-closed malformed/partial/replaced state; compile đúng source/lock, repin hash, cùng session/task identity. Gate vẫn BLOCKED đến khi test exact pair; không giải bằng xóa lock heuristics.
4. **Permission suspension và account runtime lease.** Intake pin account ID/config revision (không token); live check trước dispatch/tool; revoke queued/running, lease expiry/fence, active-use teardown, no silent default-account switch, credential generation/rotation, provider revocation status khác local disconnect. Thử entirely synthetic broker trước quyền dùng provider thật; work C clone hiện chưa được audit trong báo cáo này.
5. **Giải F4/F5 và secure-form destination race trên frontend owner.** Browser/Tauri owned profile kiểm reject-outbox hồi phục, late ACK không đổi bot/draft/focus, endpoint change khi pending, secrets không vào chat/localStorage/analytics/log. Không dùng Node VM để gọi UX accepted.
6. **Cross-channel/Project identities và ACL trước connector.** Source channel/verified actor/audience, reply-in-place, permission-bound context resolver, immutable file versions, delegation provenance/child wake/aggregate receipts, stop epoch. Test source và target private canaries hai chiều; F3 hiện chỉ là private direct-routing.
7. **File input + retained context usable.** Upload/rejection states, version/hash và retention, host read capability theo ACL, symlink/junction/TOCTOU handle tests, tampered file download, restore đầy đủ journal/artifact/vault khác code checkpoint.
8. **Đo scheduler và boundary native.** Three-background saturation, nhiều bot/foreground, CPU/RAM/quota/fairness/queue bounds; owned native WebView hostile IPC/navigation/iframe tests. Không nâng concurrency dựa số test hay một foreground fixture latency.
9. **Cloud/device/mobile gates riêng.** Auth/owner isolation/DB/object/cursor sync, leases/revoke, backup-restore/unknown_outcome, genuine two-device offline/reconnect; physical phone keyboard/background/resume/app preview/OAuth/push reconcile. Local journal/SSE và 390px không thay gate này.
10. **Nghiệm thu UX rồi model/release riêng.** Review screenshot/click flow mới với user; bỏ tiêu chí visual cũ đã reject. Provider thật/GLM exact model quality chỉ chạy sau separate authorization, config và budget được xác minh. Installer signed/update/rollback/license/distribution checks riêng; không gọi bản local đã complete vì build/test count.

## Bàn giao và giới hạn còn lại

F1 và F4/F5 đã gửi trực tiếp runtime/integrator thread `01a1299d-c1f3-72db-9e36-eafba745401c` kèm source locations/probe/hướng sửa. Gửi source parent `01a118a0-97fb-7590-8a6e-8c734519c0ed` trả `thread not found`; integrator đã được báo để chuyển tiếp. Platform final notification là đường bàn giao còn lại.

Các lỗi/gates trên là kết quả tại 3cb9b6f, không đánh giá patch chưa tích hợp hay phủ nhận evidence đúng scope đã có. Cần auditor kiểm lại **diff và targeted failed probes** sau integrator sửa; chỉ broaden test khi có thay đổi boundary mới. Audit không xác nhận release, cloud sync, mobile, model quality hoặc security toàn hệ thống.
