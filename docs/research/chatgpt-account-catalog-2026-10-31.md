# ChatGPT: danh sách theo tài khoản và lỗi model — 2026-10-31

Code chính: `db96f667925e6acb4d06380c3a63d68dab36745f`, base `63749587c84ced19a61883d7455c396a5838c90f`; 15 gate PASS trên source manifest SHA-256 `3017af417c6f6ea4c085ccfbc4a7f3683e00e7db542d174a2e13a40156ddcc58`. Follow-up UI: `5cc9f37682a829f11c2e87515dbcf786ffa2a259`, source manifest `4fdb1647461318f792d419debf7e78af859b3767a0c859eb9e25b548a408033d`; sửa cảnh báo model cũ sau khi lưu và thêm browser regression. Bốn gate UI/build được chạy lại trên follow-up. Diff manifest xác nhận chỉ app.js và test browser đổi; Go/Core không đổi, không chạy lại full Go/Core cho follow-up. Không có file nguồn đổi trong bất kỳ gate nào. Làm solo, không dùng subagent.

## Bằng chứng và phạm vi kết luận

Người dùng gửi “yo” lúc 00:10 và nhận lỗi chung sau bản sửa CLI trước. Lý do gốc từ stderr của lượt đó đã bị bản cũ bỏ đi; không thể khẳng định lại HTTP status của lượt thất bại từ journal chỉ còn `core_failed`.

Đã đọc catalog của kết nối ChatGPT hiện tại qua adapter Core, chỉ cho phép GET tới endpoint models, chặn refresh credential và mọi endpoint khác trong probe. Kết quả có tám model: GPT-6.1 Sol, GPT-6 Astra, GPT-6.1 Terra, GPT-6 Sol, GPT-6 Luna và ba GPT-5.6. Không có `gpt-5.5` đang được bot chọn từ preset tĩnh. Discovery local xác nhận Codex 0.160.0 sẵn sàng qua PATH. Điều này xác nhận sai lệch danh sách và khả năng đọc catalog bằng kết nối hiện tại; không phải bằng chứng một model đã sinh câu trả lời thành công.

Probe chỉ xuất tên model, transport và các boolean hiện diện/hết hạn credential; không in token, account ID hay raw response nhạy cảm. Agent không tự gọi endpoint completion hoặc phát lại hội thoại. Trong lúc kiểm tra, người dùng tự chọn GPT-6.1-Sol và gửi tin mới qua preview: UI hiển thị câu trả lời cho “yo” lúc 00:32. Đây là bằng chứng một lượt chat thật đã trả lời sau thay đổi; không phải đánh giá chất lượng hoặc xác nhận mọi model đều dùng được.

## Thay đổi

- [Core setup bridge](../../core/src/adapters/aurora-setup.ts) lấy danh sách theo tài khoản, tên hiển thị và yêu cầu phiên bản Codex từ các adapter Core hiện có. ChatGPT không còn quảng bá preset cố định như model tài khoản; các provider API khác được ghi rõ là danh sách gợi ý.
- [Catalog Aurora](../../app/native/internal/aurora/quick_models.go) giới hạn thời gian/kích thước stdout, xác minh workspace, giữ cache trong bộ nhớ, hủy cache khi đăng nhập thành công và không nhận kết quả của tài khoản cũ sau một lượt đăng nhập khác.
- Quick setup tải catalog khi mở ChatGPT đã kết nối và sau đăng nhập, có nút tải lại, trạng thái tải/lỗi và tên model theo catalog. Không tự chọn hay tự gửi. Model cũ vẫn được hiển thị nhưng không cho lưu nếu không có trong catalog; model cần Codex mới hơn được đánh dấu không khả dụng.
- Bộ chọn nâng cao cũng giữ nguyên ID không có trong menu; không hiển thị nhầm thành mặc định máy chủ. Header/hồ sơ phản ánh tên model được chọn cho tin nhắn mới, kèm trạng thái không khả dụng nếu catalog đã xác nhận.
- Trước mỗi lượt ChatGPT, Core launcher kiểm tra model trong catalog hiện tại và thành phần hỗ trợ. Lựa chọn không còn trong catalog bị chặn trước completion với thông báo rõ. Lỗi HTTP/auth/quota/network/support được ánh xạ sang mã hữu hạn, không lộ stderr. Thông báo lỗi có nút **AI của bot**.

Không đổi Neko Core agent loop, transport, quyền công cụ, Rust, dependency, billing fallback hoặc tài khoản. Các phiên bản binary đã cài được discovery kiểm tra; lượt này không cài thêm support pack.

## Kiểm thử

Windows amd64, PowerShell 7.6, Go 1.27.1, Bun 1.4.0, Node 25.9.0, Python 3.13.7, Chrome 156.0.8078.49. Go CWD `app/native`, Bun CWD `core`, Python CWD root repo. Môi trường allowlist, test home/XDG/npmrc tách riêng, không kế thừa credentials provider; PSModuleAnalysisCachePath=NUL. Browser dùng profile tạm, HTTP fixtures local, chặn remote HTTPS. Các gate nặng chạy tuần tự.

