# Neko Aurora: Core Windows và tài khoản local

Code sản phẩm được kiểm thử trên cây Git sạch: `ef5dd4ad60cc36ddb5c36427c2e03e71221ea549`. Thư mục chính là `E:\Sach\Sua\NekoBot`; bản kiểm thử độc lập là `C:\Users\Admin\Documents\Codex\2026-10-11\task\checkpoint-verify-20261011`. Commit bằng chứng sau đó chỉ bổ sung tài liệu/log và sửa nhãn phạm vi trong script kiểm thử mặc định; không thay đổi runtime hay UI.

Đây là lát cắt local chạy được, chưa hoàn tất sản phẩm hoặc toàn bộ giai đoạn 1. Provider thật, cloud và grants tiếp tục hoãn theo yêu cầu. Không deploy hoặc push công khai.

## Code đã chạy

Go sở hữu journal bền vững, scheduler, nghĩa vụ công việc, event và artifact. Rust kiểm tra hợp đồng ACP, receipt, root, activation, tool ceiling và sở hữu tiến trình Core. Neko Core sở hữu vòng lặp model/tool. Tauri hiển thị giao diện Go qua loopback. Wiii không bị nhập vào sản phẩm này.

Meiiie và Davin có lịch sử riêng. Foreground và background giữ request, attempt và Core session riêng; nhận tin mới không hủy hoặc sửa mục tiêu cũ. ACK của admission/text/artifact đi sau commit. File UTF-8 bất biến có operation receipt, fingerprint và SHA-256; tải lại kiểm tra byte/hash. Retry cùng nonce không tạo nghĩa vụ thứ hai. Effect đã ghi file nhưng chưa ACK được đối soát bằng hash; kết quả không xác định chặn mutation mới.

Khởi động lại Go giữ nghĩa vụ chưa xong ở `recovery_required`. Resume giữ session/task cũ và tăng activation. Rust trả lời MCP disconnect khi đóng Core, chặn tool mới và để Core tự nhả khóa. Cancel tăng epoch sản phẩm và chặn event/tool đến muộn.

Core Windows 1.8.0 được biên dịch riêng từ snapshot `3d5725f222815e15b12a212e372cf17057a459c0`: 772 file khớp manifest, dependency lock cố định, không chạy lifecycle scripts. SHA executable và manifest được pin chung cho Go/Rust. Giữ LICENSE/NOTICE AGPL bên cạnh binary. Không chạy bundle Grok; đây không phải installer đã ký hoặc sandbox của hệ điều hành.

Nhiều account card có ID, nonce và revision; khóa được gửi bằng stdin vào Rust rồi mã hóa DPAPI theo Windows user và account binding. Không có API xuất plaintext. Form mật khẩu tách khỏi chat/outbox, xóa khi submit/đổi provider/Escape/đóng; response cũ bị fence khi ngữ cảnh form đã đổi. Card luôn là `stored_unverified`; chưa chọn tài khoản hoặc cấp credential lease cho Core. Plugin manifest được lưu ở `manifest_only_disabled`, chưa có quyền thực thi. Xóa card chỉ xóa đường sử dụng metadata local; dọn ciphertext mồ côi và thu hồi khóa phía provider chưa triển khai. Xem [hợp đồng tài khoản](connections-local.md).

Browser outbox ghi nonce trước POST và replay cùng nonce sau reload; không lưu trường bí mật. Giao diện có reply, trạng thái job, recover/cancel và file download. Kiểm thử dùng cả cửa sổ Tauri WebView2 thật với profile riêng và viewport 390px. Viewport nhỏ chưa chứng minh hành trình điện thoại thật.

Context hội thoại hiện là phần lịch sử đã hoàn thành cùng bot, giới hạn 8 KB, đọc khi dispatch và mô tả là context không cấp thêm quyền. Chưa có context pin bất biến tại thời điểm nhận việc, lựa chọn tài liệu bằng AI hoặc ACL qua nhiều kênh. Không gọi cơ chế này là durable memory hoàn chỉnh.

## Bằng chứng trên commit sạch

Log gốc nằm ở [evidence](evidence/2026-10-11-native/validation.json). Các suite đều chạy lại sau checkpoint, không dùng test lịch sử làm bằng chứng.

