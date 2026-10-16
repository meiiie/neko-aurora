# Backup cục bộ nhất quán và restore dry-run chỉ đọc

Application đã kiểm thử: `70216c063ab0a3825d65eaed9a892654d2287d8b`, tiếp nối delivery `3977042965d37fffef7540ed7d1343ea19f2a427`. Commit bàn giao sau application chỉ sửa tài liệu; manifest ghi hai ID riêng. Không dùng test lịch sử làm bằng chứng cho code mới.

## Phần đã triển khai

CLI tạo snapshot khi service đã dừng, có writer lock độc quyền và không có Core PID đã ghi còn sống hoặc chưa xác định. Journal được replay chỉ đọc: không sửa torn tail, tạo identity, reconcile authority hoặc recovery. File được giữ handle không chia sẻ write/delete; inventory trước/sau phải khớp. Committed artifact/upload thiếu bytes hoặc sai hash làm backup thất bại. Không tự dừng tiến trình để vượt lock.

Snapshot bao gồm journal, raw upload/result bytes, task/Core checkpoints, receipts, authority/tombstone metadata, account vault đã mã hóa và executor identity. Chỉ bỏ root writer.lock. Browser profile, localStorage draft/outbox, source Git và binaries nằm ngoài service state, nên không thuộc snapshot này. Không gọi provider, giải mã credential vault hay upload backup.

Mỗi payload và manifest chi tiết được bảo vệ bằng DPAPI current-user, entropy gắn snapshot và file metadata. Tên/path riêng không xuất hiện trong public manifest; public envelope chỉ có ID snapshot, kiểu bảo vệ, kích thước/hash của manifest mã hóa. Backup/restore roots mới có protected current-user-only Windows DACL; không đổi ACL thư mục đang có. Checksum chứng minh tính toàn vẹn, không cấp quyền thực thi hoặc xác thực nguồn tải xuống.

Restore chỉ vào thư mục mới, tách khỏi nguồn/backup bằng kiểm tra lexical path và danh tính thư mục thực. Từ chối trailing-dot/space, dot segments, tilde/8.3 aliases, UNC/device/ADS, reparse/hardlink và filename không hỗ trợ. So sánh physical ancestors nhằm chặn alias ổ đĩa; không tạo SUBST mapping trong bài thử. Phạm vi nhất quán là service đã dừng và filesystem actor hợp tác, không phải cách ly khỏi người sở hữu cố ý đổi namespace đồng thời.

Sau verify toàn bộ protected bytes, restore đặt marker chỉ đọc trước payload. Journal/file giữ bytes và mtime, nhận readonly attribute; strict replay kiểm sequence/references. Runtime/configured launch bị chặn, API chỉ cho GET; catalog/vault không khởi tạo hay reconcile. UI báo “Bản khôi phục · Chỉ đọc”, Send disabled và không replay outbox. Status của việc cũ là lịch sử, không chứng minh job đang chạy. Không có API bật lại hoặc bỏ marker. Target thất bại giữ nguyên để kiểm tra; không tự xóa/reuse. Không tự khôi phục grants hoặc chuyển old pin sang authority hiện tại.

## Thao tác offline

Đường dẫn tuyệt đối, parent của đích phải tồn tại và đích phải chưa tồn tại. Dừng service/window sở hữu state bằng chính launcher/cửa sổ của nó; không xóa lock hoặc kill PID đoán theo tên. Dùng binary đúng manifest và Windows user đã tạo snapshot:

```powershell
$control = 'E:\Sach\Sua\NekoBot\.build\local-backup-70216c0\neko-control.exe'
& $control --state 'E:\NekoState' --backup-create 'E:\NekoBackups\snapshot-001'
& $control --backup-verify 'E:\NekoBackups\snapshot-001'
& $control --restore-dry-run 'E:\NekoBackups\snapshot-001' --restore-target 'E:\NekoReview\restore-001'
& $control --state 'E:\NekoReview\restore-001' --listen '127.0.0.1:8765'
```

Ví dụ không tạo các parent hoặc gọi credential/provider. Bốn flag offline mutually exclusive với runtime fixture. JSON stdout chỉ trả summary, không nội dung/path/secret. `backup_state_busy` cần dừng writer đúng cách; `backup_process_live_or_unknown` cần xác minh tiến trình, không ép mở. `backup_invalid`/`backup_unsafe_path` cần kiểm source/path/quota, không tái dùng partial target.

Hạn mức: 5.000 file, 64 MiB/file, 512 MiB tổng, manifest 2 MiB. Giới hạn protocol wire 64 KiB không đổi. Backup vượt hạn mức thất bại, không im lặng bỏ bớt file. Verify kiểm schema/checksum/DPAPI/bytes; restore thêm semantic journal/reference checks. Chỉ mở bản đã restore qua localhost với local-cookie/Host/Origin boundary.

