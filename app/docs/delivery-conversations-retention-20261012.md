# Hội thoại riêng, xuất lịch sử và retention preview local

Application được kiểm thử là `fb48e125eac59b29671448e6206d79f765f38c62`, tiếp nối delivery `caaa9f7de0adb25075460e82441082787041cd27`. Commit bàn giao sau application chỉ sửa tài liệu; `launch.json` và manifest local ghi hai ID riêng. Không lấy các lần kiểm thử cũ làm bằng chứng cho bản mới.

## Hành vi đã triển khai

Menu cạnh ô nhập → **Hội thoại**: tạo phiên riêng, chọn lại phiên, lưu trữ/khôi phục, tải lịch sử JSON hoặc text, lưu cửa sổ xem trước retention. Bot ID/alias giữ nguyên. Draft, scroll và việc đã nhận thuộc phiên ban đầu. Archive chặn tin mới; không dừng job cũ, đổi context pin, chuyển file hay thu hồi quyền. Typed supplement/replace của việc cũ vẫn phải qua kiểm tra lineage/CAS/authority; không có field JSON để vượt archive. Tin @bot khác giữ hội thoại của người gửi và không nhập lịch sử riêng của bot đích.

Export là snapshot hẹp của hội thoại riêng: tin công khai, câu trả lời/trạng thái và metadata/đường tải file có scope. Không xuất secrets, account/runtime/context pin, artifact bytes, shared-channel/group/project bundle hay Core checkpoint. Đường tải vẫn cần phiên/quyền/bytes hợp lệ. Đây chưa phải product backup hoặc import/restore.

Retention luôn `enabled:false`. Days=0 giữ tất cả; days>0 chỉ tính preview theo tuổi tạo UTC, ước lượng text/reply bytes và lý do bảo vệ. Pin lịch sử trong journal vẫn được bảo vệ sau refresh/restart. Active jobs, effects chưa rõ, stop chưa có proof, nhóm, task/context links, result files và project sources được bảo vệ. Không có purge/delete/scheduler/compaction hoặc nút bật xóa; toàn bộ dữ liệu thử là synthetic. Số bytes không phải dung lượng ổ đĩa sẽ thu hồi.

UI giữ URL/body/nonce gốc khi ACK chưa rõ, dùng revision đang hiển thị cho CAS và không kéo navigation/draft/focus về phiên cũ. Command retry của lifecycle/retention hiện chỉ tồn tại trong vòng đời trang; reload lấy metadata đã commit và không tự POST. Không nhận là durable intent retry sau reload.

## Bằng chứng đúng application

| Gate | Kết quả | Phạm vi |
| --- | --- | --- |
| Fresh Go test/vet/build | PASS 323 top-level +376 subtests | Clean fb48e1; optional DOCX reference gate skip nếu không cấu hình |
| Hội thoại UI | PASS 53 =16 source VM +37 actual Go/browser | Chrome headless, journal/API thật, queued local; không Core/model/native window |
| Native bảo toàn phiên/file | PASS 11 | Go + Rust host + Core thật qua controlled loopback; background cũ sống khi foreground mới hoàn thành, bytes/hash/mtime và authentic stop proof |
| Existing UI/context/focus | PASS 42 | Compiled Go và CDP fixtures được ghi nhãn |
| Light/pixel regression | PASS 23 | Static fixture; không physical phone |
| Independent final | PASS 13 ledger +3 authenticated API +34 function/DOM checks | Read-only exact Git objects, private synthetic probes; không mượn actual browser/native receipt |

Pure export/planner modules có TDD/bounds evidence riêng; full Go suite ở fb48e1 chạy lại các test hiện tại. Planner fuzz399462 executions là receipt worker theo source hash, không relabel thành fuzz ở commit cuối. Formal Go -race chưa chạy vì thiếu C compiler.

Hai P2 đã tái hiện RED ở165777b được sửa: bảo vệ pin lịch sử retention, và từ chối ordinary fresh accepted replay sau archive. Probe gốc được giữ, thêm typed linked-control/28 forged-lineage rejections và unknown JSON bypass-field rejection. Không phát hiện lỗi quan trọng mới trong review cuối. Failed development/harness rounds được giữ và phân loại; không che bằng PASS cuối.

## Chạy, rollback và phần tiếp theo

Từ root: `.\scripts\run-prebuilt-local.ps1 -Manifest .checkpoints\local-conversations-release\launch.json -CheckOnly`, rồi cùng lệnh bỏ `-CheckOnly`. Default chỉ lưu journal và chờ runtime; không gọi provider. Dùng state synthetic riêng để review, xem [run guide](run-local-current.md). Host/desktop/Core pins giữ nguyên và được hash-bind riêng; không claim rerun toàn bộ Rust/Core/Tauri.

Các event conversation/retention mới yêu cầu binary hiểu schema. Dừng owned service và giữ state backup trước khi rollback; không mở journal đã tiến lên bằng binary cũ. Các ZIP, hai packet cũ, preview/media cũ và ba file thay đổi local được kiểm hash trước/sau. Packet mới dùng thư mục riêng; không tạo ZIP cuối.

[Đối chiếu đầy đủ định hướng](gap-alignment-conversations-20261012.md) đọc cả12 REQ/18 SCN và DOCX; phần source165 ở đó là baseline của người đọc. [Gap matrix hiện tại](product-gap-matrix-20261011.md) cập nhật theo kết quả kết hợp fb48e1. Còn thiếu product backup/restore, bot archive/default model/account, shared/project/group export, semantic memory/full memory-off, các parser khác, cloud identity/transport/pairing, provider broker/OAuth và task-bound secure challenge. Phone/keyboard/safe-area/AT, user review controls mới và native Tauri UI vẫn chưa nghiệm thu. Wiii owner consent còn bị công cụ chặn; không bypass.

Thứ tự local tiếp theo: consistent backup + isolated restore dry-run; bot archive/restore + defaults; task-bound challenge/provider-boundary simulator. Không tự mở real provider, secrets/grants, cloud/deploy hoặc quyền ngoài scope đã giao.
