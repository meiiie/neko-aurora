# Trạng thái bot và mặc định local

Phạm vi L01 từ backlog `79f275e`, backend contract `5f8e361`. UI giữ bố cục light đã duyệt. Mở **Bot & mặc định yêu cầu mới** từ menu cạnh ô nhập; không thêm một thanh cấu hình thường trực. Bộ lọc **Đã lưu trữ** ở sidebar cho xem lại bot đã lưu trữ. Bot đang xem luôn giữ trong danh sách để thao tác lưu trữ không đổi hội thoại hay làm mất bản nháp.

Lưu trữ bot chỉ ngăn nhận yêu cầu mới, gồm bot nguồn và bot nhận qua `@alias`. Lịch sử, tệp và các điều khiển cho việc đã nhận vẫn hiện trong đúng hội thoại. Khôi phục giữ ID, alias, các phiên, account/context pin và file binding cũ. UI không cấp quyền, đổi alias, chuyển lịch sử hay tự dừng công việc.

Trong thanh gửi tin, **Mặc định của bot nhận** là lựa chọn riêng với **Không chọn tài khoản** và tài khoản cụ thể. Bot nhận được xác định sau prefix `@alias`; outbox giữ `useBotDefaults`, `expectedBotStateRevision`, body và nonce gốc. Tài khoản mặc định stale/removed/rotated khóa intake mặc định; UI không tự tìm tài khoản khác. Override manual hoặc không tài khoản phải do người dùng chọn rõ. ACK chỉ dọn đúng bản chờ, có pin và revision đúng intent; metadata mới không đổi pin của yêu cầu cũ.

Mặc định tài khoản giữ exact account ID/revision và model trong catalog. Model riêng không tài khoản là preference/requested configuration. Provider/model thật và broker chưa được xác minh; giao diện nói rõ giới hạn này. UI không đọc khóa, gửi credential hoặc thay actual fixture model. Phối hợp hai bot tiếp tục dùng một account selection explicit cho cả nhóm; bot-default mode cần đổi sang tài khoản cụ thể hoặc không tài khoản trước khi mở nhóm. Bot đã lưu trữ bị loại khỏi danh sách tham gia cho nhóm mới, nhưng nhóm đã nhận giữ nguyên participants và điều khiển.

Editor giữ revision đã hiển thị. SSE hoặc cập nhật bên ngoài khiến editor stale thì giữ bản nháp và khóa mutation đến khi đọc lại rõ ràng. Archive/default command giữ original body/nonce trong page lifetime; retry không đổi revision. Đọc lại có thể đối soát `bot_control` receipt chỉ đọc, so exact typed input gồm bot ID; không dựng Work từ metadata. `400/404/409` yêu cầu đọc lại trước intent mới. Reload không tự gửi lại metadata command. Durable pending metadata là L05, chưa được suy ra từ L01.

Dialog dùng controls tối thiểu 44px, fields 16px, scrollport giới hạn theo viewport và focus trap. Biên nhận muộn không đổi bot, đóng form mới hoặc ghi đè bản nháp; restore-only mode khóa các mutation. Phần readonly trong runner L01 là authored UI mode fixture, không thay bằng chứng CLI restored viewer của Root.

## Kiểm tra và bằng chứng

`scripts/ui_bot_states_local.mjs` kiểm hàm UI nguyên bản bằng Node VM, ghi source/hash/command/environment/status. `--go` chạy executable local do Root build, journal riêng và Chrome headless; bốn assets served phải khớp byte source. Fault injection giữ HTTP response đã commit rồi đổi thành 503; một ca intake đóng SSE của chính fixture để ACK không tự đến qua kênh khác. Không Core/provider thật, native window, production state hay cloud.

RED missing-feature, wire/probe correction, manual-account guard và receipt validation được giữ trong `E:/CodexScratch/local-daily-use-20261012/ui-worker/`. Working runs mang `workingTree:true` và asset hashes; chỉ combined clean commit + executable VCS/hash mới được coi exact application evidence. Kết quả actual/browser và commit cuối sẽ được Root ghi vào packet sau khi chạy xong.

Gates còn ngoài scope: genuine Core background/file preservation do Root phụ trách; user review controls mới, native Tauri, physical phone/keyboard/nonzero safe-area/AT speech, real provider/model/broker và cloud.
