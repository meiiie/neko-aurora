# Neko Aurora — independent acceptance audit, vòng 2

Ngày kiểm: 2026-10-11. Kết luận: các lỗi runtime cụ thể đã được retest và sửa trên `60c1811ca37e0b20ccb203d92da7d2315e21a157`; chưa đủ nghiệm thu toàn sản phẩm. UI bị user từ chối vẫn là gate riêng. Account runtime lease, cross-channel provenance/ACL, automatic Core crash resume, phone/cloud và real-provider evaluation vẫn chưa được chứng minh hoặc chưa implemented.

## Snapshot, quyền và bằng chứng

- Snapshot âm: `8dc8eb082242429b05884a8dcbacff02bac0b336`, archive SHA256 `3dab4c79dc2adc2e917ae4c912140b635f2dc82ea787e1520545efea7769863a`.
- Snapshot sửa: `60c1811ca37e0b20ccb203d92da7d2315e21a157`, archive SHA256 `2b68552410dbffc28288c971e837e45221354ed3204f7ca40a79d9d806a4f8b3`.
- Đọc Git objects ở clone `C:/Users/Admin/Documents/Codex/2026-10-11/task/checkpoint-verify-20261011/.git`; không lấy working tree đang sửa. Mỗi archive gồm 100 file tracked của AGENTS/README/native/host/contracts/scripts/docs. Manifest kiểm lại sau validation: không drift. Frontend không thuộc archive runtime này.
- Chỉ thêm probe/harness, binaries và state trong `C:/Users/Admin/Documents/Codex/2026-10-11/task-2/acceptance-audit-v2`; cache riêng vòng 1 được dùng lại. Không sửa app/runtime/frontend/repository/index, không reset/stash/commit/push, không chiếm GUI, không giết process của worker khác.
- Native Core được copy vào scratch sau khi xác minh pin SHA256 `4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36`. Chỉ chạy binaries tự build từ archive và native Core đã pin, provider fixture loopback do harness sở hữu. Không credential thật, grant, paid provider hoặc deploy.
- Đã đọc AGENTS, định hướng/checkpoint và tài liệu context/connections/UX; áp dụng neko-contest-quality, systematic-debugging, verification-before-completion. Static Grok đã được đối chiếu read-only ở vòng 1; không thực thi/copy bundle hoặc assets. Vòng 2 không tái nghiệm thu UI.
- Báo cáo lịch sử vòng 1: `E:/Sach/Sua/NekoBot/docs/reviews/acceptance-independent-20261011.md`, raw SHA256 `a22224dfdfe14d72387722b54c723dc22474e81d0b3f84ccd052504c3d86b279`. Findings lịch sử không tự biến thành kết quả của commit mới.

Evidence chính: [validation-independent-round2.json](validation-independent-round2.json), hai source manifests, raw Go JSONL, Rust logs và hai `e2e-permissions-independent-*.json`. Các file probe nằm trong `snapshot-*/native/internal/ledger/independent*.go`.

## Kết quả kiểm tra thực chạy

Môi trường: Windows 11 build 26300; PowerShell; Go 1.27.2, Rust/Cargo 1.97.1, Python 3.13.7. Go dùng GOCACHE/GOMODCACHE ở audit scratch; GOENV/GOWORK/GOPROXY/GOSUMDB=off. Cargo offline/locked với target riêng. TEMP/TMP đều thuộc scratch. Test API/native được cấp quyền riêng cho own loopback fixture; không external provider.

