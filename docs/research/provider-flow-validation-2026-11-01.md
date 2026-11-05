# Kiểm chứng provider và hội thoại — 2026-11-01

Code commit: `b782ad4534d4b108d745a51395245401a2920ed8` trên `codex/openmaus-interface`, base `45a03832724b0eb995495e817ff289c61d5f87f7`. Tự triển khai và tự review; không dùng subagent hoặc gọi đó là independent review. Chưa push/deploy.

Source manifest cuối (app/core/tools): `9a2621442f34dd7a17de8e04566582043a4788b0ee057158d8863c792d82c49b`. Nhật ký local: `.audit/2026-11-01-provider-flow/validation.jsonl`; từng bản ghi có command/argv, cwd, environment, thời gian, base, diff hash, source manifest và trạng thái. Source không đổi trong từng lượt kiểm tra. File audit/binary/state không nằm trong source commit.

## Môi trường

Windows amd64, Go 1.27.1, Bun 1.4.0, Node 25.9.0, Python 3.13.7, Chrome 156. Child process nhận allowlist môi trường, home/XDG/npmrc tạm, không nhận credentials/provider environment thật. Browser dùng Chrome profile tạm; chỉ các fixture browser bảo toàn OS USERPROFILE để tìm executable. `PSModuleAnalysisCachePath=NUL`; computer và brief loop tắt trong preview.

`GO` = `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe`; `BUN` = `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe`. Các đường dẫn này mô tả máy kiểm thử, không phải dependency tải về kèm repository.

## Kết quả