| Kiểm tra | Trạng thái | Phạm vi |
| --- | --- | --- |
| Go `test -count=1 -v ./...`, `vet ./...`, build | PASS | 21 test cấp cao; Windows Go 1.27.2, có shared canonical cases |
| Rust `cargo test --locked`, build và fmt check | PASS | 10 test: ACP/conformance và DPAPI thật; Rust 1.97.1 |
| Tauri `cargo build --locked` và fmt check | PASS | Windows debug shell; chưa phải release đã ký |
| Core Windows process suite | PASS 6 kiểm tra hành vi | Reply trước khi background xong, file, retry, kill Go/resume, cancel và từ chối an toàn sau kill Core |
| Cùng process suite tại đường dẫn E | PASS | Root/receipt, binary và file dùng tại consumer Windows thật |
| Task state nằm vật lý trên ổ E | PASS | Suite riêng đặt TEMP/TMP dưới `.build/fixture-tmp`; kiểm tra receipt/root, file và phục hồi trên volume E |
| DPAPI/API end-to-end | PASS 4 kiểm tra | Nhiều account, nonce/secret conflict, plaintext không trên đĩa/projection, plugin tắt, restart/revision; chạy cả C và E |
| UI Tauri WebView2 thật | PASS 15 kiểm tra | Lịch sử, outbox, secret clearing/race, desktop/390px, reduced motion, Core overlap, link/file checksum, reload |
| Hai client, journal, SSE và hard kill/restart Go mặc định | PASS 6 kiểm tra | Suite này cố ý không cấu hình Core; không đánh giá khả năng agent |
| Forced Core crash resume | BLOCKED | Core v1 để lại writer lock và không có API phục hồi stale lock an toàn |
| Provider thật/model quality/cloud/điện thoại thật | DEFERRED / CHƯA KIỂM THỬ | Không dùng khóa thật hoặc dịch vụ trả phí |

Process suite C đo reply foreground 7,294 giây khi background còn chạy. Suite tại E đo 6,036 giây. Đây là độ trễ fixture local, không phải p95, SLA hoặc hiệu năng provider thật. Gate từ chối an toàn khi Core bị kill đã pass; khả năng resume trường hợp đó vẫn BLOCKED. Test giữ nguyên session mapping và byte/hash của lock, không tự xóa lock hoặc đổi sang session khác.

Suite bổ sung với task state vật lý trên ổ E cũng pass 6 kiểm tra, foreground 5,970 giây. Báo cáo gốc riêng là `e2e-core-e-drive-state-ef5dd4a.json`.

Các binary được kiểm thử và đã materialize vào `.build` ở E:

| Binary | SHA-256 |
| --- | --- |
| Go | `66ad3a386a4be20437f68a09568c51dfbf1e3a3935bc949fefb7c5ab631e3d55` |
| Rust host | `fe177370307bedd1f2149dfe1b611964d22ee49eeed8d31300e7e8033da3b66b` |
| Core Windows | `4b3fb6202551912f86b5dd5faffccf75d854174c2ddd6b9fb331470a2b2e0b36` |
| Tauri | `35c4a007ae8aa736bb5600e283c6742e3804e5c35f054822942a295cff119136` |

Một lần test metadata trong filesystem sandbox bị từ chối thao tác; cùng suite với quyền filesystem ứng dụng đã pass, gồm kiểm tra đầy đủ và end-to-end. Không có auto-review rejection. Compiler cảnh báo cache không hard-link được và dùng copy; build vẫn pass.

## Chạy và phục hồi

Từ E, chạy preview có Core thật và provider fixture chỉ trên loopback:

```powershell
C:\Python313\python.exe -X utf8 scripts/e2e_core.py --native-core --serve
```

Mở URL `READY` được in ra; Enter đóng preview. State của preview là tạm thời và được dọn khi đóng. Các marker `[fixture:background]` / `[fixture:artifact]` kích hoạt tình huống kiểm thử xác định; không quảng bá phản hồi fixture là AI thật. Launch mặc định bằng `scripts/run-local.ps1` giữ lịch sử trong `.state` nhưng chờ runtime khi chưa có cấu hình. `.state`, ciphertext, binary, dependencies và ZIP không nằm trong Git.

Checkpoint code `.checkpoints/neko-native-ef5dd4a.bundle` chứa toàn bộ lịch sử Git, đã `bundle verify`; SHA-256 `de1cd3c05c1b80958f8f2be0a13b323e473dd04fc29c026de52d4d2caae2ce40`. Bundle tài liệu cuối cùng, kiểm tra clone/fsck và checksum được ghi riêng trong `.checkpoints`. Bundle không thay thế bản sao dữ liệu `.state` hoặc 15 ZIP gốc. Không ghi credentials vào snapshot.

Đã giữ nguyên ZIP gốc, Grok, tài liệu hướng dẫn, source App c9dbbd7/Core 3d5725f và repo NekoCore cạnh bên. App d61a45c và các thay đổi native cũ vẫn chưa tìm lại được. Bản [checkpoint intake](CHECKPOINT-2026-10-11.md) là lịch sử, không phải trạng thái triển khai hiện tại.

## Việc tiếp theo

Lát cắt độc lập tiếp theo: lưu context pin có provenance ngay khi nhận việc, rồi kiểm thử queue/restart và giới hạn scope. Tiếp đó là durable permission suspension và account-bound runtime lease trước khi dùng provider thật.

Gate phục hồi Core bị kill cần API writer recovery có receipt/ownership, kiểm tra tiến trình sở hữu và outcome chưa chắc chắn; không thêm heuristics tự xóa lock. Giữ gate BLOCKED cho đến khi có hợp đồng và test riêng.

Còn thiếu routing kênh và giao việc @bot, context/project/task ACL và durable memory đầy đủ, account OAuth/live rotation, plugin execution, executor lease, database/cloud sync, installer và hành trình điện thoại thật. Các lát cắt này cần gate riêng; kết quả hiện tại không được gọi là hoàn tất tất cả yêu cầu.