| Command / phạm vi | 8dc8eb0 | 60c1811 |
| --- | --- | --- |
| `go test -json -count=1 ./...`, native archive + own probes | FAIL: 40 pass, đúng 2 own P2 fail | PASS: 46 top-level, không skip/fail; gồm 39 repository + 7 own probes |
| `go vet ./...` | PASS | PASS |
| `go build -o <own scratch>/neko-control-<label>.exe ./cmd/neko-control` | PASS | PASS |
| `cargo test --offline --locked` | PASS: 3 unit + 7 conformance | PASS: 3 unit + 7 conformance |
| `cargo build --offline --locked` | PASS | PASS |
| Independent adapted `e2e_permissions.py --commit <hash>` | PASS 6 actual native checks | PASS 6 actual native checks |
| Go race detector | NOT RUN: GCC không có trong môi trường Windows | NOT RUN; 20 concurrent schedules không thay thế race detector |
| GUI / real phone / cloud / model/provider eval | NOT RUN | NOT RUN |

Native harness giữ nguyên các assertions của stock script; chỉ đổi ROOT sang archive riêng, kiểm revision bằng source manifest thay cho working-tree assertion và ghi rõ source mode trong report (`workingTree: null`). Các thay đổi harness và hashes được ghi trong `native-harness-manifest*.json`; không giả vờ archive là clean working tree.

Native checks xác minh bằng handle của đúng executable Core trong scratch rằng process đang chạy trước revoke và đã thoát trước receipt/refresh; sau đó kiểm CAS/nonce, fresh context/new task và archive, deny file effect, regrant không tự resume, và restart giữ suspension/provenance. Bộ này không tái hiện native resume qua writer-lock crash path.

Hashes binaries actual native trên 60: Go `82d13888bf68be4edb515bfd634f51e89a4ca0e553ebce9a058bc7432b5d2a5f`; Rust host `8006addc9a32e2e38ce8a518ba2109b52faafc11fa5fdfcd7e7b8421b38f64ed`; Core như pin phía trên. Số test chỉ mô tả evidence; không dùng để gắn nhãn complete.

## Findings và retest

### F1 lịch sử — P1 recovery toàn dịch vụ sau lỗi artifact: RESOLVED trong scope đã test

Probe gốc được copy nguyên trạng từ vòng 1: ghi intent artifact, chặn deterministic artifact path để physical write fail, cancel, close/open/recovery. PASS cả 8dc và 60; task cancelled/epoch giữ nguyên, unresolved effect được reconcile task-local, intake khác vẫn hoạt động. Repository tests bổ sung missing/partial/exact physical bytes và repeat restart cũng PASS. Source60: `native/internal/ledger/runtime.go:183` recovery và lifecycle-independent receipt replay/reconciliation; không xóa lock hoặc retry effect mù.

### F2 lịch sử — P2 completion với unresolved effect: RESOLVED trong scope đã test

Probe gốc unchanged và repository completion tests PASS cả hai revision. Source60 `native/internal/ledger/runtime.go:167` reject operation chưa completed; completionVersion 1 enforce replay, historical unversioned records vẫn đọc được. Đây là internal store invariant probe, không được diễn giải thành remote exploit hay model evaluation.

### R2-01 — P2 resume revoke-before-admission receipt so PID cũ: FAIL8dc → PASS60

Repro8dc: Core admitted PID401 → stop receipt → recovery_required → requeue → Claim attempt mới → revoke source trước new Admit → current attempt receipt PID402 bị trả `nonce_conflict`, task giữ `suspending`. Source8dc `permissions.go:187`/`:191` so `w.Core` cũ; `runtime.go:116` Claim chỉ clear stop proof; `runtime/manager.go:244` xử lý receipt bỏ error. Đây là availability/provenance regression fail-closed; probe không chứng minh quyền hay effect trái phép.

Patch suggestion đã gửi ngay: lưu durable current-attempt binding riêng với saved Core provenance, bind validated new admission trước permission gate, chỉ receipt của binding/attempt đó được dùng, không coi PID mới chưa xác minh là saved Core đã stop.

Retest60 theo contract mới: unbound402 reject; binding402 cùng session/task root/id/hash/authority, activation2 được stage sau revoke; revoked Admit vẫn reject; old401 receipt reject; current402 receipt accept; restart3 giữ Core401 và current binding402, trạng thái suspended. Source60 `permissions.go:189`, `:200`, `:230`; `runtime.go:125`, `:148`, `:384`; `runtime/manager.go:259`. Probe được điều chỉnh riêng trên snapshot60 để thêm bước mới; negative probe8dc không bị sửa.