DPAPI thường gắn cùng Windows user và máy. Bài này chỉ nghiệm thu same-user/same-machine; không phải portable backup, offsite/3-2-1 hay bảo đảm phục hồi khi mất ổ/profile. Không chuyển encrypted snapshot ra ngoài. Xem [Microsoft DPAPI](https://learn.microsoft.com/en-us/windows/win32/api/dpapi/nf-dpapi-cryptprotectdata). Portable recovery cần thiết kế key/password riêng trước khi dùng secrets hay cloud.

## Evidence đúng application

| Gate | Kết quả | Phạm vi |
| --- | --- | --- |
| Fresh Go test/vet/build | PASS353 top-level +408 subtests | Clean70216c0; optional DOCX reference skip nếu không cấu hình |
| CLI backup/verify/restore/readonly HTTP | PASS38 | Compiled Go thật, Windows DPAPI, copy dataset synthetic đã có genuine Core receipts; không chạy Core mới |
| Lịch sử/file/context/nonce | Exact byte/hash/mtime và21 file299680 bytes | Original dataset và source-copy bất biến; executor identity giữ nguyên |
| Existing conversation browser | PASS53 | Actual Go/Chrome headless, journal riêng; không Tauri/phone/model |
| Readonly browser + VM guards | PASS10 actual browser +7 Node VM | Snapshot/Send/outbox/reload, scoped authored pending intent; không native GUI |
| Independent review | PASS30 tests +36 subtests backend0dd; UI702 VM7 | Immutable backend0dd, hai causal RED→GREEN và probes riêng; source backend byte-equivalent702. Không mượn Root drill/browser |

Retain hai P2 RED tại0680: Win32 trailing-dot target làm restore nằm trong backup; manifest205 file/~74 KiB bị wire limit làm verify/restore thất bại. Probe gốc giữ nguyên. Sửa path before Clean, physical identity fence và strict bounded backup JSON parser (duplicate/casefold duplicate, UTF-8/surrogate, depth, integer, unknown fields). Protocol/Core/host/desktop source không đổi.

Hai vòng drill đầu giữ nguyên:0680 harness đọc writer.lock đang bị service giữ;9a harness từ chối cả Windows console host. Diagnostic xác minh child duy nhất là System32/conhost.exe; final38 chỉ miễn exact system path+hash đó và từ chối mọi child khác. Không dùng lỗi harness để bỏ gate native-dispatch. Toàn bộ owned test processes đã đóng. Vòng readonly mobile0dd có gate an toàn PASS nhưng main notice sai/ẩn; causal visible RED được giữ, final702 PASS cùng assertions. CSS chỉ mở notice readonly tại viewport thấp; normal light direction không đổi.

## Bàn giao và phần còn mở

Packet mới `.checkpoints/local-backup-release` có exact manifest, binary pins, source bundle, drivers, sanitized receipts và retained negatives. Không chứa raw state, decrypted manifest, encrypted backup payloads, credential blobs, cookie/profile hoặc fixture config. Các backup/drill synthetic chỉ ở E scratch local.17 ZIP,3 packet cũ, preview/handoff/review và3 raw local modifications được hash trước/sau; không tạo ZIP cuối hay push.

Launcher dùng `-Manifest .checkpoints\local-backup-release\launch.json` rõ ràng; default receipt cũ giữ nguyên. Host/desktop/Core pins không đổi, không claim full Rust/Core/Tauri rerun. File readonly là defense chống old binary write-open, không phải chống user cố ý sửa file attributes/marker. Đừng mở restored state như active state hoặc dùng binary cũ để bỏ qua schema/authority.

[Ma trận đầy đủ](product-gap-matrix-20261011.md) phân biệt code còn thiếu và nghiệm thu: active/portable restore, browser draft backup, bot lifecycle/defaults, semantic memory, parser sandbox, purge/log retention; cloud auth/pairing/transport, broker/OAuth/challenge/connectors vẫn thiếu code. User controls/native Tauri/physical phone/keyboard/AT, Wiii owner dialog và real-model/cloud vẫn là gate riêng. Không cần quyết định mới để hoàn thành local dry-run đã giao. Local slice kế tiếp hợp lý: bot archive/restore và defaults cho request mới, sau đó secure challenge với simulator; real provider/cloud vẫn hoãn.

Independent review: [final report](../.checkpoints/local-backup-release/independent/review-final-backend-0dd1f27-ui-70216c0.md). Header/source proof/commands và original negatives có trong packet.
