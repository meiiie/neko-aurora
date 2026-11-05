# Core chat và hiển thị model — 2026-10-30

Code commit: `b12fcd2e08dc5343bf030b937ad0c880f76abb18`. Các gate chạy trên working tree từ base `159d7a8f18bbeebf90fce08fba2ab51ce1bc45a4`; source manifest SHA-256 `d6ed7c964746f62d869300b9adfedf8daf69c90c2f1565ef08876e1f51e745e3` trùng nhau ở cả 11 gate, không có file nguồn thay đổi trong lúc chạy. Commit chỉ ghi lại đúng candidate đã kiểm tra. Làm solo, không dùng subagent.

## Nguyên nhân và sửa lỗi

Aurora đặt `NEKO_CREDENTIAL_HOME` khi gọi Core CLI bằng lệnh `run`. Core chỉ cho phép biến này ở trusted profile bridge (`profiles --json` hoặc `acp --host-profile nekobot`), nên dừng với lỗi `Credential home requires the trusted profile bridge entrypoint` trước khi gọi provider. Aurora gom lỗi đó thành `core_failed`, khiến người dùng không biết lỗi thuộc ứng dụng hay tài khoản.

Đã tái hiện bằng CLI nguồn thật, home kiểm thử trống, fetch bị chặn; mã thoát 1 và thông báo guard chính xác được lưu trong audit. [Core launcher](../../app/native/internal/aurora/core.go) nay bỏ biến bridge khỏi môi trường lệnh chat thông thường, giữ HOME/USERPROFILE/XDG riêng và allowlist môi trường. Neko Core vẫn giữ nguyên guard; vị trí lưu đăng nhập trong home Aurora không đổi, không cần di chuyển credential hay đăng nhập lại vì lỗi này.

Các lỗi khởi động, quyền thư mục, cấu hình và đăng nhập được ánh xạ sang mã hữu hạn; giao diện dùng câu dễ hiểu thay vì `core_failed`. Không đưa raw stderr/token vào hội thoại. Mã lỗi journal cũ vẫn giữ nguyên, chỉ đổi cách trình bày.

## Model nào được chọn

- Provider và model hiện ngay dưới tên bot, là lựa chọn cho **tin nhắn mới**, không phải chứng nhận model đã trả lời tin nhắn cũ.
- Bấm tên bot → **AI của bot** để xem hoặc đổi; cho biết bot chọn riêng hay kế thừa **AI mặc định Aurora**.
- Cài đặt → **AI mặc định Aurora** chỉ áp dụng cho bot chưa có lựa chọn riêng.
- Quick setup hiển thị lựa chọn hiện hành, đồng bộ sau lưu/reload. Thiếu đăng nhập vẫn giữ tên model đã chọn kèm trạng thái chưa kết nối; không âm thầm thay provider.
- State/agent events trả metadata theo từng bot; provider HTTP tùy chỉnh có nhãn model của chính cấu hình đó.

## Kiểm chứng

Windows amd64; PowerShell 7.6, Go 1.27.1, Bun 1.4.0, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.49. Go CWD là `app/native`; Python/Node CWD là root repo. Các lệnh dùng môi trường allowlist, home/XDG/npmrc riêng, không kế thừa provider credentials; PSModuleAnalysisCachePath=NUL. Browser dùng profile Playwright tạm, fixtures HTTP local và chặn remote HTTPS. Không chạy đồng thời các gate nặng.

| Kiểm tra | Lệnh chính xác | Kết quả | Thời gian |
|---|---|---|---|
| quick-api | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -run Test(Quick\|Core\|HybridFalls) ./internal/aurora` | PASS | 17.98 s |
| quick-setup-browser | `python -u app\tests\aurora\quick-setup-browser.py` | PASS | 22.21 s |
| ai-setup-browser | `python -u app\tests\aurora\ai-setup-browser.py` | PASS | 18.07 s |
| workspace-browser | `python -u app\tests\aurora\workspace-browser.py` | PASS | 23.43 s |
| composer-browser | `python -u app\tests\aurora\composer-browser.py` | PASS | 39.31 s |
| startup-browser | `python -u app\tests\aurora\startup-browser.py` | PASS | 32.17 s |
| syntax | `python .audit/2026-10-28-grok/syntax.py` | PASS | 8.95 s |
| go-serial | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS | 487.1 s |
| web-assets | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS | 4.45 s |
| vet | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS | 3.02 s |
| build | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-30-core-runtime\neko-aurora.exe ./cmd/neko-aurora` | PASS | 2.87 s |

Full Go: **2125 PASS, 38 SKIP, 0 FAIL**, tính cả ca con; SKIP không tính là PASS. Quick setup browser có sáu nhóm kiểm tra đạt, gồm model đúng sau lưu/reload, hồ sơ bot, mobile/EN, giữ draft, không tự gửi, không fallback và hủy/thử lại đăng nhập.

[Regression CLI thật](../../app/native/internal/aurora/core_cli_test.go) đi từ Go Core.Stream qua Bun, `core/bin/neko.ts`, agent loop và adapter ChatGPT. Test dùng credential tổng hợp trong home tạm và chặn mọi fetch bằng fixture; kiểm tra model `gpt-5.5`, không tool, đúng credential fixture, rồi trả SSE hữu hạn. Đây là kiểm tra ranh giới thực thi thật với mạng giả lập, không phải acceptance tài khoản ChatGPT thật. Fixture cũ chỉ kiểm tra executable giả và từng xác nhận nhầm biến bridge; đã sửa kỳ vọng và bổ sung ca CLI thật này.

Một lần browser FAIL vì test để hồ sơ bot mở trước khi chuyển viewport; đã sửa thao tác đóng hồ sơ trong test, không nới assertion. Các lần FAIL và các source manifest trước bản cuối vẫn được giữ ở `.audit/2026-10-30-core-runtime/validation.jsonl`.

## Bản local đang chạy

Đã build Go mới vào `app/.build/core-runtime-20261030`, xác minh hash của host/Tauri giữ nguyên và launch manifest trỏ tới code commit trên. Launcher kiểm tra đúng PID/executable đang sở hữu cổng trước khi thay service. Native desktop instance binding PASS tại `http://127.0.0.1:63501/app/`; state cũ được giữ nguyên.

Tab bên phải đã reload và kiểm tra trực tiếp: dưới tên Meiiie và trong hồ sơ có **ChatGPT Plus/Pro · gpt-5.5**; lỗi cũ hiển thị câu dễ hiểu. Các trường cấu hình không bí mật cho thấy Meiiie chọn riêng `core-chatgpt:gpt-5.5`, mặc định Aurora chưa đặt. Không đổi tài khoản/provider, không tự gửi lại tin nhắn, không đọc hoặc in credential thật. Tab được để ở hội thoại sẵn sàng cho người dùng thử.

Bằng chứng local (không commit): `preview-model.png`, `preview-profile-model.png`, `launch-result.json`, `launcher.log`, candidate scan, staged check, command logs và manifests trong `.audit/2026-10-30-core-runtime`. Dòng launcher cũ “Runtime not configured” nói về legacy profile registry, không phải quick setup; trạng thái quick setup được xác nhận từ UI/API và lựa chọn bot.

Chưa gọi model thật bằng tài khoản người dùng trong lượt này. Khả năng model/quota của tài khoản thực, OAuth remote và chất lượng câu trả lời chưa được xác nhận bằng fixture. Không phải model evaluation, cloud sync, kiểm thử điện thoại vật lý hoặc production release; không push/deploy.