Giới hạn: trường hợp resumed admission race được kiểm bằng typed store/receipt fixtures. Native6 xác minh process wait của fresh-context path; automatic native crash resume vẫn BLOCKED bởi Core pin v1.

### R2-02 — P2 refresh commit encoded prompt vượt IPC bound: FAIL8dc → PASS60

Repro8dc: intake 27000 backslashes còn fit; revoke source nhỏ của queued task; hoàn thành source mới có reply 3000 backslashes; RefreshContext commit launch frame JSON 66992 bytes >64000, tăng sequence13→14. Source8dc `permissions.go:210` thiếu check đang có tại intake `store.go:369`. Đề nghị dùng shared encoded bound và reject trước commit, giữ epoch/status/pin/archive/sequence.

Repro unchanged trên60 PASS: ErrInvalid và durable state/sequence không đổi. Source60 `permissions.go:251`, `:272`, intake `store.go:337`, claim gate `runtime.go:125`.

Integrator còn báo compatibility regression ở f5 khi historical unversioned oversized refresh bị coi là corrupt. Audit không thực chạy f5. Audit độc lập tạo journal bằng actual8dc RefreshContext (66992 bytes), rồi dùng60 Open/RecoverInterrupted/restart3: PASS; chỉ task đó suspended với `context_prompt_oversized`, old pin/archive/identity giữ nguyên, oversized refresh/dispatch vẫn deny, unrelated intake vẫn sống. Source60 `store.go:86`, `:211`; `runtime.go:183`, `:395`; `permissions.go:268`. Historical zero version được giữ readable, new version1 enforce bound. Evidence: `legacy-export-8dc8eb0.txt` và full Go60 JSONL.

### Race/cancel bổ sung: PASS trong giới hạn fixture

20 schedules đồng thời Claim/revoke: cả hai thứ tự đều giữ immutable intake pin; sau revoke current epoch không nhận output/tool; restart không revive. Cancel khi suspending rồi nhận late current stop receipt: restart3 vẫn cancelled/desired cancelled, không refresh. Không thấy thêm regression trong những paths này. Cần formal race detector và controlled native barriers ở release candidate; finite schedules không phải chứng minh mọi interleaving.

## Requirement coverage: implemented / tested / proposed

