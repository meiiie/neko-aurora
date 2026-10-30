# Thông báo và đường dẫn thiết lập AI — 2026-10-30

Code commit: `0c1775ea94d637a96d5e23b346364f4a93e18f1e`, base `3611e9c792292b9bee7c95032a5766e5e7eb98ca`. Source manifest SHA256: `75b9ffcb949fc3a3a4872e359e942f85f58a09e147180522fc07795891715a47`. Build/test chạy trên working tree tương đương commit này; không sửa source trong gate. Một agent tự thực hiện, không subagent hoặc independent review.

Khi lượt trả lời thất bại vì chưa cấu hình AI, API cung cấp recovery action có cấu trúc. UI giải thích tin nhắn đã lưu nhưng chưa xử lý và mở phần thiết lập từ nút Thiết lập AI. Chỉ thông báo hệ thống có recovery action mới được tạo nút, không suy luận từ nội dung người dùng hoặc model. Action giữ workspace/bot hiện tại, giữ bản nháp và không gửi lại tin nhắn. Thông báo cũ được chiếu lại từ journal; lời trích dẫn do người dùng gửi vẫn giữ nguyên.

Trang mô hình hiển thị trạng thái cấu hình mặc định từ API, thay cho dấu xác nhận GLM luôn có sẵn. Có cấu hình không đồng nghĩa kết nối đã được kiểm chứng. API cũ thiếu trạng thái được hiển thị là chưa xác định. Có hướng dẫn thêm nhà cung cấp, chọn mô hình cho đúng bot và thử lại khi tải cấu hình thất bại. Không đổi credential storage, quyền thực thi hoặc cách gọi provider.

## Môi trường và kiểm tra

Windows amd64, PowerShell 7.6, Go 1.27.1, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.49. Runner giữ biến OS trong allowlist, loại credentials provider; Go dùng USERPROFILE/XDG/npmrc kiểm thử riêng, `GOTOOLCHAIN=local`, `AURORA_BRIEF_LOOP=off`, `AURORA_COMPUTER=off`, `PSModuleAnalysisCachePath=NUL`. Browser dùng profile tạm và HTTP fixtures, không tài khoản thật. Nhật ký lệnh, môi trường, timestamps, hash và exit code nằm trong `.audit/2026-10-30-ai-setup/validation.jsonl`.

| Kiểm tra | Lệnh từ root, trừ Go chạy tại app/native | Kết quả |
|---|---|---|
| Failure/config API | `go test -v -count=1 -run 'Test(NoKeyIsHonest\|ModelsReportsDefaultConfiguration\|ProviderModelThinkingAndForm)$' ./internal/aurora` | RED trước sửa; GREEN sau sửa. Provider fixture phụ thuộc computer bị SKIP |
| UI thiết lập | `python -u app/tests/aurora/ai-setup-browser.py` | PASS, 6 nhóm: recovery, đúng bot/draft, lỗi/retry, cấu hình true/false/unknown, reload/mobile, English/no mutations |
| UI hồi quy | `python -u app/tests/aurora/workspace-browser.py`; `composer-browser.py`; `startup-browser.py` trong cùng thư mục | PASS cả 3 scripts |
| Toàn package Aurora | `go test -json -count=1 -timeout=300s ./internal/aurora` | PASS: 555 test/subtest, 25 SKIP, không FAIL; 59,65 giây |
| Embedded assets | `go test -count=1 ./internal/web` | PASS |
| Static/build | `go vet ./...`; `go build -o <audit>/neko-aurora.exe ./cmd/neko-aurora` | PASS |
| JavaScript/whitespace | `node --check` cho app.js, i18n.js, sw.js; `git diff --check` | PASS |

Browser harness ban đầu click lại trong cửa sổ chống ghost-click 400 ms sau khi đóng sheet; lần sau fixture còn ghi đè ngôn ngữ về vi ở mỗi reload. Sửa hai lỗi harness bằng chờ đóng sheet và chỉ khởi tạo ngôn ngữ khi chưa có lựa chọn. Giữ assertion sản phẩm và lưu các log FAIL. Sau đó xem trực tiếp ảnh desktop 1440×1000 và mobile viewport 390×844; làm rõ nút chính bằng nền tương phản và chạy lại các gate trên source cuối.

## Kiểm tra bản đang chạy

Giữ state của phiên desktop trước đó, thay riêng control-plane binary bằng build mới và mở lại qua `app/scripts/run-prebuilt-local.ps1 -Desktop` với manifest hash dưới `app/.build/ai-setup-20261030/`. Không thay dữ liệu hội thoại hoặc cấu hình provider. URL giữ ở loopback port 63501, đường dẫn `/app/#bot-meiiie`.

Reload tab trong Codex đã hiển thị recovery card mới cho các lượt lỗi đã lưu; nút Set up AI mở Models & providers và API thực báo AI connection not set up. Đóng cấu hình trở lại hội thoại; tab được giữ mở cho người dùng. Ảnh local `preview-updated.png` trong audit là bản đang chạy, khác với ảnh fixture. Bản xem trước dùng tiếng Anh theo preference của tab, còn nội dung người dùng giữ tiếng Việt.

Quét trước code checkpoint: 9 candidate files, 56 newly reachable blobs, 0 recognizable secret match; không in matched values. Không push/deploy, không nhập key, không tạo grant và không gọi provider thật. 25 SKIP giữ đúng giới hạn môi trường; đây là xác minh UX/API cục bộ, không chứng minh AI đã trả lời hoặc acceptance trên điện thoại vật lý. Rust không đổi nên dùng lại binary desktop/host đã xác minh ở lượt native trước.
