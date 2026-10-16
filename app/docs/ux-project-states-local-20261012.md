# Lưu trữ và khôi phục dự án local

L02 nối contract `local-project-states-contract-20261012.md` vào panel **Dự án & tệp nguồn** hiện có; không thêm thanh điều khiển thường trực. Selector vẫn liệt kê dự án đã lưu trữ, có nhãn rõ. Lưu trữ không đổi dự án đang chọn, bản nháp, lịch sử, tài liệu, nhóm đã nhận hay quyền gốc của công việc.

Nút **Lưu trữ dự án / Khôi phục dự án** dùng lifecycle revision riêng, không dùng `Project.revision`. Metadata `projectState` chỉ được nhận khi ID dự án đã biết, revision không giảm và cùng revision không mâu thuẫn. Snapshot/SSE metadata kích hoạt resnapshot, không thêm Work rỗng. Lịch sử và tệp được render theo exact channel/project hiện có; việc cũ giữ context/resources/account pin.

Khi lưu trữ, Send, thêm tệp mới và phối hợp mới bị khóa. Xem văn bản, tải tệp và điều khiển nghĩa vụ đã nhận giữ quyền hiện có; không tự regrant hay sửa resource pin. Restore mở lại intake mà không xoay revision quyền, repin file hay tiếp tục job tự động. Không delete/pause toàn dự án, không mở rộng ACL.

Pending lifecycle command giữ exact ID, action, revision, body và nonce trong page lifetime. ACK muộn chỉ cập nhật metadata của dự án gốc, không chuyển view hoặc ghi đè bản nháp. Đối soát đọc `project_control` receipt với original typed input; unknown không tự POST, retry giữ nguyên intent. Stale/rejected cần đọc lại rõ ràng trước intent mới. Reload thấy metadata đã commit nhưng không tự gửi lại command; durable metadata intents là L05 riêng.

Controls 44px, focus fallback tới Close trước mutation làm control mất khả dụng trong mobile panel. Read-only restore khóa lifecycle/upload/intake mới. Browser readonly assertion là UI-mode fixture; CLI/restore proof của Root được ghi riêng.

## Kiểm tra

`scripts/ui_project_states_local.mjs` ghi full code commit, workingTree, source/binary/harness/asset hashes, command, environment và PASS/FAIL. VM dùng hàm UI nguyên bản để kiểm lifecycle/authority separation và receipt identity. `--go` dùng Go executable local, journal và Chrome profile riêng trên E; actual file upload/download/read, archive/restore, ACK-loss, old-pin invariance, late navigation và mobile/short controls.

Outputs ở `E:/CodexScratch/local-daily-use-20261012/ui-worker/`. RED missing behavior và failed setup được giữ. Breakpoint hiện có đóng desktop panel khi vào mobile; harness phải mở lại Files sau resize rồi đo control, không gọi hidden panel là lỗi sản phẩm. Final exact application evidence do Root checkpoint và chạy lại khi source quiescent.

Gates riêng còn cần: genuine Core preservation, native Tauri, physical phone/keyboard/nonzero safe-area/AT speech, user review new controls, real provider/model/broker và cloud. Local preview/read không chứng minh phạm vi đó.