| Gate | Lệnh chính xác | Kết quả | Thời gian |
|---|---|---|---|
| quick-api | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -run Test(Quick\|Core\|HybridFalls) ./internal/aurora` | PASS | 12.74 s |
| quick-core | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/aurora-setup.test.ts test/chatgpt-auth.test.ts test/provider-choice.test.ts test/config.test.ts` | PASS | 2.14 s |
| chatgpt-routing | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/chatgpt-provider.test.ts test/chatgpt-app-server-provider.test.ts` | PASS | 4.0 s |
| typecheck | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run typecheck` | PASS | 3.77 s |
| lint | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run lint` | PASS | 4.22 s |
| quick-setup-browser | `python -u app\tests\aurora\quick-setup-browser.py` | PASS | 13.42 s |
| ai-setup-browser | `python -u app\tests\aurora\ai-setup-browser.py` | PASS | 10.39 s |
| workspace-browser | `python -u app\tests\aurora\workspace-browser.py` | PASS | 12.58 s |
| composer-browser | `python -u app\tests\aurora\composer-browser.py` | PASS | 28.29 s |
| startup-browser | `python -u app\tests\aurora\startup-browser.py` | PASS | 21.0 s |
| syntax | `python .audit/2026-10-28-grok/syntax.py` | PASS | 4.37 s |
| go-serial | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | PASS | 318.53 s |
| web-assets | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS | 4.53 s |
| vet | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS | 4.06 s |
| build | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-31-chatgpt-catalog\neko-aurora.exe ./cmd/neko-aurora` | PASS | 4.36 s |

Follow-up UI `5cc9f37682a829f11c2e87515dbcf786ffa2a259`:

| Gate | Lệnh chính xác | Kết quả | Thời gian |
|---|---|---|---|
| quick-setup-browser | `python -u app\tests\aurora\quick-setup-browser.py` | PASS | 14.52 s |
| syntax | `python .audit/2026-10-28-grok/syntax.py` | PASS | 4.43 s |
| web-assets | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS | 6.79 s |
| build | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-10-31-chatgpt-catalog\neko-aurora.exe ./cmd/neko-aurora` | PASS | 5.07 s |

Full Go: **2127 PASS, 38 SKIP, 0 FAIL**, tính cả ca con. Core bridge/auth/config: 75 PASS; provider/native routing: 39 PASS. SKIP không tính là PASS. Full Core release gate không chạy vì chỉ đổi bridge riêng của Aurora; đã chạy subsystem regressions, typecheck và lint theo Core testing contract.

Go integration chạy CLI nguồn thật qua Bun với credential tổng hợp và fetch giả lập, bao gồm preflight qua setup bridge thật và chặn model không có trong catalog. Native transport được kiểm tra bằng các fixture Core hiện có; các test tự động không gọi model bằng tài khoản thật. Browser kiểm tra tải/retry catalog, giữ lựa chọn không khả dụng, không tự gửi, đổi model phải bấm lưu, xóa cảnh báo cũ sau lưu thành công, giữ draft, selector nâng cao, reload và VI/EN ở desktop/mobile viewport. Đã xem ảnh viewport hẹp; không coi đó là physical-phone acceptance.

Giữ các lần FAIL: assertion Playwright `to_be_disabled` không phản ánh disabled option dù DOM có thuộc tính; đổi sang kiểm tra thuộc tính disabled. Lint yêu cầu parse error tại biên I/O; helper nhận string sau khi CLI kiểm tra Error. Một batch được dừng chủ động trước full Go để sửa selector nâng cao khi review; 15 gate sau đó PASS trên db96f66. Follow-up UI được kiểm tra riêng như bảng trên.

## Preview và giới hạn

Preview local db96f66 được build vào `app/.build/chatgpt-catalog-20261031`; follow-up UI vào `app/.build/chatgpt-catalog-20261031-r2`. Manifest trỏ đúng code commit tương ứng, kiểm tra hash host/Tauri và chủ sở hữu cổng trước khi thay Go service. State và đăng nhập giữ nguyên tại cổng 63501. Ảnh/launch receipt, catalog probe, secret scan và log/manifest được giữ local trong `.audit/2026-10-31-chatgpt-catalog`, không commit account store hoặc journal. Header hiện ChatGPT Plus/Pro · GPT-6.1-Sol sau lựa chọn và lượt thử của người dùng.

Một lượt completion thật do người dùng gửi đã được quan sát trên UI; không suy rộng thành model evaluation, chất lượng nội dung trả lời, mọi tài khoản/model, cloud sync, production release hay physical-device acceptance. Quyền dùng model/quota có thể đổi sau catalog; lỗi completion vẫn cần xử lý riêng. Agent không tự gửi tin thử, không push/deploy.