| Check | Command chính xác (cwd được ghi trong JSONL) | Kết quả | Thời gian |
|---|---|---|---|
| quick-core | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/aurora-models.test.ts test/aurora-setup.test.ts test/chatgpt-auth.test.ts test/provider-choice.test.ts test/config.test.ts` | PASS | 43.65s |
| typecheck | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run typecheck` | PASS | 5.37s |
| lint | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file run lint` | PASS | 5.52s |
| quick-api | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -run Test(Quick\|Core\|HybridFalls) ./internal/aurora` | PASS | 35.88s |
| ai-setup | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -v -count=1 -run Test(NoKeyIsHonest\|ModelsReportsDefaultConfiguration\|ProviderModelThinkingAndForm)$ ./internal/aurora` | PASS | 13.03s |
| quick-setup-browser | `python -u app\tests\aurora\quick-setup-browser.py` | PASS | 23.45s |
| go-serial | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./...` | FAIL | 458.35s |
| go-affected | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -json -p=1 -count=1 -timeout=300s ./internal/aurora ./internal/hub ./cmd/neko-aurora ./internal/web` | PASS | 133.77s |
| web-assets | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe test -count=1 ./internal/web` | PASS | 10.09s |
| vet | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe vet ./...` | PASS | 6.14s |
| build | `E:/Sach/Sua/TradingCungLong/hinto-trader/.tools/go/bin/go.exe build -o E:\Sach\Sua\neko-aurora\.audit\2026-11-01-provider-flow\neko-aurora.exe ./cmd/neko-aurora` | PASS | 7.3s |
| syntax | `python .audit/2026-10-28-grok/syntax.py` | PASS | 6.54s |
| workspace-browser | `python -u app\tests\aurora\workspace-browser.py` | PASS | 18.77s |
| ai-setup-browser | `python -u app\tests\aurora\ai-setup-browser.py` | PASS | 14.23s |
| composer-browser | `python -u app\tests\aurora\composer-browser.py` | PASS | 36.37s |
| startup-browser | `python -u app\tests\aurora\startup-browser.py` | PASS | 32.72s |
| chatgpt-routing | `C:/Users/Admin/AppData/Roaming/npm/node_modules/bun/bin/bun.exe --no-env-file test test/chatgpt-provider.test.ts test/chatgpt-app-server-provider.test.ts` | PASS | 5.38s |

Core catalog/config/auth: 78 test PASS. API CLI đi qua Go → Bun → Core agent/provider thật cho sáu route API, nhưng toàn bộ HTTP là fixture; kiểm tra endpoint, key, model và phản hồi stream. Các regression ChatGPT kiểm tra transport và xử lý model được giữ riêng. Typecheck/lint trên Core trước bản sửa Go cuối vẫn hợp lệ: toàn bộ file Core giữ nguyên hash.

Lượt toàn bộ Go ghi nhận 2137 test/subtest PASS và một FAIL `TestNoKeyIsHonest`: nhánh chưa có AI để runtime rỗng nên ledger từ chối bắt đầu và test chờ quá hạn. Đã sửa giữ runtime hợp lệ với model rỗng. Sau đó kiểm tra riêng AI setup PASS và chạy lại toàn bộ package bị tác động (`aurora`, `hub`, `cmd/neko-aurora`, `web`): 629 test/subtest PASS, không FAIL. Các package còn lại của lượt toàn bộ đều PASS và source không đổi. Không đổi nhãn command toàn bộ đã FAIL thành PASS; kiểm chứng cuối là tập hợp hai lượt có manifest đối chiếu. `go vet ./...` kiểm tra phần Go cuối và build toàn binary đã chạy lại sau sửa UI. Bản sửa sau preview chỉ thay app.js, service-worker version và browser regression; backend giữ nguyên hash của lượt Go đã đạt. Các browser regression, syntax/assets và build đã chạy lại trên UI cuối.

Các lần typecheck/lint FAIL trước đó phát hiện thiếu union kiểu trả về và parsing JSON thủ công; đã sửa bằng kiểu trả về đầy đủ và Zod schema hiện có. Log FAIL không bị xóa. Sau bỏ typing giả, một lần composer test bắt được scroll event tải lịch sử cũ trước khi khôi phục vị trí cuối; đã khôi phục scroll đồng bộ ngay sau render và giữ nguyên assertion trích dẫn, sau đó kiểm thử lại PASS. Browser kiểm tra cả bảy route, key thay đổi, catalog lỗi, model cũ bị loại, explicit activation, không tự gửi tin, draft, keyboard composer entry, AI đang chạy khác AI tiếp theo, viewport 390px, tiếng Anh và console error. Preview phát hiện lịch sử chạy lại hiệu ứng gõ; đã bỏ animation giả trên phản hồi lưu sẵn, thêm ca reload kiểm tra đầy đủ code/table và không phát lại typing.

## Phạm vi

Không gọi completion bằng khóa/tài khoản thật, không đổi kết nối đang lưu của người dùng. Catalog hợp lệ không chứng minh quota/generation hoặc chất lượng câu trả lời. Không phải cloud sync, physical-device acceptance hoặc release certification. Rust/desktop source không đổi; preview dùng lại binary host/Tauri đã đối chiếu hash, không tuyên bố chạy lại Rust suite ở lượt này.

Xem [nghiên cứu OpenMausBot và hợp đồng](provider-flow-openmaus-2026-11-01.md), [cách sử dụng](../ai-setup.md). Bằng chứng preview, ảnh giao diện và manifest launch lưu trong thư mục audit local nêu trên.

## Preview thực tế

Đã mở lại `http://127.0.0.1:63501/app/#bot-meiiie` từ build `app/.build/provider-flow-20261101-r2`, giữ nguyên state local. Launcher đối chiếu hash và desktop instance binding PASS; chỉ còn một service và một desktop của build này. Tab bên phải đã reload và giữ mở ở phần chọn AI.

Meiiie vẫn chọn **ChatGPT Plus/Pro · GPT-6.1-Sol**. Catalog tài khoản trả về tám lựa chọn, không dùng preset giả. Đã mở cả hai form GLM để kiểm tra hướng dẫn Coding Plan/API, trạng thái cần khóa và chưa có model; không nhập khóa, không lưu lựa chọn mới và không gửi tin nhắn. Reload hiển thị lịch sử đầy đủ ngay, không chạy lại hiệu ứng gõ.

Bằng chứng local: `preview-result.json`, `preview-connected-model.png`, `launch-result.json`, `launcher.log`. Dòng launcher về runtime chưa cấu hình thuộc legacy profile registry, không phản ánh quick setup; trạng thái quick setup được kiểm tra qua UI thật. Một lần helper launch trả exit 1 vì không có listener cũ; đã sửa nhánh không có process để thoát thành công, không bỏ kiểm tra path/PID/hash của process.
