# Chạy và phục hồi local

[run-prebuilt-local.ps1](../scripts/run-prebuilt-local.ps1) mặc định giữ receipt cũ `.checkpoints/local-product-release/launch.json`. Lát cắt bot/DOCX/local replica mới dùng `-Manifest .checkpoints\local-backup-release\launch.json` tường minh để giữ nguyên bản cũ. Script chỉ nhận executable có đúng tên/hash bên trong `.build`, không qua junction/symlink. Receipt là integrity local, không xác thực một bản tải không rõ nguồn. Không chạy executable/receipt từ người lạ.

## Xem bản hiện tại

Ở `E:\Sach\Sua\NekoBot`:

```powershell
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-backup-release\launch.json -CheckOnly
.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-backup-release\launch.json
```

Mở `http://127.0.0.1:8765`. Không cần Go/Cargo để xem. Thêm `-Desktop` cho Tauri; port8765 cần trống. Launcher không gắn vào service chiếm cổng và chỉ dừng process Go chính nó đã tạo.

Default chưa có provider/runtime: tin, nhóm, project và lệnh được ghi journal nhưng bot chưa trả lời bằng mô hình. Không tự dùng account secret hay dựng cloud session. `-CoreFixtureConfig <file>` chỉ dành cho **controlled loopback fixture đã review**, với executable/pin/lease tương thích. Không dùng đường fixture cho API key hoặc endpoint provider thật.

Tên bot: hội thoại riêng → nút bút trong header → sửa tên → Lưu tên. Alias và task cũ giữ nguyên. ACK chưa rõ có “Thử lại cùng yêu cầu”; mở lại form sau409 để dùng revision hiện tại. [Contract](bot-display-names-local.md).

`-StateDirectory E:\CodexScratch\neko-review-state` mở state mới riêng. Không dùng hai writer cho cùng state; store lock bảo vệ journal. State không vào Git. Nếu cổng bị chiếm, đóng service bằng launcher/cửa sổ đã tạo nó, không kill PID đoán theo tên.

## Dừng, backup, rollback

Dừng service/window trước khi sao lưu toàn bộ state. Giữ journal, artifact/upload bytes, account metadata/DPAPI blobs và receipt/outbox cùng bộ dữ liệu. Không xoá lock để ép mở. DPAPI gắn Windows user/machine; copy sang máy khác không đồng nghĩa credential dùng được. Bản sao cùng ổ không đủ backup ngoài môi trường.

Task đã nhận không biến mất khi đóng UI. Service interruption có thể đưa task sang trạng thái cần kiểm tra/dừng/phục hồi. Chỉ explicit V2 action với current CAS và genuine stop/effect/lease proof mới tiếp tục được; intake/rename receipt không cấp quyền chạy. Journal cũ thiếu proof fail closed.

Rollback: dừng owned processes, giữ state backup, dùng bundle/checkpoint và binaries/manifest tương ứng. Không mở journal có event mới bằng binary cũ không hiểu schema. `bot.renamed` tương thích khi bản mới đọc journal cũ; downgrade sau rename cần state backup trước event ấy. Giữ ZIP/bundle/evidence nguyên bản, không reset hay xoá thay đổi chưa phân loại.

## Gate còn mở

Ảnh/video mới là compiled-Go/browser viewport và local queued UI, không chứng minh điện thoại, AT speech, provider/cloud hay owner consent. Tauri/Core stop/effect/group gates của checkpoint65 giữ riêng; rename không tự gắn nhãn rerun lên chúng.

Wiii approve/Cancel bị chặn do Computer Use native lỗi khởi tạo; không giả approval qua CDP/IPC. ETW whole-process trace chưa có quyền token/công cụ PID-filtered hợp lệ; không mở quyền Windows rộng để vượt gate. [Ma trận](product-gap-matrix-20261011.md) ghi cả thiếu code lẫn thiếu nghiệm thu.

## Bot preferences, DOCX và local replica

Menu bên cạnh ô nhập có “Hướng dẫn & lịch sử của bot”. Lưu cấu hình áp dụng cho yêu cầu mới, không sửa pin của việc đang chạy. Tắt lịch sử chỉ bỏ các câu trả lời đã hoàn tất khỏi ngữ cảnh hội thoại mới; không xóa journal, task memory hay quyền đọc file. Upload DOCX vào đúng project/nhóm rồi chọn đọc văn bản; preview không thực thi markup hay mở liên kết. Tên tệp dài chỉ hiện hai dòng trong hộp thấp, tên đầy đủ vẫn là nhãn truy cập. [Contract và evidence](delivery-local-bot-file-sync-20261012.md).

Local replica là endpoint GET-only cho hai client kiểm thử trên cùng máy; chưa có cloud identity, pairing hoặc phone app. Giữ backup state trước event cấu hình bot/upload DOCX mới. Binary cũ chưa hiểu event mới không được mở journal đã tiến lên; rollback cần binary/schema và state backup tương ứng. Bản cũ và media Library5fa được giữ nguyên.

## Hội thoại và retention preview hiện tại

Menu cạnh ô nhập → Hội thoại → tạo/chọn/lưu trữ/khôi phục. Archive chỉ chặn fresh intake; job/context/file cũ giữ nguyên và typed controls vẫn cần authority/CAS. Export JSON/text chỉ gồm lịch sử công khai hội thoại riêng và file metadata, chưa là backup bytes. Retention days là cửa sổ preview, luôn disabled, không xóa dữ liệu. Days=0 giữ tất cả. Retry lifecycle là page-lifetime, reload chỉ đọc metadata đã commit. [Bàn giao đúng applicationfb48e1](delivery-conversations-retention-20261012.md).

Rollback sau conversation.created/conversation.updated/retention.preference_saved cần binary tương thích và state backup trước event. Không dùng binary cũ để mở journal mới. New screenshots gắn fb48e1 là actual compiled Go/browser queued local; native11 có receipt riêng, không phone/AT/provider/cloud acceptance.

## Backup và kiểm tra phục hồi hiện tại

[CLI guide và evidence đúng application70216c0](delivery-local-backup-20261012.md). Dùng `.build/local-backup-70216c0/neko-control.exe`; `--backup-create`/`--backup-verify`/`--restore-dry-run` là offline, mutually exclusive, chỉ nhận absolute paths với đích mới. Dừng writer/Core đúng cách trước backup. Snapshot mã hóa payload và private manifest bằng current-user Windows DPAPI; không plaintext staging, credential vault không bị giải mã.

Restore vào root riêng có user-onlyDACL và marker chỉ đọc; GET history/file được, Send/POST/runtime/recovery bị chặn. Old statuses là lịch sử, không job đang chạy; không có API enable/remove-marker. Không phục hồi vào `.state` đang có, không xóa lock hoặc dùng binary cũ để bỏ fence. Fail giữ partial target để kiểm tra, không reuse/xóa tự động. No upload, portable/offsite/key recovery, browser profile/outbox backup hay active migration acceptance. Original ZIP/packets/previews/localchanges giữ nguyên.
