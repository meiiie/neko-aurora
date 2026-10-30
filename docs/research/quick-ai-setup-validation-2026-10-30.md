# Thiết lập nhanh AI — kiểm chứng ngày 2026-10-30

Source commit: `231f487b2bbf325ed4a07deec776a384f3a585ee`. Các lệnh chạy trên working tree trước checkpoint; toàn bộ 1885 hash nguồn của manifest `68c2b037b7f3e84c61156758eda463f76c0a7c499d7fb1573edf108bf6d11833` khớp nội dung đã commit. Nhánh `codex/openmaus-interface`; chưa push hoặc phát hành thay đổi này.

## Kết quả cho người dùng

**Thiết lập AI** mở phần chọn provider trước. GLM có hai lựa chọn riêng: Coding Plan và API theo mức sử dụng. Người dùng dán khóa rồi chọn **Lưu và dùng**. ChatGPT có **Đăng nhập ChatGPT**, hủy/thử lại và đăng nhập lại; hoàn tất OAuth xong mới chọn **Dùng kết nối này**. Không cần nhập ID provider hoặc endpoint. Model lấy từ cấu hình Core; có thể chọn trong phần tùy chọn. OpenAI API, Claude API, Gemini API và DeepSeek API cũng có preset.

Mở từ bot thì chỉ đổi lựa chọn model cho bot đó. Mở từ cài đặt chung thì đặt mặc định cho các bot chưa có model riêng. Khóa của mỗi provider là kết nối dùng chung trong workspace. Form nâng cao và cấu hình model của bot vẫn nằm trong **Cấu hình nâng cao**. Lưu cấu hình hoặc đăng nhập không gửi lại tin nhắn, không tạo work và không xóa bản nháp.

## Nối với Neko Core

- [Preset và auth](../../core/src/adapters/aurora-setup.ts) đọc `DEFAULTS.profiles`; không sao chép endpoint/model vào frontend. GLM Coding Plan giữ giao thức Anthropic; GLM API giữ giao thức OpenAI compatible.
- [Bridge thiết lập](../../core/bin/aurora-setup.ts) dùng OAuth PKCE và browser opener đã có của Core. stdout chỉ chứa catalog hoặc trạng thái/link đăng nhập, không chứa access/refresh token. Lỗi công khai không trả raw provider response.
- [Runtime Aurora](../../app/native/internal/aurora/quick_setup.go) dùng ranh giới Core CLI hiện có với `--no-tools`. Profile Core đã chọn không tự fallback sang API khác khi thiếu cấu hình. Model/runtime của work và trạng thái mặc định phản ánh profile được chọn.
- API keys được ghi vào vault của Aurora. OAuth được Core lưu trong `aurora/core-home` của state Aurora, không đọc hoặc ghi home Neko Core riêng của người dùng. Thư mục có quyền owner-only; Windows kiểm tra cả ACL kế thừa cho file con, Linux/macOS dùng 0700 cho thư mục (chưa chạy acceptance trên các OS đó).
- Windows tìm `bun.exe` thật cả khi cài qua npm. Quick processes dùng `--no-env-file`, `--no-install` và bunfig tin cậy của Core, environment được chọn rõ ràng và home riêng. Cửa sổ console con được ẩn.
- Giữ nguyên chặn cửa sổ ngoài của Tauri. Khi người dùng bấm đăng nhập, Core mở trình duyệt hệ điều hành. Browser link là phương án mở lại trong bản web; không nới navigation của Tauri.
- Quick setup chỉ bật ở chế độ local RunTurns khi có Core/Bun/dependencies; hosted và attested executor giữ luồng cấu hình riêng. Thiếu Core vẫn có form provider nâng cao. Profile CLI chỉ hỗ trợ chat chữ trong luồng này; không được đưa vào bộ chọn model đọc ảnh.

## Kiểm thử

Windows amd64; PowerShell 7.6, Go 1.27.1, Bun 1.4.0, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.49. Kiểm tra nặng chạy tuần tự, không subagent. Environment allowlist loại credentials provider; USERPROFILE/XDG/npmrc kiểm thử riêng. Browser dùng profile tạm, HTTP fixtures, service worker bị chặn. Brief/computer tự chạy bị tắt trong harness. CWD cho Go là `app/native`, Bun là `core`, Python/Node còn lại là root repo; đường dẫn toolchain và environment từng lệnh được ghi trong `validation.jsonl`.