| Requirement | Implemented trong source60 | Evidence và phần chưa nghiệm thu |
| --- | --- | --- |
| Durable task/context pin tại intake | Request/message/nonce/owner-space-bot-conversation, immutable hash và v2 access revision; atomic durable journal trước ack | Fresh Go pin/forgery/torn-tail/idempotency tests PASS; real8dc→60 replay PASS. Không phải remote sync hay recovered lost history |
| Foreground responsiveness/đa luồng | Manager slots bot+lane, tổng4 và background tối đa3; Go mutex giữ invariant, Core sở hữu loop | Scheduling code có, historical process evidence đã đọc; round2 không load-test latency/starvation hay GUI render. Cần concurrent foreground/background/cancel trên release binary |
| Bot identity và tag provenance | Persistent local bot registry; direct local alias routing; local owner/conversation isolation | Go registry/routing tests PASS. Không có verified cross-channel actor/audience/publication ACL/reply-in-place. F3 vòng1 còn là scope gate; không claim tag local đáp ứng REQ-PB004/005/006 |
| Context reuse revoke | Local source access CAS/nonce; ancestry gate; suspended obligation, stopped native receipt, explicit refresh/archive | Fresh Go và native6 PASS cả hai commits; two P2 corrected on60. Đây là local reuse permission, không connector/provider grant hay deletion |
| Account DPAPI/secrets | Multi-account metadata stored_unverified; bounded stdin Rust vault, Windows user DPAPI và account-bound entropy; public projections không chứa plaintext | Fresh connection4 và real Windows DPAPI unit PASS. Wrong-account test là wrong entropy/binding, không test khác Windows user. Best-effort memory wipe; không đảm bảo mọi browser/allocator copy. Credential thật chưa dùng |
| Account runtime lease/revoke/rotation | CHƯA IMPLEMENTED: runtime config vẫn Host/Core/Supervisor/FixtureEndpoint; account store không cấp lease cho Core | Metadata deletion không dừng/revoke real provider session. Cần immutable account revision binding, short lease, broker không lộ key, runtime stop/revoke/rotate isolation tests bằng fake provider trước khi live auth |
| IPC capabilities | Host admission/capability/owner/attempt/epoch/root checks; strict bounded canonical frames; UI loopback session/origin boundary | Go protocol/API và Rust conformance PASS. Tauri native capabilities cũ không được UI audit vòng2 chứng minh. Local single owner không phải multi-owner auth/cloud ACL |
| Files/replay/idempotency | Deterministic operation key/digest, receipt, immutable artifacts, fencing; reconcile unknown không repeat | Fresh Go recovery/cancel/receipt/path/conflict tests PASS; native6 deny effect after revoke. Cần release fault injection process death ở từng effect/ack boundary, không chỉ typed fixture |
| Core crash locks | Pin hiện tại safe refusal; giữ saved binding và unresolved outcomes | Forced-kill safe refusal evidence của integrator đã đọc; round2 không chạy kill suite. Automatic resume vẫn BLOCKED v1; Core writer-v2 patch chưa được audit tại đây, không xóa stale lock mù |
| UX/Tauri/mobile/cloud | Desktop/local baseline có; frontend đang được worker redesign độc lập | UI pass19/native21 cũ là historical narrow checks và user đã chê UI. Cần review visual/interaction mới. Không real phone, cloud delivery/sync, OAuth hay provider/model acceptance |

## Checklist theo thứ tự phụ thuộc

1. Giữ negative evidence8dc; tích hợp source60 vào runtime candidate, rerun exact combined revision với frontend mới. Các corrections runtime trong báo cáo đã PASS; chưa kiểm merge candidate khác.
2. Hoàn thành contract/audit Core writer-v2 và fault injection crash/restart, writer fencing/PID reuse/contention/durable journal; safe refusal và safe resume phải được nghiệm thu riêng. Không bỏ pin/hash gate.
3. Thêm account runtime lease thực qua trusted host/broker: bind account+revision+actor+task, expiration, cancel/revoke/rotate; kiểm broker/history/log/command line/network isolation với synthetic secrets và fake providers. Metadata stored_unverified chưa đáp ứng bước này.
4. Chốt channel actor/audience/provenance và read/publication ACL trước hỗ trợ tags xuyên channel hoặc nhiều owner. Context pin/revoke local không tự cấp publication authority.
5. Kiểm release concurrency bằng race detector khi có toolchain, deterministic barriers revoke/admit/stop, interrupted binding commit, duplicate/old receipt, latency foreground khi background đầy và long history. Không suy ra complete từ số tests.
6. Nghiệm thu UI redesign với user: bot switching khi ack muộn, outbox permanent failure/edit/discard, permission conflicts/refresh, account modal/revision races, keyboard/focus/scroll, file review/download và retained task visibility. F4/F5 vòng1 chưa được round2 retest frontend; UI pass cũ không đủ.
7. Sau các dependencies trên: real device phone, signed Tauri release/accessibility/layout; rồi cloud identity/secrets/replay/sync và provider/model checks với separate authorization. Không deploy hay provider thật trong audit này.

Không còn P1/P2 mới tái hiện được trên paths đã kiểm của60; những gate và giới hạn phía trên vẫn mở. Báo cáo chỉ nhận xét hai immutable snapshots và phạm vi thực chạy, không kết luận toàn repo hay sản phẩm đã complete.
