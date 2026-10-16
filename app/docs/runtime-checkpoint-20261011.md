# Checkpoint runtime local ngày 2026-10-11

Code đã kiểm chứng: `33efa44902737f4db885a522dce099c664a62700`. Đợt này sửa khả năng phục hồi receipt, thu hồi quyền dùng một yêu cầu làm ngữ cảnh và khởi động lại rõ ràng bằng ngữ cảnh mới. Dùng Core đã pin qua adapter Tauri/Rust/Go; chưa thay Core hoặc triển khai cloud. ZIP và phần sửa của các worker được giữ nguyên.

## Thay đổi đã triển khai

- Reconcile file effect sau cancel/restart theo receipt riêng. Bytes đúng được xác nhận hoàn tất; thiếu hoặc partial được đánh dấu unknown và giữ fence. Task bị cancel không được hồi sinh; lỗi IO của một task không làm hỏng toàn journal. Hoàn tất task mới bắt buộc mọi operation đã có receipt completed.
- Quyền dùng source làm ngữ cảnh có revision, CAS và nonce durable. Thu hồi source chặn claim/admission/output/effect, kể cả nguồn gián tiếp. Regrant không tự chạy lại pin cũ. Rust xác nhận Core đã dừng sau OS wait; UI không được suy diễn dừng từ PID không tương ứng.
- Binding của lần chạy hiện tại tách khỏi Core provenance lưu từ lần trước. Receipt của child resume mới được đối chiếu đúng attempt; receipt cũ, session/PID sai và attempt chưa được xác minh bị từ chối.
- Refresh rõ ràng chỉ được commit sau khi dừng đã được chứng minh, với expected epoch, không còn effect chưa xử lý và còn dung lượng archive. Giữ nguyên request/message, lưu pin/reply/Core cũ vào archive và gửi pin mới qua adapter. Kiểm tra bound 64.000 bytes sau JSON encoding trước commit.
- Journal refresh lịch sử được đọc theo version cũ. Prompt lịch sử quá lớn chỉ khiến task đó suspended với `context_prompt_oversized`; giữ pin/archive/receipt và để các bot khác tiếp tục dùng được.

## Bằng chứng đúng commit

Report và manifest nằm tại [validation-runtime-33efa44.json](evidence/2026-10-11-permissions/validation-runtime-33efa44.json). Mỗi suite native chạy với binary build từ Git sạch `33efa44`, state riêng và provider fixture loopback, không dùng account hay grant thật.

| Kiểm tra | Kết quả |
|---|---|
| Go suite mới, 39 top-level + 16 subtest | PASS |
| Go vet và build metadata commit/modified=false | PASS |
| Hai repro gốc F1/F2 không sửa assertion | PASS |
| Native permission, cùng kernel handle Core được quan sát đã exit | PASS 6 |
| Native context bytes/provenance/pin qua Core | PASS 3 |
| Native foreground/background, file, nonce, Go crash/restart, cancel | PASS 6 |
| Audit độc lập final 33efa44, legacy journal thật từ 8dc | PASS trong scope ledger/HTTP recorder |
| Rust offline | PASS 10 tại f5afe64; toàn host tree byte-identical đến 33efa44, không claim rerun Rust ở commit mới |

Go SHA256 `3e5833decdf1b0d15b783657c76f82df7e5e3062e821d8c9fecb55cc9c0027d8`; Rust host `cbb4ab187f1c210f6a71a6d0e061367a65c4c1fface61ca9f4712a593c332ff9`; Core pin giữ `4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36`. Audit vòng 2 trên 60c1811 là evidence riêng; không dùng nó thay audit final 33efa44.

## UI được lưu riêng

Nhánh private `ui-integration-f6` có checkpoint evidence `44d2c01699034bb3aa79738a2698a6568a0a5183`. Code native UI test tại `eafbe3dc8a4b66e41cdd6f0b6f0242bde104d330` PASS 5 context và 38 tương tác/Core trong Tauri WebView2 thật. Headless integration riêng PASS 61; audit mobile độc lập f6 PASS 28 regression + 28 context, dùng fixture cho phần client phòng vệ.

Giao diện graphite/mint đã bị người dùng từ chối và không được nhập vào main. Các sửa outbox, late ACK, context control và focus ở nhánh này chưa được chuyển sang assets main. Nhánh `ui-light-reference-f6` đang xây dựng lại theo ảnh/video sáng được cung cấp, giữ các safety guard. Pixel video có group thread, upload/artifact và stop chung không phải bằng chứng rằng API group/shared project/upload/cancel-all đã tồn tại.

Bundle private UI `ui-safety-44d2c01.bundle` SHA256 `fa615c793a8b648ebea240075bf101ad8f090a0d73c4bc5aaa8b6dbc61feb12b` đã được Git verify. Code và báo cáo native UI nằm trong bundle; PNG và geometry vẫn được giữ ở clone kiểm thử.

## Fail / blocked / bước tiếp

- **BLOCKED:** tự resume sau cưỡng bức tắt Core pin v1 có stale writer lock. Kiểm tra hiện tại chứng minh fail closed, không chứng minh tự phục hồi. Không xóa lock hoặc đổi session âm thầm.
- **FAIL / không promote:** Core writer-v2 candidate `7b396c8` còn 2 lỗi EPERM trong Bun suite (2096 PASS, 26 SKIP). Bản baseline cũng tái hiện lỗi atomic rename; chưa giải quyết nguyên nhân Windows. [Bàn giao riêng](reviews/core-writer-v2-blocked-20261011.md) giữ patch/bundle/binary chưa promote.
- **Chưa triển khai/accept:** account runtime lease thực, ACL và provenance khi công bố qua kênh, group shared project, sync cloud/multihost, provider thật, phone thật, keyboard/AT speech và visual mới. Go race detector còn thiếu GCC; các schedule probe không thay race detector.
- Bước tiếp theo là giữ runtime đã kiểm chứng, nhập các safety guard vào bản UI sáng sau kiểm tra hợp nhất, rồi hoàn thiện account runtime lease và contract writer recovery trước mở provider hoặc cloud. Không dùng các PASS local làm bằng chứng cho các scope còn thiếu.