| Kiểm tra | Lệnh | Kết quả | Thời gian |
|---|---|---|---|
| privatefs | `go test -v -count=1 ./internal/privatefs ./internal/vault ./internal/auroraback` | PASS | 10.22 s |
| quick-api | `go test -v -count=1 -run Test(Quick|CoreQuick|CoreStream|HybridFalls) ./internal/aurora` | PASS | 14.26 s |
| quick-core | `bun --no-env-file test test/aurora-setup.test.ts test/chatgpt-auth.test.ts test/provider-choice.test.ts test/config.test.ts` | PASS | 1.93 s |
| typecheck | `bun --no-env-file run typecheck` | PASS | 3.66 s |
| lint | `bun --no-env-file run lint` | PASS | 4.95 s |
| quick-setup-browser | `python -u app\tests\aurora\quick-setup-browser.py` | PASS | 12.4 s |
| ai-setup-browser | `python -u app\tests\aurora\ai-setup-browser.py` | PASS | 10.14 s |
| workspace-browser | `python -u app\tests\aurora\workspace-browser.py` | PASS | 12.29 s |
| composer-browser | `python -u app\tests\aurora\composer-browser.py` | PASS | 28.25 s |
| startup-browser | `python -u app\tests\aurora\startup-browser.py` | PASS | 22.3 s |
| go-serial | `go test -json -p=1 -count=1 -timeout=300s ./...` | PASS | 276.56 s |
| web-assets | `go test -count=1 ./internal/web` | PASS | 5.32 s |
| vet | `go vet ./...` | PASS | 5.51 s |
| build | `go build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-30-quick-setup\neko-aurora.exe ./cmd/neko-aurora` | PASS | 9.76 s |
| syntax | `python .audit/2026-10-28-grok/syntax.py` | PASS | 12.64 s |

Full Go ghi 2122 kết quả PASS, 38 SKIP, 0 FAIL, tính cả ca con. SKIP vẫn là SKIP. Core có 74 test PASS cho bridge mới, OAuth, provider choice và config. Go integration đọc catalog từ Core thật trong home tạm; streaming chọn profile được kiểm tra bằng executable fixture. Browser kiểm tra lưu lỗi/thử lại, reload, workspace khác, lựa chọn cũ, hủy/thất bại/thành công đăng nhập giả lập, kích hoạt rõ ràng, không tự gửi, giữ draft, VI/EN, desktop 1440×1000 và viewport 390×844. Ảnh desktop/điện thoại đã được xem.

Kiểm tra preview đầu tiên phát hiện thư mục trên ổ E: có owner nhưng chỉ được cấp Modify; yêu cầu WRITE_OWNER không cần thiết khiến catalog bị vô hiệu hóa. Đã sửa để xác minh owner hiện tại rồi chỉ dùng WRITE_DAC để đặt ACL riêng, thêm ca Windows Modify-only và chạy lại toàn bộ gate. Probe trực tiếp trên đúng thư mục state xác nhận quyền riêng và catalog 7 provider.

Giữ lại các lần FAIL trong audit: lỗi compile tên field trong fixture, lint kiểu event, dấu ngoặc fixture Python đã được sửa; một lần composer vướng assertion cửa sổ quote rồi hai lượt sau PASS. Không nới assertion hay tuyên bố sửa composer từ kết quả thử lại này.

## Bản đang chạy và giới hạn

Đã build Go mới, kiểm tra manifest/hash và khởi động lại đúng preview tại `http://127.0.0.1:63501/app/`, giữ state cũ. Host/Tauri binary giữ nguyên hash đã kiểm chứng ở lượt trước; Rust không đổi. Tab bên phải đã được tải lại và kiểm tra trực tiếp có GLM/API-key và ChatGPT/sign-in. Bằng chứng local: `.audit/2026-10-30-quick-setup/preview-quick-setup.png`, `preview-chatgpt.png`, `launch-result.json`, `validation.jsonl`, manifests và log từng lệnh. Các artifacts/state không được commit.

Trong kiểm thử do Codex thực hiện: không nhập key thật, không đăng nhập tài khoản thật, không gọi GLM/ChatGPT trả phí. Sau phần kiểm chứng giao diện, tab chuyển sang trạng thái chờ đăng nhập ChatGPT; Codex giữ nguyên để người dùng tiếp tục, không coi đây là bằng chứng OAuth remote đã thành công. Lưu key chỉ xác nhận lưu cấu hình, không xác minh key/quota. OAuth remote, khả năng gọi model của tài khoản thật, catalog model theo tài khoản và chất lượng câu trả lời cần người dùng tự đăng nhập/nhập khóa để thử. Đây không phải model evaluation, cloud sync, kiểm thử điện thoại vật lý hay bản production release.
