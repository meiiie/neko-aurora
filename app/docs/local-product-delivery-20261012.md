# Local product checkpoint: 2026-10-12

Tested application **5fa4c4337ff390767dd844f24d764ce9d901cea9**, first rename code43, original baseline a0/65. Delivery after5fa changes only documentation and the PS argument-test HANDLE lifetime; app/host/Core/desktop runtime bytes are unchanged. Primary materialization, bundle, binary hashes and media are bound by [manifest](../.checkpoints/local-product-release/manifest.json) and [launch receipt](../.checkpoints/local-product-release/launch.json).

Đã triển khai đổi tên hiển thị bot theo stable ID, giữ alias/history/task/context/permissions; API CAS/nonce receipts bền vững, old-create/rename retries không làm tên quay ngược. Snapshot/SSE/ACK dùng cùng counter guard. Native Desktop quoting giữ đúng trailing backslashes. Prebuilt launcher có hash/path/reparse checks, default chỉ ghi journal, không tự cấu hình provider. README và ma trận phân biệt rõ các phần thiếu code.

| Kiểm chứng mới | Kết quả / phạm vi |
| --- | --- |
| Full Go/vet/build clean5fa | 243 top-level +250 subtests PASS; rebuilt Go SHA `6e0a2ebcefef88db75e225410bed370c52888352f69d9184979fd78529a2dcd1`. |
| Actual Go/browser5fa | 16 PASS; delayed snapshot thật trong khi SSE đóng, newer rename ACK, lost ACK same-nonce retry, late navigation/draft/focus, desktop/mobile viewports. |
| Native rename5fa | 4 PASS; original live Core context/binding/lease/epoch giữ nguyên, foreground mới dùng tên mới và hoàn thành trước job nền, original retry, authentic child-stop on cancel. |
| Snapshot VM5fa | 4 PASS; rename ACK, create ACK, newer snapshot, inconsistent same-counter metadata. Đây là exact-source fixture, không thay actual browser. |
| Native argv | 3 PASS PowerShell7; Root PS5.1 final3 PASS với retained HANDLE để đọc ExitCode đúng. PS5.1 initial harness fail giữ lại; actual argv lúc đó đúng, chưa phải product failure. Reviewer optional PS5.1 vẫn giữ đúng blocker riêng. |
| Launcher | Default service/integrity10 ở43 và current promoted run được ghi riêng; CheckOnly, wrong hash/path/schema, state có spaces, saved waiting_host, owned-child cleanup. Không mở Tauri GUI trong suite này. |
| Independent review | Hai P2 đã RED trên43 và closed trên5fa; no additional important findings trong bounded ledger/API/narrow diff inspection. Full suites/GUI không được gọi là independent rerun. |
| Demo | Hai genuine Core sessions, streamed foreground/background, một artifact312 bytes có download hash khớp. Actual renderer recording qua CDP, provider loopback xác định; không chứng minh reasoning của model thật. |

Các vòng FAIL và correction giữ nguyên: missing initial SSE wait; initial count-parser zero derivative (raw Go log/exit unchanged); UTF-8 metadata writer; first recording frame cap truncated trước file (Library old version retained); review43 snapshot/argv RED; PS5.1 test-process ExitCode cache. Chỉ source fixes có repro thật được đưa vào app; harness corrections được phân biệt.

Host `5bee…1423`, Core `97ca…47fb`, desktop `fe55…1f89` giữ source/pin và historical receipts riêng. Không lặp Rust26, Tauri16+10, group9/service9/effect3 chỉ để tăng số. Audit v6 của baselinea0/65 giữ report/hash nguyên, không dùng làm rename proof.

17 ZIP, 26 media có trước lát cắt, ba EOL-only script working files và state được hash-preserve; source bundle/local checkpoints không chứa credentials hay state người dùng. Media mới được lưu Library theo cùng identity/version; Windows xattrs không áp dụng được, association giữ riêng trong packet. PNG/video revision names ở `preview` không ghi đè approved preview cũ.

Chưa nghiệm thu: current Tauri rename UI, physical phone/keyboard/nonzero safe-area/AT speech, new controls visual review, provider/model/account broker/OAuth/cloud, external connector/multi-owner scope, Wiii owner approve/Cancel và PID-filtered ETW. Active semantic pause, installable mobile/cloud transport, full bot configuration/archive và wider document extraction còn thiếu code, được nêu trong [ma trận](product-gap-matrix-20261011.md). Historical UI timeout attribution vẫn có giới hạn. Formal Go-race chưa có C toolchain.

Run: `scripts/run-prebuilt-local.ps1 -CheckOnly`, rồi chạy không flag để mở service local; thêm `-Desktop` để dùng shell đã có. Default không trả lời bằng mô hình khi chưa có runtime. [Backup/rollback](run-local-current.md) yêu cầu dừng owned processes, giữ state backup; downgrade sau `bot.renamed` cần state trước event mới. Chưa tạo ZIP bàn giao cuối hay deploy/push/grants.
